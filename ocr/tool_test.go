package ocr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestToolDeclaresOnlyFilePath(t *testing.T) {
	// R-V1UC-V54E
	tool := Tool(t.TempDir(), t.TempDir(), New(APIKey("test")))
	if tool.Name() != "OCR" {
		t.Fatalf("Name() = %q, want OCR", tool.Name())
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(tool.JSONSchema(), &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Properties) != 1 || schema.Properties["file_path"] == nil {
		t.Fatalf("schema properties = %v, want only file_path", schema.Properties)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "file_path" {
		t.Fatalf("schema required = %v, want [file_path]", schema.Required)
	}
}

func TestToolRejectsEscapesBeforeNetwork(t *testing.T) {
	// R-V329-8WV3
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.pdf")
	writePDF(t, outsideFile, "outside")
	if err := os.Symlink(outsideFile, filepath.Join(root, "link.pdf")); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	server := newOCRServer(t, &requests, func(int32) (int, string) {
		return http.StatusOK, responseWithPages("should not be called")
	})
	tool := Tool(root, t.TempDir(), New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	for _, filePath := range []string{filepath.Join("..", filepath.Base(outside), "outside.pdf"), "link.pdf"} {
		if _, err := tool.Call(context.Background(), inputJSON(t, filePath)); err == nil {
			t.Fatalf("Call(%q) succeeded, want confinement error", filePath)
		}
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want 0", got)
	}
}

func TestToolWritesContentAddressedArtifacts(t *testing.T) {
	// R-V4A5-MOLS
	root, cacheDir, source, document := toolPaths(t)
	var requests atomic.Int32
	rawResponse := responseWithPages("page text")
	server := newOCRServer(t, &requests, func(int32) (int, string) {
		return http.StatusOK, rawResponse
	})
	tool := Tool(root, cacheDir, New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	if _, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source))); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(document)
	entry := filepath.Join(cacheDir, "scan-"+hex.EncodeToString(sum[:4]))
	names := directoryNames(t, entry)
	if strings.Join(names, ",") != "response.json,transcript.md" {
		t.Fatalf("cache artifacts = %v, want response.json and transcript.md", names)
	}
	if got := string(readFile(t, filepath.Join(entry, "response.json"))); got != rawResponse {
		t.Fatalf("response.json differs from provider response:\n%s", got)
	}
	wantTranscript, err := Transcript([]byte(rawResponse))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(readFile(t, filepath.Join(entry, "transcript.md"))); got != wantTranscript {
		t.Fatalf("transcript.md = %q, want %q", got, wantTranscript)
	}
}

func TestToolSourceEditCreatesNewCacheEntry(t *testing.T) {
	// R-V6PY-E836
	root, cacheDir, source, _ := toolPaths(t)
	var requests atomic.Int32
	server := newOCRServer(t, &requests, func(call int32) (int, string) {
		return http.StatusOK, responseWithPages("call " + strconv.Itoa(int(call)))
	})
	tool := Tool(root, cacheDir, New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	if _, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source))); err != nil {
		t.Fatal(err)
	}
	writePDF(t, source, "edited bytes")
	if _, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source))); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests = %d, want 2", got)
	}
	if got := directoryNames(t, cacheDir); len(got) != 2 || got[0] == got[1] {
		t.Fatalf("cache entries = %v, want two distinct entries", got)
	}
}

func TestToolUnchangedSourceUsesCache(t *testing.T) {
	// R-V7XU-RZTV
	root, cacheDir, source, _ := toolPaths(t)
	var requests atomic.Int32
	server := newOCRServer(t, &requests, func(int32) (int, string) {
		return http.StatusOK, responseWithPages("cached")
	})
	tool := Tool(root, cacheDir, New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	first, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source)))
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("HTTP requests = %d, want 1", got)
	}
	if transcriptPath(first) != transcriptPath(second) {
		t.Fatalf("transcript paths differ: %q and %q", transcriptPath(first), transcriptPath(second))
	}
}

func TestToolRepairsMissingTranscriptFromRawResponse(t *testing.T) {
	// R-V95R-5RKK
	root, cacheDir, source, _ := toolPaths(t)
	var requests atomic.Int32
	server := newOCRServer(t, &requests, func(int32) (int, string) {
		return http.StatusOK, responseWithPages("repair me")
	})
	tool := Tool(root, cacheDir, New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	first, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source)))
	if err != nil {
		t.Fatal(err)
	}
	path := transcriptPath(first)
	before := readFile(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	second, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source)))
	if err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("HTTP requests = %d, want 1", got)
	}
	if got := readFile(t, path); string(got) != string(before) {
		t.Fatalf("repaired transcript differs:\n%s\nwant:\n%s", got, before)
	}
	if transcriptPath(second) != path {
		t.Fatalf("repaired result path = %q, want %q", transcriptPath(second), path)
	}
}

func TestToolReturnsCompletePreview(t *testing.T) {
	// R-VADN-JJB9
	transcript := "<!-- page 1 -->\nshort transcript"
	result := toolResult("/cache/transcript.md", transcript)
	if !strings.Contains(result, "Status: complete") || !strings.HasSuffix(result, transcript) {
		t.Fatalf("result does not mark and include complete transcript:\n%s", result)
	}
}

func TestToolPreviewStopsAtLastPageBoundary(t *testing.T) {
	// R-VBLJ-XB1Y
	page1 := strings.Repeat("a", 14_900)
	page2 := strings.Repeat("b", 14_900)
	page3 := strings.Repeat("c", 1_000)
	transcript := transcriptForPages(page1, page2, page3)
	preview, status := transcriptPreview(transcript)
	if preview != transcript[:len(preview)] {
		t.Fatal("preview is not a verbatim transcript prefix")
	}
	if len([]rune(preview)) > maxPreviewCharacters {
		t.Fatalf("preview has %d characters, want <= %d", len([]rune(preview)), maxPreviewCharacters)
	}
	if !strings.HasSuffix(preview, "\n\n") || strings.Contains(preview, "<!-- page 3 -->") {
		t.Fatalf("preview did not stop at the page 2 boundary: %q", preview[len(preview)-40:])
	}
	for _, want := range []string{"2 of 3 pages shown", strconv.Itoa(len([]rune(preview))), strconv.Itoa(len([]rune(transcript)))} {
		if !strings.Contains(status, want) {
			t.Fatalf("status = %q, want %q", status, want)
		}
	}
}

func TestToolOversizedFirstPageStopsAtLineBoundary(t *testing.T) {
	// R-VCTG-B2SN
	line := strings.Repeat("x", 20_000)
	transcript := transcriptForPages(line+"\n"+line, "page two")
	preview, status := transcriptPreview(transcript)
	if len([]rune(preview)) > maxPreviewCharacters {
		t.Fatalf("preview has %d characters, want <= %d", len([]rune(preview)), maxPreviewCharacters)
	}
	if !strings.HasSuffix(preview, "\n") || preview != transcript[:len(preview)] {
		t.Fatalf("preview is not a transcript prefix ending at a line boundary")
	}
	if !strings.Contains(status, "page 1 partially shown") {
		t.Fatalf("status = %q, want partial page marker", status)
	}
}

func TestToolFailedExtractionLeavesNoEntryAndRetriesFresh(t *testing.T) {
	// R-VE1C-OUJC
	root, cacheDir, source, _ := toolPaths(t)
	var requests atomic.Int32
	server := newOCRServer(t, &requests, func(call int32) (int, string) {
		if call == 1 {
			return http.StatusBadRequest, `{"error":{"code":400,"message":"bad document"}}`
		}
		return http.StatusOK, responseWithPages("eventual success")
	})
	tool := Tool(root, cacheDir, New(APIKey("test"), WithBaseURL(server.URL), WithHTTPClient(server.Client())))
	if _, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source))); err == nil {
		t.Fatal("first Call succeeded, want provider error")
	}
	if entries, err := os.ReadDir(cacheDir); err == nil && len(entries) != 0 {
		t.Fatalf("failed extraction left cache entries: %v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := tool.Call(context.Background(), inputJSON(t, filepath.Base(source))); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("HTTP requests = %d, want fresh second request (2 total)", got)
	}
	if got := directoryNames(t, cacheDir); len(got) != 1 {
		t.Fatalf("successful retry cache entries = %v, want one", got)
	}
}

func TestToolResultNamesOnlyTranscriptArtifact(t *testing.T) {
	// R-VF99-2MA1
	result := toolResult("/cache/example/transcript.md", "text")
	if !strings.Contains(result, "/cache/example/transcript.md") {
		t.Fatalf("result does not name transcript.md: %q", result)
	}
	if strings.Contains(result, "response.json") {
		t.Fatalf("result names private response artifact: %q", result)
	}
}

func newOCRServer(t *testing.T, requests *atomic.Int32, response func(int32) (int, string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		status, body := response(call)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func responseWithPages(pages ...string) string {
	var content []map[string]string
	content = append(content, map[string]string{"type": "text", "text": `<file name="fixture.pdf">`})
	for _, page := range pages {
		content = append(content, map[string]string{"type": "text", "text": page})
	}
	content = append(content, map[string]string{"type": "text", "text": "</file>"})
	response := map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"annotations": []any{map[string]any{
					"type": "file",
					"file": map[string]any{"content": content},
				}},
			},
		}},
	}
	raw, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func transcriptForPages(pages ...string) string {
	var transcript strings.Builder
	for i, page := range pages {
		if i > 0 {
			transcript.WriteString("\n\n")
		}
		transcript.WriteString("<!-- page ")
		transcript.WriteString(strconv.Itoa(i + 1))
		transcript.WriteString(" -->\n")
		transcript.WriteString(page)
	}
	return transcript.String()
}

func toolPaths(t *testing.T) (root, cacheDir, source string, document []byte) {
	t.Helper()
	root = t.TempDir()
	cacheDir = t.TempDir()
	source = filepath.Join(root, "scan.pdf")
	document = []byte("%PDF-1.4\nfixture bytes")
	if err := os.WriteFile(source, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return root, cacheDir, source, document
}

func writePDF(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"+body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func inputJSON(t *testing.T, filePath string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"file_path": filePath})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func transcriptPath(result string) string {
	line := strings.SplitN(result, "\n", 2)[0]
	return strings.TrimPrefix(line, "Transcript: ")
}

func directoryNames(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}
