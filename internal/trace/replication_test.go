package trace

import (
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestClassifyReplicatedPairs(t *testing.T) {
	tests := []struct {
		name     string
		pairs    []ReplicatedPairObservation
		outcome  ReplicatedOutcome
		evidence evidence.State
		changed  int
		noChange int
		unknown  int
	}{
		{name: "change", pairs: []ReplicatedPairObservation{{Differences: 1, EvidenceState: evidence.Observed}}, outcome: ReplicatedChange, evidence: evidence.Observed, changed: 1},
		{name: "same", pairs: []ReplicatedPairObservation{{EvidenceState: evidence.Observed}}, outcome: NoChangeObserved, evidence: evidence.Observed, noChange: 1},
		{name: "mixed", pairs: []ReplicatedPairObservation{{Differences: 1, EvidenceState: evidence.Observed}, {EvidenceState: evidence.Observed}}, outcome: MixedInconsistent, evidence: evidence.Observed, changed: 1, noChange: 1},
		{name: "unknown", pairs: []ReplicatedPairObservation{{Unknowns: 1, EvidenceState: evidence.Unknown}}, outcome: ReplicationUnknown, evidence: evidence.Unknown, unknown: 1},
		{name: "empty", outcome: ReplicationUnknown, evidence: evidence.Unknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyReplicatedPairs(test.pairs)
			if got.Outcome != test.outcome || got.EvidenceState != test.evidence || got.ChangedPairs != test.changed || got.NoChangePairs != test.noChange || got.UnknownPairs != test.unknown {
				t.Fatalf("ClassifyReplicatedPairs() = %#v", got)
			}
		})
	}
}
