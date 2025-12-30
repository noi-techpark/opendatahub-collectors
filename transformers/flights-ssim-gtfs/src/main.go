// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"opendatahub.com/ssim2gtfs"
	ssim "opendatahub.com/ssimparser"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
	AGENCY_NAME string
	AGENCY_URL  string
	AGENCY_TZ   string

	AWS_REGION            string
	AWS_S3_FILE_NAME      string
	AWS_S3_BUCKET_NAME    string
	AWS_ACCESS_KEY_ID     string
	AWS_ACCESS_SECRET_KEY string
}

// Create your own datatype for unmarshalling the Raw Data
type RawType struct {
	File     []byte
	Filename string
	Dir      string
	Mtime    string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	// Create a custom AWS configuration
	awsConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(env.AWS_REGION),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(env.AWS_ACCESS_KEY_ID, env.AWS_ACCESS_SECRET_KEY, ""),
		),
	)
	ms.FailOnError(context.Background(), err, "failed to create AWS config")
	// Create an S3 client
	s3Client := s3.NewFromConfig(awsConfig)

	listener := tr.NewTr[RawType](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawType]) error {
		slog.Info("Incoming SSIM file")

		// parse ssim file
		parser := ssim.NewParser()
		ssimData, err := parser.Parse(bytes.NewReader(r.Rawdata.File))
		if err != nil {
			return fmt.Errorf("cannot parse ssim: %w", err)
		}

		// we write to a tmep dir for conversion
		gtfsFile, err := os.CreateTemp("", "gtfs-*.zip")
		ms.FailOnError(ctx, err, "could not create temp file for gtfs conversion")
		defer os.Remove(gtfsFile.Name())
		defer gtfsFile.Close()

		// Convert to GTFS
		converter := ssim2gtfs.NewSSIMToGTFSConverter(env.AGENCY_NAME, env.AGENCY_URL, env.AGENCY_TZ)
		err = converter.Convert(ssimData, gtfsFile.Name())
		if err != nil {
			return err
		}

		data, err := os.ReadFile(gtfsFile.Name())
		if err != nil {
			return err
		}

		slog.Info("Conversion done. Pushing to S3")

		_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(env.AWS_S3_BUCKET_NAME),
			Key:    aws.String(env.AWS_S3_FILE_NAME),
			Body:   bytes.NewReader(data),
		})
		ms.FailOnError(ctx, err, "cannot push to S3")

		slog.Info("S3 push done")

		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}
