package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestVerifyReplicatedClassifiesAggregateOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		sameSecond  bool
		sameAll     bool
		wantOutcome ReplicatedOutcome
	}{
		{name: "replicated change", wantOutcome: ReplicatedChange},
		{name: "no change observed", sameAll: true, wantOutcome: NoChangeObserved},
		{name: "mixed", sameSecond: true, wantOutcome: MixedInconsistent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := makeReplicatedRoot(t, test.sameSecond, test.sameAll)
			summary, err := VerifyReplicated(root)
			if err != nil {
				t.Fatalf("VerifyReplicated() error = %v", err)
			}
			if summary.Outcome != test.wantOutcome ||
				summary.EvidenceState != "observed" ||
				summary.Pairs != 2 ||
				summary.PairsPerOrder != 1 ||
				summary.CompletedPairs != 2 ||
				summary.UnknownPairs != 0 {
				t.Fatalf("summary = %#v", summary)
			}
			if test.wantOutcome == ReplicatedChange && summary.ChangedPairs != 2 {
				t.Fatalf("changed pairs = %d", summary.ChangedPairs)
			}
			if test.wantOutcome == NoChangeObserved && summary.NoChangePairs != 2 {
				t.Fatalf("no-change pairs = %d", summary.NoChangePairs)
			}
			data, err := json.Marshal(summary)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"standard", "personalized", "request_id"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("summary exposed %q: %s", secret, data)
				}
			}
		})
	}
}

func TestVerifyReplicatedReturnsSafeEvidenceIdentities(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := os.ReadFile(filepath.Join(root, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	if summary.ReceiptSHA256 != digestSHA256(receipt) || len(summary.ReceiptSHA256) != 64 {
		t.Fatalf("receipt digest = %q", summary.ReceiptSHA256)
	}
	for _, pair := range summary.PairSummaries {
		if pair.EvidenceSHA256 == "" || len(pair.EvidenceSHA256) != 64 {
			t.Fatalf("pair evidence digest missing: %#v", pair)
		}
	}
}

func TestVerifyReplicatedRejectsTamperedVerifiedOutputs(t *testing.T) {
	for _, name := range []string{"evidence.json", "report.md"} {
		t.Run(name, func(t *testing.T) {
			root := makeReplicatedRoot(t, false, false)
			path := filepath.Join(root, "pair-001-baseline-treatment", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, '\n')
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyReplicated(root); err == nil {
				t.Fatal("VerifyReplicated() error = nil")
			}
		})
	}
}

func TestVerifyReplicatedRejectsCrossPairProvenanceMismatch(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	for _, kind := range []string{"baseline", "treatment"} {
		path := filepath.Join(root, "pair-001-treatment-baseline", kind, "session.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var record adb.SessionRecord
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		record.PackageSHA256 = strings.Repeat("e", 64)
		data, err = json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := VerifyReplicated(root); err == nil {
		t.Fatal("VerifyReplicated() error = nil")
	}
}

func TestVerifyReplicatedRejectsReceiptManifestMetadataMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*adb.ReplicatedRunRecord)
	}{
		{name: "manifest", mutate: func(record *adb.ReplicatedRunRecord) { record.ManifestName = "other" }},
		{name: "declared variable", mutate: func(record *adb.ReplicatedRunRecord) { record.DeclaredVariable = "region" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := makeReplicatedRoot(t, false, false)
			data, err := os.ReadFile(filepath.Join(root, "replication.json"))
			if err != nil {
				t.Fatal(err)
			}
			var record adb.ReplicatedRunRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			test.mutate(&record)
			writeReplicatedRecordForTest(t, root, record)
			if _, err := VerifyReplicated(root); err == nil {
				t.Fatal("VerifyReplicated() error = nil")
			}
		})
	}
}

func TestVerifyReplicatedReturnsUnknownForIncompleteRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "replicated")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	first := makeReplicatedPair(t, root, 1, adb.ReplicationOrderBaselineTreatment, false)
	second := makeReplicatedPair(t, root, 1, adb.ReplicationOrderTreatmentBaseline, false)
	record := adb.ReplicatedRunRecord{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     "experiment-001-email",
		DeclaredVariable: "email",
		PairsPerOrder:    1,
		ResetPolicy:      adb.ReplicationResetPolicy,
		Status:           adb.ReplicationStatusIncomplete,
		CompletedPairs:   1,
		FailurePair:      1,
		FailureOrder:     adb.ReplicationOrderTreatmentBaseline,
		Pairs: []adb.ReplicatedPairRecord{
			first,
			{
				Pair:          second.Pair,
				Order:         second.Order,
				Directory:     second.Directory,
				FirstSession:  second.FirstSession,
				SecondSession: second.SecondSession,
				Status:        adb.ReplicationStatusIncomplete,
			},
		},
	}
	writeReplicatedRecordForTest(t, root, record)

	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatalf("VerifyReplicated() error = %v", err)
	}
	if summary.Outcome != ReplicationUnknown ||
		summary.EvidenceState != "unknown" ||
		summary.ChangedPairs != 1 ||
		summary.UnknownPairs != 1 ||
		summary.CompletedPairs != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestVerifyReplicatedRejectsOrderContractMismatch(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	path := filepath.Join(root, "replication.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record adb.ReplicatedRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.Pairs[1].FirstSession = "baseline"
	writeReplicatedRecordForTest(t, root, record)
	if _, err := VerifyReplicated(root); err == nil {
		t.Fatal("VerifyReplicated() error = nil")
	}
}

func makeReplicatedRoot(t *testing.T, sameSecond, sameAll bool) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "replicated")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	makeReplicatedPair(t, root, 1, adb.ReplicationOrderBaselineTreatment, sameAll)
	makeReplicatedPair(t, root, 1, adb.ReplicationOrderTreatmentBaseline, sameAll || sameSecond)
	first := adb.ReplicatedPairRecord{
		Pair:          1,
		Order:         adb.ReplicationOrderBaselineTreatment,
		Directory:     "pair-001-baseline-treatment",
		FirstSession:  "baseline",
		SecondSession: "treatment",
		Status:        adb.ReplicationStatusComplete,
	}
	second := adb.ReplicatedPairRecord{
		Pair:          1,
		Order:         adb.ReplicationOrderTreatmentBaseline,
		Directory:     "pair-001-treatment-baseline",
		FirstSession:  "treatment",
		SecondSession: "baseline",
		Status:        adb.ReplicationStatusComplete,
	}
	record := adb.ReplicatedRunRecord{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     "experiment-001-email",
		DeclaredVariable: "email",
		PairsPerOrder:    1,
		ResetPolicy:      adb.ReplicationResetPolicy,
		Status:           adb.ReplicationStatusComplete,
		CompletedPairs:   2,
		Pairs:            []adb.ReplicatedPairRecord{first, second},
	}
	writeReplicatedRecordForTest(t, root, record)
	return root
}

func makeReplicatedPair(
	t *testing.T,
	root string,
	pair int,
	order string,
	same bool,
) adb.ReplicatedPairRecord {
	t.Helper()
	options := runOptions{}
	if same {
		options.treatmentStorage = baselineObservation
		options.treatmentNetworkBody = baselineObservation
	}
	runDir := makeRun(t, options)
	if order == adb.ReplicationOrderTreatmentBaseline {
		shiftSession(t, filepath.Join(runDir, "treatment", "session.json"), -30*time.Second)
	}
	directory := "pair-001-" + order
	if err := os.Rename(runDir, filepath.Join(root, directory)); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(filepath.Join(root, directory)); err != nil {
		t.Fatalf("Write(%s): %v", directory, err)
	}
	first, second := "baseline", "treatment"
	if order == adb.ReplicationOrderTreatmentBaseline {
		first, second = second, first
	}
	return adb.ReplicatedPairRecord{
		Pair:          pair,
		Order:         order,
		Directory:     directory,
		FirstSession:  first,
		SecondSession: second,
		Status:        adb.ReplicationStatusComplete,
	}
}

func shiftSession(t *testing.T, path string, delta time.Duration) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record adb.SessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.StartedAt = record.StartedAt.Add(delta)
	record.FinishedAt = record.FinishedAt.Add(delta)
	for index := range record.Steps {
		record.Steps[index].StartedAt = record.Steps[index].StartedAt.Add(delta)
		record.Steps[index].FinishedAt = record.Steps[index].FinishedAt.Add(delta)
	}
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeReplicatedRecordForTest(t *testing.T, root string, record adb.ReplicatedRunRecord) {
	t.Helper()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "replication.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyReplicatedRejectsMalformedReceiptContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*adb.ReplicatedRunRecord)
	}{
		{name: "schema", mutate: func(record *adb.ReplicatedRunRecord) { record.SchemaVersion = 2 }},
		{name: "manifest", mutate: func(record *adb.ReplicatedRunRecord) { record.ManifestName = "" }},
		{name: "pairs", mutate: func(record *adb.ReplicatedRunRecord) { record.PairsPerOrder = 0 }},
		{name: "reset policy", mutate: func(record *adb.ReplicatedRunRecord) { record.ResetPolicy = "manual" }},
		{name: "status", mutate: func(record *adb.ReplicatedRunRecord) { record.Status = "running" }},
		{name: "completed count", mutate: func(record *adb.ReplicatedRunRecord) { record.CompletedPairs = -1 }},
		{name: "pair number", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs[0].Pair = 0 }},
		{name: "pair order", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs[0].Order = "other" }},
		{name: "directory", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs[0].Directory = "other" }},
		{name: "sessions", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs[0].FirstSession = "treatment" }},
		{name: "pair status", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs[0].Status = "running" }},
		{name: "duplicate", mutate: func(record *adb.ReplicatedRunRecord) { record.Pairs = append(record.Pairs, record.Pairs[0]) }},
		{name: "missing complete pair", mutate: func(record *adb.ReplicatedRunRecord) {
			record.Pairs = record.Pairs[:1]
			record.CompletedPairs = 1
		}},
		{name: "complete failure marker", mutate: func(record *adb.ReplicatedRunRecord) { record.FailurePair = 1 }},
		{name: "incomplete failure", mutate: func(record *adb.ReplicatedRunRecord) {
			record.Status = adb.ReplicationStatusIncomplete
			record.CompletedPairs = 2
			record.FailurePair = 1
			record.FailureOrder = adb.ReplicationOrderTreatmentBaseline
			record.Pairs[1].Status = adb.ReplicationStatusIncomplete
		}},
		{name: "incomplete failure missing", mutate: func(record *adb.ReplicatedRunRecord) {
			record.Status = adb.ReplicationStatusIncomplete
			record.FailurePair = 2
			record.FailureOrder = adb.ReplicationOrderTreatmentBaseline
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := makeReplicatedRoot(t, false, false)
			data, err := os.ReadFile(filepath.Join(root, "replication.json"))
			if err != nil {
				t.Fatal(err)
			}
			var record adb.ReplicatedRunRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			test.mutate(&record)
			writeReplicatedRecordForTest(t, root, record)
			if _, err := VerifyReplicated(root); err == nil {
				t.Fatal("VerifyReplicated() error = nil")
			}
		})
	}
}

func TestVerifyReplicatedRejectsMalformedReceiptEncoding(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "empty root", data: ""},
		{name: "duplicate key", data: `{"schema_version":1,"schema_version":1}`},
		{name: "unknown field", data: `{"unexpected":true}`},
		{name: "trailing data", data: `{"schema_version":1} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "replicated")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "replication.json"), []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyReplicated(root); err == nil {
				t.Fatal("VerifyReplicated() error = nil")
			}
		})
	}
	if _, err := VerifyReplicated(""); err == nil {
		t.Fatal(`VerifyReplicated("") error = nil`)
	}
	if _, err := VerifyReplicated(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("VerifyReplicated(missing) error = nil")
	}
}

func TestVerifyReplicatedRejectsMissingPairDirectory(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	if err := os.RemoveAll(filepath.Join(root, "pair-001-baseline-treatment")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReplicated(root); err == nil {
		t.Fatal("VerifyReplicated() error = nil")
	}
}

func TestVerifyReplicatedAllowsMissingFuturePairsAsUnknown(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	data, err := os.ReadFile(filepath.Join(root, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record adb.ReplicatedRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.PairsPerOrder = 2
	record.Pairs = record.Pairs[:1]
	record.Pairs = append(record.Pairs, adb.ReplicatedPairRecord{
		Pair:          1,
		Order:         adb.ReplicationOrderTreatmentBaseline,
		Directory:     "pair-001-treatment-baseline",
		FirstSession:  "treatment",
		SecondSession: "baseline",
		Status:        adb.ReplicationStatusIncomplete,
	})
	record.CompletedPairs = 1
	record.Status = adb.ReplicationStatusIncomplete
	record.FailurePair = 1
	record.FailureOrder = adb.ReplicationOrderTreatmentBaseline
	writeReplicatedRecordForTest(t, root, record)

	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatalf("VerifyReplicated() error = %v", err)
	}
	if summary.Outcome != ReplicationUnknown || summary.UnknownPairs != 3 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestReplicatedEvidenceStateBoundaries(t *testing.T) {
	if got := replicatedEvidenceState(Summary{AnswerState: evidence.Unknown}); got != evidence.Unknown {
		t.Fatalf("unknown evidence state = %q", got)
	}
	if got := replicatedEvidenceState(Summary{AnswerState: evidence.Observed}); got != evidence.Observed {
		t.Fatalf("observed evidence state = %q", got)
	}
	if got := replicatedEvidenceState(Summary{}); got != evidence.Unknown {
		t.Fatalf("empty evidence state = %q", got)
	}
	if got := aggregateEvidenceState([]ReplicatedPairSummary{{EvidenceState: evidence.Inferred}}); got != evidence.Unknown {
		t.Fatalf("inferred aggregate state = %q", got)
	}
	if got := aggregateEvidenceState(nil); got != evidence.Observed {
		t.Fatalf("empty aggregate state = %q", got)
	}
}

func TestVerifyReplicatedRejectsInvalidPairEvidence(t *testing.T) {
	tests := []struct {
		name string
		edit func(*testing.T, string)
	}{
		{
			name: "first session",
			edit: func(t *testing.T, root string) {
				if err := os.WriteFile(filepath.Join(root, "pair-001-baseline-treatment", "baseline", "session.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "second session",
			edit: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, "pair-001-baseline-treatment", "treatment", "session.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "overlapping order",
			edit: func(t *testing.T, root string) {
				shiftSession(t, filepath.Join(root, "pair-001-baseline-treatment", "treatment", "session.json"), -15*time.Second)
			},
		},
		{
			name: "invalid comparison",
			edit: func(t *testing.T, root string) {
				path := filepath.Join(root, "pair-001-baseline-treatment", "baseline", "observations", "storage.json")
				if err := os.WriteFile(path, []byte(`{"schema_version":`), 0o600); err != nil {
					t.Fatal(err)
				}
				updateArtifactForTest(t, filepath.Join(root, "pair-001-baseline-treatment", "baseline", "session.json"), "observations/storage.json", []byte(`{"schema_version":`))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := makeReplicatedRoot(t, false, false)
			test.edit(t, root)
			if _, err := VerifyReplicated(root); err == nil {
				t.Fatal("VerifyReplicated() error = nil")
			}
		})
	}
}

func TestVerifyReplicatedClassifiesCompletePairWithUnknownEvidence(t *testing.T) {
	root := makeReplicatedRoot(t, false, false)
	sessionPath := filepath.Join(
		root,
		"pair-001-baseline-treatment",
		"treatment",
		"session.json",
	)
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	var record adb.SessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	markStorageCaptureFailure(&record)
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(
		root,
		"pair-001-baseline-treatment",
		"treatment",
		"observations",
		"storage.json",
	)); err != nil {
		t.Fatal(err)
	}
	removePairOutputs(t, filepath.Join(root, "pair-001-baseline-treatment"))
	if _, err := Write(filepath.Join(root, "pair-001-baseline-treatment")); err != nil {
		t.Fatalf("Write(unknown pair): %v", err)
	}

	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatalf("VerifyReplicated() error = %v", err)
	}
	if summary.Outcome != ReplicationUnknown || summary.UnknownPairs != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func updateArtifactForTest(t *testing.T, path, artifactPath string, data []byte) {
	t.Helper()
	sessionData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record adb.SessionRecord
	if err := json.Unmarshal(sessionData, &record); err != nil {
		t.Fatal(err)
	}
	for index := range record.Artifacts {
		if record.Artifacts[index].Path == artifactPath {
			sum := sha256.Sum256(data)
			record.Artifacts[index].SizeBytes = len(data)
			record.Artifacts[index].SHA256 = hex.EncodeToString(sum[:])
		}
	}
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func removePairOutputs(t *testing.T, pairDir string) {
	t.Helper()
	for _, name := range []string{"evidence.json", "report.md"} {
		if err := os.Remove(filepath.Join(pairDir, name)); err != nil {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
}
