package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseAskAllCompare(t *testing.T) {
	comparison := trace.CaseQuestionRoundComparison{
		SchemaVersion:      1,
		ComparisonID:       "case-question-round-answer-change",
		ComparisonQuestion: "Did the bounded case answers change between these retained question rounds?",
		OrderBasis:         "caller",
		Result:             "changed",
		FirstRoundSHA256:   strings.Repeat("a", 64),
		SecondRoundSHA256:  strings.Repeat("b", 64),
		FirstCaseSHA256:    strings.Repeat("c", 64),
		SecondCaseSHA256:   strings.Repeat("d", 64),
		Compared:           3,
		Changed:            1,
		ChangedQuestions: []trace.CaseQuestionRoundChange{{
			QuestionID:          trace.CaseQuestionOutcomes,
			FirstResult:         "available",
			SecondResult:        "available",
			FirstEvidenceState:  evidence.Observed,
			SecondEvidenceState: evidence.Unknown,
			ChangeKinds:         []string{"outcomes", "evidence-state"},
		}},
	}

	var stdout, stderr strings.Builder
	exitCode := runTraceCaseAskAllCompare([]string{"first.json", "second.json"}, &stdout, &stderr, func(first, second string) (trace.CaseQuestionRoundComparison, error) {
		if first != "first.json" || second != "second.json" {
			t.Fatalf("compare args = %q, %q", first, second)
		}
		return comparison, nil
	})
	if exitCode != 0 || !strings.Contains(stdout.String(), "trace case question round comparison complete") || !strings.Contains(stdout.String(), "first_case_sha256") || !strings.Contains(stdout.String(), "change_kinds: outcomes,evidence-state") || stderr.Len() != 0 {
		t.Fatalf("human comparison = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	exitCode = runTraceCaseAskAllCompare([]string{"--json", "first.json", "second.json"}, &stdout, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
		return comparison, nil
	})
	if exitCode != 0 || !strings.Contains(stdout.String(), `"first_case_sha256"`) || !strings.Contains(stdout.String(), `"change_kinds":["outcomes","evidence-state"]`) || stderr.Len() != 0 {
		t.Fatalf("JSON comparison = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceCaseAskAllCompareFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"first.json"}, {"first.json", "second.json", "extra"}, {"--json=invalid", "first.json", "second.json"}} {
		var stdout, stderr strings.Builder
		exitCode := runTraceCaseAskAllCompare(args, &stdout, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
			t.Fatal("comparer called for invalid usage")
			return trace.CaseQuestionRoundComparison{}, nil
		})
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	}

	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseAskAllCompare([]string{"first.json", "second.json"}, &stdout, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
		return trace.CaseQuestionRoundComparison{}, errors.New("round comparison failed safely")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "round comparison failed safely") {
		t.Fatalf("comparison error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runTraceCaseAskAllCompare([]string{"first.json", "second.json"}, failingWriter{}, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
		return trace.CaseQuestionRoundComparison{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human writer error = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceCaseAskAllCompare([]string{"--json", "first.json", "second.json"}, failingWriter{}, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
		return trace.CaseQuestionRoundComparison{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON writer error = %d, stderr=%q", exitCode, stderr.String())
	}
	comparison := trace.CaseQuestionRoundComparison{
		ChangedQuestions: []trace.CaseQuestionRoundChange{{QuestionID: trace.CaseQuestionSources, ChangeKinds: []string{"sources"}}},
	}
	stderr.Reset()
	if exitCode := runTraceCaseAskAllCompare([]string{"first.json", "second.json"}, &failAfterWriter{failAt: 2}, &stderr, func(string, string) (trace.CaseQuestionRoundComparison, error) {
		return comparison, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("changed question writer error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestTraceCaseCompareDispatch(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "ask", "all", "compare", "missing-first.json", "missing-second.json"}, &stdout, &stderr); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "first question round") {
		t.Fatalf("trace case compare dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
