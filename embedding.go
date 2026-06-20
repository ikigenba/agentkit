package agentkit

import "context"

// InputType describes how an embedding input will be used.
type InputType int

const (
	InputUnspecified InputType = iota
	InputQuery
	InputDocument
)

// EmbeddingProvider is implemented by provider sub-packages that support
// embedding models.
type EmbeddingProvider interface {
	Embed(ctx context.Context, req *EmbedRequest) *EmbedRoundTrip
	Name() string
	Pricing(model string) (EmbeddingPricing, bool)
}

// EmbedRequest is one embedding provider call's input, built by Embedder.
type EmbedRequest struct {
	Model      string
	Inputs     []string
	Role       InputType
	Dimensions int
	Retry      RetryPolicy
}

// EmbedRoundTrip is one low-level embedding provider call result.
type EmbedRoundTrip struct {
	vectors  [][]float32
	usage    EmbeddingUsage
	warnings []Warning
	err      error
}

// NewEmbedRoundTrip constructs one embedding provider result.
func NewEmbedRoundTrip(vectors [][]float32, usage EmbeddingUsage, warnings []Warning, err error) *EmbedRoundTrip {
	return &EmbedRoundTrip{
		vectors:  cloneFloat32Vectors(vectors),
		usage:    usage,
		warnings: append([]Warning(nil), warnings...),
		err:      err,
	}
}

// Vectors returns the embedding vectors produced by the provider.
func (r *EmbedRoundTrip) Vectors() [][]float32 {
	if r == nil {
		return nil
	}
	return cloneFloat32Vectors(r.vectors)
}

// Usage returns this provider round-trip's token usage.
func (r *EmbedRoundTrip) Usage() EmbeddingUsage {
	if r == nil {
		return EmbeddingUsage{}
	}
	return r.usage
}

// Warnings returns provider degradation warnings from this embedding call.
func (r *EmbedRoundTrip) Warnings() []Warning {
	if r == nil {
		return nil
	}
	return append([]Warning(nil), r.warnings...)
}

// Err returns this provider round-trip's terminal error.
func (r *EmbedRoundTrip) Err() error {
	if r == nil {
		return ErrInvalidConfig
	}
	return r.err
}

// EmbeddingUsage reports token consumption for an embedding call.
type EmbeddingUsage struct {
	InputTokens int64
	Total       int64
}

// EmbeddingPricing is one embedding model's per-token rate.
//
// Rates are nano-USD per input token.
type EmbeddingPricing struct {
	InputToken int64
}

// Cost computes one embedding call's nano-USD cost.
func (p EmbeddingPricing) Cost(u EmbeddingUsage) Cost {
	return Cost(u.InputTokens * p.InputToken)
}

// EmbeddingSpec describes a provider embedding model's dimensional limits.
type EmbeddingSpec struct {
	NativeDimension int
	MinDimension    int
	MaxDimension    int
	MaxInputTokens  int
}

// EmbeddingInspector exposes a provider package's supported embedding models.
type EmbeddingInspector interface {
	EmbeddingSpec(model string) (EmbeddingSpec, bool)
	SupportedEmbeddings() map[string]EmbeddingSpec
}

func addEmbeddingUsage(a, b EmbeddingUsage) EmbeddingUsage {
	return EmbeddingUsage{
		InputTokens: a.InputTokens + b.InputTokens,
		Total:       a.Total + b.Total,
	}
}

func cloneFloat32Vectors(vectors [][]float32) [][]float32 {
	if vectors == nil {
		return nil
	}
	cloned := make([][]float32, len(vectors))
	for i, vector := range vectors {
		cloned[i] = append([]float32(nil), vector...)
	}
	return cloned
}
