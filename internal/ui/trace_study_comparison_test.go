package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestHandlerRendersTraceStudyComparison(t *testing.T) {
	comparison := trace.ReplicationStudyQuestionRoundComparison{
		SchemaVersion:      1,
		ComparisonID:       "replication-study-question-round-answer-change",
		ComparisonQuestion: "<script>private comparison question</script>",
		OrderBasis:         trace.ReplicationStudyOrderBasis,
		Result:             "changed",
		FirstRoundSHA256:   strings.Repeat("a", 64),
		SecondRoundSHA256:  strings.Repeat("b", 64),
		FirstStudySHA256:   strings.Repeat("c", 64),
		SecondStudySHA256:  strings.Repeat("d", 64),
		Compared:           3,
		Changed:            1,
		ChangedQuestions: []trace.ReplicationStudyQuestionRoundChange{{
			QuestionID:          trace.StudyQuestionOutcome,
			FirstResult:         "<b>replicated-change</b>",
			SecondResult:        "unknown",
			FirstEvidenceState:  evidence.Observed,
			SecondEvidenceState: evidence.Unknown,
			FirstOutcome:        trace.ReplicatedChange,
			SecondOutcome:       trace.ReplicationUnknown,
			ChangeKinds:         []string{"result", "outcome", "evidence-state", "support-counts"},
		}},
	}
	h := newHandler(handler{
		root:                 "archive-root",
		index:                func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		traceStudyPath:       "first-study.json",
		traceStudyComparison: func() (trace.ReplicationStudyQuestionRoundComparison, error) { return comparison, nil },
	})

	indexRecorder := httptest.NewRecorder()
	h.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK || !strings.Contains(indexRecorder.Body.String(), "Open retained study comparison") {
		t.Fatalf("index status = %d, body=%q", indexRecorder.Code, indexRecorder.Body.String())
	}

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study-comparison", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("comparison status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Compare retained study reflections",
		"same",
		"changed",
		"incomparable",
		"caller",
		"first study SHA-256",
		"second round SHA-256",
		"compared",
		"Changed questions",
		trace.StudyQuestionOutcome,
		"first result",
		"second result",
		"first outcome",
		"second outcome",
		"first evidence state",
		"second evidence state",
		"support-counts",
		"does not establish chronology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("comparison body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, "<b>replicated-change</b>") {
		t.Fatal("comparison page did not escape dynamic values")
	}
	if !strings.Contains(body, "&lt;script&gt;") || !strings.Contains(body, "&lt;b&gt;replicated-change&lt;/b&gt;") {
		t.Fatal("comparison page escaped-value evidence missing")
	}
	for _, private := range []string{"first-study.json", "target-device", "https://", "--private-arg"} {
		if strings.Contains(body, private) {
			t.Fatalf("comparison page disclosed private detail %q", private)
		}
	}

	postRecorder := httptest.NewRecorder()
	h.ServeHTTP(postRecorder, httptest.NewRequest(http.MethodPost, "/trace-study-comparison", nil))
	if postRecorder.Code != http.StatusMethodNotAllowed || postRecorder.Header().Get("Allow") != "GET" {
		t.Fatalf("POST comparison status = %d, allow=%q, body=%q", postRecorder.Code, postRecorder.Header().Get("Allow"), postRecorder.Body.String())
	}
}

func TestHandlerTraceStudyComparisonFailsClosed(t *testing.T) {
	valid := trace.ReplicationStudyQuestionRoundComparison{Result: "same"}
	for name, h := range map[string]handler{
		"unconfigured": {},
		"comparison error": {
			traceStudyComparison: func() (trace.ReplicationStudyQuestionRoundComparison, error) {
				return trace.ReplicationStudyQuestionRoundComparison{}, errors.New("private parser path and captured value")
			},
		},
		"valid": {
			traceStudyComparison: func() (trace.ReplicationStudyQuestionRoundComparison, error) {
				return valid, nil
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study-comparison/suffix", nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("unexpected route status = %d, body=%q", recorder.Code, recorder.Body.String())
			}

			recorder = httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study-comparison", nil))
			if name == "unconfigured" {
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("unconfigured status = %d, body=%q", recorder.Code, recorder.Body.String())
				}
				return
			}
			if name == "comparison error" {
				if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace study comparison unavailable\n" {
					t.Fatalf("error status = %d, body=%q", recorder.Code, recorder.Body.String())
				}
				if strings.Contains(recorder.Body.String(), "private") {
					t.Fatal("comparison error leaked internal details")
				}
				return
			}
			if recorder.Code != http.StatusOK {
				t.Fatalf("valid status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
		})
	}
}
func TestHandlerReportsIncomparableStudyComparisonAsNotCompared(t *testing.T) {
	h := newHandler(handler{
		traceStudyComparison: func() (trace.ReplicationStudyQuestionRoundComparison, error) {
			return trace.ReplicationStudyQuestionRoundComparison{Result: "incomparable"}, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace-study-comparison", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("incomparable status = %d, body=%q", recorder.Code, body)
	}
	if !strings.Contains(body, "No fixed question projection was compared.") {
		t.Fatal("incomparable comparison did not report that no projections were compared")
	}
	if strings.Contains(body, "No fixed question projection changed.") {
		t.Fatal("incomparable comparison was rendered as unchanged")
	}
}
