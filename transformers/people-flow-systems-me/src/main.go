// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env tr.Env

type SensorPayload struct {
	Type string
	Data struct {
		Name      string
		Direction string
		Timestamp MaybeTimezoneTime
	}
}

// Payload is a string containing a JSON
func (sp *SensorPayload) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	type Alias SensorPayload
	var alias Alias
	if err := json.Unmarshal([]byte(raw), &alias); err != nil {
		return err
	}

	*sp = SensorPayload(alias)
	return nil
}

type MaybeTimezoneTime struct {
	time.Time
}

// Depending on sensor, Timestamp does not have a timezone, so we have to handle both cases
func (ct *MaybeTimezoneTime) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)

	// Try parsing with timezone first (RFC3339 variants)
	if t, err := time.Parse(time.RFC3339, str); err == nil {
		ct.Time = t
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		ct.Time = t
		return nil
	}

	// We have no timezone, so assume we're in Italy
	romeLocation, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		return err
	}

	for _, format := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(format, str, romeLocation); err == nil {
			ct.Time = t
			return nil
		}
	}

	return fmt.Errorf("unable to parse timestamp: %s", str)
}

type RawType struct {
	Topic   string
	MsgId   int
	Payload SensorPayload
}

const STATIONTYPE = "PeopleCounter"
const PERIOD = 600

var datatype = bdplib.CreateDataType("countPeople", "", "Number of people passing by", "sum")

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	b.SyncDataTypes([]bdplib.DataType{datatype})
	ms.FailOnError(context.Background(), b.SyncStations(STATIONTYPE, []bdplib.Station{}, true, false), "could not sync stations")

	listener := tr.NewTr[RawType](context.Background(), env)
	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawType]) error {
		recs := b.CreateDataMap()
		recs.AddRecord("stationcode", datatype.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), -999, PERIOD))
		err := b.PushData(STATIONTYPE, recs)
		if err != nil {
			return err
		}
		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}
