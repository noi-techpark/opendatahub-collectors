// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/go-silky"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var env struct {
	dc.Env
	CRON        string
	CONFIG_PATH string
}

// xmlInterceptClient wraps an HTTP client and transparently converts XML responses to JSON
// so that the apigorowler/silky framework (which always JSON-decodes responses) can handle them.
type xmlInterceptClient struct {
	inner *http.Client
}

func (c *xmlInterceptClient) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "xml") && !strings.Contains(contentType, "text/html") {
		return resp, nil
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read XML response body: %w", err)
	}

	jsonBytes, err := convertXMLToJSON(body)
	if err != nil {
		// If conversion fails, wrap raw XML as a JSON string so the pipeline doesn't panic
		slog.Warn("XML to JSON conversion failed, wrapping raw", "err", err)
		jsonBytes, _ = json.Marshal(string(body))
	}

	resp.Body = io.NopCloser(bytes.NewReader(jsonBytes))
	resp.Header.Set("Content-Type", "application/json")
	resp.ContentLength = int64(len(jsonBytes))
	return resp, nil
}

// convertXMLToJSON parses arbitrary XML into a nested map and marshals it to JSON.
// Attributes are stored with an "@" prefix, text content under "#text".
// Repeated sibling elements are automatically collected into arrays.
func convertXMLToJSON(data []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false

	// stack of (tagName, map) pairs
	type frame struct {
		key  string
		node map[string]interface{}
	}
	var stack []frame

	var root interface{}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML decode error: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			node := map[string]interface{}{}
			for _, attr := range t.Attr {
				node["@"+attr.Name.Local] = attr.Value
			}
			stack = append(stack, frame{key: t.Name.Local, node: node})

		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			// Collapse nodes that only have "#text" to a plain string
			var val interface{} = top.node
			if len(top.node) == 1 {
				if text, ok := top.node["#text"]; ok {
					val = text
				}
			} else if len(top.node) == 0 {
				val = ""
			}

			if len(stack) == 0 {
				// This is the root element
				root = map[string]interface{}{top.key: val}
			} else {
				parent := stack[len(stack)-1].node
				if existing, exists := parent[top.key]; exists {
					switch v := existing.(type) {
					case []interface{}:
						parent[top.key] = append(v, val)
					default:
						parent[top.key] = []interface{}{v, val}
					}
				} else {
					parent[top.key] = val
				}
			}

		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text != "" && len(stack) > 0 {
				top := stack[len(stack)-1]
				if existing, ok := top.node["#text"]; ok {
					top.node["#text"] = existing.(string) + text
				} else {
					top.node["#text"] = text
				}
			}
		}
	}

	if root == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(root)
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	craw, errors, err := silky.NewApiCrawler(env.CONFIG_PATH)
	ms.FailOnError(context.Background(), err, "failed to load call config", "validation", errors)

	retryClient := retryablehttp.NewClient()
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, checkErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		if resp != nil && (resp.StatusCode >= 400 && resp.StatusCode < 500) {
			if resp.StatusCode != 429 {
				return false, fmt.Errorf("unrecoverable client error: %d", resp.StatusCode)
			}
		}
		return shouldRetry, checkErr
	}

	// Wrap the retryable standard client with our XML interceptor
	craw.SetClient(&xmlInterceptClient{inner: retryClient.StandardClient()})

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, c := collector.StartCollection(context.Background())
		defer c.End(ctx)

		logger.Get(ctx).Debug("collecting")

		crawlCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		if craw.GetDataStream() != nil {
			go func(ctx context.Context) {
				defer tel.FlushOnPanic()
				for {
					select {
					case <-ctx.Done():
						return
					case stream_d, ok := <-craw.GetDataStream():
						if !ok {
							return
						}
						enc_data, err := json.Marshal(stream_d)
						ms.FailOnError(ctx, err, "failed to encode data", "err", err, "data", stream_d)

						pubCtx := ctx
						var pubSpan trace.Span = noop.Span{}
						rootContext := trace.SpanContextFromContext(ctx)
						if rootContext.IsValid() {
							pubCtx, pubSpan = tel.TraceStart(context.Background(), fmt.Sprintf("%s.data-stream", tel.GetServiceName()),
								trace.WithLinks(trace.Link{
									SpanContext: rootContext,
								}),
								trace.WithSpanKind(trace.SpanKindInternal),
							)
						}

						err = c.Publish(pubCtx, &rdb.RawAny{
							Provider:  env.PROVIDER,
							Timestamp: time.Now(),
							Rawdata:   string(enc_data),
						})
						ms.FailOnError(pubCtx, err, "failed to publish", "err", err)
						pubSpan.End()
					}
				}
			}(crawlCtx)
		}

		err = craw.Run(crawlCtx, map[string]any{})
		ms.FailOnError(ctx, err, "failed to crawl", "err", err)

		if craw.GetDataStream() == nil {
			enc_data, err := json.Marshal(craw.GetData())
			ms.FailOnError(ctx, err, "failed to encode data", "err", err, "data", craw.GetData())

			err = c.Publish(ctx, &rdb.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   string(enc_data),
			})
			ms.FailOnError(ctx, err, "failed to publish", "err", err)
		}

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}
