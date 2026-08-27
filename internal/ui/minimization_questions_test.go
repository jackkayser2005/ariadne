package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestHandlerRendersSavedMinimizationQuestionArtifacts(t *testing.T) {
	summary := selectedMinimizationSummary()
	minimizationSHA256 := strings.Repeat("c", 64)
	round, err := minimize.AnswerMinimizationQuestionRound(summary, minimizationSHA256)
	if err != nil {
		t.Fatal(err)
	}
	roundSHA256, err := minimize.MinimizationQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := minimize.MinimizationQuestionReceipt{
		MinimizationQuestionAnswer: round.Answers[0],
		RoundSHA256:                roundSHA256,
		Round:                      round,
	}
	receiptSHA256, err := minimize.MinimizationQuestionReceiptSHA256(receipt)
	if err != nil {
		t.Fatal(err)
	}
	roundSummary := minimize.MinimizationQuestionRoundVerificationSummary{
		SchemaVersion:      round.SchemaVersion,
		MinimizationSHA256: round.MinimizationSHA256,
		Questions:          len(round.Answers),
		Candidates:         len(round.Candidates),
		RoundSHA256:        roundSHA256,
	}
	receiptSummary := minimize.MinimizationQuestionReceiptVerificationSummary{
		SchemaVersion:       receipt.SchemaVersion,
		QuestionID:          receipt.QuestionID,
		Question:            receipt.Question,
		Result:              receipt.Result,
		EvidenceState:       receipt.EvidenceState,
		MinimizationSHA256:  receipt.MinimizationSHA256,
		RoundSHA256:         receipt.RoundSHA256,
		ReceiptSHA256:       receiptSHA256,
		SelectionState:      receipt.SelectionState,
		SelectedCandidate:   receipt.SelectedCandidate,
		CandidateCount:      receipt.CandidateCount,
		SupportedCandidates: receipt.SupportedCandidates,
		UnknownCandidates:   receipt.UnknownCandidates,
	}
	h := newHandler(handler{
		minimizationPath:      `C:\private\minimization-secret-value`,
		minimizationVerify:    func(string) (minimize.MinimizationSummary, string, error) { return summary, minimizationSHA256, nil },
		minimizationRoundPath: `C:\private\round-secret-path`,
		minimizationRoundRead: func(string) (minimize.MinimizationQuestionRound, minimize.MinimizationQuestionRoundVerificationSummary, error) {
			return round, roundSummary, nil
		},
		minimizationReceiptPath: `C:\private\receipt-secret-path`,
		minimizationReceiptRead: func(string) (minimize.MinimizationQuestionReceipt, minimize.MinimizationQuestionReceiptVerificationSummary, error) {
			return receipt, receiptSummary, nil
		},
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/minimization?question_id="+minimize.MinimizationQuestionSelection, nil)
	h.ServeHTTP(recorder, request)
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Fixed minimization questions",
		"Durable question round",
		"Selected question receipt",
		minimize.MinimizationQuestionSelection,
		"question result",
		"Question result and evidence state are independent",
		roundSHA256,
		receiptSHA256,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, secret := range []string{"private", "secret-value", "round-secret-path", "receipt-secret-path", "raw-secret-manifest"} {
		if strings.Contains(body, secret) {
			t.Fatalf("question page disclosed %q", secret)
		}
	}
}

func TestHandlerRejectsUnknownMinimizationQuestionID(t *testing.T) {
	summary := selectedMinimizationSummary()
	h := newHandler(handler{
		minimizationPath: "minimization",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
			return summary, strings.Repeat("c", 64), nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization?question_id=unknown", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerFailsClosedOnMinimizationQuestionIdentityDrift(t *testing.T) {
	summary := selectedMinimizationSummary()
	minimizationSHA256 := strings.Repeat("c", 64)
	round, err := minimize.AnswerMinimizationQuestionRound(summary, minimizationSHA256)
	if err != nil {
		t.Fatal(err)
	}
	roundSHA256, err := minimize.MinimizationQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := minimize.MinimizationQuestionReceipt{
		MinimizationQuestionAnswer: round.Answers[0],
		RoundSHA256:                roundSHA256,
		Round:                      round,
	}
	base := handler{
		minimizationPath:   "minimization",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) { return summary, minimizationSHA256, nil },
	}
	tests := []struct {
		name   string
		mutate func(*handler)
	}{
		{name: "round digest", mutate: func(h *handler) {
			h.minimizationRoundPath = "round.json"
			h.minimizationRoundRead = func(string) (minimize.MinimizationQuestionRound, minimize.MinimizationQuestionRoundVerificationSummary, error) {
				bad := round
				bad.MinimizationSHA256 = strings.Repeat("d", 64)
				return bad, minimize.MinimizationQuestionRoundVerificationSummary{}, nil
			}
		}},
		{name: "round reader unavailable", mutate: func(h *handler) {
			h.minimizationRoundPath = "round.json"
		}},
		{name: "receipt digest", mutate: func(h *handler) {
			h.minimizationReceiptPath = "receipt.json"
			h.minimizationReceiptRead = func(string) (minimize.MinimizationQuestionReceipt, minimize.MinimizationQuestionReceiptVerificationSummary, error) {
				bad := receipt
				bad.MinimizationSHA256 = strings.Repeat("d", 64)
				return bad, minimize.MinimizationQuestionReceiptVerificationSummary{}, nil
			}
		}},
		{name: "receipt reader error", mutate: func(h *handler) {
			h.minimizationReceiptPath = "receipt.json"
			h.minimizationReceiptRead = func(string) (minimize.MinimizationQuestionReceipt, minimize.MinimizationQuestionReceiptVerificationSummary, error) {
				return minimize.MinimizationQuestionReceipt{}, minimize.MinimizationQuestionReceiptVerificationSummary{}, errors.New("private receipt error")
			}
		}},
		{name: "selected question mismatch", mutate: func(h *handler) {
			h.minimizationReceiptPath = "receipt.json"
			h.minimizationReceiptRead = func(string) (minimize.MinimizationQuestionReceipt, minimize.MinimizationQuestionReceiptVerificationSummary, error) {
				bad := receipt
				bad.QuestionID = minimize.MinimizationQuestionSupport
				return bad, minimize.MinimizationQuestionReceiptVerificationSummary{}, nil
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := base
			test.mutate(&h)
			recorder := httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization?question_id="+minimize.MinimizationQuestionSelection, nil))
			if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private") {
				t.Fatal("identity error leaked")
			}
		})
	}
}
