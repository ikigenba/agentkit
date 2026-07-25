package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenRouterRequestUsesFileParserFloor(t *testing.T) {
	// R-UQV9-F7G5
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		captured, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	pdf := []byte("%PDF-1.4\nfixture")
	client := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := client.Do(context.Background(), "statement.pdf", pdf); err != nil {
		t.Fatal(err)
	}

	var request map[string]any
	if err := json.Unmarshal(captured, &request); err != nil {
		t.Fatal(err)
	}
	if got := request["max_tokens"]; got != float64(1) {
		t.Fatalf("max_tokens = %#v, want 1", got)
	}
	messages := request["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content has %d parts, want one file part and no text part", len(content))
	}
	filePart := content[0].(map[string]any)
	if filePart["type"] != "file" {
		t.Fatalf("content type = %#v, want file", filePart["type"])
	}
	file := filePart["file"].(map[string]any)
	prefix := "data:application/pdf;base64,"
	fileData, ok := file["file_data"].(string)
	if !ok || !strings.HasPrefix(fileData, prefix) {
		t.Fatalf("file_data = %#v, want PDF data URI", file["file_data"])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(fileData, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(pdf) {
		t.Fatalf("decoded document differs from input: %q", decoded)
	}
	plugins := request["plugins"].([]any)
	plugin := plugins[0].(map[string]any)
	if plugin["id"] != "file-parser" ||
		plugin["pdf"].(map[string]any)["engine"] != "mistral-ocr" {
		t.Fatalf("plugins = %#v, want mistral-ocr file-parser", plugins)
	}
}

func TestOpenRouterRequestModelDefaultAndOverride(t *testing.T) {
	// R-US35-SZ6U
	var models []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		models = append(models, request.Model)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	pdf := []byte("%PDF-1.4\nfixture")
	defaultClient := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if _, err := defaultClient.Do(context.Background(), "default.pdf", pdf); err != nil {
		t.Fatal(err)
	}
	overrideClient := New(
		APIKey("test-key"),
		WithModel("vendor/x"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	if _, err := overrideClient.Do(context.Background(), "override.pdf", pdf); err != nil {
		t.Fatal(err)
	}
	want := []string{defaultModel, "vendor/x"}
	if len(models) != len(want) || models[0] != want[0] || models[1] != want[1] {
		t.Fatalf("models = %q, want %q", models, want)
	}
}

func TestClientHasNoExportedFields(t *testing.T) {
	// R-GMLN-XC8W
	clientType := reflect.TypeOf(Client{})
	exported := 0
	for i := 0; i < clientType.NumField(); i++ {
		if clientType.Field(i).IsExported() {
			exported++
		}
	}
	if exported != 0 {
		t.Fatalf("Client has %d exported fields, want zero", exported)
	}
}
