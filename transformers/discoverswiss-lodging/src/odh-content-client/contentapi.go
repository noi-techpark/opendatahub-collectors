// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentClient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/http"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type RawFilterId struct {
	Items []struct {
		Id string `json:"Id"`
	} `json:"Items"`
}

func contentApiSpan(ctx context.Context, url, method string) (context.Context, trace.Span) {
	ctx, clientSpan := tel.TraceStart(
		ctx,
		fmt.Sprintf("Content Api: [%s] %s", method, url),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	clientSpan.SetAttributes(
		attribute.String("db.name", "content-api"),
		attribute.String("peer.host", "content-api"),
	)
	return ctx, clientSpan
}

func GetAccomodationIdByRawFilter(ctx context.Context, id string, baseURL string) (string, error) {
	ctx, clientSpan := contentApiSpan(ctx, baseURL, "GET")
	defer clientSpan.End()

	newurl, err := url.Parse(fmt.Sprintf(baseURL, id))
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	client := retryablehttp.NewClient()
	client.HTTPClient.Transport = &http.TracingRoundTripper{}

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", newurl.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("error during http request: %w", err)
	}

	defer resp.Body.Close()

	var rawFilterId RawFilterId

	err = json.NewDecoder(resp.Body).Decode(&rawFilterId)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not decode response: %w", err)
	}

	if len(rawFilterId.Items) > 0 {
		return rawFilterId.Items[0].Id, nil
	} else {
		return "", nil
	}

}

// Option 1: Using TokenSource (automatic refresh)
func GetAccessToken(tokenURL, clientID, clientSecret string) (oauth2.TokenSource, error) {
	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
	}

	ctx := context.Background()
	ts := config.TokenSource(ctx)

	// Verify the credentials work by getting initial token
	_, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return ts, nil
}

func PutContentApi(ctx context.Context, url *url.URL, token string, payload interface{}, id string) (string, error) {
	ctx, clientSpan := contentApiSpan(ctx, url.String(), "PUT")
	defer clientSpan.End()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not marshal payload: %w", err)
	}

	u := fmt.Sprintf("%s/%s", url.String(), id)

	newurl, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, "PUT", newurl.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not create http request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := retryablehttp.NewClient()
	client.HTTPClient.Transport = &http.TracingRoundTripper{}

	resp, err := client.Do(req)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("error during http request: %w", err)
	}

	return strconv.Itoa(resp.StatusCode), nil
}

func PostContentApi(ctx context.Context, url *url.URL, token string, payload interface{}) (string, error) {
	ctx, clientSpan := contentApiSpan(ctx, url.String(), "POST")
	defer clientSpan.End()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not marshal payload: %w", err)
	}
	u := url

	req, err := retryablehttp.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("could not create http request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := retryablehttp.NewClient()
	client.HTTPClient.Transport = &http.TracingRoundTripper{}

	resp, err := client.Do(req)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("error during http request: %w", err)
	}

	return strconv.Itoa(resp.StatusCode), nil
}
