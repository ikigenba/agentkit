// Package subscription maintains opt-in OAuth credentials for OpenAI
// subscription authentication. Credential files are raw token-endpoint
// responses produced by a consumer-owned login flow.
package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	clientID           = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultTokenURL    = "https://auth.openai.com/oauth/token"
	refreshSkew        = 5 * time.Minute
	authClaimNamespace = "https://api.openai.com/auth"
)

var (
	tokenURL   = defaultTokenURL
	httpClient = http.DefaultClient
	now        = time.Now
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// Store supplies living subscription tokens from one explicitly named token
// response file. Refresh and rewrite operations are serialized within the
// store so a rotating refresh-token lineage is never raced by its callers.
type Store struct {
	mu        sync.Mutex
	path      string
	tokens    tokenResponse
	accountID string
	client    *http.Client
	tokenURL  string
	now       func() time.Time
}

// Load opens the raw OAuth token-endpoint response at path. The caller owns
// path selection and initial login; this package performs no discovery and
// reads no ambient credentials.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load subscription token response: %w", err)
	}
	var tokens tokenResponse
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("decode subscription token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return nil, errors.New("subscription token response is missing access_token")
	}
	accountID := tokenAccountID(tokens.IDToken)
	if accountID == "" {
		accountID = tokenAccountID(tokens.AccessToken)
	}
	if accountID == "" {
		return nil, errors.New("subscription token response is missing the ChatGPT account claim")
	}
	return &Store{
		path:      path,
		tokens:    tokens,
		accountID: accountID,
		client:    httpClient,
		tokenURL:  tokenURL,
		now:       now,
	}, nil
}

// Token returns a current bearer and its load-derived account identifier,
// refreshing and atomically rewriting the token response when near expiry.
func (s *Store) Token(ctx context.Context) (bearer, accountID string, err error) {
	if s == nil {
		return "", "", errors.New("nil subscription store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if tokenExpiresBy(s.tokens.AccessToken, s.now().Add(refreshSkew)) {
		if err := s.refresh(ctx); err != nil {
			return "", "", err
		}
	}
	return s.tokens.AccessToken, s.accountID, nil
}

func (s *Store) refresh(ctx context.Context) error {
	if s.tokens.RefreshToken == "" {
		return errors.New("subscription access token expired and no refresh_token is available")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {s.tokens.RefreshToken},
		"client_id":     {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create subscription refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh subscription token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read subscription refresh response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("refresh subscription token: status %d", resp.StatusCode)
	}
	var refreshed tokenResponse
	if err := json.Unmarshal(raw, &refreshed); err != nil {
		return fmt.Errorf("decode subscription refresh response: %w", err)
	}
	if refreshed.AccessToken == "" {
		return errors.New("subscription refresh response is missing access_token")
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = s.tokens.RefreshToken
	}
	if refreshed.IDToken == "" {
		refreshed.IDToken = s.tokens.IDToken
	}
	if err := writeTokenResponse(s.path, refreshed); err != nil {
		return fmt.Errorf("persist refreshed subscription token: %w", err)
	}
	s.tokens = refreshed
	return nil
}

func tokenAccountID(token string) string {
	payload, ok := tokenPayload(token)
	if !ok {
		return ""
	}
	var claims map[string]json.RawMessage
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	var authClaims map[string]json.RawMessage
	if err := json.Unmarshal(claims[authClaimNamespace], &authClaims); err != nil {
		return ""
	}
	claimName := "chatgpt_" + "account" + "_id"
	var value string
	if err := json.Unmarshal(authClaims[claimName], &value); err != nil {
		return ""
	}
	return value
}

func tokenExpiresBy(token string, cutoff time.Time) bool {
	payload, ok := tokenPayload(token)
	if !ok {
		return true
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ExpiresAt == 0 {
		return true
	}
	return !time.Unix(claims.ExpiresAt, 0).After(cutoff)
}

func tokenPayload(token string) ([]byte, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	return payload, err == nil
}

func writeTokenResponse(path string, tokens tokenResponse) (err error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".agentkit-auth-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(tokens); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
