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
			return bundle.Summary{ManifestName: "<manifest>", Differences: 1}, nil
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
		{name: "index", path: "/", want: []string{"Archived bundles", "run-001", "&lt;manifest&gt;"}},
		{name: "run", path: "/run?directory=run-001", want: []string{"Ask a bounded question", "Did it change?"}},
		{name: "ask", path: "/ask?directory=run-001&question_id=counterfactual-change", want: []string{"Question result", "observed", findingID}},
		{name: "finding", path: "/finding?directory=run-001&finding_id=" + findingID, want: []string{"Finding detail", "network.body", "baseline/network.json", "&lt;safe-source&gt;"}},
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
