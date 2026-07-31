package toolkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebSearchConstruction(t *testing.T) {
	// R-1EKW-L5BR
	if tool := WebSearch("key"); tool == nil {
		t.Fatal("WebSearch(non-empty key) returned nil")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal(`WebSearch("") did not panic`)
			}
		}()
		WebSearch("")
	}()
}

func TestWebSearchSchema(t *testing.T) {
	// R-1FSS-YX2G
	tool := WebSearch("key")
	if tool.Name() != "WebSearch" {
		t.Fatalf("Name() = %q, want WebSearch", tool.Name())
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.JSONSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.Required, []string{"query"}) {
		t.Fatalf("required = %v, want [query]", schema.Required)
	}
	wantProperties := []string{
		"query", "count", "offset", "country", "search_lang", "freshness",
		"safesearch", "result_filter", "extra_snippets", "spellcheck",
	}
	if len(schema.Properties) != len(wantProperties) {
		t.Fatalf("properties = %v, want exactly %v", schema.Properties, wantProperties)
	}
	for _, name := range wantProperties {
		property, ok := schema.Properties[name]
		if !ok || property.Description == "" {
			t.Errorf("property %q = %+v, want a non-empty description", name, property)
		}
	}
}

func TestWebSearchRequest(t *testing.T) {
	// R-1H0P-COT5
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/res/v1/web/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Subscription-Token"); got != "constructed-key" {
			t.Errorf("X-Subscription-Token = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		wantQuery := map[string][]string{
			"q":                {"brave search"},
			"text_decorations": {"0"},
			"count":            {"20"},
			"offset":           {"2"},
			"country":          {"DE"},
			"search_lang":      {"de"},
			"freshness":        {"pw"},
			"safesearch":       {"strict"},
			"result_filter":    {"web,news"},
			"extra_snippets":   {"true"},
			"spellcheck":       {"false"},
		}
		if !reflect.DeepEqual(map[string][]string(r.URL.Query()), wantQuery) {
			t.Errorf("query = %v, want %v", r.URL.Query(), wantQuery)
		}
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	t.Cleanup(server.Close)
	setWebSearchBaseURL(t, server.URL)

	input := map[string]any{
		"query": "brave search", "count": 20, "offset": 2, "country": "DE",
		"search_lang": "de", "freshness": "pw", "safesearch": "strict",
		"result_filter": []string{"web", "news"}, "extra_snippets": true,
		"spellcheck": false,
	}
	if _, err := callTool(t, WebSearch("constructed-key"), input); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}

	var omittedQuery map[string][]string
	omitted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		omittedQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	t.Cleanup(omitted.Close)
	setWebSearchBaseURL(t, omitted.URL)
	if _, err := callTool(t, WebSearch("key"), map[string]any{"query": "minimal"}); err != nil {
		t.Fatal(err)
	}
	wantOmitted := map[string][]string{"q": {"minimal"}, "text_decorations": {"0"}}
	if !reflect.DeepEqual(omittedQuery, wantOmitted) {
		t.Fatalf("omitted query = %v, want %v", omittedQuery, wantOmitted)
	}
}

func TestWebSearchReducesResponse(t *testing.T) {
	// R-1I8L-QGJU
	payload := `{
		"query":{"original":"agentkit"},
		"mixed":{"main":[{"type":"web","index":0}]},
		"web":{"results":[{
			"title":"AgentKit","url":"https://example.com/agentkit","description":"An agent toolkit.",
			"extra_snippets":["First excerpt","Second excerpt"],
			"meta_url":{"hostname":"example.com"},"profile":{"name":"Example"}
		}]},
		"news":{"results":[{"title":"Agent news","url":"https://example.com/news","description":"News item.","age":"1 day ago"}]},
		"faq":{"results":[{"title":"What is it?","url":"https://example.com/faq","description":"A toolkit."}]}
	}`
	server := newWebSearchServer(t, http.StatusOK, payload, "")
	setWebSearchBaseURL(t, server.URL)

	got, err := callTool(t, WebSearch("key"), map[string]any{"query": "agentkit"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"web": []any{map[string]any{
			"title": "AgentKit", "url": "https://example.com/agentkit", "description": "An agent toolkit.",
			"extra_snippets": []any{"First excerpt", "Second excerpt"},
		}},
		"news": []any{map[string]any{
			"title": "Agent news", "url": "https://example.com/news", "description": "News item.",
		}},
		"faq": []any{map[string]any{
			"title": "What is it?", "url": "https://example.com/faq", "description": "A toolkit.",
		}},
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("result = %#v, want %#v", decoded, want)
	}
	for _, absent := range []string{"mixed", "query", "meta_url", "profile", "videos", "discussions", "infobox"} {
		if strings.Contains(got, `"`+absent+`"`) {
			t.Errorf("result contains omitted field or section %q: %s", absent, got)
		}
	}
}

func TestWebSearchHTTPError(t *testing.T) {
	// R-1JGI-48AJ
	tests := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		want       []string
	}{
		{
			name:   "measured invalid subscription token",
			status: http.StatusUnprocessableEntity,
			body:   `{"type":"ErrorResponse","error":{"id":"x","status":422,"code":"SUBSCRIPTION_TOKEN_INVALID","detail":"The provided subscription token is invalid."}}`,
			want:   []string{"422 Unprocessable Entity", "The provided subscription token is invalid."},
		},
		{
			name:       "rate limit with retry delay",
			status:     http.StatusTooManyRequests,
			body:       `not json`,
			retryAfter: "15",
			want:       []string{"429 Too Many Requests", "Retry-After", "15"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newWebSearchServer(t, tt.status, tt.body, tt.retryAfter)
			setWebSearchBaseURL(t, server.URL)
			got, err := callTool(t, WebSearch("key"), map[string]any{"query": "test"})
			if err == nil || got != "" {
				t.Fatalf("call = %q, %v; want empty result and error", got, err)
			}
			for _, part := range tt.want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error = %q, want %q", err, part)
				}
			}
		})
	}
}

func TestWebSearchTimeout(t *testing.T) {
	// R-1KOE-I018
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	setWebSearchBaseURL(t, server.URL)
	oldTimeout := webSearchTimeout
	webSearchTimeout = 50 * time.Millisecond
	t.Cleanup(func() { webSearchTimeout = oldTimeout })

	start := time.Now()
	got, err := callTool(t, WebSearch("key"), map[string]any{"query": "hang"})
	elapsed := time.Since(start)
	if err == nil || got != "" {
		t.Fatalf("call = %q, %v; want timeout error", got, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
		t.Fatalf("error = %q, want deadline exceeded", err)
	}
	if elapsed > time.Second {
		t.Fatalf("timeout took %v, want less than one second with test duration", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("server never received request")
	}
	server.CloseClientConnections()
}

func setWebSearchBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	old := braveWebSearchBaseURL
	braveWebSearchBaseURL = baseURL
	t.Cleanup(func() { braveWebSearchBaseURL = old })
}

func newWebSearchServer(t *testing.T, status int, body, retryAfter string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}
