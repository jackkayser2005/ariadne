package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveQuestionTransitionHistoryAcceptanceRecord(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveArchiveQuestionTransitionHistoryQuestionRound(historyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	if _, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(historyPath, "answer-state-transitions", receiptPath); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(t.TempDir(), "acceptance.json")
	summary, err := SaveArchiveQuestionTransitionHistoryAcceptanceRecord(roundPath, receiptPath, recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || summary.QuestionID != "answer-state-transitions" || !validDigest(summary.TransitionHistorySHA256) || !validDigest(summary.QuestionRoundSHA256) || !validDigest(summary.ReceiptSHA256) || !validDigest(summary.AcceptanceSHA256) {
		t.Fatalf("save summary = %#v", summary)
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("acceptance record exposed a captured value: %s", data)
	}
	verified, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(recordPath)
	if err != nil || verified != summary {
		t.Fatalf("verified acceptance record = %#v, %v; want %#v", verified, err, summary)
	}

	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	formattedPath := filepath.Join(t.TempDir(), "formatted-acceptance.json")
	if err := os.WriteFile(formattedPath, append(formatted.Bytes(), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	formattedSummary, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(formattedPath)
	if err != nil || formattedSummary.AcceptanceSHA256 != summary.AcceptanceSHA256 {
		t.Fatalf("formatted acceptance record = %#v, %v; want %#v", formattedSummary, err, summary)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAcceptanceRecord(roundPath, receiptPath, recordPath); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("acceptance record overwrite error = %v", err)
	}
}

func TestArchiveQuestionTransitionHistoryAcceptanceRecordRejectsInvalidRecords(t *testing.T) {
	record := ArchiveQuestionTransitionHistoryAcceptanceRecord{
		SchemaVersion:           1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
		QuestionRoundSHA256:     strings.Repeat("b", 64),
		QuestionID:              "answer-state-transitions",
		ReceiptSHA256:           strings.Repeat("c", 64),
	}
	valid, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "invalid JSON"},
		{name: "duplicate", data: bytes.Replace(valid, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1), want: "duplicate key"},
		{name: "unknown", data: bytes.Replace(valid, []byte("{"), []byte(`{"extra":true,`), 1), want: "unknown field"},
		{name: "trailing", data: append(append([]byte(nil), valid...), []byte("{}")...), want: "trailing data"},
		{name: "schema", data: bytes.Replace(valid, []byte(`"schema_version":1`), []byte(`"schema_version":2`), 1), want: "unsupported schema_version"},
		{name: "history digest", data: bytes.Replace(valid, []byte(strings.Repeat("a", 64)), []byte("bad"), 1), want: "transition_history_sha256 is invalid"},
		{name: "round digest", data: bytes.Replace(valid, []byte(strings.Repeat("b", 64)), []byte("bad"), 1), want: "question_round_sha256 is invalid"},
		{name: "question ID", data: bytes.Replace(valid, []byte(`"answer-state-transitions"`), []byte(`"other"`), 1), want: "question ID is invalid"},
		{name: "receipt digest", data: bytes.Replace(valid, []byte(strings.Repeat("c", 64)), []byte("bad"), 1), want: "receipt_sha256 is invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "acceptance.json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyArchiveQuestionTransitionHistoryAcceptanceRecord() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestArchiveQuestionTransitionHistoryAcceptanceRecordRequiresPaths(t *testing.T) {
	if _, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty record path error = %v", err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("accepted missing acceptance record")
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAcceptanceRecord(" ", " ", " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty save path error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxOutputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized record error = %v", err)
	}
}

func TestSaveArchiveQuestionTransitionHistoryAcceptanceRecordRejectsMismatches(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	historyPath := writeArchiveQuestionTransitionHistory(t, history)
	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveArchiveQuestionTransitionHistoryQuestionRound(historyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	roundSummary, err := VerifyArchiveQuestionTransitionHistoryQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	otherHistory := history
	otherHistory.Transitions = append([]ArchiveQuestionTransition(nil), history.Transitions...)
	otherHistory.Transitions[0].FromReflectionSHA256 = strings.Repeat("f", 64)
	otherHistoryPath := writeArchiveQuestionTransitionHistory(t, otherHistory)
	otherReceiptPath := filepath.Join(t.TempDir(), "other-receipt.json")
	if _, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(otherHistoryPath, "answer-state-transitions", otherReceiptPath); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAcceptanceRecord(roundPath, otherReceiptPath, filepath.Join(t.TempDir(), "mismatch.json")); err == nil || !strings.Contains(err.Error(), "history identities do not match") {
		t.Fatalf("history mismatch error = %v", err)
	}

	mutatedRound := AnswerArchiveQuestionTransitionHistoryQuestionRound(history, roundSummary.TransitionHistorySHA256)
	mutatedRound.Questions = append([]ArchiveQuestionTransitionHistoryQuestionRoundItem(nil), mutatedRound.Questions...)
	mutatedRound.Questions[0].Result = "same"
	mutatedRoundPath := writeQuestionRoundForComparison(t, mutatedRound)
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	if _, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(historyPath, "answer-state-transitions", receiptPath); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAcceptanceRecord(mutatedRoundPath, receiptPath, filepath.Join(t.TempDir(), "result-mismatch.json")); err == nil || !strings.Contains(err.Error(), "results do not match") {
		t.Fatalf("result mismatch error = %v", err)
	}
}
