// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	xhttp "github.com/noi-techpark/opendatahub-go-sdk/tel/http"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	ErrUnauthorized   = errors.New("unauthorized")
	ErrAlreadyExists  = errors.New("data exists already")
	ErrNoDataToUpdate = errors.New("data to update not found")
	ErrNoData         = errors.New("no data")
)

// --- Custom Error Handling Structures ---

// ErrorBody is a general-purpose structure to capture common JSON error response formats.
// This structure is designed to catch errors from the API endpoint.
type ErrorBody struct {
	// Common fields for simple error reporting
	ErrorID     int    `json:"error,omitempty"`
	ErrorReason string `json:"errorreason,omitempty"`

	// Common fields for validation/problem details (RFC 7807)
	Type   string `json:"type,omitempty"`
	Title  string `json:"title,omitempty"`
	Status int    `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`

	// For validation errors
	Errors map[string][]string `json:"errors,omitempty"`
}

// APIError wraps an HTTP error response, including the status code and the deserialized body.
type APIError struct {
	StatusCode int
	URL        string
	Body       ErrorBody
	rawBody    []byte
}

// Error implements the error interface.
func (e *APIError) Error() string {
	var b strings.Builder

	// Base context
	fmt.Fprintf(&b, "API request to %s failed with status %d", e.URL, e.StatusCode)

	// RFC 7807-style context
	if e.Body.Title != "" {
		fmt.Fprintf(&b, ": %s", e.Body.Title)
	} else if e.Body.ErrorReason != "" {
		fmt.Fprintf(&b, ": %s", e.Body.ErrorReason)
	}

	// Add detailed description if available
	if e.Body.Detail != "" {
		fmt.Fprintf(&b, " - %s", e.Body.Detail)
	}

	// Include validation errors (if any)
	if len(e.Body.Errors) > 0 {
		fmt.Fprintf(&b, "\nValidation errors:")
		for field, messages := range e.Body.Errors {
			fmt.Fprintf(&b, "\n  • %s: %s", field, strings.Join(messages, "; "))
		}
	}

	// Only show raw body if nothing else is available
	if b.Len() == 0 && len(e.rawBody) > 0 {
		fmt.Fprintf(&b, "\nRaw response: %s", string(e.rawBody))
	}

	return b.String()
}

// Unwrap returns the underlying raw body, useful for inspection.
func (e *APIError) Unwrap() []byte {
	return e.rawBody
}

// --- Client Configuration and Initialization (Unchanged) ---

// Config holds the necessary configuration for the ContentClient.
type Config struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	DisableOAuth bool
	RetryMax     int
	Timeout      time.Duration
}

// ContentClient encapsulates the configuration and shared resources for making
// requests to the Content API. It is safe for concurrent use across goroutines.
type ContentClient struct {
	BaseURL string
	// Internal retryable client for making requests. It uses the configured
	// HTTP client which includes the tracing and optional OAuth transport.
	client *retryablehttp.Client
}

// NewContentClient creates a new, configured ContentClient.
func NewContentClient(cfg Config) (*ContentClient, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// 1. Initialize the base retryable client
	retryClient := retryablehttp.NewClient()
	// Set default configuration values if not specified
	if cfg.RetryMax == 0 {
		cfg.RetryMax = 3 // Default to 3 retries
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second // Default to 30 seconds timeout
	}
	retryClient.Logger = nil                     // Disable default logging
	retryClient.HTTPClient.Timeout = cfg.Timeout // Set overall request timeout

	// 2. Set the base transport for tracing
	baseTransport := &xhttp.TracingRoundTripper{}
	retryClient.HTTPClient.Transport = baseTransport

	// 3. Configure OAuth transport if not disabled
	if !cfg.DisableOAuth {
		if cfg.TokenURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
			return nil, fmt.Errorf("oauth is enabled but TokenURL, ClientID, or ClientSecret is missing")
		}

		config := &clientcredentials.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			TokenURL:     cfg.TokenURL,
		}

		ts := config.TokenSource(context.Background())
		_, err := ts.Token()
		if err != nil {
			return nil, fmt.Errorf("failed to get initial oauth token: %w", err)
		}

		oauthTransport := &oauth2.Transport{
			Source: ts,
			Base:   baseTransport,
		}

		retryClient.HTTPClient.Transport = oauthTransport
	}

	if baseURL.Path != "" && baseURL.Path[len(baseURL.Path)-1] != '/' {
		baseURL.Path += "/"
	}

	return &ContentClient{
		BaseURL: baseURL.String(),
		client:  retryClient,
	}, nil
}

// contentApiSpan creates and starts a new OpenTelemetry span for an API call.
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

// --- Refactored doRequest ---

// doRequest is a private helper to execute the HTTP request using the configured client.
// It centralizes error handling by checking the status code and deserializing any error body.
func (c *ContentClient) doRequest(ctx context.Context, method string, reqURL string, body interface{}) (*http.Response, error) {
	ctx, clientSpan := contentApiSpan(ctx, reqURL, method)
	defer clientSpan.End()

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			clientSpan.RecordError(err)
			clientSpan.SetStatus(codes.Error, "marshalling payload failed")
			return nil, fmt.Errorf("could not marshal payload: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, "request creation failed")
		return nil, fmt.Errorf("could not create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, "http request failed")
		return nil, fmt.Errorf("error during http request: %w", err)
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		clientSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		clientSpan.SetStatus(codes.Error, "unauthorized")
		return nil, ErrUnauthorized
	}

	// Handle non-2xx status codes (HTTP-level errors)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read the body for error message
		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			clientSpan.RecordError(readErr)
			clientSpan.SetStatus(codes.Error, "failed to read error response body")
			return nil, fmt.Errorf("API call failed (Status: %d) but failed to read response body: %w", resp.StatusCode, readErr)
		}

		// 400 string special handling
		bodyString := string(bodyBytes)
		var specialErr error = nil
		switch bodyString {
		case "Data exists already":
			specialErr = ErrAlreadyExists
		case "No Data":
			specialErr = ErrNoData
		case "Data to update Not Found":
			specialErr = ErrNoDataToUpdate
		}
		if specialErr != nil {
			clientSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
			clientSpan.RecordError(specialErr)
			clientSpan.SetStatus(codes.Error, fmt.Sprintf("HTTP %d error", resp.StatusCode))
			return nil, specialErr
		}

		// Attempt to unmarshal the error body
		var errorBody ErrorBody
		unmarshalErr := json.Unmarshal(bodyBytes, &errorBody)

		// Create the custom APIError
		apiError := &APIError{
			StatusCode: resp.StatusCode,
			URL:        reqURL,
			Body:       errorBody,
			rawBody:    bodyBytes,
		}

		// Log error details to the span
		clientSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
		clientSpan.RecordError(apiError)
		clientSpan.SetStatus(codes.Error, fmt.Sprintf("HTTP %d error", resp.StatusCode))

		if unmarshalErr != nil {
			// Note: We return the APIError even if unmarshaling failed, as it contains
			// the status code and raw body. The fact that unmarshaling failed can be
			// seen in the APIError's string representation if needed.
			return nil, fmt.Errorf("API call failed (Status: %d). Failed to unmarshal error body: %w. Original error: %s",
				resp.StatusCode, unmarshalErr, apiError.Error())
		}

		// Return the structured APIError
		return nil, apiError
	}

	return resp, nil
}

// --- Refactored API Methods ---

// Get performs a generic GET request to the Content API.
func (c *ContentClient) Get(ctx context.Context, apiPath string, queryParams map[string]string, responseStruct interface{}) error {
	resourceURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("internal error: could not parse base URL: %w", err)
	}

	// 1. Build the URL path
	resourceURL.Path = path.Join(resourceURL.Path, apiPath)

	// 2. Add query parameters
	if len(queryParams) > 0 {
		q := resourceURL.Query()
		for key, value := range queryParams {
			q.Add(key, value)
		}
		resourceURL.RawQuery = q.Encode()
	}

	// 3. Execute the request
	// The previous errorResponse struct check is now handled centrally in doRequest
	resp, err := c.doRequest(ctx, http.MethodGet, resourceURL.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 4. Decode the response
	target := responseStruct
	if target == nil {
		var defaultMap map[string]interface{}
		target = &defaultMap
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		_, clientSpan := contentApiSpan(ctx, resourceURL.String(), http.MethodGet)
		clientSpan.RecordError(err)
		clientSpan.SetStatus(codes.Error, "decoding response failed")
		clientSpan.End()
		return fmt.Errorf("could not decode response: %w", err)
	}

	return nil
}

// Put performs a PUT request to update a content item by ID.
func (c *ContentClient) Put(ctx context.Context, apiPath string, id string, payload interface{}) error {
	resourceURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("internal error: could not parse base URL: %w", err)
	}
	resourceURL.Path = path.Join(resourceURL.Path, apiPath, id)

	// doRequest now handles error deserialization, so no need for nil here.
	resp, err := c.doRequest(ctx, http.MethodPut, resourceURL.String(), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// PutMultiple performs a PUT request to Upsert a list of entries.
func (c *ContentClient) PutMultiple(ctx context.Context, apiPath string, payload interface{}) error {
	resourceURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("internal error: could not parse base URL: %w", err)
	}
	resourceURL.Path = path.Join(resourceURL.Path, apiPath)

	// doRequest now handles error deserialization, so no need for a local validation struct.
	resp, err := c.doRequest(ctx, http.MethodPut, resourceURL.String(), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// Post performs a POST request to create a content item.
func (c *ContentClient) Post(ctx context.Context, apiPath string, queryParams map[string]string, payload interface{}) error {
	resourceURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("internal error: could not parse base URL: %w", err)
	}
	resourceURL.Path = path.Join(resourceURL.Path, apiPath)

	// Add query parameters
	if len(queryParams) > 0 {
		q := resourceURL.Query()
		for key, value := range queryParams {
			q.Add(key, value)
		}
		resourceURL.RawQuery = q.Encode()
	}

	// doRequest now handles error deserialization, so no need for nil here.
	resp, err := c.doRequest(ctx, http.MethodPost, resourceURL.String(), payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
