package toolkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebFetchSchema(t *testing.T) {
	// R-1LWA-VRRX
	tool := WebFetch()
	if tool.Name() != "WebFetch" {
		t.Fatalf("Name() = %q, want WebFetch", tool.Name())
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
	if !reflect.DeepEqual(schema.Required, []string{"url"}) {
		t.Fatalf("required = %v, want [url]", schema.Required)
	}
	if len(schema.Properties) != 2 {
		t.Fatalf("properties = %v, want exactly url and timeout_seconds", schema.Properties)
	}
	for _, name := range []string{"url", "timeout_seconds"} {
		property, ok := schema.Properties[name]
		if !ok || property.Description == "" {
			t.Errorf("property %q = %+v, want a non-empty description", name, property)
		}
	}
}

func TestWebFetchRejectsNonHTTPURLsBeforeRequest(t *testing.T) {
	// R-1N47-9JIM
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	for _, target := range []string{"ftp://example.com/page", "file:///tmp/page", "relative/page", server.URL + "/%zz"} {
		got, err := callTool(t, WebFetch(), map[string]any{"url": target})
		if err == nil || got != "" {
			t.Errorf("WebFetch(%q) = %q, %v; want empty result and input error", target, got, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want none", requests.Load())
	}
}

func TestHTMLToMarkdownPreservesBlocksAndDropsNonContent(t *testing.T) {
	// R-1OC3-NB9B
	input := `<!doctype html><html><head><title>secret title</title><style>hidden style</style></head>
	<body><script>hidden script</script><h1>Alpha</h1><h6>Omega</h6>
	<ul><li>red</li><li>blue</li></ul><ol><li>first</li><li>second</li></ol>
	<pre><code>if x &lt; y {
	return x
}</code></pre></body></html>`
	got, err := htmlToMarkdown(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Alpha", "###### Omega", "- red", "- blue", "1. first", "1. second", "```\nif x < y {\n\treturn x\n}\n```"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown missing %q:\n%s", want, got)
		}
	}
	for _, dropped := range []string{"secret title", "hidden style", "hidden script"} {
		if strings.Contains(got, dropped) {
			t.Errorf("markdown contains dropped content %q:\n%s", dropped, got)
		}
	}
}

func TestWebFetchConvertsLinksAndEntities(t *testing.T) {
	// R-1QRW-EUQP
	server := newWebFetchServer(t, "text/html", `<p>Tom &amp; Jerry&#39;s <a href="https://example.com/a?x=1&amp;y=2">page</a></p>`)
	got, err := callTool(t, WebFetch(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Tom & Jerry's [page](https://example.com/a?x=1&y=2)" {
		t.Fatalf("markdown = %q", got)
	}
}

func TestWebFetchReturnsTextVerbatim(t *testing.T) {
	// R-1RZS-SMHE
	want := "  first <tag> &amp;\nsecond\n"
	server := newWebFetchServer(t, "text/plain; charset=utf-8", want)
	got, err := callTool(t, WebFetch(), map[string]any{"url": server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("WebFetch = %q, want %q", got, want)
	}
}

func TestWebFetchRejectsNonTextContent(t *testing.T) {
	// R-1T7P-6E83
	tests := []struct {
		contentType string
		body        string
	}{
		{"image/png", "\x89PNG\r\n\x1a\nprivate pixels"},
		{"application/pdf", "%PDF-1.7\nprivate document"},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			server := newWebFetchServer(t, tt.contentType, tt.body)
			got, err := callTool(t, WebFetch(), map[string]any{"url": server.URL})
			if err == nil || got != "" {
				t.Fatalf("WebFetch = %q, %v; want empty result and error", got, err)
			}
			if !strings.Contains(err.Error(), tt.contentType) || strings.Contains(err.Error(), "private") {
				t.Fatalf("error = %q, want type and no body content", err)
			}
		})
	}
}

func TestWebFetchEnforcesBodyCapAndFollowsRedirect(t *testing.T) {
	// R-1UFL-K5YS
	oversized := newWebFetchServer(t, "text/plain", strings.Repeat("x", maxWebFetchBodyBytes+1))
	got, err := callTool(t, WebFetch(), map[string]any{"url": oversized.URL})
	if err == nil || got != "" || !strings.Contains(err.Error(), "5 MB cap") {
		t.Fatalf("oversized WebFetch = %q, %v; want empty result and 5 MB cap error", got, err)
	}

	final := newWebFetchServer(t, "text/plain", "redirect destination")
	redirect := httptest.NewServer(http.RedirectHandler(final.URL, http.StatusFound))
	t.Cleanup(redirect.Close)
	got, err = callTool(t, WebFetch(), map[string]any{"url": redirect.URL})
	if err != nil {
		t.Fatal(err)
	}
	if got != "redirect destination" {
		t.Fatalf("redirect result = %q", got)
	}
}

func TestWebFetchTimeoutDefaultRangeAndDeadline(t *testing.T) {
	// R-1VNH-XXPH
	quick := newWebFetchServer(t, "text/plain", "ok")
	if got, err := callTool(t, WebFetch(), map[string]any{"url": quick.URL}); err != nil || got != "ok" {
		t.Fatalf("default timeout call = %q, %v", got, err)
	}
	for _, timeout := range []int{-1, 121} {
		got, err := callTool(t, WebFetch(), map[string]any{"url": quick.URL, "timeout_seconds": timeout})
		if err == nil || got != "" || !strings.Contains(err.Error(), "between 1 and 120") {
			t.Errorf("timeout %d = %q, %v; want range error", timeout, got, err)
		}
	}

	started := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(hanging.Close)
	start := time.Now()
	got, err := callTool(t, WebFetch(), map[string]any{"url": hanging.URL, "timeout_seconds": 1})
	elapsed := time.Since(start)
	if err == nil || got != "" {
		t.Fatalf("hanging call = %q, %v; want timeout error", got, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
		t.Fatalf("timeout error = %q, want deadline exceeded", err)
	}
	if elapsed < 750*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("timeout elapsed %v, want approximately 1 second", elapsed)
	}
	select {
	case <-started:
	default:
		t.Fatal("hanging server never received request")
	}
}

func newWebFetchServer(t *testing.T, contentType, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestWebFetchHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	input, err := json.Marshal(map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := WebFetch().Call(ctx, input)
	if err == nil || got != "" {
		t.Fatalf("cancelled WebFetch = %q, %v; want cancellation error", got, err)
	}
}
