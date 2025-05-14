// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"github.com/oliveagle/jsonpath"
	"gopkg.in/yaml.v3"
	"opendatahub.com/multi-rest-poller/oauth"
)

type RootConfig struct {
	Call              *CallConfig              `yaml:"http_call"`
	MultipleRootCalls *RootMultipleCallsConfig `yaml:"http_calls"`
}

func (r RootConfig) SelectorType() string {
	if r.Call != nil {
		return r.Call.DataSelectorType
	}
	return r.MultipleRootCalls.DataSelectorType
}

type RootMultipleCallsConfig struct {
	NestedCalls      []CallConfig `yaml:"nested_calls"`
	DataSelectorType string       `yaml:"data_selector_type"`
}

type CallConfig struct {
	URL               string            `yaml:"url"`
	Method            string            `yaml:"method"`
	Headers           map[string]string `yaml:"headers"`
	Stream            bool              `yaml:"stream"`
	DataSelector      string            `yaml:"data_selector"`
	DataSelectorType  string            `yaml:"data_selector_type"`
	NestedCalls       []CallConfig      `yaml:"nested_calls"`
	ParamSelectorType string            `yaml:"param_selector_type,omitempty"`
	ParamSelectors    []string          `yaml:"param_selectors,omitempty"`
	DataDestination   string            `yaml:"data_destination_field,omitempty"`
	Pagination        *Pagination       `yaml:"pagination,omitempty"`
}

type Pagination struct {
	RequestStrategy string        `yaml:"request_strategy"` // header | query | body
	LookupStrategy  string        `yaml:"lookup_strategy"`  // header | body | increment
	OffsetBuilder   OffsetBuilder `yaml:"offset_builder"`   //
	RequestKey      string        `yaml:"request_key"`      // where to put the offset for next requests
}

type OffsetBuilder struct {
	CurrentStart     int    `yaml:"current_start"`
	Next             string `yaml:"next_field"`
	Increment        int    `yaml:"increment"`
	NextType         string `yaml:"next_type"`
	BreakOnNextEmpty bool   `yaml:"break_on_next_empty"`
}

type encoder func(d any) (string, error)

var oauthProvider *oauth.OAuthProvider = nil

// LoadConfig reads the YAML configuration from the given file path,
// unmarshals it into a CallConfig instance, and returns a pointer to it.
func LoadConfig(filename string) (*RootConfig, error) {
	// Read the configuration file.
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	// Unmarshal YAML into the RootConfig struct.
	var config RootConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if nil == config.Call && nil == config.MultipleRootCalls {
		return nil, fmt.Errorf("either 'http_call' or 'http_calls' needs to be set")
	}

	return &config, nil
}

// Poll is the entry point that starts the recursive processing and returns the final result as a string.
func Poll(config *RootConfig, stream chan<- any) (any, error) {
	if env.AUTH_STRATEGY == "oauth2" {
		oauthProvider = oauth.NewOAuthProvider()
	}

	var result interface{} = nil
	var err error

	if config.Call != nil {
		result, err = processCall(*config.Call, stream)
		if err != nil {
			return nil, err
		}
	} else if config.MultipleRootCalls != nil {
		calls_result := map[string]interface{}{}
		wrapped_calls := CallConfig{
			NestedCalls: config.MultipleRootCalls.NestedCalls,
		}

		err := handleNestedCalls(wrapped_calls, &calls_result)
		result = calls_result
		if err != nil {
			return nil, err
		}
	}

	// calls went good but selector returned nil
	if result == nil {
		return nil, nil
	}

	// calls went good but no results
	if array_result, ok := result.([]interface{}); ok && len(array_result) == 0 {
		return nil, nil
	}

	return result, nil
}

func GetEncoder(c RootConfig) func(d any) (string, error) {
	return func(d any) (string, error) {
		// Based on the configured DataSelectorType, convert the result to a string.
		if c.SelectorType() == "json" {
			// For JSON responses, marshal the result.
			finalBytes, err := json.Marshal(d)
			if err != nil {
				return "", fmt.Errorf("error marshalling final result: %s", err.Error())
			}
			return string(finalBytes), nil
		}

		// For non-JSON types assume the result is a string or can be converted.
		switch res := d.(type) {
		case string:
			return res, nil
		case []byte:
			return string(res), nil
		default:
			return fmt.Sprintf("%v", res), nil
		}
	}
}

// extractData attempts to extract a value using a JSONPath selector.
// If an "index out of range" error occurs, it returns nil
func extractData(result []byte, selector_type, selector string) (interface{}, error) {
	if selector_type == "json" {
		slog.Debug("extracting with json selector", "selector", selector)

		var jsonData interface{}
		if err := json.Unmarshal(result, &jsonData); err != nil {
			return nil, fmt.Errorf("error unmarshalling json: %s", err.Error())
		}
		// If a DataSelector is provided, extract the specified portion.
		if selector != "" {
			val, err := jsonpath.JsonPathLookup(jsonData, selector)
			if err != nil {
				if strings.Contains(err.Error(), "index") {
					// Handle gracefully: log or assign a default value.
					// For this example, we default to an empty string.
					return nil, nil
				}
				return nil, fmt.Errorf("error in json data selector %s: %s", selector, err.Error())
			}
			return val, nil
		}
		return jsonData, nil
	} else if selector_type == "string" {
		return string(result), nil
	}
	// TODO do xml extractor using https://github.com/antchfx/xmlquery
	return result, nil
}

// existsData check if a JSONPath selector exists in data.
func existsData(result []byte, selector_type, selector string) bool {
	r, err := extractData(result, selector_type, selector)
	return err == nil && r != nil
}

func httpRequest(method, url string, headers map[string]string, body any) ([]byte, error) {
	// TODO BODY for POST
	var req_body io.Reader = nil

	req, err := retryablehttp.NewRequest(method, url, req_body)
	if err != nil {
		return nil, fmt.Errorf("could not create request for url %s: %s", url, err.Error())
	}

	// set headers
	for key, value := range headers {
		req.Header.Add(key, value)
	}

	// Inject authentication headers if needed.
	if oauthProvider != nil {
		token, err := oauthProvider.GetToken()
		if err != nil {
			return nil, fmt.Errorf("could not get oauth token: %s", err.Error())
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	} else if env.AUTH_STRATEGY == "basic" {
		req.SetBasicAuth(env.BASIC_AUTH_USERNAME, env.BASIC_AUTH_PASSWORD)
	} else if env.AUTH_STRATEGY == "bearer" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", env.AUTH_BEARER_TOKEN))
	}

	client := retryablehttp.NewClient()
	client.Logger = logger.Get(context.Background())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error during http request for %s: %s", url, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request returned non-OK status %d for url %s", resp.StatusCode, url)
	}
	defer resp.Body.Close()

	res_body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body for %s: %s", url, err.Error())
	}
	return res_body, nil
}

func handleNestedCalls(parent_call CallConfig, data *map[string]any) error {
	for _, nestedCall := range parent_call.NestedCalls {
		slog.Info("handling nested call", "template", nestedCall.URL)
		// Copy the nested call config and update the URL.
		// Extract parameters using the nested call's ParamSelectors.
		params := []interface{}{}
		for _, selector := range nestedCall.ParamSelectors {
			val, err := jsonpath.JsonPathLookup(*data, selector)
			if err != nil {
				return fmt.Errorf("error in param selector %s: %s", selector, err.Error())
			}
			params = append(params, fmt.Sprintf("%v", val))
		}
		if len(params) != 0 {
			// Format the nested URL using the extracted parameters.
			nestedCall.URL = fmt.Sprintf(nestedCall.URL, params...)
		}
		// Recursively process the nested call.
		nestedResult, err := processCall(nestedCall, nil)
		if err != nil {
			return fmt.Errorf("error processing nested call for url %s: %s", nestedCall.URL, err.Error())
		}
		// Insert the nested result into the current entity.
		(*data)[nestedCall.DataDestination] = nestedResult
	}

	return nil
}

func getTree(config CallConfig, method, url string, headers map[string]string, body any) (any, []byte, error) {
	/// -------------------- CALL
	// TODO BODY for POST
	body_res, err := httpRequest(method, url, headers, body)
	if err != nil {
		return nil, nil, err
	}

	/// -------------------- RESULT MANIPULATION
	result, err := extractData(body_res, config.DataSelectorType, config.DataSelector)
	if err != nil {
		return nil, nil, fmt.Errorf("error extracting data: %s", err.Error())
	}

	// Process nested calls if defined.
	if len(config.NestedCalls) == 0 {
		return result, body_res, nil
	}

	/// -------------------- NESTED CALLS
	switch data := result.(type) {
	case []interface{}:
		// Iterate over each entity in the slice.
		for i, item := range data {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue // Skip non-object items.
			}
			err := handleNestedCalls(config, &itemMap)
			if err != nil {
				return nil, nil, err
			}
			data[i] = itemMap
		}
		result = data
	case map[string]interface{}:
		// Process nested calls for a single object.
		err := handleNestedCalls(config, &data)
		if err != nil {
			return nil, nil, err
		}
		result = data
	}

	return result, body_res, nil
}

func handleStream(config CallConfig, data any, stream chan<- any) (any, bool) {
	if stream == nil || !config.Stream {
		return data, false
	}

	array_data, ok := data.([]interface{})
	// if not array, stream the whole data and return nil
	if !ok {
		stream <- data
		return nil, true
	}

	// if array stream element by element and return empty array
	for _, d := range array_data {
		stream <- d
	}

	return []any{}, true
}

// processCall sends the HTTP request for the given config, optionally extracts data using a JSONPath selector,
// and then processes any nested calls recursively.
func processCall(config CallConfig, stream chan<- any) (interface{}, error) {
	slog.Info("pulling endpoint", "endpoint", config.URL)

	if config.Pagination != nil && config.Pagination.LookupStrategy == "body" {
		if config.DataSelectorType == "" || config.DataSelector == "" {
			return nil, fmt.Errorf(
				"pagination with response_strategy == 'body' requires data_selector and data_selector_type to be set",
			)
		}
	}

	/// -------------------- CALL first time is the same for paginated and not paginated
	result, body_result, err := getTree(config, config.Method, config.URL, config.Headers, nil)
	if err != nil {
		return nil, fmt.Errorf("error getting data from url %s: %s", config.URL, err.Error())
	}

	// handle steram
	result, _ = handleStream(config, result, stream)

	/// -------------------- PAGINATION
	if config.Pagination != nil {
		array_result, ok := result.([]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot paginate if results are not arrays url %s", config.URL)
		}
		pagination_results, err := doPaginatedRequests(config, body_result, stream)
		if err != nil {
			return nil, fmt.Errorf("error performing pagination url %s: %s", config.URL, err.Error())
		}
		array_result = append(array_result, pagination_results...)
		result = array_result
	}

	return result, nil
}

// doPaginatedRequest loops requests and aggregates the data from each page.
func doPaginatedRequests(config CallConfig, first_call_body []byte, stream chan<- any) ([]interface{}, error) {
	p := config.Pagination
	offsetBuilder := p.OffsetBuilder
	prev_call_body := first_call_body

	// We'll accumulate all item-data pages into a single slice
	allItems := make([]interface{}, 0)

	// Current offset can start at offsetBuilder.CurrentStart
	var currentOffset interface{} = offsetBuilder.CurrentStart

	for {
		var newOffsetFound bool
		var err error

		switch p.LookupStrategy {
		case "body":
			currentOffset, newOffsetFound, err = computeNextOffsetBody(config, prev_call_body)
		case "increment":
			// Current offset must be numeric to add increment
			cur, err := toInt(currentOffset)
			if err != nil {
				// Can't increment a non-integer offset
				return nil, fmt.Errorf("cannot increment non-integer offset %v: %w", currentOffset, err)
			}
			currentOffset = cur + offsetBuilder.Increment
			newOffsetFound = true

		// TODO handle 'header' lookup strategy
		default:
			return nil, fmt.Errorf("unsupported pagination lookup strategy %q", p.LookupStrategy)
		}

		if err != nil {
			return nil, err
		}

		if !newOffsetFound {
			break
		}

		url := config.URL
		headers := cloneHeaders(config.Headers)

		p := config.Pagination
		var body []byte = nil

		slog.Info("pulling", "url", config.URL, "pagination", currentOffset)

		switch p.RequestStrategy {
		case "header":
			// Place offset in the headers as p.RequestKey
			headers[p.RequestKey] = fmt.Sprintf("%v", currentOffset)

		case "query":
			// Place offset as query param, e.g. ?page=OFFSET
			url, err = buildURLWithQueryParam(config.URL, p.RequestKey, fmt.Sprintf("%v", currentOffset))
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("unsupported pagination strategy %q", p.RequestStrategy)
		}

		result, body_Result, err := getTree(config, config.Method, url, headers, body)
		if err != nil {
			return nil, fmt.Errorf("error extracting data on url %s: %s", config.URL, err.Error())
		}

		// If no data, we stop
		if result == nil {
			slog.Debug("no data extracted; stopping pagination")
			break
		}

		result, streamed := handleStream(config, result, stream)

		if !streamed {
			// no need to check if the refecltion goes well since we already did it for the first call in "processCall"
			array_result := result.([]interface{})
			// If empty array data, we stop
			if len(array_result) == 0 {
				slog.Debug("no data extracted; stopping pagination")
				break
			}

			allItems = append(allItems, array_result...)
		}
		prev_call_body = body_Result
	}

	return allItems, nil
}

func computeNextOffsetBody(config CallConfig, response_body []byte) (interface{}, bool, error) {
	var nextOffset interface{} = nil
	newOffsetFound := true
	offsetBuilder := config.Pagination.OffsetBuilder
	var val interface{} = nil
	var err error = nil

	if existsData(response_body, config.DataSelectorType, offsetBuilder.Next) {
		val, err = extractData(response_body, config.DataSelectorType, offsetBuilder.Next)
		if err != nil {
			return nil, false, fmt.Errorf("error extracting next offset: %w", err)
		}
	}

	// If we found a next offset in the JSON
	if val != nil && !isEmptyValue(val) {
		// Convert it to int/string if needed
		switch offsetBuilder.NextType {
		case "int":
			intVal, err := toInt(val)
			if err != nil {
				return nil, false, fmt.Errorf("next offset is not convertible to int: %v", err)
			}
			nextOffset = intVal
		case "string":
			nextOffset = fmt.Sprintf("%v", val)
		default:
			// if no next_type is specified, fallback to raw
			nextOffset = val
		}
	}

	if offsetBuilder.BreakOnNextEmpty && isEmptyValue(nextOffset) {
		return nil, false, nil
	}

	return nextOffset, newOffsetFound, nil
}

// -------------------- Utility Helpers --------------------
func cloneHeaders(original map[string]string) map[string]string {
	clone := make(map[string]string, len(original))
	for k, v := range original {
		clone[k] = v
	}
	return clone
}

func buildURLWithQueryParam(baseURL, key, val string) (string, error) {
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", fmt.Errorf("could not parse url %s: %w", baseURL, err)
	}
	q := req.URL.Query()
	q.Set(key, val)
	req.URL.RawQuery = q.Encode()
	return req.URL.String(), nil
}

// isEmptyValue is a basic check to see if an interface is nil, empty string, or numeric zero.
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch vt := v.(type) {
	case string:
		return vt == ""
	case int:
		return vt == 0
	case float64:
		return vt == 0.0
	}
	return false
}

// toInt tries to convert an interface{} to int
func toInt(v interface{}) (int, error) {
	switch vt := v.(type) {
	case float64:
		return int(vt), nil
	case float32:
		return int(vt), nil
	case int:
		return vt, nil
	case int64:
		return int(vt), nil
	case string:
		// Attempt parse
		return strconv.Atoi(vt)
	default:
		return 0, fmt.Errorf("value %v is not numeric or string", v)
	}
}
