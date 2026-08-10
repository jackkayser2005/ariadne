package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

func TestRunAskArchiveTransitionsAskAllCompare(t *testing.T) {
	comparison := bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{
		SchemaVersion:                 1,
		ComparisonID:                  "question-round-answer-change",
		ComparisonQuestion:            "Did the bounded answer results change between these retained question rounds?",
		OrderBasis:                    "caller",
		Result:                        "changed",
		FirstRoundSHA256:              strings.Repeat("a", 64),
		SecondRoundSHA256:             strings.Repeat("b", 64),
		FirstTransitionHistorySHA256:  strings.Repeat("c", 64),
		SecondTransitionHistorySHA256: strings.Repeat("d", 64),
		Compared:                      4,
		Changed:                       1,
		ChangedQuestions: []bundle.ArchiveQuestionTransitionHistoryQuestionRoundChange{{
			QuestionID:   "answer-state-transitions",
			FirstResult:  "same",
			SecondResult: "changed",
		}},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllCompare([]string{"first.json", "second.json"}, &stdout, &stderr, func(first, second string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			if first != "first.json" || second != "second.json" {
				t.Fatalf("compare args = %q, %q", first, second)
			}
			return comparison, nil
		})
		want := "archive reflection question round comparison complete\n" +
			"schema_version: 1\ncomparison_id: question-round-answer-change\n" +
			"comparison_question: Did the bounded answer results change between these retained question rounds?\n" +
			"order_basis: caller\nresult: changed\n" +
			"first_round_sha256: " + strings.Repeat("a", 64) + "\n" +
			"second_round_sha256: " + strings.Repeat("b", 64) + "\n" +
			"first_transition_history_sha256: " + strings.Repeat("c", 64) + "\n" +
			"second_transition_history_sha256: " + strings.Repeat("d", 64) + "\n" +
			"compared: 4\nchanged: 1\nchanged_questions:\n" +
			"- question_id: answer-state-transitions\n  first_result: same\n  second_result: changed\n" +
			"note: this compares fixed bounded question results in caller order; it does not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllCompare([]string{"--json", "first.json", "second.json"}, &stdout, &stderr, func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return comparison, nil
		})
		want := `{"schema_version":1,"comparison_id":"question-round-answer-change","comparison_question":"Did the bounded answer results change between these retained question rounds?","order_basis":"caller","result":"changed","first_round_sha256":"` + strings.Repeat("a", 64) + `","second_round_sha256":"` + strings.Repeat("b", 64) + `","first_transition_history_sha256":"` + strings.Repeat("c", 64) + `","second_transition_history_sha256":"` + strings.Repeat("d", 64) + `","compared":4,"changed":1,"changed_questions":[{"question_id":"answer-state-transitions","first_result":"same","second_result":"changed"}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskAllCompareFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"first.json"}, {"first.json", "second.json", "extra"}, {"--json=invalid", "first.json", "second.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllCompare(args, &stdout, &stderr, func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
				t.Fatal("question round comparer called for invalid usage")
				return bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskAllCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("compare error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllCompare([]string{"first.json", "second.json"}, &stdout, &stderr, func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, errors.New("round is invalid")
		})
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "round is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskAllCompare() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"first.json", "second.json"}, {"--json", "first.json", "second.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllCompare(args, failingWriter{}, &stderr, func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
				return bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, nil
			})
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskAllCompare() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	comparison := bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{
		ChangedQuestions: []bundle.ArchiveQuestionTransitionHistoryQuestionRoundChange{{QuestionID: "answer-state-transitions", FirstResult: "same", SecondResult: "changed"}},
	}
	var stderr bytes.Buffer
	exitCode := runAskArchiveTransitionsAskAllCompare([]string{"first.json", "second.json"}, &failAfterWriter{failAt: 2}, &stderr, func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
		return comparison, nil
	})
	if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("changed question write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}
