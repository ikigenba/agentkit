// Package subscription maintains opt-in OAuth credentials for xAI
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
	clientID    = "b1a00492-073a-47ea-816f-4c329264a828"
	tokenURL    = "https://auth.x.ai/oauth2/token"
	refreshSkew = 5 * time.Minute
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// Store supplies living xAI subscription tokens from one explicitly named
// token-response file. Refresh and rewrite operations are serialized so a
// rotating refresh-token lineage is never raced by concurrent callers.
type Store struct {
	mu       sync.Mutex
	path     string
	tokens   tokenResponse
	client   *http.Client
	tokenURL string
	now      func() time.Time
}

// Load opens the raw OAuth token-endpoint response at path. The caller owns
// path selection and initial login; this package performs no path discovery
// and reads no ambient credentials.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load xAI subscription token response: %w", err)
	}
	var tokens tokenResponse
	if err := json.Unmarshal(raw, &tokens); err != nil {
		return nil, fmt.Errorf("decode xAI subscription token response: %w", err)
	}
	if tokens.AccessToken == "" {
		return nil, errors.New("xAI subscription token response is missing access_token")
	}
	return &Store{
		path:     path,
		tokens:   tokens,
		client:   http.DefaultClient,
		tokenURL: tokenURL,
		now:      time.Now,
	}, nil
}

// Token returns a current bearer, refreshing and atomically rewriting the
// token response when the access token is expired or near expiry.
func (s *Store) Token(ctx context.Context) (string, error) {
	if s == nil {
		return "", errors.New("nil xAI subscription store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if tokenExpiresBy(s.tokens.AccessToken, s.now().Add(refreshSkew)) {
		if err := s.refresh(ctx); err != nil {
			return "", err
		}
	}
	return s.tokens.AccessToken, nil
}

func (s *Store) refresh(ctx context.Context) error {
	if s.tokens.RefreshToken == "" {
		return errors.New("xAI subscription access token expired and no refresh_token is available")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {s.tokens.RefreshToken},
		"client_id":     {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create xAI subscription refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh xAI subscription token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read xAI subscription refresh response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("refresh xAI subscription token: status %d", resp.StatusCode)
	}
	var refreshed tokenResponse
	if err := json.Unmarshal(raw, &refreshed); err != nil {
		return fmt.Errorf("decode xAI subscription refresh response: %w", err)
	}
	if refreshed.AccessToken == "" {
		return errors.New("xAI subscription refresh response is missing access_token")
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = s.tokens.RefreshToken
	}
	if refreshed.IDToken == "" {
		refreshed.IDToken = s.tokens.IDToken
	}
	if err := writeTokenResponse(s.path, refreshed); err != nil {
		return fmt.Errorf("persist refreshed xAI subscription token: %w", err)
	}
	s.tokens = refreshed
	return nil
}

func tokenExpiresBy(token string, cutoff time.Time) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return true
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
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

func writeTokenResponse(path string, tokens tokenResponse) (err error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".agentkit-xai-auth-*")
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
