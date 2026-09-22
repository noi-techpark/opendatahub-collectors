// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

// TestTransform_Station3 exercises a full crawl-run payload built from real
// FAMAS responses captured via bdp-commons/data-collectors/traffic-provBZ/
// famas-api-test (station metadata + 284 real DatiAggregatiSuPostazioni
// records for station "3", covering both real lane/direction combinations
// and the "other direction" rows the API also returns and that must be
// filtered out - see addTrafficMeasurements).
func TestTransform_Station3(t *testing.T) {
	loadSensorTypeMapping("../resources/sensor-type-mapping.csv")

	var in RawData
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in1.json"))

	timestamp, err := time.Parse("2006-01-02", "2026-09-22")
	require.Nil(t, err)
	raw := rdb.Raw[RawData]{Rawdata: in, Timestamp: timestamp}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, "testdata/out1.json"); err != nil {
		t.Logf("No snapshot found, generating testdata/out1.json: %v", err)
		require.Nil(t, testsuite.WriteOutput(req, "testdata/out1.json"))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

// TestTransform_LiveFullCrawl runs the transformer against a real, complete
// api-crawler output: all 8 FAMAS stations from a live run of
// traffic-famas-prov-bz.silky.yaml executed from inside the cluster (the FAMAS
// endpoint is IP-filtered), captured verbatim. This is a structural smoke
// test over the shape every real station in production currently has (lane
// counts, class codes, missing fields), not a hand-verified value check.
func TestTransform_LiveFullCrawl(t *testing.T) {
	loadSensorTypeMapping("../resources/sensor-type-mapping.csv")

	var in RawData
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in2.json"))
	require.Len(t, in.Stations, 8)

	timestamp, err := time.Parse("2006-01-02", "2026-09-22")
	require.Nil(t, err)
	raw := rdb.Raw[RawData]{Rawdata: in, Timestamp: timestamp}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, "testdata/out2.json"); err != nil {
		t.Logf("No snapshot found, generating testdata/out2.json: %v", err)
		require.Nil(t, testsuite.WriteOutput(req, "testdata/out2.json"))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}
