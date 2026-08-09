package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyArchiveQuestionReport(t *testing.T) {
	path, report := savedArchiveQuestionReport(t, "a-run")

	summary, err := VerifyArchiveQuestionReport(path)
	if err != nil {
		t.Fatal(err)
	}
	want := ArchiveQuestionVerificationSummary{
		SchemaVersion: 2,
		QuestionID:    "counterfactual-change",
		Checked:       1,
	}
	if summary != want {
		t.Fatalf("VerifyArchiveQuestionReport() = %#v, want %#v", summary, want)
	}
	if report.Results[0].Answer == nil || strings.Contains(report.Results[0].Answer.Question, "standard") {
		t.Fatalf("saved report was not bounded: %#v", report.Results[0].Answer)
	}
}

func TestVerifyArchiveQuestionReportAcceptsUnavailableEntry(t *testing.T) {
	root := t.TempDir()
	archiveRun(t, root, "current", runOptions{})
	legacyRun := makeRun(t, runOptions{sessionSchemaVersion: 5})
	legacyDir := filepath.Join(root, "legacy")
	if err := os.Rename(legacyRun, legacyDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(legacyDir); err != nil {
		t.Fatal(err)
	}
	report, err := AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	path := writeArchiveQuestionReport(t, report)

	summary, err := VerifyArchiveQuestionReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Checked != 2 || report.Results[1].Available {
		t.Fatalf("unavailable report = %#v", report)
	}
}

func TestVerifyArchiveQuestionReportRejectsInvalidReports(t *testing.T) {
	path, report := savedArchiveQuestionReport(t, "a-run")
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "invalid JSON"},
		{name: "duplicate", data: bytes.Replace(valid, []byte(`"schema_version":2`), []byte(`"schema_version":2,"schema_version":2`), 1), want: "duplicate key"},
		{name: "unknown", data: bytes.Replace(valid, []byte("{"), []byte(`{"extra":true,`), 1), want: "unknown field"},
		{name: "trailing", data: append(append([]byte(nil), valid...), []byte("{}")...), want: "trailing data"},
		{name: "schema", data: bytes.Replace(valid, []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1), want: "unsupported schema_version"},
		{name: "state", data: bytes.Replace(valid, []byte(`"answer_state":"observed"`), []byte(`"answer_state":"invalid"`), 1), want: "archive answer state is invalid"},
		{name: "digest", data: bytes.Replace(valid, []byte(`"source_evidence_sha256":"`), []byte(`"source_evidence_sha256":"bad`), 1), want: "archive result provenance is invalid"},
		{name: "finding", data: bytes.Replace(valid, []byte(`"sha256:`), []byte(`"sha256:x`), 1), want: "archive answer finding ID is invalid"},
		{name: "recorded time", data: bytes.Replace(valid, []byte(`"recorded_at":"2026-07-25T12:00:00Z"`), []byte(`"recorded_at":"not-a-time"`), 1), want: "archive result recorded_at is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalidPath := writeArchiveQuestionReportBytes(t, test.data)
			if _, err := VerifyArchiveQuestionReport(invalidPath); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyArchiveQuestionReport() error = %v, want %q", err, test.want)
			}
		})
	}

	var ordered ArchiveQuestionReport
	root := t.TempDir()
	archiveRun(t, root, "z-run", runOptions{})
	archiveRun(t, root, "a-run", runOptions{})
	ordered, err = AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	ordered.Results[0], ordered.Results[1] = ordered.Results[1], ordered.Results[0]
	if _, err := VerifyArchiveQuestionReport(writeArchiveQuestionReport(t, ordered)); err == nil || !strings.Contains(err.Error(), "not ordered chronologically") {
		t.Fatalf("VerifyArchiveQuestionReport() order error = %v", err)
	}
}

func TestVerifyArchiveQuestionReportRejectsUnsafeUnknownReason(t *testing.T) {
	root := t.TempDir()
	archiveRun(t, root, "complete", runOptions{})
	storageRun := makeStorageFailureRun(t, "")
	storageDir := filepath.Join(root, "storage-gap")
	if err := os.Rename(storageRun, storageDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(storageDir); err != nil {
		t.Fatal(err)
	}
	report, err := AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(treatmentStorageObservationUnknownReason), []byte("private"), 1)
	if _, err := VerifyArchiveQuestionReport(writeArchiveQuestionReportBytes(t, data)); err == nil || !strings.Contains(err.Error(), "archive answer reason is invalid") {
		t.Fatalf("VerifyArchiveQuestionReport() reason error = %v", err)
	}
}

func TestVerifyArchiveQuestionReportRejectsOversizedReport(t *testing.T) {
	path := writeArchiveQuestionReportBytes(t, bytes.Repeat([]byte("x"), maxOutputBytes+1))
	if _, err := VerifyArchiveQuestionReport(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("VerifyArchiveQuestionReport() oversized error = %v", err)
	}
}

func TestVerifyArchiveQuestionReportRequiresPath(t *testing.T) {
	if _, err := VerifyArchiveQuestionReport(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("VerifyArchiveQuestionReport() empty path error = %v", err)
	}
	if _, err := VerifyArchiveQuestionReport(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("VerifyArchiveQuestionReport() accepted missing path")
	}
}

func savedArchiveQuestionReport(t *testing.T, names ...string) (string, ArchiveQuestionReport) {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		archiveRun(t, root, name, runOptions{})
	}
	report, err := AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	return writeArchiveQuestionReport(t, report), report
}

func writeArchiveQuestionReport(t *testing.T, report ArchiveQuestionReport) string {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return writeArchiveQuestionReportBytes(t, data)
}

func writeArchiveQuestionReportBytes(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive-question.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
