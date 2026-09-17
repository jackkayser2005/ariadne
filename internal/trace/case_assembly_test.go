package trace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestAssembleCaseCreatesVerifiedWorkspace(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion: caseAssemblyPlanSchemaVersion,
		OrderBasis:    "caller",
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceArchive,
			ArtifactPath:      archivePath,
			QuestionRoundPath: archiveRoundPath,
		}},
	})
	outputDir := filepath.Join(root, "assembled")

	summary, err := AssembleCase(planPath, outputDir)
	if err != nil {
		t.Fatalf("AssembleCase() error = %v", err)
	}
	if summary.Entries != 1 || summary.CaseSHA256 == "" || summary.DisclosureRoundSHA256 == "" || summary.CoverageState != evidence.Observed {
		t.Fatalf("assembly summary = %#v", summary)
	}
	if len(summary.Questions) != len(CaseDisclosureQuestions()) {
		t.Fatalf("assembly questions = %#v", summary.Questions)
	}
	if summary.Questions[0].QuestionID != CaseDisclosureQuestionCoverage || summary.Questions[0].Result != caseDisclosureResultComplete || summary.Questions[0].EvidenceState != evidence.Observed {
		t.Fatalf("coverage question = %#v", summary.Questions[0])
	}
	if summary.Questions[1].QuestionID != CaseDisclosureQuestionOverlap || summary.Questions[1].Result != caseDisclosureResultNoOverlap || summary.Questions[1].EvidenceState != evidence.Observed {
		t.Fatalf("overlap question = %#v", summary.Questions[1])
	}

	casePath := filepath.Join(outputDir, "case.json")
	if verified := mustVerifyCase(t, casePath); verified.CaseSHA256 != summary.CaseSHA256 {
		t.Fatalf("verified case summary = %#v", verified)
	}
	roundPath := filepath.Join(outputDir, "disclosure-round.json")
	round, roundSummary, err := ReadCaseDisclosureQuestionRound(roundPath)
	if err != nil {
		t.Fatalf("ReadCaseDisclosureQuestionRound() error = %v", err)
	}
	if roundSummary.RoundSHA256 != summary.DisclosureRoundSHA256 || roundSummary.CaseSHA256 != summary.CaseSHA256 || round.CaseSHA256 != summary.CaseSHA256 {
		t.Fatalf("round summary = %#v, round = %#v", roundSummary, round)
	}
	for _, name := range []string{"case.json", "disclosure-round.json"} {
		data, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), root) || strings.Contains(string(data), archivePath) {
			t.Fatalf("%s exposed local input paths: %s", name, data)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "assembly.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected assembly summary artifact, err = %v", err)
	}
}

func TestVerifyCaseAssemblyRechecksDerivedRound(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion: caseAssemblyPlanSchemaVersion,
		OrderBasis:    "caller",
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceArchive,
			ArtifactPath:      archivePath,
			QuestionRoundPath: archiveRoundPath,
		}},
	})
	outputDir := filepath.Join(root, "assembled")
	want, err := AssembleCase(planPath, outputDir)
	if err != nil {
		t.Fatalf("AssembleCase() error = %v", err)
	}
	got, err := VerifyCaseAssembly(outputDir)
	if err != nil {
		t.Fatalf("VerifyCaseAssembly() error = %v", err)
	}
	if got.CaseSHA256 != want.CaseSHA256 || got.DisclosureRoundSHA256 != want.DisclosureRoundSHA256 || got.CoverageState != want.CoverageState || len(got.Questions) != len(want.Questions) {
		t.Fatalf("verified assembly = %#v, want %#v", got, want)
	}
	roundPath := filepath.Join(outputDir, "disclosure-round.json")
	round, _, err := ReadCaseDisclosureQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	round.Traces++
	for index := range round.Answers {
		round.Answers[index].Traces = round.Traces
	}
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(roundPath, append(data, 10), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyCaseAssembly(outputDir); err == nil || !strings.Contains(err.Error(), "does not match case") {
		t.Fatalf("VerifyCaseAssembly() error = %v, want derived-round mismatch", err)
	}
}
func TestAssembleCasePreservesCrossBoundaryOverlap(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("1", 64)
	androidInput := writeStandaloneArchiveInputWithAdapter(t, root, "android", validArchiveTrace("location"), "android-experiment-001", procedure)
	browserDocument := validArchiveTrace("location")
	browserDocument.Events[0].Source = "browser"
	browserInput := writeStandaloneArchiveInputWithAdapter(t, root, "browser", browserDocument, "browser-redacted-audit", procedure)
	androidArchive, androidRound := saveAssemblyArchive(t, root, "android", androidInput)
	browserArchive, browserRound := saveAssemblyArchive(t, root, "browser", browserInput)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion: caseAssemblyPlanSchemaVersion,
		OrderBasis:    "caller",
		Entries: []CaseAssemblyPlanEntry{
			{Kind: CaseEntryTraceArchive, ArtifactPath: androidArchive, QuestionRoundPath: androidRound},
			{Kind: CaseEntryTraceArchive, ArtifactPath: browserArchive, QuestionRoundPath: browserRound},
		},
	})

	summary, err := AssembleCase(planPath, filepath.Join(root, "assembled"))
	if err != nil {
		t.Fatalf("AssembleCase() error = %v", err)
	}
	if summary.CoverageState != evidence.Observed || summary.Questions[1].Result != caseDisclosureResultOverlap || summary.Questions[1].EvidenceState != evidence.Observed {
		t.Fatalf("cross-boundary summary = %#v", summary)
	}
}

func TestAssembleCasePreservesPartialUnknownSemantics(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("2", 64)
	partialDocument := validArchiveTrace("region")
	partialDocument.Completeness = Partial
	partialInput := writeStandaloneArchiveInputWithAdapter(t, root, "partial", partialDocument, "android-experiment-001", procedure)
	completeDocument := validArchiveTrace("location")
	completeDocument.Events[0].Source = "browser"
	completeInput := writeStandaloneArchiveInputWithAdapter(t, root, "complete", completeDocument, "browser-redacted-audit", procedure)
	partialArchive, partialRound := saveAssemblyArchive(t, root, "partial", partialInput)
	completeArchive, completeRound := saveAssemblyArchive(t, root, "complete", completeInput)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion: caseAssemblyPlanSchemaVersion,
		OrderBasis:    "caller",
		Entries: []CaseAssemblyPlanEntry{
			{Kind: CaseEntryTraceArchive, ArtifactPath: partialArchive, QuestionRoundPath: partialRound},
			{Kind: CaseEntryTraceArchive, ArtifactPath: completeArchive, QuestionRoundPath: completeRound},
		},
	})

	summary, err := AssembleCase(planPath, filepath.Join(root, "assembled"))
	if err != nil {
		t.Fatalf("AssembleCase() error = %v", err)
	}
	if summary.CoverageState != evidence.Unknown || summary.Questions[0].Result != caseDisclosureResultUnknown || summary.Questions[0].EvidenceState != evidence.Unknown {
		t.Fatalf("partial coverage summary = %#v", summary)
	}
	if summary.Questions[1].Result != caseDisclosureResultUnknown || summary.Questions[1].EvidenceState != evidence.Unknown {
		t.Fatalf("partial no-overlap summary = %#v", summary)
	}
}

func TestVerifyCaseAssemblyRejectsUnavailableWorkspace(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"", filepath.Join(root, "missing")} {
		if _, err := VerifyCaseAssembly(path); err == nil {
			t.Fatalf("VerifyCaseAssembly(%q) accepted unavailable workspace", path)
		}
	}
	filePath := filepath.Join(root, "file")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyCaseAssembly(filePath); err == nil {
		t.Fatal("VerifyCaseAssembly() accepted a file as workspace")
	}
	emptyDir := filepath.Join(root, "empty")
	if err := os.Mkdir(emptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyCaseAssembly(emptyDir); err == nil || !strings.Contains(err.Error(), "case is unavailable") {
		t.Fatalf("VerifyCaseAssembly() error = %v, want missing case", err)
	}
}
func TestAssembleCaseFailsAtomically(t *testing.T) {
	root := t.TempDir()
	planPath := writeCaseAssemblyPlan(t, root, validCaseAssemblyPlan())
	outputDir := filepath.Join(root, "missing-input")
	if _, err := AssembleCase(planPath, outputDir); err == nil {
		t.Fatal("AssembleCase() accepted missing child artifacts")
	}
	if _, err := os.Stat(outputDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed assembly left output, err = %v", err)
	}

	existingDir := filepath.Join(root, "existing")
	if err := os.Mkdir(existingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(existingDir, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AssembleCase(planPath, existingDir); err == nil {
		t.Fatal("AssembleCase() overwrote an existing output directory")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing output changed: data=%q err=%v", data, err)
	}

	if _, err := AssembleCase("", filepath.Join(root, "blank-plan")); err == nil {
		t.Fatal("AssembleCase() accepted a blank plan path")
	}
	if _, err := AssembleCase(planPath, ""); err == nil {
		t.Fatal("AssembleCase() accepted a blank output path")
	}
	if _, err := AssembleCase(planPath, filepath.Join(root, "missing", "nested")); err == nil {
		t.Fatal("AssembleCase() created a missing output parent")
	}
}

func TestDecodeCaseAssemblyPlanRejectsMalformedDocuments(t *testing.T) {
	valid := mustAssemblyJSON(t, validCaseAssemblyPlan())
	cases := map[string][]byte{
		"empty":           nil,
		"not object":      []byte(`[]`),
		"unknown field":   append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"extra":true}`)...),
		"duplicate key":   []byte(`{"schema_version":1,"schema_version":1,"order_basis":"caller","entries":[]}`),
		"trailing data":   append(valid, []byte(` {}`)...),
		"invalid utf8":    []byte{0xff},
		"bad schema":      []byte(`{"schema_version":2,"order_basis":"caller","entries":[]}`),
		"bad order":       []byte(`{"schema_version":1,"order_basis":"chronological","entries":[]}`),
		"empty entries":   []byte(`{"schema_version":1,"order_basis":"caller","entries":[]}`),
		"bad kind":        []byte(`{"schema_version":1,"order_basis":"caller","entries":[{"kind":"other","artifact_path":"a","question_round_path":"b"}]}`),
		"blank artifact":  []byte(`{"schema_version":1,"order_basis":"caller","entries":[{"kind":"trace-archive","artifact_path":" ","question_round_path":"b"}]}`),
		"newline path":    []byte(`{"schema_version":1,"order_basis":"caller","entries":[{"kind":"trace-archive","artifact_path":"a\n","question_round_path":"b"}]}`),
		"duplicate entry": mustAssemblyJSON(t, CaseAssemblyPlan{SchemaVersion: 1, OrderBasis: "caller", Entries: []CaseAssemblyPlanEntry{{Kind: CaseEntryTraceArchive, ArtifactPath: "a", QuestionRoundPath: "b"}, {Kind: CaseEntryTraceArchive, ArtifactPath: "a", QuestionRoundPath: "b"}}}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCaseAssemblyPlan(data); err == nil {
				t.Fatal("DecodeCaseAssemblyPlan() accepted malformed input")
			}
		})
	}
	tooLarge := make([]byte, maxCaseAssemblyPlanBytes+1)
	if _, err := DecodeCaseAssemblyPlan(tooLarge); err == nil {
		t.Fatal("DecodeCaseAssemblyPlan() accepted oversized input")
	}
	if _, err := ReadCaseAssemblyPlan(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadCaseAssemblyPlan() accepted a missing file")
	}
}

func TestCaseAssemblySummaryValidation(t *testing.T) {
	valid := CaseAssemblySummary{
		SchemaVersion:         caseAssemblySummarySchemaVersion,
		Entries:               1,
		CaseSHA256:            strings.Repeat("a", 64),
		DisclosureRoundSHA256: strings.Repeat("b", 64),
		CoverageState:         evidence.Observed,
		Questions: []CaseAssemblyQuestionSummary{
			{QuestionID: CaseDisclosureQuestionCoverage, Result: caseDisclosureResultComplete, EvidenceState: evidence.Observed},
			{QuestionID: CaseDisclosureQuestionOverlap, Result: caseDisclosureResultNoOverlap, EvidenceState: evidence.Observed},
		},
	}
	if err := validateCaseAssemblySummary(valid); err != nil {
		t.Fatalf("valid summary rejected: %v", err)
	}
	mutations := []func(*CaseAssemblySummary){
		func(value *CaseAssemblySummary) { value.SchemaVersion = 2 },
		func(value *CaseAssemblySummary) { value.Entries = 0 },
		func(value *CaseAssemblySummary) { value.CaseSHA256 = "bad" },
		func(value *CaseAssemblySummary) { value.CoverageState = evidence.Inferred },
		func(value *CaseAssemblySummary) { value.Questions = nil },
		func(value *CaseAssemblySummary) { value.Questions[0].QuestionID = "unknown" },
		func(value *CaseAssemblySummary) { value.Questions[0].Result = caseDisclosureResultNoOverlap },
		func(value *CaseAssemblySummary) { value.Questions[0].EvidenceState = evidence.Inferred },
	}
	for index, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if err := validateCaseAssemblySummary(candidate); err == nil {
			t.Fatalf("mutation %d accepted", index)
		}
	}
}

func validCaseAssemblyPlan() CaseAssemblyPlan {
	return CaseAssemblyPlan{
		SchemaVersion: caseAssemblyPlanSchemaVersion,
		OrderBasis:    "caller",
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceArchive,
			ArtifactPath:      "missing-archive.json",
			QuestionRoundPath: "missing-round.json",
		}},
	}
}

func writeCaseAssemblyPlan(t *testing.T, root string, plan CaseAssemblyPlan) string {
	t.Helper()
	path := filepath.Join(root, "case-plan.json")
	if err := os.WriteFile(path, mustAssemblyJSON(t, plan), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func saveAssemblyArchive(t *testing.T, root, name string, input ArchiveInput) (string, string) {
	t.Helper()
	archivePath := filepath.Join(root, name+"-archive.json")
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, name+"-archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	return archivePath, roundPath
}

func mustVerifyCase(t *testing.T, path string) CaseVerificationSummary {
	t.Helper()
	summary, err := VerifyCase(path)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func mustAssemblyJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
