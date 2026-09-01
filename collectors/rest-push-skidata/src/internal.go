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

	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	e.Use(middleware.Recover())

	e.PUT("/internal/credentials", func(c echo.Context) error {
		var creds []FacilityCredential
		if err := c.Bind(&creds); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "expected [{facility, username, password}]")
		}
		// An empty set would unsubscribe every facility. The backoffice never
		// sends one, so it means a bug or a truncated body, and acting on it
		// would take the whole collector down silently.
		if len(creds) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "refusing an empty credential set")
		}
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
