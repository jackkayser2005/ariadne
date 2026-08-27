package minimize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestAskMinimizationQuestionsFromVerifiedRunAndRejectMalformedArtifacts(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "candidate-001-city")
	makeValidChildReplication(t, childDir)
	child, err := bundle.VerifyReplicated(childDir)
	if err != nil {
		t.Fatal(err)
	}
	summary := MinimizationSummary{
		SchemaVersion:          SummarySchemaVersion,
		PlanName:               "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         SelectionSelected,
		SelectedCandidate:      "city",
		CandidateResults:       []CandidateResult{candidateResult("city", "candidate-001-city", child)},
	}
	if err := Save(root, summary); err != nil {
		t.Fatal(err)
	}
	answer, err := AskMinimizationQuestion(root, MinimizationQuestionSelection)
	if err != nil || answer.Result != string(SelectionSelected) {
		t.Fatalf("AskMinimizationQuestion() = %#v, %v", answer, err)
	}
	answers, err := AskAllMinimizationQuestions(root)
	if err != nil || len(answers) != len(MinimizationQuestions()) {
		t.Fatalf("AskAllMinimizationQuestions() = %#v, %v", answers, err)
	}

	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveMinimizationQuestionRound(root, roundPath); err != nil {
		t.Fatal(err)
	}
	malformedRound := filepath.Join(t.TempDir(), "malformed-round.json")
	if err := os.WriteFile(malformedRound, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMinimizationQuestionRound(malformedRound); err == nil {
		t.Fatal("ReadMinimizationQuestionRound() accepted malformed JSON")
	}

	malformedReceipt := filepath.Join(t.TempDir(), "malformed-receipt.json")
	if err := os.WriteFile(malformedReceipt, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMinimizationQuestionReceipt(malformedReceipt); err == nil {
		t.Fatal("ReadMinimizationQuestionReceipt() accepted malformed JSON")
	}
	if _, err := AskMinimizationQuestionReceipt(roundPath, MinimizationQuestionSelection); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveMinimizationQuestionReceipt(roundPath, MinimizationQuestionSelection, filepath.Join(t.TempDir(), "receipt.json")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(answer.Question, "private") {
		t.Fatal("answer contained a private value")
	}
}
