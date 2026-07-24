// Package evidence defines how Ariadne qualifies conclusions.
package evidence

// State describes the relationship between a conclusion and its support.
type State string

const (
	// Observed means captured artifacts directly contain the reported behavior.
	Observed State = "observed"
	// Inferred means evidence supports the conclusion without directly capturing it.
	Inferred State = "inferred"
	// Claimed means a vendor, operator, or other source states the behavior.
	Claimed State = "claimed"
	// Unknown means the available capture cannot establish what happened.
	Unknown State = "unknown"
)

// Valid reports whether the state is part of Ariadne's evidence model.
func (s State) Valid() bool {
	switch s {
	case Observed, Inferred, Claimed, Unknown:
		return true
	default:
		return false
	}
}
