// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

type Station struct {
	bdplib.Station
	MaxTimestamp int64
	MinTimestamp int64
}

func readStations(ctx context.Context, db *sqlx.DB, origin, stationType string) ([]Station, error) {
	// without station mapping
	// query := `
	// 	SELECT code, name, geo, min_timestamp, max_timestamp,
	// 		(SELECT data FROM a22.a22_station_detail WHERE a22_station_detail.code = t.code) AS metadata
	// 	FROM a22.a22_station t
	// 	WHERE min_timestamp IS NOT NULL
	// 	AND max_timestamp IS NOT NULL`

	// Mapped station old to new
	query := `
		SELECT
			n.code,
			n.name,
			n.geo,
			LEAST(
				n.min_timestamp,
				o.min_timestamp
			) AS min_timestamp,
			GREATEST(
				n.max_timestamp,
				o.max_timestamp
			) AS max_timestamp,
			d.data as metadata
		FROM a22.a22_station o
		left outer JOIN a22.a22_station_mapping m ON m.old = o.code
		JOIN a22.a22_station n ON case when m.new is not null then m.new = n.code else n.code = o.code end
		left outer join a22.a22_station_detail d on d.code = n.code
		where o.code not in (select "new" from a22.a22_station_mapping asm )
		and not (n.min_timestamp is null and o.min_timestamp is null);`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []Station

	for rows.Next() {
		var code, name, geo, metadataStr string
		var min_timestamp, max_timestamp sql.NullInt32
		if err := rows.Scan(&code, &name, &geo, &min_timestamp, &max_timestamp, &metadataStr); err != nil {
			return nil, err
		}

		coords := strings.Split(geo, ",")
		if len(coords) != 2 {
			continue
		}
		var lat, lng float64
		fmt.Sscanf(coords[0], "%f", &lat)
		fmt.Sscanf(coords[1], "%f", &lng)

		meta := make(map[string]interface{})
		meta["a22_metadata"] = metadataStr

		station := Station{
			Station: bdplib.Station{
				Id:          code,
				Name:        name,
				Latitude:    lat,
				Longitude:   lng,
				Origin:      origin,
				StationType: stationType,
				MetaData:    meta,
			},
			MinTimestamp: int64(min_timestamp.Int32) * 1000,
			MaxTimestamp: int64(max_timestamp.Int32) * 1000,
		}

		stations = append(stations, station)
	}

	sensorUtils.AddSensorTypeMetadata(stations)
	return stations, nil
}

type ninjaResponse struct {
	Mvalidtime string `json:"mvalidtime"`
	Tname      string `json:"tname"`
	Scode      string `json:"scode"`
}

type measurementMap struct {
	first           time.Time
	Last            time.Time
	LastByDataTypes map[string]time.Time
}

func (m measurementMap) shouldElaborate(dataType string, ts time.Time) bool {
	last, ok := m.LastByDataTypes[dataType]
	if !ok || last.Before(ts) {
		return true
	}
	return false
}

// startFrom returns the earliest (minimum) "last" timestamp among all
// data‐types if all are present, or 0 if any are missing.
func (m measurementMap) startFrom(s Station) time.Time {
	// Check presence of every required data‐type
	for _, dt := range allDataTypes {
		// neet to exclude camera specific data types from normal stations, othwrwise we will have these stations starting from
		// min timestamp every time
		isCamera := IsCamera(s)
		if !isCamera && (dt == DataTypeEuroPct ||
			dt == DataTypeNationalityCount) {
			continue
		}
		if _, ok := m.LastByDataTypes[dt]; !ok {
			return time.Time{}
		}
	}

	return m.first
}

func getMeasurementsByStation(ctx context.Context, oauth *OAuthProvider) (map[string]*measurementMap, error) {
	token, err := oauth.GetToken()
	if err != nil {
		return nil, err
	}

	odhts.C.AuthToken = token

	req := odhts.DefaultRequest()
	req.StationTypes = append(req.StationTypes, sensorStationType)
	req.Repr = odhts.FlatNode
	req.DataTypes = dataTypesFilter
	req.Where = "sorigin.eq.A22,mperiod.eq.600"
	req.Select = "mvalidtime,tname,scode"
	req.Limit = 10000

	res := odhts.Response[[]ninjaResponse]{}
	if err := odhts.Latest(req, &res); err != nil {
		return nil, err
	}

	const layout = "2006-01-02 15:04:05.000-0700"

	measurements := make(map[string]*measurementMap)

	for _, r := range res.Data {
		t, err := time.Parse(layout, r.Mvalidtime)
		if err != nil {
			logger.Get(ctx).Error("invalid ninja measurement timestamp format", "measurement", r)
			continue // Skip invalid timestamp
		}

		meas, exists := measurements[r.Scode]
		if !exists {
			meas = &measurementMap{
				first:           t,
				Last:            t,
				LastByDataTypes: make(map[string]time.Time),
			}
			measurements[r.Scode] = meas
		} else {
			if t.Before(meas.first) {
				meas.first = t
			}
			if t.After(meas.Last) {
				meas.Last = t
			}
		}

		// Update latest time per data type
		if dt, ok := meas.LastByDataTypes[r.Tname]; !ok || t.After(dt) {
			meas.LastByDataTypes[r.Tname] = t
		}
	}

	return measurements, nil
}
