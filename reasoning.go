package agentkit

// ReasoningKind describes the native reasoning-control shape a model exposes.
type ReasoningKind int

const (
	ReasoningEnum ReasoningKind = iota
	ReasoningRange
	ReasoningToggle
)

// ReasoningValue carries exactly one native reasoning form. The zero value is
// unset, meaning provider default.
type ReasoningValue struct {
	tag    reasoningValueTag
	level  string
	budget int
}

type reasoningValueTag int

const (
	reasoningUnset reasoningValueTag = iota
	reasoningLevel
	reasoningBudget
	reasoningDisabled
)

// Level carries a provider-native reasoning level string.
func Level(s string) ReasoningValue {
	return ReasoningValue{tag: reasoningLevel, level: s}
}

// Budget carries a provider-native reasoning token budget.
func Budget(n int) ReasoningValue {
	return ReasoningValue{tag: reasoningBudget, budget: n}
}

// DisableReasoning requests the selected model's native reasoning-off form.
func DisableReasoning() ReasoningValue {
	return ReasoningValue{tag: reasoningDisabled}
}

// IsUnset reports whether v leaves reasoning at the model default.
func (v ReasoningValue) IsUnset() bool {
	return v.tag == reasoningUnset
}

// Level returns v's native level string when v was built by Level.
func (v ReasoningValue) Level() (string, bool) {
	if v.tag != reasoningLevel {
		return "", false
	}
	return v.level, true
}

// Budget returns v's native token budget when v was built by Budget.
func (v ReasoningValue) Budget() (int, bool) {
	if v.tag != reasoningBudget {
		return 0, false
	}
	return v.budget, true
}

// Disabled reports whether v was built by DisableReasoning.
func (v ReasoningValue) Disabled() bool {
	return v.tag == reasoningDisabled
}

// Sentinel records a magic integer budget value and its native meaning.
type Sentinel struct {
	Value   int
	Meaning string
}

// ReasoningSpec is the inspectable native-vocabulary descriptor for one model.
type ReasoningSpec struct {
	Term       string
	Kind       ReasoningKind
	Levels     []string
	Min, Max   int
	Sentinels  []Sentinel
	Default    ReasoningValue
	CanDisable bool
}

// Accepts reports whether v is native to this model's reasoning spec.
func (s ReasoningSpec) Accepts(v ReasoningValue) bool {
	switch {
	case v.IsUnset():
		return true
	case v.Disabled():
		return s.CanDisable
	}
	if level, ok := v.Level(); ok {
		if s.Kind != ReasoningEnum {
			return false
		}
		for _, accepted := range s.Levels {
			if level == accepted {
				return true
			}
		}
		return false
	}
	budget, ok := v.Budget()
	if !ok || s.Kind != ReasoningRange {
		return false
	}
	for _, sentinel := range s.Sentinels {
		if budget == sentinel.Value {
			return true
		}
	}
	return budget >= s.Min && budget <= s.Max
}

// ReasoningInspector reads a provider's credential-blind reasoning vocabulary.
type ReasoningInspector interface {
	ReasoningSpec(model string) (ReasoningSpec, bool)
	SupportedReasoning() map[string]ReasoningSpec
}
