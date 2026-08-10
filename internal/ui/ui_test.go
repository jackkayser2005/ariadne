package ui

import (
	"errors"
	"html/template"
	"io"
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

func TestHandlerRendersPortableExport(t *testing.T) {
	findingID := "sha256:" + strings.Repeat("a", 64)
	sourceDigest := strings.Repeat("b", 64)
	exportDigest := strings.Repeat("c", 64)
	question := "Did changing the declared variable influence an observed output?"
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: question}}
		},
		exportPath: "portable-export.json",
		exportAsk: func(path, questionID string) (bundle.Answer, error) {
			if path != "portable-export.json" || questionID != "counterfactual-change" {
				t.Fatalf("AskExport() args = %q, %q", path, questionID)
			}
			return bundle.Answer{
				QuestionID:           questionID,
				Question:             question,
				State:                "observed",
				FindingIDs:           []string{findingID},
				SourceEvidenceSHA256: sourceDigest,
				ExportSHA256:         exportDigest,
			}, nil
		},
		exportFind: func(path, id string) (bundle.Finding, error) {
			if path != "portable-export.json" || id != findingID {
				t.Fatalf("FindExport() args = %q, %q", path, id)
			}
			return bundle.Finding{
				Question:             question,
				AnswerState:          "observed",
				Kind:                 "difference",
				Classification:       "changed",
				ID:                   id,
				Field:                "variant",
				State:                "observed",
				Evidence:             []string{"baseline/observations/storage.json#/variant"},
				SourceEvidenceSHA256: sourceDigest,
				ExportSHA256:         exportDigest,
			}, nil
		},
	})

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "index", path: "/", want: []string{"Portable redacted export", "Ask the portable export"}},
		{name: "question", path: "/export-ask?question_id=counterfactual-change", want: []string{"Question result", question, "Portable export identity", sourceDigest, exportDigest, findingID, "/export-finding"}},
		{name: "finding", path: "/export-finding?finding_id=" + findingID, want: []string{"Finding detail", "portable export", "variant", "baseline/observations/storage.json#/variant", "Portable export identity", sourceDigest, exportDigest}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			body := recorder.Body.String()
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%q", recorder.Code, body)
			}
			for _, want := range test.want {
				if !strings.Contains(body, want) {
					t.Fatalf("body missing %q: %s", want, body)
				}
			}
			for _, secret := range []string{"secret-value", "portable-export.json"} {
				if strings.Contains(body, secret) {
					t.Fatalf("body disclosed %q", secret)
				}
			}
		})
	}
}

func TestHandlerHidesPortableExportErrors(t *testing.T) {
	h := newHandler(handler{
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: "counterfactual-change", Text: "private question"}}
		},
		exportAsk: func(string, string) (bundle.Answer, error) {
			return bundle.Answer{}, errors.New("private export question failure")
		},
		exportFind: func(string, string) (bundle.Finding, error) {
			return bundle.Finding{}, errors.New("private export finding failure")
		},
	})
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/export-ask?question_id=counterfactual-change", want: "export question unavailable"},
		{path: "/export-finding?finding_id=sha256:" + strings.Repeat("a", 64), want: "export finding unavailable"},
	} {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			body := recorder.Body.String()
			if recorder.Code != http.StatusUnprocessableEntity || body != test.want+"\n" {
				t.Fatalf("status = %d, body=%q", recorder.Code, body)
			}
			if strings.Contains(body, "private") {
				t.Fatal("body disclosed internal error")
			}
		})
	}
}

func TestHandlerWithReviewAndExportConfiguresExport(t *testing.T) {
	h := HandlerWithReviewAndExport(t.TempDir(), "", "", "portable-export.json")
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Portable redacted export") {
		t.Fatalf("status = %d, body=%q", recorder.Code, recorder.Body.String())
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

func TestHandlerRendersReflectionHistory(t *testing.T) {
	history := bundle.ArchiveQuestionTransitionHistory{
		SchemaVersion:   3,
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		QuestionID:      "counterfactual-change",
		Question:        "Did changing the declared variable influence an observed output?",
		OrderBasis:      "caller",
		Snapshots:       3,
		SnapshotSummaries: []bundle.ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("c", 64), Observed: 1, Unavailable: 1, Checked: 2},
		},
		Transitions: []bundle.ArchiveQuestionTransition{
			{
				FromReflectionSHA256: strings.Repeat("a", 64),
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              1,
				StateChanges: []bundle.ArchiveQuestionStateChange{{
					Directory:  "run-001",
					OlderState: "observed",
					NewerState: "unknown",
				}},
			},
			{
				FromReflectionSHA256: strings.Repeat("b", 64),
				ToReflectionSHA256:   strings.Repeat("c", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              1,
				StateChanges: []bundle.ArchiveQuestionStateChange{{
					Directory:  "run-001",
					OlderState: "unknown",
					NewerState: "unavailable",
				}},
			},
		},
	}
	summary := bundle.ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           3,
		HistoryID:               history.HistoryID,
		HistoryQuestion:         history.HistoryQuestion,
		QuestionID:              history.QuestionID,
		OrderBasis:              history.OrderBasis,
		Snapshots:               history.Snapshots,
		Transitions:             len(history.Transitions),
		TransitionHistorySHA256: strings.Repeat("c", 64),
	}
	questionRound := bundle.AnswerArchiveQuestionTransitionHistoryQuestionRound(history, summary.TransitionHistorySHA256)
	questionRoundSHA256, err := bundle.ArchiveQuestionTransitionHistoryQuestionRoundSHA256(questionRound)
	if err != nil {
		t.Fatal(err)
	}
	selectedReceipt, err := bundle.AnswerArchiveQuestionTransitionHistoryReceipt(history, summary.TransitionHistorySHA256, "answer-state-repeated-changes")
	if err != nil {
		t.Fatal(err)
	}
	selectedReceiptSHA256, err := bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSHA256(selectedReceipt)
	if err != nil {
		t.Fatal(err)
	}
	acceptanceRecord := bundle.ArchiveQuestionTransitionHistoryAcceptanceRecord{
		SchemaVersion:           1,
		TransitionHistorySHA256: summary.TransitionHistorySHA256,
		QuestionRoundSHA256:     questionRoundSHA256,
		QuestionID:              selectedReceipt.QuestionID,
		ReceiptSHA256:           selectedReceiptSHA256,
	}
	acceptanceSHA256, err := bundle.ArchiveQuestionTransitionHistoryAcceptanceRecordSHA256(acceptanceRecord)
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		questions: func() []bundle.Question {
			return []bundle.Question{{ID: history.QuestionID, Text: history.Question}}
		},
		history: func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return history, summary, nil
		},
		acceptance: func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{
				SchemaVersion:           acceptanceRecord.SchemaVersion,
				TransitionHistorySHA256: acceptanceRecord.TransitionHistorySHA256,
				QuestionRoundSHA256:     acceptanceRecord.QuestionRoundSHA256,
				QuestionID:              acceptanceRecord.QuestionID,
				ReceiptSHA256:           acceptanceRecord.ReceiptSHA256,
				AcceptanceSHA256:        acceptanceSHA256,
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{"Saved reflection history", "verified ledger", history.Question, "Question round", "fixed, verified, read only", "aria-label=\"Verified history question round\"", "answer-state-transitions", "answer-state-repeated-changes", "answer-state-snapshot-summaries", "answer-state-summary-changes", "history_question_id=answer-state-transitions", "history_question_id=answer-state-repeated-changes", "history_question_id=answer-state-snapshot-summaries", "history_question_id=answer-state-summary-changes", "caller", "snapshots", "3", "transitions", "2", "history SHA-256", "question round SHA-256", questionRoundSHA256, strings.Repeat("c", 64), "safe snapshot summaries", "checked 1", "checked 2", "History question", "changed", "changed transitions", "transition 1", "changed entries", "transition 1: run-001", "incomparable transitions", "Repeated-change question", "repeated", "Snapshot-summary question", "Snapshot-change question", "Did the bounded answer-state summary change at any supplied boundary?", "available", "transition 2", strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), "changed archive entries", "run-001", "observed", "unknown", "does not establish chronology"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, want := range []string{"history-acceptance-record", "Portable question acceptance", "select bound question", "acceptance SHA-256", acceptanceSHA256} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing acceptance binding %q: %s", want, body)
		}
	}
	if strings.Contains(body, "secret-value") {
		t.Fatal("body disclosed a raw value")
	}

	selectedRecorder := httptest.NewRecorder()
	h.ServeHTTP(selectedRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-repeated-changes", nil))
	selectedBody := selectedRecorder.Body.String()
	if selectedRecorder.Code != http.StatusOK || !strings.Contains(selectedBody, `aria-current="page"`) || !strings.Contains(selectedBody, `aria-label="Ask verified history question answer-state-repeated-changes"`) || !strings.Contains(selectedBody, `id="history-answer-receipt-answer-state-repeated-changes"`) || !strings.Contains(selectedBody, `aria-label="Portable history answer receipt"`) || !strings.Contains(selectedBody, `aria-label="Portable history answer receipt JSON"`) || !strings.Contains(selectedBody, "Portable answer receipt") || !strings.Contains(selectedBody, "receipt SHA-256") || !strings.Contains(selectedBody, selectedReceiptSHA256) || !strings.Contains(selectedBody, "raw-value-free") || !strings.Contains(selectedBody, "matched") || !strings.Contains(selectedBody, `id="history-question-answer-state-repeated-changes"`) || strings.Contains(selectedBody, `id="history-answer-receipt-answer-state-transitions"`) || strings.Contains(selectedBody, `id="history-question-answer-state-transitions"`) {
		t.Fatalf("selected history question status = %d, body=%q", selectedRecorder.Code, selectedBody)
	}

	snapshotRecorder := httptest.NewRecorder()
	h.ServeHTTP(snapshotRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-snapshot-summaries", nil))
	snapshotBody := snapshotRecorder.Body.String()
	if snapshotRecorder.Code != http.StatusOK || !strings.Contains(snapshotBody, `aria-current="page"`) || !strings.Contains(snapshotBody, `id="history-question-answer-state-snapshot-summaries"`) || !strings.Contains(snapshotBody, "mismatch") || strings.Contains(snapshotBody, `id="history-question-answer-state-transitions"`) {
		t.Fatalf("selected snapshot question status = %d, body=%q", snapshotRecorder.Code, snapshotBody)
	}

	summaryRecorder := httptest.NewRecorder()
	h.ServeHTTP(summaryRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-summary-changes", nil))
	summaryBody := summaryRecorder.Body.String()
	if summaryRecorder.Code != http.StatusOK || !strings.Contains(summaryBody, `aria-current="page"`) || !strings.Contains(summaryBody, `id="history-question-answer-state-summary-changes"`) || strings.Contains(summaryBody, `id="history-question-answer-state-transitions"`) {
		t.Fatalf("selected summary question status = %d, body=%q", summaryRecorder.Code, summaryBody)
	}

	invalidRecorder := httptest.NewRecorder()
	h.ServeHTTP(invalidRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=not-a-question", nil))
	if invalidRecorder.Code != http.StatusNotFound || !strings.Contains(invalidRecorder.Body.String(), "history question not found") {
		t.Fatalf("invalid history question status = %d, body=%q", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	server := httptest.NewServer(h)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/?history_question_id=answer-state-summary-changes")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	rendered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/html") || !strings.Contains(string(rendered), questionRoundSHA256) || !strings.Contains(string(rendered), "history-answer-receipt-answer-state-summary-changes") {
		t.Fatalf("rendered history flow status = %d, content-type=%q, body=%q", response.StatusCode, response.Header.Get("Content-Type"), rendered)
	}
}

func TestHandlerHidesReflectionHistoryErrors(t *testing.T) {
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		history: func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistory{}, bundle.ArchiveQuestionTransitionVerificationSummary{}, errors.New("private history failure")
		},
		acceptance: func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{
				SchemaVersion:           1,
				TransitionHistorySHA256: strings.Repeat("a", 64),
				QuestionRoundSHA256:     strings.Repeat("b", 64),
				QuestionID:              "answer-state-transitions",
				ReceiptSHA256:           strings.Repeat("c", 64),
				AcceptanceSHA256:        strings.Repeat("d", 64),
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-transitions", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "Saved reflection history is unavailable") {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private history failure") {
		t.Fatal("body disclosed reflection history error")
	}
	if !strings.Contains(body, "Portable question acceptance") || !strings.Contains(body, "history unavailable") {
		t.Fatalf("acceptance status missing from failed-history page: %s", body)
	}

	withoutHistory := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
	})
	withoutHistoryRecorder := httptest.NewRecorder()
	withoutHistory.ServeHTTP(withoutHistoryRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-transitions", nil))
	if withoutHistoryRecorder.Code != http.StatusOK || strings.Contains(withoutHistoryRecorder.Body.String(), "history question not found") {
		t.Fatalf("known history question without source status = %d, body=%q", withoutHistoryRecorder.Code, withoutHistoryRecorder.Body.String())
	}
}

func TestHandlerHidesAcceptanceErrors(t *testing.T) {
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		history: func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistory{}, bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
		},
		acceptance: func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("private acceptance failure")
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "Portable question acceptance") || !strings.Contains(body, "unavailable") {
		t.Fatalf("acceptance error status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private acceptance failure") {
		t.Fatal("body disclosed acceptance verification error")
	}
}

func TestHandlerRendersQuestionRoundComparison(t *testing.T) {
	comparison := bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{
		SchemaVersion:                 1,
		ComparisonID:                  "question-round-answer-change",
		ComparisonQuestion:            "Did the bounded answer results change between these retained question rounds?",
		OrderBasis:                    "caller",
		Result:                        "changed",
		FirstRoundSHA256:              strings.Repeat("a", 64),
		SecondRoundSHA256:             strings.Repeat("b", 64),
		FirstTransitionHistorySHA256:  strings.Repeat("c", 64),
		SecondTransitionHistorySHA256: strings.Repeat("d", 64),
		Compared:                      4,
		Changed:                       1,
		ChangedQuestions: []bundle.ArchiveQuestionTransitionHistoryQuestionRoundChange{{
			QuestionID:   "answer-state-transitions",
			FirstResult:  "same",
			SecondResult: "changed",
		}},
	}
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		history: func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistory{}, bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
		},
		compareRounds: func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return comparison, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"history-question-round-comparison",
		"Retained question rounds",
		comparison.ComparisonQuestion,
		comparison.Result,
		comparison.OrderBasis,
		comparison.FirstRoundSHA256,
		comparison.SecondRoundSHA256,
		comparison.FirstTransitionHistorySHA256,
		comparison.SecondTransitionHistorySHA256,
		"answer-state-transitions",
		"Ask changed retained question answer-state-transitions",
		"history_question_id=answer-state-transitions",
		"same",
		"changed",
		"does not establish chronology",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	selectedRecorder := httptest.NewRecorder()
	h.ServeHTTP(selectedRecorder, httptest.NewRequest(http.MethodGet, "/?history_question_id=answer-state-transitions", nil))
	if selectedRecorder.Code != http.StatusOK {
		t.Fatalf("changed question route status = %d, body=%q", selectedRecorder.Code, selectedRecorder.Body.String())
	}
	withoutHistory := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		compareRounds: func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return comparison, nil
		},
	})
	withoutHistoryRecorder := httptest.NewRecorder()
	withoutHistory.ServeHTTP(withoutHistoryRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if withoutHistoryRecorder.Code != http.StatusOK || strings.Contains(withoutHistoryRecorder.Body.String(), "Ask changed retained question") {
		t.Fatalf("comparison without history status = %d, body=%q", withoutHistoryRecorder.Code, withoutHistoryRecorder.Body.String())
	}
}

func TestHandlerHidesQuestionRoundComparisonErrors(t *testing.T) {
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		compareRounds: func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, errors.New("private round comparison failure")
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "Retained question round comparison is unavailable") {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private round comparison failure") {
		t.Fatal("body disclosed round comparison error")
	}
}

func TestHandlerRendersSavedReflectionComparison(t *testing.T) {
	comparison := bundle.ArchiveQuestionComparison{
		ComparisonQuestion:    "Did the bounded answer state change between these saved reflection snapshots?",
		QuestionID:            "counterfactual-change",
		Question:              "Did changing the declared variable influence an observed output?",
		Result:                "changed",
		OlderReflectionSHA256: strings.Repeat("a", 64),
		NewerReflectionSHA256: strings.Repeat("b", 64),
		Compared:              3,
		Changed:               1,
		OlderOnly:             0,
		NewerOnly:             0,
		StateChanges: []bundle.ArchiveQuestionStateChange{{
			Directory:  "run-001",
			OlderState: "observed",
			NewerState: "unknown",
		}},
	}
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		compareCurrent: func() (bundle.ArchiveQuestionComparison, error) {
			return comparison, nil
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"Saved reflection vs current",
		"bounded comparison",
		comparison.Question,
		comparison.ComparisonQuestion,
		comparison.Result,
		comparison.OlderReflectionSHA256,
		comparison.NewerReflectionSHA256,
		"compared",
		"changed",
		"does not establish chronology", "Changed archive entries", "run-001", "observed", "unknown",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestHandlerHidesSavedReflectionComparisonErrors(t *testing.T) {
	h := newHandler(handler{
		root:  "archive-root",
		index: func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		compareCurrent: func() (bundle.ArchiveQuestionComparison, error) {
			return bundle.ArchiveQuestionComparison{}, errors.New("private comparison failure")
		},
	})

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "Saved reflection comparison is unavailable") {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	if strings.Contains(body, "private comparison failure") {
		t.Fatal("body disclosed saved reflection comparison error")
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
