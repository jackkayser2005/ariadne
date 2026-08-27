package ui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestHandlerRendersSourceNeutralLadderMinimization(t *testing.T) {
	summary := selectedBrowserLadderSummary()
	if err := summary.Validate(); err != nil {
		t.Fatal(err)
	}
	rootPath := "C:\\private\\browser-ladder-secret"
	rootDigest := strings.Repeat("d", 64)
	h := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:      t.TempDir(),
		MinimizationPath: rootPath,
		MinimizationLadderVerify: func(path string) (minimize.LadderSummary, string, error) {
			if path != rootPath {
				t.Fatalf("ladder verifier path = %q", path)
			}
			return summary, rootDigest, nil
		},
	})

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
		"browser-account-minimize",
		"account-id",
		"reference",
		"omitted",
		"all-non-disclosure-fields-equal-v1",
		"browser-local-fixture",
		"adapter version",
		"procedure SHA-256",
		"outbound",
		"fresh-ephemeral-profile-before-each-session",
		"Minimum tested sufficient disclosure",
		"Fixed minimization questions",
		"minimum-tested-selection",
		"minimization-support",
		"supported",
		"sufficient",
		"no-change-observed",
		"observed",
		rootDigest,
		strings.Repeat("a", 64),
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ladder minimization body missing %q: %s", want, body)
		}
	}

	for _, secret := range []string{
		"browser-ladder-secret",
		"candidate-001-omitted",
		"driver-argument",
		"https://",
		"account-value",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("ladder minimization page disclosed %q", secret)
		}
	}

	postRecorder := httptest.NewRecorder()
	h.ServeHTTP(postRecorder, httptest.NewRequest(http.MethodPost, "/minimization", nil))
	if postRecorder.Code != http.StatusMethodNotAllowed || postRecorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /minimization status = %d, allow=%q, body=%q", postRecorder.Code, postRecorder.Header().Get("Allow"), postRecorder.Body.String())
	}
}

func TestHandlerRendersVerifiedLadderQuestionArtifacts(t *testing.T) {
	summary := selectedBrowserLadderSummary()
	rootDigest := strings.Repeat("d", 64)
	artifactDir := t.TempDir()
	roundPath := filepath.Join(artifactDir, "round.json")
	if _, err := minimize.SaveLadderQuestionRound(summary, rootDigest, roundPath); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(artifactDir, "receipt.json")
	if _, err := minimize.SaveLadderQuestionReceipt(roundPath, minimize.MinimizationQuestionSelection, receiptPath); err != nil {
		t.Fatal(err)
	}
	h := HandlerWithReviewOptions(ReviewOptions{
		ArchiveRoot:             t.TempDir(),
		MinimizationPath:        "browser-ladder",
		MinimizationRoundPath:   roundPath,
		MinimizationReceiptPath: receiptPath,
		MinimizationLadderVerify: func(path string) (minimize.LadderSummary, string, error) {
			if path != "browser-ladder" {
				t.Fatalf("ladder verifier path = %q", path)
			}
			return summary, rootDigest, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("saved ladder artifact status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"saved and verified",
		"Selected question receipt",
		"minimum-tested-selection",
		rootDigest,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("saved ladder artifact body missing %q: %s", want, body)
		}
	}
	for _, secret := range []string{
		roundPath,
		receiptPath,
		"candidate-001-omitted",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("saved ladder artifact page disclosed %q", secret)
		}
	}
}
func TestHandlerHidesSourceNeutralLadderVerificationErrors(t *testing.T) {
	h := newHandler(handler{
		minimizationPath: "C:\\private\\browser-ladder-secret",
		minimizationLadderVerify: func(string) (minimize.LadderSummary, string, error) {
			return minimize.LadderSummary{}, "", errors.New("private path C:\\private\\browser-ladder-secret")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
		t.Fatalf("ladder verification error status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsLegacyQuestionArtifactsForSourceNeutralLadder(t *testing.T) {
	h := newHandler(handler{
		minimizationPath:      "minimization",
		minimizationRoundPath: "legacy-round",
		minimizationLadderVerify: func(string) (minimize.LadderSummary, string, error) {
			return selectedBrowserLadderSummary(), strings.Repeat("d", 64), nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/minimization", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "minimization unavailable\n" {
		t.Fatalf("source-neutral ladder with legacy artifacts status = %d, body=%q", recorder.Code, recorder.Body.String())
	}
}
func selectedBrowserLadderSummary() minimize.LadderSummary {
	return minimize.LadderSummary{
		SchemaVersion:          minimize.LadderSummarySchemaVersion,
		PlanName:               "browser-account-minimize",
		Variable:               "account-id",
		ReferenceCandidate:     "reference",
		FunctionalityCriterion: minimize.FunctionalityCriterionAllNonDisclosureFields,
		Adapter:                "browser-local-fixture",
		AdapterVersion:         2,
		ProcedureSHA256:        strings.Repeat("b", 64),
		Scope:                  "outbound",
		ResetPolicy:            "fresh-ephemeral-profile-before-each-session",
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         minimize.SelectionSelected,
		SelectedCandidate:      "omitted",
		CandidateResults: []minimize.LadderCandidateResult{
			{
				ID:             "omitted",
				Directory:      "candidate-001-omitted",
				Classification: minimize.CandidateSufficient,
				Outcome:        bundle.NoChangeObserved,
				EvidenceState:  evidence.Observed,
				ReceiptSHA256:  strings.Repeat("a", 64),
				Pairs:          2,
				PairsPerOrder:  1,
				CompletedPairs: 2,
				ChangedPairs:   0,
				NoChangePairs:  2,
				UnknownPairs:   0,
			},
		},
	}
}
