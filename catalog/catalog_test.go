package catalog

import (
	"reflect"
	"testing"

	"github.com/ikigenba/agentkit"
)

func TestLookupReturnsChatEmbeddingAndUnknownEntries(t *testing.T) {
	// R-DMDH-5FOB
	chat, ok := Lookup("claude-opus-4-8")
	if !ok || chat.Vendor != VendorAnthropic || len(chat.Offerings) != 1 {
		t.Fatalf("Lookup(chat) = %#v/%v, want Anthropic entry with one offering", chat, ok)
	}
	if got := chat.Offerings[0]; got.Provider != agentkit.ProviderAnthropic || got.Pricing == nil || got.Reasoning == nil || got.Context == 0 {
		t.Fatalf("Lookup(chat) offering = %#v, want complete Anthropic terms", got)
	}

	embedding, ok := Lookup("text-embedding-3-small")
	wantEmbedding := &EmbeddingInfo{
		Pricing:         agentkit.EmbeddingPricing{InputToken: 20},
		NativeDimension: 1536,
		MinDimension:    1,
		MaxDimension:    1536,
		MaxInputTokens:  8192,
	}
	if !ok || !reflect.DeepEqual(embedding.Embedding, wantEmbedding) {
		t.Fatalf("Lookup(embedding) = %#v/%v, want %#v", embedding, ok, wantEmbedding)
	}

	if unknown, ok := Lookup("future-model-not-yet-cataloged"); ok || !reflect.DeepEqual(unknown, Entry{}) {
		t.Fatalf("Lookup(unknown) = %#v/%v, want zero/false", unknown, ok)
	}
}

func TestOfferingOrderIsNonEmptyAndSelectsDefault(t *testing.T) {
	// R-LOW2-SDWG
	for model, stored := range entries {
		if len(stored.Offerings) == 0 {
			t.Fatalf("%s has no offerings", model)
		}
		got, ok := Lookup(model)
		if !ok || !reflect.DeepEqual(got.Offerings, stored.Offerings) {
			t.Errorf("Lookup(%q).Offerings = %#v, want authored order %#v", model, got.Offerings, stored.Offerings)
		}
		if resolved := Resolve("", model); resolved.Coverage != Curated || !reflect.DeepEqual(resolved.Offering, stored.Offerings[0]) {
			t.Errorf("Resolve(\"\", %q) = %#v, want first offering %#v", model, resolved, stored.Offerings[0])
		}
	}

	const model = "test-offering-order"
	first := Offering{Provider: agentkit.ProviderGoogle, Context: 1}
	second := Offering{Provider: agentkit.ProviderOpenAI, Context: 2}
	entries[model] = Entry{Model: model, Vendor: VendorGoogle, Offerings: []Offering{first, second}}
	t.Cleanup(func() { delete(entries, model) })
	if got := Resolve("", model).Offering; !reflect.DeepEqual(got, first) {
		t.Fatalf("Resolve before reorder = %#v, want %#v", got, first)
	}
	entries[model] = Entry{Model: model, Vendor: VendorGoogle, Offerings: []Offering{second, first}}
	if got := Resolve("", model).Offering; !reflect.DeepEqual(got, second) {
		t.Fatalf("Resolve after reorder = %#v, want %#v", got, second)
	}
}

func TestOfferingsAndOfferAgree(t *testing.T) {
	// R-LRBV-JXDU
	offerings := Offerings("glm-5.2")
	if len(offerings) != 2 || offerings[0].Provider != agentkit.ProviderZAI || offerings[1].Provider != agentkit.ProviderOpenRouter {
		t.Fatalf("Offerings(glm-5.2) = %#v, want ZAI then OpenRouter", offerings)
	}
	for i, want := range offerings {
		got, ok := Offer("glm-5.2", want.Provider)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Errorf("Offer(glm-5.2, %q) = %#v/%v, want offering %d %#v", want.Provider, got, ok, i, want)
		}
	}
	if got := Offerings("unknown"); got != nil {
		t.Fatalf("Offerings(unknown) = %#v, want nil", got)
	}
	if got, ok := Offer("glm-5.2", agentkit.ProviderGoogle); ok || !reflect.DeepEqual(got, Offering{}) {
		t.Fatalf("Offer(missing provider) = %#v/%v, want zero/false", got, ok)
	}
}

func TestResolveCoversCuratedPassthruAndUnrouted(t *testing.T) {
	// R-DNLD-J7F0
	defaultRoute := Resolve("", "glm-5.2")
	if defaultRoute.Coverage != Curated || defaultRoute.Provider != agentkit.ProviderZAI ||
		defaultRoute.WireModel != "glm-5.2" || !reflect.DeepEqual(defaultRoute.Offering, Offerings("glm-5.2")[0]) {
		t.Fatalf("Resolve(default) = %#v, want first curated offering", defaultRoute)
	}

	namedRoute := Resolve(agentkit.ProviderOpenRouter, "glm-5.2")
	if namedRoute.Coverage != Curated || namedRoute.Provider != agentkit.ProviderOpenRouter ||
		namedRoute.WireModel != "z-ai/glm-5.2" || !reflect.DeepEqual(namedRoute.Offering, Offerings("glm-5.2")[1]) {
		t.Fatalf("Resolve(named) = %#v, want OpenRouter offering", namedRoute)
	}

	for _, test := range []struct {
		name       string
		model      string
		wantVendor VendorID
	}{
		{name: "cataloged model", model: "glm-5.2", wantVendor: VendorZAI},
		{name: "uncataloged model", model: "future-model"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := Resolve(agentkit.ProviderGoogle, test.model)
			want := Resolution{Vendor: test.wantVendor, Provider: agentkit.ProviderGoogle, WireModel: test.model, Coverage: Passthru}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Resolve(passthru) = %#v, want %#v", got, want)
			}
		})
	}

	got := Resolve("", "future-model")
	want := Resolution{WireModel: "future-model", Coverage: Unrouted}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve(unrouted) = %#v, want %#v", got, want)
	}
}

func TestWireModelIsDerivedFromVendorAndProvider(t *testing.T) {
	// R-LQ3Z-65N5
	grok, _ := Lookup("grok-4.5")
	if got := grok.WireModel(agentkit.ProviderOpenRouter); got != "x-ai/grok-4.5" {
		t.Fatalf("grok OpenRouter wire model = %q", got)
	}
	glm, _ := Lookup("glm-5.2")
	if got := glm.WireModel(agentkit.ProviderOpenRouter); got != "z-ai/glm-5.2" {
		t.Fatalf("GLM OpenRouter wire model = %q", got)
	}
	if got := glm.WireModel(agentkit.ProviderZAI); got != "glm-5.2" {
		t.Fatalf("GLM direct wire model = %q", got)
	}
	if got := Resolve(agentkit.ProviderOpenRouter, "glm-5.2").WireModel; got != glm.WireModel(agentkit.ProviderOpenRouter) {
		t.Fatalf("Resolve wire model = %q, want Entry.WireModel result", got)
	}
	glm.Vendor = VendorXAI
	if got := glm.WireModel(agentkit.ProviderOpenRouter); got != "x-ai/glm-5.2" {
		t.Fatalf("wire model after vendor change = %q, want derived x-ai namespace", got)
	}
}

func TestVendorAndProviderIDsAgreeWhereBothExist(t *testing.T) {
	// R-LXFD-GS3B
	matches := map[VendorID]agentkit.ProviderID{
		VendorAnthropic: agentkit.ProviderAnthropic,
		VendorOpenAI:    agentkit.ProviderOpenAI,
		VendorGoogle:    agentkit.ProviderGoogle,
		VendorZAI:       agentkit.ProviderZAI,
	}
	for vendor, provider := range matches {
		if string(vendor) != string(provider) {
			t.Errorf("vendor %q != provider %q", vendor, provider)
		}
	}
	if VendorZAI != "z-ai" || agentkit.ProviderZAI != "z-ai" {
		t.Fatalf("ZAI ids = %q/%q, want z-ai on both sides", VendorZAI, agentkit.ProviderZAI)
	}
	providers := map[string]bool{
		string(agentkit.ProviderAnthropic):  true,
		string(agentkit.ProviderOpenAI):     true,
		string(agentkit.ProviderGoogle):     true,
		string(agentkit.ProviderZAI):        true,
		string(agentkit.ProviderOpenRouter): true,
	}
	for _, vendor := range []VendorID{VendorXAI, VendorDeepSeek, VendorMoonshot} {
		if providers[string(vendor)] {
			t.Errorf("vendor-only id %q unexpectedly matches a provider package", vendor)
		}
	}
}

func TestListCuratedIncludesExactlyProviderOfferings(t *testing.T) {
	// R-DOT9-WZ5P
	openRouter := ListCurated(agentkit.ProviderOpenRouter)
	want := []string{"deepseek-v4-flash", "deepseek-v4-pro", "glm-4.6", "glm-4.7", "glm-5.1", "glm-5.2", "grok-4.20", "grok-4.20-multi-agent", "grok-4.3", "grok-4.5", "kimi-k2.6", "kimi-k2.7-code", "kimi-k3"}
	got := make([]string, len(openRouter))
	for i, entry := range openRouter {
		got[i] = entry.Model
		if _, ok := Offer(entry.Model, agentkit.ProviderOpenRouter); !ok {
			t.Fatalf("ListCurated(openrouter) included entry without offering: %#v", entry)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListCurated(openrouter) = %v, want %v", got, want)
	}
	if got := ListCurated(agentkit.ProviderID("provider-with-no-catalog-rows")); len(got) != 0 {
		t.Fatalf("ListCurated(empty) = %#v, want empty", got)
	}
}

func TestCheckUsesNamedOfferingReasoningSpec(t *testing.T) {
	// R-LYN9-UJU0
	const model = "test-provider-specific-reasoning"
	entries[model] = Entry{
		Model: model, Vendor: VendorZAI,
		Offerings: []Offering{
			{Provider: agentkit.ProviderZAI, Reasoning: enumReasoning("effort", []string{"high"}, "high", false)},
			{Provider: agentkit.ProviderOpenRouter, Reasoning: toggleReasoning(true, true, DefaultOff)},
		},
	}
	t.Cleanup(func() { delete(entries, model) })

	accepted, spec, ok := Check(model, agentkit.ProviderZAI, agentkit.Level("high"))
	if !ok || !accepted || spec == nil || spec.Kind != ReasoningEnum {
		t.Fatalf("Check(native value) = %v/%#v/%v", accepted, spec, ok)
	}
	accepted, spec, ok = Check(model, agentkit.ProviderOpenRouter, agentkit.Level("high"))
	if !ok || accepted || spec == nil || spec.Kind != ReasoningToggle {
		t.Fatalf("Check(foreign value) = %v/%#v/%v", accepted, spec, ok)
	}
	accepted, spec, ok = Check(model, agentkit.ProviderGoogle, agentkit.Level("high"))
	if ok || accepted || spec != nil {
		t.Fatalf("Check(missing offering) = %v/%#v/%v, want false/nil/false", accepted, spec, ok)
	}
}

func TestLookupReturnsIndependentMetadata(t *testing.T) {
	first, _ := Lookup("glm-5.2")
	first.Offerings[0].Pricing.Tiers[0].Output = -1
	first.Offerings[0].Reasoning.Levels[0] = "mutated"
	first.Offerings[1].Context = -1

	second, _ := Lookup("glm-5.2")
	if second.Offerings[0].Pricing.Tiers[0].Output != 4400 ||
		second.Offerings[0].Reasoning.Levels[0] != "high" ||
		second.Offerings[1].Context != 202_752 {
		t.Fatalf("Lookup exposed mutable catalog storage: %#v", second)
	}
}
