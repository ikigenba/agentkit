package openrouterx

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestRoutingProviderOptionsEmitsExactFragmentAndRoundTrips(t *testing.T) {
	// R-DSGZ-2ADS
	routing := Routing{
		Order:    []string{"anthropic", "openai"},
		Only:     []string{"anthropic"},
		Sort:     "price",
		MaxPrice: &MaxPrice{Prompt: 1.25, Completion: 2.5},
		ZDR:      true,
	}
	want := json.RawMessage(`{"provider":{"order":["anthropic","openai"],"only":["anthropic"],"sort":"price","max_price":{"prompt":1.25,"completion":2.5},"zdr":true}}`)
	got := routing.ProviderOptions()
	if !bytes.Equal(got, want) {
		t.Fatalf("ProviderOptions() = %s, want byte-identical %s", got, want)
	}

	req := agentkit.Request{ProviderOptions: got}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(req.ProviderOptions, &merged); err != nil {
		t.Fatalf("ProviderOptions did not round-trip through Request: %v", err)
	}
	if !bytes.Equal(req.ProviderOptions, want) || !bytes.Equal(merged["provider"], wantProviderObject(t, want)) {
		t.Fatalf("Request.ProviderOptions changed fragment: %s", req.ProviderOptions)
	}

	if empty := (Routing{}).ProviderOptions(); !bytes.Equal(empty, json.RawMessage(`{"provider":{}}`)) {
		t.Fatalf("empty Routing emitted fields that were not set: %s", empty)
	}
}

func wantProviderObject(t *testing.T, options json.RawMessage) json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(options, &object); err != nil {
		t.Fatal(err)
	}
	return object["provider"]
}
