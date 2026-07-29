package agentkit

// ProviderID names a provider package.
type ProviderID string

const (
	ProviderAnthropic  ProviderID = "anthropic"
	ProviderOpenAI     ProviderID = "openai"
	ProviderGoogle     ProviderID = "google"
	ProviderZAI        ProviderID = "z-ai"
	ProviderOpenRouter ProviderID = "openrouter"
)

// AuthMode names the credential mechanism used by a provider handle.
type AuthMode string

const (
	AuthAPIKey       AuthMode = "apikey"
	AuthSubscription AuthMode = "subscription"
)

// Identity identifies both the provider package and its credential mode.
type Identity struct {
	Provider ProviderID
	Auth     AuthMode
}

// String renders an identity as a compact display label.
func (i Identity) String() string {
	switch {
	case i.Provider == "":
		return string(i.Auth)
	case i.Auth == "":
		return string(i.Provider)
	default:
		return string(i.Provider) + "." + string(i.Auth)
	}
}
