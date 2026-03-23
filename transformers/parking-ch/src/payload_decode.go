// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
)

// ErrChunkedPayload is returned when the payload looks like a chunk envelope.
// Chunk reassembly is not yet supported; the message will be Nacked.
var ErrChunkedPayload = errors.New("unsupported chunked payload: reassembly not yet implemented")

// MultiFormatMiddleware wraps a typed handler into a string handler that
// auto-detects and decodes the payload format before forwarding.
// Supported formats (tried in order):
//  1. Plain JSON
//  2. Base64-encoded JSON
//  3. Gzip + Base64-encoded JSON
//
// Chunked envelopes are detected early and explicitly rejected.
func MultiFormatMiddleware[P any](h tr.Handler[P]) tr.Handler[string] {
	return func(ctx context.Context, raw *rdb.Raw[string]) error {
		decoded, err := DecodePayload[P](raw.Rawdata)
		if err != nil {
			return fmt.Errorf("multi-format decode: %w", err)
		}
		return h(ctx, &rdb.Raw[P]{
			Provider:  raw.Provider,
			Timestamp: raw.Timestamp,
			Rawdata:   decoded,
		})
	}
}

// DecodePayload tries to decode a raw string payload into the target type P.
// It attempts formats in order: plain JSON → base64(JSON) → gzip+base64(JSON).
// If the payload looks like a chunked envelope, it returns ErrChunkedPayload.
func DecodePayload[P any](raw string) (P, error) {
	var zero P

	if isChunkedEnvelope(raw) {
		return zero, ErrChunkedPayload
	}

	// 1. Try plain JSON
	if v, err := tryPlainJSON[P](raw); err == nil {
		slog.Debug("payload decoded as plain JSON", "len", len(raw))
		return v, nil
	}

	// 2. Try base64(JSON)
	if v, err := tryBase64JSON[P](raw); err == nil {
		slog.Debug("payload decoded as base64 JSON", "len", len(raw))
		return v, nil
	}

	// 3. Try gzip+base64(JSON)
	if v, err := tryGzipBase64JSON[P](raw); err == nil {
		slog.Debug("payload decoded as gzip+base64 JSON", "len", len(raw))
		return v, nil
	}

	return zero, fmt.Errorf("unable to decode payload: no supported format matched (len=%d)", len(raw))
}

// tryPlainJSON attempts to unmarshal the raw string directly as JSON.
func tryPlainJSON[P any](raw string) (P, error) {
	var v P
	err := json.Unmarshal([]byte(raw), &v)
	return v, err
}

// tryBase64JSON decodes the raw string from standard base64, then unmarshals as JSON.
func tryBase64JSON[P any](raw string) (P, error) {
	var v P
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(decoded, &v)
	return v, err
}

// tryGzipBase64JSON decodes the raw string from standard base64, decompresses with gzip,
// then unmarshals the result as JSON.
func tryGzipBase64JSON[P any](raw string) (P, error) {
	var v P
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return v, err
	}

	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return v, err
	}
	defer reader.Close()

	uncompressed, err := io.ReadAll(reader)
	if err != nil {
		return v, err
	}

	err = json.Unmarshal(uncompressed, &v)
	return v, err
}

// isChunkedEnvelope checks whether the payload looks like a chunk envelope
// by inspecting the first bytes for common chunk metadata field names.
// This is a lightweight heuristic to avoid full JSON parsing on every message.
func isChunkedEnvelope(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	// Inspect only the beginning of the payload for chunk-related keys
	peek := trimmed
	if len(peek) > 512 {
		peek = peek[:512]
	}
	return strings.Contains(peek, `"chunk_index"`) ||
		strings.Contains(peek, `"total_chunks"`) ||
		strings.Contains(peek, `"chunkIndex"`) ||
		strings.Contains(peek, `"totalChunks"`)
}
