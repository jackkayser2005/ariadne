package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

func acceptanceSummaryForTest() bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary {
	return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{
		SchemaVersion:           1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
		QuestionRoundSHA256:     strings.Repeat("b", 64),
		QuestionID:              "answer-state-transitions",
		ReceiptSHA256:           strings.Repeat("c", 64),
		AcceptanceSHA256:        strings.Repeat("d", 64),
	}
}

func TestRunArchiveTransitionsAcceptanceDispatch(t *testing.T) {
	for _, args := range [][]string{
		{"experiment", "ask-archive", "transitions", "acceptance"},
		{"experiment", "ask-archive", "transitions", "acceptance", "unknown", "round.json", "receipt.json"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := run(args, &stdout, &stderr); exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("acceptance dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAcceptanceSave(t *testing.T) {
	summary := acceptanceSummaryForTest()
	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAcceptanceSave([]string{"round.json", "receipt.json", "acceptance.json"}, &stdout, &stderr, func(round, receipt, record string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			if round != "round.json" || receipt != "receipt.json" || record != "acceptance.json" {
				t.Fatalf("acceptance save args = %q, %q, %q", round, receipt, record)
			}
			return summary, nil
		})
		for _, want := range []string{"archive question acceptance record saved", "transition_history_sha256: " + strings.Repeat("a", 64), "question_round_sha256: " + strings.Repeat("b", 64), "question_id: answer-state-transitions", "receipt_sha256: " + strings.Repeat("c", 64), "acceptance_sha256: " + strings.Repeat("d", 64)} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("human output missing %q: %q", want, stdout.String())
			}
		}
		if exitCode != 0 || stderr.Len() != 0 {
			t.Fatalf("save = %d, stderr=%q", exitCode, stderr.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAcceptanceSave([]string{"--json", "round.json", "receipt.json", "acceptance.json"}, &stdout, &stderr, func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"acceptance_sha256":"`+strings.Repeat("d", 64)+`"`) {
			t.Fatalf("json save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAcceptanceSaveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"round.json", "receipt.json"}, {"round.json", "receipt.json", "acceptance.json", "extra"}, {"--json=invalid", "round.json", "receipt.json", "acceptance.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAcceptanceSave(args, &stdout, &stderr, func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
				t.Fatal("acceptance saver called for invalid usage")
				return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("save invalid = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	exitCode := runAskArchiveTransitionsAcceptanceSave([]string{"round.json", "receipt.json", "acceptance.json"}, &stdout, &stderr, func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
		return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("identity mismatch")
	})
	if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "identity mismatch") {
		t.Fatalf("save error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runAskArchiveTransitionsAcceptanceSave([]string{"round.json", "receipt.json", "acceptance.json"}, failingWriter{}, &stderr, func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
		return acceptanceSummaryForTest(), nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("save write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunAskArchiveTransitionsAcceptanceVerify(t *testing.T) {
	summary := acceptanceSummaryForTest()
	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAcceptanceVerify([]string{"--expect-sha256", strings.Repeat("d", 64), "acceptance.json"}, &stdout, &stderr, func(path string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			if path != "acceptance.json" {
				t.Fatalf("acceptance verify path = %q", path)
			}
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "does not prove that a UI driver performed the selection") {
			t.Fatalf("verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAcceptanceVerify([]string{"--json", "acceptance.json"}, &stdout, &stderr, func(string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"question_id":"answer-state-transitions"`) {
			t.Fatalf("json verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAcceptanceVerifyFailures(t *testing.T) {
	verify := func(string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
		return acceptanceSummaryForTest(), nil
	}
	for _, args := range [][]string{nil, {"acceptance.json", "extra"}, {"--json=invalid", "acceptance.json"}, {"--expect-sha256", "bad", "acceptance.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAcceptanceVerify(args, &stdout, &stderr, func(string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
				t.Fatal("acceptance verifier called for invalid usage")
				return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, nil
			})
			if exitCode != 2 || stdout.Len() != 0 || (stderr.String() != usage && !strings.Contains(stderr.String(), "expect-sha256")) {
				t.Fatalf("verify invalid = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
	var stdout, stderr bytes.Buffer
	exitCode := runAskArchiveTransitionsAcceptanceVerify([]string{"acceptance.json"}, &stdout, &stderr, func(string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
		return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("record invalid")
	})
	if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "record invalid") {
		t.Fatalf("verify error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runAskArchiveTransitionsAcceptanceVerify([]string{"--expect-sha256", strings.Repeat("e", 64), "acceptance.json"}, &stdout, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "acceptance SHA-256 mismatch") {
		t.Fatalf("verify mismatch = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, args := range [][]string{{"acceptance.json"}, {"--json", "acceptance.json"}} {
		var writeStderr bytes.Buffer
		var output bytes.Buffer
		if exitCode := runAskArchiveTransitionsAcceptanceVerify(args, failingWriter{}, &writeStderr, verify); exitCode != 1 || !strings.Contains(writeStderr.String(), "write output") {
			t.Fatalf("verify write error = %d, output=%q", exitCode, output.String())
		}
	}
}
