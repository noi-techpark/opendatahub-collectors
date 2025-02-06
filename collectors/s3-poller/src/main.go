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
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dc"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
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
	slog.Info("Starting data collector...")
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	// Create a custom AWS configuration
	customConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(env.AWS_REGION),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(env.AWS_ACCESS_KEY_ID, env.AWS_ACCESS_SECRET_KEY, ""),
		),
	)
	ms.FailOnError(err, "failed to create AWS config")

	// Create an S3 client
	s3Client := s3.NewFromConfig(customConfig)

	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		// Get the object from S3
		output, err := s3Client.GetObject(context.Background(), &s3.GetObjectInput{
			Bucket: aws.String(env.AWS_S3_BUCKET_NAME),
			Key:    aws.String(env.AWS_S3_FILE_NAME),
		})
		if err != nil {
			slog.Error("error while getting s3 object:", "err", err, "bucket", env.AWS_S3_BUCKET_NAME, "file", env.AWS_S3_FILE_NAME)
			return
		}

		defer output.Body.Close()
		body, err := io.ReadAll(output.Body)
		if err != nil {
			slog.Error("error reading response body:", "err", err)
			return
		}

		var raw any
		if env.RAW_BINARY {
			raw = body
		} else {
			raw = string(body)
		}

		mq <- dto.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   raw,
		}
		slog.Info("Polling job completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	slog.Info("Setup complete. Starting cron scheduler")
	c.Run()
}
