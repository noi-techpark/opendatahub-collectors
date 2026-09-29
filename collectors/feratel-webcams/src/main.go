// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON string

	// Feratel XML feed URL. The language query param is overwritten per request.
	HTTP_URL string
	// Query param that selects the feed language
	LANGUAGE_PARAM string `default:"lg"`
	// Comma separated list of languages to fetch, e.g. "de,it,en"
	LANGUAGES string `default:"de,it,en"`
}

var httpClient = &http.Client{Timeout: 2 * time.Minute}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Feratel webcams data collector...")

	defer tel.FlushOnPanic()

	baseURL, err := url.Parse(env.HTTP_URL)
	ms.FailOnError(context.Background(), err, "failed parsing HTTP_URL")

	languages := parseLanguages(env.LANGUAGES)
	if len(languages) == 0 {
		ms.FailOnError(context.Background(), fmt.Errorf("LANGUAGES is empty"), "invalid configuration")
	}

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		collector.GetInputChannel() <- dc.NewInput[dc.EmptyData](context.Background(), nil)
	})

	slog.Info("Setup complete. Starting cron scheduler", "languages", languages)
	go func() {
		c.Run()
	}()

	err = collector.Start(context.Background(), func(ctx context.Context, a dc.EmptyData) (*rdb.RawAny, error) {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		// Fetch every language. If one fails, the whole poll fails, so the
		// transformer never receives a partially translated record.
		feeds := make(map[string]string, len(languages))
		for _, lang := range languages {
			body, err := fetch(ctx, languageURL(baseURL, env.LANGUAGE_PARAM, lang))
			if err != nil {
				slog.Error("failed to fetch feed", "lang", lang, "err", err)
				return nil, err
			}
			feeds[lang] = body
		}

		raw, err := json.Marshal(feeds)
		if err != nil {
			return nil, fmt.Errorf("could not marshal feeds: %w", err)
		}

		slog.Info("Polling job completed", "runtime_ms", time.Since(jobstart).Milliseconds())
		return &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   string(raw),
		}, nil
	})
	ms.FailOnError(context.Background(), err, "collector failed")
}

func fetch(ctx context.Context, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error during http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http request returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	return string(body), nil
}

// languageURL returns a copy of base with the language query param set to lang
func languageURL(base *url.URL, param string, lang string) string {
	u := *base
	q := u.Query()
	q.Set(param, lang)
	u.RawQuery = q.Encode()
	return u.String()
}

func parseLanguages(s string) []string {
	var langs []string
	for _, l := range strings.Split(s, ",") {
		l = strings.TrimSpace(l)
		if l != "" {
			langs = append(langs, l)
		}
	}
	return langs
}
