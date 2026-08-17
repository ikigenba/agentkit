//go:build integration

package catalog

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/openrouter"
)

func TestOpenRouterOfferingWireNamesAreLive(t *testing.T) {
	// R-4NJ4-SJ41
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	for _, entry := range ListCurated(agentkit.ProviderOpenRouter) {
		entry := entry
		t.Run(entry.Model, func(t *testing.T) {
			roundTrip := openRouterRoundTrip(t, key, entry.WireModel(agentkit.ProviderOpenRouter), agentkit.ReasoningValue{})
			if err := roundTrip.Err(); err != nil {
				t.Fatalf("minimal round trip: %v", err)
			}
		})
	}
}

func TestOpenRouterCanDisableClaimsAreLive(t *testing.T) {
	// R-DOVZ-2LNS
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		t.Skip("OPENROUTER_API_KEY is not set")
	}
	for _, entry := range ListCurated(agentkit.ProviderOpenRouter) {
		offering, ok := Offer(entry.Model, agentkit.ProviderOpenRouter)
		if !ok || offering.Reasoning == nil {
			continue
		}
		entry, spec := entry, offering.Reasoning
		t.Run(entry.Model, func(t *testing.T) {
			roundTrip := openRouterRoundTrip(t, key, entry.WireModel(agentkit.ProviderOpenRouter), agentkit.DisableReasoning())
			err := roundTrip.Err()
			if spec.CanDisable {
				if err != nil {
					t.Fatalf("off-form rejected despite CanDisable: %v", err)
				}
				if text := messageText(roundTrip.Message()); text == "" {
					t.Fatal("off-form completed without assistant text")
				}
				return
			}
			if err == nil {
				t.Fatal("off-form completed despite CanDisable false")
			}
			if !errors.Is(err, agentkit.ErrInvalidRequest) {
				t.Fatalf("off-form error = %v, want provider invalid-request classification", err)
			}
		})
	}
}

func openRouterRoundTrip(t *testing.T, key, model string, reasoning agentkit.ReasoningValue) *agentkit.RoundTrip {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	return openrouter.New(openrouter.APIKey(key)).RoundTrip(ctx, &agentkit.Request{
		Model: model,
		Messages: []agentkit.Message{{
			Role:   agentkit.RoleUser,
			Blocks: []agentkit.Block{agentkit.TextBlock{Text: "Reply with OK."}},
		}},
		Gen: agentkit.GenSettings{MaxTokens: 256, Reasoning: reasoning},
	})
}

func messageText(message agentkit.Message) string {
	var text strings.Builder
	for _, block := range message.Blocks {
		if block, ok := block.(agentkit.TextBlock); ok {
			text.WriteString(block.Text)
		}
	}
	return strings.TrimSpace(text.String())
}
