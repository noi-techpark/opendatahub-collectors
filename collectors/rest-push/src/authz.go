// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Nerzal/gocloak/v14"
	"github.com/golang-jwt/jwt/v5/request"
	"github.com/labstack/echo/v4"
)

type UMAAuthz struct {
	Authserver string
	Realm      string
	ClientId   string
}

func NewUMAAuthz() UMAAuthz {
	return UMAAuthz{
		Authserver: Config.AuthURL,
		Realm:      Config.AuthRealm,
		ClientId:   Config.AuthClientId,
	}
}

func (a *UMAAuthz) AuthorizeURL(token string, url string) (bool, error) {
	client := gocloak.NewClient(a.Authserver)

	ctx := context.Background()

	// https://www.keycloak.org/docs/latest/authorization_services/index.html#_service_obtaining_permissions
	// Get a authorization decision from keycloak, supplying a URI
	t := true
	formatURI := "uri"
	req := gocloak.RequestingPartyTokenOptions{
		Audience:                      &a.ClientId,
		Permissions:                   []string{url},
		PermissionResourceMatchingURI: &t,
		PermissionResourceFormat:      &formatURI,
	}

	res, err := client.GetRequestingPartyPermissionDecision(ctx, token, a.Realm, req)
	if err != nil {
		return false, err
	}
	return *res.Result, nil
}

func (a UMAAuthz) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get Oauth token
			token, err := request.AuthorizationHeaderExtractor.ExtractToken(c.Request())
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "Missing or invalid Authorization header")
			}

			url := fmt.Sprintf("/%s/%s", c.Param("provider"), c.Param("dataset"))

			authorized, err := a.AuthorizeURL(token, url)
			if err != nil {
				switch e := err.(type) {
				// Handle 401 vs 403
				case *gocloak.APIError:
					if e.Code == http.StatusUnauthorized {
						return echo.NewHTTPError(e.Code, "Authentication failed").WithInternal(e)
					}
				}
				return echo.NewHTTPError(http.StatusForbidden, "Not authorized").WithInternal(err)
			}
			if !authorized {
				return echo.NewHTTPError(http.StatusForbidden, "Not authorized")
			}

			if err := next(c); err != nil {
				return err
			}
			return nil
		}
	}
}
