package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerRendersTraceCaseReflection(t *testing.T) {
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

	indexRecorder := httptest.NewRecorder()
	h.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK || !strings.Contains(indexRecorder.Body.String(), "Open trace case review") {
		t.Fatalf("index status = %d, body=%q", indexRecorder.Code, indexRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case?question_id=ignored", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace case status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Trace case reflection",
		"Verified case identity",
		"caller",
		"archives",
		"replicated ledgers",
		"unknown entries",
		"case SHA-256",
		"browser-redacted-audit",
		trace.CaseQuestionSources,
		trace.CaseQuestionOutcomes,
		trace.CaseQuestionSupport,
		trace.CaseEntryTraceArchive,
		trace.CaseEntryTraceReplication,
		string(trace.ReplicatedChange),
		"evidence state",
		"unknown",
		"raw-value-free",
		"does not infer chronology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace case body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, casePath) || strings.Contains(body, "secret-value") || strings.Contains(body, "target-device") || strings.Contains(body, "--private-arg") {
		t.Fatal("trace case page disclosed a local path or unsafe value")
	}

	postRecorder := httptest.NewRecorder()
	h.ServeHTTP(postRecorder, httptest.NewRequest(http.MethodPost, "/trace-case", nil))
	if postRecorder.Code != http.StatusMethodNotAllowed || postRecorder.Header().Get("Allow") != "GET" {
		t.Fatalf("POST /trace-case status = %d, allow=%q, body=%q", postRecorder.Code, postRecorder.Header().Get("Allow"), postRecorder.Body.String())
	}
}

func TestHandlerTraceCaseFailsClosed(t *testing.T) {
	validCase, validSummary := validUITraceCase(t)
	for name, h := range map[string]handler{
		"unconfigured":       {},
		"reader unavailable": {traceCasePath: "case.json"},
		"reader error": {
			traceCasePath: "case.json",
			traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
				return trace.CasePackage{}, trace.CaseVerificationSummary{}, errors.New("private decoder path and raw value")
			},
		},
		"summary drift": {
			traceCasePath: "case.json",
			traceCaseRead: func(string) (trace.CasePackage, trace.CaseVerificationSummary, error) {
				validSummary.CaseSHA256 = strings.Repeat("a", 64)
				return validCase, validSummary, nil
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-case", nil))
			if name == "unconfigured" {
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace case unavailable\n" {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private") {
				t.Fatal("trace case page disclosed an internal verification error")
			}
		})
	}
}

func validUITraceCase(t *testing.T) (trace.CasePackage, trace.CaseVerificationSummary) {
	t.Helper()
	root := t.TempDir()
	archive, _ := validUITraceArchive(t)
	archivePath := filepath.Join(root, "archive.json")
	writeUIJSON(t, archivePath, archive)
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	if _, err := trace.SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}

	procedure := strings.Repeat("d", 64)
	inputs := make([]trace.ReplicationPairInput, 0, 2)
	for index, order := range []string{trace.OrderBaselineTreatment, trace.OrderTreatmentBaseline} {
		baselinePath := filepath.Join(root, "baseline-"+string(rune('a'+index))+".json")
		treatmentPath := filepath.Join(root, "treatment-"+string(rune('a'+index))+".json")
		writeUIJSON(t, baselinePath, uiReplicationTrace("region"))
		writeUIJSON(t, treatmentPath, uiReplicationTrace("region", "consent"))
		baselineSessionPath := filepath.Join(root, "baseline-session-"+string(rune('a'+index))+".json")
		treatmentSessionPath := filepath.Join(root, "treatment-session-"+string(rune('a'+index))+".json")
		if _, err := trace.SaveSessionPair(baselinePath, treatmentPath, baselineSessionPath, treatmentSessionPath, trace.SessionPairInput{
			Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: procedure, Scope: "outbound", Order: order,
		}); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, trace.ReplicationPairInput{
			BaselineTracePath: baselinePath, TreatmentTracePath: treatmentPath,
			BaselineSessionPath: baselineSessionPath, TreatmentSessionPath: treatmentSessionPath,
			ResetConfirmed: true,
		})
	}
	ledgerPath := filepath.Join(root, "replication.json")
	if _, err := trace.SaveReplicationLedger(inputs, ledgerPath); err != nil {
		t.Fatal(err)
	}
	ledgerRoundPath := filepath.Join(root, "replication-round.json")
	if _, err := trace.SaveReplicationQuestionRound(ledgerPath, ledgerRoundPath); err != nil {
		t.Fatal(err)
	}

	casePath := filepath.Join(root, "case.json")
	if _, err := trace.SaveCase([]trace.CaseInput{
		{Kind: trace.CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: trace.CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := trace.ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	return casePackage, summary
}

func uiReplicationTrace(fields ...string) trace.Document {
	return trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: fields,
		}},
	}
}

func writeUIJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
