package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestMinimizationInMemoryQuestionReceiptAndHelpers(t *testing.T) {
	summary := selectedMinimizationSummary()
	digest := strings.Repeat("c", 64)
	round, err := minimize.AnswerMinimizationQuestionRound(summary, digest)
	if err != nil {
		t.Fatal(err)
	}
	if got := selectedMinimizationQuestionID(minimize.MinimizationQuestionSelection); got != minimize.MinimizationQuestionSelection {
		t.Fatalf("selected question ID = %q", got)
	}
	if got := selectedMinimizationQuestionID("not-a-question"); got != "" {
		t.Fatalf("invalid selected question ID = %q", got)
	}
	answer, err := minimizationAnswerFromRound(round, minimize.MinimizationQuestionSelection)
	if err != nil || answer.QuestionID != minimize.MinimizationQuestionSelection {
		t.Fatalf("minimizationAnswerFromRound() = %#v, %v", answer, err)
	}
	if _, err := minimizationAnswerFromRound(round, "not-a-question"); err == nil {
		t.Fatal("minimizationAnswerFromRound() accepted an invalid ID")
	}

	h := newHandler(handler{
		minimizationPath: "run",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
			return summary, digest, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization?question_id="+minimize.MinimizationQuestionSelection, nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Selected question receipt") {
		t.Fatalf("in-memory receipt status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestReviewHandlerConfiguresMinimizationArtifacts(t *testing.T) {
	h := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:             t.TempDir(),
		MinimizationPath:        "run",
		MinimizationRoundPath:   "round.json",
		MinimizationReceiptPath: "receipt.json",
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
		t.Fatalf("configured minimization status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "round.json") || strings.Contains(recorder.Body.String(), "receipt.json") {
		t.Fatal("configured artifact path leaked")
	}
}

func TestHandlerFailsClosedOnMinimizationQuestionReaderErrors(t *testing.T) {
	summary := selectedMinimizationSummary()
	h := newHandler(handler{
		minimizationPath: "run",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
			return summary, strings.Repeat("c", 64), nil
		},
		minimizationRoundPath: "round.json",
		minimizationRoundRead: func(string) (minimize.MinimizationQuestionRound, minimize.MinimizationQuestionRoundVerificationSummary, error) {
			return minimize.MinimizationQuestionRound{}, minimize.MinimizationQuestionRoundVerificationSummary{}, errors.New("private round error")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
		t.Fatalf("round reader status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerKeepsLegacyMinimizationBeforeLadderFallback(t *testing.T) {
	summary := selectedMinimizationSummary()
	ladderCalled := false
	h := newHandler(handler{
		minimizationPath: "run",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
			return summary, strings.Repeat("c", 64), nil
		},
		minimizationLadderVerify: func(string) (minimize.LadderSummary, string, error) {
			ladderCalled = true
			return minimize.LadderSummary{}, "", errors.New("ladder fallback should not run")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	if recorder.Code != http.StatusOK || ladderCalled {
		t.Fatalf("legacy minimization status = %d, ladderCalled=%t, body=%q", recorder.Code, ladderCalled, recorder.Body.String())
	}
}
