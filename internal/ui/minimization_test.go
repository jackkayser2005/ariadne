package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestHandlerRendersMinimizationReview(t *testing.T) {
	summary := selectedMinimizationSummary()
	rootPath := `C:\private\minimization-secret-value`
	rootDigest := strings.Repeat("c", 64)
	h := newHandler(handler{
		root:             "archive-root",
		index:            func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		minimizationPath: rootPath,
		minimizationVerify: func(path string) (minimize.MinimizationSummary, string, error) {
			if path != rootPath {
				t.Fatalf("VerifyWithIdentity() path = %q", path)
			}
			return summary, rootDigest, nil
		},
	})

	indexRecorder := httptest.NewRecorder()
	h.ServeHTTP(indexRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if indexRecorder.Code != http.StatusOK {
		t.Fatalf("index status = %d, body=%q", indexRecorder.Code, indexRecorder.Body.String())
	}
	for _, want := range []string{"Minimum-disclosure experiment", "Open minimization review"} {
		if !strings.Contains(indexRecorder.Body.String(), want) {
			t.Fatalf("index body missing %q: %s", want, indexRecorder.Body.String())
		}
	}

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("minimization status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Review tested disclosure levels.",
		"Verified minimization identity",
		"raw-value-free",
		"Minimum tested sufficient disclosure",
		"android-location-minimize",
		"location",
		"exact",
		"city",
		"omitted",
		"sufficient",
		"no-change-observed",
		"observed",
		"pairs per order",
		"total ordered runs",
		rootDigest,
		strings.Repeat("a", 64),
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("minimization body missing %q: %s", want, body)
		}
	}
	if strings.Index(body, "city") > strings.Index(body, "omitted") {
		t.Fatal("candidate ladder order was not preserved")
	}
	for _, secret := range []string{"private", "secret-value", "raw-secret-manifest", "candidate-001-city-secret-path"} {
		if strings.Contains(body, secret) {
			t.Fatalf("minimization page disclosed %q", secret)
		}
	}

	postRecorder := httptest.NewRecorder()
	h.ServeHTTP(postRecorder, httptest.NewRequest(http.MethodPost, "/minimization", nil))
	if postRecorder.Code != http.StatusMethodNotAllowed || postRecorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /minimization status = %d, allow=%q, body=%q", postRecorder.Code, postRecorder.Header().Get("Allow"), postRecorder.Body.String())
	}
}

func TestHandlerRendersMinimizationUncertaintySeparately(t *testing.T) {
	summary := selectedMinimizationSummary()
	summary.EvidenceState = evidence.Unknown
	summary.SelectionState = minimize.SelectionUnknown
	summary.SelectedCandidate = ""
	summary.CandidateResults[0].Classification = minimize.CandidateInsufficient
	summary.CandidateResults[0].Outcome = bundle.ReplicatedChange
	summary.CandidateResults[0].ChangedPairs = 4
	summary.CandidateResults[0].NoChangePairs = 0
	summary.CandidateResults[1].Classification = minimize.CandidateUnknown
	summary.CandidateResults[1].Outcome = bundle.ReplicationUnknown
	summary.CandidateResults[1].EvidenceState = evidence.Unknown
	summary.CandidateResults[1].CompletedPairs = 0
	summary.CandidateResults[1].ChangedPairs = 0
	summary.CandidateResults[1].NoChangePairs = 0
	summary.CandidateResults[1].UnknownPairs = 4
	h := newHandler(handler{
		minimizationPath: "minimization",
		minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
			return summary, strings.Repeat("d", 64), nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("minimization uncertainty status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"No minimum tested sufficient disclosure was established",
		"insufficient",
		"replicated-change",
		"unknown",
		"No minimum tested sufficient disclosure",
		"Classification is the bounded functionality conclusion",
		"Outcome summarizes the replicated counterfactual result",
		"Evidence state qualifies support",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("uncertainty body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "selected candidate</dt>") {
		t.Fatal("uncertain minimization page rendered a selected candidate")
	}
}

func TestHandlerHidesMinimizationErrorsAndUnconfiguredRoute(t *testing.T) {
	for name, h := range map[string]handler{
		"unconfigured":       {},
		"reader unavailable": {minimizationPath: "minimization"},
		"reader error": {
			minimizationPath: "minimization",
			minimizationVerify: func(string) (minimize.MinimizationSummary, string, error) {
				return minimize.MinimizationSummary{}, "", errors.New("private path and raw secret")
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newHandler(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
			if h.minimizationPath == "" {
				if recorder.Code != http.StatusNotFound {
					t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
				}
			} else if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private") || strings.Contains(recorder.Body.String(), "secret") {
				t.Fatal("minimization route disclosed internal verification error")
			}
		})
	}
}

func TestHandlerWithReviewOptionsChecksHostAndSetsSecurityHeaders(t *testing.T) {
	h := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:  t.TempDir(),
		ExpectedHost: "127.0.0.1:8787",
	})

	allowed := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/", nil)
	allowed.Host = "127.0.0.1:8787"
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, allowed)
	if recorder.Code != http.StatusOK {
		t.Fatalf("allowed host status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
	for header, want := range map[string]string{
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'",
		"Referrer-Policy":         "no-referrer",
		"X-Content-Type-Options":  "nosniff",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Fatalf("header %s = %q, want %q", header, got, want)
		}
	}

	rejected := httptest.NewRequest(http.MethodGet, "http://evil.example/", nil)
	rejected.Host = "evil.example"
	recorder = httptest.NewRecorder()
	h.ServeHTTP(recorder, rejected)
	if recorder.Code != http.StatusMisdirectedRequest || recorder.Body.String() != "host not allowed\n" {
		t.Fatalf("rejected host status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerNormalizesEquivalentHostAuthorities(t *testing.T) {
	defaultPortHandler := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:  t.TempDir(),
		ExpectedHost: "127.0.0.1:80",
	})
	defaultPortRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	defaultPortRequest.Host = "127.0.0.1"
	recorder := httptest.NewRecorder()
	defaultPortHandler.ServeHTTP(recorder, defaultPortRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("default-port host status = %d, body=%q", recorder.Code, recorder.Body.String())
	}

	ipv6Handler := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:  t.TempDir(),
		ExpectedHost: "[::1]:8787",
	})
	ipv6Request := httptest.NewRequest(http.MethodGet, "http://[::1]:8787/", nil)
	ipv6Request.Host = "[0:0:0:0:0:0:0:1]:8787"
	recorder = httptest.NewRecorder()
	ipv6Handler.ServeHTTP(recorder, ipv6Request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("equivalent IPv6 host status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}
func selectedMinimizationSummary() minimize.MinimizationSummary {
	return minimize.MinimizationSummary{
		SchemaVersion:          minimize.SummarySchemaVersion,
		PlanName:               "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: minimize.FunctionalityCriterionAllNonDisclosureFields,
		PairsPerOrder:          2,
		EvidenceState:          evidence.Observed,
		SelectionState:         minimize.SelectionSelected,
		SelectedCandidate:      "city",
		CandidateResults: []minimize.CandidateResult{
			{
				ID:             "city",
				ManifestName:   "android-location-minimize-city",
				Directory:      "candidate-001-city",
				Classification: minimize.CandidateSufficient,
				Outcome:        bundle.NoChangeObserved,
				EvidenceState:  evidence.Observed,
				ReceiptSHA256:  strings.Repeat("a", 64),
				Pairs:          4,
				PairsPerOrder:  2,
				CompletedPairs: 4,
				ChangedPairs:   0,
				NoChangePairs:  4,
				UnknownPairs:   0,
			},
			{
				ID:             "omitted",
				ManifestName:   "android-location-minimize-omitted",
				Directory:      "candidate-002-omitted",
				Classification: minimize.CandidateInsufficient,
				Outcome:        bundle.ReplicatedChange,
				EvidenceState:  evidence.Observed,
				ReceiptSHA256:  strings.Repeat("b", 64),
				Pairs:          4,
				PairsPerOrder:  2,
				CompletedPairs: 4,
				ChangedPairs:   4,
				NoChangePairs:  0,
				UnknownPairs:   0,
			},
		},
	}
}
