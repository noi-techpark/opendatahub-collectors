// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"

	"github.com/Nerzal/gocloak/v14"
)

func TestAuthorize(t *testing.T) {
	a := UMAAuthz{
		Authserver: "http://keycloak:8080",
		Realm:      "test",
		ClientId:   "opendatahub-push",
	}

	client := gocloak.NewClient(a.Authserver)
	ctx := context.Background()

	t.Run("authorized user", func(t *testing.T) {
		token, err := client.Login(ctx, a.ClientId, "yFdtTiTpl5o2WPgR9e7kkIn9ROwgQGAT", a.Realm, "pusher", "testpassword")
		if err != nil {
			t.Fatal("Unable to login with Keycloak", "url", a.Authserver, "realm", a.Realm, "clientId", a.ClientId, "err", err)
		}

		ok, err := a.AuthorizeURL(token.AccessToken, "/testprovider/testdataset")
		if err != nil || !ok {
			t.Error("Expected access to authorized resource", "err", err, "result", ok)
		}

		ok, err = a.AuthorizeURL(token.AccessToken, "/testprovider/nonexistingdataset")
		if err == nil || ok {
			t.Error("Expected denial for non-existing resource", "err", err, "result", ok)
		}

		ok, err = a.AuthorizeURL(token.AccessToken, "/testprovider/forbidden")
		if err == nil || ok {
			t.Error("Expected denial for explicitly forbidden resource", "err", err, "result", ok)
		}
	})

	t.Run("user authorized for a different URL", func(t *testing.T) {
		// otherpusher is authorized for /testprovider/otherdataset only, not for testdataset
		token, err := client.Login(ctx, a.ClientId, "yFdtTiTpl5o2WPgR9e7kkIn9ROwgQGAT", a.Realm, "otherpusher", "testpassword")
		if err != nil {
			t.Fatal("Unable to login with Keycloak as otherpusher", "err", err)
		}

		ok, err := a.AuthorizeURL(token.AccessToken, "/testprovider/otherdataset")
		if err != nil || !ok {
			t.Error("Expected access to otherpusher's authorized resource", "err", err, "result", ok)
		}

		ok, err = a.AuthorizeURL(token.AccessToken, "/testprovider/testdataset")
		if err == nil || ok {
			t.Error("otherpusher should not access a resource authorized for a different user", "err", err, "result", ok)
		}
	})

	t.Run("unauthorized user", func(t *testing.T) {
		// unpusher is a valid Keycloak user but has no UMA policies granting access to anything
		token, err := client.Login(ctx, a.ClientId, "yFdtTiTpl5o2WPgR9e7kkIn9ROwgQGAT", a.Realm, "unpusher", "testpassword")
		if err != nil {
			t.Fatal("Unable to login with Keycloak as unpusher", "err", err)
		}

		ok, err := a.AuthorizeURL(token.AccessToken, "/testprovider/testdataset")
		if err == nil || ok {
			t.Error("Unauthorized user should not access testdataset", "err", err, "result", ok)
		}
	})
}
