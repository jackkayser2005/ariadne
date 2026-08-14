package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunReplicatedRecordsBothOrdersAndSafeMetadata(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	program := replicationTestProgram(t)
	var invocations [][]string
	capture := func(procedurePath, programPath string, args []string, outputPath string) (CaptureSummary, error) {
		if procedurePath == "" || programPath != program || len(args) != 3 || args[0] != "--mode" || args[1] != "secret" {
			t.Fatalf("capture invocation = %q, %q, %#v", procedurePath, programPath, args)
		}
		invocations = append(invocations, slices.Clone(args))
		variant := args[len(args)-1]
		if variant != "baseline-value" && variant != "treatment-value" {
			t.Fatalf("controlled argument = %q", variant)
		}
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, variant, portabletrace.Complete)}, nil
	}
	root := filepath.Join(t.TempDir(), "replicated")
	input := ReplicationInput{
		ProcedurePath: procedurePath,
		ProgramPath:   program,
		SharedArgs:    []string{"--mode", "secret"},
		BaselineArg:   "baseline-value",
		TreatmentArg:  "treatment-value",
		OutputDir:     root,
		Pairs:         2,
	}
	if err := runReplicatedWith(context.Background(), input, capture, func(string) (string, error) { return strings.Repeat("a", 64), nil }); err != nil {
		t.Fatal(err)
	}
	wantInvocations := [][]string{
		{"--mode", "secret", "baseline-value"},
		{"--mode", "secret", "treatment-value"},
		{"--mode", "secret", "treatment-value"},
		{"--mode", "secret", "baseline-value"},
		{"--mode", "secret", "baseline-value"},
		{"--mode", "secret", "treatment-value"},
		{"--mode", "secret", "treatment-value"},
		{"--mode", "secret", "baseline-value"},
	}
	if !reflect.DeepEqual(invocations, wantInvocations) {
		t.Fatalf("capture order = %#v, want %#v", invocations, wantInvocations)
	}
	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != portabletrace.ReplicatedChange || summary.EvidenceState != evidence.Observed || summary.Pairs != 4 || summary.CompletedPairs != 4 || summary.ChangedPairs != 4 || summary.NoChangePairs != 0 || summary.UnknownPairs != 0 || summary.ResetPolicy != ProxyReplicationResetPolicy || !summary.ConditionValuesWithheld || summary.ControlledArgumentCount != 1 || !portabletrace.ValidSHA256(summary.ExecutionIdentitySHA256) {
		t.Fatalf("replication summary = %#v", summary)
	}
	for _, pair := range summary.PairSummaries {
		if pair.Outcome != "changed" || pair.EvidenceState != evidence.Observed || pair.Differences != 1 || pair.Unknowns != 0 || !portabletrace.ValidSHA256(pair.PairSHA256) {
			t.Fatalf("pair summary = %#v", pair)
		}
	}
	receipt, err := os.ReadFile(filepath.Join(root, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"baseline-value", "treatment-value", "secret", program, "example.com:443"} {
		if strings.Contains(string(receipt), secret) {
			t.Fatalf("receipt exposed %q: %s", secret, receipt)
		}
	}
	if strings.Contains(string(receipt), "procedure_sha256") {
		t.Fatalf("replication receipt exposed a deterministic procedure identity: %s", receipt)
	}
	if strings.Contains(string(receipt), "condition_values") && !strings.Contains(string(receipt), "condition_values_withheld") {
		t.Fatalf("receipt did not retain the explicit withholding marker: %s", receipt)
	}
}

func TestVerifyReplicatedPreservesPartialOutcomeState(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "partial")
	capture := func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, "same", portabletrace.Partial)}, nil
	}
	if err := runReplicatedWith(context.Background(), ReplicationInput{
		ProcedurePath: procedurePath,
		ProgramPath:   replicationTestProgram(t),
		BaselineArg:   "baseline",
		TreatmentArg:  "treatment",
		OutputDir:     root,
		Pairs:         1,
	}, capture, func(string) (string, error) { return strings.Repeat("b", 64), nil }); err != nil {
		t.Fatal(err)
	}
	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != portabletrace.NoChangeObserved || summary.EvidenceState != evidence.Unknown || summary.NoChangePairs != 2 || summary.UnknownPairs != 0 {
		t.Fatalf("partial summary = %#v", summary)
	}
	for _, pair := range summary.PairSummaries {
		if pair.Outcome != "same" || pair.EvidenceState != evidence.Unknown || pair.Unknowns != 0 {
			t.Fatalf("partial pair = %#v", pair)
		}
	}
}

func TestVerifyReplicatedClassifiesMixedAndUnknown(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	program := replicationTestProgram(t)
	root := filepath.Join(t.TempDir(), "mixed")
	capture := func(_, _ string, args []string, outputPath string) (CaptureSummary, error) {
		variant := "same"
		if strings.Contains(outputPath, "pair-001") {
			variant = args[len(args)-1]
		}
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, variant, portabletrace.Complete)}, nil
	}
	if err := runReplicatedWith(context.Background(), ReplicationInput{ProcedurePath: procedurePath, ProgramPath: program, BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: root, Pairs: 2}, capture, func(string) (string, error) { return strings.Repeat("c", 64), nil }); err != nil {
		t.Fatal(err)
	}
	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != portabletrace.MixedInconsistent || summary.EvidenceState != evidence.Observed || summary.ChangedPairs != 2 || summary.NoChangePairs != 2 || summary.UnknownPairs != 0 {
		t.Fatalf("mixed summary = %#v", summary)
	}

	failedRoot := filepath.Join(t.TempDir(), "failed")
	calls := 0
	failingCapture := func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
		calls++
		if calls == 2 {
			return CaptureSummary{}, errors.New("capture failed safely")
		}
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, "baseline", portabletrace.Complete)}, nil
	}
	err = runReplicatedWith(context.Background(), ReplicationInput{ProcedurePath: procedurePath, ProgramPath: program, BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: failedRoot, Pairs: 1}, failingCapture, func(string) (string, error) { return strings.Repeat("d", 64), nil })
	if err == nil || !strings.Contains(err.Error(), "capture failed safely") {
		t.Fatalf("failure = %v", err)
	}
	failedSummary, err := VerifyReplicated(failedRoot)
	if err != nil {
		t.Fatal(err)
	}
	if failedSummary.Outcome != portabletrace.ReplicationUnknown || failedSummary.EvidenceState != evidence.Unknown || failedSummary.CompletedPairs != 0 || failedSummary.UnknownPairs != 2 {
		t.Fatalf("failed summary = %#v", failedSummary)
	}
}

func TestRunReplicatedRejectsUnsafeInputsAndCancellation(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	program := replicationTestProgram(t)
	base := ReplicationInput{ProcedurePath: procedurePath, ProgramPath: program, BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: filepath.Join(t.TempDir(), "out"), Pairs: 1}
	tests := []struct {
		name  string
		input ReplicationInput
		want  string
	}{
		{name: "missing paths", input: ReplicationInput{ProgramPath: program, BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: "out", Pairs: 1}, want: "paths"},
		{name: "pair count", input: func() ReplicationInput { value := base; value.Pairs = 9; return value }(), want: "between"},
		{name: "same condition", input: func() ReplicationInput { value := base; value.BaselineArg = value.TreatmentArg; return value }(), want: "differ"},
		{name: "relative program", input: func() ReplicationInput { value := base; value.ProgramPath = "program"; return value }(), want: "absolute"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := runReplicatedWith(context.Background(), test.input, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, func(string) (string, error) { return strings.Repeat("e", 64), nil }); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runReplicatedWith() error = %v, want %q", err, test.want)
			}
		})
	}
	if err := runReplicatedWith(context.Background(), base, nil, func(string) (string, error) { return strings.Repeat("f", 64), nil }); err == nil || !strings.Contains(err.Error(), "capture") {
		t.Fatalf("nil capture error = %v", err)
	}
	if err := runReplicatedWith(context.Background(), base, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, nil); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("nil identity error = %v", err)
	}
	if err := runReplicatedWith(context.Background(), base, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, func(string) (string, error) { return "bad", nil }); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("bad identity error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := base
	cancelled.OutputDir = filepath.Join(t.TempDir(), "cancelled")
	if err := runReplicatedWith(ctx, cancelled, func(string, string, []string, string) (CaptureSummary, error) {
		t.Fatal("capture ran after cancellation")
		return CaptureSummary{}, nil
	}, func(string) (string, error) { return strings.Repeat("1", 64), nil }); err == nil {
		t.Fatal("cancelled replication succeeded")
	}
	if summary, err := VerifyReplicated(cancelled.OutputDir); err != nil || summary.Outcome != portabletrace.ReplicationUnknown || summary.CompletedPairs != 0 {
		t.Fatalf("cancelled receipt = %#v, err=%v", summary, err)
	}
	existing := base
	existing.OutputDir = filepath.Join(t.TempDir(), "existing")
	if err := os.MkdirAll(existing.OutputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runReplicatedWith(context.Background(), existing, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, func(string) (string, error) { return strings.Repeat("2", 64), nil }); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("existing output error = %v", err)
	}
	if err := RunReplicated(context.Background(), ReplicationInput{}); err == nil || !strings.Contains(err.Error(), "paths") {
		t.Fatalf("RunReplicated() invalid input = %v", err)
	}
	stagedValidation := base
	stagedValidation.Pairs = maxProxyReplicationPairs + 1
	if err := RunReplicated(context.Background(), stagedValidation); err == nil || !strings.Contains(err.Error(), "between") {
		t.Fatalf("RunReplicated() staged validation = %v", err)
	}
}

func TestRunReplicatedHandlesIdentityAndPairFailures(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	base := ReplicationInput{ProcedurePath: procedurePath, ProgramPath: replicationTestProgram(t), BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: filepath.Join(t.TempDir(), "out"), Pairs: 1}
	validIdentity := func(string) (string, error) { return strings.Repeat("4", 64), nil }
	if err := runReplicatedWith(nil, base, func(_, _ string, _ []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, "same", portabletrace.Complete)}, nil
	}, validIdentity); err != nil {
		t.Fatal(err)
	}
	wrongProcedure := base
	wrongProcedure.OutputDir = filepath.Join(t.TempDir(), "wrong-procedure")
	if err := runReplicatedWith(context.Background(), wrongProcedure, func(_, _ string, _ []string, _ string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: strings.Repeat("5", 64)}, nil
	}, validIdentity); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("wrong procedure identity = %v", err)
	}
	badTrace := base
	badTrace.OutputDir = filepath.Join(t.TempDir(), "bad-trace")
	if err := runReplicatedWith(context.Background(), badTrace, func(_, _ string, _ []string, _ string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256}, nil
	}, validIdentity); err == nil || !strings.Contains(err.Error(), "trace identity") {
		t.Fatalf("bad trace identity = %v", err)
	}
	missingPair := base
	missingPair.OutputDir = filepath.Join(t.TempDir(), "missing-pair")
	if err := runReplicatedWith(context.Background(), missingPair, func(_, _ string, _ []string, _ string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: portabletrace.VerificationSummary{Completeness: portabletrace.Complete, TraceSHA256: strings.Repeat("6", 64)}}, nil
	}, validIdentity); err == nil || !strings.Contains(err.Error(), "proxy replication pair") {
		t.Fatalf("missing pair files = %v", err)
	}
	identityError := base
	identityError.OutputDir = filepath.Join(t.TempDir(), "identity-error")
	if err := runReplicatedWith(context.Background(), identityError, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, func(string) (string, error) { return "", errors.New("hash failed") }); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("identity error = %v", err)
	}
	procedureError := base
	procedureError.ProcedurePath = filepath.Join(t.TempDir(), "missing-procedure.json")
	procedureError.OutputDir = filepath.Join(t.TempDir(), "procedure-error")
	if err := runReplicatedWith(context.Background(), procedureError, func(string, string, []string, string) (CaptureSummary, error) { return CaptureSummary{}, nil }, validIdentity); err == nil || !strings.Contains(err.Error(), "procedure") {
		t.Fatalf("procedure error = %v", err)
	}
}

func TestVerifyReplicatedClassifiesPartialDifferenceAsUnknown(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "partial-difference")
	capture := func(_, _ string, args []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, args[len(args)-1], portabletrace.Partial)}, nil
	}
	if err := runReplicatedWith(context.Background(), ReplicationInput{ProcedurePath: procedurePath, ProgramPath: replicationTestProgram(t), BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: root, Pairs: 1}, capture, func(string) (string, error) { return strings.Repeat("7", 64), nil }); err != nil {
		t.Fatal(err)
	}
	summary, err := VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != portabletrace.ReplicationUnknown || summary.EvidenceState != evidence.Unknown || summary.UnknownPairs != 2 {
		t.Fatalf("partial difference summary = %#v", summary)
	}
}

func TestVerifyReplicatedRejectsTamperingAndBounds(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "replicated")
	program := replicationTestProgram(t)
	capture := func(_, _ string, args []string, outputPath string) (CaptureSummary, error) {
		return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: writeReplicationTrace(t, outputPath, args[len(args)-1], portabletrace.Complete)}, nil
	}
	input := ReplicationInput{ProcedurePath: procedurePath, ProgramPath: program, BaselineArg: "baseline", TreatmentArg: "treatment", OutputDir: root, Pairs: 1}
	if err := runReplicatedWith(context.Background(), input, capture, func(string) (string, error) { return strings.Repeat("3", 64), nil }); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, "replication.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "unknown field", data: []byte(strings.Replace(string(receipt), "{\n", "{\n  \"extra\":true,\n", 1)), want: "invalid"},
		{name: "duplicate", data: []byte(strings.Replace(string(receipt), "\"schema_version\": 1,", "\"schema_version\": 1,\n  \"schema_version\": 1,", 1)), want: "invalid"},
		{name: "trailing", data: append(append([]byte(nil), receipt...), []byte("{}")...), want: "trailing"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			if err := os.WriteFile(receiptPath, mutation.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyReplicated(root); err == nil || !strings.Contains(err.Error(), mutation.want) {
				t.Fatalf("VerifyReplicated() error = %v, want %q", err, mutation.want)
			}
			if err := os.WriteFile(receiptPath, receipt, 0o600); err != nil {
				t.Fatal(err)
			}
		})
	}
	record, _, err := readReplicationRecord(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeReplicationRecord(root, record); err == nil || !strings.Contains(err.Error(), "create") {
		t.Fatalf("exclusive receipt write error = %v", err)
	}
	if _, _, err := readReplicationRecord(""); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("empty receipt error = %v", err)
	}
	if err := os.Remove(filepath.Join(root, "pair-001-baseline-treatment", "baseline", "trace.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyReplicated(root); err == nil || !strings.Contains(err.Error(), "pair file") {
		t.Fatalf("missing pair file error = %v", err)
	}
	if _, err := readReplicationFile(filepath.Join(root, "missing"), 10); err == nil {
		t.Fatal("readReplicationFile() accepted missing file")
	}
	if _, err := hashExecutable(filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("hashExecutable() accepted missing file")
	}
	digest, err := hashExecutable(replicationTestProgram(t))
	if err != nil || !portabletrace.ValidSHA256(digest) {
		t.Fatalf("hashExecutable() = %q, err = %v", digest, err)
	}
}

func TestValidateReplicationRecordRejectsMetadata(t *testing.T) {
	root := t.TempDir()
	valid := ReplicatedRunRecord{
		SchemaVersion: ProxyReplicationSchemaVersion, Adapter: Adapter, AdapterVersion: AdapterVersion,
		Scope: "outbound", PairsPerOrder: 1,
		ResetPolicy: ProxyReplicationResetPolicy, ControlledArgumentPosition: "final", ControlledArgumentCount: 1,
		ConditionValuesWithheld: true, ExecutionIdentitySHA256: strings.Repeat("b", 64), Status: ProxyReplicationStatusComplete,
		CompletedPairs: 0,
	}
	for _, mutate := range []func(*ReplicatedRunRecord){
		func(value *ReplicatedRunRecord) { value.SchemaVersion = 2 },
		func(value *ReplicatedRunRecord) { value.Adapter = "other" },
		func(value *ReplicatedRunRecord) { value.AdapterVersion = 2 },
		func(value *ReplicatedRunRecord) { value.Scope = "storage" },
		func(value *ReplicatedRunRecord) { value.PairsPerOrder = 0 },
		func(value *ReplicatedRunRecord) { value.ResetPolicy = "none" },
		func(value *ReplicatedRunRecord) { value.ControlledArgumentPosition = "first" },
		func(value *ReplicatedRunRecord) { value.ControlledArgumentCount = 2 },
		func(value *ReplicatedRunRecord) { value.ConditionValuesWithheld = false },
		func(value *ReplicatedRunRecord) { value.ExecutionIdentitySHA256 = "bad" },
		func(value *ReplicatedRunRecord) { value.Status = "running" },
		func(value *ReplicatedRunRecord) { value.CompletedPairs = 1 },
		func(value *ReplicatedRunRecord) { value.Pairs = nil },
	} {
		value := valid
		mutate(&value)
		if err := validateReplicationRecord(root, value); err == nil {
			t.Fatalf("validateReplicationRecord() accepted %#v", value)
		}
	}
	if _, err := VerifyReplicated(filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("missing root error = %v", err)
	}
	incompleteRoot := filepath.Join(root, "incomplete")
	if err := os.MkdirAll(filepath.Join(incompleteRoot, "pair-001-baseline-treatment"), 0o700); err != nil {
		t.Fatal(err)
	}
	incomplete := valid
	incomplete.Status = ProxyReplicationStatusIncomplete
	incomplete.CompletedPairs = 0
	incomplete.FailurePair = 1
	incomplete.FailureOrder = ""
	incomplete.Pairs = []ReplicatedPairRecord{{Pair: 1, Order: portabletrace.OrderBaselineTreatment, Directory: "pair-001-baseline-treatment", FirstSession: portabletrace.RoleBaseline, SecondSession: portabletrace.RoleTreatment, Status: ProxyReplicationStatusIncomplete}}
	if err := validateReplicationRecord(incompleteRoot, incomplete); err == nil || !strings.Contains(err.Error(), "missing its failure") {
		t.Fatalf("incomplete failure metadata = %v", err)
	}
	fileRoot := filepath.Join(root, "file-pair")
	if err := os.MkdirAll(fileRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fileRoot, "pair-001-baseline-treatment"), []byte("not-a-directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	incomplete.FailureOrder = portabletrace.OrderBaselineTreatment
	if err := validateReplicationRecord(fileRoot, incomplete); err == nil || !strings.Contains(err.Error(), "pair directory") {
		t.Fatalf("file pair directory metadata = %v", err)
	}
	completeRoot := filepath.Join(root, "complete-metadata")
	for _, directory := range []string{"pair-001-baseline-treatment", "pair-001-treatment-baseline"} {
		if err := os.MkdirAll(filepath.Join(completeRoot, directory), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	complete := valid
	complete.CompletedPairs = 2
	complete.Pairs = []ReplicatedPairRecord{
		{Pair: 1, Order: portabletrace.OrderBaselineTreatment, Directory: "pair-001-baseline-treatment", FirstSession: portabletrace.RoleBaseline, SecondSession: portabletrace.RoleTreatment, Status: ProxyReplicationStatusComplete, BaselineTraceSHA256: strings.Repeat("c", 64), TreatmentTraceSHA256: strings.Repeat("d", 64), BaselineSessionSHA256: strings.Repeat("e", 64), TreatmentSessionSHA256: strings.Repeat("f", 64), PairSHA256: strings.Repeat("0", 64)},
		{Pair: 1, Order: portabletrace.OrderTreatmentBaseline, Directory: "pair-001-treatment-baseline", FirstSession: portabletrace.RoleTreatment, SecondSession: portabletrace.RoleBaseline, Status: ProxyReplicationStatusComplete, BaselineTraceSHA256: strings.Repeat("1", 64), TreatmentTraceSHA256: strings.Repeat("2", 64), BaselineSessionSHA256: strings.Repeat("3", 64), TreatmentSessionSHA256: strings.Repeat("4", 64), PairSHA256: strings.Repeat("5", 64)},
	}
	for _, mutate := range []func(*ReplicatedRunRecord){
		func(value *ReplicatedRunRecord) { value.Pairs[0].Status = "running" },
		func(value *ReplicatedRunRecord) { value.Pairs[0].BaselineTraceSHA256 = "bad" },
		func(value *ReplicatedRunRecord) { value.CompletedPairs = 1 },
		func(value *ReplicatedRunRecord) { value.FailurePair = 1 },
		func(value *ReplicatedRunRecord) { value.Pairs[1].Directory = "other" },
	} {
		value := complete
		value.Pairs = append([]ReplicatedPairRecord(nil), complete.Pairs...)
		mutate(&value)
		if err := validateReplicationRecord(completeRoot, value); err == nil {
			t.Fatalf("validateReplicationRecord() accepted complete mutation %#v", value)
		}
	}
	if _, err := readReplicationFile(completeRoot, 10); err == nil {
		t.Fatal("readReplicationFile() accepted a directory")
	}
}

func TestClassifyProxyPairsHandlesEmptyAndEvidence(t *testing.T) {
	empty := classifyProxyPairs(nil)
	if empty.Outcome != portabletrace.ReplicationUnknown || empty.EvidenceState != evidence.Unknown {
		t.Fatalf("empty classification = %#v", empty)
	}
	classification := classifyProxyPairs([]ReplicatedPairSummary{
		{Outcome: "changed", EvidenceState: evidence.Observed},
		{Outcome: "same", EvidenceState: evidence.Unknown},
	})
	if classification.Outcome != portabletrace.MixedInconsistent || classification.EvidenceState != evidence.Unknown || classification.ChangedPairs != 1 || classification.NoChangePairs != 1 {
		t.Fatalf("evidence classification = %#v", classification)
	}
}

func TestStageExecutableBindsTheRunToItsCopiedBytes(t *testing.T) {
	program := replicationTestProgram(t)
	staged, digest, cleanup, err := stageExecutable(program)
	if err != nil {
		t.Fatal(err)
	}
	if staged == program || !portabletrace.ValidSHA256(digest) {
		cleanup()
		t.Fatalf("staged executable = %q, digest = %q", staged, digest)
	}
	stagedDigest, err := hashExecutable(staged)
	if err != nil || stagedDigest != digest {
		cleanup()
		t.Fatalf("staged digest = %q, err = %v; want %q", stagedDigest, err, digest)
	}
	cleanup()
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatalf("staged executable remains after cleanup: %v", err)
	}
	if _, _, _, err := stageExecutable("relative-program"); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("stageExecutable() accepted relative path: %v", err)
	}
	if _, _, _, err := stageExecutable(t.TempDir()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("stageExecutable() accepted a directory: %v", err)
	}
}

func replicationTestProgram(t *testing.T) string {
	t.Helper()
	program, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func writeReplicationTrace(t *testing.T, path, variant, completeness string) portabletrace.VerificationSummary {
	t.Helper()
	fields := []string{"region"}
	if variant == "treatment-value" || variant == "treatment" {
		fields = []string{"account-id", "region"}
	}
	if variant == "same" {
		fields = []string{"unknown"}
	}
	document := portabletrace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  completeness,
		Events: []portabletrace.Event{{
			Source: "proxy", Channel: "network", Kind: "request", Destination: "first-party", Fields: fields,
		}},
	}
	data, err := json.Marshal(document)
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
