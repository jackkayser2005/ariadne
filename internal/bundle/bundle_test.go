package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/analysis"
	"github.com/jackkayser2005/ariadne/internal/collector"
	evidencestate "github.com/jackkayser2005/ariadne/internal/evidence"
)

const (
	baselineObservation  = `{"schema_version":1,"region":"us-east","variant":"standard"}`
	treatmentObservation = `{"schema_version":1,"region":"us-east","variant":"personalized"}`
)

func TestWrite(t *testing.T) {
	runDir := makeRun(t, runOptions{})

	summary, err := Write(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ManifestName != "experiment-001-email" ||
		summary.Differences != 1 ||
		summary.Unknowns != 0 ||
		summary.Question != "Did changing email influence an observed output?" ||
		summary.AnswerState != evidencestate.Observed ||
		summary.ManifestContractSHA256 != strings.Repeat("c", 64) ||
		summary.AriadneRevision != strings.Repeat("b", 40) ||
		summary.RecordedAt != "2026-07-25T12:00:00Z" ||
		summary.AriadneModified {
		t.Fatalf("Write() = %#v", summary)
	}

	evidence, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(evidence, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 7 ||
		document.Question != "Did changing email influence an observed output?" ||
		document.AnswerState != evidencestate.Observed ||
		document.Comparison.SchemaVersion != 5 ||
		len(document.Artifacts) != 6 ||
		len(document.Comparison.Differences) != 1 ||
		len(document.Comparison.Unknowns) != 0 ||
		document.Comparison.Differences[0].Field != "variant" ||
		document.Comparison.Differences[0].Kind != "changed" ||
		!strings.HasPrefix(document.Comparison.Differences[0].ID, "sha256:") ||
		document.Target.AndroidAPI != 35 ||
		document.Target.Architecture != "x86_64" ||
		document.Target.PackageVersionCode != 1 ||
		document.Target.PackageSHA256 != strings.Repeat("a", 64) ||
		document.Target.AriadneRevision != strings.Repeat("b", 40) ||
		document.ManifestContractSHA256 != strings.Repeat("c", 64) {
		t.Fatalf("evidence = %#v", document)
	}

	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, expected := range []string{
		"# Evidence Report",
		"Observed differences: 1",
		"Unknown conclusions: 0",
		"<code>variant</code>",
		"Finding ID: <code>sha256:",
		"Kind: <code>changed</code>",
		"<code>standard</code>",
		"<code>personalized</code>",
		"Verified artifacts: 6",
		"Android API: 35",
		"Architecture: <code>x86_64</code>",
		"Package version code: 1",
		"Package SHA-256: <code>" + strings.Repeat("a", 64) + "</code>",
		"Ariadne revision: <code>" + strings.Repeat("b", 40) + "</code>",
		"Ariadne modified: false",
		"Manifest contract SHA-256: <code>",
		"Question: <code>Did changing email influence an observed output?</code>",
		"Answer state: <code>observed</code>",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("report missing %q:\n%s", expected, text)
		}
	}
	for _, secret := range []string{"baseline@example.invalid", "treatment@example.invalid"} {
		if strings.Contains(string(evidence), secret) || strings.Contains(text, secret) {
			t.Fatalf("bundle exposed persona value %q", secret)
		}
	}
}

func TestFindingIDsAreDigestBacked(t *testing.T) {
	artifacts := []artifact{
		{Path: "baseline/observations/storage.json", SHA256: strings.Repeat("a", 64)},
		{Path: "treatment/observations/storage.json", SHA256: strings.Repeat("b", 64)},
	}
	comparison := analysis.Comparison{
		Differences: []analysis.Difference{{
			Field:     "variant",
			Kind:      "changed",
			Baseline:  "standard",
			Treatment: "personalized",
			State:     evidencestate.Observed,
			Evidence: []string{
				"baseline/observations/storage.json#/variant",
				"treatment/observations/storage.json#/variant",
			},
		}},
		Unknowns: []analysis.Unknown{{
			Field:    "region",
			State:    evidencestate.Unknown,
			Reason:   "capture gap",
			Evidence: []string{"baseline/observations/storage.json#/region"},
		}},
	}
	if err := assignFindingIDs(&comparison, artifacts); err != nil {
		t.Fatal(err)
	}
	differenceID := comparison.Differences[0].ID
	unknownID := comparison.Unknowns[0].ID
	if differenceID == "" || unknownID == "" || differenceID == unknownID {
		t.Fatalf("finding IDs = %q, %q", differenceID, unknownID)
	}
	if strings.Contains(differenceID, "standard") || strings.Contains(differenceID, "personalized") {
		t.Fatalf("difference ID exposed an observed value: %q", differenceID)
	}

	comparison.Differences[0].Baseline = "changed-value"
	comparison.Differences[0].Treatment = "another-value"
	if err := assignFindingIDs(&comparison, artifacts); err != nil {
		t.Fatal(err)
	}
	if comparison.Differences[0].ID != differenceID {
		t.Fatalf("value-only change changed ID: %q != %q", comparison.Differences[0].ID, differenceID)
	}
	artifacts[0].SHA256 = strings.Repeat("c", 64)
	if err := assignFindingIDs(&comparison, artifacts); err != nil {
		t.Fatal(err)
	}
	if comparison.Differences[0].ID == differenceID {
		t.Fatal("artifact digest change did not change finding ID")
	}
}

func TestFindingIDsRejectMissingSources(t *testing.T) {
	comparison := analysis.Comparison{
		Differences: []analysis.Difference{{
			Field:    "variant",
			Kind:     "changed",
			State:    evidencestate.Observed,
			Evidence: []string{"baseline/observations/missing.json#/variant"},
		}},
	}
	if err := assignFindingIDs(&comparison, nil); err == nil || !strings.Contains(err.Error(), "missing artifact") {
		t.Fatalf("assignFindingIDs() error = %v", err)
	}
}

func TestVerifyPreservesOutputs(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	evidenceBefore, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportBefore, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Verify(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Differences != 1 || summary.Unknowns != 0 {
		t.Fatalf("Verify() = %#v", summary)
	}
	evidenceAfter, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportAfter, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(evidenceBefore, evidenceAfter) || !bytes.Equal(reportBefore, reportAfter) {
		t.Fatal("Verify() changed existing outputs")
	}
}

func TestVerifyRequiresRunDirectory(t *testing.T) {
	_, err := Verify(" \t")
	if err == nil || !strings.Contains(err.Error(), "run directory is required") {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyAcceptsLegacyEvidenceSchemas(t *testing.T) {
	for _, test := range []struct {
		name           string
		options        runOptions
		evidenceSchema int
	}{
		{name: "schema-4", options: runOptions{sessionSchemaVersion: 2}, evidenceSchema: 4},
		{name: "schema-6", options: runOptions{}, evidenceSchema: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, test.options)
			document, _, err := buildDocument(runDir, false)
			if err != nil {
				t.Fatal(err)
			}
			evidenceData, err := encodeDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			reportData, err := encodeReport(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeOutputs(runDir, evidenceData, reportData); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(runDir); err != nil {
				t.Fatal(err)
			}
			if document.SchemaVersion != test.evidenceSchema || document.Comparison.SchemaVersion != 4 {
				t.Fatalf("legacy document = %#v", document)
			}
			if len(document.Comparison.Differences) != 1 || document.Comparison.Differences[0].ID != "" {
				t.Fatalf("legacy findings = %#v", document.Comparison.Differences)
			}
		})
	}
}

func TestVerifyRejectsUnsupportedEvidenceSchema(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if err := os.WriteFile(filepath.Join(runDir, "evidence.json"), []byte("{\"schema_version\":8}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Verify(runDir)
	if err == nil || !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestFindReturnsSafeDifference(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	evidenceBefore, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportBefore, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	finding, err := Find(runDir, document.Comparison.Differences[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if finding.Question != document.Question ||
		finding.AnswerState != evidencestate.Observed ||
		finding.Kind != "difference" ||
		finding.Classification != "changed" ||
		finding.Field != "variant" ||
		finding.State != evidencestate.Observed ||
		len(finding.Evidence) != 4 {
		t.Fatalf("Find() = %#v", finding)
	}
	if strings.Contains(strings.Join(finding.Evidence, " "), "standard") ||
		strings.Contains(strings.Join(finding.Evidence, " "), "personalized") {
		t.Fatalf("Find() exposed observed values: %#v", finding)
	}
	evidenceAfter, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportAfter, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(evidenceBefore, evidenceAfter) || !bytes.Equal(reportBefore, reportAfter) {
		t.Fatal("Find() changed outputs")
	}
}

func TestFindReturnsSafeUnknown(t *testing.T) {
	runDir := makeStorageFailureRun(t, "")
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	finding, err := Find(runDir, document.Comparison.Unknowns[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if finding.AnswerState != evidencestate.Unknown ||
		finding.Kind != "unknown" ||
		finding.Classification != "" ||
		finding.State != evidencestate.Unknown ||
		finding.Reason != treatmentStorageObservationUnknownReason ||
		len(finding.Evidence) != 3 {
		t.Fatalf("Find() = %#v", finding)
	}
}

func TestSafeUnknownReasonIsVerifierOwned(t *testing.T) {
	if got := safeUnknownReason("capture gap"); got != "" {
		t.Fatalf("safeUnknownReason(untrusted) = %q", got)
	}
	if got := safeUnknownReason(treatmentStorageObservationUnknownReason); got != treatmentStorageObservationUnknownReason {
		t.Fatalf("safeUnknownReason(known) = %q", got)
	}
}

func TestFindRejectsInvalidOrUnknownID(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(runDir, "not-a-finding-id"); err == nil || !strings.Contains(err.Error(), "ID is invalid") {
		t.Fatalf("Find() invalid ID error = %v", err)
	}
	if _, err := Find(runDir, "sha256:"+strings.Repeat("f", 64)); err == nil || !strings.Contains(err.Error(), "finding not found") {
		t.Fatalf("Find() unknown ID error = %v", err)
	}
}

func TestFindRejectsTamperedArtifact(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "baseline", "observations", "storage.json"), []byte(`{"tampered":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Find(runDir, document.Comparison.Differences[0].ID)
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("Find() tamper error = %v", err)
	}
}

func TestQuestions(t *testing.T) {
	want := []Question{
		{ID: "counterfactual-change", Text: "Did changing the declared variable influence an observed output?"},
		{ID: "capture-complete", Text: "Were all required observations captured for both sessions?"},
		{ID: "source-integrity", Text: "Do the verified findings still match their source artifacts?"},
	}
	got := Questions()
	if len(got) != len(want) {
		t.Fatalf("Questions() length = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("Questions()[%d] = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func TestAskQuestionCatalog(t *testing.T) {
	for _, test := range []struct {
		name           string
		makeRun        func(*testing.T) string
		wantState      evidencestate.State
		wantReason     string
		wantFindingIDs int
	}{
		{
			name:           "complete",
			makeRun:        func(t *testing.T) string { return makeRun(t, runOptions{}) },
			wantState:      evidencestate.Observed,
			wantFindingIDs: 1,
		},
		{
			name: "storage gap",
			makeRun: func(t *testing.T) string {
				return makeStorageFailureRun(t, "")
			},
			wantState:      evidencestate.Unknown,
			wantReason:     treatmentStorageObservationUnknownReason,
			wantFindingIDs: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := test.makeRun(t)
			if _, err := Write(runDir); err != nil {
				t.Fatal(err)
			}
			for _, question := range Questions() {
				questionID := question.ID
				answer, err := Ask(runDir, questionID)
				if err != nil {
					t.Fatalf("Ask(%q): %v", questionID, err)
				}
				wantState := test.wantState
				if questionID == "source-integrity" {
					wantState = evidencestate.Observed
				}
				wantReason := test.wantReason
				if questionID == "source-integrity" {
					wantReason = ""
				}
				if answer.QuestionID != questionID ||
					answer.Question != question.Text ||
					answer.State != wantState ||
					answer.Reason != wantReason ||
					len(answer.FindingIDs) != test.wantFindingIDs {
					t.Fatalf("Ask(%q) = %#v", questionID, answer)
				}
				if strings.Contains(answer.Question, "standard") ||
					strings.Contains(answer.Question, "personalized") {
					t.Fatalf("Ask(%q) exposed observed value: %#v", questionID, answer)
				}
				for _, id := range answer.FindingIDs {
					if !validFindingID(id) {
						t.Fatalf("Ask(%q) finding ID = %q", questionID, id)
					}
				}
			}
		})
	}
}

func TestAskRejectsInvalidOrTamperedBundles(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Ask(runDir, "not-a-question"); err == nil || !strings.Contains(err.Error(), "question ID is invalid") {
		t.Fatalf("Ask() invalid question error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "baseline", "observations", "storage.json"), []byte(`{"tampered":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Ask(runDir, "counterfactual-change")
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("Ask() tamper error = %v", err)
	}
}

func TestVerifyRejectsTamperedArtifact(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	evidenceBefore, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportBefore, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir, "baseline", "observations", "storage.json")
	if err := os.WriteFile(path, []byte(`{"tampered":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(runDir)
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("Verify() error = %v", err)
	}
	evidenceAfter, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	reportAfter, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(evidenceBefore, evidenceAfter) || !bytes.Equal(reportBefore, reportAfter) {
		t.Fatal("Verify() changed outputs after tamper")
	}
}

func TestVerifyRejectsModifiedOutputs(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		data []byte
		want string
	}{
		{name: "evidence", path: "evidence.json", data: []byte("\n"), want: "evidence output does not match"},
		{name: "report", path: "report.md", data: []byte("\n"), want: "report output does not match"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{})
			if _, err := Write(runDir); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runDir, test.path)
			file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write(test.data); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			_, err = Verify(runDir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}
}

func TestVerifyRejectsMissingOutputs(t *testing.T) {
	for _, name := range []string{"evidence.json", "report.md"} {
		t.Run(name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{})
			if _, err := Write(runDir); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(runDir, name)); err != nil {
				t.Fatal(err)
			}
			_, err := Verify(runDir)
			if err == nil || !strings.Contains(err.Error(), "output: open") {
				t.Fatalf("Verify() error = %v", err)
			}
		})
	}
}

func TestEncodeOutputsRejectOversizedData(t *testing.T) {
	evidence := document{ManifestName: strings.Repeat("x", maxOutputBytes)}
	if _, err := encodeDocument(evidence); err == nil || !strings.Contains(err.Error(), "evidence output exceeds") {
		t.Fatalf("encodeDocument() error = %v", err)
	}
	if _, err := encodeReport(evidence); err == nil || !strings.Contains(err.Error(), "report output exceeds") {
		t.Fatalf("encodeReport() error = %v", err)
	}
}

func TestWriteAcceptsLegacySessions(t *testing.T) {
	runDir := makeRun(t, runOptions{sessionSchemaVersion: 2})

	summary, err := Write(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Question != "" || summary.AnswerState != "" || summary.ManifestContractSHA256 != "" || summary.RecordedAt != "" {
		t.Fatalf("legacy summary = %#v", summary)
	}
}

func TestWriteAcceptsStableIDSessionsWithoutContract(t *testing.T) {
	runDir := makeRun(t, runOptions{sessionSchemaVersion: 5})

	summary, err := Write(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Question != "" || summary.AnswerState != "" || summary.ManifestContractSHA256 != "" || summary.RecordedAt != "" {
		t.Fatalf("stable-ID summary = %#v", summary)
	}
}

func TestWriteIncompleteTreatment(t *testing.T) {
	runDir := makeStorageFailureRun(t, "")

	summary, err := Write(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ManifestName != "experiment-001-email" ||
		summary.Differences != 0 ||
		summary.Unknowns != 2 {
		t.Fatalf("Write() = %#v", summary)
	}

	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 7 ||
		document.Question != "Did changing email influence an observed output?" ||
		document.AnswerState != evidencestate.Unknown ||
		document.Comparison.SchemaVersion != 5 ||
		len(document.Artifacts) != 5 ||
		len(document.Comparison.UnchangedFields) != 0 ||
		len(document.Comparison.Differences) != 0 ||
		len(document.Comparison.Unknowns) != 2 {
		t.Fatalf("evidence = %#v", document)
	}
	for _, unknown := range document.Comparison.Unknowns {
		if unknown.State != "unknown" ||
			unknown.Reason != "treatment storage observation was not captured" ||
			len(unknown.Evidence) != 3 ||
			!strings.HasPrefix(unknown.ID, "sha256:") {
			t.Fatalf("unknown = %#v", unknown)
		}
	}

	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, expected := range []string{
		"Verified artifacts: 5",
		"Observed differences: 0",
		"Unknown conclusions: 2",
		"No counterfactual difference was established.",
		"## Unknowns",
		"Finding ID: <code>sha256:",
		"treatment storage observation was not captured",
		"No stable fields were established.",
		"Question: <code>Did changing email influence an observed output?</code>",
		"Answer state: <code>unknown</code>",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("report missing %q:\n%s", expected, text)
		}
	}
	for _, value := range []string{"standard", "personalized"} {
		if strings.Contains(string(data), value) || strings.Contains(text, value) {
			t.Fatalf("partial bundle exposed observed value %q", value)
		}
	}
}

func TestWriteIncompleteTreatmentRejectsInvalidNetwork(t *testing.T) {
	const secret = "do-not-print-partial-value"
	runDir := makeStorageFailureRun(
		t,
		`{"schema_version":1,"region":"us-east","variant":"`+secret+`","extra":true}`,
	)

	_, err := Write(runDir)
	if err == nil || !strings.Contains(err.Error(), "observation field value must be a string") {
		t.Fatalf("Write() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Write() exposed partial value: %v", err)
	}
	assertNoOutputs(t, runDir)
}

func TestWriteRequiresRunDirectory(t *testing.T) {
	_, err := Write(" \t")
	if err == nil || !strings.Contains(err.Error(), "run directory is required") {
		t.Fatalf("Write() error = %v", err)
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T) string
	}{
		{name: "complete", make: func(t *testing.T) string {
			return makeRun(t, runOptions{})
		}},
		{name: "incomplete", make: func(t *testing.T) string {
			return makeStorageFailureRun(t, "")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := test.make(t)
			second := test.make(t)
			if _, err := Write(first); err != nil {
				t.Fatal(err)
			}
			if _, err := Write(second); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"evidence.json", "report.md"} {
				firstData, err := os.ReadFile(filepath.Join(first, name))
				if err != nil {
					t.Fatal(err)
				}
				secondData, err := os.ReadFile(filepath.Join(second, name))
				if err != nil {
					t.Fatal(err)
				}
				if string(firstData) != string(secondData) {
					t.Fatalf("%s is not deterministic", name)
				}
			}
		})
	}
}

func TestWriteRejectsTamperedArtifactWithoutExposingIt(t *testing.T) {
	const secret = "do-not-print-tampered-value"
	runDir := makeRun(t, runOptions{})
	path := filepath.Join(runDir, "baseline", "observations", "storage.json")
	if err := os.WriteFile(path, []byte(`{"value":"`+secret+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Write(runDir)
	if err == nil || !strings.Contains(err.Error(), "integrity check failed") {
		t.Fatalf("Write() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Write() exposed artifact value: %v", err)
	}
	assertNoOutputs(t, runDir)
}

func TestWriteRejectsInvalidSessions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*adb.SessionRecord)
		want   string
	}{
		{
			name: "schema",
			mutate: func(record *adb.SessionRecord) {
				record.SchemaVersion = 1
			},
			want: "schema_version or kind",
		},
		{
			name: "kind",
			mutate: func(record *adb.SessionRecord) {
				record.Kind = "other"
			},
			want: "schema_version or kind",
		},
		{
			name: "metadata",
			mutate: func(record *adb.SessionRecord) {
				record.Package = ""
			},
			want: "package is invalid",
		},
		{
			name: "persona fields",
			mutate: func(record *adb.SessionRecord) {
				record.PersonaFields = 0
			},
			want: "persona_fields",
		},
		{
			name: "volatile fields order",
			mutate: func(record *adb.SessionRecord) {
				record.VolatileFields = []string{"timestamp", "request_id"}
			},
			want: "not canonical",
		},
		{
			name: "duplicate volatile field",
			mutate: func(record *adb.SessionRecord) {
				record.VolatileFields = []string{"request_id", "request_id"}
			},
			want: "duplicate field",
		},
		{
			name: "legacy volatile fields",
			mutate: func(record *adb.SessionRecord) {
				record.SchemaVersion = 3
				record.VolatileFields = []string{"request_id"}
			},
			want: "legacy session volatile_fields",
		},
		{
			name: "Android API",
			mutate: func(record *adb.SessionRecord) {
				record.AndroidAPI = 0
			},
			want: "android_api",
		},
		{
			name: "package version",
			mutate: func(record *adb.SessionRecord) {
				record.PackageVersionCode = 0
			},
			want: "package_version_code",
		},
		{
			name: "package digest",
			mutate: func(record *adb.SessionRecord) {
				record.PackageSHA256 = "invalid"
			},
			want: "package_sha256",
		},
		{
			name: "manifest contract digest",
			mutate: func(record *adb.SessionRecord) {
				record.ManifestContractSHA256 = "invalid"
			},
			want: "manifest_contract_sha256",
		},
		{
			name: "legacy manifest contract digest",
			mutate: func(record *adb.SessionRecord) {
				record.SchemaVersion = 5
				record.ManifestContractSHA256 = strings.Repeat("c", 64)
			},
			want: "legacy session manifest_contract_sha256",
		},
		{
			name: "Ariadne revision",
			mutate: func(record *adb.SessionRecord) {
				record.AriadneRevision = "invalid"
			},
			want: "ariadne_revision",
		},
		{
			name: "invalid status",
			mutate: func(record *adb.SessionRecord) {
				record.Status = "other"
			},
			want: "session status is invalid",
		},
		{
			name: "complete with failure stage",
			mutate: func(record *adb.SessionRecord) {
				record.FailureStage = "capture_storage"
			},
			want: "complete session failure_stage is invalid",
		},
		{
			name: "incomplete with invalid failure stage",
			mutate: func(record *adb.SessionRecord) {
				record.Status = "incomplete"
				record.FailureStage = "other"
			},
			want: "incomplete session failure_stage is invalid",
		},
		{
			name: "unsupported incomplete stage",
			mutate: func(record *adb.SessionRecord) {
				record.Status = "incomplete"
				record.FailureStage = "start"
			},
			want: "session is incomplete at start",
		},
		{
			name: "incomplete step status",
			mutate: func(record *adb.SessionRecord) {
				record.Status = "incomplete"
				record.FailureStage = "capture_storage"
				record.Artifacts = record.Artifacts[:1]
			},
			want: `step "capture_storage" is invalid`,
		},
		{
			name: "failed step",
			mutate: func(record *adb.SessionRecord) {
				record.Steps[2].Status = "error"
				record.Steps[2].ExitCode = 1
			},
			want: `step "start" is invalid`,
		},
		{
			name: "wrong step",
			mutate: func(record *adb.SessionRecord) {
				record.Steps[1].Name = "other"
			},
			want: `step "connect_network" is invalid`,
		},
		{
			name: "missing steps",
			mutate: func(record *adb.SessionRecord) {
				record.Steps = record.Steps[:len(record.Steps)-1]
			},
			want: "step sequence is incomplete",
		},
		{
			name: "timestamp",
			mutate: func(record *adb.SessionRecord) {
				record.FinishedAt = record.StartedAt.Add(-time.Second)
			},
			want: "timestamps",
		},
		{
			name: "step timestamp",
			mutate: func(record *adb.SessionRecord) {
				record.Steps[0].StartedAt = time.Time{}
			},
			want: `step "reset" is invalid`,
		},
		{
			name: "artifact count",
			mutate: func(record *adb.SessionRecord) {
				record.Artifacts = record.Artifacts[:1]
			},
			want: "expected 2 artifacts",
		},
		{
			name: "artifact metadata",
			mutate: func(record *adb.SessionRecord) {
				record.Artifacts[0].SHA256 = "invalid"
			},
			want: "metadata is invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{mutateTreatment: test.mutate})
			_, err := Write(runDir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Write() error = %v, want containing %q", err, test.want)
			}
			assertNoOutputs(t, runDir)
		})
	}
}

func TestWriteRejectsMixedSessionSchemas(t *testing.T) {
	runDir := makeRun(t, runOptions{
		mutateTreatment: func(record *adb.SessionRecord) {
			record.SchemaVersion = 2
			record.TapResourceID = ""
			record.ManifestContractSHA256 = ""
			record.Status = ""
			record.Steps = append(record.Steps[:3], record.Steps[4:]...)
		},
	})

	_, err := Write(runDir)
	if err == nil || !strings.Contains(err.Error(), "session metadata disagree") {
		t.Fatalf("Write() error = %v", err)
	}
	assertNoOutputs(t, runDir)
}

func TestWriteRejectsSessionAndSourceDisagreement(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*adb.SessionRecord)
	}{
		{
			name: "device",
			mutate: func(record *adb.SessionRecord) {
				record.Device = "emulator-5556"
			},
		},
		{
			name: "package digest",
			mutate: func(record *adb.SessionRecord) {
				record.PackageSHA256 = strings.Repeat("c", 64)
			},
		},
		{
			name: "volatile fields",
			mutate: func(record *adb.SessionRecord) {
				record.VolatileFields = []string{"request_id"}
			},
		},
		{
			name: "manifest contract digest",
			mutate: func(record *adb.SessionRecord) {
				record.ManifestContractSHA256 = strings.Repeat("d", 64)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{mutateTreatment: test.mutate})
			_, err := Write(runDir)
			if err == nil || !strings.Contains(err.Error(), "session metadata disagree") {
				t.Fatalf("Write() error = %v", err)
			}
		})
	}

	t.Run("observation sources", func(t *testing.T) {
		runDir := makeRun(t, runOptions{
			treatmentNetworkBody: baselineObservation,
		})
		_, err := Write(runDir)
		if err == nil || !strings.Contains(err.Error(), "observations disagree") {
			t.Fatalf("Write() error = %v", err)
		}
	})
}

func TestWriteRejectsInvalidSessionJSON(t *testing.T) {
	tests := []struct {
		name   string
		change func(string) string
		want   string
	}{
		{
			name: "duplicate",
			change: func(input string) string {
				return strings.Replace(
					input,
					`"schema_version": 6,`,
					`"schema_version": 6, "schema_version": 6,`,
					1,
				)
			},
			want: `duplicate key "schema_version"`,
		},
		{
			name: "unknown",
			change: func(input string) string {
				return strings.Replace(input, `"kind":`, `"extra": true, "kind":`, 1)
			},
			want: `unknown field "extra"`,
		},
		{
			name:   "trailing",
			change: func(input string) string { return input + `{}` },
			want:   "trailing data",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{})
			path := filepath.Join(runDir, "baseline", "session.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(test.change(string(data))), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = Write(runDir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Write() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestWriteRefusesExistingOutput(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	reportPath := filepath.Join(runDir, "report.md")
	if err := os.WriteFile(reportPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Write(runDir)
	if err == nil || !strings.Contains(err.Error(), "output already exists") {
		t.Fatalf("Write() error = %v", err)
	}
	data, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "existing" {
		t.Fatalf("existing report changed: %q", data)
	}
	if _, err := os.Stat(filepath.Join(runDir, "evidence.json")); !os.IsNotExist(err) {
		t.Fatalf("evidence.json exists: %v", err)
	}
}

func TestWriteEscapesReportMetadata(t *testing.T) {
	runDir := makeRun(t, runOptions{
		mutateBaseline: func(record *adb.SessionRecord) {
			record.ManifestName = `<script>alert(1)</script>`
		},
		mutateTreatment: func(record *adb.SessionRecord) {
			record.ManifestName = `<script>alert(1)</script>`
		},
	})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(report), "<script>") ||
		!strings.Contains(string(report), "&lt;script&gt;") {
		t.Fatalf("report metadata was not escaped:\n%s", report)
	}
}

func TestWriteNoDifferences(t *testing.T) {
	runDir := makeRun(t, runOptions{
		treatmentStorage:     baselineObservation,
		treatmentNetworkBody: baselineObservation,
	})
	summary, err := Write(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Differences != 0 {
		t.Fatalf("Write() = %#v", summary)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(report), "No observed differences.") {
		t.Fatalf("report = %s", report)
	}
}

func TestWriteRecordsVolatileFieldNormalization(t *testing.T) {
	const baseline = `{"schema_version":1,"region":"us-east","request_id":"baseline-id","variant":"standard"}`
	const treatment = `{"schema_version":1,"region":"us-east","request_id":"treatment-id","variant":"personalized"}`
	runDir := makeRun(t, runOptions{
		baselineStorage:      baseline,
		baselineNetworkBody:  baseline,
		treatmentStorage:     treatment,
		treatmentNetworkBody: treatment,
		volatileFields:       []string{"request_id"},
	})

	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document document
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Comparison.Differences) != 1 ||
		document.Comparison.Differences[0].Field != "variant" ||
		len(document.Comparison.NormalizedFields) != 1 ||
		document.Comparison.NormalizedFields[0] != "request_id" {
		t.Fatalf("comparison = %#v", document.Comparison)
	}
	report, err := os.ReadFile(filepath.Join(runDir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(
		string(report),
		"removed declared volatile observation field request_id from comparison",
	) {
		t.Fatalf("report = %s", report)
	}
	for kind, want := range map[string]string{
		"baseline":  baseline,
		"treatment": treatment,
	} {
		raw, err := os.ReadFile(
			filepath.Join(runDir, kind, "observations", "storage.json"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != want {
			t.Fatalf("%s raw observation changed: %s", kind, raw)
		}
	}
}

type runOptions struct {
	sessionSchemaVersion int
	baselineStorage      string
	baselineNetworkBody  string
	treatmentStorage     string
	treatmentNetworkBody string
	volatileFields       []string
	mutateBaseline       func(*adb.SessionRecord)
	mutateTreatment      func(*adb.SessionRecord)
}

func markStorageCaptureFailure(record *adb.SessionRecord) {
	record.Status = "incomplete"
	record.FailureStage = "capture_storage"
	for index := range record.Steps {
		if record.Steps[index].Name == "capture_storage" {
			record.Steps[index].Status = "error"
			record.Steps[index].ExitCode = -1
			break
		}
	}
	record.Artifacts = record.Artifacts[:1]
}

func makeStorageFailureRun(t *testing.T, networkBody string) string {
	t.Helper()
	runDir := makeRun(t, runOptions{
		treatmentNetworkBody: networkBody,
		mutateTreatment:      markStorageCaptureFailure,
	})
	if err := os.Remove(
		filepath.Join(runDir, "treatment", "observations", "storage.json"),
	); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func makeRun(t *testing.T, options runOptions) string {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "run")
	if options.sessionSchemaVersion == 0 {
		options.sessionSchemaVersion = 6
	}
	if options.baselineStorage == "" {
		options.baselineStorage = baselineObservation
	}
	if options.baselineNetworkBody == "" {
		options.baselineNetworkBody = baselineObservation
	}
	if options.treatmentStorage == "" {
		options.treatmentStorage = treatmentObservation
	}
	if options.treatmentNetworkBody == "" {
		options.treatmentNetworkBody = treatmentObservation
	}
	writeSession(
		t,
		runDir,
		"baseline",
		options.baselineStorage,
		options.baselineNetworkBody,
		options.volatileFields,
		options.sessionSchemaVersion,
		options.mutateBaseline,
	)
	writeSession(
		t,
		runDir,
		"treatment",
		options.treatmentStorage,
		options.treatmentNetworkBody,
		options.volatileFields,
		options.sessionSchemaVersion,
		options.mutateTreatment,
	)
	return runDir
}

func writeSession(
	t *testing.T,
	runDir, kind, storageBody, networkBody string,
	volatileFields []string,
	schemaVersion int,
	mutate func(*adb.SessionRecord),
) {
	t.Helper()
	sessionDir := filepath.Join(runDir, kind)
	observationDir := filepath.Join(sessionDir, "observations")
	if err := os.MkdirAll(observationDir, 0o700); err != nil {
		t.Fatal(err)
	}
	network := networkJSON(t, networkBody)
	storage := []byte(storageBody)
	if err := os.WriteFile(filepath.Join(observationDir, "network.json"), network, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(observationDir, "storage.json"), storage, 0o600); err != nil {
		t.Fatal(err)
	}

	started := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if kind == "treatment" {
		started = started.Add(20 * time.Second)
	}
	stepNames := expectedSteps
	if schemaVersion < 5 {
		stepNames = legacyExpectedSteps
	}
	steps := make([]adb.StepRecord, len(stepNames))
	for index, name := range stepNames {
		stepStart := started.Add(time.Duration(index+1) * time.Second)
		steps[index] = adb.StepRecord{
			Name:       name,
			StartedAt:  stepStart,
			FinishedAt: stepStart.Add(time.Second),
			Status:     "ok",
			ExitCode:   0,
		}
	}
	record := adb.SessionRecord{
		SchemaVersion:      schemaVersion,
		Kind:               kind,
		ManifestName:       "experiment-001-email",
		DeclaredVariable:   "email",
		PersonaFields:      2,
		VolatileFields:     append([]string(nil), volatileFields...),
		ADBVersion:         "1.0.41",
		Device:             "emulator-5554",
		Package:            "dev.ariadne.fixture",
		AndroidAPI:         35,
		Architecture:       "x86_64",
		PackageVersionCode: 1,
		PackageSHA256:      strings.Repeat("a", 64),
		AriadneRevision:    strings.Repeat("b", 40),
		StartedAt:          started,
		FinishedAt:         started.Add(10 * time.Second),
		Steps:              steps,
		Artifacts: []adb.Artifact{
			adbArtifact("http_request", "POST /observe", "observations/network.json", network),
			adbArtifact(
				"android_private_storage",
				"files/observation.json",
				"observations/storage.json",
				storage,
			),
		},
	}
	if schemaVersion >= 5 {
		record.TapResourceID = "dev.ariadne.fixture:id/observe_button"
	}
	if schemaVersion >= 6 {
		record.ManifestContractSHA256 = strings.Repeat("c", 64)
	}
	if schemaVersion >= 3 {
		record.Status = "complete"
	}
	if mutate != nil {
		mutate(&record)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func networkJSON(t *testing.T, body string) []byte {
	t.Helper()
	data, err := json.MarshalIndent(collector.Observation{
		SchemaVersion: 1,
		Method:        "POST",
		Path:          "/observe",
		ContentType:   "application/json",
		BodyBase64:    base64.StdEncoding.EncodeToString([]byte(body)),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func adbArtifact(kind, source, path string, data []byte) adb.Artifact {
	sum := sha256.Sum256(data)
	return adb.Artifact{
		Kind:      kind,
		Source:    source,
		Path:      path,
		SizeBytes: len(data),
		SHA256:    hex.EncodeToString(sum[:]),
	}
}

func assertNoOutputs(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{"evidence.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists: %v", name, err)
		}
	}
}
