// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"opendatahub.com/rest-push-skidata/skidata"
)

// skidataStub serves the counting categories endpoint the collector calls.
func skidataStub(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "countingcategories/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestPublishesCategoriesKeyedByFacility(t *testing.T) {
	const cats = `[
	  {"carparkId":0,"countingCategoryId":1,"name":"SostaBreve","capacity":160,"occupancyLimit":160,"freeLimit":159},
	  {"carparkId":0,"countingCategoryId":2,"name":"Abbonati","capacity":85,"occupancyLimit":85,"freeLimit":84},
	  {"carparkId":0,"countingCategoryId":3,"name":"Totale","capacity":245,"occupancyLimit":245,"freeLimit":244},
	  {"carparkId":1,"countingCategoryId":3,"name":"Totale","capacity":50,"occupancyLimit":50,"freeLimit":49}
	]`
	api := skidataStub(t, cats, http.StatusOK)
	defer api.Close()

	var gotHeader http.Header
	var gotBody []byte
	var gotPath string
	writer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer writer.Close()

	env.SKIDATA_BASE_URL = api.URL
	env.RAW_WRITER_URL = writer.URL
	env.SKIDATA_CATEGORIES_PROVIDER = "skidata/counting-categories"

	err := fetchAndPublishCategories(context.Background(),
		FacilityCredential{Username: "u", Password: "p", Facility: "0607242"})
	if err != nil {
		t.Fatalf("fetchAndPublishCategories: %v", err)
	}

	// The facility must arrive as a meta header — it is the only field the
	// bridge can later group on.
	if v := gotHeader.Get("X-OpenDataHub-facility"); v != "0607242" {
		t.Errorf("facility meta header = %q, want 0607242", v)
	}
	if !strings.HasPrefix(gotPath, "/skidata/counting-categories/") {
		t.Errorf("path = %q, want the counting-categories provider", gotPath)
	}
	if ct := gotHeader.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}

	// The whole facility goes in one document: a partial one is
	// indistinguishable from a facility that lost a carpark.
	var got []skidata.CountingCategory
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("published body is not a category list: %v (%s)", err, gotBody)
	}
	if len(got) != 4 {
		t.Fatalf("published %d categories, want 4", len(got))
	}
	if countCarparks(got) != 2 {
		t.Errorf("published %d carparks, want 2", countCarparks(got))
	}
	if got[2].CountingCategoryId != 3 || got[2].Capacity != 245 {
		t.Errorf("total category not carried through: %+v", got[2])
	}
}

// An empty list means the fetch described nothing, not that the facility has no
// carparks. Publishing it would overwrite a real topology with an empty one.
func TestRefusesToPublishAnEmptyTopology(t *testing.T) {
	api := skidataStub(t, `[]`, http.StatusOK)
	defer api.Close()

	published := false
	writer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		published = true
		w.WriteHeader(http.StatusOK)
	}))
	defer writer.Close()

	env.SKIDATA_BASE_URL = api.URL
	env.RAW_WRITER_URL = writer.URL
	env.SKIDATA_CATEGORIES_PROVIDER = "skidata/counting-categories"

	err := fetchAndPublishCategories(context.Background(),
		FacilityCredential{Facility: "0607242"})
	if err == nil {
		t.Fatal("an empty category list was accepted")
	}
	if published {
		t.Error("an empty topology was written to the lake")
	}
}

// A 200 carrying something that is not a category list is a failed fetch, and
// must not reach the lake. (Deliberately not a 5xx: the retrying client would
// spend fifteen seconds proving the same point.)
func TestFetchFailureIsNotPublished(t *testing.T) {
	api := skidataStub(t, `{"error":"not a list"}`, http.StatusOK)
	defer api.Close()

	published := false
	writer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		published = true
		w.WriteHeader(http.StatusOK)
	}))
	defer writer.Close()

	env.SKIDATA_BASE_URL = api.URL
	env.RAW_WRITER_URL = writer.URL
	env.SKIDATA_CATEGORIES_PROVIDER = "skidata/counting-categories"

	if err := fetchAndPublishCategories(context.Background(),
		FacilityCredential{Facility: "0607242"}); err == nil {
		t.Fatal("a failed fetch was reported as success")
	}
	if published {
		t.Error("a failed fetch still wrote to the lake")
	}
}
