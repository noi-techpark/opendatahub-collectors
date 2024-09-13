// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type payload struct {
	MsgId   int
	Topic   string
	Payload strMqttPayload
}

// the Payload JSON is a string that we have to first unmarshal
type strMqttPayload mqttPayload

type mqttPayload []struct {
	DateTimeAcquisition time.Time
	ControlUnitId       string
	Resval              []struct {
	}
}

func unmarshalRaw(s string) (payload, error) {
	var p payload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return p, fmt.Errorf("error unmarshalling payload json: %w", err)
	}

	return p, nil
}
