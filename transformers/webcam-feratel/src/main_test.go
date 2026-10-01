// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"

	contentmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-feratel/content-model"
)

func Test_Transform_Snapshot(t *testing.T) {
	fixedNow := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	defer func() { timeNow = time.Now }()

	for _, suffix := range []string{"", "_full"} {
		t.Run("Snapshot"+suffix, func(t *testing.T) {
			mock := clibmock.NewContentMock()
			contentClient = mock
			webcamCache = clib.NewCache[contentmodel.WebcamInfo]()

			// Read wrapped string from in.json or in_full.json
			inFile := "../testdata/in" + suffix + ".json"
			b, err := os.ReadFile(inFile)
			if err != nil {
				t.Fatalf("failed to read %s: %v", inFile, err)
			}

			var rawXMLString string
			err = json.Unmarshal(b, &rawXMLString)
			if err != nil {
				t.Fatalf("failed to unmarshal %s: %v", inFile, err)
			}

			r := &rdb.Raw[string]{
				Rawdata:   rawXMLString,
				Timestamp: fixedNow,
			}

			err = Transform(context.TODO(), r)
			if err != nil {
				t.Fatalf("Transform failed: %v", err)
			}

			calls := mock.Calls()

			var expected clibmock.MockCalls
			outFile := "../testdata/out" + suffix + ".json"
			err = testsuite.LoadOutput(&expected, outFile)
			if err != nil {
				t.Logf("No snapshot found, generating %s", outFile)
				err = testsuite.WriteOutput(calls, outFile)
				if err != nil {
					t.Fatalf("failed to write snapshot: %v", err)
				}
				t.Log("Snapshot generated. Re-run the test to validate.")
				return
			}

			clibmock.CompareMockCalls(t, expected, calls)
		})
	}
}

// Test_Transform_MultiLanguage checks that each language takes title, contact info and keywords
// from its own feed, and falls back to the base language when a translation is missing.
func Test_Transform_MultiLanguage(t *testing.T) {
	fixedNow := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	defer func() { timeNow = time.Now }()

	mock := clibmock.NewContentMock()
	contentClient = mock
	webcamCache = clib.NewCache[contentmodel.WebcamInfo]()

	b, err := os.ReadFile("../testdata/raw.xml")
	if err != nil {
		t.Fatalf("failed to read raw.xml: %v", err)
	}
	de := string(b)
	it := strings.NewReplacer(
		`l="El Forn"`, `l="Il Forno"`,
		`<country lkz="AND" ioc="AND">Andorra</country>`, `<country lkz="AND" ioc="AND">Andorra IT</country>`,
		`<keywords>Canillo (Grandvalira),Canillo,Andorra,`, `<keywords>Canillo (Grandvalira),Canillo,Andorra IT,`,
		`?lg=de&amp;cam=15065`, `?lg=it&amp;cam=15065`,
	).Replace(de)
	// the english feed does not contain the cam, so en must fall back to the german values
	en := strings.Replace(de, `panid="15065"`, `panid="99999"`, 1)

	rawdata, _ := json.Marshal(map[string]string{"de": de, "it": it, "en": en})
	err = Transform(context.TODO(), &rdb.Raw[string]{Rawdata: string(rawdata), Timestamp: fixedNow})
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()
	if len(calls.Posts) != 1 {
		t.Fatalf("expected 1 webcam to be created from the base feed, got %d", len(calls.Posts))
	}
	var webcam contentmodel.WebcamInfo
	if err := json.Unmarshal(calls.Posts[0].Payload, &webcam); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	checks := []struct {
		name, got, want string
	}{
		{"Shortname", webcam.Shortname, "El Forn"},
		{"de title", webcam.Detail["de"].Title, "El Forn"},
		{"it title", webcam.Detail["it"].Title, "Il Forno"},
		{"en title (fallback)", webcam.Detail["en"].Title, "El Forn"},
		{"de country", webcam.ContactInfos["de"].CountryName, "Andorra"},
		{"it country", webcam.ContactInfos["it"].CountryName, "Andorra IT"},
		{"en country (fallback)", webcam.ContactInfos["en"].CountryName, "Andorra"},
		{"it url", webcam.ContactInfos["it"].Url, "https://www.feratel.com/webcams/andorra/canillo/canillo-el-forn?lg=it&cam=15065"},
		{"it keyword", webcam.Detail["it"].Keywords[2], "Andorra IT"},
		{"de keyword", webcam.Detail["de"].Keywords[2], "Andorra"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: expected %q, got %q", c.name, c.want, c.got)
		}
	}
}

func Test_Transform_EmptyFeed_NoDeactivation(t *testing.T) {
	mock := clibmock.NewContentMock()
	contentClient = mock

	r := &rdb.Raw[string]{
		Rawdata:   `<?xml version="1.0" encoding="utf-8"?><feratel><content><portal></portal></content></feratel>`,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	if err := Transform(context.TODO(), r); err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()
	if len(calls.Gets)+len(calls.Puts)+len(calls.Posts)+len(calls.PutMultiples) != 0 {
		t.Fatalf("expected no API calls for a feed without webcams, got %+v", calls)
	}
}
