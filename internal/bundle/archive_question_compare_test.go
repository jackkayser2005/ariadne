package bundle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCompareArchiveQuestionReports(t *testing.T) {
	olderPath, olderReport := savedArchiveQuestionReport(t, "a-run")
	same, err := CompareArchiveQuestionReports(olderPath, olderPath)
	if err != nil {
		t.Fatal(err)
	}
	if same.Result != "same" || same.Compared != 1 || same.Changed != 0 || same.OlderOnly != 0 || same.NewerOnly != 0 {
		t.Fatalf("same comparison = %#v", same)
	}
	if same.OlderReflectionSHA256 != same.NewerReflectionSHA256 || !validDigest(same.OlderReflectionSHA256) {
		t.Fatalf("same comparison identities = %#v", same)
	}

	olderReport.Results[0].Answer.State = evidence.Unknown
	olderReport.Summary.Observed = 0
	olderReport.Summary.Unknown = 1
	changedPath := writeArchiveQuestionReport(t, olderReport)
	changed, err := CompareArchiveQuestionReports(olderPath, changedPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Result != "changed" || changed.Compared != 1 || changed.Changed != 1 {
		t.Fatalf("changed comparison = %#v", changed)
	}

	newerPath, _ := savedArchiveQuestionReport(t, "a-run", "b-run")
	incomparable, err := CompareArchiveQuestionReports(olderPath, newerPath)
	if err != nil {
		t.Fatal(err)
	}
	if incomparable.Result != "incomparable" || incomparable.Compared != 1 || incomparable.OlderOnly != 0 || incomparable.NewerOnly != 1 {
		t.Fatalf("incomparable comparison = %#v", incomparable)
	}

	data, err := json.Marshal(incomparable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("comparison exposed a raw value: %s", data)
	}
}

func TestCompareArchiveQuestionReportsRejectsMismatchedQuestions(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})
	newerReport, err := AskArchive(root, "capture-complete")
	if err != nil {
		t.Fatal(err)
	}
	newerPath := writeArchiveQuestionReport(t, newerReport)
	if _, err := CompareArchiveQuestionReports(olderPath, newerPath); err == nil || !strings.Contains(err.Error(), "questions do not match") {
		t.Fatalf("CompareArchiveQuestionReports() error = %v", err)
	}
}

func TestCompareArchiveQuestionReportsRejectsInvalidOlderReport(t *testing.T) {
	if _, err := CompareArchiveQuestionReports(" ", "newer.json"); err == nil || !strings.Contains(err.Error(), "older reflection") {
		t.Fatalf("CompareArchiveQuestionReports() error = %v", err)
	}
}
