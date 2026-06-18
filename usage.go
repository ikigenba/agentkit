package agentkit

// Usage reports token consumption for a turn in disjoint buckets.
//
// The summing buckets are InputUncached, CacheReadInput, CacheWriteInput,
// Output, and ReasoningOutput; they sum to Total. CacheWrite5m and
// CacheWrite1h are an informational sub-split of CacheWriteInput and are not
// added again. Any field a provider cannot report stays 0.
type Usage struct {
	InputUncached   int64
	CacheReadInput  int64
	CacheWriteInput int64
	CacheWrite5m    int64
	CacheWrite1h    int64
	Output          int64
	ReasoningOutput int64
	Total           int64
}
