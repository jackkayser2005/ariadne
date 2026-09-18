package bundle

import (
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestClassifyReplicatedPairsKeepsOutcomeSeparateFromEvidence(t *testing.T) {
	tests := []struct {
		name         string
		pairs        []ReplicatedPairObservation
		wantOutcome  ReplicatedOutcome
		wantEvidence evidence.State
		wantChanged  int
		wantNoChange int
		wantUnknown  int
	}{
		{name: "replicated change", pairs: []ReplicatedPairObservation{{Differences: 1, EvidenceState: evidence.Observed}, {Differences: 2, EvidenceState: evidence.Observed}}, wantOutcome: ReplicatedChange, wantEvidence: evidence.Observed, wantChanged: 2},
		{name: "no change", pairs: []ReplicatedPairObservation{{EvidenceState: evidence.Observed}, {EvidenceState: evidence.Observed}}, wantOutcome: NoChangeObserved, wantEvidence: evidence.Observed, wantNoChange: 2},
		{name: "mixed", pairs: []ReplicatedPairObservation{{Differences: 1, EvidenceState: evidence.Observed}, {EvidenceState: evidence.Observed}}, wantOutcome: MixedInconsistent, wantEvidence: evidence.Observed, wantChanged: 1, wantNoChange: 1},
		{name: "unknown", pairs: []ReplicatedPairObservation{{Differences: 1, EvidenceState: evidence.Observed}, {Differences: 1, Unknowns: 1, EvidenceState: evidence.Unknown}}, wantOutcome: ReplicationUnknown, wantEvidence: evidence.Unknown, wantChanged: 1, wantUnknown: 1},
		{name: "empty", wantOutcome: ReplicationUnknown, wantEvidence: evidence.Unknown, wantUnknown: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ClassifyReplicatedPairs(test.pairs)
			if got.Outcome != test.wantOutcome || got.EvidenceState != test.wantEvidence || got.ChangedPairs != test.wantChanged || got.NoChangePairs != test.wantNoChange || got.UnknownPairs != test.wantUnknown {
				t.Fatalf("ClassifyReplicatedPairs() = %#v", got)
			}
		})
	}
}
