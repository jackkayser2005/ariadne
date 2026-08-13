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
		receiptSHA256, err := ArchiveQuestionTransitionHistoryAnswerReceiptSHA256(receipt)
		if err != nil || receiptSHA256 != summary.ReceiptSHA256 {
			t.Fatalf("ArchiveQuestionTransitionHistoryAnswerReceiptSHA256(%q) = %q, %v; want %q", question.ID, receiptSHA256, err, summary.ReceiptSHA256)
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
		{name: "nested detail", data: mutateArchiveQuestionTransitionHistoryAnswerReceiptAnswer(t, valid, func(answer map[string]json.RawMessage) {
			answer["transitions"] = json.RawMessage(`-1`)
			answer["changed_transitions"] = json.RawMessage(`[999]`)
		}), want: "answer transitions is invalid"},
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

func TestValidateArchiveQuestionTransitionHistoryReceiptAnswerDetails(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := func() ArchiveQuestionTransitionHistoryAnswer {
		return ArchiveQuestionTransitionHistoryAnswer{
			SchemaVersion:           1,
			QuestionID:              "answer-state-transitions",
			Question:                "At which supplied boundaries did the bounded answer state change?",
			Result:                  "changed",
			TransitionHistorySHA256: digest,
			Transitions:             2,
			ChangedTransitions:      []int{1},
			IncomparableTransitions: []int{},
			ChangedEntries: []ArchiveQuestionTransitionHistoryChange{{
				Transition:           1,
				FromReflectionSHA256: digest,
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Directory:            "run-001",
				OlderState:           "observed",
				NewerState:           "unknown",
			}},
		}
	}
	repeated := func() ArchiveQuestionTransitionHistoryRepeatedAnswer {
		return ArchiveQuestionTransitionHistoryRepeatedAnswer{
			SchemaVersion:           1,
			QuestionID:              "answer-state-repeated-changes",
			Question:                "Did any safe archive entry change at more than one supplied boundary?",
			Result:                  "repeated",
			TransitionHistorySHA256: digest,
			Transitions:             2,
			RepeatedEntries: []ArchiveQuestionTransitionHistoryRepeatedChange{{
				Directory: "run-001",
				Changes: []ArchiveQuestionTransitionHistoryChange{
					{Transition: 1, FromReflectionSHA256: digest, ToReflectionSHA256: strings.Repeat("b", 64), Directory: "run-001", OlderState: "observed", NewerState: "unknown"},
					{Transition: 2, FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64), Directory: "run-001", OlderState: "unknown", NewerState: "unavailable"},
				},
			}},
		}
	}
	snapshot := func() ArchiveQuestionTransitionHistorySnapshotAnswer {
		return ArchiveQuestionTransitionHistorySnapshotAnswer{
			SchemaVersion:           1,
			QuestionID:              "answer-state-snapshot-summaries",
			Question:                "What bounded answer-state summary did each supplied reflection snapshot record?",
			Result:                  "available",
			TransitionHistorySHA256: digest,
			Snapshots:               2,
			SnapshotSummaries: []ArchiveQuestionTransitionSnapshot{
				{ReflectionSHA256: digest, Observed: 1, Checked: 1},
				{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
			},
		}
	}
	summary := func() ArchiveQuestionTransitionHistorySummaryAnswer {
		return ArchiveQuestionTransitionHistorySummaryAnswer{
			SchemaVersion:           1,
			QuestionID:              "answer-state-summary-changes",
			Question:                "Did the bounded answer-state summary change at any supplied boundary?",
			Result:                  "changed",
			TransitionHistorySHA256: digest,
			Transitions:             2,
			ChangedTransitions:      []int{2},
		}
	}

	if err := validateArchiveQuestionTransitionHistoryAnswer(base()); err != nil {
		t.Fatalf("valid history answer: %v", err)
	}
	if err := validateArchiveQuestionTransitionHistoryRepeatedAnswer(repeated()); err != nil {
		t.Fatalf("valid repeated answer: %v", err)
	}
	if err := validateArchiveQuestionTransitionHistorySnapshotAnswer(snapshot()); err != nil {
		t.Fatalf("valid snapshot answer: %v", err)
	}
	if err := validateArchiveQuestionTransitionHistorySummaryAnswer(summary()); err != nil {
		t.Fatalf("valid summary answer: %v", err)
	}

	tests := []struct {
		name string
		want string
		err  func() error
	}{
		{name: "history transitions", want: "answer transitions is invalid", err: func() error {
			answer := base()
			answer.Transitions = 0
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history schema", want: "answer schema_version is invalid", err: func() error {
			answer := base()
			answer.SchemaVersion = 2
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history question ID", want: "answer question ID is invalid", err: func() error {
			answer := base()
			answer.QuestionID = "other"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history question text", want: "answer question text is invalid", err: func() error {
			answer := base()
			answer.Question = "other"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history answer result", want: "answer result is invalid", err: func() error {
			answer := base()
			answer.Result = "invalid"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history answer digest", want: "answer transition_history_sha256 is invalid", err: func() error {
			answer := base()
			answer.TransitionHistorySHA256 = "bad"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history index", want: "ordering or range", err: func() error {
			answer := base()
			answer.ChangedTransitions = []int{0}
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history overlap", want: "indexes overlap", err: func() error {
			answer := base()
			answer.IncomparableTransitions = []int{1}
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history result", want: "result does not match detail", err: func() error {
			answer := base()
			answer.Result = "same"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history entry identity", want: "reflection identity", err: func() error {
			answer := base()
			answer.ChangedEntries[0].FromReflectionSHA256 = "bad"
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history transition identity consistency", want: "reflection identity is inconsistent", err: func() error {
			answer := base()
			answer.ChangedEntries = append(answer.ChangedEntries, ArchiveQuestionTransitionHistoryChange{
				Transition:           1,
				FromReflectionSHA256: strings.Repeat("c", 64),
				ToReflectionSHA256:   strings.Repeat("d", 64),
				Directory:            "run-002",
				OlderState:           "observed",
				NewerState:           "unknown",
			})
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history entry directory", want: "directory is invalid", err: func() error {
			answer := base()
			answer.ChangedEntries[0].Directory = ""
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history entry state", want: "state is invalid", err: func() error {
			answer := base()
			answer.ChangedEntries[0].OlderState = answer.ChangedEntries[0].NewerState
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "history entry count", want: "entry count does not match", err: func() error {
			answer := base()
			answer.ChangedEntries = nil
			return validateArchiveQuestionTransitionHistoryAnswer(answer)
		}},
		{name: "repeated transitions", want: "answer transitions is invalid", err: func() error {
			answer := repeated()
			answer.Transitions = 0
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated result", want: "entries do not match result", err: func() error {
			answer := repeated()
			answer.Result = "none"
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated empty", want: "entries do not match result", err: func() error {
			answer := repeated()
			answer.RepeatedEntries = nil
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated directory", want: "directory ordering", err: func() error {
			answer := repeated()
			answer.RepeatedEntries[0].Directory = ""
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated count", want: "needs multiple changes", err: func() error {
			answer := repeated()
			answer.RepeatedEntries[0].Changes = answer.RepeatedEntries[0].Changes[:1]
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated change", want: "changes are invalid", err: func() error {
			answer := repeated()
			answer.RepeatedEntries[0].Changes[1].Directory = "other"
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "repeated transition identity consistency", want: "reflection identity is inconsistent", err: func() error {
			answer := repeated()
			answer.RepeatedEntries = append(answer.RepeatedEntries, ArchiveQuestionTransitionHistoryRepeatedChange{
				Directory: "run-002",
				Changes: []ArchiveQuestionTransitionHistoryChange{
					{Transition: 1, FromReflectionSHA256: strings.Repeat("c", 64), ToReflectionSHA256: strings.Repeat("d", 64), Directory: "run-002", OlderState: "observed", NewerState: "unknown"},
					{Transition: 2, FromReflectionSHA256: strings.Repeat("d", 64), ToReflectionSHA256: strings.Repeat("e", 64), Directory: "run-002", OlderState: "unknown", NewerState: "unavailable"},
				},
			})
			return validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer)
		}},
		{name: "snapshot count", want: "answer snapshots is invalid", err: func() error {
			answer := snapshot()
			answer.Snapshots = 1
			return validateArchiveQuestionTransitionHistorySnapshotAnswer(answer)
		}},
		{name: "snapshot unavailable", want: "summaries do not match result", err: func() error {
			answer := snapshot()
			answer.Result = "unavailable"
			return validateArchiveQuestionTransitionHistorySnapshotAnswer(answer)
		}},
		{name: "snapshot length", want: "summary count does not match", err: func() error {
			answer := snapshot()
			answer.SnapshotSummaries = answer.SnapshotSummaries[:1]
			return validateArchiveQuestionTransitionHistorySnapshotAnswer(answer)
		}},
		{name: "summary transitions", want: "answer transitions is invalid", err: func() error {
			answer := summary()
			answer.Transitions = 0
			return validateArchiveQuestionTransitionHistorySummaryAnswer(answer)
		}},
		{name: "summary index", want: "ordering or range", err: func() error {
			answer := summary()
			answer.ChangedTransitions = []int{3}
			return validateArchiveQuestionTransitionHistorySummaryAnswer(answer)
		}},
		{name: "summary result", want: "result does not match detail", err: func() error {
			answer := summary()
			answer.Result = "same"
			return validateArchiveQuestionTransitionHistorySummaryAnswer(answer)
		}},
		{name: "summary unavailable", want: "changed transitions do not match", err: func() error {
			answer := summary()
			answer.Result = "unavailable"
			return validateArchiveQuestionTransitionHistorySummaryAnswer(answer)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.err(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	receipt, err := AskArchiveQuestionTransitionHistoryReceipt(historyPath, "answer-state-summary-changes")
	if err != nil {
		t.Fatal(err)
	}
	var answer ArchiveQuestionTransitionHistorySummaryAnswer
	if err := json.Unmarshal(receipt.Answer, &answer); err != nil {
		t.Fatal(err)
	}
	if err := validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256); err != nil {
		t.Fatalf("valid receipt metadata: %v", err)
	}
	for _, test := range []struct {
		name string
		want string
		call func() error
	}{
		{name: "schema", want: "answer schema_version is invalid", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, 2, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
		}},
		{name: "question", want: "answer question does not match receipt", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, "other", answer.Question, answer.Result, answer.TransitionHistorySHA256)
		}},
		{name: "text", want: "answer question does not match receipt", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, "other", answer.Result, answer.TransitionHistorySHA256)
		}},
		{name: "result", want: "answer result is invalid", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, "invalid", answer.TransitionHistorySHA256)
		}},
		{name: "result mismatch", want: "answer result does not match receipt", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, "same", answer.TransitionHistorySHA256)
		}},
		{name: "digest", want: "answer transition_history_sha256 does not match receipt", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, strings.Repeat("b", 64))
		}},
		{name: "invalid digest", want: "answer transition_history_sha256 does not match receipt", call: func() error {
			return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, "bad")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
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
