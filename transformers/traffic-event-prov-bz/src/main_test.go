// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

// setup wires the package globals the way main() does, for tests.
func setup(t *testing.T) {
	t.Helper()
	var err error
	location, err = time.LoadLocation(PROVIDER_TIMEZONE)
	require.NoError(t, err)
	tags, err = clib.ReadTagDefs("../resources/tags.json")
	require.NoError(t, err)
}

// Test_Transform_Snapshot runs the full Transform against a mocked Content
// API over the new-format feed sample and snapshots the resulting calls.
func Test_Transform_Snapshot(t *testing.T) {
	setup(t)

	mock := clibmock.NewContentMock()
	contentClient = mock
	annCache = clib.NewCache[odhContentModel.Announcement]()

	var in []dto.TrafficEvent
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in.json"))

	r := &rdb.Raw[[]dto.TrafficEvent]{
		Rawdata:   in,
		Timestamp: time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC),
	}

	require.NoError(t, Transform(context.TODO(), r))

	calls := mock.Calls()

	var expected clibmock.MockCalls
	err := testsuite.LoadOutput(&expected, "testdata/out.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out.json")
		require.NoError(t, testsuite.WriteOutput(calls, "testdata/out.json"))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	clibmock.CompareMockCalls(t, expected, calls)
}

// Test_HandlesBothFeedFormats is the core "handle both" guarantee: the feed
// historically typed numeric fields (messageId, messageTypeId, ...) as
// integers and now types them as strings. The same logical event in either
// representation must map to a byte-identical Announcement.
func Test_HandlesBothFeedFormats(t *testing.T) {
	setup(t)

	var newFmt, oldFmt []dto.TrafficEvent
	require.NoError(t, testsuite.LoadInputData(&newFmt, "testdata/in.json"))
	require.NoError(t, testsuite.LoadInputData(&oldFmt, "testdata/in_legacy.json"))
	require.Equal(t, len(newFmt), len(oldFmt))
	require.NotEmpty(t, newFmt)

	for i := range newFmt {
		// Both representations must yield the same id (and thus the same
		// deterministic URN) and the same mapped announcement.
		require.Equal(t, newFmt[i].MessageID.String(), oldFmt[i].MessageID.String(),
			"messageId text differs between formats at index %d", i)

		idNew := generateID(newFmt[i])
		idOld := generateID(oldFmt[i])
		require.Equal(t, idNew, idOld, "generated id differs between formats at index %d", i)

		annNew, err := MapTrafficEventToAnnouncement(tags, newFmt[i], idNew)
		require.NoError(t, err)
		annOld, err := MapTrafficEventToAnnouncement(tags, oldFmt[i], idOld)
		require.NoError(t, err)

		require.Equal(t, annNew, annOld,
			"mapped announcement differs between string-typed and int-typed feed at index %d", i)
	}
}

// Test_CloseDetection_PreservesProviderEndTime pins the ended-detection
// contract. An announcement that drops out of the feed is closed at the batch
// timestamp only when the provider never gave it an end date; a planned end
// date from the feed must survive, otherwise every restart rewrites future end
// dates to "now".
func Test_CloseDetection_PreservesProviderEndTime(t *testing.T) {
	setup(t)

	mock := clibmock.NewContentMock()
	contentClient = mock
	annCache = clib.NewCache[odhContentModel.Announcement]()

	sourceTime := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	plannedEnd := time.Date(2026, 12, 24, 0, 0, 0, 0, time.UTC)

	planned := odhContentModel.Announcement{EndTime: &plannedEnd}
	planned.ID = clib.StringPtr("urn:announcements:provincebz:planned")
	planned.Mapping.ProviderProvinceBz.Id = "planned"

	openEnded := odhContentModel.Announcement{}
	openEnded.ID = clib.StringPtr("urn:announcements:provincebz:open")
	openEnded.Mapping.ProviderProvinceBz.Id = "open"

	annCache.Set(*planned.ID, planned, 1)
	annCache.Set(*openEnded.ID, openEnded, 2)

	// Empty batch: neither announcement is in the feed any more.
	r := &rdb.Raw[[]dto.TrafficEvent]{Rawdata: []dto.TrafficEvent{}, Timestamp: sourceTime}
	require.NoError(t, Transform(context.TODO(), r))

	calls := mock.Calls()
	require.Len(t, calls.PutMultiples, 1)

	var written []odhContentModel.Announcement
	require.NoError(t, json.Unmarshal(calls.PutMultiples[0].Payload, &written))
	require.Len(t, written, 2)

	byID := map[string]odhContentModel.Announcement{}
	for _, a := range written {
		byID[*a.ID] = a
	}

	gotPlanned := byID[*planned.ID].EndTime
	require.NotNil(t, gotPlanned)
	require.True(t, plannedEnd.Equal(*gotPlanned),
		"provider end date must survive close-detection, got %s want %s", gotPlanned, plannedEnd)

	gotOpen := byID[*openEnded.ID].EndTime
	require.NotNil(t, gotOpen)
	require.True(t, sourceTime.Equal(*gotOpen),
		"open-ended announcement must close at the batch timestamp, got %s want %s", gotOpen, sourceTime)
}

// Test_CloseDetection_ClosesOnlyWhatLeftTheFeed makes sure an announcement
// still present in the batch is never closed.
func Test_CloseDetection_ClosesOnlyWhatLeftTheFeed(t *testing.T) {
	setup(t)

	mock := clibmock.NewContentMock()
	contentClient = mock
	annCache = clib.NewCache[odhContentModel.Announcement]()

	var in []dto.TrafficEvent
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in.json"))
	require.NotEmpty(t, in)

	// Prime the cache with exactly what the batch contains, plus one stale
	// entry that is not in the feed.
	for _, e := range in {
		id := generateID(e)
		ann, err := MapTrafficEventToAnnouncement(tags, e, id)
		require.NoError(t, err)
		annCache.Set(id, ann, 0)
	}

	stale := odhContentModel.Announcement{}
	stale.ID = clib.StringPtr("urn:announcements:provincebz:stale")
	stale.Mapping.ProviderProvinceBz.Id = "stale"
	annCache.Set(*stale.ID, stale, 0)

	sourceTime := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	r := &rdb.Raw[[]dto.TrafficEvent]{Rawdata: in, Timestamp: sourceTime}
	require.NoError(t, Transform(context.TODO(), r))

	var written []odhContentModel.Announcement
	require.NoError(t, json.Unmarshal(mock.Calls().PutMultiples[0].Payload, &written))

	closed := map[string]bool{}
	for _, a := range written {
		if a.EndTime != nil && sourceTime.Equal(*a.EndTime) {
			closed[*a.ID] = true
		}
	}

	require.True(t, closed[*stale.ID], "announcement absent from the batch must be closed")
	for _, e := range in {
		require.False(t, closed[generateID(e)],
			"announcement still in the batch must not be closed (messageId %s)", e.MessageID.String())
	}
}

func Test_FlexString_UnmarshalsStringNumberAndNull(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{`"578910"`, "578910"},
		{`578910`, "578910"},
		{`""`, ""},
		{`null`, ""},
		{`"SS12"`, "SS12"},
		{`300012`, "300012"},
	}
	for _, c := range cases {
		var fs dto.FlexString
		require.NoError(t, json.Unmarshal([]byte(c.raw), &fs), "raw=%s", c.raw)
		require.Equal(t, c.want, fs.String(), "raw=%s", c.raw)
	}
}

func Test_FlexFloat_UnmarshalsNumberStringAndNull(t *testing.T) {
	cases := []struct {
		raw       string
		wantValid bool
		wantValue float64
	}{
		{`11.638`, true, 11.638},
		{`"11.638"`, true, 11.638},
		{`null`, false, 0},
		{`""`, false, 0},
		{`"not-a-number"`, false, 0},
	}
	for _, c := range cases {
		var ff dto.FlexFloat
		require.NoError(t, json.Unmarshal([]byte(c.raw), &ff), "raw=%s", c.raw)
		require.Equal(t, c.wantValid, ff.Valid, "raw=%s", c.raw)
		if c.wantValid {
			require.InDelta(t, c.wantValue, ff.Value, 1e-9, "raw=%s", c.raw)
		}
	}
}
