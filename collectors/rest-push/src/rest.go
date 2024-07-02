// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"text/template"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type pushResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
}

func serve(send chan<- restMsg) {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Decompress())
	e.Use(middleware.CORS())

	e.GET("/", func(c echo.Context) error {
		return c.Redirect(http.StatusPermanentRedirect, Config.SwaggerURL)
	})

	e.GET("/health", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	e.POST("/push/:provider/:dataset", func(c echo.Context) error {
		return push(c, send)
	}, NewUMAAuthz().Middleware())

	authUrl := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", Config.AuthURL, Config.AuthRealm)
	apispec := loadApispec("openapi3.yaml.tmpl", authUrl)
	e.GET("/apispec", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml; charset=utf-8", apispec)
	})

	e.Logger.Fatal(e.Start(":8080"))
}

func loadApispec(file string, authUrl string) []byte {
	t, err := template.ParseFiles(file)
	if err != nil {
		log.Fatal("Error loading apispec", "err", err)
	}
	var buf bytes.Buffer
	t.Execute(&buf, map[string]string{
		"authurl": authUrl,
	})
	if err != nil {
		log.Fatal("Error loading apispec", "err", err)
	}
	return buf.Bytes()
}

func push(c echo.Context, sendQ chan<- restMsg) error {
	var msg restMsg
	msg.ID = uuid.NewString()
	msg.Timestamp = time.Now()

	msg.Provider = c.Param("provider")
	msg.Dataset = c.Param("dataset")

	msg.Query = c.QueryParams()

	slog.Debug("Incoming push", "msg", msg)

	msg.ContentType = c.Request().Header.Get(echo.HeaderContentType)

	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Unable to read request body").WithInternal(err)
	}
	msg.Payload = body
	msg.Response = make(chan bool)
	sendQ <- msg

	ok := <-msg.Response

	if ok {
		return c.JSON(http.StatusOK, pushResponse{"Data accepted", msg.ID})
	} else {
		msg.Payload = []byte("not logged")
		slog.Error("Got nok from rabbitmq", "UID", msg.ID, "msg", msg)
		return echo.NewHTTPError(http.StatusInternalServerError, "Unable to relay data")
	}
}
