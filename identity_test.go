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
	}
	for _, tt := range tests {
		if got := tt.identity.String(); got != tt.want {
			t.Errorf("Identity(%#v).String() = %q, want %q", tt.identity, got, tt.want)
		}
	}
}
