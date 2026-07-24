// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
)

// pushPaths are the URL prefixes Skidata pushes to. Only the "/push/..." form
// was served originally, but some facilities were configured with the shorter
// form; on the shared push.api host those requests fall through to the
// dc-rest-push catch-all ingress and are silently answered with 404, so their
// data never reaches this collector. Serving both prefixes makes either vendor
// configuration land here.
var pushPaths = []string{
	"/push/skidata/parking-stations",
	"/skidata/parking-stations",
}

func serve(inputCh chan<- dc.Input[PushPayload]) {
	e := newRouter(inputCh)
	e.Logger.Fatal(e.Start(":8080"))
}

func newRouter(inputCh chan<- dc.Input[PushPayload]) *echo.Echo {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	// Registered globally and unconditionally: every request on every path is
	// probed, with no sampling. This is what makes a push attributable even
	// when it never reaches a handler — wrong URL (404) or bad credentials
	// (401) included, which are precisely the cases that used to be invisible.
	e.Use(facilityContext)

	noContent := func(c echo.Context) error { return c.NoContent(http.StatusOK) }

	e.GET("/health", noContent)

	for _, p := range pushPaths {
		e.GET(p, noContent)
		e.HEAD(p, noContent)
		e.GET(p+"/health", noContent)
		e.HEAD(p+"/health", noContent)
		e.GET(p+"/v1/health", noContent)
		e.HEAD(p+"/v1/health", noContent)

		g := e.Group(p, middleware.BasicAuth(validateInbound))
		push := func(c echo.Context) error { return handlePush(c, inputCh) }
		// The vendor appends its own suffix (e.g. "/pushEvents",
		// "/v1/postEvent"). The suffix is irrelevant to us, so accept any.
		g.POST("", push)
		g.POST("/*", push)
	}

	return e
}

// facilityCtxKeyType is unexported so the context value cannot collide with
// keys set by other packages.
type facilityCtxKeyType struct{}

var facilityCtxKey = facilityCtxKeyType{}

// facilityKey is the echo context key holding the extracted facility id.
const facilityKey = "facility"

// maxProbeBody bounds how much of a possibly unauthenticated request body is
// buffered while sniffing the facility id.
const maxProbeBody = 1 << 20 // 1 MiB

// FacilityFromContext returns the facility id extracted by facilityContext, or
// "" when it could not be determined.
func FacilityFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(facilityCtxKey).(string)
	return s
}

// facilityContext sniffs the facility id out of the push payload, puts it on
// the echo and request contexts, and logs the outcome of every request —
// success, 401, 404 and any other error alike — so it is possible to tell which
// facilities actually deliver data. It runs for every request, with no
// sampling and no path filtering.
//
// It is deliberately failsafe: an unreadable body, malformed json, unexpected
// types or an outright panic all degrade to an empty facility id and never
// affect the request itself. The body is always handed to the next handler
// intact.
func facilityContext(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		facility := probeFacility(c)

		c.Set(facilityKey, facility)
		if req := c.Request(); req != nil {
			// WithContext shallow-copies the request, preserving the body
			// probeFacility already restored.
			c.SetRequest(req.WithContext(context.WithValue(req.Context(), facilityCtxKey, facility)))
		}

		err := next(c)

		attrs := []any{
			"facility", facility,
			"status", responseStatus(c, err),
		}
		if req := c.Request(); req != nil {
			attrs = append(attrs, "method", req.Method, "uri", req.RequestURI)
		}
		if err != nil {
			attrs = append(attrs, "err", err.Error())
		}
		slog.Info("skidata push", attrs...)

		return err
	}
}

// responseStatus reports the status the client will actually see. An error
// returned by a downstream handler is only turned into a response after the
// middleware chain unwinds, so it has to be read off the error itself.
func responseStatus(c echo.Context, err error) int {
	if err != nil {
		var he *echo.HTTPError
		if errors.As(err, &he) {
			return he.Code
		}
		return http.StatusInternalServerError
	}
	if c != nil && c.Response() != nil {
		if s := c.Response().Status; s != 0 {
			return s
		}
	}
	// Handler wrote nothing and reported no error: net/http emits 200.
	return http.StatusOK
}

// probeFacility reads and restores the request body, then extracts the facility
// id from it. It never panics and never fails: anything it cannot determine
// comes back as "".
func probeFacility(c echo.Context) (facility string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("facility extraction panicked, continuing without it", "recover", fmt.Sprint(r))
			facility = ""
		}
	}()

	if c == nil {
		return ""
	}
	req := c.Request()
	if req == nil || req.Body == nil {
		return ""
	}

	buf, rerr := io.ReadAll(io.LimitReader(req.Body, maxProbeBody))
	// Hand back a complete body no matter what: the bytes consumed here,
	// followed by anything left unread for payloads beyond maxProbeBody.
	req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), req.Body))
	if rerr != nil {
		return ""
	}

	return extractFacility(buf)
}

// extractFacility mirrors how the parking-skidata transformer derives the
// facility id from a push event (transformers/parking-skidata/src/main.go):
// carpark.facilityNr zero-padded to 7 digits.
func extractFacility(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	cp, ok := m["carpark"].(map[string]any)
	if !ok {
		return ""
	}
	nr, ok := asInt(cp["facilityNr"])
	if !ok {
		return ""
	}
	return fmt.Sprintf("%07d", nr)
}

// asInt leniently converts a json value to an int. encoding/json decodes
// numbers into float64 here; the other cases just make the sniffer tolerant of
// a vendor changing the payload shape.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		// Reject NaN and values far outside int range instead of converting.
		if n != n || n < -1e15 || n > 1e15 {
			return 0, false
		}
		return int(n), true
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

func validateInbound(username, password string, c echo.Context) (bool, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(env.INBOUND_AUTH_USER)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(env.INBOUND_AUTH_PASS)) == 1 {
		return true, nil
	}
	return false, nil
}

func handlePush(c echo.Context, inputCh chan<- dc.Input[PushPayload]) error {
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Unable to read request body").WithInternal(err)
	}

	slog.Debug("Incoming push", "facility", c.Get(facilityKey))

	inputCh <- dc.NewInput(c.Request().Context(), PushPayload{
		Body: body,
	})

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Data accepted",
	})
}
