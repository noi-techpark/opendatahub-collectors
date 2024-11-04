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
	"time"
)

func getAllLocations(rabbit RabbitC, provider string) error {
	slog.Debug("Pulling all locations")
	url := cfg.PULL_LOCATIONS_ENDPOINT
	for url != "" {
		// Our mongodb cannot handle huge files, hence we push piecewise
		locations, next, err := getPage(url, cfg.PULL_TOKEN)
		if err != nil {
			slog.Error("error getting locations")
			return err
		}

		err = rabbit.Publish(mqMsg{
			Provider:  provider,
			Timestamp: time.Now(),
			Rawdata:   locations,
		}, cfg.RABBITMQ_EXCHANGE)
		if err != nil {
			slog.Error("error getting locations")
			return err
		}
		url = next
	}

	return nil
}

func getPage(url string, token string) ([]map[string]any, string, error) {
	req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
	if err != nil {
		slog.Error("failed creating http request", "url", url, "err", err)
		return nil, "", err
	}

	req.Header = http.Header{
		"Authorization": {fmt.Sprintf("Token %s", token)},
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("error during http request:", "err", err)
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("http request returned non-OK status", "statusCode", resp.StatusCode)
		return nil, "", fmt.Errorf("http request returned non-OK status")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("error reading response body:", "err", err)
		return nil, "", err
	}

	oResp := OCPIResp[[]map[string]any]{}

	if err := json.Unmarshal(body, &oResp); err != nil {
		slog.Error("error unmarshalling get reponse body", "err", err)
		return nil, "", err
	}

	if oResp.StatusCode != 1000 {
		slog.Error("ocpi status code not OK", "statusCode", oResp.StatusCode, "msg", oResp.StatusMessage)
		return nil, "", err
	}

	// but wait! there is more!
	// As per spec, if the nextpage header is returned, there are more pages at that URL
	nextpage := resp.Header.Get("Link")

	return oResp.Data, nextpage, nil
}
