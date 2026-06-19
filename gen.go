package agentkit

// GenSettings holds uniform generation controls. The zero value is "use each
// provider's defaults": nil/0 fields are omitted from the request.
type GenSettings struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   int
	Reasoning   ReasoningValue
}

// Warning records a requested setting a provider could not honor as asked.
type Warning struct {
	Setting string
	Code    WarningCode
	Detail  string
}

// WarningCode classifies non-fatal provider-side setting degradation.
type WarningCode int

const (
	WarnReasoningUnsupported WarningCode = iota
	WarnReasoningCannotDisable
	WarnToolChoiceForced
	WarnToolSchemaLossy
)
