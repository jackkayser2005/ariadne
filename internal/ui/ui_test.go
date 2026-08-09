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
				Normalizations:           []string{"<normalization>", "second normalization"},
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
		{name: "archive question lens", path: "/?question_id=counterfactual-change", want: []string{"Question lens", "oldest first", "Dated results are ordered", "Archive question summary", "Did it change?", "observed", "unknown", "unavailable", "checked", "Verified provenance", "recorded (UTC)", "2026-07-25T12:00:00Z", "target package", "&lt;package&gt;", "Android API", "35", "architecture", "&lt;architecture&gt;", "package version", "7", "package SHA-256", strings.Repeat("d", 64), "normalization", "&lt;normalization&gt;", "second normalization", strings.Repeat("c", 64), "&lt;revision&gt;", "clean", "Open answer details", "run-001", "&lt;manifest&gt;"}},
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

func TestHandlerRendersCurrentReflectionIdentity(t *testing.T) {
	report := bundle.ArchiveQuestionReport{
		SchemaVersion: 2,
		QuestionID:    "counterfactual-change",
		Question:      "Did changing the declared variable influence an observed output?",
		Summary: bundle.ArchiveQuestionSummary{
			Observed: 1,
			Checked:  1,
		},
		Results: []bundle.ArchiveQuestionResult{{
			Directory:    "run-001",
			ManifestName: "current",
			RecordedAt:   "2026-07-25T12:00:00Z",
			Provenance: &bundle.ArchiveQuestionProvenance{
				ManifestContractSHA256: strings.Repeat("c", 64),
				SourceEvidenceSHA256:   strings.Repeat("d", 64),
				AriadneRevision:        strings.Repeat("a", 40),
			},
			Answer: &bundle.Answer{
				QuestionID: "counterfactual-change",
				Question:   "Did changing the declared variable influence an observed output?",
				State:      "observed",
				FindingIDs: []string{},
			},
			Available: true,
		}},
	}
	digest, err := bundle.ArchiveQuestionReportReflectionSHA256(report)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return []bundle.ArchiveEntry{{Directory: "run-001", ManifestName: "current"}}, nil
		},
		verify: func(string) (bundle.Summary, error) {
			return bundle.Summary{RecordedAt: "2026-07-25T12:00:00Z"}, nil
		},
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "Did changing the declared variable influence an observed output?"}}
		},
		ask: func(string, string) (bundle.Answer, error) {
			return *report.Results[0].Answer, nil
		},
		askArchive: func(root, questionID string) (bundle.ArchiveQuestionReport, error) {
			if root != "archive-root" || questionID != "counterfactual-change" {
				t.Fatalf("AskArchive() args = %q, %q", root, questionID)
			}
			return report, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?question_id=counterfactual-change", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{"Current reflection", "derived in memory", "reflection SHA-256", digest, "bundles checked", "raw-value-free"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestHandlerHidesCurrentReflectionErrors(t *testing.T) {
	h := newHandler(handler{
		root: "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) {
			return nil, nil
		},
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "Did it change?"}}
		},
		askArchive: func(string, string) (bundle.ArchiveQuestionReport, error) {
			return bundle.ArchiveQuestionReport{}, errors.New("private archive reflection failure")
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?question_id=counterfactual-change", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "current reflection could not be derived") {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private archive reflection failure") {
		t.Fatal("body disclosed current reflection error")
	}
}

func TestSummarizeArchiveAnswers(t *testing.T) {
	summary := summarizeArchiveAnswers([]archiveQuestionResult{
		{Available: true, Answer: bundle.Answer{State: "observed"}},
		{Available: true, Answer: bundle.Answer{State: "unknown"}},
		{Available: false},
	})
	want := archiveQuestionSummary{Total: 3, Observed: 1, Unknown: 1, Unavailable: 1}
	if summary != want {
		t.Fatalf("summarizeArchiveAnswers() = %#v, want %#v", summary, want)
	}
}

func TestSortArchiveQuestionResults(t *testing.T) {
	results := []archiveQuestionResult{
		{Directory: "run-z", Summary: bundle.Summary{RecordedAt: "2026-07-25T12:00:00Z"}},
		{Directory: "legacy", Summary: bundle.Summary{}},
		{Directory: "run-old", Summary: bundle.Summary{RecordedAt: "2026-07-24T12:00:00Z"}},
		{Directory: "run-fraction", Summary: bundle.Summary{RecordedAt: "2026-07-25T12:00:00.1Z"}},
		{Directory: "run-a", Summary: bundle.Summary{RecordedAt: "2026-07-25T12:00:00Z"}},
	}
	sortArchiveQuestionResults(results)
	want := []string{"run-old", "run-a", "run-z", "run-fraction", "legacy"}
	for index, result := range results {
		if result.Directory != want[index] {
			t.Fatalf("sortArchiveQuestionResults() = %#v, want directories %#v", results, want)
		}
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
