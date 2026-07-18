package subscription

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreRefreshesExpiredTokenOnceAndAtomicallyRewrites(t *testing.T) {
	fixedNow := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	var refreshes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse refresh form: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token":  testJWT(fixedNow.Add(time.Hour)),
			"refresh_token": "rotated-refresh",
			"id_token":      "rotated-id",
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	initial := authFile{
		Tokens: tokens{
			AccessToken:  testJWT(fixedNow.Add(-time.Minute)),
			RefreshToken: "old-refresh",
			AccountID:    "acct-1",
		},
	}
	writeTestAuth(t, path, initial, 0o644)
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	store.client = server.Client()
	store.tokenURL = server.URL
	store.now = func() time.Time { return fixedNow }

	const callers = 20
	start := make(chan struct{})
	results := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			bearer, account, err := store.Token(context.Background())
			if err != nil {
				results <- "error: " + err.Error()
				return
			}
			results <- bearer + "|" + account
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	// R-DHHV-MCPJ
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refresh requests = %d, want 1", got)
	}
	wantResult := testJWT(fixedNow.Add(time.Hour)) + "|acct-1"
	for got := range results {
		if got != wantResult {
			t.Errorf("Token result = %q, want %q", got, wantResult)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("auth mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten auth: %v", err)
	}
	var rewritten authFile
	if err := json.Unmarshal(raw, &rewritten); err != nil {
		t.Fatalf("rewritten auth is not complete JSON: %v", err)
	}
	if rewritten.Tokens.RefreshToken != "rotated-refresh" || rewritten.Tokens.IDToken != "rotated-id" {
		t.Fatalf("rotated tokens = %#v", rewritten.Tokens)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read auth directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "auth.json" {
		t.Fatalf("atomic rewrite left temporary files: %v", entries)
	}
}

func TestFlowUsesPKCEVerifiesStateAndWritesCompatibleAuth(t *testing.T) {
	fixedNow := time.Date(2026, 7, 17, 13, 0, 0, 0, time.UTC)
	random := append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 64)...)
	state := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	verifier := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 64))
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	var exchanges atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse exchange form: %v", err)
		}
		if r.Form.Get("code") != "manual-code" || r.Form.Get("code_verifier") != verifier || r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("exchange form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"access_token":  "access",
			"refresh_token": "refresh",
			"id_token":      "id",
			"account_id":    "acct-login",
		})
	}))
	defer server.Close()

	restore := setLoginGlobals(server.URL+"/authorize", server.URL, server.Client(), random, fixedNow)
	defer restore()
	flow, err := BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	callback := "http://localhost:1455/auth/callback?code=manual-code&state=" + url.QueryEscape(state)
	path := filepath.Join(t.TempDir(), "auth.json")

	// R-I8OP-9XZ7
	if err := flow.Complete(context.Background(), path, callback); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	printed, err := url.Parse(flow.AuthorizeURL())
	if err != nil {
		t.Fatalf("parse printed authorize URL: %v", err)
	}
	query := printed.Query()
	if printed.Path != "/authorize" || query.Get("state") != state || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != challenge {
		t.Fatalf("authorize URL = %s", printed)
	}
	if exchanges.Load() != 1 {
		t.Fatalf("token exchanges = %d, want 1", exchanges.Load())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat auth file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("auth mode = %o, want 600", info.Mode().Perm())
	}
	var auth map[string]any
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	if err := json.Unmarshal(raw, &auth); err != nil {
		t.Fatalf("decode auth file: %v", err)
	}
	tokenMap, ok := auth["tokens"].(map[string]any)
	if !ok || tokenMap["access_token"] != "access" || tokenMap["refresh_token"] != "refresh" || tokenMap["id_token"] != "id" || tokenMap["account_id"] != "acct-login" {
		t.Fatalf("auth JSON shape = %#v", auth)
	}
	if auth["OPENAI_API_KEY"] != nil || auth["last_refresh"] != fixedNow.Format(time.RFC3339) {
		t.Fatalf("auth metadata = %#v", auth)
	}

	badRandom := append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 64)...)
	restoreBad := setLoginGlobals(server.URL+"/authorize", server.URL, server.Client(), badRandom, fixedNow)
	defer restoreBad()
	badFlow, err := BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin for wrong state: %v", err)
	}
	badPath := filepath.Join(t.TempDir(), "auth.json")
	err = badFlow.Complete(context.Background(), badPath, "http://localhost:1455/auth/callback?code=manual-code&state=wrong")
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("wrong-state error = %v", err)
	}
	if exchanges.Load() != 1 {
		t.Fatalf("wrong state performed token exchange; count = %d", exchanges.Load())
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatalf("wrong state wrote auth file: %v", err)
	}
}

func TestFlowRejectsMissingOrMalformedRedirectWithoutExchange(t *testing.T) {
	var exchanges atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	random := append(bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 64)...)
	restore := setLoginGlobals(server.URL+"/authorize", server.URL, server.Client(), random, time.Now())
	defer restore()
	flow, err := BeginLogin()
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}

	// R-I9WL-NPPW
	for _, pasted := range []string{"", ":not-a-url", "/auth/callback?code=x&state=y"} {
		err := flow.Complete(context.Background(), filepath.Join(t.TempDir(), "auth.json"), pasted)
		if err == nil || !strings.Contains(err.Error(), "full redirect URL expected") {
			t.Errorf("Complete(%q) error = %v, want full redirect URL error", pasted, err)
		}
		if err != nil && strings.Contains(err.Error(), "state") {
			t.Errorf("Complete(%q) error = %v, want distinct from state mismatch", pasted, err)
		}
	}
	if got := exchanges.Load(); got != 0 {
		t.Fatalf("invalid redirects performed %d token exchanges, want 0", got)
	}
}

func testJWT(expiry time.Time) string {
	payload, _ := json.Marshal(map[string]int64{"exp": expiry.Unix()})
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeTestAuth(t *testing.T, path string, auth authFile, mode os.FileMode) {
	t.Helper()
	raw, err := json.Marshal(auth)
	if err != nil {
		t.Fatalf("marshal auth: %v", err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatalf("write auth: %v", err)
	}
}

func setLoginGlobals(auth, token string, client *http.Client, random []byte, fixed time.Time) func() {
	oldAuthorizeURL, oldTokenURL := authorizeURL, tokenURL
	oldHTTPClient, oldRandomBytes, oldNow := httpClient, randomBytes, now
	authorizeURL, tokenURL = auth, token
	offset := 0
	httpClient, randomBytes, now = client, func(dst []byte) (int, error) {
		n := copy(dst, random[offset:])
		offset += n
		return n, nil
	}, func() time.Time { return fixed }
	return func() {
		authorizeURL, tokenURL = oldAuthorizeURL, oldTokenURL
		httpClient, randomBytes, now = oldHTTPClient, oldRandomBytes, oldNow
	}
}
