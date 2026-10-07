package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleOAuthClient is the seam between the HTTP handler and Google's real
// OAuth endpoints (OPS-068a) — an interface, not a concrete type, so
// handler tests can swap in a fake instead of making real network calls to
// Google, the same reason every domain in this codebase (teams, workitems,
// notifications) is built behind an interface rather than a concrete
// struct.
type GoogleOAuthClient interface {
	// AuthCodeURL returns the URL to redirect the browser to, with state
	// embedded for the callback to verify against (CSRF protection).
	AuthCodeURL(state string) string
	// Exchange trades an authorization code (from the callback's ?code=)
	// for the Google identity it belongs to.
	Exchange(ctx context.Context, code string) (GoogleIdentity, error)
}

// googleUserInfoURL is Google's standard OpenID Connect userinfo endpoint.
// Calling it with the access token (rather than verifying the ID token's
// JWT signature by hand, or adding an OIDC library just for that one
// check) keeps this starter's dependency surface minimal — the same
// instinct behind "net/http, not Gin" and "hand-rolled migrations, not
// golang-migrate" elsewhere in this codebase.
const googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"

type OAuth2GoogleClient struct {
	config     *oauth2.Config
	httpClient *http.Client
}

func NewOAuth2GoogleClient(clientID, clientSecret, redirectURL string) OAuth2GoogleClient {
	return OAuth2GoogleClient{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint:     google.Endpoint,
		},
		httpClient: http.DefaultClient,
	}
}

func (c OAuth2GoogleClient) AuthCodeURL(state string) string {
	return c.config.AuthCodeURL(state)
}

func (c OAuth2GoogleClient) Exchange(ctx context.Context, code string) (GoogleIdentity, error) {
	token, err := c.config.Exchange(ctx, code)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: exchange google code: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: build google userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: call google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return GoogleIdentity{}, fmt.Errorf("auth: google userinfo returned %d: %s", resp.StatusCode, body)
	}

	var payload struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return GoogleIdentity{}, fmt.Errorf("auth: decode google userinfo: %w", err)
	}

	if payload.Subject == "" || payload.Email == "" {
		return GoogleIdentity{}, fmt.Errorf("auth: google userinfo missing sub or email")
	}

	return GoogleIdentity{
		Subject: payload.Subject,
		Email:   payload.Email,
		Name:    payload.Name,
	}, nil
}

// GenerateOAuthState returns a random, URL-safe value for the OAuth
// state parameter — CSRF protection on the redirect round trip. Not a
// credential (never hashed, never compared against anything secret), just
// needs to be unguessable and tied to this one browser session, which is
// why the handler stores it in a short-lived cookie rather than anywhere
// durable.
func GenerateOAuthState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("auth: generate oauth state: %w", err)
	}

	return base64.URLEncoding.EncodeToString(raw), nil
}
