package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunFixtureReplicatedRecordsBothOrdersAndVerifiesChange(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	var variants []string
	capture := func(procedurePath, driverPath string, args []string, outputPath string) (CaptureSummary, error) {
		if procedurePath == "" || driverPath != "driver" || len(args) < 2 || args[len(args)-2] != "--fixture-variant" {
			t.Fatalf("capture invocation = %q, %q, %#v", procedurePath, driverPath, args)
		}
		variant := args[len(args)-1]
		if variant != "baseline" && variant != "treatment" {
			t.Fatalf("fixture variant = %q", variant)
		}
		variants = append(variants, variant)
		summary := writeFixtureTrace(t, outputPath, variant, portabletrace.Complete)
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: summary}, nil
	}
	root := filepath.Join(t.TempDir(), "replicated")
	if err := runFixtureReplicatedWith(context.Background(), FixtureReplicationInput{
		ProcedurePath: procedurePath,
		DriverPath:    "driver",
		DriverArgs:    []string{"--fixture"},
		OutputDir:     root,
		Pairs:         2,
	}, capture); err != nil {
		t.Fatal(err)
	}
	if want := []string{"baseline", "treatment", "treatment", "baseline", "baseline", "treatment", "treatment", "baseline"}; !slices.Equal(variants, want) {
		t.Fatalf("capture order = %#v, want %#v", variants, want)
	}
	summary, err := VerifyFixtureReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != portabletrace.ReplicatedChange || summary.EvidenceState != evidence.Observed || summary.Pairs != 4 || summary.CompletedPairs != 4 || summary.ChangedPairs != 4 || summary.NoChangePairs != 0 || summary.UnknownPairs != 0 || summary.ResetPolicy != BrowserReplicationResetPolicy {
		t.Fatalf("replication summary = %#v", summary)
	}
	for _, pair := range summary.PairSummaries {
		if pair.Outcome != "changed" || pair.EvidenceState != evidence.Observed || pair.Differences != 1 || pair.Unknowns != 0 || !portabletrace.ValidSHA256(pair.PairSHA256) {
			t.Fatalf("pair summary = %#v", pair)
		}
	}
	recordData, err := os.ReadFile(filepath.Join(root, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recordData), "session_id") || strings.Contains(string(recordData), "consent=") || strings.Contains(string(recordData), "https://") {
		t.Fatalf("replication receipt exposed fixture data: %s", recordData)
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "session_id") || strings.Contains(string(data), "consent=") || strings.Contains(string(data), "https://") {
			t.Fatalf("replication output exposed fixture data in %s: %s", path, data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyFixtureReplicatedClassifiesNoChangeAndUnknown(t *testing.T) {
	tests := []struct {
		name         string
		completeness string
		wantOutcome  portabletrace.ReplicatedOutcome
		wantEvidence evidence.State
	}{
		{name: "no change", completeness: portabletrace.Complete, wantOutcome: portabletrace.NoChangeObserved, wantEvidence: evidence.Observed},
		{name: "unknown coverage", completeness: portabletrace.Partial, wantOutcome: portabletrace.ReplicationUnknown, wantEvidence: evidence.Unknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
			procedure, _, err := ReadProcedure(procedurePath)
			if err != nil {
				t.Fatal(err)
			}
			procedureSHA256, err := ProcedureSHA256(procedure)
			if err != nil {
				t.Fatal(err)
			}
			capture := func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
				summary := writeFixtureTrace(t, outputPath, "baseline", test.completeness)
				return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: summary}, nil
			}
			root := filepath.Join(t.TempDir(), "replicated")
			if err := runFixtureReplicatedWith(context.Background(), FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: root, Pairs: 1}, capture); err != nil {
				t.Fatal(err)
			}
			summary, err := VerifyFixtureReplicated(root)
			if err != nil {
				t.Fatal(err)
			}
			if summary.Outcome != test.wantOutcome || summary.EvidenceState != test.wantEvidence || summary.NoChangePairs != 2 && test.wantOutcome == portabletrace.NoChangeObserved {
				t.Fatalf("summary = %#v", summary)
			}
		})
	}
}

func TestRunFixtureReplicatedRejectsUnsafeInputs(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	base := FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: filepath.Join(t.TempDir(), "out"), Pairs: 1}
	tests := []struct {
		name  string
		input FixtureReplicationInput
		want  string
	}{
		{name: "missing procedure", input: FixtureReplicationInput{DriverPath: "driver", OutputDir: "out", Pairs: 1}, want: "paths"},
		{name: "pair count", input: func() FixtureReplicationInput { value := base; value.Pairs = 9; return value }(), want: "between"},
		{name: "caller variant", input: func() FixtureReplicationInput {
			value := base
			value.DriverArgs = []string{"--fixture-variant", "treatment"}
			return value
		}(), want: "controlled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := runFixtureReplicatedWith(context.Background(), test.input, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runFixtureReplicatedWith() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := runFixtureReplicatedWith(context.Background(), base, nil); err == nil || !strings.Contains(err.Error(), "capture") {
		t.Fatalf("nil capture error = %v", err)
	}
	if err := runFixtureReplicatedWith(context.Background(), base, func(string, string, []string, string) (CaptureSummary, error) {
		return CaptureSummary{}, errors.New("driver failed")
	}); err == nil || !strings.Contains(err.Error(), "driver failed") {
		t.Fatalf("capture failure = %v", err)
	}
}

func TestVerifyFixtureReplicatedRejectsTamperedReceiptAndUnsafePath(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	capture := func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeFixtureTrace(t, outputPath, "baseline", portabletrace.Complete)}, nil
	}
	root := filepath.Join(t.TempDir(), "replicated")
	if err := runFixtureReplicatedWith(context.Background(), FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: root, Pairs: 1}, capture); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, "replication.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		name string
		data string
		want string
	}{
		{name: "unknown field", data: strings.Replace(string(receipt), "{\n", "{\n  \"extra\":true,\n", 1), want: "invalid"},
		{name: "duplicate", data: strings.Replace(string(receipt), "\"schema_version\": "+strconv.Itoa(BrowserReplicationSchemaVersion)+",", "\"schema_version\": "+strconv.Itoa(BrowserReplicationSchemaVersion)+",\n  \"schema_version\": "+strconv.Itoa(BrowserReplicationSchemaVersion)+",", 1), want: "invalid"},
		{name: "wrong status", data: strings.Replace(string(receipt), `"status": "complete"`, `"status": "incomplete"`, 1), want: "missing"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if err := os.WriteFile(receiptPath, []byte(mutation.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyFixtureReplicated(root); err == nil || !strings.Contains(err.Error(), mutation.want) {
				t.Fatalf("VerifyFixtureReplicated() error = %v, want %q", err, mutation.want)
			}
			if err := os.WriteFile(receiptPath, receipt, 0o600); err != nil {
				t.Fatal(err)
			}
		})
	}
	record, _, err := readFixtureReplicationRecord(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFixtureReplicationRecord(root, record); err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatalf("exclusive receipt write error = %v", err)
	}
	if _, _, err := readFixtureReplicationRecord(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty receipt path error = %v", err)
	}
	if err := os.WriteFile(receiptPath, append(receipt, []byte("{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readFixtureReplicationRecord(root); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing receipt error = %v", err)
	}
	if err := os.WriteFile(receiptPath, receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		name   string
		mutate func(*ReplicatedRunRecord)
	}{
		{name: "pair status", mutate: func(value *ReplicatedRunRecord) { value.Pairs[0].Status = "running" }},
		{name: "pair duplicate", mutate: func(value *ReplicatedRunRecord) { value.Pairs = append(value.Pairs, value.Pairs[0]) }},
		{name: "pair directory", mutate: func(value *ReplicatedRunRecord) { value.Pairs[0].Directory = "other" }},
		{name: "pair session", mutate: func(value *ReplicatedRunRecord) { value.Pairs[0].FirstSession = "other" }},
		{name: "completed mismatch", mutate: func(value *ReplicatedRunRecord) { value.CompletedPairs = 1 }},
		{name: "complete metadata", mutate: func(value *ReplicatedRunRecord) { value.FailurePair = 1 }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			value := record
			value.Pairs = append([]ReplicatedPairRecord(nil), record.Pairs...)
			mutation.mutate(&value)
			if err := validateFixtureReplicationRecord(root, value); err == nil {
				t.Fatal("validateFixtureReplicationRecord() accepted invalid pair metadata")
			}
		})
	}
	incomplete := record
	incomplete.Status = BrowserReplicationStatusIncomplete
	incomplete.CompletedPairs = 1
	incomplete.Pairs = append([]ReplicatedPairRecord(nil), record.Pairs...)
	incomplete.Pairs[1].Status = BrowserReplicationStatusIncomplete
	incomplete.FailurePair = incomplete.Pairs[1].Pair
	incomplete.FailureOrder = incomplete.Pairs[1].Order
	if err := validateFixtureReplicationRecord(root, incomplete); err != nil {
		t.Fatalf("valid incomplete metadata rejected: %v", err)
	}
	incomplete.FailurePair = 8
	if err := validateFixtureReplicationRecord(root, incomplete); err == nil || !strings.Contains(err.Error(), "missing its failure") {
		t.Fatalf("incomplete failure metadata error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, "pair-001-baseline-treatment", "baseline", "trace.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFixtureReplicated(root); err == nil || !strings.Contains(err.Error(), "pair") {
		t.Fatalf("missing trace error = %v", err)
	}
}

func TestRunFixtureReplicatedHandlesCancellationAndCaptureContract(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	base := FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: filepath.Join(t.TempDir(), "cancelled"), Pairs: 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runFixtureReplicatedWith(ctx, base, func(string, string, []string, string) (CaptureSummary, error) {
		t.Fatal("capture ran after cancellation")
		return CaptureSummary{}, nil
	}); err == nil {
		t.Fatal("cancelled replication succeeded")
	}
	if summary, err := VerifyFixtureReplicated(base.OutputDir); err != nil || summary.Outcome != portabletrace.ReplicationUnknown || summary.CompletedPairs != 0 {
		t.Fatalf("cancelled receipt = %#v, err=%v", summary, err)
	}

	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	identityRoot := filepath.Join(t.TempDir(), "identity")
	if err := runFixtureReplicatedWith(context.Background(), FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: identityRoot, Pairs: 1}, func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: strings.Repeat("d", 64), Trace: writeFixtureTrace(t, outputPath, "baseline", portabletrace.Complete)}, nil
	}); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("procedure identity error = %v", err)
	}
	missingTraceRoot := filepath.Join(t.TempDir(), "missing-trace")
	if err := runFixtureReplicatedWith(context.Background(), FixtureReplicationInput{ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: missingTraceRoot, Pairs: 1}, func(_, _ string, _ []string, _ string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256}, nil
	}); err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("missing trace error = %v", err)
	}
	invalidCandidate := base
	invalidCandidate.CandidateID = "unsafe"
	invalidCandidate.OutputDir = filepath.Join(t.TempDir(), "invalid-candidate")
	if err := runFixtureReplicatedWith(context.Background(), invalidCandidate, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }); err == nil || !strings.Contains(err.Error(), "candidate") {
		t.Fatalf("invalid candidate error = %v", err)
	}
	if err := RunFixtureReplicated(context.Background(), FixtureReplicationInput{}); err == nil || !strings.Contains(err.Error(), "paths") {
		t.Fatalf("RunFixtureReplicated() invalid input = %v", err)
	}
}

func TestValidateFixtureReplicationRecordRejectsMetadata(t *testing.T) {
	root := t.TempDir()
	pairs := []ReplicatedPairRecord{
		{Pair: 1, Order: portabletrace.OrderBaselineTreatment, Directory: "pair-001-baseline-treatment", FirstSession: "baseline", SecondSession: "treatment", Status: BrowserReplicationStatusComplete},
		{Pair: 1, Order: portabletrace.OrderTreatmentBaseline, Directory: "pair-001-treatment-baseline", FirstSession: "treatment", SecondSession: "baseline", Status: BrowserReplicationStatusComplete},
	}
	for _, pair := range pairs {
		if err := os.Mkdir(filepath.Join(root, pair.Directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	valid := ReplicatedRunRecord{
		SchemaVersion: BrowserReplicationSchemaVersion, Adapter: BrowserReplicationAdapter, AdapterVersion: BrowserReplicationAdapterVersion,
		ProcedureSHA256: strings.Repeat("a", 64), Scope: "outbound", PairsPerOrder: 1,
		ResetPolicy: BrowserReplicationResetPolicy, Status: BrowserReplicationStatusComplete, CompletedPairs: 2, Pairs: pairs,
	}
	tests := []struct {
		name   string
		mutate func(*ReplicatedRunRecord)
	}{
		{name: "schema", mutate: func(record *ReplicatedRunRecord) { record.SchemaVersion = BrowserReplicationLegacySchemaVersion }},
		{name: "adapter", mutate: func(record *ReplicatedRunRecord) { record.Adapter = "other" }},
		{name: "version", mutate: func(record *ReplicatedRunRecord) { record.AdapterVersion = BrowserReplicationLegacyAdapterVersion }},
		{name: "procedure", mutate: func(record *ReplicatedRunRecord) { record.ProcedureSHA256 = "bad" }},
		{name: "scope", mutate: func(record *ReplicatedRunRecord) { record.Scope = "storage" }},
		{name: "pairs", mutate: func(record *ReplicatedRunRecord) { record.PairsPerOrder = 0 }},
		{name: "reset", mutate: func(record *ReplicatedRunRecord) { record.ResetPolicy = "none" }},
		{name: "status", mutate: func(record *ReplicatedRunRecord) { record.Status = "running" }},
		{name: "completed", mutate: func(record *ReplicatedRunRecord) { record.CompletedPairs = 3 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			test.mutate(&record)
			if err := validateFixtureReplicationRecord(root, record); err == nil {
				t.Fatal("validateFixtureReplicationRecord() accepted invalid metadata")
			}
		})
	}
	legacy := valid
	legacy.SchemaVersion = BrowserReplicationLegacySchemaVersion
	legacy.AdapterVersion = BrowserReplicationLegacyAdapterVersion
	if err := validateFixtureReplicationRecord(root, legacy); err != nil {
		t.Fatalf("validateFixtureReplicationRecord() rejected readable legacy metadata: %v", err)
	}
	legacyRoot := t.TempDir()
	for _, pair := range legacy.Pairs {
		pairRoot := filepath.Join(legacyRoot, pair.Directory)
		baselineTrace := filepath.Join(pairRoot, "baseline", "trace.json")
		treatmentTrace := filepath.Join(pairRoot, "treatment", "trace.json")
		writeFixtureTrace(t, baselineTrace, "baseline", portabletrace.Complete)
		writeFixtureTrace(t, treatmentTrace, "treatment", portabletrace.Complete)
		if _, err := portabletrace.SaveSessionPair(baselineTrace, treatmentTrace, filepath.Join(pairRoot, "baseline", "session.json"), filepath.Join(pairRoot, "treatment", "session.json"), portabletrace.SessionPairInput{
			Adapter: BrowserReplicationAdapter, AdapterVersion: BrowserReplicationLegacyAdapterVersion, ProcedureSHA256: legacy.ProcedureSHA256, Scope: legacy.Scope, Order: pair.Order,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeFixtureReplicationRecord(legacyRoot, legacy); err != nil {
		t.Fatalf("write legacy receipt: %v", err)
	}
	legacySummary, err := VerifyFixtureReplicated(legacyRoot)
	if err != nil || legacySummary.Outcome != portabletrace.ReplicatedChange || legacySummary.EvidenceState != evidence.Observed {
		t.Fatalf("legacy end-to-end verification = %#v, %v", legacySummary, err)
	}
	if _, err := VerifyFixtureReplicated(filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("missing root error = %v", err)
	}
}

func writeFixtureTrace(t *testing.T, path, variant, completeness string) portabletrace.VerificationSummary {
	t.Helper()
	events := []portabletrace.Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}}
	if variant == "treatment" {
		events[0].Fields = []string{"account-id", "region"}
	}
	data, err := json.Marshal(portabletrace.Document{SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: completeness, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := portabletrace.Verify(path)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}
