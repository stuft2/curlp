package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stuft2/httpp/internal/config"
)

type Provider struct {
	Client *http.Client
	Cache  Cache
}

func (p Provider) Apply(ctx context.Context, req *http.Request, a config.Auth) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("configured authentication requires HTTPS")
	}
	switch a.Type {
	case "basic":
		secret, err := Resolve(ctx, a.Password)
		if err != nil {
			return err
		}
		req.SetBasicAuth(a.Username, secret)
	case "bearer":
		token, err := Resolve(ctx, a.Token)
		if err != nil {
			return err
		}
		if !validToken(token) {
			return fmt.Errorf("invalid bearer token")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	case "oauth2-client-credentials":
		token, err := p.token(ctx, req.URL.Hostname(), a)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	default:
		return fmt.Errorf("unsupported authentication type")
	}
	return nil
}
func validToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c <= 32 || c >= 127 {
			return false
		}
	}
	return true
}
func (p Provider) token(ctx context.Context, host string, a config.Auth) (string, error) {
	key := cacheKey(host, a)
	if token, ok := p.Cache.load(key, time.Now()); ok && validToken(token) {
		return token, nil
	}
	clientID, err := Resolve(ctx, a.ClientID)
	if err != nil {
		return "", fmt.Errorf("resolve OAuth client ID: %w", err)
	}
	secret, err := Resolve(ctx, a.ClientSecret)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	if len(a.Scopes) > 0 {
		form.Set("scope", strings.Join(a.Scopes, " "))
	}
	if a.TokenAuthMethod == "client_secret_post" {
		form.Set("client_id", clientID)
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("invalid token endpoint")
	}
	if req.URL.Scheme != "https" || req.URL.Hostname() == "" || req.URL.User != nil {
		return "", fmt.Errorf("token endpoint requires HTTPS")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if a.TokenAuthMethod != "client_secret_post" {
		req.SetBasicAuth(url.QueryEscape(clientID), url.QueryEscape(secret))
	}
	client := http.Client{Timeout: 30 * time.Second}
	if p.Client != nil {
		client = *p.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	started := time.Now()
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("OAuth token request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("OAuth token endpoint returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return "", fmt.Errorf("invalid or oversized OAuth token response")
	}
	var token struct {
		AccessToken string          `json:"access_token"`
		TokenType   string          `json:"token_type"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if json.Unmarshal(b, &token) != nil || !validToken(token.AccessToken) || !strings.EqualFold(token.TokenType, "Bearer") {
		return "", fmt.Errorf("invalid OAuth Bearer token response")
	}
	var seconds int64
	if json.Unmarshal(token.ExpiresIn, &seconds) == nil && seconds > 0 && seconds <= int64((1<<63-1)/int64(time.Second)) {
		p.Cache.save(key, token.AccessToken, started.Add(time.Duration(seconds)*time.Second))
	}
	return token.AccessToken, nil
}
