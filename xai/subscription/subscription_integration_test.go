//go:build integration

package subscription

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ikigenba/agentkit"
	"github.com/ikigenba/agentkit/xai"
)

func TestSubscriptionIntegrationRoundTrip(t *testing.T) {
	// R-DYRT-T04Z
	path := os.Getenv("XAI_SUBSCRIPTION_AUTH_FILE")
	if path == "" {
		t.Skip("XAI_SUBSCRIPTION_AUTH_FILE does not name a raw token-response credential file")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("raw token-response credential file is not present")
		}
		t.Fatalf("stat auth file: %v", err)
	}
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	stream := (&agentkit.Conversation{
		Provider: xai.New(xai.Subscription(store)),
		Model:    "grok-4.20",
		System:   "Answer concisely.",
	}).Send(ctx, "Reply with one short sentence.")
	var text strings.Builder
	for event := range stream.Events() {
		if done, ok := event.(agentkit.MessageDone); ok {
			for _, block := range done.Message.Blocks {
				if block, ok := block.(agentkit.TextBlock); ok {
					text.WriteString(block.Text)
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Fatal("completed stream has no assistant text")
	}
}
