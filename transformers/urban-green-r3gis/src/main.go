// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

const (
	UrbanGreenIDTemplate = "urn:urbangreen:r3gis"
	UrbanGreenSource     = "R3GIS"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string

	MODE           string
	LOAD_FILE_PATH string
	BATCH_SIZE     int `default:"200"`
}

var timeNow = time.Now
var contentClient clib.ContentAPI
var StandardsProto *Standards

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")

	StandardsProto, err = LoadStandards("../resources")
	ms.FailOnError(context.Background(), err, "failed to load standards")

	err = SyncUrbanGreenTags(context.Background(), contentClient, StandardsProto)
	ms.FailOnError(context.Background(), err, "failed to sync standards tags")

	if env.MODE == "loader" && env.LOAD_FILE_PATH != "" {
		slog.Info("Running in loader mode", "file", env.LOAD_FILE_PATH, "batchSize", env.BATCH_SIZE)

		loader := NewUrbanGreenLoader(contentClient, StandardsProto, env.BATCH_SIZE)
		err = loader.Load(context.Background(), env.LOAD_FILE_PATH)
		ms.FailOnError(context.Background(), err, "failed to load urban green data")

		slog.Info("Loader completed successfully")
	} else {
		listener := tr.NewTr[string](context.Background(), env.Env)

		err = listener.Start(context.Background(), tr.RawBase64JsonMiddleware(Transform))
		ms.FailOnError(context.Background(), err, "error while listening to queue")
	}
}

// Transform processes a single UrbanGreen JSON message from the raw data bridge
func Transform(ctx context.Context, r *rdb.Raw[dto.UrbanGreenMessage]) error {
	logger.Get(ctx).Info("Processing urban green message",
		"method", r.Rawdata.Method,
		"id", r.Rawdata.Id,
		"greenCode", r.Rawdata.GreenCode,
	)

	syncTime := timeNow().UTC().Truncate(time.Millisecond)

	mapped, err := MapUrbanGreenMessageToUrbanGreen(r.Rawdata, StandardsProto, syncTime)
	if err != nil {
		return fmt.Errorf("failed to map message: %w", err)
	}

	if strings.EqualFold(r.Rawdata.Method, "DELETE") {
		mapped.Active = false
	}

	err = contentClient.PutMultiple(ctx, "UrbanGreen", []odhContentModel.UrbanGreen{mapped})
	if err != nil {
		return fmt.Errorf("failed to upsert urban green: %w", err)
	}

	logger.Get(ctx).Info("Successfully processed urban green message",
		"method", r.Rawdata.Method,
		"id", r.Rawdata.Id,
	)
	return nil
}
