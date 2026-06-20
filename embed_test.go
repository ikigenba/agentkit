package agentkit

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

const testEmbeddingModel = "test-embedding-model"

var testEmbeddingPricing = EmbeddingPricing{InputToken: 11}

type fakeEmbeddingProvider struct {
	name       string
	models     map[string]EmbeddingPricing
	trips      []*EmbedRoundTrip
	embedFn    func(context.Context, *EmbedRequest) *EmbedRoundTrip
	calls      []EmbedRequest
	nameCalls  int
	priceCalls []string
}

func newFakeEmbeddingProvider(trips ...*EmbedRoundTrip) *fakeEmbeddingProvider {
	return &fakeEmbeddingProvider{
		name:   "fake-embeddings",
		models: map[string]EmbeddingPricing{testEmbeddingModel: testEmbeddingPricing},
		trips:  trips,
	}
}

func (p *fakeEmbeddingProvider) Embed(ctx context.Context, req *EmbedRequest) *EmbedRoundTrip {
	p.calls = append(p.calls, cloneEmbedRequest(req))
	if p.embedFn != nil {
		return p.embedFn(ctx, req)
	}
	if len(p.trips) == 0 {
		return &EmbedRoundTrip{vectors: [][]float32{{1, 2}}, usage: EmbeddingUsage{InputTokens: 1, Total: 1}}
	}
	rt := p.trips[0]
	p.trips = p.trips[1:]
	return rt
}

func (p *fakeEmbeddingProvider) Name() string {
	p.nameCalls++
	return p.name
}

func (p *fakeEmbeddingProvider) Pricing(model string) (EmbeddingPricing, bool) {
	p.priceCalls = append(p.priceCalls, model)
	pricing, ok := p.models[model]
	return pricing, ok
}

func TestEmbedRejectsMissingConfigWithoutProviderCall(t *testing.T) {
	ctx := context.Background()

	t.Run("missing provider", func(t *testing.T) {
		// R-Y87O-NUL7
		_, err := (&Embedder{Model: testEmbeddingModel}).Embed(ctx, []string{"hello"}, InputQuery)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Embed() error = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		// R-Y87O-NUL7
		provider := newFakeEmbeddingProvider()
		_, err := (&Embedder{Provider: provider}).Embed(ctx, []string{"hello"}, InputQuery)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("Embed() error = %v, want ErrInvalidConfig", err)
		}
		if len(provider.calls) != 0 {
			t.Fatalf("provider calls = %d, want 0", len(provider.calls))
		}
		if len(provider.priceCalls) != 0 {
			t.Fatalf("pricing calls = %d, want 0", len(provider.priceCalls))
		}
	})
}

func TestEmbedRejectsEmptyInputsWithoutProviderCall(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
	}{
		{name: "nil", inputs: nil},
		{name: "empty", inputs: []string{}},
		{name: "empty string in batch", inputs: []string{"hello", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// R-Y9FL-1MBW
			provider := newFakeEmbeddingProvider()
			_, err := (&Embedder{Provider: provider, Model: testEmbeddingModel}).Embed(context.Background(), tt.inputs, InputDocument)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Embed() error = %v, want ErrInvalidInput", err)
			}
			if len(provider.calls) != 0 {
				t.Fatalf("provider calls = %d, want 0", len(provider.calls))
			}
			if len(provider.priceCalls) != 0 {
				t.Fatalf("pricing calls = %d, want 0", len(provider.priceCalls))
			}
		})
	}
}

func TestEmbedAccountsUsageAndCostAcrossSuccessfulCalls(t *testing.T) {
	// R-YFJ2-YH1D
	// R-YQI6-EEPM
	firstUsage := EmbeddingUsage{InputTokens: 3, Total: 5}
	secondUsage := EmbeddingUsage{InputTokens: 7, Total: 9}
	provider := newFakeEmbeddingProvider(
		&EmbedRoundTrip{
			vectors:  [][]float32{{1, 2}, {3, 4}},
			usage:    firstUsage,
			warnings: []Warning{{Setting: "dimensions", Detail: "rounded"}},
		},
		&EmbedRoundTrip{
			vectors: [][]float32{{5, 6}},
			usage:   secondUsage,
		},
	)
	retry := RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	embedder := &Embedder{
		Provider:   provider,
		Model:      testEmbeddingModel,
		Dimensions: 256,
		Retry:      retry,
	}

	first, err := embedder.Embed(context.Background(), []string{"hello", "world"}, InputQuery)
	if err != nil {
		t.Fatalf("first Embed() error = %v, want nil", err)
	}
	if first.Usage() != firstUsage {
		t.Fatalf("first Usage() = %#v, want %#v", first.Usage(), firstUsage)
	}
	if first.Cost() != testEmbeddingPricing.Cost(firstUsage) {
		t.Fatalf("first Cost() = %d, want %d", first.Cost(), testEmbeddingPricing.Cost(firstUsage))
	}
	if embedder.TotalUsage() != firstUsage {
		t.Fatalf("TotalUsage() after first = %#v, want %#v", embedder.TotalUsage(), firstUsage)
	}
	if embedder.TotalCost() != testEmbeddingPricing.Cost(firstUsage) {
		t.Fatalf("TotalCost() after first = %d, want %d", embedder.TotalCost(), testEmbeddingPricing.Cost(firstUsage))
	}
	if got, want := provider.calls[0], (EmbedRequest{
		Model:      testEmbeddingModel,
		Inputs:     []string{"hello", "world"},
		Role:       InputQuery,
		Dimensions: 256,
		Retry:      retry,
	}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first request = %#v, want %#v", got, want)
	}

	second, err := embedder.Embed(context.Background(), []string{"again"}, InputDocument)
	if err != nil {
		t.Fatalf("second Embed() error = %v, want nil", err)
	}
	wantUsage := EmbeddingUsage{InputTokens: firstUsage.InputTokens + secondUsage.InputTokens, Total: firstUsage.Total + secondUsage.Total}
	wantCost := testEmbeddingPricing.Cost(firstUsage) + testEmbeddingPricing.Cost(secondUsage)
	if second.Usage() != secondUsage {
		t.Fatalf("second Usage() = %#v, want %#v", second.Usage(), secondUsage)
	}
	if second.Cost() != testEmbeddingPricing.Cost(secondUsage) {
		t.Fatalf("second Cost() = %d, want %d", second.Cost(), testEmbeddingPricing.Cost(secondUsage))
	}
	if embedder.TotalUsage() != wantUsage {
		t.Fatalf("TotalUsage() after second = %#v, want %#v", embedder.TotalUsage(), wantUsage)
	}
	if embedder.TotalCost() != wantCost {
		t.Fatalf("TotalCost() after second = %d, want %d", embedder.TotalCost(), wantCost)
	}
}

func TestEmbeddingPricingCostUsesInputTokensOnly(t *testing.T) {
	// R-YQI6-EEPM
	pricing := EmbeddingPricing{InputToken: 13}
	usage := EmbeddingUsage{InputTokens: 17, Total: 999}
	want := Cost(17 * 13)
	if got := pricing.Cost(usage); got != want {
		t.Fatalf("EmbeddingPricing.Cost() = %d, want %d", got, want)
	}
}

func cloneEmbedRequest(req *EmbedRequest) EmbedRequest {
	if req == nil {
		return EmbedRequest{}
	}
	cloned := *req
	cloned.Inputs = append([]string(nil), req.Inputs...)
	return cloned
}
