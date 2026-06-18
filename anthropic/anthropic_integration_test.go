//go:build integration

package anthropic

import (
	"context"
	"os"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestAnthropicIntegrationSkipsWithoutCredential(t *testing.T) {
	// R-WJLM-7QRP
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY is not set")
	}
	conv := &agentkit.Conversation{Provider: New(key), Model: ModelHaiku45}
	stream := conv.Send(context.Background(), "Reply with one short sentence.")
	for range stream.Events() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
}
