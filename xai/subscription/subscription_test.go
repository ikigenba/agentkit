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

func TestLoadRawTokenResponseReturnsUnexpiredBearer(t *testing.T) {
	fixedNow := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "auth.json")
	want := testJWT(fixedNow.Add(time.Hour), map[string]any{"sub": "user-without-account-id"})
	writeTestTokens(t, path, tokenResponse{
		AccessToken:  want,
		RefreshToken: "refresh-token",
		IDToken:      testJWT(fixedNow.Add(time.Hour), nil),
	}, 0o600)

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	store.now = func() time.Time { return fixedNow }
	got, err := store.Token(context.Background())

	// R-DTW8-9X67
	if err != nil || got != want {
		t.Fatalf("Token() = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLoadRejectsResponsesWithoutTopLevelAccessToken(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty raw response", raw: `{}`},
		{name: "grok CLI wrapper", raw: `{"https://accounts.x.ai::client":{"access_token":"nested"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			if err := os.WriteFile(path, []byte(tt.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := Load(path)

			// R-DV44-NOWW
			if store != nil || err == nil || !strings.Contains(err.Error(), "access_token") {
				t.Fatalf("Load() = (%v, %v), want nil store and access_token error", store, err)
			}
		})
	}
}

func TestStoreRefreshesOnceAndAtomicallyRewrites(t *testing.T) {
	fixedNow := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	newBearer := testJWT(fixedNow.Add(time.Hour), nil)
	var refreshes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes.Add(1)
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("client_id") != clientID {
			t.Errorf("refresh form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  newBearer,
			RefreshToken: "new-refresh",
			IDToken:      "new-id",
		})
	}))
	defer server.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	writeTestTokens(t, path, tokenResponse{
		AccessToken:  testJWT(fixedNow.Add(4*time.Minute), nil),
		RefreshToken: "old-refresh",
		IDToken:      "old-id",
	}, 0o644)
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	store.client, store.tokenURL = server.Client(), server.URL
	store.now = func() time.Time { return fixedNow }

	const callers = 24
	start := make(chan struct{})
	results := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			bearer, err := store.Token(context.Background())
			if err != nil {
				results <- "error: " + err.Error()
				return
			}
			results <- bearer
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	// R-DWC1-1GNL
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refresh requests = %d, want 1", got)
	}
	for got := range results {
		if got != newBearer {
			t.Errorf("Token() bearer = %q, want %q", got, newBearer)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("credential mode = %o, want 600", got)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var rewritten tokenResponse
	if err := json.Unmarshal(raw, &rewritten); err != nil {
		t.Fatalf("rewritten credential is incomplete JSON: %v", err)
	}
	if rewritten.AccessToken != newBearer || rewritten.RefreshToken != "new-refresh" || rewritten.IDToken != "new-id" {
		t.Errorf("rewritten credentials = %#v", rewritten)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "auth.json" {
		t.Errorf("atomic rewrite left unexpected files: %v", entries)
	}
}

func TestRefreshCarriesForwardOrRotatesTokens(t *testing.T) {
	fixedNow := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		response    tokenResponse
		wantRefresh string
		wantID      string
	}{
		{
			name:        "omitted values are carried forward",
			response:    tokenResponse{AccessToken: testJWT(fixedNow.Add(time.Hour), nil)},
			wantRefresh: "old-refresh",
			wantID:      "old-id",
		},
		{
			name: "returned values rotate",
			response: tokenResponse{
				AccessToken:  testJWT(fixedNow.Add(time.Hour), nil),
				RefreshToken: "rotated-refresh",
				IDToken:      "rotated-id",
			},
			wantRefresh: "rotated-refresh",
			wantID:      "rotated-id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "auth.json")
			writeTestTokens(t, path, tokenResponse{
				AccessToken:  testJWT(fixedNow.Add(-time.Minute), nil),
				RefreshToken: "old-refresh",
				IDToken:      "old-id",
			}, 0o600)
			store, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			store.client, store.tokenURL = server.Client(), server.URL
			store.now = func() time.Time { return fixedNow }
			if _, err := store.Token(context.Background()); err != nil {
				t.Fatalf("Token: %v", err)
			}

			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			var rewritten map[string]any
			if err := json.Unmarshal(raw, &rewritten); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			fresh, err := Load(path)
			if err != nil {
				t.Fatalf("fresh Load: %v", err)
			}
			fresh.now = func() time.Time { return fixedNow }
			bearer, err := fresh.Token(context.Background())

			// R-DXJX-F8EA
			if err != nil || bearer != tt.response.AccessToken || rewritten["refresh_token"] != tt.wantRefresh || rewritten["id_token"] != tt.wantID {
				t.Fatalf("fresh Token/rewrite = (%q, %v)/%#v", bearer, err, rewritten)
			}
			if len(rewritten) != 3 {
				t.Fatalf("rewrite is not the raw three-field token-response shape: %#v", rewritten)
			}
		})
	}
}

func testJWT(expiry time.Time, extra map[string]any) string {
	claims := map[string]any{"exp": expiry.Unix()}
	for key, value := range extra {
		claims[key] = value
	}
	payload, _ := json.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func writeTestTokens(t *testing.T, path string, tokens tokenResponse, mode os.FileMode) {
	t.Helper()
	raw, err := json.Marshal(tokens)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, mode); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
