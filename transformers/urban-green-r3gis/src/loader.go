// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"

	"opendatahub.com/tr-traffic-event-prov-bz/dto"
)

const DefaultBatchSize = 200

// UrbanGreenLoader handles loading and syncing urban green data from CSV files
type UrbanGreenLoader struct {
	contentClient clib.ContentAPI
	standards     *Standards
	batchSize     int
}

// NewUrbanGreenLoader creates a new loader instance
func NewUrbanGreenLoader(client clib.ContentAPI, standards *Standards, batchSize int) *UrbanGreenLoader {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &UrbanGreenLoader{
		contentClient: client,
		standards:     standards,
		batchSize:     batchSize,
	}
}

// Load reads a CSV file, maps all rows to UrbanGreen, and upserts in batches
func (l *UrbanGreenLoader) Load(ctx context.Context, filePath string) error {
	slog.Info("Loading urban green data from CSV", "file", filePath, "batchSize", l.batchSize)

	// Load CSV
	rows, err := LoadUrbanGreenCSV(filePath)
	if err != nil {
		return fmt.Errorf("failed to load CSV: %w", err)
	}

	slog.Info("Loaded rows from CSV", "count", len(rows))

	// Map and send in batches
	syncTime := time.Now().UTC()
	batch := make([]odhContentModel.UrbanGreen, 0, l.batchSize)
	totalSent := 0
	errorCount := 0

	for i, raw := range rows {
		urbanGreen, err := MapUrbanGreenRowToUrbanGreen(raw, l.standards, syncTime)
		if err != nil {
			slog.Warn("Failed to map row", "index", i, "id", raw.ID, "code", raw.Code, "error", err)
			errorCount++
			continue
		}

		batch = append(batch, urbanGreen)

		// Send batch when full
		if len(batch) >= l.batchSize {
			if err := l.sendBatch(ctx, batch); err != nil {
				return fmt.Errorf("failed to send batch at row %d: %w", i, err)
			}
			totalSent += len(batch)
			slog.Info("Batch sent", "sent", totalSent, "total", len(rows))
			batch = batch[:0] // Reset batch
		}
	}

	// Send remaining items
	if len(batch) > 0 {
		if err := l.sendBatch(ctx, batch); err != nil {
			return fmt.Errorf("failed to send final batch: %w", err)
		}
		totalSent += len(batch)
	}

	slog.Info("Load completed", "totalSent", totalSent, "errors", errorCount)
	return nil
}

func (l *UrbanGreenLoader) sendBatch(ctx context.Context, batch []odhContentModel.UrbanGreen) error {
	return l.contentClient.PutMultiple(ctx, "UrbanGreen", batch)
}

// LoadUrbanGreenCSV reads the urban green export CSV and returns a slice of UrbanGreenRow
func LoadUrbanGreenCSV(filePath string) ([]dto.UrbanGreenRow, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file is empty or has no data rows")
	}

	var rows []dto.UrbanGreenRow
	for i, record := range records[1:] { // Skip header
		if len(record) < 10 {
			return nil, fmt.Errorf("row %d has insufficient columns: expected 10, got %d", i+2, len(record))
		}

		rows = append(rows, dto.UrbanGreenRow{
			Provider:              record[0],
			SpecVersion:           record[1],
			ID:                    record[2],
			Code:                  record[3],
			AdditionalInformation: record[4],
			State:                 record[5],
			PutOnSite:             record[6],
			RemovedFromSite:       record[7],
			UpdatedAt:             record[8],
			TheGeom:               record[9],
		})
	}

	return rows, nil
}
