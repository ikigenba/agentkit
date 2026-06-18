package agentkit

// Pricing is one model's per-token rates, as one or more context-length tiers.
//
// Rates are nano-USD per token. A turn is rated entirely at the highest tier
// whose MinInputTokens is less than or equal to the turn's total input tokens.
type Pricing struct {
	Tiers []RateTier
}

// RateTier is the per-token rate set for one context band.
type RateTier struct {
	MinInputTokens int64
	InputUncached  int64
	CacheReadInput int64
	CacheWrite5m   int64
	CacheWrite1h   int64
	Output         int64
}

// Cost is an amount in nano-USD.
type Cost int64

// USD converts the cost to dollars for display.
func (c Cost) USD() float64 {
	return float64(c) / 1_000_000_000
}

// Cost computes one turn's nano-USD cost using the tier selected by the turn's
// total input tokens. Reasoning output bills at the output rate.
func (p Pricing) Cost(u Usage) Cost {
	if len(p.Tiers) == 0 {
		return 0
	}

	totalInput := u.InputUncached + u.CacheReadInput + u.CacheWriteInput
	tier := p.Tiers[0]
	for _, candidate := range p.Tiers[1:] {
		if totalInput >= candidate.MinInputTokens {
			tier = candidate
		}
	}

	return Cost(
		u.InputUncached*tier.InputUncached +
			u.CacheReadInput*tier.CacheReadInput +
			u.CacheWrite5m*tier.CacheWrite5m +
			u.CacheWrite1h*tier.CacheWrite1h +
			(u.Output+u.ReasoningOutput)*tier.Output,
	)
}
