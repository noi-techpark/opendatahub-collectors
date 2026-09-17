// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func marshalToMap(t *testing.T, m mqMsg) map[string]any {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestFromRestMapsContentTypeAndQuery(t *testing.T) {
	ts := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	msg := fromRest(restMsg{
		ID:          "abc",
		Timestamp:   ts,
		Provider:    "enrichment",
		Dataset:     "parking",
		ContentType: "application/json",
		Payload:     []byte(`{"hello":"world"}`),
		Query:       map[string][]string{"key": {"skidata|0404467_0"}},
	})

	if msg.Provider != "enrichment/parking" {
		t.Errorf("Provider = %q", msg.Provider)
	}
	if msg.ContentType != "application/json" {
		t.Errorf("ContentType = %q, want it carried through from the request", msg.ContentType)
	}
	if got := msg.Query["key"]; got != "skidata|0404467_0" {
		t.Errorf("Query[key] = %v, want the flattened scalar", got)
	}
}

// Query parameters have to land at the document root as scalars, because the
// raw data bridge groups only on root-level fields.
func TestMarshalPutsQueryUnderMeta(t *testing.T) {
	ts := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	out := marshalToMap(t, mqMsg{
		Provider:    "enrichment/parking",
		Timestamp:   ts,
		Rawdata:     []byte(`{"a":1}`),
		ID:          "abc",
		ContentType: "application/json",
		Query:       map[string]any{"key": "A", "origin": "skidata"},
	})

	meta, ok := out["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want a sub-document — the bridge groups on meta.<field> "+
			"and cannot reach the document root", out["meta"])
	}
	if meta["key"] != "A" {
		t.Errorf("meta.key = %v, want the scalar A", meta["key"])
	}
	if meta["origin"] != "skidata" {
		t.Errorf("meta.origin = %v", meta["origin"])
	}
	// And nothing leaked to the root, where it would be unreachable.
	if _, leaked := out["key"]; leaked {
		t.Error("key was written to the document root, where no reference table can see it")
	}
	if out["content_type"] != "application/json" {
		t.Errorf("content_type = %v", out["content_type"])
	}
	if out["provider"] != "enrichment/parking" {
		t.Errorf("provider = %v", out["provider"])
	}
	// rawdata keeps its existing base64 representation — existing consumers
	// decode it that way and must not break.
	want := base64.StdEncoding.EncodeToString([]byte(`{"a":1}`))
	if out["rawdata"] != want {
		t.Errorf("rawdata = %v, want unchanged base64 %q", out["rawdata"], want)
	}
}

// Nesting removes the whole class of collision: a parameter named after a
// pipeline field lands at meta.provider and cannot reach the root, so there is
// nothing left to validate against and nothing to clobber.
func TestQueryCannotReachTheEnvelope(t *testing.T) {
	out := marshalToMap(t, mqMsg{
		Provider:  "enrichment/parking",
		Timestamp: time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC),
		Rawdata:   []byte("real"),
		ID:        "real-id",
		Query: map[string]any{
			"provider": "attacker/elsewhere",
			"rawdata":  "clobbered",
			"id":       "clobbered",
		},
	})

	if out["provider"] != "enrichment/parking" {
		t.Errorf("provider = %v, want the envelope value", out["provider"])
	}
	if out["id"] != "real-id" {
		t.Errorf("id = %v, want the envelope value", out["id"])
	}
	if out["rawdata"] != base64.StdEncoding.EncodeToString([]byte("real")) {
		t.Errorf("rawdata = %v, want the envelope value", out["rawdata"])
	}
	meta := out["meta"].(map[string]any)
	if meta["provider"] != "attacker/elsewhere" {
		t.Errorf("meta.provider = %v; the parameter is kept, just somewhere harmless", meta["provider"])
	}
}

func TestMarshalOmitsContentTypeWhenAbsent(t *testing.T) {
	out := marshalToMap(t, mqMsg{Provider: "p/d", ID: "x", Rawdata: []byte("y")})
	if _, present := out["content_type"]; present {
		t.Error("content_type present although the request carried none")
	}
}

// Names are lowercased so a document published through this collector presents
// the same field names as one published through raw-writer-2, which lowercases
// the remainder of an X-OpenDataHub-* header.
func TestFlattenQueryLowercasesNames(t *testing.T) {
	got := flattenQuery(map[string][]string{"Key": {"A"}, "ORIGIN": {"skidata"}})
	if got["key"] != "A" || got["origin"] != "skidata" {
		t.Errorf("flattenQuery = %#v, want lowercased names", got)
	}
}

func TestFlattenQuery(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]string
		want map[string]any
	}{
		{"nil", nil, nil},
		{"empty", map[string][]string{}, nil},
		{"single value becomes a scalar",
			map[string][]string{"key": {"A"}}, map[string]any{"key": "A"}},
		{"repeated value stays an array",
			map[string][]string{"tag": {"a", "b"}}, map[string]any{"tag": []string{"a", "b"}}},
		{"valueless parameter is dropped",
			map[string][]string{"flag": {}}, map[string]any{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := flattenQuery(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("flattenQuery(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
