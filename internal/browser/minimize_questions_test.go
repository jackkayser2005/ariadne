package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestFixtureMinimizationQuestionWrappersVerifyAndReuseLadder(t *testing.T) {
	root := writeVerifiedBrowserMinimization(t)
	answers, err := AskAllFixtureMinimizationQuestions(root)
	if err != nil || len(answers) != len(minimize.LadderQuestions()) {
		t.Fatalf("AskAllFixtureMinimizationQuestions() = %#v, %v", answers, err)
	}
	answer, err := AskFixtureMinimizationQuestion(root, minimize.MinimizationQuestionSelection)
	if err != nil {
		t.Fatalf("AskFixtureMinimizationQuestion() error = %v", err)
	}
	if answer.QuestionID != minimize.MinimizationQuestionSelection || answer.Result != string(minimize.SelectionSelected) || answer.SelectedCandidate != BrowserFixtureOmittedCandidate {
		t.Fatalf("browser ladder answer = %#v", answer)
	}

	roundPath := filepath.Join(t.TempDir(), "nested", "round.json")
	roundSummary, err := SaveFixtureMinimizationQuestionRound(root, roundPath)
	if err != nil {
		t.Fatalf("SaveFixtureMinimizationQuestionRound() error = %v", err)
	}
	if roundSummary.Questions != len(minimize.LadderQuestions()) || roundSummary.Candidates != 1 {
		t.Fatalf("browser ladder round summary = %#v", roundSummary)
	}
	round, readSummary, err := minimize.ReadLadderQuestionRound(roundPath)
	if err != nil || readSummary != roundSummary || len(round.Answers) != len(answers) {
		t.Fatalf("browser ladder round = %#v, %#v, %v", round, readSummary, err)
	}
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	receiptSummary, err := minimize.SaveLadderQuestionReceipt(roundPath, minimize.MinimizationQuestionSelection, receiptPath)
	if err != nil {
		t.Fatalf("SaveLadderQuestionReceipt() error = %v", err)
	}
	if receiptSummary.QuestionID != minimize.MinimizationQuestionSelection || receiptSummary.RoundSHA256 != roundSummary.RoundSHA256 {
		t.Fatalf("browser ladder receipt summary = %#v", receiptSummary)
	}
	for _, path := range []string{roundPath, receiptPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"https://", "account-value", "chrome.exe", "Program Files", "session_id"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("browser question artifact %q disclosed %q", path, secret)
			}
		}
	}
}

func TestFixtureMinimizationQuestionWrappersRejectTamperedLadder(t *testing.T) {
	root := writeVerifiedBrowserMinimization(t)
	receiptPath := filepath.Join(root, "minimization.json")
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary minimize.LadderSummary
	if err := json.Unmarshal(data, &summary); err != nil {
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
	if _, err := AskAllFixtureMinimizationQuestions(root); err == nil {
		t.Fatal("AskAllFixtureMinimizationQuestions() accepted a tampered ladder")
	}
	if _, err := AskFixtureMinimizationQuestion(root, minimize.MinimizationQuestionSelection); err == nil {
		t.Fatal("AskFixtureMinimizationQuestion() accepted a tampered ladder")
	}
	if _, err := SaveFixtureMinimizationQuestionRound(root, filepath.Join(t.TempDir(), "round.json")); err == nil {
		t.Fatal("SaveFixtureMinimizationQuestionRound() accepted a tampered ladder")
	}
}
