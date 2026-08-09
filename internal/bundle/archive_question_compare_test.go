package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
	if same.SchemaVersion != 2 || same.Result != "same" || same.Compared != 1 || same.Changed != 0 || same.OlderOnly != 0 || same.NewerOnly != 0 || len(same.StateChanges) != 0 {
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
	if changed.Result != "changed" || changed.Compared != 1 || changed.Changed != 1 || len(changed.StateChanges) != 1 || changed.StateChanges[0].Directory != "a-run" || changed.StateChanges[0].OlderState != "observed" || changed.StateChanges[0].NewerState != "unknown" {
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

func TestCompareArchiveQuestionStateChangesAreSorted(t *testing.T) {
	older := ArchiveQuestionReport{
		QuestionID: "counterfactual-change",
		Question:   "Did changing the declared variable influence an observed output?",
		Results: []ArchiveQuestionResult{
			{Directory: "b-run", Available: true, Answer: &Answer{State: evidence.Observed}},
			{Directory: "a-run", Available: true, Answer: &Answer{State: evidence.Unknown}},
		},
	}
	newer := ArchiveQuestionReport{
		QuestionID: older.QuestionID,
		Question:   older.Question,
		Results: []ArchiveQuestionResult{
			{Directory: "b-run", Available: true, Answer: &Answer{State: evidence.Unknown}},
			{Directory: "a-run", Available: true, Answer: &Answer{State: evidence.Observed}},
		},
	}
	comparison := compareVerifiedArchiveQuestionReports(
		older,
		ArchiveQuestionVerificationSummary{ReflectionSHA256: strings.Repeat("a", 64)},
		newer,
		ArchiveQuestionVerificationSummary{ReflectionSHA256: strings.Repeat("b", 64)},
	)
	if len(comparison.StateChanges) != 2 || comparison.StateChanges[0].Directory != "a-run" || comparison.StateChanges[1].Directory != "b-run" {
		t.Fatalf("state changes = %#v", comparison.StateChanges)
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
	if history.SchemaVersion != 2 || history.HistoryID != "answer-state-transitions" ||
		history.HistoryQuestion != "At which supplied boundaries did the bounded answer state change?" ||
		history.QuestionID != "counterfactual-change" || history.OrderBasis != "caller" ||
		history.Snapshots != 3 || len(history.Transitions) != 2 {
		t.Fatalf("history metadata = %#v", history)
	}
	first := history.Transitions[0]
	if first.Result != "changed" || first.Compared != 1 || first.Changed != 1 || first.FromOnly != 0 || first.ToOnly != 0 || len(first.StateChanges) != 1 || first.StateChanges[0].Directory != "a-run" || first.StateChanges[0].OlderState != "observed" || first.StateChanges[0].NewerState != "unknown" {
		t.Fatalf("changed transition = %#v", first)
	}
	second := history.Transitions[1]
	if second.Result != "incomparable" || second.Compared != 1 || second.Changed != 1 || second.FromOnly != 0 || second.ToOnly != 1 || len(second.StateChanges) != 1 || second.StateChanges[0].Directory != "a-run" || second.StateChanges[0].OlderState != "unknown" || second.StateChanges[0].NewerState != "observed" {
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

func TestAnswerArchiveQuestionTransitionHistory(t *testing.T) {
	history := ArchiveQuestionTransitionHistory{
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		Transitions: []ArchiveQuestionTransition{
			{Result: "same"},
			{Result: "changed", Changed: 1},
			{Result: "incomparable"},
			{Result: "incomparable", Changed: 1},
		},
	}
	answer := AnswerArchiveQuestionTransitionHistory(history, strings.Repeat("a", 64))
	if answer.SchemaVersion != 1 || answer.QuestionID != history.HistoryID || answer.Question != history.HistoryQuestion || answer.Result != "changed" || answer.TransitionHistorySHA256 != strings.Repeat("a", 64) || answer.Transitions != 4 {
		t.Fatalf("history answer metadata = %#v", answer)
	}
	if !reflect.DeepEqual(answer.ChangedTransitions, []int{2, 4}) || !reflect.DeepEqual(answer.IncomparableTransitions, []int{3, 4}) {
		t.Fatalf("history answer indexes = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistory(ArchiveQuestionTransitionHistory{
		HistoryID:       history.HistoryID,
		HistoryQuestion: history.HistoryQuestion,
		Transitions:     []ArchiveQuestionTransition{{Result: "incomparable"}},
	}, strings.Repeat("b", 64))
	if answer.Result != "incomparable" || len(answer.ChangedTransitions) != 0 || !reflect.DeepEqual(answer.IncomparableTransitions, []int{1}) {
		t.Fatalf("incomparable history answer = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistory(ArchiveQuestionTransitionHistory{
		HistoryID:       history.HistoryID,
		HistoryQuestion: history.HistoryQuestion,
		Transitions:     []ArchiveQuestionTransition{{Result: "same"}},
	}, strings.Repeat("c", 64))
	if answer.Result != "same" || len(answer.ChangedTransitions) != 0 || len(answer.IncomparableTransitions) != 0 {
		t.Fatalf("same history answer = %#v", answer)
	}
}

func TestAskArchiveQuestionTransitionHistory(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	path := writeArchiveQuestionTransitionHistory(t, history)
	answer, err := AskArchiveQuestionTransitionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "changed" || !validDigest(answer.TransitionHistorySHA256) || !reflect.DeepEqual(answer.ChangedTransitions, []int{1}) {
		t.Fatalf("AskArchiveQuestionTransitionHistory() = %#v", answer)
	}
	if _, err := AskArchiveQuestionTransitionHistory(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistory() empty path error = %v", err)
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

func TestSaveArchiveQuestionTransitionHistory(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	newerPath, _ := savedArchiveQuestionReport(t, "a-run")
	historyPath := filepath.Join(t.TempDir(), "transitions.json")

	summary, err := SaveArchiveQuestionTransitionHistory([]string{olderPath, newerPath}, historyPath)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyArchiveQuestionTransitionHistory(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary != verified || summary.HistoryID != "answer-state-transitions" || summary.Snapshots != 2 || summary.Transitions != 1 || !validDigest(summary.TransitionHistorySHA256) {
		t.Fatalf("SaveArchiveQuestionTransitionHistory() summary = %#v, verified = %#v", summary, verified)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("saved transition history does not end with a newline: %q", data)
	}
	for _, rawValue := range []string{"standard", "personalized", "baseline@example.invalid", "treatment@example.invalid", "emulator-5554"} {
		if strings.Contains(string(data), rawValue) {
			t.Fatalf("SaveArchiveQuestionTransitionHistory() exposed raw value %q: %s", rawValue, data)
		}
	}
	if _, err := SaveArchiveQuestionTransitionHistory([]string{olderPath, newerPath}, historyPath); err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("second SaveArchiveQuestionTransitionHistory() error = %v", err)
	}
}

func TestSaveArchiveQuestionTransitionHistoryRequiresPath(t *testing.T) {
	if _, err := SaveArchiveQuestionTransitionHistory(nil, " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("SaveArchiveQuestionTransitionHistory() error = %v", err)
	}
}
