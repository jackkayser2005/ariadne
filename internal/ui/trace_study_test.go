package ui

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerRendersTraceStudyReflection(t *testing.T) {
	study, summary, studyPath := validUITraceStudy(t)
	h := newHandler(handler{
		root:           "archive-root",
		index:          func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceStudyPath: studyPath,
		traceStudyRead: func(path string) (trace.ReplicationStudy, trace.StudyVerificationSummary, error) {
			if path != studyPath {
				t.Fatalf("ReadReplicationStudy() path = %q", path)
			}
			return study, summary, nil
		},
	})

	indexRecorder := httptest.NewRecorder()
	h.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK || !strings.Contains(indexRecorder.Body.String(), "Open replication study review") {
		t.Fatalf("index status = %d, body=%q", indexRecorder.Code, indexRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study?question_id=ignored", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace study status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Replication study reflection",
		"Verified replication study identity",
		"contrast commitment SHA-256",
		"caller",
		"supported runs",
		"question round SHA-256",
		trace.StudyQuestionOutcome,
		trace.StudyQuestionSupport,
		trace.StudyQuestionConsistency,
		string(trace.ReplicatedChange),
		"supported",
		"observed",
		"raw-value-free",
		"does not infer chronology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace study body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, studyPath) || strings.Contains(body, "secret-value") || strings.Contains(body, "target-device") || strings.Contains(body, "--private-arg") {
		t.Fatal("trace study page disclosed a local path or unsafe value")
	}

	postRecorder := httptest.NewRecorder()
	h.ServeHTTP(postRecorder, httptest.NewRequest(http.MethodPost, "/trace-study", nil))
	if postRecorder.Code != http.StatusMethodNotAllowed || postRecorder.Header().Get("Allow") != "GET" {
		t.Fatalf("POST /trace-study status = %d, allow=%q, body=%q", postRecorder.Code, postRecorder.Header().Get("Allow"), postRecorder.Body.String())
	}
}

func TestHandlerTraceStudyFailsClosed(t *testing.T) {
	validStudy, validSummary, studyPath := validUITraceStudy(t)
	validStudyReader := func(string) (trace.ReplicationStudy, trace.StudyVerificationSummary, error) {
		return validStudy, validSummary, nil
	}
	for name, h := range map[string]handler{
		"unconfigured":       {},
		"reader unavailable": {traceStudyPath: "study.json"},
		"reader error": {
			traceStudyPath: "study.json",
			traceStudyRead: func(string) (trace.ReplicationStudy, trace.StudyVerificationSummary, error) {
				return trace.ReplicationStudy{}, trace.StudyVerificationSummary{}, errors.New("private decoder path and raw value")
			},
		},
		"summary drift": {
			traceStudyPath: studyPath,
			traceStudyRead: func(string) (trace.ReplicationStudy, trace.StudyVerificationSummary, error) {
				drifted := validSummary
				drifted.StudySHA256 = strings.Repeat("a", 64)
				return validStudy, drifted, nil
			},
		},
		"round reader unavailable": {
			traceStudyPath:      studyPath,
			traceStudyRead:      validStudyReader,
			traceStudyRoundPath: "round.json",
		},
		"receipt reader unavailable": {
			traceStudyPath:        studyPath,
			traceStudyRead:        validStudyReader,
			traceStudyReceiptPath: "receipt.json",
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study", nil))
			if name == "unconfigured" {
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace study unavailable\n" {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private") {
				t.Fatal("trace study page disclosed an internal verification error")
			}
		})
	}
}

func validUITraceStudy(t *testing.T) (trace.ReplicationStudy, trace.StudyVerificationSummary, string) {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("d", 64)
	studyInputs := make([]trace.StudyInput, 0, 2)
	for runIndex, fields := range [][]string{{"region", "session-id"}, {"region", "location"}} {
		pairInputs := make([]trace.ReplicationPairInput, 0, 2)
		for pairIndex, order := range []string{trace.OrderBaselineTreatment, trace.OrderTreatmentBaseline} {
			name := fmt.Sprintf("run-%d-pair-%d", runIndex+1, pairIndex+1)
			baselinePath := filepath.Join(root, name+"-baseline.json")
			treatmentPath := filepath.Join(root, name+"-treatment.json")
			baseline := uiReplicationTrace(fields...)
			treatmentFields := append(append([]string{}, fields...), "consent")
			treatment := uiReplicationTrace(treatmentFields...)
			writeUIJSON(t, baselinePath, baseline)
			writeUIJSON(t, treatmentPath, treatment)
			baselineSessionPath := filepath.Join(root, name+"-baseline-session.json")
			treatmentSessionPath := filepath.Join(root, name+"-treatment-session.json")
			if _, err := trace.SaveSessionPair(baselinePath, treatmentPath, baselineSessionPath, treatmentSessionPath, trace.SessionPairInput{
				Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: procedure, Scope: "outbound", Order: order,
			}); err != nil {
				t.Fatal(err)
			}
			pairInputs = append(pairInputs, trace.ReplicationPairInput{
				BaselineTracePath: baselinePath, TreatmentTracePath: treatmentPath,
				BaselineSessionPath: baselineSessionPath, TreatmentSessionPath: treatmentSessionPath,
				ResetConfirmed: true,
			})
		}
		ledgerPath := filepath.Join(root, fmt.Sprintf("ledger-%d.json", runIndex+1))
		if _, err := trace.SaveReplicationLedger(pairInputs, ledgerPath); err != nil {
			t.Fatal(err)
		}
		roundPath := filepath.Join(root, fmt.Sprintf("round-%d.json", runIndex+1))
		if _, err := trace.SaveReplicationQuestionRound(ledgerPath, roundPath); err != nil {
			t.Fatal(err)
		}
		studyInputs = append(studyInputs, trace.StudyInput{LedgerPath: ledgerPath, RoundPath: roundPath})
	}
	studyPath := filepath.Join(root, "study.json")
	if _, err := trace.SaveReplicationStudy(strings.Repeat("a", 64), studyInputs, studyPath); err != nil {
		t.Fatal(err)
	}
	study, summary, err := trace.ReadReplicationStudy(studyPath)
	if err != nil {
		t.Fatal(err)
	}
	return study, summary, studyPath
}

func TestHandlerRendersDurableTraceStudyRoundAndReceipt(t *testing.T) {
	study, summary, studyPath := validUITraceStudy(t)
	roundPath := filepath.Join(t.TempDir(), "study-round.json")
	if _, err := trace.SaveReplicationStudyQuestionRound(studyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(t.TempDir(), "study-receipt.json")
	if _, err := trace.SaveReplicationStudyQuestionReceipt(roundPath, trace.StudyQuestionOutcome, receiptPath); err != nil {
		t.Fatal(err)
	}
	h := HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceCaseAndStudyArtifacts(
		"", "", "", "", "", "", "", "", "", "", "",
		studyPath, roundPath, receiptPath,
	)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study?question_id="+trace.StudyQuestionOutcome, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace study status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"Durable question round",
		"Selected question receipt",
		summary.StudySHA256,
		trace.StudyQuestionOutcome,
		"receipt SHA-256",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace study body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, studyPath) || strings.Contains(body, roundPath) || strings.Contains(body, receiptPath) {
		t.Fatal("durable study page disclosed a local path")
	}
	_ = study
}

func TestHandlerTraceStudyFailsClosedOnDurableDrift(t *testing.T) {
	study, summary, studyPath := validUITraceStudy(t)
	round, err := trace.AnswerReplicationStudyQuestionRound(study, summary)
	if err != nil {
		t.Fatal(err)
	}
	round.Answers[0].Result = string(trace.NoChangeObserved)
	h := handler{
		traceStudyPath:      studyPath,
		traceStudyRead:      trace.ReadReplicationStudy,
		traceStudyRoundPath: "round.json",
		traceStudyRoundRead: func(string) (trace.ReplicationStudyQuestionRound, trace.ReplicationStudyQuestionRoundVerificationSummary, error) {
			return round, trace.ReplicationStudyQuestionRoundVerificationSummary{
				SchemaVersion: 1,
				StudySHA256:   summary.StudySHA256,
				Questions:     3,
				RoundSHA256:   strings.Repeat("a", 64),
			}, nil
		},
	}
	recorder := httptest.NewRecorder()
	newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace study unavailable\n" {
		t.Fatalf("tampered study round status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRendersInMemoryTraceStudyReceipt(t *testing.T) {
	_, _, studyPath := validUITraceStudy(t)
	h := newHandler(handler{
		traceStudyPath: studyPath,
		traceStudyRead: trace.ReadReplicationStudy,
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study?question_id="+trace.StudyQuestionOutcome, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("trace study status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{"Selected question receipt", "receipt SHA-256", trace.StudyQuestionOutcome, "raw-value-free"} {
		if !strings.Contains(body, want) {
			t.Fatalf("in-memory receipt body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, studyPath) {
		t.Fatal("in-memory receipt page disclosed a local path")
	}
}
