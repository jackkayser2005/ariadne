package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyArchiveQuestionTransitionHistoryAnswerReceipt(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	for _, question := range ArchiveQuestionTransitionHistoryQuestions() {
		receipt, err := AskArchiveQuestionTransitionHistoryReceipt(historyPath, question.ID)
		if err != nil {
			t.Fatalf("AskArchiveQuestionTransitionHistoryReceipt(%q): %v", question.ID, err)
		}
		path := filepath.Join(t.TempDir(), question.ID+".json")
		data, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}

		summary, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(path)
		if err != nil {
			t.Fatalf("VerifyArchiveQuestionTransitionHistoryAnswerReceipt(%q): %v", question.ID, err)
		}
		if summary.SchemaVersion != 1 || summary.QuestionID != question.ID || summary.Question != question.Text ||
			!validDigest(summary.TransitionHistorySHA256) || !validDigest(summary.ReceiptSHA256) {
			t.Fatalf("verification summary for %q = %#v", question.ID, summary)
		}
		if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
			t.Fatalf("receipt %q exposed a captured value: %s", question.ID, data)
		}
	}

	receipt, err := AskArchiveQuestionTransitionHistoryReceipt(historyPath, "answer-state-summary-changes")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	formattedPath := filepath.Join(t.TempDir(), "formatted-receipt.json")
	if err := os.WriteFile(formattedPath, append(formatted.Bytes(), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	formattedSummary, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(formattedPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalSummary, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(writeArchiveQuestionTransitionHistoryAnswerReceiptBytes(t, data))
	if err != nil {
		t.Fatal(err)
	}
	if formattedSummary.ReceiptSHA256 != canonicalSummary.ReceiptSHA256 {
		t.Fatalf("formatted receipt identity = %q, canonical = %q", formattedSummary.ReceiptSHA256, canonicalSummary.ReceiptSHA256)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryAnswerReceiptRejectsInvalidReceipts(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	receipt, err := AskArchiveQuestionTransitionHistoryReceipt(historyPath, "answer-state-summary-changes")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(receipt)
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
		{name: "question ID", data: bytes.Replace(valid, []byte(`"question_id":"answer-state-summary-changes"`), []byte(`"question_id":"other"`), 1), want: "question ID is invalid"},
		{name: "question text", data: bytes.Replace(valid, []byte(`"question":"Did the bounded answer-state summary change at any supplied boundary?"`), []byte(`"question":"other"`), 1), want: "question text does not match catalog"},
		{name: "answer result", data: mutateArchiveQuestionTransitionHistoryAnswerReceipt(t, valid, func(object map[string]json.RawMessage) {
			object["result"] = json.RawMessage(`"changed"`)
		}), want: "answer result does not match receipt"},
		{name: "invalid result", data: mutateArchiveQuestionTransitionHistoryAnswerReceiptAnswer(t, mutateArchiveQuestionTransitionHistoryAnswerReceipt(t, valid, func(object map[string]json.RawMessage) {
			object["result"] = json.RawMessage(`"invalid"`)
		}), func(answer map[string]json.RawMessage) {
			answer["result"] = json.RawMessage(`"invalid"`)
		}), want: "answer result is invalid"},
		{name: "nested unknown", data: mutateArchiveQuestionTransitionHistoryAnswerReceiptAnswer(t, valid, func(answer map[string]json.RawMessage) {
			answer["extra"] = json.RawMessage(`true`)
		}), want: "unknown field"},
		{name: "answer array", data: mutateArchiveQuestionTransitionHistoryAnswerReceipt(t, valid, func(object map[string]json.RawMessage) {
			object["answer"] = json.RawMessage(`[]`)
		}), want: "cannot unmarshal array"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(writeArchiveQuestionTransitionHistoryAnswerReceiptBytes(t, test.data)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyArchiveQuestionTransitionHistoryAnswerReceipt() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyArchiveQuestionTransitionHistoryAnswerReceiptRequiresPath(t *testing.T) {
	if _, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty receipt path error = %v", err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("accepted missing receipt path")
	}
	if _, err := VerifyArchiveQuestionTransitionHistoryAnswerReceipt(writeArchiveQuestionTransitionHistoryAnswerReceiptBytes(t, bytes.Repeat([]byte("x"), maxOutputBytes+1))); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized receipt error = %v", err)
	}
}

func writeArchiveQuestionTransitionHistoryAnswerReceiptBytes(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive-question-answer-receipt.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mutateArchiveQuestionTransitionHistoryAnswerReceipt(t *testing.T, data []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	mutated, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return mutated
}

func mutateArchiveQuestionTransitionHistoryAnswerReceiptAnswer(t *testing.T, data []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return mutateArchiveQuestionTransitionHistoryAnswerReceipt(t, data, func(object map[string]json.RawMessage) {
		var answer map[string]json.RawMessage
		if err := json.Unmarshal(object["answer"], &answer); err != nil {
			t.Fatal(err)
		}
		mutate(answer)
		encoded, err := json.Marshal(answer)
		if err != nil {
			t.Fatal(err)
		}
		object["answer"] = encoded
	})
}
