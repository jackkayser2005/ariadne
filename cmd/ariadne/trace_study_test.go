package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceStudyCommands(t *testing.T) {
	summary := trace.StudyVerificationSummary{
		SchemaVersion:       1,
		ContrastSHA256:      strings.Repeat("a", 64),
		OrderBasis:          trace.ReplicationStudyOrderBasis,
		Runs:                2,
		Pairs:               4,
		SupportedRuns:       2,
		ResetConfirmedPairs: 4,
		BalancedRuns:        2,
		CompletePairs:       4,
		ChangedRuns:         2,
		MixedRuns:           0,
		Outcome:             trace.ReplicatedChange,
		EvidenceState:       "observed",
		Reason:              "every supported replicated run contains a safe category difference",
		StudySHA256:         strings.Repeat("b", 64),
	}
	args := []string{"--contrast-sha256", summary.ContrastSHA256, "study.json", "ledger-1.json", "round-1.json", "ledger-2.json", "round-2.json"}
	save := func(contrast string, inputs []trace.StudyInput, output string) (trace.StudyVerificationSummary, error) {
		if contrast != summary.ContrastSHA256 || output != "study.json" || len(inputs) != 2 || inputs[0].LedgerPath != "ledger-1.json" || inputs[1].RoundPath != "round-2.json" {
			t.Fatalf("save inputs = %#v, contrast = %q, output = %q", inputs, contrast, output)
		}
		return summary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceStudySave(args, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study saved") || !strings.Contains(stdout.String(), "outcome: replicated-change") || stderr.Len() != 0 {
		t.Fatalf("save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudySave(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var saved trace.StudyVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil || saved != summary {
		t.Fatalf("JSON save = %#v, err=%v", saved, err)
	}

	verify := func(path string) (trace.StudyVerificationSummary, error) {
		if path != "study.json" {
			t.Fatalf("verify path = %q", path)
		}
		return summary, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"--expect-sha256", summary.StudySHA256, "study.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study verified") || stderr.Len() != 0 {
		t.Fatalf("verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyVerify([]string{"--json", "study.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var verified trace.StudyVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verified); err != nil || verified != summary {
		t.Fatalf("JSON verify = %#v, err=%v", verified, err)
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "save usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave([]string{"study.json", "ledger.json", "round.json"}, out, errOut, save)
		}},
		{name: "invalid contrast", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave([]string{"--contrast-sha256=bad", "study.json", "ledger.json", "round.json", "ledger-2.json", "round-2.json"}, out, errOut, save)
		}},
		{name: "verify usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify(nil, out, errOut, verify)
		}},
		{name: "invalid expected study", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"--expect-sha256=bad", "study.json"}, out, errOut, verify)
		}},
		{name: "save error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave(args, out, errOut, func(string, []trace.StudyInput, string) (trace.StudyVerificationSummary, error) {
				return trace.StudyVerificationSummary{}, errors.New("save failed safely")
			})
		}},
		{name: "verify error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"study.json"}, out, errOut, func(string) (trace.StudyVerificationSummary, error) {
				return trace.StudyVerificationSummary{}, errors.New("verify failed safely")
			})
		}},
		{name: "expected mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"--expect-sha256=" + strings.Repeat("c", 64), "study.json"}, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}

	stderr.Reset()
	if exitCode := runTraceStudySave(args, failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudySave(append([]string{"--json"}, args...), failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"study.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"--json", "study.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestWriteTraceStudySummary(t *testing.T) {
	if err := writeTraceStudySummary(failingWriter{}, "study", trace.StudyVerificationSummary{}); err == nil {
		t.Fatal("writeTraceStudySummary() accepted a failing writer")
	}
}

func TestRunTraceStudyQuestionCommands(t *testing.T) {
	answer := trace.StudyQuestionAnswer{
		SchemaVersion: 1,
		QuestionID:    trace.StudyQuestionOutcome,
		Question:      "What aggregate outcome did the independent runs produce?",
		Result:        string(trace.ReplicatedChange),
		EvidenceState: "observed",
		Outcome:       trace.ReplicatedChange,
		StudySHA256:   strings.Repeat("a", 64),
		Runs:          2,
		Pairs:         4,
		SupportedRuns: 2,
		BalancedRuns:  2,
		CompletePairs: 4,
		ChangedRuns:   2,
		Reason:        "every supported replicated run contains a safe category difference",
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceStudyQuestions(nil, &stdout, &stderr, trace.ReplicationStudyQuestions); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study question catalog") || !strings.Contains(stdout.String(), trace.StudyQuestionConsistency) || stderr.Len() != 0 {
		t.Fatalf("questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyQuestions([]string{"--json"}, &stdout, &stderr, trace.ReplicationStudyQuestions); exitCode != 0 {
		t.Fatalf("JSON questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var catalog []trace.StudyQuestion
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil || len(catalog) != 3 {
		t.Fatalf("catalog = %#v, err=%v", catalog, err)
	}

	ask := func(path, questionID string) (trace.StudyQuestionAnswer, error) {
		if path != "study.json" || questionID != trace.StudyQuestionOutcome {
			t.Fatalf("ask = %q, %q", path, questionID)
		}
		return answer, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceStudyAsk([]string{"study.json", trace.StudyQuestionOutcome}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "result: replicated-change") || stderr.Len() != 0 {
		t.Fatalf("ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAsk([]string{"--json", "study.json", trace.StudyQuestionOutcome}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var asked trace.StudyQuestionAnswer
	if err := json.Unmarshal(stdout.Bytes(), &asked); err != nil || asked != answer {
		t.Fatalf("asked = %#v, err=%v", asked, err)
	}

	askAll := func(path string) ([]trace.StudyQuestionAnswer, error) {
		if path != "study.json" {
			t.Fatalf("ask all path = %q", path)
		}
		return []trace.StudyQuestionAnswer{answer}, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceStudyAskAll([]string{"study.json"}, &stdout, &stderr, askAll); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study questions answered") || stderr.Len() != 0 {
		t.Fatalf("ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskAll([]string{"--json", "study.json"}, &stdout, &stderr, askAll); exitCode != 0 {
		t.Fatalf("JSON ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var askedAll []trace.StudyQuestionAnswer
	if err := json.Unmarshal(stdout.Bytes(), &askedAll); err != nil || len(askedAll) != 1 || askedAll[0] != answer {
		t.Fatalf("asked all = %#v, err=%v", askedAll, err)
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "questions usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyQuestions([]string{"extra"}, out, errOut, trace.ReplicationStudyQuestions)
		}},
		{name: "ask usage", call: func(out, errOut *bytes.Buffer) int { return runTraceStudyAsk([]string{"study.json"}, out, errOut, ask) }},
		{name: "ask all usage", call: func(out, errOut *bytes.Buffer) int { return runTraceStudyAskAll(nil, out, errOut, askAll) }},
		{name: "ask error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAsk([]string{"study.json", trace.StudyQuestionOutcome}, out, errOut, func(string, string) (trace.StudyQuestionAnswer, error) {
				return trace.StudyQuestionAnswer{}, errors.New("ask failed safely")
			})
		}},
		{name: "ask all error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAll([]string{"study.json"}, out, errOut, func(string) ([]trace.StudyQuestionAnswer, error) { return nil, errors.New("ask all failed safely") })
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}

	for _, test := range []struct {
		name string
		call func(io.Writer, *bytes.Buffer) int
	}{
		{name: "questions human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyQuestions(nil, out, errOut, trace.ReplicationStudyQuestions)
		}},
		{name: "questions JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyQuestions([]string{"--json"}, out, errOut, trace.ReplicationStudyQuestions)
		}},
		{name: "ask human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAsk([]string{"study.json", trace.StudyQuestionOutcome}, out, errOut, ask)
		}},
		{name: "ask JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAsk([]string{"--json", "study.json", trace.StudyQuestionOutcome}, out, errOut, ask)
		}},
		{name: "ask all human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAll([]string{"study.json"}, out, errOut, askAll)
		}},
		{name: "ask all JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAll([]string{"--json", "study.json"}, out, errOut, askAll)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if exitCode := test.call(failingWriter{}, &errOut); exitCode != 1 || !strings.Contains(errOut.String(), "write output") {
				t.Fatalf("call = %d, stderr=%q", exitCode, errOut.String())
			}
		})
	}
}

func TestWriteTraceStudyAnswer(t *testing.T) {
	if err := writeTraceStudyAnswer(failingWriter{}, trace.StudyQuestionAnswer{}); err == nil {
		t.Fatal("writeTraceStudyAnswer() accepted a failing writer")
	}
}

func TestRunTraceStudyDurableQuestionCommands(t *testing.T) {
	roundSummary := trace.ReplicationStudyQuestionRoundVerificationSummary{
		SchemaVersion: 1,
		StudySHA256:   strings.Repeat("a", 64),
		Questions:     3,
		RoundSHA256:   strings.Repeat("b", 64),
	}
	answer := trace.StudyQuestionAnswer{
		SchemaVersion: 1,
		QuestionID:    trace.StudyQuestionOutcome,
		Question:      "What aggregate outcome did the independent runs produce?",
		Result:        string(trace.ReplicatedChange),
		EvidenceState: "observed",
		Outcome:       trace.ReplicatedChange,
		StudySHA256:   roundSummary.StudySHA256,
		Runs:          2,
		Pairs:         4,
		SupportedRuns: 2,
		Reason:        "every supported replicated run contains a safe category difference",
	}
	receipt := trace.ReplicationStudyQuestionReceipt{
		StudyQuestionAnswer: answer,
		RoundSHA256:         roundSummary.RoundSHA256,
	}
	receiptSummary := trace.ReplicationStudyQuestionReceiptVerificationSummary{
		SchemaVersion: 1,
		QuestionID:    answer.QuestionID,
		Question:      answer.Question,
		Result:        answer.Result,
		EvidenceState: answer.EvidenceState,
		StudySHA256:   answer.StudySHA256,
		RoundSHA256:   roundSummary.RoundSHA256,
		ReceiptSHA256: strings.Repeat("c", 64),
		Outcome:       answer.Outcome,
	}
	var stdout, stderr bytes.Buffer
	saveRound := func(studyPath, roundPath string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error) {
		if studyPath != "study.json" || roundPath != "round.json" {
			t.Fatalf("save round paths = %q, %q", studyPath, roundPath)
		}
		return roundSummary, nil
	}
	if exitCode := runTraceStudyAskAllSave([]string{"study.json", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round saved") {
		t.Fatalf("save round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskAllSave([]string{"--json", "study.json", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 {
		t.Fatalf("JSON save round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var savedRound trace.ReplicationStudyQuestionRoundVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &savedRound); err != nil || savedRound != roundSummary {
		t.Fatalf("saved round = %#v, err=%v", savedRound, err)
	}
	verifyRound := func(path string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error) {
		if path != "round.json" {
			t.Fatalf("verify round path = %q", path)
		}
		return roundSummary, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceStudyAskAllVerify([]string{"--expect-sha256", roundSummary.RoundSHA256, "round.json"}, &stdout, &stderr, verifyRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round verified") {
		t.Fatalf("verify round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskAllVerify([]string{"--json", "round.json"}, &stdout, &stderr, verifyRound); exitCode != 0 {
		t.Fatalf("JSON verify round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var verifiedRound trace.ReplicationStudyQuestionRoundVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verifiedRound); err != nil || verifiedRound != roundSummary {
		t.Fatalf("verified round = %#v, err=%v", verifiedRound, err)
	}
	askReceipt := func(path, questionID string) (trace.ReplicationStudyQuestionReceipt, error) {
		if path != "round.json" || questionID != trace.StudyQuestionOutcome {
			t.Fatalf("ask receipt args = %q, %q", path, questionID)
		}
		return receipt, nil
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceipt([]string{"round.json", trace.StudyQuestionOutcome}, &stdout, &stderr, askReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "question receipt") {
		t.Fatalf("ask receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceipt([]string{"--json", "round.json", trace.StudyQuestionOutcome}, &stdout, &stderr, askReceipt); exitCode != 0 {
		t.Fatalf("JSON ask receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var askedReceipt trace.ReplicationStudyQuestionReceipt
	if err := json.Unmarshal(stdout.Bytes(), &askedReceipt); err != nil || askedReceipt.QuestionID != receipt.QuestionID || askedReceipt.RoundSHA256 != receipt.RoundSHA256 {
		t.Fatalf("asked receipt = %#v, err=%v", askedReceipt, err)
	}
	saveReceipt := func(roundPath, questionID, receiptPath string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error) {
		if roundPath != "round.json" || questionID != trace.StudyQuestionOutcome || receiptPath != "receipt.json" {
			t.Fatalf("save receipt args = %q, %q, %q", roundPath, questionID, receiptPath)
		}
		return receiptSummary, nil
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceiptSave([]string{"round.json", trace.StudyQuestionOutcome, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt saved") {
		t.Fatalf("save receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceiptSave([]string{"--json", "round.json", trace.StudyQuestionOutcome, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 {
		t.Fatalf("JSON save receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var savedReceipt trace.ReplicationStudyQuestionReceiptVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &savedReceipt); err != nil || savedReceipt != receiptSummary {
		t.Fatalf("saved receipt = %#v, err=%v", savedReceipt, err)
	}
	verifyReceipt := func(path string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error) {
		if path != "receipt.json" {
			t.Fatalf("verify receipt path = %q", path)
		}
		return receiptSummary, nil
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceiptVerify([]string{"--expect-sha256", receiptSummary.ReceiptSHA256, "receipt.json"}, &stdout, &stderr, verifyReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt verified") {
		t.Fatalf("verify receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyAskReceiptVerify([]string{"--json", "receipt.json"}, &stdout, &stderr, verifyReceipt); exitCode != 0 {
		t.Fatalf("JSON verify receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var verifiedReceipt trace.ReplicationStudyQuestionReceiptVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verifiedReceipt); err != nil || verifiedReceipt != receiptSummary {
		t.Fatalf("verified receipt = %#v, err=%v", verifiedReceipt, err)
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "round save usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllSave([]string{"study.json"}, out, errOut, saveRound)
		}},
		{name: "round save error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllSave([]string{"study.json", "round.json"}, out, errOut, func(string, string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error) {
				return trace.ReplicationStudyQuestionRoundVerificationSummary{}, errors.New("round save failed safely")
			})
		}},
		{name: "round verify usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify(nil, out, errOut, verifyRound)
		}},
		{name: "round verify invalid expected", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify([]string{"--expect-sha256=bad", "round.json"}, out, errOut, verifyRound)
		}},
		{name: "round verify error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify([]string{"round.json"}, out, errOut, func(string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error) {
				return trace.ReplicationStudyQuestionRoundVerificationSummary{}, errors.New("round verify failed safely")
			})
		}},
		{name: "round verify mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify([]string{"--expect-sha256=" + strings.Repeat("d", 64), "round.json"}, out, errOut, verifyRound)
		}},
		{name: "receipt usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceipt([]string{"round.json"}, out, errOut, askReceipt)
		}},
		{name: "receipt error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceipt([]string{"round.json", trace.StudyQuestionOutcome}, out, errOut, func(string, string) (trace.ReplicationStudyQuestionReceipt, error) {
				return trace.ReplicationStudyQuestionReceipt{}, errors.New("receipt ask failed safely")
			})
		}},
		{name: "receipt save usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptSave([]string{"round.json", trace.StudyQuestionOutcome}, out, errOut, saveReceipt)
		}},
		{name: "receipt save error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptSave([]string{"round.json", trace.StudyQuestionOutcome, "receipt.json"}, out, errOut, func(string, string, string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error) {
				return trace.ReplicationStudyQuestionReceiptVerificationSummary{}, errors.New("receipt save failed safely")
			})
		}},
		{name: "receipt verify usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify(nil, out, errOut, verifyReceipt)
		}},
		{name: "receipt verify invalid expected", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify([]string{"--expect-sha256=bad", "receipt.json"}, out, errOut, verifyReceipt)
		}},
		{name: "receipt verify error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify([]string{"receipt.json"}, out, errOut, func(string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error) {
				return trace.ReplicationStudyQuestionReceiptVerificationSummary{}, errors.New("receipt verify failed safely")
			})
		}},
		{name: "receipt verify mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify([]string{"--expect-sha256=" + strings.Repeat("e", 64), "receipt.json"}, out, errOut, verifyReceipt)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}
	for _, test := range []struct {
		name string
		call func(io.Writer, *bytes.Buffer) int
	}{
		{name: "round save human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllSave([]string{"study.json", "round.json"}, out, errOut, saveRound)
		}},
		{name: "round save JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllSave([]string{"--json", "study.json", "round.json"}, out, errOut, saveRound)
		}},
		{name: "round verify human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify([]string{"round.json"}, out, errOut, verifyRound)
		}},
		{name: "round verify JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskAllVerify([]string{"--json", "round.json"}, out, errOut, verifyRound)
		}},
		{name: "receipt ask human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceipt([]string{"round.json", trace.StudyQuestionOutcome}, out, errOut, askReceipt)
		}},
		{name: "receipt ask JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceipt([]string{"--json", "round.json", trace.StudyQuestionOutcome}, out, errOut, askReceipt)
		}},
		{name: "receipt save human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptSave([]string{"round.json", trace.StudyQuestionOutcome, "receipt.json"}, out, errOut, saveReceipt)
		}},
		{name: "receipt save JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptSave([]string{"--json", "round.json", trace.StudyQuestionOutcome, "receipt.json"}, out, errOut, saveReceipt)
		}},
		{name: "receipt verify human write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify([]string{"receipt.json"}, out, errOut, verifyReceipt)
		}},
		{name: "receipt verify JSON write", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runTraceStudyAskReceiptVerify([]string{"--json", "receipt.json"}, out, errOut, verifyReceipt)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var errOut bytes.Buffer
			if exitCode := test.call(failingWriter{}, &errOut); exitCode != 1 || !strings.Contains(errOut.String(), "write output") {
				t.Fatalf("call = %d, stderr=%q", exitCode, errOut.String())
			}
		})
	}
}

func TestWriteTraceStudyRoundAndReceipt(t *testing.T) {
	roundSummary := trace.ReplicationStudyQuestionRoundVerificationSummary{}
	receiptSummary := trace.ReplicationStudyQuestionReceiptVerificationSummary{}
	if err := writeTraceStudyRoundSummary(failingWriter{}, "round", roundSummary); err == nil {
		t.Fatal("writeTraceStudyRoundSummary() accepted a failing writer")
	}
	if err := writeTraceStudyReceiptSummary(failingWriter{}, "receipt", receiptSummary); err == nil {
		t.Fatal("writeTraceStudyReceiptSummary() accepted a failing writer")
	}
	if err := writeTraceStudyReceipt(failingWriter{}, "receipt", trace.ReplicationStudyQuestionReceipt{}, ""); err == nil {
		t.Fatal("writeTraceStudyReceipt() accepted a failing writer")
	}
}
