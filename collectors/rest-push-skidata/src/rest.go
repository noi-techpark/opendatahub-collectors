// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/subtle"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
)

func serve(inputCh chan<- dc.Input[PushPayload]) {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.GET("/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	e.GET("/push/skidata/parking-stations", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.HEAD("/push/skidata/parking-stations", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/push/skidata/parking-stations/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.HEAD("/push/skidata/parking-stations/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	skidata := e.Group("/push/skidata/parking-stations",
		middleware.BasicAuth(validateInbound))

	skidata.POST("/:facilityId", func(c echo.Context) error {
		return handlePush(c, inputCh)
	})

	e.Logger.Fatal(e.Start(":8080"))
}

func validateInbound(username, password string, c echo.Context) (bool, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte(env.INBOUND_AUTH_USER)) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(env.INBOUND_AUTH_PASS)) == 1 {
		return true, nil
	}
	return false, nil
}

func handlePush(c echo.Context, inputCh chan<- dc.Input[PushPayload]) error {
	facilityId := c.Param("facilityId")

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Unable to read request body").WithInternal(err)
	}

	slog.Debug("Incoming push", "facilityId", facilityId)

	inputCh <- dc.NewInput(c.Request().Context(), PushPayload{
		FacilityId: facilityId,
		Body:       body,
	})

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Data accepted",
	})
}
