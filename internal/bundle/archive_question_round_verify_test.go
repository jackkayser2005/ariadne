package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyArchiveQuestionTransitionHistoryQuestionRound(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	round, err := AskArchiveQuestionTransitionHistoryQuestionRound(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	roundSHA256, err := ArchiveQuestionTransitionHistoryQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(t.TempDir(), "round.json")
	summary, err := SaveArchiveQuestionTransitionHistoryQuestionRound(historyPath, roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 2 || summary.TransitionHistorySHA256 != round.TransitionHistorySHA256 || summary.Questions != 4 || summary.RoundSHA256 != roundSHA256 {
		t.Fatalf("save summary = %#v", summary)
	}
	data, err := os.ReadFile(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("round exposed a captured value: %s", data)
	}
	verified, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if verified != summary {
		t.Fatalf("verification summary = %#v, want %#v", verified, summary)
	}

	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	formattedPath := filepath.Join(t.TempDir(), "formatted-round.json")
	if err := os.WriteFile(formattedPath, append(formatted.Bytes(), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	formattedSummary, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(formattedPath)
	if err != nil {
		t.Fatal(err)
	}
	if formattedSummary.RoundSHA256 != summary.RoundSHA256 {
		t.Fatalf("formatted round identity = %q, want %q", formattedSummary.RoundSHA256, summary.RoundSHA256)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryQuestionRound(historyPath, roundPath); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("round save overwrite error = %v", err)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryQuestionRoundRejectsInvalidRounds(t *testing.T) {
	round := AnswerArchiveQuestionTransitionHistoryQuestionRound(validArchiveQuestionTransitionHistory(), strings.Repeat("a", 64))
	valid, err := json.Marshal(round)
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
		{name: "schema", data: bytes.Replace(valid, []byte(`"schema_version":2`), []byte(`"schema_version":3`), 1), want: "unsupported schema_version"},
		{name: "history question ID", data: mutateQuestionRound(t, valid, func(round *ArchiveQuestionTransitionHistoryQuestionRoundAnswer) {
			round.HistoryQuestionID = "other"
		}), want: "history_question_id is invalid"},
		{name: "history digest", data: bytes.Replace(valid, []byte(strings.Repeat("a", 64)), []byte("bad"), 1), want: "transition_history_sha256 is invalid"},
		{name: "count", data: mutateQuestionRound(t, valid, func(round *ArchiveQuestionTransitionHistoryQuestionRoundAnswer) {
			round.Questions = round.Questions[:1]
		}), want: "question count does not match catalog"},
		{name: "question ID", data: mutateQuestionRound(t, valid, func(round *ArchiveQuestionTransitionHistoryQuestionRoundAnswer) {
			round.Questions[0].QuestionID = "other"
		}), want: "question ID or order"},
		{name: "question text", data: mutateQuestionRound(t, valid, func(round *ArchiveQuestionTransitionHistoryQuestionRoundAnswer) {
			round.Questions[0].Question = "other"
		}), want: "question text"},
		{name: "result", data: mutateQuestionRound(t, valid, func(round *ArchiveQuestionTransitionHistoryQuestionRoundAnswer) {
			round.Questions[0].Result = "invalid"
		}), want: "question result"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "round.json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyArchiveQuestionTransitionHistoryQuestionRound() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyArchiveQuestionTransitionHistoryQuestionRoundRequiresPath(t *testing.T) {
	if _, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty round path error = %v", err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("accepted missing round path")
	}
	if _, err := SaveArchiveQuestionTransitionHistoryQuestionRound(" ", " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty save path error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxOutputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized round error = %v", err)
	}
}

func mutateQuestionRound(t *testing.T, data []byte, mutate func(*ArchiveQuestionTransitionHistoryQuestionRoundAnswer)) []byte {
	t.Helper()
	var round ArchiveQuestionTransitionHistoryQuestionRoundAnswer
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatal(err)
	}
	mutate(&round)
	mutated, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	return mutated
}
