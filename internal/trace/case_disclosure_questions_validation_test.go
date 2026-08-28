package trace

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCaseDisclosureQuestionValidationBoundaries(t *testing.T) {
	round := validDisclosureQuestionValidationRound(t)
	for _, test := range []struct {
		name   string
		mutate func(*CaseDisclosureQuestionRound)
	}{
		{"schema", func(candidate *CaseDisclosureQuestionRound) { candidate.SchemaVersion++ }},
		{"order", func(candidate *CaseDisclosureQuestionRound) { candidate.OrderBasis = "chronological" }},
		{"case digest", func(candidate *CaseDisclosureQuestionRound) { candidate.CaseSHA256 = "invalid" }},
		{"trace count", func(candidate *CaseDisclosureQuestionRound) { candidate.Traces = 0 }},
		{"coverage state", func(candidate *CaseDisclosureQuestionRound) { candidate.CoverageState = evidence.State("claimed") }},
		{"nil categories", func(candidate *CaseDisclosureQuestionRound) { candidate.Categories = nil }},
		{"unsorted categories", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Categories[0], candidate.Categories[1] = candidate.Categories[1], candidate.Categories[0]
		}},
		{"invalid category", func(candidate *CaseDisclosureQuestionRound) { candidate.Categories[0].Category = "" }},
		{"nil boundaries", func(candidate *CaseDisclosureQuestionRound) { candidate.Categories[0].Boundaries = nil }},
		{"duplicate category", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Categories[1].Category = candidate.Categories[0].Category
		}},
		{"unsorted boundaries", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Categories[1].Boundaries[0], candidate.Categories[1].Boundaries[1] = candidate.Categories[1].Boundaries[1], candidate.Categories[1].Boundaries[0]
		}},
		{"invalid source", func(candidate *CaseDisclosureQuestionRound) { candidate.Categories[0].Boundaries[0].Source = "" }},
		{"invalid adapter", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Categories[0].Boundaries[0].Adapter = "android-experiment-001"
		}},
		{"duplicate boundary", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Categories[0].Boundaries = append(candidate.Categories[0].Boundaries, candidate.Categories[0].Boundaries[0])
		}},
		{"answer count", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers = candidate.Answers[:1] }},
		{"answer schema", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].SchemaVersion++ }},
		{"answer question ID", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].QuestionID = "wrong" }},
		{"answer question", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].Question = "wrong" }},
		{"answer case digest", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].CaseSHA256 = "invalid" }},
		{"answer trace count", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].Traces++ }},
		{"answer coverage", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Answers[0].CoverageState = evidence.State("claimed")
		}},
		{"answer evidence state", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Answers[0].EvidenceState = evidence.State("claimed")
		}},
		{"answer overlaps", func(candidate *CaseDisclosureQuestionRound) {
			candidate.Answers[0].OverlappingCategories = []string{"wrong"}
		}},
		{"answer reason", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].Reason = "wrong" }},
		{"answer result", func(candidate *CaseDisclosureQuestionRound) { candidate.Answers[0].Result = "wrong" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneCaseDisclosureQuestionRound(round)
			test.mutate(&candidate)
			if _, err := CaseDisclosureQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("CaseDisclosureQuestionRoundSHA256() accepted invalid round")
			}
		})
	}

	receipt := CaseDisclosureQuestionReceipt{
		CaseDisclosureQuestionAnswer: round.Answers[1],
		RoundSHA256:                  mustCaseDisclosureQuestionRoundSHA256(t, round),
		Round:                        round,
	}
	for _, test := range []struct {
		name   string
		mutate func(*CaseDisclosureQuestionReceipt)
	}{
		{"schema", func(candidate *CaseDisclosureQuestionReceipt) { candidate.SchemaVersion++ }},
		{"question ID", func(candidate *CaseDisclosureQuestionReceipt) { candidate.QuestionID = "wrong" }},
		{"answer", func(candidate *CaseDisclosureQuestionReceipt) { candidate.Question = "wrong" }},
		{"round digest", func(candidate *CaseDisclosureQuestionReceipt) { candidate.RoundSHA256 = "invalid" }},
		{"round", func(candidate *CaseDisclosureQuestionReceipt) { candidate.Round = CaseDisclosureQuestionRound{} }},
		{"round identity", func(candidate *CaseDisclosureQuestionReceipt) { candidate.RoundSHA256 = strings.Repeat("c", 64) }},
	} {
		t.Run("receipt "+test.name, func(t *testing.T) {
			candidate := receipt
			test.mutate(&candidate)
			if _, err := CaseDisclosureQuestionReceiptSHA256(candidate); err == nil {
				t.Fatal("CaseDisclosureQuestionReceiptSHA256() accepted invalid receipt")
			}
		})
	}
	if _, err := caseDisclosureQuestionAnswerFromRound(round, "wrong"); err == nil {
		t.Fatal("caseDisclosureQuestionAnswerFromRound() accepted an unknown question")
	}
	if _, err := CaseDisclosureQuestionReceiptSHA256(CaseDisclosureQuestionReceipt{}); err == nil {
		t.Fatal("CaseDisclosureQuestionReceiptSHA256() accepted an empty receipt")
	}
	if compareDisclosureCategories(round.Categories[0], round.Categories[1]) == 0 || compareDisclosureCategories(round.Categories[1], round.Categories[0]) == 0 {
		t.Fatal("compareDisclosureCategories() did not order categories")
	}
	if compareDisclosureBoundaries(round.Categories[1].Boundaries[0], round.Categories[1].Boundaries[1]) == 0 || compareDisclosureBoundaries(round.Categories[1].Boundaries[1], round.Categories[1].Boundaries[0]) == 0 {
		t.Fatal("compareDisclosureBoundaries() did not order boundaries")
	}
}

func validDisclosureQuestionValidationRound(t *testing.T) CaseDisclosureQuestionRound {
	t.Helper()
	archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
	ledgerPath, ledgerRoundPath := writeCaseLedger(t)
	casePath := filepath.Join(t.TempDir(), "case.json")
	if _, err := SaveCase([]CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	return round
}

func mustCaseDisclosureQuestionRoundSHA256(t *testing.T, round CaseDisclosureQuestionRound) string {
	t.Helper()
	digest, err := CaseDisclosureQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
