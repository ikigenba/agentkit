package catalog

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestLookupReturnsAdvisoryChatEmbeddingAndUnknownMetadata(t *testing.T) {
	// R-DMDH-5FOB
	chat, ok := Lookup("claude-opus-4-8")
	if !ok || chat.Provider != "anthropic" || chat.Pricing == nil || chat.Reasoning == nil || chat.Context == 0 {
		t.Fatalf("Lookup(chat) = %#v/%v, want complete Anthropic metadata", chat, ok)
	}
	if !chat.Reasoning.Accepts(chat.Reasoning.Default) {
		t.Fatalf("chat reasoning default is not accepted by its own spec: %#v", chat.Reasoning)
	}

	embedding, ok := Lookup("text-embedding-3-small")
	wantEmbedding := &EmbeddingInfo{
		Pricing:         agentkit.EmbeddingPricing{InputToken: 20},
		NativeDimension: 1536,
		MinDimension:    1,
		MaxDimension:    1536,
		MaxInputTokens:  8192,
	}
	if !ok || !reflect.DeepEqual(embedding.Embedding, wantEmbedding) || embedding.Pricing != nil {
		t.Fatalf("Lookup(embedding) = %#v/%v, want %#v", embedding, ok, wantEmbedding)
	}

	if unknown, ok := Lookup("future-model-not-yet-cataloged"); ok || !reflect.DeepEqual(unknown, Entry{}) {
		t.Fatalf("Lookup(unknown) = %#v/%v, want zero/false", unknown, ok)
	}

	accepted, spec, ok := Check("claude-opus-4-8", agentkit.Level("high"))
	if !ok || !accepted || spec == nil {
		t.Fatalf("Check(cataloged accepted value) = %v/%#v/%v", accepted, spec, ok)
	}
	accepted, spec, ok = Check("future-model-not-yet-cataloged", agentkit.Level("future"))
	if ok || accepted || spec != nil {
		t.Fatalf("Check(unknown) = %v/%#v/%v, want false/nil/false", accepted, spec, ok)
	}
}

func TestResolveUsesDefaultAndNamedRoutesAndPassesUnknownThrough(t *testing.T) {
	// R-DNLD-J7F0
	tests := []struct {
		name         string
		provider     string
		model        string
		wantProvider string
		wantModel    string
		wantOK       bool
	}{
		{name: "default direct route", model: "claude-opus-4-8", wantProvider: "anthropic", wantModel: "claude-opus-4-8", wantOK: true},
		{name: "default listed route", model: "glm-5.2", wantProvider: "zai", wantModel: "glm-5.2", wantOK: true},
		{name: "named aggregator route", provider: "openrouter", model: "glm-5.2", wantProvider: "openrouter", wantModel: "z-ai/glm-5.2", wantOK: true},
		{name: "unknown model", provider: "openrouter", model: "vendor/new-model", wantProvider: "openrouter", wantModel: "vendor/new-model"},
		{name: "unrouted provider", provider: "google", model: "glm-5.2", wantProvider: "google", wantModel: "glm-5.2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, model, _, ok := Resolve(test.provider, test.model)
			if provider != test.wantProvider || model != test.wantModel || ok != test.wantOK {
				t.Fatalf("Resolve(%q, %q) = %q/%q/%v, want %q/%q/%v", test.provider, test.model, provider, model, ok, test.wantProvider, test.wantModel, test.wantOK)
			}
		})
	}
}

func TestListByProviderIncludesDefaultAndRoutedEntriesOnly(t *testing.T) {
	// R-DOT9-WZ5P
	openRouter := ListByProvider("openrouter")
	want := []string{"deepseek-v4-flash", "deepseek-v4-pro", "glm-4.6", "glm-4.7", "glm-5.1", "glm-5.2", "grok-4.20", "grok-4.20-multi-agent", "grok-4.3", "grok-4.5", "kimi-k2.6", "kimi-k2.7-code", "kimi-k3"}
	got := make([]string, len(openRouter))
	for i, entry := range openRouter {
		got[i] = entry.Model
		if _, routed := entry.Routes["openrouter"]; !routed {
			t.Fatalf("ListByProvider(openrouter) included unrouted entry %#v", entry)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListByProvider(openrouter) = %v, want %v", got, want)
	}

	openAI := ListByProvider("openai")
	if len(openAI) == 0 {
		t.Fatal("ListByProvider(openai) returned no default-route entries")
	}
	for _, entry := range openAI {
		if entry.Provider != "openai" {
			t.Fatalf("ListByProvider(openai) included unrelated entry %#v", entry)
		}
	}
	if got := ListByProvider("provider-with-no-catalog-rows"); len(got) != 0 {
		t.Fatalf("ListByProvider(empty) = %#v, want empty", got)
	}
}

func TestAggregatorDefaultEntriesResolveToVendorNamespacedRoutes(t *testing.T) {
	// R-4MB8-ERDC
	listed := make(map[string]Entry)
	for _, entry := range ListByProvider("openrouter") {
		listed[entry.Model] = entry
	}

	for model, entry := range entries {
		if entry.Provider != "openrouter" {
			continue
		}
		slug := entry.Routes["openrouter"]
		if slug == "" || !strings.Contains(slug, "/") {
			t.Errorf("aggregator-default entry %q has invalid OpenRouter route %q", model, slug)
		}
		provider, wireModel, _, ok := Resolve("", model)
		if !ok || provider != "openrouter" || wireModel != slug || wireModel == model {
			t.Errorf("Resolve(\"\", %q) = %q/%q/%v, want openrouter/%q/true", model, provider, wireModel, ok, slug)
		}
		if _, ok := listed[model]; !ok {
			t.Errorf("ListByProvider(openrouter) omitted aggregator-default entry %q", model)
		}
	}
}

func TestLookupReturnsIndependentMetadata(t *testing.T) {
	first, _ := Lookup("glm-5.2")
	first.Routes["openrouter"] = "mutated"
	first.Pricing.Tiers[0].Output = -1
	first.Reasoning.Levels[0] = "mutated"

	second, _ := Lookup("glm-5.2")
	if second.Routes["openrouter"] != "z-ai/glm-5.2" || second.Pricing.Tiers[0].Output != 4400 || second.Reasoning.Levels[0] != "high" {
		t.Fatalf("Lookup exposed mutable catalog storage: %#v", second)
	}
}
