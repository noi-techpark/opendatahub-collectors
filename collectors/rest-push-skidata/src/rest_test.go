// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"encoding/base64"

	"github.com/labstack/echo/v4"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/stretchr/testify/require"
)

// realPush is an actual payload captured from the live Skidata feed.
const realPush = `{"trafficSignalState":0,"name":"Totale","freeLimit":74,"level":35,"capacity":75,` +
	`"occupancyLimit":75,"externalCounting":false,"trafficSignalMode":0,` +
	`"carpark":{"name":"BestParking","facilityNr":608448,"id":0,"shortName":"1"},"countingCategoryId":3}`

// runMiddleware drives facilityContext with a spy handler and reports what the
// downstream handler actually saw.
func runMiddleware(t *testing.T, body string) (gotBody string, facility any, status int) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/push/skidata/parking-stations/pushEvents", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	spy := func(c echo.Context) error {
		b, err := io.ReadAll(c.Request().Body)
		require.NoError(t, err)
		gotBody = string(b)
		return c.NoContent(http.StatusOK)
	}

	require.NotPanics(t, func() {
		err := facilityContext(spy)(c)
		require.NoError(t, err)
	})
	return gotBody, c.Get(facilityKey), rec.Code
}

// TestFacilityContext_ExtractsAndPreservesBody is the load-bearing test: the
// facility must be derived exactly like the transformer does, and the body must
// reach the handler byte-for-byte. A middleware that consumed the body would
// silently break every push.
func TestFacilityContext_ExtractsAndPreservesBody(t *testing.T) {
	body, facility, status := runMiddleware(t, realPush)

	require.Equal(t, realPush, body, "handler must receive the body untouched")
	require.Equal(t, "0608448", facility, "facility must be zero-padded to 7 digits like the transformer")
	require.Equal(t, http.StatusOK, status)
}

// TestFacilityContext_Failsafe: no input may panic or alter the body, however
// broken it is. Facility simply degrades to "".
func TestFacilityContext_Failsafe(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		facility string
	}{
		{"empty body", "", ""},
		{"not json", "this is not json at all", ""},
		{"truncated json", `{"carpark":{"facilityNr":608448`, ""},
		{"json array", `[1,2,3]`, ""},
		{"json null", `null`, ""},
		{"no carpark", `{"name":"Totale","level":35}`, ""},
		{"carpark not object", `{"carpark":"nope"}`, ""},
		{"carpark null", `{"carpark":null}`, ""},
		{"missing facilityNr", `{"carpark":{"id":0}}`, ""},
		{"facilityNr null", `{"carpark":{"facilityNr":null}}`, ""},
		{"facilityNr bool", `{"carpark":{"facilityNr":true}}`, ""},
		{"facilityNr object", `{"carpark":{"facilityNr":{"a":1}}}`, ""},
		// Lenient conversions that should still resolve.
		{"facilityNr as string", `{"carpark":{"facilityNr":"608448","id":0}}`, "0608448"},
		{"facilityNr as float", `{"carpark":{"facilityNr":608448.0,"id":1}}`, "0608448"},
		{"missing carpark id", `{"carpark":{"facilityNr":608448}}`, "0608448"},
		{"carpark id bad type", `{"carpark":{"facilityNr":608448,"id":"x"}}`, "0608448"},
		// Absurd numbers must not blow up the int conversion.
		{"huge facilityNr", `{"carpark":{"facilityNr":1e300}}`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, facility, status := runMiddleware(t, tc.body)
			require.Equal(t, tc.body, body, "body must survive untouched")
			require.Equal(t, tc.facility, facility)
			require.Equal(t, http.StatusOK, status)
		})
	}
}

// TestFacilityContext_LargeBodyNotTruncated guards the probe's size limit: a
// payload bigger than maxProbeBody must still reach the handler in full.
func TestFacilityContext_LargeBodyNotTruncated(t *testing.T) {
	big := `{"pad":"` + strings.Repeat("x", maxProbeBody+4096) + `"}`
	body, facility, _ := runMiddleware(t, big)
	require.Equal(t, big, body, "oversized body must not be truncated")
	require.Equal(t, "", facility)
}

// TestRouter_BothPrefixesAccepted covers the actual routing fix: the vendor's
// URL works with and without the /push prefix, and with any suffix.
func TestRouter_BothPrefixesAccepted(t *testing.T) {
	env.INBOUND_AUTH_USER = "user"
	env.INBOUND_AUTH_PASS = "pass"
	ch := make(chan dc.Input[PushPayload], 16)
	e := newRouter(ch)

	paths := []string{
		"/push/skidata/parking-stations/pushEvents",
		"/push/skidata/parking-stations/v1/postEvent",
		"/push/skidata/parking-stations",
		"/skidata/parking-stations/pushEvents",
		"/skidata/parking-stations/v1/postEvent",
		"/skidata/parking-stations",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, p, strings.NewReader(realPush))
			req.SetBasicAuth("user", "pass")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, "push to %s must be accepted", p)
		})
	}
	require.Len(t, ch, len(paths), "every accepted push must be forwarded downstream")
}

// TestRouter_Unauthorized401 confirms bad credentials are still rejected on the
// newly exposed prefix (it must not become an open endpoint).
func TestRouter_Unauthorized401(t *testing.T) {
	env.INBOUND_AUTH_USER = "user"
	env.INBOUND_AUTH_PASS = "pass"
	ch := make(chan dc.Input[PushPayload], 4)
	e := newRouter(ch)

	for _, p := range []string{"/push/skidata/parking-stations/pushEvents", "/skidata/parking-stations/pushEvents"} {
		req := httptest.NewRequest(http.MethodPost, p, strings.NewReader(realPush))
		req.SetBasicAuth("user", "wrong")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "bad creds on %s", p)
	}
	require.Len(t, ch, 0, "rejected pushes must not be forwarded")
}

// TestRouter_WrongUrlStillAttributed is the whole point of extracting on every
// request: a vendor pushing to a URL we do not serve gets a 404, and that push
// must still be traceable back to a facility instead of vanishing.
func TestRouter_WrongUrlStillAttributed(t *testing.T) {
	var seen any
	ch := make(chan dc.Input[PushPayload], 1)
	e := newRouter(ch)
	// Capture what facilityContext put on the context for an unrouted path.
	e.Any("/totally/wrong/url", func(c echo.Context) error {
		seen = c.Get(facilityKey)
		return echo.NewHTTPError(http.StatusNotFound, "nope")
	})

	req := httptest.NewRequest(http.MethodPost, "/totally/wrong/url", strings.NewReader(realPush))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "0608448", seen, "a 404'd push must still carry its facility")
	require.Len(t, ch, 0, "nothing is ingested from an unrouted path")
}

func TestPasswordify(t *testing.T) {
	require.Equal(t, "<empty>", passwordify(""))
	require.Equal(t, "<3 chars>", passwordify("abc"))
	require.Equal(t, "<6 chars>", passwordify("abcdef"))
	// first 3 + last 3 + length, middle hidden
	require.Equal(t, "abc...xyz (9)", passwordify("abcmmmxyz"))
	require.Equal(t, "OPE...585 (25)", passwordify("OPENDATAHUB_BRUOSP_640585"))
}

// TestBasicAuthShape proves credentials are masked (never logged in full) and
// that a broken/absent Authorization header degrades to a marker, not a panic.
func TestBasicAuthShape(t *testing.T) {
	mk := func(v string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		if v != "" {
			r.Header.Set("Authorization", v)
		}
		return r
	}
	basic := func(u, p string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
	}

	u, p := basicAuthShape(mk(basic("OPENDATAHUB_582998", "TZPv6q3o0YKt3XlEe7qjl2d0H90D63aTV57Xs4uS")))
	require.Equal(t, "OPE...998 (18)", u)
	require.Equal(t, "TZP...4uS (40)", p)
	require.NotContains(t, p, "6q3o", "must not log the middle of the password")

	u, p = basicAuthShape(mk(""))
	require.Equal(t, "<none>", u)
	require.Equal(t, "<none>", p)

	u, _ = basicAuthShape(mk("Bearer sometoken"))
	require.Equal(t, "<non-basic>", u)

	u, _ = basicAuthShape(mk("Basic !!!notbase64!!!"))
	require.Equal(t, "<unparseable>", u)

	require.NotPanics(t, func() { basicAuthShape(nil) })
}

// TestResponseStatus checks the status reported to the log for the error paths,
// where the response has not been written yet when the middleware unwinds.
func TestResponseStatus(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodPost, "/", nil), httptest.NewRecorder())

	require.Equal(t, http.StatusUnauthorized, responseStatus(c, echo.ErrUnauthorized))
	require.Equal(t, http.StatusInternalServerError, responseStatus(c, io.ErrUnexpectedEOF))
	require.Equal(t, http.StatusOK, responseStatus(c, nil))
}

// TestHealthEndpointsUnauthenticated: probes and vendor health checks must keep
// working without credentials on both prefixes.
func TestHealthEndpointsUnauthenticated(t *testing.T) {
	ch := make(chan dc.Input[PushPayload], 1)
	e := newRouter(ch)

	paths := []string{
		"/health",
		"/push/skidata/parking-stations/health",
		"/push/skidata/parking-stations/v1/health",
		"/skidata/parking-stations/health",
		"/skidata/parking-stations/v1/health",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "health %s", p)
	}
}
