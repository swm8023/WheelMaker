package im

// Ability is a bitmask describing optional IM channel capabilities.
type Ability uint32

const (
	AbilitySendDebug Ability = 1 << iota
	AbilitySendOptions
	AbilityCardActions
)

// AbilityProvider reports supported channel abilities.
type AbilityProvider interface {
	Abilities() Ability
}

// Has reports whether all requested abilities are supported.
func (a Ability) Has(v Ability) bool {
	return a&v == v
}

// DetectAbilities resolves abilities from an explicit provider first,
// then falls back to interface-based capability detection for compatibility.
func DetectAbilities(ch any) Ability {
	if p, ok := ch.(AbilityProvider); ok {
		return p.Abilities()
	}

	var out Ability
	if _, ok := ch.(DebugSender); ok {
		out |= AbilitySendDebug
	}
	if _, ok := ch.(OptionSender); ok {
		out |= AbilitySendOptions
	}
	if _, ok := ch.(CardActionSubscriber); ok {
		out |= AbilityCardActions
	}
	return out
}
