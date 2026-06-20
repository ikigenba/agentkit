package openaicompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestEmbeddingProviderPostsEmbeddingsAndOrdersVectors(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		fmt.Fprint(w, `{"data":[{"index":1,"embedding":[3,4]},{"index":0,"embedding":[1,2]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`)
	}))
	defer server.Close()

	provider := NewEmbeddingProvider(EmbeddingConfig{
		Provider:   "compat",
		BaseURL:    server.URL,
		APIKey:     "key",
		HTTPClient: server.Client(),
		Pricing:    map[string]agentkit.EmbeddingPricing{"model": {InputToken: 1}},
		Specs:      map[string]agentkit.EmbeddingSpec{"model": {NativeDimension: 2, MinDimension: 1, MaxDimension: 2}},
	})
	rt := provider.Embed(context.Background(), &agentkit.EmbedRequest{
		Model:      "model",
		Inputs:     []string{"first", "second"},
		Role:       agentkit.InputDocument,
		Dimensions: 2,
	})

	if err := rt.Err(); err != nil {
		t.Fatalf("Embed() error = %v, want nil", err)
	}
	wantVectors := [][]float32{{1, 2}, {3, 4}}
	if got := rt.Vectors(); !reflect.DeepEqual(got, wantVectors) {
		t.Fatalf("vectors = %#v, want %#v", got, wantVectors)
	}
	if got, want := rt.Usage(), (agentkit.EmbeddingUsage{InputTokens: 7, Total: 7}); got != want {
		t.Fatalf("usage = %#v, want %#v", got, want)
	}
	for _, key := range []string{"role", "task", "input_type"} {
		if _, ok := request[key]; ok {
			t.Fatalf("request carried %q: %#v", key, request)
		}
	}
}
