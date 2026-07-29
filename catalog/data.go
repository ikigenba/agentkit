package catalog

import "github.com/ikigenba/agentkit"

func pricing(tiers ...agentkit.RateTier) *agentkit.Pricing {
	return &agentkit.Pricing{Tiers: tiers}
}

func fixed(v agentkit.ReasoningValue) ReasoningDefault {
	return ReasoningDefault{Mode: DefaultFixed, Value: v}
}

func enumReasoning(term string, levels []string, defaultLevel string, canDisable bool) *ReasoningSpec {
	return &ReasoningSpec{
		Term: term, Kind: ReasoningEnum, Levels: levels,
		CanDisable: canDisable, Default: fixed(agentkit.Level(defaultLevel)),
	}
}

func toggleReasoning(canEnable, canDisable bool, mode ReasoningDefaultMode) *ReasoningSpec {
	return &ReasoningSpec{
		Term: "thinking", Kind: ReasoningToggle,
		CanEnable: canEnable, CanDisable: canDisable, Default: ReasoningDefault{Mode: mode},
	}
}

func chatEntry(model string, vendor VendorID, provider agentkit.ProviderID, context int64, rates *agentkit.Pricing, reasoning *ReasoningSpec, alternatives ...Offering) Entry {
	offerings := []Offering{{
		Provider: provider, Pricing: rates, Reasoning: reasoning, Context: context,
	}}
	offerings = append(offerings, alternatives...)
	return Entry{Model: model, Vendor: vendor, Offerings: offerings}
}

func routerOffering(context int64) Offering {
	return Offering{Provider: agentkit.ProviderOpenRouter, Context: context}
}

func embeddingEntry(model string, vendor VendorID, provider agentkit.ProviderID, context int64, info EmbeddingInfo) Entry {
	return Entry{
		Model: model, Vendor: vendor,
		Offerings: []Offering{{Provider: provider, Context: context}},
		Embedding: &info,
	}
}

var entries = map[string]Entry{
	"claude-opus-4-8": chatEntry(
		"claude-opus-4-8", VendorAnthropic, agentkit.ProviderAnthropic, 1_000_000,
		pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, CacheWrite5m: 6250, CacheWrite1h: 10000, Output: 25000}),
		enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "high", true),
	),
	"claude-sonnet-4-6": chatEntry(
		"claude-sonnet-4-6", VendorAnthropic, agentkit.ProviderAnthropic, 1_000_000,
		pricing(agentkit.RateTier{InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000}),
		enumReasoning("effort", []string{"low", "medium", "high", "max"}, "high", true),
	),
	"claude-haiku-4-5": chatEntry(
		"claude-haiku-4-5", VendorAnthropic, agentkit.ProviderAnthropic, 200_000,
		pricing(agentkit.RateTier{InputUncached: 1000, CacheReadInput: 100, CacheWrite5m: 1250, CacheWrite1h: 2000, Output: 5000}),
		&ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 1024, Max: 4096, Sentinels: []Sentinel{{Value: 0, Meaning: "off"}}, CanDisable: true, Default: ReasoningDefault{Mode: DefaultOff}},
	),
	"claude-fable-5": chatEntry(
		"claude-fable-5", VendorAnthropic, agentkit.ProviderAnthropic, 1_000_000,
		pricing(agentkit.RateTier{InputUncached: 10000, CacheReadInput: 1000, CacheWrite5m: 12500, CacheWrite1h: 20000, Output: 50000}),
		enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "medium", false),
	),
	"claude-sonnet-5": chatEntry(
		"claude-sonnet-5", VendorAnthropic, agentkit.ProviderAnthropic, 1_000_000,
		pricing(agentkit.RateTier{InputUncached: 3000, CacheReadInput: 300, CacheWrite5m: 3750, CacheWrite1h: 6000, Output: 15000}),
		enumReasoning("effort", []string{"low", "medium", "high", "xhigh", "max"}, "medium", true),
	),
	"gemini-2.5-flash": chatEntry(
		"gemini-2.5-flash", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
		pricing(agentkit.RateTier{InputUncached: 300, CacheReadInput: 30, Output: 2500}),
		&ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 0, Max: 24576, Sentinels: []Sentinel{{Value: 0, Meaning: "off"}, {Value: -1, Meaning: "dynamic"}}, CanDisable: true, Default: ReasoningDefault{Mode: DefaultDynamic}},
	),
	"gemini-2.5-pro": chatEntry(
		"gemini-2.5-pro", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
		pricing(agentkit.RateTier{InputUncached: 1250, CacheReadInput: 125, Output: 10000}, agentkit.RateTier{MinInputTokens: 200001, InputUncached: 2500, CacheReadInput: 250, Output: 15000}),
		&ReasoningSpec{Term: "thinking budget", Kind: ReasoningRange, Min: 128, Max: 32768, Sentinels: []Sentinel{{Value: -1, Meaning: "dynamic"}}, Default: ReasoningDefault{Mode: DefaultDynamic}},
	),
	"gemini-3.5-flash": chatEntry(
		"gemini-3.5-flash", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
		pricing(agentkit.RateTier{InputUncached: 1500, CacheReadInput: 150, Output: 9000}),
		enumReasoning("thinking level", []string{"minimal", "low", "medium", "high"}, "medium", false),
	),
	"gemini-3.1-flash-lite": chatEntry(
		"gemini-3.1-flash-lite", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
		pricing(agentkit.RateTier{InputUncached: 250, CacheReadInput: 25, Output: 1500}),
		enumReasoning("thinking level", []string{"minimal", "low", "medium", "high"}, "medium", false),
	),
	"gemini-3.1-pro-preview": chatEntry(
		"gemini-3.1-pro-preview", VendorGoogle, agentkit.ProviderGoogle, 1_048_576,
		pricing(agentkit.RateTier{InputUncached: 2000, CacheReadInput: 200, Output: 12000}, agentkit.RateTier{MinInputTokens: 200001, InputUncached: 4000, CacheReadInput: 400, Output: 18000}),
		enumReasoning("thinking level", []string{"low", "medium", "high"}, "high", false),
	),
	"gpt-5.5-pro": chatEntry(
		"gpt-5.5-pro", VendorOpenAI, agentkit.ProviderOpenAI, 1_050_000,
		pricing(agentkit.RateTier{InputUncached: 30000, CacheReadInput: 30000, Output: 180000}),
		enumReasoning("effort", []string{"high", "xhigh"}, "high", false),
	),
	"gpt-5.5": chatEntry(
		"gpt-5.5", VendorOpenAI, agentkit.ProviderOpenAI, 1_050_000,
		pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, Output: 30000}, agentkit.RateTier{MinInputTokens: 272001, InputUncached: 10000, CacheReadInput: 1000, Output: 45000}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	),
	"gpt-5.4": chatEntry(
		"gpt-5.4", VendorOpenAI, agentkit.ProviderOpenAI, 1_050_000,
		pricing(agentkit.RateTier{InputUncached: 2500, CacheReadInput: 250, Output: 15000}, agentkit.RateTier{MinInputTokens: 272001, InputUncached: 5000, CacheReadInput: 500, Output: 22500}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	),
	"gpt-5.4-mini": chatEntry(
		"gpt-5.4-mini", VendorOpenAI, agentkit.ProviderOpenAI, 400_000,
		pricing(agentkit.RateTier{InputUncached: 750, CacheReadInput: 75, Output: 4500}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	),
	"gpt-5.4-nano": chatEntry(
		"gpt-5.4-nano", VendorOpenAI, agentkit.ProviderOpenAI, 400_000,
		pricing(agentkit.RateTier{InputUncached: 200, CacheReadInput: 20, Output: 1250}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "none", true),
	),
	"gpt-5.6-sol": chatEntry(
		"gpt-5.6-sol", VendorOpenAI, agentkit.ProviderOpenAI, 1_050_000,
		pricing(agentkit.RateTier{InputUncached: 5000, CacheReadInput: 500, Output: 30000}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	),
	"gpt-5.6-terra": chatEntry(
		"gpt-5.6-terra", VendorOpenAI, agentkit.ProviderOpenAI, 1_050_000,
		pricing(agentkit.RateTier{InputUncached: 2500, CacheReadInput: 250, Output: 15000}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	),
	"gpt-5.6-luna": chatEntry(
		"gpt-5.6-luna", VendorOpenAI, agentkit.ProviderOpenAI, 400_000,
		pricing(agentkit.RateTier{InputUncached: 1000, CacheReadInput: 100, Output: 6000}),
		enumReasoning("effort", []string{"none", "low", "medium", "high", "xhigh"}, "medium", true),
	),
	"grok-4.5": chatEntry(
		"grok-4.5", VendorXAI, agentkit.ProviderOpenRouter, 256_000,
		pricing(agentkit.RateTier{InputUncached: 3000, Output: 15000}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"grok-4.3": chatEntry(
		"grok-4.3", VendorXAI, agentkit.ProviderOpenRouter, 256_000,
		pricing(agentkit.RateTier{InputUncached: 3000, Output: 15000}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"grok-4.20": chatEntry(
		"grok-4.20", VendorXAI, agentkit.ProviderOpenRouter, 2_000_000,
		pricing(agentkit.RateTier{InputUncached: 3000, Output: 15000}, agentkit.RateTier{MinInputTokens: 200001, InputUncached: 6000, Output: 30000}),
		toggleReasoning(true, true, DefaultOff),
	),
	"grok-4.20-multi-agent": chatEntry(
		"grok-4.20-multi-agent", VendorXAI, agentkit.ProviderOpenRouter, 2_000_000,
		pricing(agentkit.RateTier{InputUncached: 6000, Output: 30000}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"deepseek-v4-flash": chatEntry(
		"deepseek-v4-flash", VendorDeepSeek, agentkit.ProviderOpenRouter, 128_000,
		pricing(agentkit.RateTier{InputUncached: 300, CacheReadInput: 30, Output: 1200}),
		toggleReasoning(true, true, DefaultDynamic),
	),
	"deepseek-v4-pro": chatEntry(
		"deepseek-v4-pro", VendorDeepSeek, agentkit.ProviderOpenRouter, 128_000,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 60, Output: 2400}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"kimi-k3": chatEntry(
		"kimi-k3", VendorMoonshot, agentkit.ProviderOpenRouter, 256_000,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 60, Output: 2500}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"kimi-k2.7-code": chatEntry(
		"kimi-k2.7-code", VendorMoonshot, agentkit.ProviderOpenRouter, 256_000,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 60, Output: 2500}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"kimi-k2.6": chatEntry(
		"kimi-k2.6", VendorMoonshot, agentkit.ProviderOpenRouter, 256_000,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 60, Output: 2500}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
	),
	"glm-5.2": chatEntry(
		"glm-5.2", VendorZAI, agentkit.ProviderZAI, 202_752,
		pricing(agentkit.RateTier{InputUncached: 1400, CacheReadInput: 260, Output: 4400}),
		enumReasoning("effort (+ toggle)", []string{"high", "max"}, "max", true),
		routerOffering(202_752),
	),
	"glm-5.1": chatEntry(
		"glm-5.1", VendorZAI, agentkit.ProviderZAI, 202_752,
		pricing(agentkit.RateTier{InputUncached: 1400, CacheReadInput: 260, Output: 4400}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
		routerOffering(202_752),
	),
	"glm-4.7": chatEntry(
		"glm-4.7", VendorZAI, agentkit.ProviderZAI, 202_752,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 110, Output: 2200}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
		routerOffering(202_752),
	),
	"glm-4.6": chatEntry(
		"glm-4.6", VendorZAI, agentkit.ProviderZAI, 202_752,
		pricing(agentkit.RateTier{InputUncached: 600, CacheReadInput: 110, Output: 2200}),
		&ReasoningSpec{Term: "thinking", Kind: ReasoningToggle, CanEnable: true, CanDisable: true, Default: fixed(agentkit.EnableReasoning())},
		routerOffering(202_752),
	),
	"text-embedding-3-small": embeddingEntry(
		"text-embedding-3-small", VendorOpenAI, agentkit.ProviderOpenAI, 8192,
		EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 20}, NativeDimension: 1536, MinDimension: 1, MaxDimension: 1536, MaxInputTokens: 8192},
	),
	"text-embedding-3-large": embeddingEntry(
		"text-embedding-3-large", VendorOpenAI, agentkit.ProviderOpenAI, 8192,
		EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 130}, NativeDimension: 3072, MinDimension: 1, MaxDimension: 3072, MaxInputTokens: 8192},
	),
	"gemini-embedding-001": embeddingEntry(
		"gemini-embedding-001", VendorGoogle, agentkit.ProviderGoogle, 2048,
		EmbeddingInfo{Pricing: agentkit.EmbeddingPricing{InputToken: 150}, NativeDimension: 3072, MinDimension: 128, MaxDimension: 3072, MaxInputTokens: 2048},
	),
}
