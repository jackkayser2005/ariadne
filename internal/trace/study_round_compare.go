package trace

import (
	"fmt"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const (
	replicationStudyQuestionRoundComparisonSchemaVersion = 1
	replicationStudyQuestionRoundComparisonID            = "replication-study-question-round-answer-change"
	replicationStudyQuestionRoundComparisonText          = "Did the bounded study answers change between these retained question rounds?"
)

const (
	studyRoundComparisonSame         = "same"
	studyRoundComparisonChanged      = "changed"
	studyRoundComparisonIncomparable = "incomparable"
)

// ReplicationStudyQuestionRoundComparison compares two verified study
// question rounds in caller-supplied order. It contains only fixed question
// projections and never infers chronology, causality, improvement, or
// regression.
type ReplicationStudyQuestionRoundComparison struct {
	SchemaVersion      int                                   `json:"schema_version"`
	ComparisonID       string                                `json:"comparison_id"`
	ComparisonQuestion string                                `json:"comparison_question"`
	OrderBasis         string                                `json:"order_basis"`
	Result             string                                `json:"result"`
	IncomparableReason string                                `json:"incomparable_reason,omitempty"`
	FirstRoundSHA256   string                                `json:"first_round_sha256"`
	SecondRoundSHA256  string                                `json:"second_round_sha256"`
	FirstStudySHA256   string                                `json:"first_study_sha256"`
	SecondStudySHA256  string                                `json:"second_study_sha256"`
	Compared           int                                   `json:"compared"`
	Changed            int                                   `json:"changed"`
	ChangedQuestions   []ReplicationStudyQuestionRoundChange `json:"changed_questions"`
}

// ReplicationStudyQuestionRoundChange identifies one fixed study question
// whose bounded result, outcome, evidence state, or support counts differ.
type ReplicationStudyQuestionRoundChange struct {
	QuestionID          string            `json:"question_id"`
	FirstResult         string            `json:"first_result"`
	SecondResult        string            `json:"second_result"`
	FirstEvidenceState  evidence.State    `json:"first_evidence_state"`
	SecondEvidenceState evidence.State    `json:"second_evidence_state"`
	FirstOutcome        ReplicatedOutcome `json:"first_outcome"`
	SecondOutcome       ReplicatedOutcome `json:"second_outcome"`
	ChangeKinds         []string          `json:"change_kinds"`
}

// CompareReplicationStudyQuestionRounds verifies both studies, verifies each
// round against its supplied study, and compares only their fixed question
// projections. The caller's order is retained as first and second.
func CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, secondStudyPath, secondRoundPath string) (ReplicationStudyQuestionRoundComparison, error) {
	firstStudy, firstStudySummary, firstRound, firstRoundSummary, err := readComparableReplicationStudyQuestionRound(firstStudyPath, firstRoundPath)
	if err != nil {
		return ReplicationStudyQuestionRoundComparison{}, fmt.Errorf("first study question round: %w", err)
	}
	secondStudy, secondStudySummary, secondRound, secondRoundSummary, err := readComparableReplicationStudyQuestionRound(secondStudyPath, secondRoundPath)
	if err != nil {
		return ReplicationStudyQuestionRoundComparison{}, fmt.Errorf("second study question round: %w", err)
	}

	comparison := ReplicationStudyQuestionRoundComparison{
		SchemaVersion:      replicationStudyQuestionRoundComparisonSchemaVersion,
		ComparisonID:       replicationStudyQuestionRoundComparisonID,
		ComparisonQuestion: replicationStudyQuestionRoundComparisonText,
		OrderBasis:         ReplicationStudyOrderBasis,
		Result:             studyRoundComparisonSame,
		FirstRoundSHA256:   firstRoundSummary.RoundSHA256,
		SecondRoundSHA256:  secondRoundSummary.RoundSHA256,
		FirstStudySHA256:   firstStudySummary.StudySHA256,
		SecondStudySHA256:  secondStudySummary.StudySHA256,
		ChangedQuestions:   make([]ReplicationStudyQuestionRoundChange, 0),
	}
	if reason := replicationStudyQuestionRoundIncompatibility(firstStudy, secondStudy); reason != "" {
		comparison.Result = studyRoundComparisonIncomparable
		comparison.IncomparableReason = reason
		return comparison, nil
	}

	questions := ReplicationStudyQuestions()
	comparison.Compared = len(questions)
	for index, question := range questions {
		firstAnswer := firstRound.Answers[index]
		secondAnswer := secondRound.Answers[index]
		changeKinds := replicationStudyQuestionRoundChangeKinds(firstAnswer, secondAnswer)
		if len(changeKinds) == 0 {
			continue
		}
		comparison.ChangedQuestions = append(comparison.ChangedQuestions, ReplicationStudyQuestionRoundChange{
			QuestionID:          question.ID,
			FirstResult:         firstAnswer.Result,
			SecondResult:        secondAnswer.Result,
			FirstEvidenceState:  firstAnswer.EvidenceState,
			SecondEvidenceState: secondAnswer.EvidenceState,
			FirstOutcome:        firstAnswer.Outcome,
			SecondOutcome:       secondAnswer.Outcome,
			ChangeKinds:         changeKinds,
		})
	}
	comparison.Changed = len(comparison.ChangedQuestions)
	if comparison.Changed > 0 {
		comparison.Result = studyRoundComparisonChanged
	}
	return comparison, nil
}

func readComparableReplicationStudyQuestionRound(studyPath, roundPath string) (ReplicationStudy, StudyVerificationSummary, ReplicationStudyQuestionRound, ReplicationStudyQuestionRoundVerificationSummary, error) {
	study, studySummary, err := ReadReplicationStudy(studyPath)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("study: %w", err)
	}
	round, roundSummary, err := ReadReplicationStudyQuestionRound(roundPath)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("question round: %w", err)
	}
	if round.StudySHA256 != studySummary.StudySHA256 || roundSummary.StudySHA256 != studySummary.StudySHA256 {
		return ReplicationStudy{}, StudyVerificationSummary{}, ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("question round does not match study")
	}
	expectedRound, err := AnswerReplicationStudyQuestionRound(study, studySummary)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("question round support: %w", err)
	}
	if round.SchemaVersion != expectedRound.SchemaVersion || round.StudySHA256 != expectedRound.StudySHA256 || !slices.Equal(round.Answers, expectedRound.Answers) {
		return ReplicationStudy{}, StudyVerificationSummary{}, ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("question round answers do not match study")
	}
	return study, studySummary, round, roundSummary, nil
}

func replicationStudyQuestionRoundIncompatibility(first, second ReplicationStudy) string {
	firstProvenance, _ := replicationStudyRunProvenance(first.Runs[0].Ledger)
	secondProvenance, _ := replicationStudyRunProvenance(second.Runs[0].Ledger)
	contrastDiffers := first.ContrastSHA256 != second.ContrastSHA256
	provenanceDiffers := firstProvenance != secondProvenance
	switch {
	case contrastDiffers && provenanceDiffers:
		return "counterfactual commitments and reviewed source provenance differ"
	case contrastDiffers:
		return "counterfactual commitments differ"
	case provenanceDiffers:
		return "reviewed source provenance differs"
	default:
		return ""
	}
}

func replicationStudyQuestionRoundChangeKinds(first, second StudyQuestionAnswer) []string {
	changeKinds := make([]string, 0, 4)
	if first.Result != second.Result {
		changeKinds = append(changeKinds, "result")
	}
	if first.Outcome != second.Outcome {
		changeKinds = append(changeKinds, "outcome")
	}
	if first.EvidenceState != second.EvidenceState {
		changeKinds = append(changeKinds, "evidence-state")
	}
	if !replicationStudyQuestionSupportCountsEqual(first, second) {
		changeKinds = append(changeKinds, "support-counts")
	}
	return changeKinds
}

func replicationStudyQuestionSupportCountsEqual(first, second StudyQuestionAnswer) bool {
	return first.Runs == second.Runs &&
		first.Pairs == second.Pairs &&
		first.SupportedRuns == second.SupportedRuns &&
		first.UnknownRuns == second.UnknownRuns &&
		first.ResetConfirmedPairs == second.ResetConfirmedPairs &&
		first.BalancedRuns == second.BalancedRuns &&
		first.CompletePairs == second.CompletePairs &&
		first.ChangedRuns == second.ChangedRuns &&
		first.NoChangeRuns == second.NoChangeRuns &&
		first.MixedRuns == second.MixedRuns &&
		first.UnknownPairs == second.UnknownPairs
}
