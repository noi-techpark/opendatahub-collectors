// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"time"
)

type OCPIResp[T any] struct {
	Data          T            `json:"data,omitempty"`
	StatusCode    int          `json:"status_code"`
	Timestamp     OCPIDateTime `json:"timestamp"`
	StatusMessage *string      `json:"status_message,omitempty"`
}

type OCPIDateTime struct {
	time.Time
}

func (t OCPIDateTime) MarshalJSON() ([]byte, error) {
	// OCPI is particular about date formats, only a subset of RFC 3339 is supported, and must be in UTC
	f := fmt.Sprintf("\"%s\"", t.Time.UTC().Format("2006-01-02T15:04:05Z"))
	return []byte(f), nil
}
