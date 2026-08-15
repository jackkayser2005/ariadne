package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceStudyAskAllCompare(t *testing.T) {
	want := trace.ReplicationStudyQuestionRoundComparison{
		SchemaVersion:      1,
		ComparisonID:       "replication-study-question-round-answer-change",
		ComparisonQuestion: "Did the bounded study answers change between these retained question rounds?",
		OrderBasis:         "caller",
		Result:             "changed",
		FirstRoundSHA256:   strings.Repeat("a", 64),
		SecondRoundSHA256:  strings.Repeat("b", 64),
		FirstStudySHA256:   strings.Repeat("c", 64),
		SecondStudySHA256:  strings.Repeat("d", 64),
		Compared:           3,
		Changed:            1,
		ChangedQuestions: []trace.ReplicationStudyQuestionRoundChange{{
			QuestionID:          "study-outcome",
			FirstResult:         "replicated-change",
			SecondResult:        "unknown",
			FirstEvidenceState:  evidence.Observed,
			SecondEvidenceState: evidence.Unknown,
			FirstOutcome:        trace.ReplicatedChange,
			SecondOutcome:       trace.ReplicationUnknown,
			ChangeKinds:         []string{"result", "outcome", "evidence-state"},
		}},
	}
	var stdout, stderr bytes.Buffer
	gotCode := runTraceStudyAskAllCompare([]string{"--json", "first-study", "first-round", "second-study", "second-round"}, &stdout, &stderr, func(firstStudy, firstRound, secondStudy, secondRound string) (trace.ReplicationStudyQuestionRoundComparison, error) {
		if firstStudy != "first-study" || firstRound != "first-round" || secondStudy != "second-study" || secondRound != "second-round" {
			t.Fatalf("comparison paths = %q %q %q %q", firstStudy, firstRound, secondStudy, secondRound)
		}
		return want, nil
	})
	if gotCode != 0 || stderr.Len() != 0 {
		t.Fatalf("runTraceStudyAskAllCompare() = %d, stderr=%q", gotCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"result":"changed"`) || !strings.Contains(stdout.String(), `"question_id":"study-outcome"`) {
		t.Fatalf("JSON output = %q", stdout.String())
	}

	stdout.Reset()
	gotCode = runTraceStudyAskAllCompare([]string{"first-study", "first-round", "second-study", "second-round"}, &stdout, &stderr, func(string, string, string, string) (trace.ReplicationStudyQuestionRoundComparison, error) {
		return want, nil
	})
	if gotCode != 0 || !strings.Contains(stdout.String(), "trace replication study question rounds compared") || !strings.Contains(stdout.String(), "change_kinds: result,outcome,evidence-state") {
		t.Fatalf("human output = %d, %q", gotCode, stdout.String())
	}
}

func TestRunTraceStudyAskAllCompareFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if gotCode := runTraceStudyAskAllCompare([]string{"first-study"}, &stdout, &stderr, nil); gotCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "trace study ask all compare") {
		t.Fatalf("argument failure = %d, stdout=%q, stderr=%q", gotCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if gotCode := runTraceStudyAskAllCompare([]string{"first-study", "first-round", "second-study", "second-round"}, &stdout, &stderr, func(string, string, string, string) (trace.ReplicationStudyQuestionRoundComparison, error) {
		return trace.ReplicationStudyQuestionRoundComparison{}, errors.New(`private-value`)
	}); gotCode != 1 || stdout.Len() != 0 || stderr.String() != "ariadne: trace study ask all compare: private-value\n" {
		t.Fatalf("comparison failure = %d, stdout=%q, stderr=%q", gotCode, stdout.String(), stderr.String())
	}
}
