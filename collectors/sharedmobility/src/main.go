// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
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
	"strconv"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON string

	RAW_BINARY bool

	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`

	PAGING_SIZE int
}

const DEFAULT_PAGE_SIZE = 50

func pageSize() int {
	if env.PAGING_SIZE > 0 {
		return env.PAGING_SIZE
	}
	return DEFAULT_PAGE_SIZE
}

func pagedURL(base *url.URL, offset int) string {
	urlCopy := *base
	q := urlCopy.Query()
	q.Set("offset", strconv.Itoa(offset))
	urlCopy.RawQuery = q.Encode()
	return urlCopy.String()
}

func main() {
	ctx := context.Background()

	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting HTTP polling collector…")

	u, err := url.Parse(env.HTTP_URL)
	ms.FailOnError(ctx, err, "failed parsing poll URL")

	collector := dc.NewDc[dc.EmptyData](ctx, env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		collector.GetInputChannel() <- dc.NewInput[dc.EmptyData](ctx, nil)
	})
	go c.Run()
	slog.Info("Setup complete. Cron scheduler started")

	err = collector.Start(ctx, func(ctx context.Context, _ dc.EmptyData) (*rdb.RawAny, error) {
		var mergedPages []json.RawMessage

		for offset := 0; ; offset += pageSize() {
			pageURL := pagedURL(u, offset)
			req, err := http.NewRequestWithContext(ctx, env.HTTP_METHOD, pageURL, http.NoBody)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Accept", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("non-OK HTTP status: %s", resp.Status)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, err
			}

			var page []json.RawMessage
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, err
			}

			mergedPages = append(mergedPages, page...)

			if len(page) < pageSize() {
				break
			}
		}

		var raw any
		if env.RAW_BINARY {
			raw = mergedPages
		} else {
			buf, err := json.Marshal(mergedPages)
			if err != nil {
				return nil, err
			}
			raw = string(buf)
		}

		return &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   raw,
		}, nil
	})
	ms.FailOnError(ctx, err, "collector terminated unexpectedly")
}
