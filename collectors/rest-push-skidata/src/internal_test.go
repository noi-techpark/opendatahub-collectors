// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func internalRequest(t *testing.T, user, pass, body string, apply func([]FacilityCredential)) int {
	t.Helper()
	e := echo.New()
	e.PUT("/internal/credentials", func(c echo.Context) error {
		var creds []FacilityCredential
		if err := c.Bind(&creds); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "bad body")
		}
		apply(creds)
		return c.NoContent(http.StatusNoContent)
	}, internalAuth)

	req := httptest.NewRequest(http.MethodPut, "/internal/credentials", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code
}

func TestTheInternalListenerRefusesWrongCredentials(t *testing.T) {
	env.INTERNAL_AUTH_USER, env.INTERNAL_AUTH_PASS = "collector", "right"
	body := `[{"facility":"A","username":"u","password":"p"}]`
	called := false
	apply := func([]FacilityCredential) { called = true }

	for _, tc := range []struct{ name, user, pass string }{
		{"no credentials", "", ""},
		{"wrong password", "collector", "wrong"},
		{"wrong user", "someone", "right"},
	} {
		if got := internalRequest(t, tc.user, tc.pass, body, apply); got != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", tc.name, got)
		}
	}
	if called {
		t.Error("an unauthorised push reached the supervisor")
	}
}

// Deleting the last credential is a legitimate operation, and the refresh would
// apply the same empty set a minute later. Refusing it here would only make the
// two paths disagree.
func TestAnEmptySetIsAppliedNotRefused(t *testing.T) {
	env.INTERNAL_AUTH_USER, env.INTERNAL_AUTH_PASS = "collector", "right"
	called := false
	got := internalRequest(t, "collector", "right", `[]`, func([]FacilityCredential) { called = true })
	if got != http.StatusNoContent {
		t.Errorf("status %d, want 204", got)
	}
	if !called {
		t.Error("an empty set was dropped instead of applied")
	}
}

func TestAnAuthorisedPushReachesTheSupervisor(t *testing.T) {
	env.INTERNAL_AUTH_USER, env.INTERNAL_AUTH_PASS = "collector", "right"
	var got []FacilityCredential
	code := internalRequest(t, "collector", "right",
		`[{"facility":"A","username":"u","password":"p"},{"facility":"B","username":"u2","password":"p2"}]`,
		func(c []FacilityCredential) { got = c })

	if code != http.StatusNoContent {
		t.Fatalf("status %d, want 204", code)
	}
	if len(got) != 2 || got[0].Facility != "A" || got[1].Password != "p2" {
		t.Fatalf("the supervisor got %+v", got)
	}
}

// The push applies the same diff as the refresh, so a pushed set that matches
// what is already running must not restart anything.
func TestAPushThatChangesNothingRestartsNothing(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	t.Cleanup(s.Stop)

	set := []FacilityCredential{cred("A", "1"), cred("B", "1")}
	s.Apply(t.Context(), set)
	waitFor(t, "the set to start", func() bool { return r.isLive("A") && r.isLive("B") })

	if got := s.Apply(t.Context(), set); got.Changed() {
		t.Fatalf("a no-op push changed something: %+v", got)
	}
}
