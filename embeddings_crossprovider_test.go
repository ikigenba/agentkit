package agentkit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/google"
	"github.com/ikigenba/agentkit/openai"
)

func TestCrossProviderEmbeddingsIdenticalCallingCode(t *testing.T) {
	openAIServer := newCrossOpenAIEmbeddingServer(t)
	defer openAIServer.Close()
	googleServer := newCrossGoogleEmbeddingServer(t)
	defer googleServer.Close()

	inputs := []string{"input-000", "input-001", "input-002"}
	run := func(provider agentkit.EmbeddingProvider, model string) *agentkit.EmbedResult {
		t.Helper()
		result, err := (&agentkit.Embedder{
			Provider:   provider,
			Model:      model,
			Dimensions: 128,
		}).Embed(context.Background(), inputs, agentkit.InputQuery)
		if err != nil {
			t.Fatalf("Embed(%s) error = %v", provider.Name(), err)
		}
		return result
	}

	openAIResult := run(openai.NewEmbedder("openai-key", openai.WithBaseURL(openAIServer.URL), openai.WithHTTPClient(openAIServer.Client())), openai.EmbedModel3Small)
	googleResult := run(google.NewEmbedder("google-key", google.WithBaseURL(googleServer.URL), google.WithHTTPClient(googleServer.Client())), google.EmbedModelGemini001)

	// R-Y5RV-WB3T, R-YHYV-Q0IR
	for name, result := range map[string]*agentkit.EmbedResult{"openai": openAIResult, "google": googleResult} {
		if len(result.Vectors) != len(inputs) {
			t.Fatalf("%s vectors = %d, want %d", name, len(result.Vectors), len(inputs))
		}
		for i, vector := range result.Vectors {
			wantFirst := float64(i+1) / math.Sqrt(float64((i+1)*(i+1)+1))
			if math.Abs(float64(vector[0])-wantFirst) > 1e-6 {
				t.Fatalf("%s vector[%d][0] = %v, want %v", name, i, vector[0], wantFirst)
			}
			if norm := crossL2(vector); math.Abs(norm-1) > 1e-6 {
				t.Fatalf("%s vector[%d] norm = %v, want 1", name, i, norm)
			}
		}
	}
}

func TestCrossProviderEmbeddingSwitchingAndRoles(t *testing.T) {
	openAIServer := newCrossOpenAIEmbeddingServer(t)
	defer openAIServer.Close()
	googleServer := newCrossGoogleEmbeddingServer(t)
	defer googleServer.Close()

	embedder := &agentkit.Embedder{
		Provider:   openai.NewEmbedder("openai-key", openai.WithBaseURL(openAIServer.URL), openai.WithHTTPClient(openAIServer.Client())),
		Model:      openai.EmbedModel3Small,
		Dimensions: 128,
	}
	if _, err := embedder.Embed(context.Background(), []string{"input-000"}, agentkit.InputUnspecified); err != nil {
		t.Fatalf("OpenAI Embed() error = %v", err)
	}

	embedder.Provider = google.NewEmbedder("google-key", google.WithBaseURL(googleServer.URL), google.WithHTTPClient(googleServer.Client()))
	embedder.Model = google.EmbedModelGemini001
	embedder.Dimensions = 256
	if _, err := embedder.Embed(context.Background(), []string{"input-001"}, agentkit.InputDocument); err != nil {
		t.Fatalf("Google Embed() error = %v", err)
	}

	// R-Y6ZS-A2UI
	if openAIServer.calls != 1 || googleServer.calls != 1 {
		t.Fatalf("provider calls = openai:%d google:%d, want 1/1", openAIServer.calls, googleServer.calls)
	}
	if openAIServer.lastModel != openai.EmbedModel3Small || openAIServer.lastDimensions != 128 {
		t.Fatalf("OpenAI request = %s/%d, want configured model/dimensions", openAIServer.lastModel, openAIServer.lastDimensions)
	}
	if googleServer.lastTaskType != "RETRIEVAL_DOCUMENT" || googleServer.lastDimensions != 256 {
		t.Fatalf("Google request task/dimensions = %q/%d, want document/256", googleServer.lastTaskType, googleServer.lastDimensions)
	}

	for _, role := range []agentkit.InputType{agentkit.InputUnspecified, agentkit.InputQuery, agentkit.InputDocument} {
		if _, err := (&agentkit.Embedder{
			Provider:   openai.NewEmbedder("openai-key", openai.WithBaseURL(openAIServer.URL), openai.WithHTTPClient(openAIServer.Client())),
			Model:      openai.EmbedModel3Small,
			Dimensions: 128,
		}).Embed(context.Background(), []string{"input-002"}, role); err != nil {
			t.Fatalf("OpenAI role %v error = %v", role, err)
		}
		if _, err := (&agentkit.Embedder{
			Provider:   google.NewEmbedder("google-key", google.WithBaseURL(googleServer.URL), google.WithHTTPClient(googleServer.Client())),
			Model:      google.EmbedModelGemini001,
			Dimensions: 128,
		}).Embed(context.Background(), []string{"input-002"}, role); err != nil {
			t.Fatalf("Google role %v error = %v", role, err)
		}
	}

	// R-YANH-FE2L
	if got, want := googleServer.taskTypes[len(googleServer.taskTypes)-3:], []string{"", "RETRIEVAL_QUERY", "RETRIEVAL_DOCUMENT"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Google task types = %#v, want %#v", got, want)
	}
	if openAIServer.sawRoleField {
		t.Fatalf("OpenAI embedding request carried a role/task field")
	}
}

type crossEmbeddingServer struct {
	*httptest.Server
	calls          int
	lastModel      string
	lastDimensions int
	lastTaskType   string
	taskTypes      []string
	sawRoleField   bool
}

func newCrossOpenAIEmbeddingServer(t *testing.T) *crossEmbeddingServer {
	t.Helper()
	state := &crossEmbeddingServer{}
	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode OpenAI request: %v", err)
		}
		state.lastModel, _ = body["model"].(string)
		if raw, ok := body["dimensions"].(float64); ok {
			state.lastDimensions = int(raw)
		}
		for _, key := range []string{"role", "task", "input_type"} {
			if _, ok := body[key]; ok {
				state.sawRoleField = true
			}
		}
		inputs := body["input"].([]any)
		data := make([]map[string]any, len(inputs))
		for i, input := range inputs {
			n := crossInputNumber(fmt.Sprint(input))
			data[i] = map[string]any{"index": i, "embedding": []float32{float32(n + 1), 1}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
			"usage": map[string]int64{
				"prompt_tokens": int64(len(inputs)),
				"total_tokens":  int64(len(inputs)),
			},
		})
	}))
	return state
}

func newCrossGoogleEmbeddingServer(t *testing.T) *crossEmbeddingServer {
	t.Helper()
	state := &crossEmbeddingServer{}
	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode Google request: %v", err)
		}
		items := body["requests"].([]any)
		embeddings := make([]map[string]any, len(items))
		for i, raw := range items {
			item := raw.(map[string]any)
			state.lastModel, _ = item["model"].(string)
			state.lastTaskType, _ = item["taskType"].(string)
			state.taskTypes = append(state.taskTypes, state.lastTaskType)
			if rawDimensions, ok := item["outputDimensionality"].(float64); ok {
				state.lastDimensions = int(rawDimensions)
			}
			content := item["content"].(map[string]any)
			parts := content["parts"].([]any)
			input := parts[0].(map[string]any)["text"].(string)
			n := crossInputNumber(input)
			embeddings[i] = map[string]any{"values": []float32{float32(n + 1), 1}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": embeddings,
			"usageMetadata": map[string]int64{
				"promptTokenCount": int64(len(items)),
			},
		})
	}))
	return state
}

func crossInputNumber(input string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(input, "input-"))
	if err != nil {
		return 0
	}
	return n
}

func crossL2(vector []float32) float64 {
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	return math.Sqrt(sum)
}
