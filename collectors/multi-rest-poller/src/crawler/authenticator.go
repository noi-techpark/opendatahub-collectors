package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type Authenticator interface {
	PrepareRequest(req *http.Request) error
}

type NoopAuthenticator struct {
}

func (np NoopAuthenticator) PrepareRequest(req *http.Request) error {
	return nil
}

type AuthenticatorConfig struct {
	OAuthConfig `yaml:",inline"`
	Type        string `yaml:"type,omitempty"` // basic | bearer | oauth
	Token       string `yaml:"token,omitempty"`
}

type AuthenticatorImpl struct {
	enabled       bool
	oauthProvider *OAuthProvider
	cfg           AuthenticatorConfig
}

func NewAuthenticator(config AuthenticatorConfig) Authenticator {
	enabled := false
	if len(config.Type) != 0 {
		enabled = true
		if config.Type != "basic" && config.Type != "bearer" && config.Type != "oauth" {
			slog.Error(fmt.Sprintf("Unsupported authentication type. Use 'basic' or 'bearer' or oauth. Got: %s", config.Type))
			panic(fmt.Sprintf("Unsupported authentication type. Use 'basic' or 'bearer' or oauth. Got: %s", config.Type))
		}
	}

	var oauthProvider *OAuthProvider = nil
	if config.Type == "oauth" {
		oauthProvider = NewOAuthProvider(config.OAuthConfig)
	}

	a := &AuthenticatorImpl{
		enabled:       enabled,
		oauthProvider: oauthProvider,
		cfg:           config,
	}
	return a
}

func (a AuthenticatorImpl) PrepareRequest(req *http.Request) error {
	if !a.enabled {
		return nil
	}

	// Inject authentication headers if needed.
	if a.cfg.Type == "oauth" {
		token, err := a.oauthProvider.GetToken()
		if err != nil {
			return fmt.Errorf("could not get oauth token: %s", err.Error())
		}
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	} else if a.cfg.Type == "basic" {
		req.SetBasicAuth(a.cfg.Username, a.cfg.Password)
	} else if a.cfg.Type == "bearer" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.cfg.Token))
	}
	return nil
}

type OAuthConfig struct {
	Method       string   `yaml:"method,omitempty"` // password | client_credentials
	TokenURL     string   `yaml:"tokenUrl,omitempty"`
	ClientID     string   `yaml:"clientId,omitempty"`
	ClientSecret string   `yaml:"clientSecret,omitempty"`
	Username     string   `yaml:"username,omitempty"`
	Password     string   `yaml:"password,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

// OAuthProvider struct
type OAuthProvider struct {
	conf        *oauth2.Config
	clientCreds *clientcredentials.Config
	token       *oauth2.Token
	mu          sync.Mutex
	username    string
	password    string
}

func NewOAuthProvider(cfg OAuthConfig) *OAuthProvider {
	authMethod := cfg.Method
	tokenURL := cfg.TokenURL
	clientID := cfg.ClientID
	clientSecret := cfg.ClientSecret

	wrapper := &OAuthProvider{
		username: cfg.Username,
		password: cfg.Password,
	}

	switch authMethod {
	case "password":
		wrapper.conf = &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				TokenURL: tokenURL,
			},
			Scopes: cfg.Scopes,
		}
	case "client_credentials":
		wrapper.clientCreds = &clientcredentials.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			TokenURL:     tokenURL,
			Scopes:       cfg.Scopes,
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
		token, err = w.conf.PasswordCredentialsToken(ctx, w.username, w.password)
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
