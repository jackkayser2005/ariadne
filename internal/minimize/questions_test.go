package minimize

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestMinimizationQuestionsKeepOutcomeAndEvidenceSeparate(t *testing.T) {
	tests := []struct {
		name            string
		city            CandidateClassification
		cityState       evidence.State
		omitted         CandidateClassification
		omittedState    evidence.State
		selection       SelectionState
		selected        string
		evidenceState   evidence.State
		supportResult   string
		supportEvidence evidence.State
	}{
		{
			name:            "selected",
			city:            CandidateSufficient,
			cityState:       evidence.Observed,
			omitted:         CandidateSufficient,
			omittedState:    evidence.Observed,
			selection:       SelectionSelected,
			selected:        "omitted",
			evidenceState:   evidence.Observed,
			supportResult:   "supported",
			supportEvidence: evidence.Observed,
		},
		{
			name:            "no sufficient candidate",
			city:            CandidateInsufficient,
			cityState:       evidence.Observed,
			omitted:         CandidateInsufficient,
			omittedState:    evidence.Observed,
			selection:       SelectionNoSufficient,
			evidenceState:   evidence.Observed,
			supportResult:   "supported",
			supportEvidence: evidence.Observed,
		},
		{
			name:            "mixed outcome with observed evidence",
			city:            CandidateMixedInconsistent,
			cityState:       evidence.Observed,
			omitted:         CandidateSufficient,
			omittedState:    evidence.Observed,
			selection:       SelectionUnknown,
			evidenceState:   evidence.Observed,
			supportResult:   "supported",
			supportEvidence: evidence.Observed,
		},
		{
			name:            "incomplete evidence",
			city:            CandidateUnknown,
			cityState:       evidence.Unknown,
			omitted:         CandidateSufficient,
			omittedState:    evidence.Observed,
			selection:       SelectionUnknown,
			evidenceState:   evidence.Unknown,
			supportResult:   "unknown",
			supportEvidence: evidence.Unknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary, err := summarize(testPlan(), 1, []CandidateResult{
				testResult("city", 0, test.city, test.cityState),
				testResult("omitted", 1, test.omitted, test.omittedState),
			})
			if err != nil {
				t.Fatalf("summarize() error = %v", err)
			}
			answers, err := AnswerAllMinimizationQuestions(summary, strings.Repeat("c", 64))
			if err != nil {
				t.Fatalf("AnswerAllMinimizationQuestions() error = %v", err)
			}
			if len(answers) != 2 {
				t.Fatalf("answers = %#v", answers)
			}
			selectionAnswer, supportAnswer := answers[0], answers[1]
			if selectionAnswer.Result != string(test.selection) || selectionAnswer.EvidenceState != test.evidenceState || selectionAnswer.SelectionState != test.selection || selectionAnswer.SelectedCandidate != test.selected {
				t.Fatalf("selection answer = %#v", selectionAnswer)
			}
			if supportAnswer.Result != test.supportResult || supportAnswer.EvidenceState != test.supportEvidence || supportAnswer.SelectionState != test.selection || supportAnswer.SelectedCandidate != test.selected {
				t.Fatalf("support answer = %#v", supportAnswer)
			}
			for _, answer := range answers {
				if answer.MinimizationSHA256 != strings.Repeat("c", 64) || answer.CandidateCount != 2 || answer.SupportedCandidates != 2 && test.supportResult == "supported" {
					t.Fatalf("answer identity or metrics = %#v", answer)
				}
			}
			if test.supportResult == "unknown" && supportAnswer.UnknownCandidates != 1 {
				t.Fatalf("unknown support count = %d", supportAnswer.UnknownCandidates)
			}
			if test.name == "mixed outcome with observed evidence" && selectionAnswer.EvidenceState != evidence.Observed {
				t.Fatal("mixed observed outcome lost its evidence state")
			}
		})
	}
}

func TestMinimizationQuestionsRejectInvalidArguments(t *testing.T) {
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerMinimizationQuestion(valid, strings.Repeat("a", 64), "not-a-question"); err == nil {
		t.Fatal("AnswerMinimizationQuestion() accepted an invalid question ID")
	}
	if _, err := AnswerMinimizationQuestion(MinimizationSummary{}, strings.Repeat("a", 64), MinimizationQuestionSelection); err == nil {
		t.Fatal("AnswerMinimizationQuestion() accepted an invalid summary")
	}
	if _, err := AnswerMinimizationQuestion(valid, "short", MinimizationQuestionSelection); err == nil {
		t.Fatal("AnswerMinimizationQuestion() accepted an invalid receipt identity")
	}
	if _, err := AnswerAllMinimizationQuestions(MinimizationSummary{}, strings.Repeat("a", 64)); err == nil {
		t.Fatal("AnswerAllMinimizationQuestions() accepted an invalid summary")
	}
	if _, err := AnswerAllMinimizationQuestions(valid, "short"); err == nil {
		t.Fatal("AnswerAllMinimizationQuestions() accepted an invalid receipt identity")
	}
	if _, err := AnswerMinimizationQuestionRound(MinimizationSummary{}, strings.Repeat("a", 64)); err == nil {
		t.Fatal("AnswerMinimizationQuestionRound() accepted an invalid summary")
	}
	if _, err := AnswerMinimizationQuestionRound(valid, "short"); err == nil {
		t.Fatal("AnswerMinimizationQuestionRound() accepted an invalid receipt identity")
	}
	if _, err := AskMinimizationQuestion(filepath.Join(t.TempDir(), "missing.json"), MinimizationQuestionSelection); err == nil {
		t.Fatal("AskMinimizationQuestion() accepted a missing minimization run")
	}
	if _, err := AskAllMinimizationQuestions(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("AskAllMinimizationQuestions() accepted a missing minimization run")
	}
	if _, ok := minimizationQuestion("not-a-question"); ok {
		t.Fatal("minimizationQuestion() accepted an invalid ID")
	}
	for _, question := range MinimizationQuestions() {
		if question.ID == "" || question.Text == "" {
			t.Fatalf("invalid question = %#v", question)
		}
	}
}

func TestMinimizationQuestionRoundIsSafeAndStable(t *testing.T) {
	_, round := validQuestionRound(t)
	digest, err := MinimizationQuestionRoundSHA256(round)
	if err != nil {
		t.Fatalf("MinimizationQuestionRoundSHA256() error = %v", err)
	}
	if !validDigest(digest) {
		t.Fatalf("round digest = %q", digest)
	}
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"37.7749-122.4194", "san-francisco", "baseline@example.invalid", "emulator-5554", "dev.ariadne.fixture"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("question round disclosed %q: %s", secret, data)
		}
	}
	decoded, err := DecodeMinimizationQuestionRound(append(data, '\n'))
	if err != nil {
		t.Fatalf("DecodeMinimizationQuestionRound() error = %v", err)
	}
	if decoded.MinimizationSHA256 != round.MinimizationSHA256 || len(decoded.Candidates) != len(round.Candidates) || len(decoded.Answers) != len(round.Answers) {
		t.Fatalf("decoded round = %#v", decoded)
	}
	verified, err := VerifyMinimizationQuestionRound(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || verified.RoundSHA256 != "" {
		t.Fatalf("VerifyMinimizationQuestionRound() = %#v, %v", verified, err)
	}

	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "oversized", data: bytes.Repeat([]byte("x"), questionArtifactMaxBytes+1)},
		{name: "invalid utf8", data: []byte{0xff}},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown field", data: []byte(`{"unexpected":true}`)},
		{name: "trailing", data: append(data, []byte(` {}`)...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeMinimizationQuestionRound(test.data); err == nil {
				t.Fatal("DecodeMinimizationQuestionRound() error = nil")
			}
		})
	}
}

func TestMinimizationQuestionRoundRejectsTampering(t *testing.T) {
	_, valid := validQuestionRound(t)
	tests := []struct {
		name   string
		mutate func(*MinimizationQuestionRound)
	}{
		{name: "schema", mutate: func(round *MinimizationQuestionRound) { round.SchemaVersion = 2 }},
		{name: "minimization identity", mutate: func(round *MinimizationQuestionRound) { round.MinimizationSHA256 = "short" }},
		{name: "empty candidates", mutate: func(round *MinimizationQuestionRound) { round.Candidates = nil }},
		{name: "duplicate candidate", mutate: func(round *MinimizationQuestionRound) { round.Candidates[1].ID = round.Candidates[0].ID }},
		{name: "candidate ID", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].ID = "../city" }},
		{name: "candidate receipt", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].ReceiptSHA256 = "short" }},
		{name: "candidate pairs per order", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].PairsPerOrder = 0 }},
		{name: "candidate completed", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].CompletedPairs = 5 }},
		{name: "candidate changed", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].ChangedPairs = -1 }},
		{name: "candidate same", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].NoChangePairs = -1 }},
		{name: "candidate unknown", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].UnknownPairs = -1 }},
		{name: "candidate count sum", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].ChangedPairs = 1 }},
		{name: "candidate evidence", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].EvidenceState = evidence.Claimed }},
		{name: "candidate outcome", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].Outcome = "other" }},
		{name: "candidate classification", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].Classification = "other" }},
		{name: "outcome counts", mutate: func(round *MinimizationQuestionRound) {
			round.Candidates[0].ChangedPairs = round.Candidates[0].Pairs
			round.Candidates[0].NoChangePairs = 0
		}},
		{name: "classification mismatch", mutate: func(round *MinimizationQuestionRound) { round.Candidates[0].Classification = CandidateInsufficient }},
		{name: "answer count", mutate: func(round *MinimizationQuestionRound) { round.Answers = round.Answers[:1] }},
		{name: "answer schema", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].SchemaVersion = 2 }},
		{name: "answer ID", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].QuestionID = MinimizationQuestionSupport }},
		{name: "answer text", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].Question = "private question" }},
		{name: "answer identity", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].MinimizationSHA256 = "short" }},
		{name: "answer evidence", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].EvidenceState = evidence.Claimed }},
		{name: "answer selection", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].SelectionState = SelectionNoSufficient }},
		{name: "answer selected candidate", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].SelectedCandidate = "city" }},
		{name: "answer candidate count", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].CandidateCount = 0 }},
		{name: "answer supported count", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].SupportedCandidates = 0 }},
		{name: "answer unknown count", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].UnknownCandidates = 1 }},
		{name: "answer result", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].Result = "private" }},
		{name: "answer reason", mutate: func(round *MinimizationQuestionRound) { round.Answers[0].Reason = "private" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneMinimizationQuestionRound(valid)
			test.mutate(&mutated)
			if _, err := MinimizationQuestionRoundSHA256(mutated); err == nil {
				t.Fatal("MinimizationQuestionRoundSHA256() accepted tampered round")
			}
		})
	}
}

func TestMinimizationQuestionReceiptRoundTripAndTampering(t *testing.T) {
	_, round := validQuestionRound(t)
	roundSHA256, err := MinimizationQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := MinimizationQuestionReceipt{
		MinimizationQuestionAnswer: round.Answers[0],
		RoundSHA256:                roundSHA256,
		Round:                      round,
	}
	receiptSHA256, err := MinimizationQuestionReceiptSHA256(receipt)
	if err != nil {
		t.Fatalf("MinimizationQuestionReceiptSHA256() error = %v", err)
	}
	if !validDigest(receiptSHA256) {
		t.Fatalf("receipt digest = %q", receiptSHA256)
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeMinimizationQuestionReceipt(append(data, '\n'))
	if err != nil {
		t.Fatalf("DecodeMinimizationQuestionReceipt() error = %v", err)
	}
	if decoded.QuestionID != receipt.QuestionID || decoded.RoundSHA256 != roundSHA256 {
		t.Fatalf("decoded receipt = %#v", decoded)
	}
	if _, err := AskMinimizationQuestionReceipt(filepath.Join(t.TempDir(), "missing.json"), MinimizationQuestionSelection); err == nil {
		t.Fatal("AskMinimizationQuestionReceipt() accepted a missing round")
	}

	tests := []struct {
		name   string
		mutate func(*MinimizationQuestionReceipt)
	}{
		{name: "schema", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.SchemaVersion = 2 }},
		{name: "round schema", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.Round.SchemaVersion = 2 }},
		{name: "round identity syntax", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.RoundSHA256 = "short" }},
		{name: "round identity mismatch", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.RoundSHA256 = strings.Repeat("a", 64) }},
		{name: "question ID", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.QuestionID = "not-a-question" }},
		{name: "answer mismatch", mutate: func(receipt *MinimizationQuestionReceipt) { receipt.Question = "private question" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := receipt
			mutated.Round = cloneMinimizationQuestionRound(receipt.Round)
			test.mutate(&mutated)
			if _, err := MinimizationQuestionReceiptSHA256(mutated); err == nil {
				t.Fatal("MinimizationQuestionReceiptSHA256() accepted tampered receipt")
			}
		})
	}

	boundaryTests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "oversized", data: bytes.Repeat([]byte("x"), questionArtifactMaxBytes+1)},
		{name: "invalid utf8", data: []byte{0xff}},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown field", data: []byte(`{"unexpected":true}`)},
		{name: "trailing", data: append(data, []byte(` {}`)...)},
	}
	for _, test := range boundaryTests {
		t.Run("decode "+test.name, func(t *testing.T) {
			if _, err := DecodeMinimizationQuestionReceipt(test.data); err == nil {
				t.Fatal("DecodeMinimizationQuestionReceipt() error = nil")
			}
		})
	}
}

func TestMinimizationQuestionArtifactsPersistWithoutRawInputs(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "candidate-001-city")
	makeValidChildReplication(t, childDir)
	child, err := bundle.VerifyReplicated(childDir)
	if err != nil {
		t.Fatalf("bundle.VerifyReplicated() error = %v", err)
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
		t.Fatalf("Save() error = %v", err)
	}

	artifacts := t.TempDir()
	roundPath := filepath.Join(artifacts, "nested", "round.json")
	roundSummary, err := SaveMinimizationQuestionRound(root, roundPath)
	if err != nil {
		t.Fatalf("SaveMinimizationQuestionRound() error = %v", err)
	}
	if roundSummary.Questions != len(MinimizationQuestions()) || roundSummary.Candidates != 1 || !validDigest(roundSummary.RoundSHA256) {
		t.Fatalf("round summary = %#v", roundSummary)
	}
	if _, err := SaveMinimizationQuestionRound(root, roundPath); err == nil {
		t.Fatal("SaveMinimizationQuestionRound() overwrote an existing round")
	}
	round, verifiedRound, err := ReadMinimizationQuestionRound(roundPath)
	if err != nil {
		t.Fatalf("ReadMinimizationQuestionRound() error = %v", err)
	}
	if verifiedRound != roundSummary || round.MinimizationSHA256 != roundSummary.MinimizationSHA256 {
		t.Fatalf("round = %#v, summary = %#v", round, roundSummary)
	}
	if verified, err := VerifyMinimizationQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyMinimizationQuestionRound() = %#v, %v", verified, err)
	}
	roundData, err := os.ReadFile(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"37.7749-122.4194", "baseline@example.invalid", "emulator-5554", "dev.ariadne.fixture"} {
		if strings.Contains(string(roundData), secret) {
			t.Fatalf("saved round disclosed %q", secret)
		}
	}

	if _, err := AskMinimizationQuestionReceipt(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskMinimizationQuestionReceipt() accepted an invalid question ID")
	}
	receiptPath := filepath.Join(artifacts, "receipt.json")
	receiptSummary, err := SaveMinimizationQuestionReceipt(roundPath, MinimizationQuestionSelection, receiptPath)
	if err != nil {
		t.Fatalf("SaveMinimizationQuestionReceipt() error = %v", err)
	}
	if receiptSummary.QuestionID != MinimizationQuestionSelection || !validDigest(receiptSummary.ReceiptSHA256) || receiptSummary.RoundSHA256 != roundSummary.RoundSHA256 {
		t.Fatalf("receipt summary = %#v", receiptSummary)
	}
	if _, err := SaveMinimizationQuestionReceipt(roundPath, MinimizationQuestionSelection, receiptPath); err == nil {
		t.Fatal("SaveMinimizationQuestionReceipt() overwrote an existing receipt")
	}
	receipt, verifiedReceipt, err := ReadMinimizationQuestionReceipt(receiptPath)
	if err != nil {
		t.Fatalf("ReadMinimizationQuestionReceipt() error = %v", err)
	}
	if verifiedReceipt != receiptSummary || receipt.QuestionID != MinimizationQuestionSelection {
		t.Fatalf("receipt = %#v, summary = %#v", receipt, receiptSummary)
	}
	if verified, err := VerifyMinimizationQuestionReceipt(receiptPath); err != nil || verified != receiptSummary {
		t.Fatalf("VerifyMinimizationQuestionReceipt() = %#v, %v", verified, err)
	}
	receiptData, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"37.7749-122.4194", "baseline@example.invalid", "emulator-5554", "dev.ariadne.fixture"} {
		if strings.Contains(string(receiptData), secret) {
			t.Fatalf("saved receipt disclosed %q", secret)
		}
	}
}

func TestMinimizationQuestionArtifactPathAndReadBoundaries(t *testing.T) {
	_, round := validQuestionRound(t)
	roundData, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMinimizationQuestionRound(""); err == nil {
		t.Fatal("ReadMinimizationQuestionRound() accepted an empty path")
	}
	if _, err := VerifyMinimizationQuestionRound(""); err == nil {
		t.Fatal("VerifyMinimizationQuestionRound() accepted an empty path")
	}
	if _, err := SaveMinimizationQuestionRound("missing", ""); err == nil {
		t.Fatal("SaveMinimizationQuestionRound() accepted an empty output path")
	}
	if _, err := SaveMinimizationQuestionReceipt("missing", MinimizationQuestionSelection, ""); err == nil {
		t.Fatal("SaveMinimizationQuestionReceipt() accepted an empty output path")
	}
	oversizedPath := filepath.Join(t.TempDir(), "round.json")
	if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), questionArtifactMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadMinimizationQuestionRound(oversizedPath); err == nil {
		t.Fatal("ReadMinimizationQuestionRound() accepted an oversized file")
	}
	if err := writeQuestionArtifact("", roundData, "round"); err == nil {
		t.Fatal("writeQuestionArtifact() accepted an empty path")
	}
	if err := writeQuestionArtifact(filepath.Join(t.TempDir(), "too-large.json"), bytes.Repeat([]byte("x"), questionArtifactMaxBytes+1), "round"); err == nil {
		t.Fatal("writeQuestionArtifact() accepted oversized data")
	}
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeQuestionArtifact(filepath.Join(parent, "round.json"), roundData, "round"); err == nil {
		t.Fatal("writeQuestionArtifact() accepted a file as its parent")
	}
}

func validQuestionRound(t *testing.T) (MinimizationSummary, MinimizationQuestionRound) {
	t.Helper()
	summary, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	round, err := AnswerMinimizationQuestionRound(summary, strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	return summary, round
}

func cloneMinimizationQuestionRound(round MinimizationQuestionRound) MinimizationQuestionRound {
	clone := round
	clone.Candidates = append([]MinimizationCandidateProjection(nil), round.Candidates...)
	clone.Answers = append([]MinimizationQuestionAnswer(nil), round.Answers...)
	return clone
}
func TestLadderQuestionArtifactsReuseCanonicalContract(t *testing.T) {
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{
		ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	receiptSHA256 := strings.Repeat("c", 64)
	catalog := LadderQuestions()
	legacyCatalog := MinimizationQuestions()
	if len(catalog) != len(legacyCatalog) {
		t.Fatalf("catalog lengths = %d and %d", len(catalog), len(legacyCatalog))
	}
	for index := range catalog {
		if catalog[index] != legacyCatalog[index] {
			t.Fatalf("catalog item %d = %#v and %#v", index, catalog[index], legacyCatalog[index])
		}
	}

	answer, err := AnswerLadderQuestion(summary, receiptSHA256, MinimizationQuestionSelection)
	if err != nil {
		t.Fatalf("AnswerLadderQuestion() error = %v", err)
	}
	if answer.Result != string(SelectionSelected) || answer.EvidenceState != evidence.Observed || answer.SelectedCandidate != "omitted" || answer.MinimizationSHA256 != receiptSHA256 {
		t.Fatalf("ladder answer = %#v", answer)
	}
	answers, err := AnswerAllLadderQuestions(summary, receiptSHA256)
	if err != nil || len(answers) != len(catalog) {
		t.Fatalf("AnswerAllLadderQuestions() = %#v, %v", answers, err)
	}
	round, err := AnswerLadderQuestionRound(summary, receiptSHA256)
	if err != nil {
		t.Fatalf("AnswerLadderQuestionRound() error = %v", err)
	}
	if round.MinimizationSHA256 != receiptSHA256 || len(round.Candidates) != 1 || len(round.Answers) != len(catalog) {
		t.Fatalf("ladder round = %#v", round)
	}
	roundPath := filepath.Join(t.TempDir(), "nested", "round.json")
	roundSummary, err := SaveLadderQuestionRound(summary, receiptSHA256, roundPath)
	if err != nil {
		t.Fatalf("SaveLadderQuestionRound() error = %v", err)
	}
	if roundSummary.Questions != len(catalog) || roundSummary.Candidates != 1 || !validDigest(roundSummary.RoundSHA256) {
		t.Fatalf("ladder round summary = %#v", roundSummary)
	}
	if _, err := SaveLadderQuestionRound(summary, receiptSHA256, roundPath); err == nil {
		t.Fatal("SaveLadderQuestionRound() overwrote an existing round")
	}
	readRound, readSummary, err := ReadLadderQuestionRound(roundPath)
	if err != nil || readSummary != roundSummary || len(readRound.Answers) != len(catalog) {
		t.Fatalf("ReadLadderQuestionRound() = %#v, %#v, %v", readRound, readSummary, err)
	}
	if verified, err := VerifyLadderQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyLadderQuestionRound() = %#v, %v", verified, err)
	}
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	receiptSummary, err := SaveLadderQuestionReceipt(roundPath, MinimizationQuestionSelection, receiptPath)
	if err != nil {
		t.Fatalf("SaveLadderQuestionReceipt() error = %v", err)
	}
	if receiptSummary.QuestionID != MinimizationQuestionSelection || receiptSummary.RoundSHA256 != roundSummary.RoundSHA256 || !validDigest(receiptSummary.ReceiptSHA256) {
		t.Fatalf("ladder receipt summary = %#v", receiptSummary)
	}
	receipt, err := AskLadderQuestionReceipt(roundPath, MinimizationQuestionSelection)
	if err != nil || receipt.QuestionID != MinimizationQuestionSelection {
		t.Fatalf("AskLadderQuestionReceipt() = %#v, %v", receipt, err)
	}
	readReceipt, readReceiptSummary, err := ReadLadderQuestionReceipt(receiptPath)
	if err != nil || readReceiptSummary != receiptSummary || readReceipt.QuestionID != receipt.QuestionID {
		t.Fatalf("ReadLadderQuestionReceipt() = %#v, %#v, %v", readReceipt, readReceiptSummary, err)
	}
	if verified, err := VerifyLadderQuestionReceipt(receiptPath); err != nil || verified != receiptSummary {
		t.Fatalf("VerifyLadderQuestionReceipt() = %#v, %v", verified, err)
	}
	for _, path := range []string{roundPath, receiptPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"37.7749-122.4194", "baseline@example.invalid", "https://", "fixture-value"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("ladder artifact %q disclosed %q", path, secret)
			}
		}
	}
}

func TestLadderQuestionsPreserveOutcomeAndEvidenceStates(t *testing.T) {
	tests := []struct {
		name        string
		outcome     portabletrace.ReplicatedOutcome
		state       evidence.State
		selection   SelectionState
		selected    string
		answerState evidence.State
		pairs       int
		changed     int
		noChange    int
		unknown     int
		completed   int
	}{
		{name: "selected", outcome: portabletrace.NoChangeObserved, state: evidence.Observed, selection: SelectionSelected, selected: "omitted", answerState: evidence.Observed, pairs: 1, noChange: 2, completed: 2},
		{name: "no sufficient", outcome: portabletrace.ReplicatedChange, state: evidence.Observed, selection: SelectionNoSufficient, answerState: evidence.Observed, pairs: 1, changed: 2, completed: 2},
		{name: "mixed", outcome: portabletrace.MixedInconsistent, state: evidence.Observed, selection: SelectionUnknown, answerState: evidence.Observed, pairs: 1, changed: 1, noChange: 1, completed: 2},
		{name: "unknown", outcome: portabletrace.ReplicationUnknown, state: evidence.Unknown, selection: SelectionUnknown, answerState: evidence.Unknown, pairs: 1, unknown: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{
				ladderResult("omitted", test.outcome, test.state, test.pairs, test.changed, test.noChange, test.unknown, test.completed),
			})
			if err != nil {
				t.Fatal(err)
			}
			answers, err := AnswerAllLadderQuestions(summary, strings.Repeat("c", 64))
			if err != nil {
				t.Fatal(err)
			}
			if answers[0].SelectionState != test.selection || answers[0].SelectedCandidate != test.selected || answers[0].EvidenceState != test.answerState {
				t.Fatalf("selection answer = %#v", answers[0])
			}
			round, err := AnswerLadderQuestionRound(summary, strings.Repeat("c", 64))
			if err != nil {
				t.Fatal(err)
			}
			candidate := round.Candidates[0]
			if candidate.Outcome != test.outcome || candidate.EvidenceState != test.state {
				t.Fatalf("candidate projection = %#v", candidate)
			}
		})
	}
}
func TestLadderQuestionAPIsRejectInvalidInputs(t *testing.T) {
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{
		ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	validReceipt := strings.Repeat("a", 64)
	if _, err := AnswerLadderQuestion(summary, validReceipt, "not-a-question"); err == nil {
		t.Fatal("AnswerLadderQuestion() accepted an invalid question ID")
	}
	if _, err := AnswerLadderQuestion(LadderSummary{}, validReceipt, MinimizationQuestionSelection); err == nil {
		t.Fatal("AnswerLadderQuestion() accepted an invalid summary")
	}
	if _, err := AnswerLadderQuestion(summary, "short", MinimizationQuestionSelection); err == nil {
		t.Fatal("AnswerLadderQuestion() accepted an invalid receipt identity")
	}
	if _, err := AnswerAllLadderQuestions(LadderSummary{}, validReceipt); err == nil {
		t.Fatal("AnswerAllLadderQuestions() accepted an invalid summary")
	}
	if _, err := AnswerAllLadderQuestions(summary, "short"); err == nil {
		t.Fatal("AnswerAllLadderQuestions() accepted an invalid receipt identity")
	}
	if _, err := AnswerLadderQuestionRound(LadderSummary{}, validReceipt); err == nil {
		t.Fatal("AnswerLadderQuestionRound() accepted an invalid summary")
	}
	if _, err := SaveLadderQuestionRound(summary, validReceipt, ""); err == nil {
		t.Fatal("SaveLadderQuestionRound() accepted an empty output path")
	}
	if _, _, err := ReadLadderQuestionRound(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadLadderQuestionRound() accepted a missing round")
	}
	if _, err := VerifyLadderQuestionRound(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("VerifyLadderQuestionRound() accepted a missing round")
	}
	if _, err := AskLadderQuestionReceipt(filepath.Join(t.TempDir(), "missing.json"), MinimizationQuestionSelection); err == nil {
		t.Fatal("AskLadderQuestionReceipt() accepted a missing round")
	}
	if _, err := SaveLadderQuestionReceipt(filepath.Join(t.TempDir(), "missing.json"), MinimizationQuestionSelection, ""); err == nil {
		t.Fatal("SaveLadderQuestionReceipt() accepted an empty output path")
	}
	if _, _, err := ReadLadderQuestionReceipt(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadLadderQuestionReceipt() accepted a missing receipt")
	}
	if _, err := VerifyLadderQuestionReceipt(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("VerifyLadderQuestionReceipt() accepted a missing receipt")
	}
}
