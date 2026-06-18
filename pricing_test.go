package agentkit_test

import (
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestPricingCostRatesEveryUsageBucketWithIntegerMath(t *testing.T) {
	// R-PTEW-5KVY
	pricing := agentkit.Pricing{
		Tiers: []agentkit.RateTier{{
			MinInputTokens: 0,
			InputUncached:  3,
			CacheReadInput: 5,
			CacheWrite5m:   7,
			CacheWrite1h:   11,
			Output:         13,
		}},
	}
	usage := agentkit.Usage{
		InputUncached:   17,
		CacheReadInput:  19,
		CacheWriteInput: 52,
		CacheWrite5m:    23,
		CacheWrite1h:    29,
		Output:          31,
		ReasoningOutput: 37,
	}

	got := pricing.Cost(usage)
	want := agentkit.Cost(17*3 + 19*5 + 23*7 + 29*11 + (31+37)*13)
	if got != want {
		t.Fatalf("pricing.Cost() = %d, want %d", got, want)
	}
}

func TestPricingCostSelectsTierByTotalInputTokens(t *testing.T) {
	// R-V2SM-WC8V
	pricing := agentkit.Pricing{
		Tiers: []agentkit.RateTier{
			{
				MinInputTokens: 0,
				InputUncached:  1,
				CacheReadInput: 10,
				Output:         100,
			},
			{
				MinInputTokens: 101,
				InputUncached:  2,
				CacheReadInput: 20,
				Output:         200,
			},
		},
	}

	atThreshold := pricing.Cost(agentkit.Usage{
		InputUncached:   60,
		CacheReadInput:  20,
		CacheWriteInput: 20,
		Output:          1,
	})
	if want := agentkit.Cost(60*1 + 20*10 + 1*100); atThreshold != want {
		t.Fatalf("cost at threshold = %d, want base-tier cost %d", atThreshold, want)
	}

	aboveThreshold := pricing.Cost(agentkit.Usage{
		InputUncached:   61,
		CacheReadInput:  20,
		CacheWriteInput: 20,
		Output:          1,
	})
	if want := agentkit.Cost(61*2 + 20*20 + 1*200); aboveThreshold != want {
		t.Fatalf("cost above threshold = %d, want high-tier cost %d", aboveThreshold, want)
	}
}

func TestCostUSDConvertsNanoUSDToDollars(t *testing.T) {
	// R-PX2L-AW41
	tests := []struct {
		name string
		cost agentkit.Cost
		want float64
	}{
		{name: "zero", cost: 0, want: 0},
		{name: "one dollar", cost: 1_000_000_000, want: 1},
		{name: "fractional dollar", cost: 1_250_000_000, want: 1.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cost.USD(); got != tt.want {
				t.Fatalf("Cost(%d).USD() = %v, want %v", tt.cost, got, tt.want)
			}
		})
	}
}
