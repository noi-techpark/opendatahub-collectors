// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/oliveagle/jsonpath"
	"gopkg.in/yaml.v3"
	"opendatahub.com/multi-rest-poller/oauth"
)

type RootConfig struct {
	Call CallConfig `yaml:"http_call"`
}

type CallConfig struct {
	URL               string            `yaml:"url"`
	Method            string            `yaml:"method"`
	Headers           map[string]string `yaml:"headers"`
	DataSelector      string            `yaml:"data_selector"`
	DataSelectorType  string            `yaml:"data_selector_type"`
	NestedCalls       []CallConfig      `yaml:"nested_calls"`
	ParamSelectorType string            `yaml:"param_selector_type,omitempty"`
	ParamSelectors    []string          `yaml:"param_selectors,omitempty"`
	DataDestination   string            `yaml:"data_destination_field,omitempty"`
}

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
	return &config, nil
}

// Poll is the entry point that starts the recursive processing and returns the final result as a string.
func Poll(config *RootConfig) (string, error) {
	if env.AUTH_STRATEGY == "oauth2" {
		oauthProvider = oauth.NewOAuthProvider()
	}

	result, err := processCall(config.Call)
	if err != nil {
		return "", err
	}

	// calls went good but selector returned nil
	if result == nil {
		return "", nil
	}

	// Based on the configured DataSelectorType, convert the result to a string.
	if config.Call.DataSelectorType == "json" {
		// For JSON responses, marshal the result.
		finalBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("error marshalling final result: %s", err.Error())
		}
		return string(finalBytes), nil
	}

	// For non-JSON types assume the result is a string or can be converted.
	switch res := result.(type) {
	case string:
		return res, nil
	case []byte:
		return string(res), nil
	default:
		return fmt.Sprintf("%v", res), nil
	}
}

// extractData attempts to extract a value using a JSONPath selector.
// If an "index out of range" error occurs, it returns nil
func extractData(item interface{}, selector string) (interface{}, error) {
	val, err := jsonpath.JsonPathLookup(item, selector)
	if err != nil {
		if strings.Contains(err.Error(), "index") {
			// Handle gracefully: log or assign a default value.
			// For this example, we default to an empty string.
			return nil, nil
		}
		return nil, fmt.Errorf("error in data selector %s: %s", selector, err.Error())
	}
	return val, nil
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
		// Format the nested URL using the extracted parameters.
		nestedCall.URL = fmt.Sprintf(nestedCall.URL, params...)
		// Recursively process the nested call.
		nestedResult, err := processCall(nestedCall)
		if err != nil {
			return fmt.Errorf("error processing nested call for url %s: %s", nestedCall.URL, err.Error())
		}
		// Insert the nested result into the current entity.
		(*data)[nestedCall.DataDestination] = nestedResult
	}

	return nil
}

// processCall sends the HTTP request for the given config, optionally extracts data using a JSONPath selector,
// and then processes any nested calls recursively.
func processCall(config CallConfig) (interface{}, error) {
	slog.Info("pulling endpoint", "endpoint", config.URL)
	/// -------------------- CALL
	req, err := http.NewRequest(config.Method, config.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request for url %s: %s", config.URL, err.Error())
	}

	// set headers
	for key, value := range config.Headers {
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
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error during http request for %s: %s", config.URL, err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request returned non-OK status %d for url %s", resp.StatusCode, config.URL)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body for %s: %s", config.URL, err.Error())
	}

	/// -------------------- RESULT MANIPULATION

	var result interface{}
	// If DataSelectorType is "json", unmarshal the response.
	if config.DataSelectorType == "json" {
		slog.Info("extracting with json selector", "selector", config.DataSelector)

		var jsonData interface{}
		if err := json.Unmarshal(body, &jsonData); err != nil {
			return nil, fmt.Errorf("error unmarshalling json for %s: %s", config.URL, err.Error())
		}
		// If a DataSelector is provided, extract the specified portion.
		if config.DataSelector != "" {
			extracted, err := extractData(jsonData, config.DataSelector)
			if err != nil {
				return nil, fmt.Errorf("error applying data selector %s on url %s: %s", config.DataSelector, config.URL, err.Error())
			}
			result = extracted
		} else {
			// Otherwise, use the full JSON.
			result = jsonData
		}
	} else {
		// For other types (e.g. binary), just return the raw response as a string.
		result = string(body)
	}
	// TODO do xml extractor using https://github.com/antchfx/xmlquery

	// Process nested calls if defined.
	if len(config.NestedCalls) == 0 {
		return result, nil
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
				return nil, err
			}
			data[i] = itemMap
		}
		result = data
	case map[string]interface{}:
		// Process nested calls for a single object.
		err := handleNestedCalls(config, &data)
		if err != nil {
			return nil, err
		}
		result = data
	}

	return result, nil
}
