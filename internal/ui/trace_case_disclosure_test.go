package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerRendersTraceCaseDisclosureQuestionsAndReceipt(t *testing.T) {
	casePackage, summary := validUITraceCase(t)
	casePath := `C:\private\trace-case.json`
	h := newHandler(handler{
		root:          "archive-root",
		index:         func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceCasePath: casePath,
		traceCaseRead: func(path string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
			if path != casePath {
				t.Fatalf("ReadCase() path = %q", path)
			}
			return casePackage, summary, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case?disclosure_question_id="+trace.CaseDisclosureQuestionOverlap, nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace case status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Disclosure map questions",
		trace.CaseDisclosureQuestionCoverage,
		trace.CaseDisclosureQuestionOverlap,
		"overlapping categories",
		"Reviewed boundaries by category",
		"Selected receipt",
		"Receipt JSON",
		"receipt SHA-256",
		"raw-value-free",
		"region",
		"browser-redacted-audit",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace case disclosure body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, casePath) || strings.Contains(body, "secret-value") || strings.Contains(body, "target-device") || strings.Contains(body, "--private-arg") {
		t.Fatal("trace case disclosure page disclosed a local path or unsafe value")
	}

	invalidRecorder := httptest.NewRecorder()
	h.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/trace-case?disclosure_question_id=not-a-question", nil))
	if invalidRecorder.Code != http.StatusNotFound || strings.Contains(invalidRecorder.Body.String(), casePath) {
		t.Fatalf("invalid disclosure question status = %d, body=%q", invalidRecorder.Code, invalidRecorder.Body.String())
	}
}
