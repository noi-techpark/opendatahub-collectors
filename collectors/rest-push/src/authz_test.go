package main

import (
	"context"
	"testing"

	"github.com/Nerzal/gocloak/v13"
)

func TestAuthorize(t *testing.T) {
	a := UMAAuthz{
		Authserver: "http://keycloak:8080",
		Realm:      "test",
		ClientId:   "opendatahub-push",
	}

	client := gocloak.NewClient(a.Authserver)

	ctx := context.Background()

	token, err := client.Login(ctx, a.ClientId, "yFdtTiTpl5o2WPgR9e7kkIn9ROwgQGAT", a.Realm, "pusher", "testpassword")
	// get a token for our user so we can do resource requests
	// token, err := client.LoginClient(ctx, "opendatahub-push", "testpassword", a.Realm)
	if err != nil {
		t.Error("Unable to login with Keycloak", "url", a.Authserver, "realm", a.Realm, "clientId", a.ClientId, "err", err)
		t.FailNow()
	}

	ok, err := a.AuthorizeURL(token.AccessToken, "/testprovider/testdataset")
	if err != nil || !ok {
		t.Error("Failed authz call", "err", err, "result", ok)
	}

	ok, err = a.AuthorizeURL(token.AccessToken, "/testprovider/nonexistingdataset")
	if err == nil || ok {
		t.Error("Resource does not exist", "err", err, "result", ok)
	}

	ok, err = a.AuthorizeURL(token.AccessToken, "/testprovider/forbidden")
	if err == nil || ok {
		t.Error("Managed to access forbidden resource", "err", err, "result", ok)
	}
}
