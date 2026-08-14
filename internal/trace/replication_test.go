package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReplicationLedgerRoundTripAndAggregate(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("a", 64)
	inputs := []ReplicationPairInput{
		writeReplicationPair(t, root, "changed-forward", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderBaselineTreatment, true),
		writeReplicationPair(t, root, "changed-reverse", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderTreatmentBaseline, true),
	}
	ledgerPath := filepath.Join(root, "replication.json")
	want, err := SaveReplicationLedger(inputs, ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if want.Pairs != 2 || want.BaselineTreatmentPairs != 1 || want.TreatmentBaselinePairs != 1 || !want.OrderBalanced || want.ResetConfirmedPairs != 2 || want.CompletePairs != 2 || want.ChangedPairs != 2 || want.NoChangePairs != 0 || want.UnknownPairs != 0 || want.Outcome != ReplicatedChange || want.EvidenceState != evidence.Observed || !ValidSHA256(want.LedgerSHA256) {
		t.Fatalf("saved summary = %#v", want)
	}
	ledger, got, err := ReadReplicationLedger(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || len(ledger.Pairs) != 2 || ledger.ResetPolicy != ReplicationResetPolicy {
		t.Fatalf("read ledger = %#v, summary = %#v, want = %#v", ledger, got, want)
	}
	verified, err := VerifyReplicationLedger(ledgerPath)
	if err != nil || verified != want {
		t.Fatalf("VerifyReplicationLedger() = %#v, %v", verified, err)
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), root) || strings.Contains(string(data), "https://") || strings.Contains(string(data), "private-value") {
		t.Fatalf("replication ledger disclosed unsafe details: %s", data)
	}
	if _, err := SaveReplicationLedger(inputs, ledgerPath); err == nil {
		t.Fatal("SaveReplicationLedger() overwrote an existing ledger")
	}
}

func TestReplicationLedgerClassifiesMixedAndNoChange(t *testing.T) {
	procedure := strings.Repeat("b", 64)
	for _, test := range []struct {
		name         string
		firstChange  bool
		secondChange bool
		wantOutcome  ReplicatedOutcome
		wantChanged  int
		wantSame     int
	}{
		{name: "mixed", firstChange: true, secondChange: false, wantOutcome: MixedInconsistent, wantChanged: 1, wantSame: 1},
		{name: "same", firstChange: false, secondChange: false, wantOutcome: NoChangeObserved, wantChanged: 0, wantSame: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			inputs := []ReplicationPairInput{
				writeReplicationPair(t, root, "first", replicationTrace("android", "region"), replicationTreatmentTrace("android", "region", test.firstChange), "android-experiment-001", procedure, OrderBaselineTreatment, true),
				writeReplicationPair(t, root, "second", replicationTrace("android", "region"), replicationTreatmentTrace("android", "region", test.secondChange), "android-experiment-001", procedure, OrderTreatmentBaseline, true),
			}
			summary, err := SaveReplicationLedger(inputs, filepath.Join(root, "replication.json"))
			if err != nil {
				t.Fatal(err)
			}
			if summary.Outcome != test.wantOutcome || summary.ChangedPairs != test.wantChanged || summary.NoChangePairs != test.wantSame || summary.EvidenceState != evidence.Observed {
				t.Fatalf("summary = %#v", summary)
			}
		})
	}
}

func TestReplicationLedgerReportsUnknownSupport(t *testing.T) {
	procedure := strings.Repeat("c", 64)
	t.Run("missing reverse order", func(t *testing.T) {
		root := t.TempDir()
		input := writeReplicationPair(t, root, "only", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderBaselineTreatment, true)
		summary, err := SaveReplicationLedger([]ReplicationPairInput{input}, filepath.Join(root, "replication.json"))
		if err != nil {
			t.Fatal(err)
		}
		if summary.OrderBalanced || summary.Outcome != ReplicationUnknown || summary.EvidenceState != evidence.Unknown || !strings.Contains(summary.Reason, "both explicit pair orders") {
			t.Fatalf("summary = %#v", summary)
		}
	})

	t.Run("reset not confirmed", func(t *testing.T) {
		root := t.TempDir()
		inputs := []ReplicationPairInput{
			writeReplicationPair(t, root, "forward", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderBaselineTreatment, false),
			writeReplicationPair(t, root, "reverse", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderTreatmentBaseline, true),
		}
		summary, err := SaveReplicationLedger(inputs, filepath.Join(root, "replication.json"))
		if err != nil {
			t.Fatal(err)
		}
		if summary.OrderBalanced != true || summary.Outcome != ReplicationUnknown || summary.EvidenceState != evidence.Unknown || summary.UnknownPairs != 1 || summary.ResetConfirmedPairs != 1 {
			t.Fatalf("summary = %#v", summary)
		}
	})

	t.Run("partial trace", func(t *testing.T) {
		root := t.TempDir()
		inputs := []ReplicationPairInput{
			writeReplicationPair(t, root, "forward", replicationTrace("browser", "region"), partialReplicationTrace("browser"), "browser-redacted-audit", procedure, OrderBaselineTreatment, true),
			writeReplicationPair(t, root, "reverse", replicationTrace("browser", "region"), partialReplicationTrace("browser"), "browser-redacted-audit", procedure, OrderTreatmentBaseline, true),
		}
		summary, err := SaveReplicationLedger(inputs, filepath.Join(root, "replication.json"))
		if err != nil {
			t.Fatal(err)
		}
		if summary.Outcome != ReplicationUnknown || summary.EvidenceState != evidence.Unknown || summary.UnknownPairs != 2 {
			t.Fatalf("summary = %#v", summary)
		}
	})
}

func TestReplicationLedgerRejectsInvalidDocuments(t *testing.T) {
	valid := validReplicationLedger(t)
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown", data: append(append([]byte(nil), data[:len(data)-1]...), []byte(`,"extra":true}`)...)},
		{name: "trailing", data: append(data, []byte(` {}`)...)},
		{name: "invalid json", data: []byte(`{`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeReplicationLedger(test.data); err == nil {
				t.Fatal("DecodeReplicationLedger() accepted malformed input")
			}
		})
	}
	if _, err := DecodeReplicationLedger(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeReplicationLedger() accepted oversized input")
	}

	for _, test := range []struct {
		name   string
		mutate func(*ReplicationLedger)
	}{
		{name: "schema", mutate: func(ledger *ReplicationLedger) { ledger.SchemaVersion = 2 }},
		{name: "reset policy", mutate: func(ledger *ReplicationLedger) { ledger.ResetPolicy = "unverified" }},
		{name: "position", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].Position = 2 }},
		{name: "trace", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].BaselineTrace.Events[0].Fields[0] = "secret-value" }},
		{name: "treatment trace", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].TreatmentTrace.Events[0].Fields[0] = "secret-value" }},
		{name: "baseline session binding", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].BaselineSession.TraceSHA256 = strings.Repeat("e", 64) }},
		{name: "treatment session binding", mutate: func(ledger *ReplicationLedger) {
			ledger.Pairs[0].TreatmentSession.TraceSHA256 = strings.Repeat("e", 64)
		}},
		{name: "session roles", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].BaselineSession.Role = RoleTreatment }},
		{name: "pair summary", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].Pair.Order = OrderTreatmentBaseline }},
		{name: "session pair identity", mutate: func(ledger *ReplicationLedger) {
			identity := strings.Repeat("e", 64)
			ledger.Pairs[0].BaselineSession.PairSHA256 = identity
			ledger.Pairs[0].TreatmentSession.PairSHA256 = identity
			ledger.Pairs[0].Pair.BaselineSessionSHA256, _ = SessionSHA256(ledger.Pairs[0].BaselineSession)
			ledger.Pairs[0].Pair.TreatmentSessionSHA256, _ = SessionSHA256(ledger.Pairs[0].TreatmentSession)
		}},
		{name: "comparison", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[0].Comparison.Differences = nil }},
		{name: "provenance", mutate: func(ledger *ReplicationLedger) { ledger.Pairs[1].Pair.Scope = "storage" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneReplicationLedger(valid)
			test.mutate(&candidate)
			if _, err := ReplicationLedgerSHA256(candidate); err == nil {
				t.Fatal("ReplicationLedgerSHA256() accepted invalid ledger")
			}
		})
	}
}

func TestReplicationLedgerRequiresInputsAndFiles(t *testing.T) {
	if _, err := SaveReplicationLedger(nil, "ledger.json"); err == nil {
		t.Fatal("SaveReplicationLedger() accepted no pairs")
	}
	if _, err := SaveReplicationLedger([]ReplicationPairInput{{}}, "ledger.json"); err == nil {
		t.Fatal("SaveReplicationLedger() accepted empty pair paths")
	}
	if _, err := SaveReplicationLedger([]ReplicationPairInput{{BaselineTracePath: "missing", TreatmentTracePath: "missing", BaselineSessionPath: "missing", TreatmentSessionPath: "missing"}}, "ledger.json"); err == nil {
		t.Fatal("SaveReplicationLedger() accepted missing pair files")
	}
	if _, _, err := ReadReplicationLedger(""); err == nil {
		t.Fatal("ReadReplicationLedger() accepted empty path")
	}
	if _, err := VerifyReplicationLedger(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("VerifyReplicationLedger() accepted missing ledger")
	}
	if _, err := ReplicationLedgerSHA256(ReplicationLedger{}); err == nil {
		t.Fatal("ReplicationLedgerSHA256() accepted empty ledger")
	}
	if _, err := ReplicationLedgerSHA256(ReplicationLedger{
		SchemaVersion: 1,
		ResetPolicy:   ReplicationResetPolicy,
		Pairs:         make([]ReplicationPair, maxReplicationPairs+1),
	}); err == nil {
		t.Fatal("ReplicationLedgerSHA256() accepted too many pairs")
	}
	if _, err := SaveReplicationLedger([]ReplicationPairInput{{}}, ""); err == nil {
		t.Fatal("SaveReplicationLedger() accepted an empty output path")
	}
	tooMany := make([]ReplicationPairInput, maxReplicationPairs+1)
	if _, err := SaveReplicationLedger(tooMany, filepath.Join(t.TempDir(), "replication.json")); err == nil {
		t.Fatal("SaveReplicationLedger() accepted too many pairs")
	}
	if _, err := readAndDecodeSession(filepath.Join(t.TempDir(), "missing-session.json")); err == nil {
		t.Fatal("readAndDecodeSession() accepted a missing session")
	}
	root := t.TempDir()
	validInput := writeReplicationPair(t, root, "valid", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", strings.Repeat("1", 64), OrderBaselineTreatment, true)
	for name, mutate := range map[string]func(*ReplicationPairInput){
		"baseline trace": func(input *ReplicationPairInput) {
			input.BaselineTracePath = filepath.Join(root, "missing-baseline-trace.json")
		},
		"treatment trace": func(input *ReplicationPairInput) {
			input.TreatmentTracePath = filepath.Join(root, "missing-treatment-trace.json")
		},
		"baseline session": func(input *ReplicationPairInput) {
			input.BaselineSessionPath = filepath.Join(root, "missing-baseline-session.json")
		},
		"treatment session": func(input *ReplicationPairInput) {
			input.TreatmentSessionPath = filepath.Join(root, "missing-treatment-session.json")
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := validInput
			mutate(&candidate)
			if _, err := newReplicationPair(candidate); err == nil {
				t.Fatal("newReplicationPair() accepted a missing input")
			}
		})
	}
	if _, err := replicationLedgerSummary(ReplicationLedger{}); err == nil {
		t.Fatal("replicationLedgerSummary() accepted an empty ledger")
	}
}

func TestReplicationLedgerRejectsMismatchedProvenanceAndMalformedFiles(t *testing.T) {
	procedure := strings.Repeat("f", 64)
	root := t.TempDir()
	first := writeReplicationPair(t, root, "first", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderBaselineTreatment, true)
	second := writeReplicationPair(t, root, "second", replicationTrace("android", "region"), replicationTrace("android", "region", "consent"), "android-experiment-001", procedure, OrderTreatmentBaseline, true)
	if _, err := SaveReplicationLedger([]ReplicationPairInput{first, second}, filepath.Join(root, "mismatched.json")); err == nil {
		t.Fatal("SaveReplicationLedger() accepted mismatched provenance")
	}
	malformed := filepath.Join(root, "malformed.json")
	if err := os.WriteFile(malformed, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadReplicationLedger(malformed); err == nil {
		t.Fatal("ReadReplicationLedger() accepted malformed JSON")
	}
	valid := validReplicationLedger(t)
	invalidSchema := cloneReplicationLedger(valid)
	invalidSchema.SchemaVersion = 2
	invalidSchemaData, err := json.Marshal(invalidSchema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationLedger(invalidSchemaData); err == nil {
		t.Fatal("DecodeReplicationLedger() accepted an unsupported schema")
	}
	firstPair, err := newReplicationPair(first)
	if err != nil {
		t.Fatal(err)
	}
	secondPair, err := newReplicationPair(second)
	if err != nil {
		t.Fatal(err)
	}
	firstPair.Position = 1
	secondPair.Position = 2
	if err := validateReplicationLedger(ReplicationLedger{
		SchemaVersion: 1,
		ResetPolicy:   ReplicationResetPolicy,
		Pairs:         []ReplicationPair{firstPair, secondPair},
	}); err == nil {
		t.Fatal("validateReplicationLedger() accepted mismatched provenance")
	}
	if err := validateReplicationProvenance(validReplicationLedger(t).Pairs[0], ReplicationPair{Pair: SessionPairVerificationSummary{Source: "android"}}); err == nil {
		t.Fatal("validateReplicationProvenance() accepted mismatched source")
	}
}

func validReplicationLedger(t *testing.T) ReplicationLedger {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("d", 64)
	inputs := []ReplicationPairInput{
		writeReplicationPair(t, root, "forward", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderBaselineTreatment, true),
		writeReplicationPair(t, root, "reverse", replicationTrace("browser", "region"), replicationTrace("browser", "region", "consent"), "browser-redacted-audit", procedure, OrderTreatmentBaseline, true),
	}
	path := filepath.Join(root, "replication.json")
	if _, err := SaveReplicationLedger(inputs, path); err != nil {
		t.Fatal(err)
	}
	ledger, _, err := ReadReplicationLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

func cloneReplicationLedger(ledger ReplicationLedger) ReplicationLedger {
	data, err := json.Marshal(ledger)
	if err != nil {
		panic(err)
	}
	var clone ReplicationLedger
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

func writeReplicationPair(t *testing.T, root, name string, baseline, treatment Document, adapter, procedure, order string, resetConfirmed bool) ReplicationPairInput {
	t.Helper()
	baselineTrace := writeTrace(t, baseline)
	treatmentTrace := writeTrace(t, treatment)
	baselineSession := filepath.Join(root, name+"-baseline-session.json")
	treatmentSession := filepath.Join(root, name+"-treatment-session.json")
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, SessionPairInput{
		Adapter: adapter, AdapterVersion: 1, ProcedureSHA256: procedure, Scope: baseline.Scope, Order: order,
	}); err != nil {
		t.Fatal(err)
	}
	return ReplicationPairInput{
		BaselineTracePath: baselineTrace, TreatmentTracePath: treatmentTrace,
		BaselineSessionPath: baselineSession, TreatmentSessionPath: treatmentSession,
		ResetConfirmed: resetConfirmed,
	}
}

func replicationTrace(source string, fields ...string) Document {
	return Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{{
			Source: source, Channel: "network", Kind: "request", Destination: "analytics", Fields: fields,
		}},
	}
}

func replicationTreatmentTrace(source string, field string, changed bool) Document {
	if !changed {
		return replicationTrace(source, field)
	}
	return replicationTrace(source, field, "consent")
}

func partialReplicationTrace(source string) Document {
	trace := replicationTrace(source)
	trace.Completeness = Partial
	trace.Events = []Event{}
	return trace
}
