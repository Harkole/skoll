package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DiscordUser represents the identity returned by Discord's /users/@me API.
type DiscordUser struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
}

// DisplayName returns either GlobalName or Username.
func (u *DiscordUser) DisplayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

// OAuthExchanger defines operations for Discord OAuth2 exchange.
type OAuthExchanger interface {
	ExchangeCode(ctx context.Context, code string) (string, error)
	FetchUser(ctx context.Context, accessToken string) (*DiscordUser, error)
}

// DefaultDiscordOAuth is the production implementation contacting Discord's API.
type DefaultDiscordOAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTPClient   *http.Client
}

// NewDefaultDiscordOAuth creates a standard Discord OAuth exchanger.
func NewDefaultDiscordOAuth(clientID, clientSecret, redirectURI string, timeout time.Duration) *DefaultDiscordOAuth {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &DefaultDiscordOAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		HTTPClient:   &http.Client{Timeout: timeout},
	}
}

// ExchangeCode exchanges an authorization code for an OAuth2 bearer token.
func (d *DefaultDiscordOAuth) ExchangeCode(ctx context.Context, code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", d.ClientID)
	data.Set("client_secret", d.ClientSecret)
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", d.RedirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://discord.com/api/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discord token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("discord token exchange error (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode discord token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access_token received from discord")
	}

	return tokenResp.AccessToken, nil
}

// FetchUser retrieves the user profile corresponding to the given OAuth2 access token.
func (d *DefaultDiscordOAuth) FetchUser(ctx context.Context, accessToken string) (*DiscordUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discord user info: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch user info from discord (status %d): %s", resp.StatusCode, string(body))
	}

	var user DiscordUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to decode user profile: %w", err)
	}
	if user.ID == "" {
		return nil, fmt.Errorf("invalid user profile returned by discord (missing ID)")
	}

	return &user, nil
}
