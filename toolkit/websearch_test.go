package toolkit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func successfulSearchResponse(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"web":{"results":[]}}`)),
		Request:    req,
	}
}

func TestWebSearchBaseURL(t *testing.T) {
	// R-1D5U-0OIN
	var requestURLs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURLs = append(requestURLs, r.URL.String())
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	t.Cleanup(server.Close)

	for _, baseURL := range []string{server.URL, server.URL + "/"} {
		_, err := callTool(t, WebSearch("key", WithBaseURL(baseURL)), map[string]any{"query": "same"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(requestURLs) != 2 {
		t.Fatalf("server received %d requests, want 2", len(requestURLs))
	}
	if requestURLs[0] != "/res/v1/web/search?q=same&text_decorations=0" {
		t.Fatalf("request URL = %q, want Brave web search path", requestURLs[0])
	}
	if requestURLs[1] != requestURLs[0] {
		t.Fatalf("trailing-slash request URL = %q, want byte-identical %q", requestURLs[1], requestURLs[0])
	}

	var defaultURL string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		defaultURL = req.URL.String()
		return successfulSearchResponse(req), nil
	})}
	if _, err := callTool(t, WebSearch("key", WithHTTPClient(client)), map[string]any{"query": "default"}); err != nil {
		t.Fatal(err)
	}
	if defaultURL != braveWebSearchBaseURL+"/res/v1/web/search?q=default&text_decorations=0" {
		t.Fatalf("default request URL = %q", defaultURL)
	}
}

func TestWebSearchInvalidBaseURLAndCredentialPrecedence(t *testing.T) {
	// R-1EDQ-EG9C
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		return successfulSearchResponse(req), nil
	})}

	got, err := callTool(t, WebSearch("key", WithBaseURL(""), WithHTTPClient(client)), map[string]any{"query": "test"})
	if got != "" || !errors.Is(err, agentkit.ErrInvalidConfig) || errors.Is(err, agentkit.ErrMissingCredential) {
		t.Fatalf("empty base URL call = %q, %v; want ErrInvalidConfig only", got, err)
	}
	if !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("error = %q, want base URL named", err)
	}

	got, err = callTool(t, WebSearch("", WithBaseURL(""), WithHTTPClient(client)), map[string]any{"query": "test"})
	if got != "" || !errors.Is(err, agentkit.ErrMissingCredential) {
		t.Fatalf("empty key and base URL call = %q, %v; want ErrMissingCredential", got, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("transport recorded %d requests, want zero", requests.Load())
	}
}

func TestWebSearchHTTPClient(t *testing.T) {
	// R-1FLM-S801
	var injectedRequests atomic.Int32
	injected := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		injectedRequests.Add(1)
		if got := req.Header.Get("X-Subscription-Token"); got != "injected-key" {
			t.Errorf("X-Subscription-Token = %q, want injected-key", got)
		}
		return successfulSearchResponse(req), nil
	})}
	if _, err := callTool(t, WebSearch("injected-key", WithHTTPClient(injected)), map[string]any{"query": "client"}); err != nil {
		t.Fatal(err)
	}
	if got := injectedRequests.Load(); got != 1 {
		t.Fatalf("injected transport round trips = %d, want 1", got)
	}

	oldTransport := http.DefaultClient.Transport
	var defaultRequests atomic.Int32
	http.DefaultClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		defaultRequests.Add(1)
		return successfulSearchResponse(req), nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = oldTransport })
	if _, err := callTool(t, WebSearch("default-key"), map[string]any{"query": "default client"}); err != nil {
		t.Fatal(err)
	}
	if got := defaultRequests.Load(); got != 1 {
		t.Fatalf("default client transport round trips = %d, want 1", got)
	}
}

func TestWebSearchMissingCredential(t *testing.T) {
	// R-UGF9-OO0Z
	var tool agentkit.Tool
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		tool = WebSearch(BraveAPIKey(""))
	}()
	if recovered != nil {
		t.Fatalf("WebSearch with an absent key panicked: %v", recovered)
	}
	if tool == nil {
		t.Fatal("WebSearch with an absent key returned nil")
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)

	tool = WebSearch(BraveAPIKey(""), WithBaseURL(server.URL))
	got, err := callTool(t, tool, map[string]any{"query": "agentkit"})
	if got != "" {
		t.Errorf("result = %q, want empty", got)
	}
	if !errors.Is(err, agentkit.ErrMissingCredential) {
		t.Fatalf("error = %v, want ErrMissingCredential", err)
	}
	for _, part := range []string{"WebSearch", "Brave Search API key"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("error = %q, want it to name %q", err, part)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("requests = %d, want 0", got)
	}
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
	input := map[string]any{
		"query": "brave search", "count": 20, "offset": 2, "country": "DE",
		"search_lang": "de", "freshness": "pw", "safesearch": "strict",
		"result_filter": []string{"web", "news"}, "extra_snippets": true,
		"spellcheck": false,
	}
	if _, err := callTool(t, WebSearch("constructed-key", WithBaseURL(server.URL)), input); err != nil {
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
	if _, err := callTool(t, WebSearch("key", WithBaseURL(omitted.URL)), map[string]any{"query": "minimal"}); err != nil {
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

	got, err := callTool(t, WebSearch("key", WithBaseURL(server.URL)), map[string]any{"query": "agentkit"})
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
			got, err := callTool(t, WebSearch("key", WithBaseURL(server.URL)), map[string]any{"query": "test"})
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
	// R-1GTJ-5ZQQ
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))

	start := time.Now()
	got, err := callTool(t, WebSearch("key",
		WithBaseURL(server.URL),
		WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}),
	), map[string]any{"query": "hang"})
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

	var observedDeadline time.Time
	var observedAt time.Time
	client := &http.Client{Timeout: 0, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		observedAt = time.Now()
		deadline, ok := req.Context().Deadline()
		if !ok {
			t.Fatal("request context has no deadline")
		}
		observedDeadline = deadline
		return successfulSearchResponse(req), nil
	})}
	if _, err := callTool(t, WebSearch("key", WithHTTPClient(client)), map[string]any{"query": "deadline"}); err != nil {
		t.Fatal(err)
	}
	if remaining := observedDeadline.Sub(observedAt); remaining <= 0 || remaining > webSearchTimeout {
		t.Fatalf("request deadline remaining = %v, want within (0, %v]", remaining, webSearchTimeout)
	}
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
