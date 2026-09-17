package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestMinimizationQuestionCommandsHandleWriteFailures(t *testing.T) {
	answer := minimize.MinimizationQuestionAnswer{
		QuestionID:    minimize.MinimizationQuestionSelection,
		Question:      "question",
		Result:        "selected",
		EvidenceState: evidence.Observed,
	}
	answers := []minimize.MinimizationQuestionAnswer{answer}
	roundSummary := minimize.MinimizationQuestionRoundVerificationSummary{RoundSHA256: strings.Repeat("a", 64)}
	receipt := minimize.MinimizationQuestionReceipt{MinimizationQuestionAnswer: answer}
	receiptSummary := minimize.MinimizationQuestionReceiptVerificationSummary{ReceiptSHA256: strings.Repeat("b", 64)}
	list := func() []minimize.MinimizationQuestion { return minimize.MinimizationQuestions() }
	ask := func(string, string) (minimize.MinimizationQuestionAnswer, error) { return answer, nil }
	askAll := func(string) ([]minimize.MinimizationQuestionAnswer, error) { return answers, nil }
	saveRound := func(string, string) (minimize.MinimizationQuestionRoundVerificationSummary, error) {
		return roundSummary, nil
	}
	verifyRound := func(string) (minimize.MinimizationQuestionRoundVerificationSummary, error) { return roundSummary, nil }
	askReceipt := func(string, string) (minimize.MinimizationQuestionReceipt, error) { return receipt, nil }
	saveReceipt := func(string, string, string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
		return receiptSummary, nil
	}
	verifyReceipt := func(string) (minimize.MinimizationQuestionReceiptVerificationSummary, error) {
		return receiptSummary, nil
	}
	cases := []struct {
		name string
		call func(io.Writer, *bytes.Buffer) int
	}{
		{name: "questions human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationQuestions(nil, out, errOut, list)
		}},
		{name: "questions JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationQuestions([]string{"--json"}, out, errOut, list)
		}},
		{name: "ask human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAsk([]string{"run", "question"}, out, errOut, ask)
		}},
		{name: "ask JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAsk([]string{"--json", "run", "question"}, out, errOut, ask)
		}},
		{name: "ask all human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAll([]string{"run"}, out, errOut, askAll)
		}},
		{name: "ask all JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAll([]string{"--json", "run"}, out, errOut, askAll)
		}},
		{name: "save round human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAllSave([]string{"run", "round.json"}, out, errOut, saveRound)
		}},
		{name: "save round JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAllSave([]string{"--json", "run", "round.json"}, out, errOut, saveRound)
		}},
		{name: "verify round human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify([]string{"round.json"}, out, errOut, verifyRound)
		}},
		{name: "verify round JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskAllVerify([]string{"--json", "round.json"}, out, errOut, verifyRound)
		}},
		{name: "ask receipt human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceipt([]string{"round.json", "question"}, out, errOut, askReceipt)
		}},
		{name: "ask receipt JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceipt([]string{"--json", "round.json", "question"}, out, errOut, askReceipt)
		}},
		{name: "save receipt human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptSave([]string{"round.json", "question", "receipt.json"}, out, errOut, saveReceipt)
		}},
		{name: "save receipt JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptSave([]string{"--json", "round.json", "question", "receipt.json"}, out, errOut, saveReceipt)
		}},
		{name: "verify receipt human", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify([]string{"receipt.json"}, out, errOut, verifyReceipt)
		}},
		{name: "verify receipt JSON", call: func(out io.Writer, errOut *bytes.Buffer) int {
			return runMinimizationAskReceiptVerify([]string{"--json", "receipt.json"}, out, errOut, verifyReceipt)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if exitCode := test.call(failingWriter{}, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("call = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestWriteMinimizationQuestionReceiptReportsReceiptWriteFailure(t *testing.T) {
	writer := &failAfterFirstWrite{}
	if err := writeMinimizationQuestionReceipt(writer, "receipt", minimize.MinimizationQuestionReceipt{}, strings.Repeat("a", 64)); err == nil {
		t.Fatal("writeMinimizationQuestionReceipt() accepted a failing second write")
	}
}

type failAfterFirstWrite struct {
	writes int
}

func (w *failAfterFirstWrite) Write(data []byte) (int, error) {
	if w.writes > 0 {
		return 0, errors.New("second write failed")
	}
	w.writes++
	return len(data), nil
}
