package xai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ikigenba/agentkit"
)

type staticTokenSource struct{ bearer string }

func (s staticTokenSource) Token(context.Context) (string, error) { return s.bearer, nil }

func TestCredentialsUseOnePublicResponsesTransport(t *testing.T) {
	// R-DGHC-2G0K
	// R-DHP8-G7R9
	tests := []struct {
		name string
		cred Credential
		want string
	}{
		{name: "API key", cred: APIKey("platform-key"), want: "platform-key"},
		{name: "subscription", cred: Subscription(staticTokenSource{"living-token"}), want: "living-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer "+tt.want {
					t.Errorf("request path/auth = %q/%q", r.URL.Path, r.Header.Get("Authorization"))
				}
				for _, name := range []string{"chatgpt-account-id", "originator", "OpenAI-Beta"} {
					if got := r.Header.Get(name); got != "" {
						t.Errorf("unexpected %s header = %q", name, got)
					}
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["store"] != false || body["instructions"] != "follow system" || fmt.Sprint(body["include"]) != "[reasoning.encrypted_content]" {
					t.Errorf("fixed Responses body = %#v", body)
				}
				writeSSE(w, textSSE("ok", `"input_tokens":1,"output_tokens":1,"total_tokens":2`))
			}))
			defer server.Close()
			stream := (&agentkit.Conversation{Provider: New(tt.cred, WithBaseURL(server.URL), WithHTTPClient(server.Client())), Model: "grok-test", System: "follow system", Pricing: &agentkit.Pricing{}}).Send(context.Background(), "hello")
			drain(stream)
			if err := stream.Err(); err != nil {
				t.Fatalf("Send: %v", err)
			}
		})
	}
}

func TestCredentialIdentityAndProviderErrorAttribution(t *testing.T) {
	// R-DIX4-TZHY
	tests := []struct {
		name string
		cred Credential
		auth agentkit.AuthMode
		text string
	}{
		{name: "API key", cred: APIKey("key"), auth: agentkit.AuthAPIKey, text: "x-ai.apikey"},
		{name: "subscription", cred: Subscription(staticTokenSource{"token"}), auth: agentkit.AuthSubscription, text: "x-ai.subscription"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, `{"error":{"code":"invalid-argument","message":"Model not found: missing"}}`)
			}))
			defer server.Close()
			p := New(tt.cred, WithBaseURL(server.URL), WithHTTPClient(server.Client()))
			want := agentkit.Identity{Provider: agentkit.ProviderXAI, Auth: tt.auth}
			if got := p.Identity(); got != want || got.String() != tt.text {
				t.Fatalf("Identity() = %#v/%q, want %#v/%q", got, got.String(), want, tt.text)
			}
			var providerErr *agentkit.Error
			if err := p.RoundTrip(context.Background(), &agentkit.Request{Model: "missing"}).Err(); !errors.As(err, &providerErr) || providerErr.Provider != agentkit.ProviderXAI || providerErr.Auth != tt.auth {
				t.Fatalf("provider error = %#v", err)
			}
		})
	}
}

func TestReportedCostOverridesPricingAndMissingCostFallsBack(t *testing.T) {
	// R-DK51-7R8N
	t.Run("reported", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeSSE(w, textSSE("ok", `"input_tokens":1,"output_tokens":1,"total_tokens":2,"cost_in_usd_ticks":6752000`))
		}))
		defer server.Close()
		p := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
		rt := p.RoundTrip(context.Background(), &agentkit.Request{Model: "grok-test"})
		if got, ok := rt.ReportedCost(); !ok || got != 675200 {
			t.Fatalf("ReportedCost() = %d/%v, want 675200/true", got, ok)
		}
		stream := (&agentkit.Conversation{Provider: p, Model: "grok-test", Pricing: &agentkit.Pricing{Tiers: []agentkit.RateTier{{InputUncached: 99, Output: 99}}}}).Send(context.Background(), "hello")
		drain(stream)
		if stream.Err() != nil || stream.Cost() != 675200 {
			t.Fatalf("stream error/cost = %v/%d", stream.Err(), stream.Cost())
		}
	})
	t.Run("omitted", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeSSE(w, textSSE("ok", `"input_tokens":1,"output_tokens":1,"total_tokens":2`))
		}))
		defer server.Close()
		p := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client()))
		if _, ok := p.RoundTrip(context.Background(), &agentkit.Request{Model: "grok-test"}).ReportedCost(); ok {
			t.Fatal("omitted cost reported as present")
		}
		priced := (&agentkit.Conversation{Provider: p, Model: "grok-test", Pricing: &agentkit.Pricing{Tiers: []agentkit.RateTier{{InputUncached: 3, Output: 5}}}}).Send(context.Background(), "hello")
		drain(priced)
		if priced.Cost() != 8 {
			t.Fatalf("supplied pricing cost = %d, want 8", priced.Cost())
		}
		unpriced := (&agentkit.Conversation{Provider: p, Model: "grok-test"}).Send(context.Background(), "hello")
		drain(unpriced)
		if unpriced.Cost() != 0 || len(unpriced.Warnings()) != 1 || unpriced.Warnings()[0].Code != agentkit.WarnCostUnknown {
			t.Fatalf("unpriced cost/warnings = %d/%#v", unpriced.Cost(), unpriced.Warnings())
		}
	})
}

func TestXAIRecordedErrorsClassify(t *testing.T) {
	// R-DLCX-LIZC
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"no credentials", 401, `{"error":{"type":"unauthenticated:no-credentials","message":"No credentials"}}`, agentkit.ErrAuthentication},
		{"incorrect key", 400, `{"error":{"code":"invalid-argument","message":"Incorrect API key provided. Please check your API key."}}`, agentkit.ErrAuthentication},
		{"model not found", 400, `{"error":{"code":"invalid-argument","message":"Model not found: grok-missing"}}`, agentkit.ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status); fmt.Fprint(w, tt.body) }))
			defer server.Close()
			err := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "test"}).Err()
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReasoningEffortAndUnsetEncoding(t *testing.T) {
	// R-DQ8J-4LY4
	tests := []struct {
		name      string
		reasoning agentkit.ReasoningValue
		want      bool
	}{
		{name: "low", reasoning: agentkit.Level("low"), want: true},
		{name: "bare enabled", reasoning: agentkit.EnableReasoning()},
		{name: "unset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&body)
				writeSSE(w, textSSE("ok", `"input_tokens":1,"output_tokens":1,"total_tokens":2`))
			}))
			defer server.Close()
			rt := New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())).RoundTrip(context.Background(), &agentkit.Request{Model: "grok-test", Gen: agentkit.GenSettings{Reasoning: tt.reasoning}})
			if rt.Err() != nil {
				t.Fatalf("RoundTrip: %v", rt.Err())
			}
			reasoning, exists := body["reasoning"].(map[string]any)
			if exists != tt.want || (tt.want && reasoning["effort"] != "low") {
				t.Fatalf("reasoning = %#v, exists=%v", reasoning, exists)
			}
		})
	}
}

type weatherInput struct {
	City string `json:"city"`
}

func TestFunctionCallPreservesCallIDInFollowUp(t *testing.T) {
	// R-DSOB-W5FI
	var mu sync.Mutex
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			writeSSE(w, toolSSE())
		} else {
			writeSSE(w, textSSE("done", `"input_tokens":1,"output_tokens":1,"total_tokens":2`))
		}
	}))
	defer server.Close()
	tool := agentkit.NewTool("weather", "weather", func(_ context.Context, in weatherInput) (string, error) {
		if in.City != "Paris" {
			t.Fatalf("input = %#v", in)
		}
		return "sunny", nil
	})
	stream := (&agentkit.Conversation{Provider: New(APIKey("key"), WithBaseURL(server.URL), WithHTTPClient(server.Client())), Model: "grok-test", Tools: []agentkit.Tool{tool}, Pricing: &agentkit.Pricing{}}).Send(context.Background(), "weather?")
	drain(stream)
	if stream.Err() != nil {
		t.Fatalf("Send: %v", stream.Err())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("requests = %d", len(bodies))
	}
	input := bodies[1]["input"].([]any)
	var call, output map[string]any
	for _, raw := range input {
		item := raw.(map[string]any)
		if item["type"] == "function_call" {
			call = item
		}
		if item["type"] == "function_call_output" {
			output = item
		}
	}
	if call["call_id"] != "call_xai_123" || call["arguments"] != `{"city":"Paris"}` || output["call_id"] != "call_xai_123" {
		t.Fatalf("follow-up call/output = %#v/%#v", call, output)
	}
}

func TestMissingCredentialsFailAtSendWithoutTransport(t *testing.T) {
	// R-E3NF-C33R
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	tests := []struct {
		name string
		cred Credential
		want string
	}{
		{"API key", APIKey(""), "xai: API key is absent"},
		{"subscription", Subscription(nil), "xai: SuperGrok subscription token source is absent"},
	}
	var messages []string
	for _, tt := range tests {
		p := New(tt.cred, WithBaseURL(server.URL), WithHTTPClient(server.Client()))
		if p == nil {
			t.Fatal("New returned nil")
		}
		stream := (&agentkit.Conversation{Provider: p, Model: "grok-test"}).Send(context.Background(), "hello")
		drain(stream)
		if !errors.Is(stream.Err(), agentkit.ErrMissingCredential) || !strings.Contains(stream.Err().Error(), tt.want) {
			t.Fatalf("%s error = %v", tt.name, stream.Err())
		}
		messages = append(messages, stream.Err().Error())
	}
	if requests != 0 || messages[0] == messages[1] {
		t.Fatalf("requests/messages = %d/%#v", requests, messages)
	}
}

func drain(stream *agentkit.Stream) {
	for range stream.Events() {
	}
}

func writeSSE(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprint(w, body)
}

func textSSE(text, usage string) string {
	return sse(`{"type":"response.output_text.delta","delta":`+fmt.Sprintf("%q", text)+`}`) + sse(`{"type":"response.completed","response":{"status":"completed","usage":{`+usage+`}}}`) + "data: [DONE]\n\n"
}

func toolSSE() string {
	return sse(`{"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_xai_123","name":"weather"}}`) +
		sse(`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"city\":\"Paris\"}"}`) +
		sse(`{"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_xai_123","name":"weather"}}`) +
		sse(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`) + "data: [DONE]\n\n"
}

func sse(data string) string { return "data: " + data + "\n\n" }
