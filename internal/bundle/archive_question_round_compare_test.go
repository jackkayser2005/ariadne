package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareArchiveQuestionTransitionHistoryQuestionRounds(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	first, err := AskArchiveQuestionTransitionHistoryQuestionRound(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Questions = append([]ArchiveQuestionTransitionHistoryQuestionRoundItem(nil), first.Questions...)
	second.Questions[0].Result = "same"
	firstPath := writeQuestionRoundForComparison(t, first)
	secondPath := writeQuestionRoundForComparison(t, second)

	comparison, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.SchemaVersion != 1 || comparison.ComparisonID != "question-round-answer-change" || comparison.OrderBasis != "caller" || comparison.Result != "changed" || comparison.Compared != 4 || comparison.Changed != 1 || len(comparison.ChangedQuestions) != 1 {
		t.Fatalf("comparison = %#v", comparison)
	}
	change := comparison.ChangedQuestions[0]
	if change.QuestionID != first.Questions[0].QuestionID || change.FirstResult != "changed" || change.SecondResult != "same" || comparison.FirstTransitionHistorySHA256 != comparison.SecondTransitionHistorySHA256 || comparison.FirstRoundSHA256 == comparison.SecondRoundSHA256 {
		t.Fatalf("comparison change = %#v, comparison = %#v", change, comparison)
	}

	same, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if same.Result != "same" || same.Compared != 4 || same.Changed != 0 || len(same.ChangedQuestions) != 0 {
		t.Fatalf("same comparison = %#v", same)
	}
}

func TestCompareArchiveQuestionTransitionHistoryQuestionRoundsRejectsInvalidPath(t *testing.T) {
	if _, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(" ", "second.json"); err == nil || !strings.Contains(err.Error(), "first question round") || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("first path error = %v", err)
	}
	first := AnswerArchiveQuestionTransitionHistoryQuestionRound(validArchiveQuestionTransitionHistory(), strings.Repeat("a", 64))
	firstPath := writeQuestionRoundForComparison(t, first)
	if _, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, " "); err == nil || !strings.Contains(err.Error(), "second question round") || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("second path error = %v", err)
	}
	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, badPath); err == nil || !strings.Contains(err.Error(), "second question round") || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("invalid second round error = %v", err)
	}
}

func TestCompareArchiveQuestionTransitionHistoryQuestionRoundsRejectsDifferentHistoryQuestions(t *testing.T) {
	first := AnswerArchiveQuestionTransitionHistoryQuestionRound(validArchiveQuestionTransitionHistory(), strings.Repeat("a", 64))
	second := first
	second.HistoryQuestionID = "capture-complete"
	firstPath := writeQuestionRoundForComparison(t, first)
	secondPath := writeQuestionRoundForComparison(t, second)
	if _, err := CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, secondPath); err == nil || !strings.Contains(err.Error(), "different history questions") {
		t.Fatalf("different history question error = %v", err)
	}
}

func writeQuestionRoundForComparison(t *testing.T, round ArchiveQuestionTransitionHistoryQuestionRoundAnswer) string {
	t.Helper()
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "round.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
