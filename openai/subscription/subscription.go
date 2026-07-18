// Package subscription maintains opt-in, Codex-CLI-compatible OAuth
// credentials for OpenAI subscription authentication.
package subscription

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	clientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultAuthURL  = "https://auth.openai.com/oauth/authorize"
	defaultTokenURL = "https://auth.openai.com/oauth/token"
	redirectURI     = "http://localhost:1455/auth/callback"
	oauthScope      = "openid profile email offline_access"
	refreshSkew     = 5 * time.Minute
)

var (
	authorizeURL           = defaultAuthURL
	tokenURL               = defaultTokenURL
	httpClient             = http.DefaultClient
	randReader   io.Reader = rand.Reader
	now                    = time.Now
)

type authFile struct {
	APIKey      *string `json:"OPENAI_API_KEY"`
	Tokens      tokens  `json:"tokens"`
	LastRefresh string  `json:"last_refresh"`
}

type tokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// Store supplies living subscription tokens from one explicitly named auth
// file. Refresh and rewrite operations are serialized within the store.
type Store struct {
	mu       sync.Mutex
	path     string
	auth     authFile
	client   *http.Client
	tokenURL string
	now      func() time.Time
}

// Load opens an existing Codex-CLI-compatible auth file at path. Because OAuth
// refresh tokens rotate, sharing the official Codex CLI's live file can cause
// refresh-lineage contention; a separately created auth file is preferable.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load subscription auth: %w", err)
	}
	var auth authFile
	if err := json.Unmarshal(raw, &auth); err != nil {
		return nil, fmt.Errorf("decode subscription auth: %w", err)
	}
	if auth.Tokens.AccessToken == "" || auth.Tokens.AccountID == "" {
		return nil, errors.New("subscription auth file is missing access_token or account_id")
	}
	return &Store{path: path, auth: auth, client: httpClient, tokenURL: tokenURL, now: now}, nil
}

// Token returns a current bearer and its account ID, refreshing and atomically
// rewriting the auth file when the access token is near expiry.
func (s *Store) Token(ctx context.Context) (bearer, accountID string, err error) {
	if s == nil {
		return "", "", errors.New("nil subscription store")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if tokenExpiresBy(s.auth.Tokens.AccessToken, s.now().Add(refreshSkew)) {
		if err := s.refresh(ctx); err != nil {
			return "", "", err
		}
	}
	return s.auth.Tokens.AccessToken, s.auth.Tokens.AccountID, nil
}

func (s *Store) refresh(ctx context.Context) error {
	if s.auth.Tokens.RefreshToken == "" {
		return errors.New("subscription access token expired and no refresh_token is available")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {s.auth.Tokens.RefreshToken},
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
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		AccountID    string `json:"account_id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("decode subscription refresh response: %w", err)
	}
	if result.AccessToken == "" {
		return errors.New("subscription refresh response is missing access_token")
	}
	s.auth.Tokens.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		s.auth.Tokens.RefreshToken = result.RefreshToken
	}
	if result.IDToken != "" {
		s.auth.Tokens.IDToken = result.IDToken
	}
	if result.AccountID != "" {
		s.auth.Tokens.AccountID = result.AccountID
	}
	s.auth.LastRefresh = s.now().UTC().Format(time.RFC3339)
	if err := writeAuthFile(s.path, s.auth); err != nil {
		return fmt.Errorf("persist refreshed subscription token: %w", err)
	}
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

// LoginIO is the input/output pair used by the headless manual-paste login.
type LoginIO struct {
	In  io.Reader
	Out io.Writer
}

// Login performs a manual-paste OAuth authorization-code PKCE flow and writes
// a fresh auth file at path.
func Login(ctx context.Context, path string, streams LoginIO) error {
	if streams.In == nil || streams.Out == nil {
		return errors.New("subscription login requires input and output")
	}
	state, err := randomURLString(32)
	if err != nil {
		return fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, err := randomURLString(64)
	if err != nil {
		return fmt.Errorf("generate PKCE verifier: %w", err)
	}
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	query := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {oauthScope},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if _, err := fmt.Fprintln(streams.Out, authorizeURL+"?"+query.Encode()); err != nil {
		return fmt.Errorf("print authorization URL: %w", err)
	}
	callbackText, err := bufio.NewReader(streams.In).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read callback URL: %w", err)
	}
	callback, err := url.Parse(strings.TrimSpace(callbackText))
	if err != nil {
		return fmt.Errorf("parse callback URL: %w", err)
	}
	if callback.Query().Get("state") != state {
		return errors.New("OAuth callback state does not match")
	}
	code := callback.Query().Get("code")
	if code == "" {
		return errors.New("OAuth callback is missing code")
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("exchange OAuth code: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("exchange OAuth code: status %d", resp.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		AccountID    string `json:"account_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode OAuth token response: %w", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || result.AccountID == "" {
		return errors.New("OAuth token response is missing required tokens or account_id")
	}
	auth := authFile{
		Tokens: tokens{
			IDToken:      result.IDToken,
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			AccountID:    result.AccountID,
		},
		LastRefresh: now().UTC().Format(time.RFC3339),
	}
	if err := writeAuthFile(path, auth); err != nil {
		return fmt.Errorf("write subscription auth: %w", err)
	}
	return nil
}

func randomURLString(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(randReader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func writeAuthFile(path string, auth authFile) (err error) {
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
	if err := encoder.Encode(auth); err != nil {
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
