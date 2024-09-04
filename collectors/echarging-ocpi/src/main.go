// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

const base_url string = "your.doma.in"

type Version struct {
	Enpoint
}

func main() {
	InitLogger()

	r := gin.New()

	if os.Getenv("GIN_LOG") == "PRETTY" {
		r.Use(gin.Logger())
	} else {
		// Enable slog logging for gin framework
		// https://github.com/samber/slog-gin
		r.Use(sloggin.New(slog.Default()))
	}

	r.Use(gin.Recovery())

	r.GET("/ocpi/cpo/versions", versions)
	r.GET("/ocpi/cpo/2.2.1/credentials", credentials)
	r.GET("/health", health)
	r.Run()
}

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func versions(c *gin.Context) {
	res := make(map string)

	c.JSON(http.StatusOK, res)
}

func credentials(c *gin.Context) {
	c.Status(http.StatusOK)
}
