package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerRendersSavedTraceCaseRoundAndReceipt(t *testing.T) {
	casePackage, caseSummary := validUITraceCase(t)
	round, err := trace.AnswerCaseDisclosureQuestionRound(casePackage, caseSummary)
	if err != nil {
		t.Fatal(err)
	}
	roundSHA256, err := trace.CaseDisclosureQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := trace.AskCaseDisclosureQuestionRound(writeUITraceCaseRound(t, round), trace.CaseDisclosureQuestionOverlap)
	if err != nil {
		t.Fatal(err)
	}
	receipt := trace.CaseDisclosureQuestionReceipt{
		CaseDisclosureQuestionAnswer: answer,
		RoundSHA256:                  roundSHA256,
		Round:                        round,
	}
	receiptSHA256, err := trace.CaseDisclosureQuestionReceiptSHA256(receipt)
	if err != nil {
		t.Fatal(err)
	}

	casePath := `C:\private\trace-case.json`
	roundPath := `C:\private\trace-case-round.json`
	receiptPath := `C:\private\trace-case-receipt.json`
	h := newHandler(handler{
		root:          "archive-root",
		index:         func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceCasePath: casePath,
		traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
			return casePackage, caseSummary, nil
		},
		traceCaseRoundPath: roundPath,
		traceCaseRoundRead: func(path string) (trace.CaseDisclosureQuestionRound, trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
			if path != roundPath {
				t.Fatalf("round path = %q", path)
			}
			return round, trace.CaseDisclosureQuestionRoundVerificationSummary{
				SchemaVersion: round.SchemaVersion,
				CaseSHA256:    round.CaseSHA256,
				Questions:     len(round.Answers),
				RoundSHA256:   roundSHA256,
			}, nil
		},
		traceCaseReceiptPath: receiptPath,
		traceCaseReceiptRead: func(path string) (trace.CaseDisclosureQuestionReceipt, trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
			if path != receiptPath {
				t.Fatalf("receipt path = %q", path)
			}
			return receipt, trace.CaseDisclosureQuestionReceiptVerificationSummary{
				SchemaVersion: receipt.SchemaVersion,
				QuestionID:    receipt.QuestionID,
				Question:      receipt.Question,
				Result:        receipt.Result,
				EvidenceState: receipt.EvidenceState,
				CaseSHA256:    receipt.CaseSHA256,
				RoundSHA256:   receipt.RoundSHA256,
				ReceiptSHA256: receiptSHA256,
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{"saved question round: verified against the current case", "saved and verified", trace.CaseDisclosureQuestionOverlap, receiptSHA256} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{casePath, roundPath, receiptPath} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("body disclosed path %q", forbidden)
		}
	}
}

func TestHandlerRejectsTraceCaseArtifactDrift(t *testing.T) {
	casePackage, caseSummary := validUITraceCase(t)
	round, err := trace.AnswerCaseDisclosureQuestionRound(casePackage, caseSummary)
	if err != nil {
		t.Fatal(err)
	}
	roundSHA256, err := trace.CaseDisclosureQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	validHandler := func() handler {
		return handler{
			traceCasePath: "case.json",
			traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
				return casePackage, caseSummary, nil
			},
			traceCaseRoundPath: "round.json",
			traceCaseRoundRead: func(string) (trace.CaseDisclosureQuestionRound, trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
				return round, trace.CaseDisclosureQuestionRoundVerificationSummary{CaseSHA256: round.CaseSHA256, RoundSHA256: roundSHA256}, nil
			},
		}
	}
	t.Run("round reader unavailable", func(t *testing.T) {
		h := validHandler()
		h.traceCaseRoundRead = nil
		recorder := httptest.NewRecorder()
		newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
		if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace case unavailable\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("round identity drift", func(t *testing.T) {
		h := validHandler()
		h.traceCaseRoundRead = func(string) (trace.CaseDisclosureQuestionRound, trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
			return round, trace.CaseDisclosureQuestionRoundVerificationSummary{CaseSHA256: round.CaseSHA256, RoundSHA256: strings.Repeat("a", 64)}, nil
		}
		recorder := httptest.NewRecorder()
		newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
		if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace case unavailable\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("receipt question mismatch", func(t *testing.T) {
		h := validHandler()
		h.traceCaseReceiptPath = "receipt.json"
		h.traceCaseReceiptRead = func(string) (trace.CaseDisclosureQuestionReceipt, trace.CaseDisclosureQuestionReceiptVerificationSummary, error) {
			return trace.CaseDisclosureQuestionReceipt{CaseDisclosureQuestionAnswer: trace.CaseDisclosureQuestionAnswer{CaseSHA256: caseSummary.CaseSHA256}, RoundSHA256: roundSHA256}, trace.CaseDisclosureQuestionReceiptVerificationSummary{}, nil
		}
		recorder := httptest.NewRecorder()
		newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case?disclosure_question_id="+trace.CaseDisclosureQuestionOverlap, nil))
		if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace case unavailable\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("reader error stays generic", func(t *testing.T) {
		h := validHandler()
		h.traceCaseRoundRead = func(string) (trace.CaseDisclosureQuestionRound, trace.CaseDisclosureQuestionRoundVerificationSummary, error) {
			return trace.CaseDisclosureQuestionRound{}, trace.CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("private path and raw value")
		}
		recorder := httptest.NewRecorder()
		newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
		if recorder.Code != http.StatusUnprocessableEntity || strings.Contains(recorder.Body.String(), "private") {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})
}

func writeUITraceCaseRound(t *testing.T, round trace.CaseDisclosureQuestionRound) string {
	t.Helper()
	path := t.TempDir() + "\\round.json"
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
