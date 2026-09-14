package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestSourceAdapterViewExplainsVerifiedRun(t *testing.T) {
	summary := trace.SourceAdapterRunSummary{
		ReceiptSHA256: strings.Repeat("a", 64),
		Receipt: trace.SourceAdapterReceipt{
			Adapter: "external-desktop-v1", AdapterVersion: 1,
			Source: "desktop", Scope: "outbound", Completeness: trace.Complete,
			Events: 2, ProvenanceSHA256: strings.Repeat("b", 64),
		},
		Trace: trace.VerificationSummary{
			Completeness: trace.Complete, Events: 2,
			TraceSHA256: strings.Repeat("c", 64),
		},
		TraceEvents: []trace.Event{{Source: "desktop", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"location"}}},
		Session: trace.SessionVerificationSummary{
			Completeness: trace.Complete, SessionSHA256: strings.Repeat("d", 64),
		},
	}
	h := newHandler(handler{
		sourceAdapterPath: "C:/private/run",
		sourceAdapterVerify: func(path string) (trace.SourceAdapterRunSummary, error) {
			if path != "C:/private/run" {
				t.Fatalf("source adapter path = %q", path)
			}
			return summary, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/source-adapter", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"What did this source observe?", "2 observation(s)", "external-desktop-v1", "See the observed paths", "Recorded source-adapter path", "Reviewed category", "analytics", "location",
		"Provenance", "verified", "Replay", "unavailable", summary.ReceiptSHA256,
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body missing %q: %s", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "C:/private/run") {
		t.Fatal("body disclosed the source path")
	}

	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/source-adapter", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status = %d, allow = %q", post.Code, post.Header().Get("Allow"))
	}
}

func TestSourceAdapterViewRejectsUnavailableRun(t *testing.T) {
	h := newHandler(handler{
		sourceAdapterPath: "run",
		sourceAdapterVerify: func(string) (trace.SourceAdapterRunSummary, error) {
			return trace.SourceAdapterRunSummary{}, errors.New("private verifier detail")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/source-adapter", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "source adapter unavailable\n" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestSourceAdapterViewEscapesEventLabels(t *testing.T) {
	h := newHandler(handler{
		sourceAdapterPath: "run",
		sourceAdapterVerify: func(string) (trace.SourceAdapterRunSummary, error) {
			return trace.SourceAdapterRunSummary{
				Trace: trace.VerificationSummary{Completeness: trace.Complete},
				TraceEvents: []trace.Event{{
					Source: "<script>alert(1)</script>", Channel: "network",
					Kind: "request", Destination: "analytics", Fields: []string{"location"},
				}},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/source-adapter", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("escaped event labels = %d %q", recorder.Code, body)
	}
}
func TestIndexShowsSourceAdapterCard(t *testing.T) {
	h := newHandler(handler{
		root:              "archive",
		index:             func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		sourceAdapterPath: "run",
		sourceAdapterVerify: func(string) (trace.SourceAdapterRunSummary, error) {
			return trace.SourceAdapterRunSummary{}, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Open source-adapter explanation") {
		t.Fatalf("index response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func BenchmarkSourceAdapterReviewRequest(b *testing.B) {
	summary := trace.SourceAdapterRunSummary{
		ReceiptSHA256: strings.Repeat("a", 64),
		Receipt: trace.SourceAdapterReceipt{
			Adapter: "external-desktop-v1", AdapterVersion: 1,
			Source: "desktop", Scope: "outbound", Completeness: trace.Complete,
			Events: 2, ProvenanceSHA256: strings.Repeat("b", 64),
		},
		Trace: trace.VerificationSummary{
			Completeness: trace.Complete, Events: 2,
			TraceSHA256: strings.Repeat("c", 64),
		},
		TraceEvents: []trace.Event{
			{Source: "desktop", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"location"}},
			{Source: "desktop", Channel: "cookie", Kind: "cookie-write", Destination: "first-party", Fields: []string{"consent"}},
		},
		Session: trace.SessionVerificationSummary{
			Completeness: trace.Complete, SessionSHA256: strings.Repeat("d", 64),
		},
	}
	h := newHandler(handler{
		sourceAdapterPath: "run",
		sourceAdapterVerify: func(string) (trace.SourceAdapterRunSummary, error) {
			return summary, nil
		},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/source-adapter", nil))
		if recorder.Code != http.StatusOK {
			b.Fatal(recorder.Code)
		}
	}
}
