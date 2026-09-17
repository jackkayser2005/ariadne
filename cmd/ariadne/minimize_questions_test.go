package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestRunMinimizationQuestionCommands(t *testing.T) {
	answer := minimize.MinimizationQuestionAnswer{
		SchemaVersion:       1,
		QuestionID:          minimize.MinimizationQuestionSelection,
		Question:            "What minimum tested sufficient disclosure, if any, did this ladder establish?",
		Result:              string(minimize.SelectionSelected),
		EvidenceState:       evidence.Observed,
		Reason:              "the tested ladder established a sufficient candidate at its least-disclosing selected position",
		MinimizationSHA256:  strings.Repeat("a", 64),
		SelectionState:      minimize.SelectionSelected,
		SelectedCandidate:   "city",
		CandidateCount:      2,
		SupportedCandidates: 2,
		UnknownCandidates:   0,
	}
	answers := []minimize.MinimizationQuestionAnswer{answer, {
		SchemaVersion:       1,
		QuestionID:          minimize.MinimizationQuestionSupport,
		Question:            "Did every candidate have complete replicated support and observed evidence?",
		Result:              "supported",
		EvidenceState:       evidence.Observed,
		Reason:              "every candidate has complete replicated support and observed evidence",
		MinimizationSHA256:  strings.Repeat("a", 64),
		SelectionState:      minimize.SelectionSelected,
		SelectedCandidate:   "city",
		CandidateCount:      2,
		SupportedCandidates: 2,
	}}
	roundSummary := minimize.MinimizationQuestionRoundVerificationSummary{
		SchemaVersion:      1,
		MinimizationSHA256: strings.Repeat("a", 64),
		Questions:          2,
		Candidates:         2,
		RoundSHA256:        strings.Repeat("b", 64),
	}
	receipt := minimize.MinimizationQuestionReceipt{
		MinimizationQuestionAnswer: answer,
		RoundSHA256:                roundSummary.RoundSHA256,
	}
	receiptSummary := minimize.MinimizationQuestionReceiptVerificationSummary{
		SchemaVersion:       1,
		QuestionID:          answer.QuestionID,
		Question:            answer.Question,
		Result:              answer.Result,
		EvidenceState:       answer.EvidenceState,
		MinimizationSHA256:  answer.MinimizationSHA256,
		RoundSHA256:         roundSummary.RoundSHA256,
		ReceiptSHA256:       strings.Repeat("c", 64),
		SelectionState:      answer.SelectionState,
		SelectedCandidate:   answer.SelectedCandidate,
		CandidateCount:      answer.CandidateCount,
		SupportedCandidates: answer.SupportedCandidates,
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runMinimizationQuestions(nil, &stdout, &stderr, func() []minimize.MinimizationQuestion {
		return minimize.MinimizationQuestions()
	}); exitCode != 0 || !strings.Contains(stdout.String(), minimize.MinimizationQuestionSelection) {
		t.Fatalf("questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationQuestions([]string{"--json"}, &stdout, &stderr, func() []minimize.MinimizationQuestion {
		return minimize.MinimizationQuestions()
	}); exitCode != 0 {
		t.Fatalf("JSON questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var catalog []minimize.MinimizationQuestion
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil || len(catalog) != 2 {
		t.Fatalf("catalog = %#v, err=%v", catalog, err)
	}

	ask := func(path, questionID string) (minimize.MinimizationQuestionAnswer, error) {
		if path != "run" || questionID != minimize.MinimizationQuestionSelection {
			t.Fatalf("ask args = %q, %q", path, questionID)
		}
		return answer, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAsk([]string{"run", minimize.MinimizationQuestionSelection}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "question answered") {
		t.Fatalf("ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationAsk([]string{"--json", "run", minimize.MinimizationQuestionSelection}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decodedAnswer minimize.MinimizationQuestionAnswer
	if err := json.Unmarshal(stdout.Bytes(), &decodedAnswer); err != nil || decodedAnswer != answer {
		t.Fatalf("answer = %#v, err=%v", decodedAnswer, err)
	}

	askAll := func(path string) ([]minimize.MinimizationQuestionAnswer, error) {
		if path != "run" {
			t.Fatalf("ask all path = %q", path)
		}
		return answers, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskAll([]string{"run"}, &stdout, &stderr, askAll); exitCode != 0 || !strings.Contains(stdout.String(), "questions answered") {
		t.Fatalf("ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationAskAll([]string{"--json", "run"}, &stdout, &stderr, askAll); exitCode != 0 {
		t.Fatalf("JSON ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decodedAnswers []minimize.MinimizationQuestionAnswer
	if err := json.Unmarshal(stdout.Bytes(), &decodedAnswers); err != nil || len(decodedAnswers) != 2 {
		t.Fatalf("answers = %#v, err=%v", decodedAnswers, err)
	}

	saveRound := func(runPath, roundPath string) (minimize.MinimizationQuestionRoundVerificationSummary, error) {
		if runPath != "run" || roundPath != "round.json" {
			t.Fatalf("save round args = %q, %q", runPath, roundPath)
		}
		return roundSummary, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskAllSave([]string{"run", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round saved") {
		t.Fatalf("save round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationAskAllSave([]string{"--json", "run", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 {
		t.Fatalf("JSON save round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decodedRoundSummary minimize.MinimizationQuestionRoundVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &decodedRoundSummary); err != nil || decodedRoundSummary != roundSummary {
		t.Fatalf("round summary = %#v, err=%v", decodedRoundSummary, err)
	}
	verifyRound := func(path string) (minimize.MinimizationQuestionRoundVerificationSummary, error) {
		if path != "round.json" {
			t.Fatalf("verify round path = %q", path)
		}
		return roundSummary, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskAllVerify([]string{"--expect-sha256", roundSummary.RoundSHA256, "round.json"}, &stdout, &stderr, verifyRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round verified") {
		t.Fatalf("verify round = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	askReceipt := func(path, questionID string) (minimize.MinimizationQuestionReceipt, error) {
		if path != "round.json" || questionID != answer.QuestionID {
			t.Fatalf("ask receipt args = %q, %q", path, questionID)
		}
		return receipt, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskReceipt([]string{"round.json", answer.QuestionID}, &stdout, &stderr, askReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "question receipt") {
		t.Fatalf("ask receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationAskReceipt([]string{"--json", "round.json", answer.QuestionID}, &stdout, &stderr, askReceipt); exitCode != 0 {
		t.Fatalf("JSON ask receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decodedReceipt minimize.MinimizationQuestionReceipt
	if err := json.Unmarshal(stdout.Bytes(), &decodedReceipt); err != nil || decodedReceipt.QuestionID != receipt.QuestionID {
		t.Fatalf("receipt = %#v, err=%v", decodedReceipt, err)
	}
	saveReceipt := func(roundPath, questionID, receiptPath string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
		if roundPath != "round.json" || questionID != answer.QuestionID || receiptPath != "receipt.json" {
			t.Fatalf("save receipt args = %q, %q, %q", roundPath, questionID, receiptPath)
		}
		return receiptSummary, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskReceiptSave([]string{"round.json", answer.QuestionID, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt saved") {
		t.Fatalf("save receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runMinimizationAskReceiptSave([]string{"--json", "round.json", answer.QuestionID, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 {
		t.Fatalf("JSON save receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decodedReceiptSummary minimize.MinimizationQuestionReceiptVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &decodedReceiptSummary); err != nil || decodedReceiptSummary != receiptSummary {
		t.Fatalf("receipt summary = %#v, err=%v", decodedReceiptSummary, err)
	}
	verifyReceipt := func(path string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
		if path != "receipt.json" {
			t.Fatalf("verify receipt path = %q", path)
		}
		return receiptSummary, nil
	}
	stdout.Reset()
	if exitCode := runMinimizationAskReceiptVerify([]string{"--expect-sha256", receiptSummary.ReceiptSHA256, "receipt.json"}, &stdout, &stderr, verifyReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt verified") {
		t.Fatalf("verify receipt = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "questions usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationQuestions([]string{"extra"}, out, errOut, minimize.MinimizationQuestions)
		}},
		{name: "ask usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAsk([]string{"run"}, out, errOut, ask)
		}},
		{name: "ask error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAsk([]string{"run", answer.QuestionID}, out, errOut, func(string, string) (minimize.MinimizationQuestionAnswer, error) {
				return minimize.MinimizationQuestionAnswer{}, errors.New("ask failed safely")
			})
		}},
		{name: "ask all usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAll(nil, out, errOut, askAll)
		}},
		{name: "ask all error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAll([]string{"run"}, out, errOut, func(string) ([]minimize.MinimizationQuestionAnswer, error) {
				return nil, errors.New("ask all failed safely")
			})
		}},
		{name: "save round usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllSave([]string{"run"}, out, errOut, saveRound)
		}},
		{name: "save round error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllSave([]string{"run", "round.json"}, out, errOut, func(string, string) (minimize.MinimizationQuestionRoundVerificationSummary, error) {
				return minimize.MinimizationQuestionRoundVerificationSummary{}, errors.New("round save failed safely")
			})
		}},
		{name: "verify round usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify(nil, out, errOut, verifyRound)
		}},
		{name: "verify round invalid expected", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify([]string{"--expect-sha256=bad", "round.json"}, out, errOut, verifyRound)
		}},
		{name: "verify round error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify([]string{"round.json"}, out, errOut, func(string) (minimize.MinimizationQuestionRoundVerificationSummary, error) {
				return minimize.MinimizationQuestionRoundVerificationSummary{}, errors.New("round verify failed safely")
			})
		}},
		{name: "verify round mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify([]string{"--expect-sha256=" + strings.Repeat("d", 64), "round.json"}, out, errOut, verifyRound)
		}},
		{name: "ask receipt usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceipt([]string{"round.json"}, out, errOut, askReceipt)
		}},
		{name: "ask receipt error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceipt([]string{"round.json", answer.QuestionID}, out, errOut, func(string, string) (minimize.MinimizationQuestionReceipt, error) {
				return minimize.MinimizationQuestionReceipt{}, errors.New("receipt ask failed safely")
			})
		}},
		{name: "save receipt usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptSave([]string{"round.json", answer.QuestionID}, out, errOut, saveReceipt)
		}},
		{name: "save receipt error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptSave([]string{"round.json", answer.QuestionID, "receipt.json"}, out, errOut, func(string, string, string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
				return minimize.MinimizationQuestionReceiptVerificationSummary{}, errors.New("receipt save failed safely")
			})
		}},
		{name: "verify receipt usage", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify(nil, out, errOut, verifyReceipt)
		}},
		{name: "verify receipt invalid expected", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify([]string{"--expect-sha256=bad", "receipt.json"}, out, errOut, verifyReceipt)
		}},
		{name: "verify receipt error", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify([]string{"receipt.json"}, out, errOut, func(string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
				return minimize.MinimizationQuestionReceiptVerificationSummary{}, errors.New("receipt verify failed safely")
			})
		}},
		{name: "verify receipt mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify([]string{"--expect-sha256=" + strings.Repeat("e", 64), "receipt.json"}, out, errOut, verifyReceipt)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}
}

func TestRunMinimizationQuestionWritersRejectWriteFailures(t *testing.T) {
	answer := minimize.MinimizationQuestionAnswer{}
	roundSummary := minimize.MinimizationQuestionRoundVerificationSummary{}
	receipt := minimize.MinimizationQuestionReceipt{}
	receiptSummary := minimize.MinimizationQuestionReceiptVerificationSummary{}
	if err := writeMinimizationQuestionAnswer(failingWriter{}, answer); err == nil {
		t.Fatal("writeMinimizationQuestionAnswer() accepted a failing writer")
	}
	if err := writeMinimizationQuestionRoundSummary(failingWriter{}, "round", roundSummary); err == nil {
		t.Fatal("writeMinimizationQuestionRoundSummary() accepted a failing writer")
	}
	if err := writeMinimizationQuestionReceipt(failingWriter{}, "receipt", receipt, ""); err == nil {
		t.Fatal("writeMinimizationQuestionReceipt() accepted a failing writer")
	}
	if err := writeMinimizationQuestionReceiptSummary(failingWriter{}, "receipt", receiptSummary); err == nil {
		t.Fatal("writeMinimizationQuestionReceiptSummary() accepted a failing writer")
	}
}

func TestRunDispatchesMinimizationQuestions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"experiment", "minimize", "questions", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), minimize.MinimizationQuestionSelection) || stderr.Len() != 0 {
		t.Fatalf("run() output = %q / %q", stdout.String(), stderr.String())
	}
}

func TestRunDispatchesBrowserMinimizationQuestions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"browser", "fixture", "minimize", "questions", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var catalog []minimize.LadderQuestion
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog) != len(minimize.LadderQuestions()) || stderr.Len() != 0 {
		t.Fatalf("run() output = %q / %q", stdout.String(), stderr.String())
	}
}
