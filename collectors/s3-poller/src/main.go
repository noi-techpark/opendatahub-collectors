// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON       string
	RAW_BINARY bool

	AWS_REGION            string
	AWS_S3_FILE_NAME      string
	AWS_S3_BUCKET_NAME    string
	AWS_ACCESS_KEY_ID     string
	AWS_ACCESS_SECRET_KEY string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	// Create a custom AWS configuration
	customConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(env.AWS_REGION),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(env.AWS_ACCESS_KEY_ID, env.AWS_ACCESS_SECRET_KEY, ""),
		),
	)
	ms.FailOnError(context.Background(), err, "failed to create AWS config")

	// Create an S3 client
	s3Client := s3.NewFromConfig(customConfig)

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		collector.GetInputChannel() <- dc.NewInput[dc.EmptyData](context.Background(), nil)
	})

	slog.Info("Setup complete. Starting cron scheduler")
	go func() {
		c.Run()
	}()

	err = collector.Start(context.Background(), func(ctx context.Context, a dc.EmptyData) (*rdb.RawAny, error) {
		// Get the object from S3
		output, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String(env.AWS_S3_BUCKET_NAME),
			Key:    aws.String(env.AWS_S3_FILE_NAME),
		})
		if err != nil {
			slog.Error("error while getting s3 object:", "err", err, "bucket", env.AWS_S3_BUCKET_NAME, "file", env.AWS_S3_FILE_NAME)
			return nil, err
		}

		defer output.Body.Close()
		body, err := io.ReadAll(output.Body)
		if err != nil {
			slog.Error("error reading response body:", "err", err)
			return nil, err
		}

		var raw any
		if env.RAW_BINARY {
			raw = body
		} else {
			raw = string(body)
		}

		return &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   raw,
		}, nil
	})
	ms.FailOnError(context.Background(), err, err.Error())
}
