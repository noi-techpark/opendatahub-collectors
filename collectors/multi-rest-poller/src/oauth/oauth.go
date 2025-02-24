// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package oauth

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuthProvider struct
type OAuthProvider struct {
	conf        *oauth2.Config
	clientCreds *clientcredentials.Config
	token       *oauth2.Token
	mu          sync.Mutex
}

// NewOAuthProvider initializes the OAuth2 wrapper
func NewOAuthProvider() *OAuthProvider {
	authMethod := os.Getenv("OAUTH_METHOD")
	tokenURL := os.Getenv("OAUTH_TOKEN_URL")
	clientID := os.Getenv("OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_CLIENT_SECRET")

	wrapper := &OAuthProvider{}

	switch authMethod {
	case "password":
		wrapper.conf = &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenURL,
			},
			Scopes: []string{"read", "write"},
		}
	case "client_credentials":
		wrapper.clientCreds = &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     tokenURL,
			Scopes:       []string{"read", "write"},
		}
	default:
		slog.Error("Unsupported OAUTH_METHOD. Use 'password' or 'client_credentials'")
		panic("Unsupported OAUTH_METHOD. Use 'password' or 'client_credentials'")
	}

	return wrapper
}

// GetToken retrieves a valid access token (refreshing if necessary)
func (w *OAuthProvider) GetToken() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	ctx := context.Background()

	// If token exists and is still valid, return it
	if w.token != nil && w.token.Valid() {
		return w.token.AccessToken, nil
	}

	// Fetch new token
	var token *oauth2.Token
	var err error

	if w.conf != nil { // Password flow
		username := os.Getenv("OAUTH_USERNAME")
		password := os.Getenv("OAUTH_PASSWORD")
		token, err = w.conf.PasswordCredentialsToken(ctx, username, password)
	} else { // Client Credentials flow
		token, err = w.clientCreds.Token(ctx)
	}

	if err != nil {
		return "", err
	}

	// Store new token
	w.token = token
	return token.AccessToken, nil
}
