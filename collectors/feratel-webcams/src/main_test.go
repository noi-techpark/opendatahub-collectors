// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/url"
	"reflect"
	"testing"
)

func TestLanguageURL(t *testing.T) {
	base, _ := url.Parse("http://wtvxmlp.feratel.com/xmlpan/x3/infoxml.jsp?pg=ABC&lg=de&showKeywords=1")

	got, _ := url.Parse(languageURL(base, "lg", "it"))
	if lg := got.Query().Get("lg"); lg != "it" {
		t.Errorf("expected lg=it, got %q", lg)
	}
	if pg := got.Query().Get("pg"); pg != "ABC" {
		t.Errorf("expected other params to be kept, got pg=%q", pg)
	}
	if base.Query().Get("lg") != "de" {
		t.Errorf("base URL must not be modified")
	}
}

func TestParseLanguages(t *testing.T) {
	got := parseLanguages(" de, it,,en ")
	want := []string{"de", "it", "en"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
