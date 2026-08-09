package bundle

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyArchiveQuestionTransitionHistory(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	path := writeArchiveQuestionTransitionHistory(t, history)

	summary, err := VerifyArchiveQuestionTransitionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 2 || summary.HistoryID != history.HistoryID ||
		summary.HistoryQuestion != history.HistoryQuestion || summary.QuestionID != history.QuestionID ||
		summary.OrderBasis != "caller" || summary.Snapshots != 2 || summary.Transitions != 1 ||
		!validDigest(summary.TransitionHistorySHA256) {
		t.Fatalf("verification summary = %#v", summary)
	}
	readHistory, readSummary, err := ReadArchiveQuestionTransitionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if readHistory.HistoryID != history.HistoryID || len(readHistory.Transitions) != len(history.Transitions) || readSummary != summary {
		t.Fatalf("ReadArchiveQuestionTransitionHistory() = %#v, %#v; want %#v, %#v", readHistory, readSummary, history, summary)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		t.Fatal(err)
	}
	formattedSummary, err := VerifyArchiveQuestionTransitionHistory(writeArchiveQuestionTransitionHistoryBytes(t, formatted.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if formattedSummary.TransitionHistorySHA256 != summary.TransitionHistorySHA256 {
		t.Fatalf("formatted transition history identity = %q, want %q", formattedSummary.TransitionHistorySHA256, summary.TransitionHistorySHA256)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryRejectsInvalidLedgers(t *testing.T) {
	valid := archiveQuestionTransitionHistoryBytes(t, validArchiveQuestionTransitionHistory())
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
		{name: "history ID", data: bytes.Replace(valid, []byte(`"history_id":"answer-state-transitions"`), []byte(`"history_id":"other"`), 1), want: "history ID is invalid"},
		{name: "missing order", data: bytes.Replace(valid, []byte(`,"order_basis":"caller"`), nil, 1), want: "missing required field \"order_basis\""},
		{name: "identity", data: bytes.Replace(valid, []byte(strings.Repeat("a", 64)), []byte("bad"), 1), want: "transition reflection identity is invalid"},
		{name: "result", data: bytes.Replace(valid, []byte(`"result":"changed"`), []byte(`"result":"other"`), 1), want: "transition result is invalid"},
		{name: "negative count", data: bytes.Replace(valid, []byte(`"changed":1`), []byte(`"changed":-1`), 1), want: "transition counts are invalid"},
		{name: "changed exceeds compared", data: bytes.Replace(valid, []byte(`"changed":1`), []byte(`"changed":2`), 1), want: "transition counts are invalid"},
		{name: "same counts", data: bytes.Replace(valid, []byte(`"result":"changed"`), []byte(`"result":"same"`), 1), want: "same transition counts are invalid"},
		{name: "changed counts", data: bytes.Replace(valid, []byte(`"changed":1`), []byte(`"changed":0`), 1), want: "changed transition counts are invalid"},
		{name: "state change count", data: bytes.Replace(valid, []byte(`"state_changes":[{"directory":"run-001","older_state":"observed","newer_state":"unknown"}]`), []byte(`"state_changes":[]`), 1), want: "transition state change count does not match changed count"},
		{name: "state changes type", data: bytes.Replace(valid, []byte(`"state_changes":[{"directory":"run-001","older_state":"observed","newer_state":"unknown"}]`), []byte(`"state_changes":{}`), 1), want: "cannot unmarshal object"},
		{name: "state change unknown field", data: bytes.Replace(valid, []byte(`{"directory":"run-001","older_state":"observed","newer_state":"unknown"}`), []byte(`{"extra":true,"directory":"run-001","older_state":"observed","newer_state":"unknown"}`), 1), want: "unknown field"},
		{name: "state change directory", data: bytes.Replace(valid, []byte(`"directory":"run-001"`), []byte(`"directory":"../run"`), 1), want: "transition state change directory ordering is invalid"},
		{name: "state change state", data: bytes.Replace(valid, []byte(`"newer_state":"unknown"`), []byte(`"newer_state":"inferred"`), 1), want: "transition state change state is invalid"},
		{name: "state change no-op", data: bytes.Replace(valid, []byte(`"newer_state":"unknown"`), []byte(`"newer_state":"observed"`), 1), want: "transition state change does not change state"},
		{name: "incomparable counts", data: bytes.Replace(valid, []byte(`"result":"changed"`), []byte(`"result":"incomparable"`), 1), want: "incomparable transition counts are invalid"},
		{name: "snapshot count", data: bytes.Replace(valid, []byte(`"snapshots":2`), []byte(`"snapshots":3`), 1), want: "transition count does not match snapshots"},
		{name: "too few snapshots", data: bytes.Replace(valid, []byte(`"snapshots":2`), []byte(`"snapshots":1`), 1), want: "snapshots must be at least two"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := VerifyArchiveQuestionTransitionHistory(writeArchiveQuestionTransitionHistoryBytes(t, test.data)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyArchiveQuestionTransitionHistory() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyArchiveQuestionTransitionHistoryAcceptsLegacyLedger(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 1
	history.Transitions[0].StateChanges = nil
	data := archiveQuestionTransitionHistoryBytes(t, history)
	if bytes.Contains(data, []byte("state_changes")) {
		t.Fatalf("legacy transition history serialized state changes: %s", data)
	}
	summary, err := VerifyArchiveQuestionTransitionHistory(writeArchiveQuestionTransitionHistoryBytes(t, data))
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || summary.Transitions != 1 || !validDigest(summary.TransitionHistorySHA256) {
		t.Fatalf("legacy verification summary = %#v", summary)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryAcceptsConnectedLegacyTransitions(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 1
	history.Snapshots = 3
	history.Transitions[0].StateChanges = nil
	history.Transitions = append(history.Transitions, ArchiveQuestionTransition{
		FromReflectionSHA256: strings.Repeat("b", 64),
		ToReflectionSHA256:   strings.Repeat("c", 64),
		Result:               "same",
	})
	if _, err := VerifyArchiveQuestionTransitionHistory(writeArchiveQuestionTransitionHistory(t, history)); err != nil {
		t.Fatalf("VerifyArchiveQuestionTransitionHistory() connected legacy error = %v", err)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryRejectsUnorderedStateChanges(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.Transitions[0].StateChanges = []ArchiveQuestionStateChange{
		{Directory: "b-run", OlderState: "observed", NewerState: "unknown"},
		{Directory: "a-run", OlderState: "observed", NewerState: "unknown"},
	}
	history.Transitions[0].Compared = 2
	history.Transitions[0].Changed = 2
	if _, err := VerifyArchiveQuestionTransitionHistory(writeArchiveQuestionTransitionHistory(t, history)); err == nil || !strings.Contains(err.Error(), "transition state change directory ordering is invalid") {
		t.Fatalf("unordered state changes error = %v", err)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryRejectsDisconnectedTransitions(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.Snapshots = 3
	history.Transitions = append(history.Transitions, ArchiveQuestionTransition{
		FromReflectionSHA256: strings.Repeat("c", 64),
		ToReflectionSHA256:   strings.Repeat("d", 64),
		Result:               "same",
	})
	path := writeArchiveQuestionTransitionHistory(t, history)
	if _, err := VerifyArchiveQuestionTransitionHistory(path); err == nil || !strings.Contains(err.Error(), "not contiguous") {
		t.Fatalf("VerifyArchiveQuestionTransitionHistory() error = %v, want disconnected identity error", err)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryRejectsOversizedLedger(t *testing.T) {
	path := writeArchiveQuestionTransitionHistoryBytes(t, bytes.Repeat([]byte("x"), maxOutputBytes+1))
	if _, err := VerifyArchiveQuestionTransitionHistory(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("VerifyArchiveQuestionTransitionHistory() oversized error = %v", err)
	}
}

func TestVerifyArchiveQuestionTransitionHistoryRequiresPath(t *testing.T) {
	if _, err := VerifyArchiveQuestionTransitionHistory(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("VerifyArchiveQuestionTransitionHistory() empty path error = %v", err)
	}
	if _, err := VerifyArchiveQuestionTransitionHistory(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("VerifyArchiveQuestionTransitionHistory() accepted missing path")
	}
}

func validArchiveQuestionTransitionHistory() ArchiveQuestionTransitionHistory {
	return ArchiveQuestionTransitionHistory{
		SchemaVersion:   2,
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		QuestionID:      "counterfactual-change",
		Question:        "Did changing the declared variable influence an observed output?",
		OrderBasis:      "caller",
		Snapshots:       2,
		Transitions: []ArchiveQuestionTransition{
			{
				FromReflectionSHA256: strings.Repeat("a", 64),
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Result:               "changed",
				Compared:             1,
				Changed:              1,
				StateChanges: []ArchiveQuestionStateChange{{
					Directory:  "run-001",
					OlderState: "observed",
					NewerState: "unknown",
				}},
			},
		},
	}
}

func writeArchiveQuestionTransitionHistory(t *testing.T, history ArchiveQuestionTransitionHistory) string {
	t.Helper()
	return writeArchiveQuestionTransitionHistoryBytes(t, archiveQuestionTransitionHistoryBytes(t, history))
}

func archiveQuestionTransitionHistoryBytes(t *testing.T, history ArchiveQuestionTransitionHistory) []byte {
	t.Helper()
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeArchiveQuestionTransitionHistoryBytes(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive-question-transitions.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
