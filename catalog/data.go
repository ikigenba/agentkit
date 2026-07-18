package catalog

import "github.com/ikigenba/agentkit"

func pricing(tiers ...agentkit.RateTier) *agentkit.Pricing {
	return &agentkit.Pricing{Tiers: tiers}
}

func enumReasoning(term string, levels []string, defaultLevel string, canDisable bool) *ReasoningSpec {
	return &ReasoningSpec{Term: term, Kind: ReasoningEnum, Levels: levels, Default: agentkit.Level(defaultLevel), CanDisable: canDisable}
}

var entries = map[string]Entry{
	"claude-opus-4-8": {
		Model: "claude-opus-4-8", Provider: "anthropic", Context: 1_000_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, CacheWrite5m: 6250, CacheWrite1h: 10000, Output: 25000}),
		Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "high", true),
	},
	"claude-sonnet-4-6": {
		Model: "claude-sonnet-4-6", Provider: "anthropic", Context: 1_000_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000}),
		Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "max"}, "high", true),
	},
	"claude-haiku-4-5": {
		Model: "claude-haiku-4-5", Provider: "anthropic", Context: 200_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1000, CacheReadInput: 100, CacheWrite5m: 1250, CacheWrite1h: 2000, Output: 5000}),
		Reasoning: &ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 1024, Max: 4096, Default: agentkit.DisableReasoning(), CanDisable: true},
	},
	"claude-fable-5": {
		Model: "claude-fable-5", Provider: "anthropic", Context: 1_000_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 10000, CacheReadInput: 1000, CacheWrite5m: 12500, CacheWrite1h: 20000, Output: 50000}),
		Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "medium", false),
	},
	"claude-sonnet-5": {
		Model: "claude-sonnet-5", Provider: "anthropic", Context: 1_000_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000}),
		Reasoning: enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "medium", true),
	},
	"gemini-2.5-flash": {
		Model: "gemini-2.5-flash", Provider: "google", Context: 1_048_576,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 300, CacheReadInput: 30, Output: 2500}),
		Reasoning: &ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 0, Max: 24576, Sentinels: []Sentinel{{Value: 0, Meaning: "off"}, {Value: -1, Meaning: "dynamic"}}, Default: agentkit.Budget(-1), CanDisable: true},
	},
	"gemini-2.5-pro": {
		Model: "gemini-2.5-pro", Provider: "google", Context: 1_048_576,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1250, CacheReadInput: 125, Output: 10000}, agentkit.RateTier{MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 250, Output: 15000}),
		Reasoning: &ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 128, Max: 32768, Sentinels: []Sentinel{{Value: -1, Meaning: "dynamic"}}, Default: agentkit.Budget(-1)},
	},
	"gemini-3.5-flash": {
		Model: "gemini-3.5-flash", Provider: "google", Context: 1_048_576,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1500, CacheReadInput: 150, Output: 9000}),
		Reasoning: enumReasoning("thinking level", []string{"minimal", "low", "medium", "high"}, "medium", false),
	},
	"gemini-3.1-flash-lite": {
		Model: "gemini-3.1-flash-lite", Provider: "google", Context: 1_048_576,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 250, CacheReadInput: 25, Output: 1500}),
		Reasoning: enumReasoning("thinking level", []string{"minimal", "low", "medium", "high"}, "medium", false),
	},
	"gemini-3.1-pro-preview": {
		Model: "gemini-3.1-pro-preview", Provider: "google", Context: 1_048_576,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 2000, CacheReadInput: 200, Output: 12000}, agentkit.RateTier{MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 400, Output: 18000}),
		Reasoning: enumReasoning("thinking level", []string{"low", "medium", "high"}, "high", false),
	},
	"gpt-5.5-pro": {
		Model: "gpt-5.5-pro", Provider: "openai", Context: 1_050_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 30000, CacheReadInput: 30000, Output: 180000}),
		Reasoning: enumReasoning("effort", []string{"high", "xhigh"}, "high", false),
	},
	"gpt-5.5": {
		Model: "gpt-5.5", Provider: "openai", Context: 1_050_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, Output: 30000}, agentkit.RateTier{MinInputTokens: 272001, InputUncached: 10000, CacheReadInput: 1000, Output: 45000}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	},
	"gpt-5.4": {
		Model: "gpt-5.4", Provider: "openai", Context: 1_050_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 2500, CacheReadInput: 250, Output: 15000}, agentkit.RateTier{MinInputTokens: 272001, InputUncached: 5000, CacheReadInput: 500, Output: 22500}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	},
	"gpt-5.4-mini": {
		Model: "gpt-5.4-mini", Provider: "openai", Context: 400_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 750, CacheReadInput: 75, Output: 4500}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	},
	"gpt-5.4-nano": {
		Model: "gpt-5.4-nano", Provider: "openai", Context: 400_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 200, CacheReadInput: 20, Output: 1250}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	},
	"gpt-5.6-sol": {
		Model: "gpt-5.6-sol", Provider: "openai", Context: 1_050_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, Output: 30000}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	},
	"gpt-5.6-terra": {
		Model: "gpt-5.6-terra", Provider: "openai", Context: 1_050_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 2500, CacheReadInput: 250, Output: 15000}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	},
	"gpt-5.6-luna": {
		Model: "gpt-5.6-luna", Provider: "openai", Context: 400_000,
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1000, CacheReadInput: 100, Output: 6000}),
		Reasoning: enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	},
	"glm-5.2": {
		Model: "glm-5.2", Provider: "zai", Context: 202_752,
		Routes:    map[string]string{"zai": "glm-5.2", "openrouter": "z-ai/glm-5.2"},
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1400, CacheReadInput: 260, Output: 4400}),
		Reasoning: enumReasoning("effort (+ toggle)", []string{"high", "max"}, "max", true),
	},
	"glm-5.1": {
		Model: "glm-5.1", Provider: "zai", Context: 202_752,
		Routes:    map[string]string{"zai": "glm-5.1", "openrouter": "z-ai/glm-5.1"},
		Pricing:   pricing(agentkit.RateTier{InputUncached: 1400, CacheReadInput: 260, Output: 4400}),
		Reasoning: enumReasoning("effort (+ toggle)", []string{"high", "max"}, "max", true),
	},
	"glm-4.7": {
		Model: "glm-4.7", Provider: "zai", Context: 202_752,
		Routes:    map[string]string{"zai": "glm-4.7", "openrouter": "z-ai/glm-4.7"},
		Pricing:   pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 110, Output: 2200}),
		Reasoning: &ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanDisable: true},
	},
	"glm-4.6": {
		Model: "glm-4.6", Provider: "zai", Context: 202_752,
		Routes:    map[string]string{"zai": "glm-4.6", "openrouter": "z-ai/glm-4.6"},
		Pricing:   pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 110, Output: 2200}),
		Reasoning: &ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanDisable: true},
	},
	"text-embedding-3-small": {
		Model: "text-embedding-3-small", Provider: "openai", Context: 8192,
		Embedding: &EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 20}, NativeDimension: 1536, MinDimension: 1, MaxDimension: 1536, MaxInputTokens: 8192},
	},
	"text-embedding-3-large": {
		Model: "text-embedding-3-large", Provider: "openai", Context: 8192,
		Embedding: &EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 130}, NativeDimension: 3072, MinDimension: 1, MaxDimension: 3072, MaxInputTokens: 8192},
	},
	"gemini-embedding-001": {
		Model: "gemini-embedding-001", Provider: "google", Context: 2048,
		Embedding: &EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 150}, NativeDimension: 3072, MinDimension: 128, MaxDimension: 3072, MaxInputTokens: 2048},
	},
}
