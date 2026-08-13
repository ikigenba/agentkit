package agentkit_test

import (
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestIdentityStringRendersProviderAndAuth(t *testing.T) {
	// R-LK0H-9AXO
	tests := []struct {
		identity agentkit.Identity
		want     string
	}{
		{
			identity: agentkit.Identity{Provider: agentkit.ProviderOpenAI, Auth: agentkit.AuthSubscription},
			want:     "openai.subscription",
		},
		{
			identity: agentkit.Identity{Provider: agentkit.ProviderZAI, Auth: agentkit.AuthAPIKey},
			want:     "z-ai.apikey",
		},
		{
			identity: agentkit.Identity{Provider: agentkit.ProviderXAI, Auth: agentkit.AuthAPIKey},
			want:     "x-ai.apikey",
		},
		{
			identity: agentkit.Identity{Provider: agentkit.ProviderXAI, Auth: agentkit.AuthSubscription},
			want:     "x-ai.subscription",
		},
	}
	for _, tt := range tests {
		if got := tt.identity.String(); got != tt.want {
			t.Errorf("Identity(%#v).String() = %q, want %q", tt.identity, got, tt.want)
		}
	}
}

func TestXAIProviderIDUsesVendorNamespaceSpelling(t *testing.T) {
	// R-LXFD-GS3B
	if got, want := agentkit.ProviderXAI, agentkit.ProviderID("x-ai"); got != want {
		t.Fatalf("ProviderXAI = %q, want %q", got, want)
	}
}
