package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadDerivesAccountFromRawTokenResponse(t *testing.T) {
	fixedNow := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	future := fixedNow.Add(time.Hour)
	tests := []struct {
		name       string
		idAccount  string
		accessAcct string
		want       string
	}{
		{name: "id token takes precedence", idAccount: "acct-from-id", accessAcct: "acct-from-access", want: "acct-from-id"},
		{name: "access token is fallback", accessAcct: "acct-from-access", want: "acct-from-access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tokens.json")
			writeTestTokens(t, path, tokenResponse{
				AccessToken:  testJWT(future, tt.accessAcct),
				RefreshToken: "refresh",
				IDToken:      testJWT(future, tt.idAccount),
			}, 0o600)

			store, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			store.now = func() time.Time { return fixedNow }
			bearer, account, err := store.Token(context.Background())
			if err != nil {
				t.Fatalf("Token: %v", err)
			}

			// R-PV6N-URM6
			if bearer != testJWT(future, tt.accessAcct) || account != tt.want {
				t.Fatalf("Token() = (%q, %q), want bearer and %q", bearer, account, tt.want)
			}
		})
	}
}

func TestLoadRejectsMissingAccessTokenAndAccountClaim(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty raw response", raw: `{}`, want: "access_token"},
		{name: "codex wrapper", raw: `{"tokens":{"access_token":"nested"}}`, want: "access_token"},
		{name: "missing account claim", raw: mustJSON(tokenResponse{
			AccessToken: testJWT(time.Now().Add(time.Hour), ""),
			IDToken:     testJWT(time.Now().Add(time.Hour), ""),
		}), want: "account claim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tokens.json")
			if err := os.WriteFile(path, []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := Load(path)

			// R-PWEK-8JCV
			if store != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() = (%v, %v), want nil store and error containing %q", store, err, tt.want)
			}
		})
	}
}

func TestRefreshCarriesForwardOrRotatesTokens(t *testing.T) {
	fixedNow := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		response    tokenResponse
		wantRefresh string
		wantIDAcct  string
	}{
		{
			name:        "omitted values are carried forward",
			response:    tokenResponse{AccessToken: testJWT(fixedNow.Add(time.Hour), "")},
			wantRefresh: "old-refresh",
			wantIDAcct:  "acct-original",
		},
		{
			name: "returned values rotate",
			response: tokenResponse{
				AccessToken:  testJWT(fixedNow.Add(time.Hour), ""),
				RefreshToken: "new-refresh",
				IDToken:      testJWT(fixedNow.Add(2*time.Hour), "acct-rotated"),
			},
			wantRefresh: "new-refresh",
			wantIDAcct:  "acct-rotated",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "tokens.json")
			writeTestTokens(t, path, tokenResponse{
				AccessToken:  testJWT(fixedNow.Add(-time.Minute), ""),
				RefreshToken: "old-refresh",
				IDToken:      testJWT(fixedNow.Add(time.Hour), "acct-original"),
			}, 0o644)
			store, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			store.client, store.tokenURL = server.Client(), server.URL
			store.now = func() time.Time { return fixedNow }
			if _, _, err := store.Token(context.Background()); err != nil {
				t.Fatalf("Token: %v", err)
			}

			var rewritten map[string]any
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &rewritten); err != nil {
				t.Fatalf("rewritten token response: %v", err)
			}
			fresh, err := Load(path)
			if err != nil {
				t.Fatalf("fresh Load: %v", err)
			}
			fresh.now = func() time.Time { return fixedNow }
			_, account, err := fresh.Token(context.Background())
			idToken, idTokenOK := rewritten["id_token"].(string)

			// R-PXMG-MB3K
			if err != nil || rewritten["refresh_token"] != tt.wantRefresh || !idTokenOK || tokenAccountID(idToken) != tt.wantIDAcct || account != tt.wantIDAcct {
				t.Fatalf("rewrite = %#v, fresh account = %q, error = %v", rewritten, account, err)
			}
			if len(rewritten) != 3 {
				t.Fatalf("rewrite is not the three-field raw token-response shape: %#v", rewritten)
			}
		})
	}
}

func TestStoreRefreshesExpiredTokenOnceAndAtomicallyRewrites(t *testing.T) {
	fixedNow := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	var refreshes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse refresh form: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  testJWT(fixedNow.Add(time.Hour), ""),
			RefreshToken: "rotated-refresh",
			IDToken:      testJWT(fixedNow.Add(time.Hour), "acct-rotated"),
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	writeTestTokens(t, path, tokenResponse{
		AccessToken:  testJWT(fixedNow.Add(-time.Minute), ""),
		RefreshToken: "old-refresh",
		IDToken:      testJWT(fixedNow.Add(time.Hour), "acct-1"),
	}, 0o644)
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	store.client, store.tokenURL = server.Client(), server.URL
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
	wantResult := testJWT(fixedNow.Add(time.Hour), "") + "|acct-1"
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
	var rewritten tokenResponse
	if err := json.Unmarshal(raw, &rewritten); err != nil {
		t.Fatalf("rewritten auth is not complete JSON: %v", err)
	}
	if rewritten.RefreshToken != "rotated-refresh" || tokenAccountID(rewritten.IDToken) != "acct-rotated" {
		t.Fatalf("rotated tokens = %#v", rewritten)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read auth directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "auth.json" {
		t.Fatalf("atomic rewrite left temporary files: %v", entries)
	}
}

func testJWT(expiry time.Time, account string) string {
	claims := map[string]any{"exp": expiry.Unix()}
	if account != "" {
		claims[authClaimNamespace] = map[string]string{"chatgpt_account_id": account}
	}
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeTestTokens(t *testing.T, path string, tokens tokenResponse, mode os.FileMode) {
	t.Helper()
	raw, err := json.Marshal(tokens)
	if err != nil {
		t.Fatalf("marshal tokens: %v", err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatalf("write tokens: %v", err)
	}
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
