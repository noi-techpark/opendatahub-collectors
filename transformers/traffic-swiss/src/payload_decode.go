// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
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

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
)

// MultiFormatMiddleware wraps a tr.Handler[T] so that it can accept *rdb.Raw[string]
// payloads encoded as plain JSON, base64+JSON, or gzip+base64+JSON.
// Chunked envelope payloads are rejected with an error.
func MultiFormatMiddleware[T any](handler tr.Handler[T]) tr.Handler[string] {
	return func(ctx context.Context, raw *rdb.Raw[string]) error {
		decoded, err := DecodePayload[T](raw.Rawdata)
		if err != nil {
			return fmt.Errorf("MultiFormatMiddleware: %w", err)
		}
		return handler(ctx, &rdb.Raw[T]{
			Provider:  raw.Provider,
			Timestamp: raw.Timestamp,
			Rawdata:   *decoded,
		})
	}
}

// DecodePayload attempts to decode a string payload in the following order:
//  1. Plain JSON
//  2. Base64-encoded JSON
//  3. Gzip-compressed + Base64-encoded JSON
//
// Returns an error if the payload is a chunked envelope or cannot be decoded.
func DecodePayload[T any](payload string) (*T, error) {
	if IsChunkedEnvelope(payload) {
		return nil, errors.New("chunked envelope payload is not supported")
	}

	// 1. Try plain JSON
	var r1 T
	if err := json.Unmarshal([]byte(payload), &r1); err == nil {
		return &r1, nil
	}

	// Attempt base64 decode for attempts 2 and 3
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("payload is not valid plain JSON or base64: plain JSON and base64 decode both failed")
	}

	// 2. Try base64 + JSON
	var r2 T
	if err := json.Unmarshal(decoded, &r2); err == nil {
		return &r2, nil
	}

	// 3. Try gzip + base64 + JSON
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("payload is not valid base64+JSON or gzip+base64+JSON: gzip open failed: %w", err)
	}
	defer reader.Close()

	jsonBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip decompressed payload: %w", err)
	}

	var r3 T
	if err := json.Unmarshal(jsonBytes, &r3); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gzip+base64+JSON payload: %w", err)
	}
	return &r3, nil
}

// IsChunkedEnvelope returns true if payload is a JSON object containing any
// of the following chunking keys: "chunkIndex", "chunk_index", "totalChunks",
// "total_chunks". Such payloads are not supported and should be rejected.
func IsChunkedEnvelope(payload string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return false
	}
	for _, key := range []string{"chunkIndex", "chunk_index", "totalChunks", "total_chunks"} {
		if _, ok := m[key]; ok {
			return true
		}
	}
	return false
}
