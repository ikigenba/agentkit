package agentkit

import (
	"context"
	"math"
)

// Embedder is one embedding workflow backed by an EmbeddingProvider.
//
// It is not safe for concurrent use.
type Embedder struct {
	Provider   EmbeddingProvider
	Model      string
	Dimensions int
	Retry      RetryPolicy

	totalUsage EmbeddingUsage
	totalCost  Cost
}

// Embed sends a batch of inputs to the configured embedding provider.
func (e *Embedder) Embed(ctx context.Context, inputs []string, role InputType) (*EmbedResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil || e.Provider == nil || e.Model == "" {
		return nil, ErrInvalidConfig
	}
	if len(inputs) == 0 {
		return nil, ErrInvalidInput
	}
	for _, input := range inputs {
		if input == "" {
			return nil, ErrInvalidInput
		}
	}
	pricing, ok := e.Provider.Pricing(e.Model)
	if !ok {
		return nil, ErrInvalidConfig
	}

	rt := e.Provider.Embed(ctx, &EmbedRequest{
		Model:      e.Model,
		Inputs:     append([]string(nil), inputs...),
		Role:       role,
		Dimensions: e.Dimensions,
		Retry:      e.Retry,
	})
	if rt == nil {
		return nil, ErrInvalidConfig
	}
	if err := rt.Err(); err != nil {
		return nil, err
	}

	usage := rt.Usage()
	cost := pricing.Cost(usage)

	result := &EmbedResult{
		Vectors:  normalizeFloat32Vectors(rt.Vectors()),
		Warnings: rt.Warnings(),
		usage:    usage,
		cost:     cost,
	}
	e.totalUsage = addEmbeddingUsage(e.totalUsage, usage)
	e.totalCost += cost
	return result, nil
}

// TotalUsage returns the cumulative usage of successful embedding calls.
func (e *Embedder) TotalUsage() EmbeddingUsage {
	if e == nil {
		return EmbeddingUsage{}
	}
	return e.totalUsage
}

// TotalCost returns the cumulative cost of successful embedding calls.
func (e *Embedder) TotalCost() Cost {
	if e == nil {
		return 0
	}
	return e.totalCost
}

// EmbedResult is one successful embedding call's result.
type EmbedResult struct {
	Vectors  [][]float32
	Warnings []Warning

	usage EmbeddingUsage
	cost  Cost
}

// Usage returns this embedding call's token usage.
func (r *EmbedResult) Usage() EmbeddingUsage {
	if r == nil {
		return EmbeddingUsage{}
	}
	return r.usage
}

// Cost returns this embedding call's cost.
func (r *EmbedResult) Cost() Cost {
	if r == nil {
		return 0
	}
	return r.cost
}

func normalizeFloat32Vectors(vectors [][]float32) [][]float32 {
	normalized := cloneFloat32Vectors(vectors)
	for i, vector := range normalized {
		var sum float64
		for _, value := range vector {
			sum += float64(value) * float64(value)
		}
		if sum == 0 {
			continue
		}
		norm := float32(math.Sqrt(sum))
		for j := range normalized[i] {
			normalized[i][j] /= norm
		}
	}
	return normalized
}
