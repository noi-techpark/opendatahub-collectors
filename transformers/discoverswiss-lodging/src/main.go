// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"go.opentelemetry.io/otel/trace"
	odhContentClient "opendatahub.com/tr-discoverswiss-lodging/odh-content-client"
	odhContentMapper "opendatahub.com/tr-discoverswiss-lodging/odh-content-mapper"
	odhContentModel "opendatahub.com/tr-discoverswiss-lodging/odh-content-model"
)

var env struct {
	tr.Env

	ODH_CORE_TOKEN_URL           string
	ODH_CORE_TOKEN_USERNAME      string
	ODH_CORE_TOKEN_PASSWORD      string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string

	ODH_API_CORE_URL string

	RAW_FILTER_URL_TEMPLATE string
}

type contextualAccomodation struct {
	ctx           context.Context
	Id            string
	Accommodation odhContentModel.Accommodation
}

func CloneContext(ctx context.Context) context.Context {
	// Start from a fresh background
	newCtx := context.Background()

	// Copy span if exists
	if span := trace.SpanFromContext(ctx); span != nil && span.SpanContext().IsValid() {
		newCtx = trace.ContextWithSpan(newCtx, span)
	}

	newCtx = logger.WithTracedLogger(newCtx)

	// Add more context values here if needed (user ID, trace ID, etc.)
	return newCtx
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	contentCoreUrl, err := url.Parse(env.ODH_API_CORE_URL)
	ms.FailOnError(context.Background(), err, "could not parse core url")
	slog.Info("core url", "value", contentCoreUrl.String())

	accoChannel := make(chan contextualAccomodation, 400)
	var putChannel = make(chan contextualAccomodation, 1000)
	var postChannel = make(chan contextualAccomodation, 1000)

	go func() {
		for acco := range accoChannel {
			ctx := acco.ctx
			logger.Get(acco.ctx).Debug("getting content accomodation", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id)
			odhID, err := odhContentClient.GetAccomodationIdByRawFilter(
				acco.ctx, acco.Accommodation.Mapping.DiscoverSwiss.Id, env.RAW_FILTER_URL_TEMPLATE,
			)
			ms.FailOnError(acco.ctx, err, "cannot get accomodation from content", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id)
			if len(odhID) > 0 && odhID != "" {
				ctx, _ := tel.TraceStart(
					CloneContext(acco.ctx),
					fmt.Sprintf("%s.put", tel.GetServiceName()),
					trace.WithSpanKind(trace.SpanKindInternal),
				)

				acco.ctx = ctx
				acco.Id = odhID
				putChannel <- acco
			} else {
				ctx, _ := tel.TraceStart(
					CloneContext(acco.ctx),
					fmt.Sprintf("%s.post", tel.GetServiceName()),
					trace.WithSpanKind(trace.SpanKindInternal),
				)

				acco.ctx = ctx
				postChannel <- acco
			}

			trace.SpanFromContext(ctx).End()
		}
	}()

	go func() {
		tokenSource, err := odhContentClient.GetAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
		ms.FailOnError(context.Background(), err, "failed to instantiate oauth token source")

		for acco := range putChannel {
			logger.Get(acco.ctx).Debug("putting content accomodation",
				"id", acco.Accommodation.Mapping.DiscoverSwiss.Id,
				"content_id", acco.Id)

			puttoken, err := tokenSource.Token()
			ms.FailOnError(acco.ctx, err, "failed to get content api token")

			_, err = odhContentClient.PutContentApi(acco.ctx, contentCoreUrl, puttoken.AccessToken, acco.Accommodation, acco.Id)
			ms.FailOnError(acco.ctx, err, "failed to put content accomodation", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id,
				"content_id", acco.Id)

			slog.Debug("put ok", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id)
			trace.SpanFromContext(acco.ctx).End()
		}
	}()

	go func() {
		token, err := odhContentClient.GetAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
		ms.FailOnError(context.Background(), err, "failed to instantiate oauth token source")

		for acco := range postChannel {
			logger.Get(acco.ctx).Debug("posting content accomodation",
				"id", acco.Accommodation.Mapping.DiscoverSwiss.Id)

			posttoken, err := token.Token()
			ms.FailOnError(acco.ctx, err, "failed to get content api token")

			_, err = odhContentClient.PostContentApi(acco.ctx, contentCoreUrl, posttoken.AccessToken, acco.Accommodation)
			ms.FailOnError(acco.ctx, err, "failed to post content accomodation", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id)

			slog.Debug("post ok", "id", acco.Accommodation.Mapping.DiscoverSwiss.Id)
			trace.SpanFromContext(acco.ctx).End()
		}
	}()

	listener := tr.NewTr[odhContentModel.LodgingBusiness](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[odhContentModel.LodgingBusiness]) error {

		logger.Get(ctx).Info("Processing accomodation", "id", r.Rawdata.Identifier)
		acco := odhContentMapper.MapLodgingBusinessToAccommodation(r.Rawdata)

		// we need to create a new span since the one in start will end after return
		// by doing so we also need to clone the context to a fresh new one since ctx will be canceled after return
		ctx, _ = tel.TraceStart(
			CloneContext(ctx),
			fmt.Sprintf("%s.async-process", tel.GetServiceName()),
			trace.WithSpanKind(trace.SpanKindInternal),
		)

		accoChannel <- contextualAccomodation{ctx: ctx, Accommodation: acco, Id: ""}
		return nil

	})
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}
