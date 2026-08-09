package bundle

import (
	"encoding/json"
	"path/filepath"
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

func TestCompareArchiveQuestionReportWithArchive(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})

	comparison, err := CompareArchiveQuestionReportWithArchive(olderPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Result != "same" || comparison.Compared != 1 || comparison.Changed != 0 ||
		comparison.OlderOnly != 0 || comparison.NewerOnly != 0 ||
		!validDigest(comparison.OlderReflectionSHA256) || !validDigest(comparison.NewerReflectionSHA256) {
		t.Fatalf("live comparison = %#v", comparison)
	}
}

func TestCompareArchiveQuestionReportWithArchiveRejectsInvalidInput(t *testing.T) {
	if _, err := CompareArchiveQuestionReportWithArchive(" ", t.TempDir()); err == nil || !strings.Contains(err.Error(), "older reflection") {
		t.Fatalf("CompareArchiveQuestionReportWithArchive() older error = %v", err)
	}
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	if _, err := CompareArchiveQuestionReportWithArchive(olderPath, filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "current archive") {
		t.Fatalf("CompareArchiveQuestionReportWithArchive() current error = %v", err)
	}
}

func TestCompareArchiveQuestionHistory(t *testing.T) {
	olderPath, olderReport := savedArchiveQuestionReport(t, "a-run")
	olderReport.Results[0].Answer.State = evidence.Unknown
	olderReport.Summary.Observed = 0
	olderReport.Summary.Unknown = 1
	changedPath := writeArchiveQuestionReport(t, olderReport)
	newerPath, _ := savedArchiveQuestionReport(t, "a-run", "b-run")

	history, err := CompareArchiveQuestionHistory([]string{olderPath, changedPath, newerPath})
	if err != nil {
		t.Fatal(err)
	}
	if history.SchemaVersion != 1 || history.HistoryID != "answer-state-transitions" ||
		history.HistoryQuestion != "At which supplied boundaries did the bounded answer state change?" ||
		history.QuestionID != "counterfactual-change" || history.OrderBasis != "caller" ||
		history.Snapshots != 3 || len(history.Transitions) != 2 {
		t.Fatalf("history metadata = %#v", history)
	}
	first := history.Transitions[0]
	if first.Result != "changed" || first.Compared != 1 || first.Changed != 1 || first.FromOnly != 0 || first.ToOnly != 0 {
		t.Fatalf("changed transition = %#v", first)
	}
	second := history.Transitions[1]
	if second.Result != "incomparable" || second.Compared != 1 || second.Changed != 1 || second.FromOnly != 0 || second.ToOnly != 1 {
		t.Fatalf("incomparable transition = %#v", second)
	}
	for _, transition := range history.Transitions {
		if !validDigest(transition.FromReflectionSHA256) || !validDigest(transition.ToReflectionSHA256) {
			t.Fatalf("transition identities = %#v", transition)
		}
	}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("history exposed a raw value: %s", data)
	}
}

func TestCompareArchiveQuestionHistoryRejectsInvalidInput(t *testing.T) {
	if _, err := CompareArchiveQuestionHistory(nil); err == nil || !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("CompareArchiveQuestionHistory() short input error = %v", err)
	}
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	if _, err := CompareArchiveQuestionHistory([]string{olderPath, "missing.json"}); err == nil || !strings.Contains(err.Error(), "reflection 2") {
		t.Fatalf("CompareArchiveQuestionHistory() invalid second report error = %v", err)
	}
}

func TestCompareArchiveQuestionHistoryRejectsMismatchedQuestions(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})
	newerReport, err := AskArchive(root, "capture-complete")
	if err != nil {
		t.Fatal(err)
	}
	newerPath := writeArchiveQuestionReport(t, newerReport)
	if _, err := CompareArchiveQuestionHistory([]string{olderPath, newerPath}); err == nil || !strings.Contains(err.Error(), "questions do not match") {
		t.Fatalf("CompareArchiveQuestionHistory() mismatch error = %v", err)
	}
}
