// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Env struct {
	dc.Env
	SheetsClientID             string `envconfig:"SHEETS_CLIENT_ID" required:"true"`
	SheetsClientSecret         string `envconfig:"SHEETS_CLIENT_SECRET" required:"true"`
	SheetsRefreshToken         string `envconfig:"SHEETS_REFRESH_TOKEN" required:"true"`
	SpreadsheetID              string `envconfig:"SPREADSHEET_ID" required:"true"`
	SpreadsheetNotificationURL string `envconfig:"SPREADSHEET_NOTIFICATION_URL"`
	TriggerPath                string `envconfig:"TRIGGER_PATH" default:"/trigger"`
	TriggerPort                string `envconfig:"TRIGGER_PORT" default:"8082"`
	TriggerMaxUpdateFrequency  int    `envconfig:"TRIGGER_MAX_UPDATE_FREQUENCY" default:"10000"`
	GoogleWatchExpirationHours int    `envconfig:"GOOGLE_WATCH_EXPIRATION_HOURS" default:"24"`
}

var env Env
var (
	currentChannelToken string
	tokenMu             sync.RWMutex
	triggerChan         = make(chan struct{}, 10)
)

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Go Google Spreadsheet Data Collector...")
	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	ctx := context.Background()
	config := &oauth2.Config{
		ClientID:     env.SheetsClientID,
		ClientSecret: env.SheetsClientSecret,
		Endpoint:     google.Endpoint,
	}
	token := &oauth2.Token{
		RefreshToken: env.SheetsRefreshToken,
	}
	httpClient := config.Client(ctx, token)

	driveService, err := drive.NewService(ctx, option.WithHTTPClient(httpClient))
	ms.FailOnError(ctx, err, "failed to create Google Drive service")

	sheetsService, err := sheets.NewService(ctx, option.WithHTTPClient(httpClient))
	ms.FailOnError(ctx, err, "failed to create Google Sheets service")

	if env.SpreadsheetNotificationURL != "" {
		go watchLoop(ctx, driveService)
	} else {
		slog.Warn("SPREADSHEET_NOTIFICATION_URL is empty; file watch notifications disabled")
	}

	go debounceHandler(ctx, collector, sheetsService)

	// initial sync after startup
	go func() {
		time.Sleep(5 * time.Second)
		slog.Info("Triggering initial spreadsheet sync...")
		triggerChan <- struct{}{}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	triggerPath := normalizeTriggerPath(env.TriggerPath)

	http.HandleFunc(triggerPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		channelTokenHeader := r.Header.Get("X-Goog-Channel-Token")
		tokenMu.RLock()
		activeToken := currentChannelToken
		tokenMu.RUnlock()

		if activeToken == "" || channelTokenHeader != activeToken {
			slog.Warn("Trigger token mismatch or watch inactive",
				"headerToken", channelTokenHeader, "activeToken", activeToken)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		slog.Info("Valid watch notification received, triggering sync")
		select {
		case triggerChan <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("Starting HTTP server", "port", env.TriggerPort, "triggerPath", triggerPath)
	err = http.ListenAndServe(":"+env.TriggerPort, nil)
	ms.FailOnError(ctx, err, "HTTP server failed")
}

func watchLoop(ctx context.Context, driveService *drive.Service) {
	for {
		channelID := uuid.New().String()
		channelToken := uuid.New().String()

		expirationMs := time.Now().Add(time.Duration(env.GoogleWatchExpirationHours) * time.Hour).UnixMilli()

		channel := &drive.Channel{
			Id:         channelID,
			Type:       "web_hook",
			Address:    env.SpreadsheetNotificationURL,
			Token:      channelToken,
			Expiration: expirationMs,
		}

		slog.Info("Creating Google Drive watch channel...",
			"channelID", channelID, "url", env.SpreadsheetNotificationURL)
		_, err := driveService.Files.Watch(env.SpreadsheetID, channel).Do()
		if err != nil {
			slog.Error("Failed to set up file watcher", "err", err)
			time.Sleep(1 * time.Minute)
			continue
		}

		tokenMu.Lock()
		currentChannelToken = channelToken
		tokenMu.Unlock()

		slog.Info("File watcher active",
			"channelID", channelID, "expiration", time.UnixMilli(expirationMs))

		// also trigger a sync on each watch renewal
		select {
		case triggerChan <- struct{}{}:
		default:
		}

		sleepDuration := time.Until(time.UnixMilli(expirationMs)) - 1*time.Minute
		if sleepDuration <= 0 {
			sleepDuration = 10 * time.Second
		}
		time.Sleep(sleepDuration)
	}
}

func debounceHandler(ctx context.Context, collector *dc.Dc[dc.EmptyData], sheetsService *sheets.Service) {
	var timer *time.Timer
	debounceDuration := time.Duration(env.TriggerMaxUpdateFrequency) * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return
		case <-triggerChan:
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounceDuration, func() {
				syncSpreadsheet(ctx, collector, sheetsService)
			})
		}
	}
}

type leanSpreadsheet struct {
	SpreadsheetID string      `json:"spreadsheetId"`
	Sheets        []leanSheet `json:"sheets"`
}

type leanSheet struct {
	Properties leanSheetProps `json:"properties"`
	Data       []leanGrid    `json:"data"`
}

type leanSheetProps struct {
	Title string `json:"title"`
	Index int    `json:"index"`
}

type leanGrid struct {
	RowData []leanRow `json:"rowData"`
}

type leanRow struct {
	Values []leanCell `json:"values"`
}

type leanCell struct {
	FormattedValue string `json:"formattedValue,omitempty"`
}

func toLean(spreadsheet *sheets.Spreadsheet) leanSpreadsheet {
	lean := leanSpreadsheet{SpreadsheetID: spreadsheet.SpreadsheetId}
	for _, s := range spreadsheet.Sheets {
		ls := leanSheet{
			Properties: leanSheetProps{
				Title: s.Properties.Title,
				Index: int(s.Properties.Index),
			},
		}
		for _, gd := range s.Data {
			lg := leanGrid{}
			for _, rd := range gd.RowData {
				lr := leanRow{}
				for _, cd := range rd.Values {
					lr.Values = append(lr.Values, leanCell{FormattedValue: cd.FormattedValue})
				}
				lg.RowData = append(lg.RowData, lr)
			}
			ls.Data = append(ls.Data, lg)
		}
		lean.Sheets = append(lean.Sheets, ls)
	}
	return lean
}

func syncSpreadsheet(ctx context.Context, collector *dc.Dc[dc.EmptyData], sheetsService *sheets.Service) {
	jobstart := time.Now()
	slog.Info("Starting spreadsheet sync...")

	cCtx, collection := collector.StartCollection(ctx)
	defer collection.End(cCtx)

	spreadsheet, err := sheetsService.Spreadsheets.Get(env.SpreadsheetID).
		IncludeGridData(true).
		Context(cCtx).
		Do()
	if err != nil {
		slog.Error("Failed to fetch spreadsheet", "err", err)
		return
	}

	lean := toLean(spreadsheet)
	jsonBytes, err := json.Marshal(lean)
	if err != nil {
		slog.Error("Failed to marshal spreadsheet", "err", err)
		return
	}

	slog.Info("Spreadsheet data stripped to cell values", "sheets", len(lean.Sheets), "bytes", len(jsonBytes))

	err = collection.Publish(cCtx, &rdb.RawAny{
		Provider:  env.PROVIDER,
		Timestamp: time.Now(),
		Rawdata:   string(jsonBytes),
	})
	if err != nil {
		slog.Error("Failed to publish raw spreadsheet", "err", err)
		return
	}

	slog.Info("Spreadsheet sync completed", "runtime_ms", time.Since(jobstart).Milliseconds())
}

func normalizeTriggerPath(path string) string {
	if path == "" {
		return "/trigger"
	}
	if path[0] != '/' {
		return "/" + path
	}
	return path
}
