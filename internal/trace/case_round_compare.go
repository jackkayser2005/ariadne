package trace

import (
	"fmt"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const (
	caseQuestionRoundComparisonSchemaVersion = 1
	caseQuestionRoundComparisonID            = "case-question-round-answer-change"
	caseQuestionRoundComparisonText          = "Did the bounded case answers change between these retained question rounds?"
)

// CaseQuestionRoundComparison is a raw-value-free comparison of two verified
// case question rounds in caller-supplied order. It does not infer chronology,
// causality, or direction.
type CaseQuestionRoundComparison struct {
	SchemaVersion      int                       `json:"schema_version"`
	ComparisonID       string                    `json:"comparison_id"`
	ComparisonQuestion string                    `json:"comparison_question"`
	OrderBasis         string                    `json:"order_basis"`
	Result             string                    `json:"result"`
	FirstRoundSHA256   string                    `json:"first_round_sha256"`
	SecondRoundSHA256  string                    `json:"second_round_sha256"`
	FirstCaseSHA256    string                    `json:"first_case_sha256"`
	SecondCaseSHA256   string                    `json:"second_case_sha256"`
	Compared           int                       `json:"compared"`
	Changed            int                       `json:"changed"`
	ChangedQuestions   []CaseQuestionRoundChange `json:"changed_questions"`
}

// CaseQuestionRoundChange identifies one fixed case question whose bounded
// projection differs between two retained rounds.
type CaseQuestionRoundChange struct {
	QuestionID          string         `json:"question_id"`
	FirstResult         string         `json:"first_result"`
	SecondResult        string         `json:"second_result"`
	FirstEvidenceState  evidence.State `json:"first_evidence_state"`
	SecondEvidenceState evidence.State `json:"second_evidence_state"`
	ChangeKinds         []string       `json:"change_kinds"`
}

// CompareCaseQuestionRounds verifies two saved case question rounds and
// compares only their fixed, raw-value-free question projections. The caller's
// order is retained as first and second; no chronology is inferred.
func CompareCaseQuestionRounds(firstPath, secondPath string) (CaseQuestionRoundComparison, error) {
	first, firstSummary, err := ReadCaseQuestionRound(firstPath)
	if err != nil {
		return CaseQuestionRoundComparison{}, fmt.Errorf("first question round: %w", err)
	}
	second, secondSummary, err := ReadCaseQuestionRound(secondPath)
	if err != nil {
		return CaseQuestionRoundComparison{}, fmt.Errorf("second question round: %w", err)
	}

	comparison := CaseQuestionRoundComparison{
		SchemaVersion:      caseQuestionRoundComparisonSchemaVersion,
		ComparisonID:       caseQuestionRoundComparisonID,
		ComparisonQuestion: caseQuestionRoundComparisonText,
		OrderBasis:         "caller",
		FirstRoundSHA256:   firstSummary.RoundSHA256,
		SecondRoundSHA256:  secondSummary.RoundSHA256,
		FirstCaseSHA256:    first.CaseSHA256,
		SecondCaseSHA256:   second.CaseSHA256,
		Compared:           len(CaseQuestions()),
		ChangedQuestions:   make([]CaseQuestionRoundChange, 0),
	}
	for index, question := range CaseQuestions() {
		firstAnswer := first.Answers[index]
		secondAnswer := second.Answers[index]
		changeKinds := caseQuestionRoundChangeKinds(question.ID, firstAnswer, secondAnswer)
		if len(changeKinds) == 0 {
			continue
		}
		comparison.ChangedQuestions = append(comparison.ChangedQuestions, CaseQuestionRoundChange{
			QuestionID:          question.ID,
			FirstResult:         firstAnswer.Result,
			SecondResult:        secondAnswer.Result,
			FirstEvidenceState:  firstAnswer.EvidenceState,
			SecondEvidenceState: secondAnswer.EvidenceState,
			ChangeKinds:         changeKinds,
		})
	}
	comparison.Changed = len(comparison.ChangedQuestions)
	if comparison.Changed == 0 {
		comparison.Result = "same"
	} else {
		comparison.Result = "changed"
	}
	return comparison, nil
}

func caseQuestionRoundChangeKinds(questionID string, first, second CaseAnswer) []string {
	changeKinds := make([]string, 0, 5)
	if first.Result != second.Result {
		changeKinds = appendCaseChangeKind(changeKinds, "result")
	}
	if first.EvidenceState != second.EvidenceState {
		changeKinds = appendCaseChangeKind(changeKinds, "evidence-state")
	}
	if !caseAnswerCountsEqual(questionID, first, second) {
		changeKinds = appendCaseChangeKind(changeKinds, "counts")
	}
	switch questionID {
	case CaseQuestionSources:
		if !slices.Equal(first.Sources, second.Sources) {
			changeKinds = appendCaseChangeKind(changeKinds, "sources")
		}
	case CaseQuestionOutcomes:
		outcomeChanged, evidenceChanged, countsChanged := caseOutcomeChangeKinds(first.Outcomes, second.Outcomes)
		if evidenceChanged {
			changeKinds = appendCaseChangeKind(changeKinds, "evidence-state")
		}
		if countsChanged && caseAnswerCountsEqual(questionID, first, second) {
			changeKinds = appendCaseChangeKind(changeKinds, "counts")
		}
		if outcomeChanged {
			changeKinds = appendCaseChangeKind(changeKinds, "outcomes")
		}
	}
	return changeKinds
}

func appendCaseChangeKind(kinds []string, kind string) []string {
	if slices.Contains(kinds, kind) {
		return kinds
	}
	return append(kinds, kind)
}

func caseOutcomeChangeKinds(first, second []CaseOutcomeSummary) (outcomeChanged, evidenceChanged, countsChanged bool) {
	if len(first) != len(second) {
		return true, false, true
	}
	for index := range first {
		if first[index].Position != second[index].Position || first[index].Outcome != second[index].Outcome {
			outcomeChanged = true
		}
		if first[index].EvidenceState != second[index].EvidenceState {
			evidenceChanged = true
		}
		if first[index].Pairs != second[index].Pairs || first[index].UnknownPairs != second[index].UnknownPairs {
			countsChanged = true
		}
	}
	return outcomeChanged, evidenceChanged, countsChanged
}

func caseAnswerCountsEqual(questionID string, first, second CaseAnswer) bool {
	if questionID == CaseQuestionSupport {
		return first.UnknownEntries == second.UnknownEntries
	}
	return first.Entries == second.Entries &&
		first.Archives == second.Archives &&
		first.Replications == second.Replications &&
		first.UnknownEntries == second.UnknownEntries
}
