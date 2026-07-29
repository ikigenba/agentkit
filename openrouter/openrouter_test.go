package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestOpenRouterUsesBakedBaseURLBearerAuthAndFreeFormModel(t *testing.T) {
	// R-DA6H-BQ9D
	// R-CRVZ-L64Y
	var _ agentkit.Provider = New(APIKey("compile-time-credential-check"))
	var _ func(Credential, ...Option) *Provider = New

	const model = "anthropic/claude-opus-4-8:floor"
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got, want := r.URL.String(), defaultBaseURL+"/chat/completions"; got != want {
			t.Errorf("URL = %q, want %q", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want bearer API key", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if got := body["model"]; got != model {
			t.Errorf("model = %#v, want %q", got, model)
		}
		return sseResponse(r, openRouterSSE(0, nil, true)), nil
	})}

	provider := New(APIKey("test-key"), WithHTTPClient(client))
	// R-LK0H-9AXO
	if got, want := provider.Identity(), (agentkit.Identity{Provider: agentkit.ProviderOpenRouter, Auth: agentkit.AuthAPIKey}); got != want {
		t.Fatalf("Identity() = %#v, want %#v", got, want)
	}
	rt := provider.RoundTrip(context.Background(), &agentkit.Request{Model: model})
	if err := rt.Err(); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
}

func TestOpenRouterExtractsReportedCostFromFinalUsageFrame(t *testing.T) {
	// R-DBED-PI02
	upstream := 0.00000075
	tests := []struct {
		name        string
		cost        float64
		upstream    *float64
		includeCost bool
		want        agentkit.Cost
		wantOK      bool
	}{
		{name: "credits cost", cost: 0.00000125, includeCost: true, want: 1250, wantOK: true},
		{name: "BYOK cost includes upstream inference", cost: 0.00000125, upstream: &upstream, includeCost: true, want: 2000, wantOK: true},
		{name: "omitted cost falls through", want: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, openRouterSSE(tt.cost, tt.upstream, tt.includeCost))
			}))
			defer server.Close()

			provider := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := provider.RoundTrip(context.Background(), &agentkit.Request{Model: strings.Join([]string{"z-ai", "glm-5.2:nitro"}, "/")})
			if err := rt.Err(); err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}
			got, ok := rt.ReportedCost()
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("ReportedCost() = %d/%v, want %d/%v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestOpenRouterReasoningLoweringUsesNormalizedReasoningObject(t *testing.T) {
	// R-DCMA-39QR
	// R-DCOZ-8W8U
	// R-DF4S-0FQ8
	tests := []struct {
		name      string
		value     agentkit.ReasoningValue
		want      map[string]any
		wantField bool
	}{
		{name: "level", value: agentkit.Level("xhigh"), want: map[string]any{"effort": "xhigh"}, wantField: true},
		{name: "budget", value: agentkit.Budget(8000), want: map[string]any{"max_tokens": float64(8000)}, wantField: true},
		{name: "enabled", value: agentkit.EnableReasoning(), want: map[string]any{"enabled": true}, wantField: true},
		{name: "disabled", value: agentkit.DisableReasoning(), want: map[string]any{"enabled": false}, wantField: true},
		{name: "unset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, openRouterSSE(0, nil, true))
			}))
			defer server.Close()

			provider := New(APIKey("test-key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			rt := provider.RoundTrip(context.Background(), &agentkit.Request{
				Model: "vendor/day-one-model",
				Gen:   agentkit.GenSettings{Reasoning: tt.value},
			})
			if err := rt.Err(); err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}
			got, present := body["reasoning"].(map[string]any)
			if present != tt.wantField || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("reasoning = %#v/%v, want %#v/%v; body = %#v", got, present, tt.want, tt.wantField, body)
			}
			if _, present := body["thinking"]; present {
				t.Fatalf("OpenRouter request used Z.ai thinking field: %#v", body)
			}
			if _, present := body["reasoning_effort"]; present {
				t.Fatalf("OpenRouter request used Z.ai reasoning_effort field: %#v", body)
			}
		})
	}
}

func TestOpenRouterEnableReasoningExactWireFragment(t *testing.T) {
	// R-DCOZ-8W8U
	// R-DF4S-0FQ8
	got, err := encodeReasoning(agentkit.EnableReasoning())
	if err != nil {
		t.Fatalf("encodeReasoning() error = %v", err)
	}
	if want := `{"reasoning":{"enabled":true}}`; string(got) != want {
		t.Fatalf("encodeReasoning() = %s, want %s", got, want)
	}
	disabled, err := encodeReasoning(agentkit.DisableReasoning())
	if err != nil {
		t.Fatalf("encodeReasoning(disabled) error = %v", err)
	}
	if want := `{"reasoning":{"enabled":false}}`; string(disabled) != want {
		t.Fatalf("encodeReasoning(disabled) = %s, want %s", disabled, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func sseResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func openRouterSSE(cost float64, upstream *float64, includeCost bool) string {
	usage := map[string]any{
		"prompt_tokens":     1,
		"completion_tokens": 1,
		"total_tokens":      2,
	}
	if includeCost {
		usage["cost"] = cost
	}
	if upstream != nil {
		usage["cost_details"] = map[string]any{"upstream_inference_cost": *upstream}
	}
	usageJSON, _ := json.Marshal(usage)
	return strings.Join([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n",
		"data: {\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{}}]}\n\n",
		"data: {\"choices\":[],\"usage\":" + string(usageJSON) + "}\n\n",
		"data: [DONE]\n\n",
	}, "")
}
