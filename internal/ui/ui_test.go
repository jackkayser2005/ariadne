package ui

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

func TestHandlerRendersReadOnlyReview(t *testing.T) {
	findingID := "sha256:" + strings.Repeat("a", 64)
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{
				Directory:    "run-001",
				ManifestName: "<manifest>",
				Differences:  1,
				Unknowns:     0,
			}}, nil
		},
		verify: func(runDir string) (bundle.Summary, error) {
			if !strings.HasSuffix(runDir, "run-001") {
				t.Fatalf("Verify() run directory = %q", runDir)
			}
			return bundle.Summary{
				ManifestName:             "<manifest>",
				Differences:              1,
				Question:                 "Did it change?",
				AnswerState:              "observed",
				ManifestContractSHA256:   strings.Repeat("c", 64),
				AriadneRevision:          "<revision>",
				RecordedAt:               "2026-07-25T12:00:00Z",
				TargetPackage:            "<package>",
				TargetAndroidAPI:         35,
				TargetArchitecture:       "<architecture>",
				TargetPackageVersionCode: 7,
				TargetPackageSHA256:      strings.Repeat("d", 64),
			}, nil
		},
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "Did it change?"}}
		},
		ask: func(runDir, questionID string) (bundle.Answer, error) {
			return bundle.Answer{
				QuestionID: questionID,
				Question:   "Did it change?",
				State:      "observed",
				FindingIDs: []string{findingID},
			}, nil
		},
		find: func(string, string) (bundle.Finding, error) {
			return bundle.Finding{
				Kind:           "difference",
				Classification: "changed",
				Field:          "network.body",
				AnswerState:    "observed",
				State:          "observed",
				Evidence:       []string{"baseline/network.json", "<safe-source>"},
			}, nil
		},
	})

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "index", path: "/", want: []string{"Archived bundles", "Ask across this archive", "Did it change?", "run-001", "&lt;manifest&gt;"}},
		{name: "archive question lens", path: "/?question_id=counterfactual-change", want: []string{"Question lens", "Did it change?", "observed", "Verified provenance", "recorded (UTC)", "2026-07-25T12:00:00Z", "target package", "&lt;package&gt;", "Android API", "35", "architecture", "&lt;architecture&gt;", "package version", "7", "package SHA-256", strings.Repeat("d", 64), strings.Repeat("c", 64), "&lt;revision&gt;", "clean", "Open answer details", "run-001", "&lt;manifest&gt;"}},
		{name: "run", path: "/run?directory=run-001", want: []string{"Verified provenance", "recorded (UTC)", "2026-07-25T12:00:00Z", "target package", "&lt;package&gt;", "Bounded question board", "Did it change?", "Open answer details", findingID, strings.Repeat("c", 64), "&lt;revision&gt;", "clean"}},
		{name: "ask", path: "/ask?directory=run-001&question_id=counterfactual-change", want: []string{"Question result", "observed", findingID, "Verified provenance", "recorded (UTC)", "2026-07-25T12:00:00Z", "target package", "&lt;package&gt;", strings.Repeat("d", 64), strings.Repeat("c", 64), "&lt;revision&gt;", "clean"}},
		{name: "finding", path: "/finding?directory=run-001&finding_id=" + findingID, want: []string{"Finding detail", "network.body", "baseline/network.json", "&lt;safe-source&gt;", "Verified provenance", "target package", "&lt;package&gt;", "Android API", "35", strings.Repeat("d", 64), "recorded (UTC)", "2026-07-25T12:00:00Z", strings.Repeat("c", 64), "&lt;revision&gt;", "clean"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			for _, want := range test.want {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body missing %q: %s", want, recorder.Body.String())
				}
			}
			if strings.Contains(recorder.Body.String(), "secret-value") {
				t.Fatal("body disclosed a raw value")
			}
			if strings.Contains(recorder.Body.String(), "%253A") {
				t.Fatal("finding ID was encoded twice")
			}
		})
	}
}

func TestHandlerMarksUnavailableArchiveQuestion(t *testing.T) {
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{
				{Directory: "run-001", ManifestName: "current"},
				{Directory: "legacy-run", ManifestName: "legacy"},
			}, nil
		},
		verify: func(runDir string) (bundle.Summary, error) {
			if strings.HasSuffix(runDir, "legacy-run") {
				return bundle.Summary{}, errors.New("private legacy verify failure")
			}
			return bundle.Summary{
				ManifestContractSHA256: strings.Repeat("c", 64),
				AriadneRevision:        "revision",
			}, nil
		},
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "Did it change?"}}
		},
		ask: func(runDir, questionID string) (bundle.Answer, error) {
			return bundle.Answer{QuestionID: questionID, State: "observed"}, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?question_id=counterfactual-change", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "unavailable") || !strings.Contains(body, "legacy-run") || !strings.Contains(body, "current") {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private legacy verify failure") {
		t.Fatal("body disclosed internal question error")
	}
}

func TestHandlerRendersUnknownReason(t *testing.T) {
	const reason = "treatment storage observation was not captured"
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{Directory: "run-001"}}, nil
		},
		verify: func(string) (bundle.Summary, error) {
			return bundle.Summary{
				Question:               "<question>",
				AnswerState:            "observed",
				ManifestContractSHA256: strings.Repeat("c", 64),
				AriadneRevision:        "<revision>",
			}, nil
		},
		ask: func(string, string) (bundle.Answer, error) {
			return bundle.Answer{
				Question: "Were all required observations captured for both sessions?",
				State:    "unknown",
				Reason:   reason,
			}, nil
		},
		find: func(string, string) (bundle.Finding, error) {
			return bundle.Finding{
				Kind:        "unknown",
				AnswerState: "unknown",
				State:       "unknown",
				Reason:      reason,
			}, nil
		},
	})

	for _, path := range []string{
		"/ask?directory=run-001&question_id=capture-complete",
		"/finding?directory=run-001&finding_id=sha256:" + strings.Repeat("a", 64),
	} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), reason) {
			t.Fatalf("path %q: status = %d, body=%q", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandlerOmitsEmptyProvenance(t *testing.T) {
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{Directory: "legacy-run"}}, nil
		},
		verify:    func(string) (bundle.Summary, error) { return bundle.Summary{ManifestName: "legacy"}, nil },
		questions: func() []bundle.Question { return nil },
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/run?directory=legacy-run", nil))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "Verified provenance") {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerFailsClosedForUnavailableQuestions(t *testing.T) {
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{Directory: "legacy-run"}}, nil
		},
		verify: func(string) (bundle.Summary, error) { return bundle.Summary{ManifestName: "legacy"}, nil },
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "private question"}}
		},
		ask: func(string, string) (bundle.Answer, error) {
			return bundle.Answer{}, errors.New("private question failure")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/run?directory=legacy-run", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Bounded questions unavailable") || strings.Contains(recorder.Body.String(), "private") {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsUnsafeRequests(t *testing.T) {
	base := handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{Directory: "run-001"}}, nil
		},
		verify:    func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		questions: func() []bundle.Question { return nil },
		ask: func(string, string) (bundle.Answer, error) {
			return bundle.Answer{}, errors.New("private question failure")
		},
		find: func(string, string) (bundle.Finding, error) {
			return bundle.Finding{}, errors.New("private finding failure")
		},
	}

	t.Run("method", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
			t.Fatalf("status = %d, allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})

	t.Run("favicon", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
		if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("wrong index path", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/other", nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("invalid archive question", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?question_id=private-question", nil))
		if recorder.Code != http.StatusNotFound || recorder.Body.String() != "question not found\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("verification error", func(t *testing.T) {
		broken := base
		broken.verify = func(string) (bundle.Summary, error) { return bundle.Summary{}, errors.New("private verify failure") }
		recorder := httptest.NewRecorder()
		newHandler(broken).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/run?directory=run-001", nil))
		if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "bundle is no longer verifiable\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("missing question", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ask?directory=run-001", nil))
		if recorder.Code != http.StatusNotFound || recorder.Body.String() != "question not found\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("missing finding", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/finding?directory=run-001", nil))
		if recorder.Code != http.StatusNotFound || recorder.Body.String() != "finding not found\n" {
			t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
		}
	})

	for _, test := range []struct {
		name string
		path string
		code int
		want string
	}{
		{name: "unknown bundle", path: "/run?directory=missing", code: http.StatusNotFound, want: "bundle not found"},
		{name: "question error", path: "/ask?directory=run-001&question_id=counterfactual-change", code: http.StatusUnprocessableEntity, want: "question unavailable"},
		{name: "finding error", path: "/finding?directory=run-001&finding_id=bad", code: http.StatusUnprocessableEntity, want: "finding unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			newHandler(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.code || !strings.Contains(recorder.Body.String(), test.want) {
				t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private") {
				t.Fatal("body disclosed internal error")
			}
		})
	}

	for _, path := range []string{
		"/ask?directory=run-001&question_id=counterfactual-change",
		"/finding?directory=run-001&finding_id=sha256:" + strings.Repeat("a", 64),
	} {
		t.Run("deep verification error "+path, func(t *testing.T) {
			broken := base
			broken.verify = func(string) (bundle.Summary, error) { return bundle.Summary{}, errors.New("private verify failure") }
			recorder := httptest.NewRecorder()
			newHandler(broken).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "bundle is no longer verifiable\n" {
				t.Fatalf("path %q: status = %d, body=%q", path, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRenderFailure(t *testing.T) {
	original := pageTemplate
	pageTemplate = template.Must(template.New("page").Parse(`{{template "missing" .}}`))
	defer func() { pageTemplate = original }()

	recorder := httptest.NewRecorder()
	render(recorder, pageData{Title: "test"})
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "page unavailable\n" {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerEmptyArchive(t *testing.T) {
	h := Handler(t.TempDir())
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "No verified bundles") {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerHidesArchiveErrors(t *testing.T) {
	h := newHandler(handler{
		root:  "private-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, errors.New("private filesystem path") },
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError || recorder.Body.String() != "archive unavailable\n" {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}
