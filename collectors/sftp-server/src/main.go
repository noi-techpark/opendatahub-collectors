// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	dc.Env
	WATCH_DIR         string `envDefault:"/data"`
	STABILITY_SECONDS int    `envDefault:"5"`
	CHECK_INTERVAL    int    `envDefault:"1"`
}

type File struct {
	Filename string
	Dir      string
	Mtime    time.Time
	File     []byte
}

type fileTracker struct {
	lastModTime time.Time
	lastSize    int64
	stableAt    time.Time
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data collector...")
	defer tel.FlushOnPanic()

	collector := dc.NewDc[File](ctx, env.Env)
	ch := collector.GetInputChannel()

	go ms.FailOnError(context.Background(), collector.Start(context.Background(), func(ctx context.Context, rw File) (*rdb.RawAny, error) {
		raw := rdb.RawAny{}
		raw.Provider = env.PROVIDER
		raw.Timestamp = rw.Mtime
		raw.Rawdata = rw
		return &raw, nil
	}), "collection terminated unexpectedly")

	ms.FailOnError(ctx, watchFiles(ctx, env.WATCH_DIR, ch), "file watcher terminated unexpectedly")
}

func watchFiles(ctx context.Context, dir string, ch chan<- dc.Input[File]) error {
	// Setup filewatcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	defer watcher.Close()

	err = watcher.Add(dir)
	if err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	slog.Info("Watching directory for file uploads", "dir", env.WATCH_DIR)

	tracked := make(map[string]*fileTracker)
	stabilityDuration := time.Duration(env.STABILITY_SECONDS) * time.Second
	checkInterval := time.Duration(env.CHECK_INTERVAL) * time.Second
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed unexpectedly")
			}

			// Track new files or modifications
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if info, err := os.Stat(event.Name); err == nil && !info.IsDir() {
					// Start tracking this file
					if _, exists := tracked[event.Name]; !exists {
						slog.Debug("Started tracking file", "path", event.Name)
						tracked[event.Name] = &fileTracker{}
					}
				}
			}

		case err := <-watcher.Errors:
			if err != nil {
				return fmt.Errorf("file watcher error: %w", err)
			}

		case <-ticker.C:
			// Check all tracked files for stability
			for filePath, tracker := range tracked {
				info, err := os.Stat(filePath)
				if err != nil {
					// File deleted or inaccessible
					slog.Warn("File no longer accessible, stopping tracking", "path", filePath)
					delete(tracked, filePath)
					continue
				}

				currentModTime := info.ModTime()
				currentSize := info.Size()

				// Check if file has changed
				if currentModTime != tracker.lastModTime || currentSize != tracker.lastSize {
					// File changed, reset stability timer
					tracker.lastModTime = currentModTime
					tracker.lastSize = currentSize
					tracker.stableAt = time.Now().Add(stabilityDuration)
				} else if !tracker.stableAt.IsZero() && time.Now().After(tracker.stableAt) {
					// File is stable, transfer complete
					slog.Info("File transfer complete", "path", filePath)

					fileData, err := os.ReadFile(filePath)
					if err != nil {
						return fmt.Errorf("failed to read file: %w", err)
					}

					raw := File{
						Filename: info.Name(),
						Dir:      filepath.Dir(filePath),
						Mtime:    currentModTime,
						File:     fileData,
					}

					ch <- dc.NewInput(ctx, raw)

					// Remove from tracking
					delete(tracked, filePath)
				}
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
