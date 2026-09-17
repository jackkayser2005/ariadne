package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunFixtureMinimizationUsesCriterionAndBindsCandidate(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	planPath := writeBrowserLadderPlan(t, testBrowserLadderPlan())
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	var candidates []string
	fakeRun := func(ctx context.Context, input FixtureReplicationInput) error {
		if input.CandidateID != BrowserFixtureOmittedCandidate || input.Pairs != 1 || input.DriverPath != "driver" {
			t.Fatalf("fixture minimization input = %#v", input)
		}
		return runFixtureReplicatedWith(ctx, input, func(procedurePath, driverPath string, args []string, outputPath string) (CaptureSummary, error) {
			if procedurePath != input.ProcedurePath || driverPath != input.DriverPath {
				t.Fatalf("capture paths = %q, %q", procedurePath, driverPath)
			}
			variant := argumentValue(args, "--fixture-variant")
			candidate := argumentValue(args, "--fixture-candidate")
			candidates = append(candidates, variant+":"+candidate)
			fields := []string{"region", "session-id"}
			if candidate == BrowserFixtureReferenceCandidate {
				fields = append(fields, "account-id")
			}
			return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeBrowserCriterionTrace(t, outputPath, fields)}, nil
		})
	}
	root := filepath.Join(t.TempDir(), "browser-minimization")
	if err := runFixtureMinimizationWith(context.Background(), FixtureMinimizationInput{
		PlanPath:      planPath,
		ProcedurePath: procedurePath,
		DriverPath:    "driver",
		DriverArgs:    []string{"--script"},
		OutputDir:     root,
		Pairs:         1,
	}, fakeRun, VerifyFixtureReplicatedForFunctionality); err != nil {
		t.Fatal(err)
	}
	if want := []string{"baseline:reference", "treatment:omitted", "treatment:omitted", "baseline:reference"}; !slices.Equal(candidates, want) {
		t.Fatalf("candidate order = %#v, want %#v", candidates, want)
	}
	summary, err := VerifyFixtureMinimization(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SelectedCandidate != BrowserFixtureOmittedCandidate || summary.SelectionState != minimize.SelectionSelected || string(summary.EvidenceState) != "observed" || summary.CandidateResults[0].Outcome != portabletrace.NoChangeObserved || summary.CandidateResults[0].Classification != minimize.CandidateSufficient {
		t.Fatalf("minimization summary = %#v", summary)
	}
	childRoot := filepath.Join(root, summary.CandidateResults[0].Directory)
	ordinary, err := VerifyFixtureReplicated(childRoot)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Outcome != portabletrace.ReplicatedChange || string(ordinary.EvidenceState) != "observed" {
		t.Fatalf("ordinary browser outcome = %#v", ordinary)
	}
	receipt, err := os.ReadFile(filepath.Join(root, "minimization.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"session_id", "account_id", "http://", "https://"} {
		if strings.Contains(string(receipt), secret) {
			t.Fatalf("portable minimization receipt disclosed %q: %s", secret, receipt)
		}
	}
}

func TestRunFixtureMinimizationBoundaryFailures(t *testing.T) {
	if err := RunFixtureMinimization(context.Background(), FixtureMinimizationInput{}); err == nil {
		t.Fatal("RunFixtureMinimization() accepted missing input")
	}
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	planPath := writeBrowserLadderPlan(t, testBrowserLadderPlan())
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	base := FixtureMinimizationInput{PlanPath: planPath, ProcedurePath: procedurePath, DriverPath: "driver", Pairs: 1}
	unknown := browserMinimizationSummary(BrowserFixtureOmittedCandidate, procedureSHA256, portabletrace.ReplicationUnknown, evidence.Unknown, 0, 0, 2)
	unknownRoot := filepath.Join(t.TempDir(), "unknown")
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, unknownRoot), func(context.Context, FixtureReplicationInput) error { return errors.New("controlled run failure") }, func(string) (BrowserReplicationSummary, error) { return unknown, nil }); err != nil {
		t.Fatalf("unknown failed run should be retained safely: %v", err)
	}
	verifyErrorRoot := filepath.Join(t.TempDir(), "verify-error")
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, verifyErrorRoot), func(context.Context, FixtureReplicationInput) error { return nil }, func(string) (BrowserReplicationSummary, error) {
		return BrowserReplicationSummary{}, errors.New("child verify failure")
	}); err == nil || !strings.Contains(err.Error(), "verify") {
		t.Fatalf("child verify failure = %v", err)
	}
	nonUnknownRoot := filepath.Join(t.TempDir(), "non-unknown")
	changed := browserMinimizationSummary(BrowserFixtureOmittedCandidate, procedureSHA256, portabletrace.ReplicatedChange, evidence.Observed, 2, 0, 0)
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, nonUnknownRoot), func(context.Context, FixtureReplicationInput) error { return errors.New("late run failure") }, func(string) (BrowserReplicationSummary, error) { return changed, nil }); err == nil || !strings.Contains(err.Error(), "non-unknown") {
		t.Fatalf("non-unknown failed run = %v", err)
	}
}

func baseWithOutput(input FixtureMinimizationInput, output string) FixtureMinimizationInput {
	input.OutputDir = output
	return input
}

func browserMinimizationSummary(candidate, procedureSHA256 string, outcome portabletrace.ReplicatedOutcome, state evidence.State, changed, noChange, unknown int) BrowserReplicationSummary {
	return BrowserReplicationSummary{
		Adapter: BrowserReplicationAdapter, AdapterVersion: BrowserReplicationAdapterVersion, ProcedureSHA256: procedureSHA256, Scope: "outbound", CandidateID: candidate, ResetPolicy: BrowserReplicationResetPolicy, ReceiptSHA256: strings.Repeat("a", 64),
		Pairs: 2, PairsPerOrder: 1, Outcome: outcome, EvidenceState: state, CompletedPairs: changed + noChange, ChangedPairs: changed, NoChangePairs: noChange, UnknownPairs: unknown,
	}
}

func TestFixtureMinimizationOutputAndContextBoundaries(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	planPath := writeBrowserLadderPlan(t, testBrowserLadderPlan())
	base := FixtureMinimizationInput{PlanPath: planPath, ProcedurePath: procedurePath, DriverPath: "driver", Pairs: 1}
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	outputFile := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(outputFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, outputFile), func(context.Context, FixtureReplicationInput) error { return nil }, VerifyFixtureReplicatedForFunctionality); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("output file error = %v", err)
	}
	invalidParent := filepath.Join(t.TempDir(), "\x00", "output")
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, invalidParent), func(context.Context, FixtureReplicationInput) error { return nil }, VerifyFixtureReplicatedForFunctionality); err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("output parent error = %v", err)
	}
	unknown := browserMinimizationSummary(BrowserFixtureOmittedCandidate, procedureSHA256, portabletrace.ReplicationUnknown, evidence.Unknown, 0, 0, 2)
	nilContextRoot := filepath.Join(t.TempDir(), "nil-context")
	if err := runFixtureMinimizationWith(nil, baseWithOutput(base, nilContextRoot), func(ctx context.Context, _ FixtureReplicationInput) error {
		if ctx == nil {
			t.Fatal("minimization runner received nil context")
		}
		return nil
	}, func(string) (BrowserReplicationSummary, error) { return unknown, nil }); err != nil {
		t.Fatalf("nil context minimization = %v", err)
	}
	provenanceRoot := filepath.Join(t.TempDir(), "provenance")
	wrong := unknown
	wrong.CandidateID = BrowserFixtureReferenceCandidate
	if err := runFixtureMinimizationWith(context.Background(), baseWithOutput(base, provenanceRoot), func(context.Context, FixtureReplicationInput) error { return nil }, func(string) (BrowserReplicationSummary, error) { return wrong, nil }); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("provenance mismatch = %v", err)
	}
}

func TestVerifyFixtureMinimizationRejectsCandidateTampering(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	planPath := writeBrowserLadderPlan(t, testBrowserLadderPlan())
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "browser-minimization")
	fakeRun := func(ctx context.Context, input FixtureReplicationInput) error {
		return runFixtureReplicatedWith(ctx, input, func(_, _ string, args []string, outputPath string) (CaptureSummary, error) {
			candidate := argumentValue(args, "--fixture-candidate")
			fields := []string{"region", "session-id"}
			if candidate == BrowserFixtureReferenceCandidate {
				fields = append(fields, "account-id")
			}
			return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeBrowserCriterionTrace(t, outputPath, fields)}, nil
		})
	}
	input := FixtureMinimizationInput{PlanPath: planPath, ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: root, Pairs: 1}
	if err := runFixtureMinimizationWith(context.Background(), input, fakeRun, VerifyFixtureReplicatedForFunctionality); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, "candidate-001-omitted", "replication.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var record ReplicatedRunRecord
	if err := json.Unmarshal(receipt, &record); err != nil {
		t.Fatal(err)
	}
	record.CandidateID = BrowserFixtureReferenceCandidate
	tampered, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, append(tampered, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFixtureMinimization(root); err == nil || !strings.Contains(err.Error(), "candidate") {
		t.Fatalf("VerifyFixtureMinimization() error = %v", err)
	}
}

func TestVerifyFixtureMinimizationRejectsFixedPlanTampering(t *testing.T) {
	root := writeVerifiedBrowserMinimization(t)
	receiptPath := filepath.Join(root, "minimization.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary minimize.LadderSummary
	if err := json.Unmarshal(receipt, &summary); err != nil {
		t.Fatal(err)
	}
	summary.Variable = "email"
	tampered, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, append(tampered, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyFixtureMinimization(root); err == nil || !strings.Contains(err.Error(), "fixed fixture plan") {
		t.Fatalf("VerifyFixtureMinimization() error = %v", err)
	}
}

func writeVerifiedBrowserMinimization(t *testing.T) string {
	t.Helper()
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	planPath := writeBrowserLadderPlan(t, testBrowserLadderPlan())
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "browser-minimization")
	fakeRun := func(ctx context.Context, input FixtureReplicationInput) error {
		return runFixtureReplicatedWith(ctx, input, func(_, _ string, args []string, outputPath string) (CaptureSummary, error) {
			candidate := argumentValue(args, "--fixture-candidate")
			fields := []string{"region", "session-id"}
			if candidate == BrowserFixtureReferenceCandidate {
				fields = append(fields, "account-id")
			}
			return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeBrowserCriterionTrace(t, outputPath, fields)}, nil
		})
	}
	input := FixtureMinimizationInput{PlanPath: planPath, ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: root, Pairs: 1}
	if err := runFixtureMinimizationWith(context.Background(), input, fakeRun, VerifyFixtureReplicatedForFunctionality); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestValidateFixtureMinimizationInputRejectsUnsupportedBoundary(t *testing.T) {
	procedurePath := writeProcedure(t, []byte(`{"schema_version":1,"procedure_id":"browser-local-fixture-v1","scope":"outbound","duration_ms":500,"max_events":8}`))
	base := FixtureMinimizationInput{PlanPath: writeBrowserLadderPlan(t, testBrowserLadderPlan()), ProcedurePath: procedurePath, DriverPath: "driver", OutputDir: filepath.Join(t.TempDir(), "out"), Pairs: 1}
	tests := []struct {
		name  string
		input FixtureMinimizationInput
		want  string
	}{
		{name: "missing plan", input: func() FixtureMinimizationInput { value := base; value.PlanPath = ""; return value }(), want: "paths"},
		{name: "pairs", input: func() FixtureMinimizationInput { value := base; value.Pairs = 0; return value }(), want: "between"},
		{name: "driver candidate", input: func() FixtureMinimizationInput {
			value := base
			value.DriverArgs = []string{"--fixture-candidate", "omitted"}
			return value
		}(), want: "controlled"},
		{name: "unsupported plan", input: func() FixtureMinimizationInput {
			value := base
			value.PlanPath = writeBrowserLadderPlan(t, minimize.LadderPlan{SchemaVersion: 1, Name: "other", Variable: "email", ReferenceCandidate: "reference", FunctionalityCriterion: minimize.FunctionalityCriterionAllNonDisclosureFields, Candidates: []string{"reference", "omitted"}})
			return value
		}(), want: "supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := runFixtureMinimizationWith(context.Background(), test.input, func(context.Context, FixtureReplicationInput) error { return nil }, VerifyFixtureReplicatedForFunctionality); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runFixtureMinimizationWith() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := runFixtureMinimizationWith(context.Background(), base, nil, VerifyFixtureReplicatedForFunctionality); err == nil || !strings.Contains(err.Error(), "dependencies") {
		t.Fatalf("nil runner error = %v", err)
	}
}

func testBrowserLadderPlan() minimize.LadderPlan {
	return minimize.LadderPlan{
		SchemaVersion:          minimize.LadderSchemaVersion,
		Name:                   "browser-account-minimize",
		Variable:               "account-id",
		ReferenceCandidate:     BrowserFixtureReferenceCandidate,
		FunctionalityCriterion: BrowserFunctionalityCriterion,
		Candidates:             []string{BrowserFixtureReferenceCandidate, BrowserFixtureOmittedCandidate},
	}
}

func writeBrowserLadderPlan(t *testing.T, plan minimize.LadderPlan) string {
	t.Helper()
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func argumentValue(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func writeBrowserCriterionTrace(t *testing.T, path string, fields []string) portabletrace.VerificationSummary {
	t.Helper()
	data, err := json.Marshal(portabletrace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  portabletrace.Complete,
		Events: []portabletrace.Event{{
			Source:      "browser",
			Channel:     "network",
			Kind:        "request",
			Destination: "analytics",
			Fields:      fields,
		}},
	})
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
