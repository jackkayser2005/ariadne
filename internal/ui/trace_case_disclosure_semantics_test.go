package ui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerPreservesPartialDisclosureSemantics(t *testing.T) {
	casePackage, summary := partialPositiveTraceCase(t)
	h := newHandler(handler{
		root:          "archive-root",
		index:         func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceCasePath: "case.json",
		traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
			return casePackage, summary, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "status status-overlap-observed") || !strings.Contains(body, "coverage state") || !strings.Contains(body, "status status-unknown") {
		t.Fatalf("partial positive UI = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "secret-value") || strings.Contains(body, "https://") || strings.Contains(body, "private") {
		t.Fatal("partial positive UI disclosed unsafe content")
	}
}

func TestHandlerRendersPartialNoOverlapAsUnknown(t *testing.T) {
	casePackage, summary := validUITraceCase(t)
	h := newHandler(handler{
		root:          "archive-root",
		index:         func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceCasePath: "case.json",
		traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
			return casePackage, summary, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "no cross-boundary overlap was observed") || !strings.Contains(body, "status status-unknown") {
		t.Fatalf("partial no-overlap UI = %d, body=%q", recorder.Code, body)
	}
}

func partialPositiveTraceCase(t *testing.T) (trace.CasePackage, trace.CaseVerificationSummary) {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("f", 64)
	inputs := make([]trace.ArchiveInput, 0, 2)
	for index, candidate := range []struct {
		name         string
		adapter      string
		completeness string
	}{
		{name: "partial", adapter: "browser-redacted-audit", completeness: trace.Partial},
		{name: "complete", adapter: "browser-local-fixture", completeness: trace.Complete},
	} {
		document := trace.Document{
			SchemaVersion: 1,
			Redacted:      true,
			Scope:         "outbound",
			Completeness:  candidate.completeness,
			Events: []trace.Event{{
				Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
			}},
		}
		tracePath := filepath.Join(root, candidate.name+"-trace-"+string(rune('a'+index))+".json")
		sessionPath := filepath.Join(root, candidate.name+"-session-"+string(rune('a'+index))+".json")
		writeUIJSON(t, tracePath, document)
		if _, err := trace.SaveSession(tracePath, sessionPath, trace.SessionInput{
			Adapter: candidate.adapter, AdapterVersion: 1, ProcedureSHA256: procedure, Role: trace.RoleStandalone, Order: trace.OrderStandalone,
		}); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, trace.ArchiveInput{TracePath: tracePath, SessionPath: sessionPath})
	}
	archivePath := filepath.Join(root, "archive.json")
	if _, err := trace.SaveArchive(inputs, archivePath); err != nil {
		t.Fatal(err)
	}
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	if _, err := trace.SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(root, "case.json")
	if _, err := trace.SaveCase([]trace.CaseInput{{Kind: trace.CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := trace.ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	return casePackage, summary
}
