// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// serveInternal runs the listener the backoffice pushes credential changes to.
//
// A SECOND port, never on the ingress. The public listener is reachable from
// the internet by design -- Skidata posts to it -- so a path on it is one
// ingress rule away from being public too. The pull remains the correctness
// guarantee (R9.8); this only removes the wait for the next refresh.
// apply deliberately takes no context: the facilities it starts must outlive
// the request that delivered them. Handing it the request's context would
// cancel every pushed facility the moment the response completed.
func serveInternal(ctx context.Context, apply func([]FacilityCredential)) {
	if env.INTERNAL_PORT == "" {
		slog.Info("no internal listener: INTERNAL_PORT is unset, credentials arrive on the refresh only")
		return
	}
	// Refused rather than started open. Both halves default to empty, and
	// comparing "" against "" succeeds -- so an unset pair would authenticate
	// `Basic Og==` and let anything in the cluster replace the credential set.
	if env.INTERNAL_AUTH_USER == "" || env.INTERNAL_AUTH_PASS == "" {
		slog.Error("refusing to start the internal listener: INTERNAL_AUTH_USER or " +
			"INTERNAL_AUTH_PASS is empty, which would accept an empty Basic header")
		return
	}

	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	// Logged like the public listener. Without this a rejected push -- wrong
	// credentials, malformed body -- produced no line at all on this side, so
	// the only evidence was a warning in the backoffice's own log.
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.PUT("/internal/credentials", func(c echo.Context) error {
		var creds []FacilityCredential
		if err := c.Bind(&creds); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expected [{facility, username, password}]")
		}
		// An empty set is applied, not refused. It is valid data -- the last
		// credential was deleted -- and a truncated body fails to decode above
		// rather than arriving as []. Refusing it here would only disagree with
		// the refresh, which applies the same empty set a minute later anyway.
		apply(creds)
		return c.NoContent(http.StatusNoContent)
	}, internalAuth)

	go func() {
		<-ctx.Done()
		e.Close()
	}()
	slog.Info("internal listener up", "port", env.INTERNAL_PORT)
	if err := e.Start(":" + env.INTERNAL_PORT); err != nil && err != http.ErrServerClosed {
		slog.Error("the internal listener stopped", "err", err)
	}
}

// internalAuth gates the push with its own credential pair, not the one used
// for the outbound pull: rotating one direction must not break the other.
func internalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, pass, ok := c.Request().BasicAuth()
		// Constant-time, and both compared even when the user is already wrong,
		// so a caller cannot learn the username from the response timing.
		okUser := subtle.ConstantTimeCompare([]byte(user), []byte(env.INTERNAL_AUTH_USER)) == 1
		okPass := subtle.ConstantTimeCompare([]byte(pass), []byte(env.INTERNAL_AUTH_PASS)) == 1
		if !ok || !okUser || !okPass {
			return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		return next(c)
	}
}
