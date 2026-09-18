package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseDisclosureCommands(t *testing.T) {
	answer := trace.CaseDisclosureQuestionAnswer{
		SchemaVersion: 1, QuestionID: trace.CaseDisclosureQuestionOverlap,
		Question: "Which reviewed categories appeared across multiple source or adapter boundaries?",
		Result:   "overlap-observed", EvidenceState: evidence.Observed,
		Reason:     "one or more reviewed categories appeared across multiple source or adapter boundaries",
		CaseSHA256: strings.Repeat("a", 64), Traces: 4, CoverageState: evidence.Observed,
		Categories: []trace.CaseDisclosureCategorySummary{{
			Category: "region", Boundaries: []trace.CaseDisclosureBoundary{{Source: "android", Adapter: "android-experiment-001"}, {Source: "browser", Adapter: "browser-redacted-audit"}},
		}},
		OverlappingCategories: []string{"region"},
	}
	answers := []trace.CaseDisclosureQuestionAnswer{answer}
	roundSummary := trace.CaseDisclosureQuestionRoundVerificationSummary{SchemaVersion: 1, CaseSHA256: strings.Repeat("a", 64), Questions: 2, RoundSHA256: strings.Repeat("b", 64)}
	receipt := trace.CaseDisclosureQuestionReceipt{CaseDisclosureQuestionAnswer: answer, RoundSHA256: strings.Repeat("b", 64)}
	receiptSummary := trace.CaseDisclosureQuestionReceiptVerificationSummary{
		SchemaVersion: 1, QuestionID: answer.QuestionID, Question: answer.Question, Result: answer.Result,
		EvidenceState: answer.EvidenceState, CaseSHA256: answer.CaseSHA256, RoundSHA256: receipt.RoundSHA256, ReceiptSHA256: strings.Repeat("c", 64),
	}
	catalog := func() []trace.CaseDisclosureQuestion { return trace.CaseDisclosureQuestions() }
	ask := func(string, string) (trace.CaseDisclosureQuestionAnswer, error) { return answer, nil }
	askAll := func(string) ([]trace.CaseDisclosureQuestionAnswer, error) { return answers, nil }
	saveRound := func(string, string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
		return roundSummary, nil
	}
	verifyRound := func(string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) { return roundSummary, nil }
	askReceipt := func(string, string) (trace.CaseDisclosureQuestionReceipt, error) { return receipt, nil }
	saveReceipt := func(string, string, string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
		return receiptSummary, nil
	}
	verifyReceipt := func(string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
		return receiptSummary, nil
	}

	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMapQuestions(nil, &stdout, &stderr, catalog); exitCode != 0 || !strings.Contains(stdout.String(), "trace case disclosure question catalog") || stderr.Len() != 0 {
		t.Fatalf("questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapQuestions([]string{"--json"}, &stdout, &stderr, catalog); exitCode != 0 || !strings.Contains(stdout.String(), trace.CaseDisclosureQuestionOverlap) {
		t.Fatalf("questions JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAsk([]string{"case.json", answer.QuestionID}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "overlapping_categories: region") {
		t.Fatalf("ask = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAsk([]string{"--json", "case.json", answer.QuestionID}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), `"question_id"`) {
		t.Fatalf("ask JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAll([]string{"case.json"}, &stdout, &stderr, askAll); exitCode != 0 || !strings.Contains(stdout.String(), "trace case disclosure questions answered") {
		t.Fatalf("ask all = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAll([]string{"--json", "case.json"}, &stdout, &stderr, askAll); exitCode != 0 || !strings.Contains(stdout.String(), `"question_id"`) {
		t.Fatalf("ask all JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAllSave([]string{"case.json", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round saved") {
		t.Fatalf("round save = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAllSave([]string{"--json", "case.json", "round.json"}, &stdout, &stderr, saveRound); exitCode != 0 || !strings.Contains(stdout.String(), `"round_sha256"`) {
		t.Fatalf("round save JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAllVerify([]string{"--expect-sha256", roundSummary.RoundSHA256, "round.json"}, &stdout, &stderr, verifyRound); exitCode != 0 || !strings.Contains(stdout.String(), "question round verified") {
		t.Fatalf("round verify = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskAllVerify([]string{"--json", "round.json"}, &stdout, &stderr, verifyRound); exitCode != 0 || !strings.Contains(stdout.String(), `"questions"`) {
		t.Fatalf("round verify JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceipt([]string{"round.json", answer.QuestionID}, &stdout, &stderr, askReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "question receipt") {
		t.Fatalf("receipt = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceipt([]string{"--json", "round.json", answer.QuestionID}, &stdout, &stderr, askReceipt); exitCode != 0 || !strings.Contains(stdout.String(), `"round_sha256"`) {
		t.Fatalf("receipt JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceiptSave([]string{"round.json", answer.QuestionID, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt saved") {
		t.Fatalf("receipt save = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceiptSave([]string{"--json", "round.json", answer.QuestionID, "receipt.json"}, &stdout, &stderr, saveReceipt); exitCode != 0 || !strings.Contains(stdout.String(), `"receipt_sha256"`) {
		t.Fatalf("receipt save JSON = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceiptVerify([]string{"--expect-sha256", receiptSummary.ReceiptSHA256, "receipt.json"}, &stdout, &stderr, verifyReceipt); exitCode != 0 || !strings.Contains(stdout.String(), "receipt verified") {
		t.Fatalf("receipt verify = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapAskReceiptVerify([]string{"--json", "receipt.json"}, &stdout, &stderr, verifyReceipt); exitCode != 0 || !strings.Contains(stdout.String(), `"receipt_sha256"`) {
		t.Fatalf("receipt verify JSON = %d, stdout=%q", exitCode, stdout.String())
	}
}

func TestRunTraceCaseDisclosureFailuresAndWriters(t *testing.T) {
	answer := trace.CaseDisclosureQuestionAnswer{QuestionID: trace.CaseDisclosureQuestionCoverage, Question: "coverage", Reason: "reason", OverlappingCategories: []string{"region"}, Categories: []trace.CaseDisclosureCategorySummary{{Category: "region", Boundaries: []trace.CaseDisclosureBoundary{{Source: "android", Adapter: "android-experiment-001"}}}}}
	roundSummary := trace.CaseDisclosureQuestionRoundVerificationSummary{RoundSHA256: strings.Repeat("b", 64)}
	receiptSummary := trace.CaseDisclosureQuestionReceiptVerificationSummary{ReceiptSHA256: strings.Repeat("c", 64)}
	receipt := trace.CaseDisclosureQuestionReceipt{CaseDisclosureQuestionAnswer: answer, RoundSHA256: strings.Repeat("b", 64)}
	tests := []struct {
		name string
		run  func(io.Writer, io.Writer) int
	}{
		{name: "questions usage", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapQuestions([]string{"extra"}, out, errOut, trace.CaseDisclosureQuestions)
		}},

		{name: "ask usage", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAsk([]string{"case.json"}, out, errOut, nil)
		}},
		{name: "ask error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAsk([]string{"case.json", "question"}, out, errOut, func(string, string) (trace.CaseDisclosureQuestionAnswer, error) {
				return trace.CaseDisclosureQuestionAnswer{}, errors.New("ask failed safely")
			})
		}},
		{name: "all usage", run: func(out, errOut io.Writer) int { return runTraceCaseMapAskAll(nil, out, errOut, nil) }},
		{name: "all error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskAll([]string{"case.json"}, out, errOut, func(string) ([]trace.CaseDisclosureQuestionAnswer, error) {
				return nil, errors.New("all failed safely")
			})
		}},
		{name: "round save usage", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskAllSave([]string{"case.json"}, out, errOut, nil)
		}},
		{name: "round save error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskAllSave([]string{"case.json", "round.json"}, out, errOut, func(string, string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
				return trace.CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("save failed safely")
			})
		}},
		{name: "round verify usage", run: func(out, errOut io.Writer) int { return runTraceCaseMapAskAllVerify(nil, out, errOut, nil) }},
		{name: "round verify bad digest", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskAllVerify([]string{"--expect-sha256", "bad", "round.json"}, out, errOut, nil)
		}},
		{name: "round verify error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskAllVerify([]string{"round.json"}, out, errOut, func(string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
				return trace.CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("verify failed safely")
			})
		}},
		{name: "receipt usage", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceipt([]string{"round.json"}, out, errOut, nil)
		}},
		{name: "receipt error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceipt([]string{"round.json", "question"}, out, errOut, func(string, string) (trace.CaseDisclosureQuestionReceipt, error) {
				return trace.CaseDisclosureQuestionReceipt{}, errors.New("receipt failed safely")
			})
		}},
		{name: "receipt save usage", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceiptSave([]string{"round.json", "question"}, out, errOut, nil)
		}},
		{name: "receipt save error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceiptSave([]string{"round.json", "question", "receipt.json"}, out, errOut, func(string, string, string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return trace.CaseDisclosureQuestionReceiptVerificationSummary{}, errors.New("save receipt failed safely")
			})
		}},
		{name: "receipt verify usage", run: func(out, errOut io.Writer) int { return runTraceCaseMapAskReceiptVerify(nil, out, errOut, nil) }},
		{name: "receipt verify bad digest", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceiptVerify([]string{"--expect-sha256", "bad", "receipt.json"}, out, errOut, nil)
		}},
		{name: "receipt verify error", run: func(out, errOut io.Writer) int {
			return runTraceCaseMapAskReceiptVerify([]string{"receipt.json"}, out, errOut, func(string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return trace.CaseDisclosureQuestionReceiptVerificationSummary{}, errors.New("verify receipt failed safely")
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if exitCode := test.run(&stdout, &stderr); exitCode != 1 && !strings.Contains(test.name, "usage") {
				t.Fatalf("exit code = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if strings.Contains(test.name, "usage") && stdout.Len() != 0 {
				t.Fatalf("usage wrote stdout = %q", stdout.String())
			}
		})
	}
	for _, test := range []struct {
		name string
		run  func(io.Writer) int
	}{
		{"questions", func(out io.Writer) int {
			return runTraceCaseMapQuestions(nil, out, &strings.Builder{}, trace.CaseDisclosureQuestions)
		}},
		{"questions JSON", func(out io.Writer) int {
			return runTraceCaseMapQuestions([]string{"--json"}, out, &strings.Builder{}, trace.CaseDisclosureQuestions)
		}},
		{"ask", func(out io.Writer) int {
			return runTraceCaseMapAsk([]string{"case.json", "question"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionAnswer, error) { return answer, nil })
		}},
		{"ask JSON", func(out io.Writer) int {
			return runTraceCaseMapAsk([]string{"--json", "case.json", "question"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionAnswer, error) { return answer, nil })
		}},
		{"all", func(out io.Writer) int {
			return runTraceCaseMapAskAll([]string{"case.json"}, out, &strings.Builder{}, func(string) ([]trace.CaseDisclosureQuestionAnswer, error) {
				return []trace.CaseDisclosureQuestionAnswer{answer}, nil
			})
		}},
		{"all JSON", func(out io.Writer) int {
			return runTraceCaseMapAskAll([]string{"--json", "case.json"}, out, &strings.Builder{}, func(string) ([]trace.CaseDisclosureQuestionAnswer, error) {
				return []trace.CaseDisclosureQuestionAnswer{answer}, nil
			})
		}},
		{"round save", func(out io.Writer) int {
			return runTraceCaseMapAskAllSave([]string{"case.json", "round.json"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
				return roundSummary, nil
			})
		}},
		{"round save JSON", func(out io.Writer) int {
			return runTraceCaseMapAskAllSave([]string{"--json", "case.json", "round.json"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
				return roundSummary, nil
			})
		}},
		{"round verify", func(out io.Writer) int {
			return runTraceCaseMapAskAllVerify([]string{"round.json"}, out, &strings.Builder{}, func(string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) { return roundSummary, nil })
		}},
		{"round verify JSON", func(out io.Writer) int {
			return runTraceCaseMapAskAllVerify([]string{"--json", "round.json"}, out, &strings.Builder{}, func(string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error) { return roundSummary, nil })
		}},
		{"receipt", func(out io.Writer) int {
			return runTraceCaseMapAskReceipt([]string{"round.json", "question"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionReceipt, error) { return receipt, nil })
		}},
		{"receipt JSON", func(out io.Writer) int {
			return runTraceCaseMapAskReceipt([]string{"--json", "round.json", "question"}, out, &strings.Builder{}, func(string, string) (trace.CaseDisclosureQuestionReceipt, error) { return receipt, nil })
		}},
		{"receipt save", func(out io.Writer) int {
			return runTraceCaseMapAskReceiptSave([]string{"round.json", "question", "receipt.json"}, out, &strings.Builder{}, func(string, string, string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return receiptSummary, nil
			})
		}},
		{"receipt save JSON", func(out io.Writer) int {
			return runTraceCaseMapAskReceiptSave([]string{"--json", "round.json", "question", "receipt.json"}, out, &strings.Builder{}, func(string, string, string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return receiptSummary, nil
			})
		}},
		{"receipt verify", func(out io.Writer) int {
			return runTraceCaseMapAskReceiptVerify([]string{"receipt.json"}, out, &strings.Builder{}, func(string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return receiptSummary, nil
			})
		}},
		{"receipt verify JSON", func(out io.Writer) int {
			return runTraceCaseMapAskReceiptVerify([]string{"--json", "receipt.json"}, out, &strings.Builder{}, func(string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
				return receiptSummary, nil
			})
		}},
	} {
		t.Run(test.name+" writer", func(t *testing.T) {
			if exitCode := test.run(failingWriter{}); exitCode != 1 {
				t.Fatalf("writer exit code = %d", exitCode)
			}
		})
	}
	if err := writeTraceCaseDisclosureAnswer(failingWriter{}, answer); err == nil {
		t.Fatal("writeTraceCaseDisclosureAnswer() accepted a failing writer")
	}
	if err := writeTraceCaseDisclosureAnswer(&caseFailAfterWriter{failAfter: 1}, answer); err == nil {
		t.Fatal("writeTraceCaseDisclosureAnswer() accepted a failing reason writer")
	}
	if err := writeTraceCaseDisclosureAnswer(&caseFailAfterWriter{failAfter: 2}, answer); err == nil {
		t.Fatal("writeTraceCaseDisclosureAnswer() accepted a failing overlap writer")
	}
	if err := writeTraceCaseDisclosureAnswer(&caseFailAfterWriter{failAfter: 3}, answer); err == nil {
		t.Fatal("writeTraceCaseDisclosureAnswer() accepted a failing category writer")
	}
	if err := writeTraceCaseDisclosureRoundSummary(failingWriter{}, "round", roundSummary); err == nil {
		t.Fatal("writeTraceCaseDisclosureRoundSummary() accepted a failing writer")
	}
	if err := writeTraceCaseDisclosureReceiptSummary(failingWriter{}, "receipt", receiptSummary); err == nil {
		t.Fatal("writeTraceCaseDisclosureReceiptSummary() accepted a failing writer")
	}
	if err := writeTraceCaseDisclosureReceipt(failingWriter{}, "receipt", receipt, strings.Repeat("d", 64)); err == nil {
		t.Fatal("writeTraceCaseDisclosureReceipt() accepted a failing writer")
	}
	if err := writeTraceCaseDisclosureReceipt(&caseFailAfterWriter{failAfter: 1}, "receipt", receipt, strings.Repeat("d", 64)); err == nil {
		t.Fatal("writeTraceCaseDisclosureReceipt() accepted a failing digest writer")
	}
	if err := writeTraceCaseDisclosureReceipt(&caseFailAfterWriter{failAfter: 2}, "receipt", receipt, strings.Repeat("d", 64)); err == nil {
		t.Fatal("writeTraceCaseDisclosureReceipt() accepted a failing reason writer")
	}
	if err := writeTraceCaseDisclosureReceipt(&caseFailAfterWriter{failAfter: 3}, "receipt", receipt, strings.Repeat("d", 64)); err == nil {
		t.Fatal("writeTraceCaseDisclosureReceipt() accepted a failing overlap writer")
	}
}

func TestRunTraceCaseDisclosureDispatchAndHelpers(t *testing.T) {
	for _, args := range [][]string{
		{"trace", "case", "map", "questions", "extra"},
		{"trace", "case", "map", "ask"},
		{"trace", "case", "map", "ask", "all", "save"},
		{"trace", "case", "map", "ask", "all", "verify"},
		{"trace", "case", "map", "ask", "receipt"},
		{"trace", "case", "map", "ask", "receipt", "save"},
		{"trace", "case", "map", "ask", "receipt", "verify"},
	} {
		var stdout, stderr strings.Builder
		if exitCode := run(args, &stdout, &stderr); exitCode != 2 || stderr.Len() == 0 {
			t.Fatalf("run(%v) = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
	if _, err := askTraceCaseDisclosureQuestion("missing-case.json", "question"); err == nil {
		t.Fatal("askTraceCaseDisclosureQuestion() accepted missing case")
	}
	if _, err := askAllTraceCaseDisclosureQuestions("missing-case.json"); err == nil {
		t.Fatal("askAllTraceCaseDisclosureQuestions() accepted missing case")
	}
}
