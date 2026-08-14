package trace

import (
	"errors"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const (
	// StudyQuestionOutcome asks for the aggregate result across independent
	// replication runs.
	StudyQuestionOutcome = "study-outcome"
	// StudyQuestionSupport asks whether every run has sufficient support.
	StudyQuestionSupport = "study-support"
	// StudyQuestionConsistency asks whether supported runs agree.
	StudyQuestionConsistency = "cross-run-consistency"
)

// StudyQuestion is one fixed, bounded question available for a replication
// study.
type StudyQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// StudyQuestionAnswer is a safe answer tied to one verified study identity.
// Result describes the question outcome; EvidenceState remains separate.
type StudyQuestionAnswer struct {
	SchemaVersion       int               `json:"schema_version"`
	QuestionID          string            `json:"question_id"`
	Question            string            `json:"question"`
	Result              string            `json:"result"`
	EvidenceState       evidence.State    `json:"evidence_state"`
	Reason              string            `json:"reason"`
	StudySHA256         string            `json:"study_sha256"`
	Runs                int               `json:"runs"`
	Pairs               int               `json:"pairs"`
	SupportedRuns       int               `json:"supported_runs"`
	UnknownRuns         int               `json:"unknown_runs"`
	ResetConfirmedPairs int               `json:"reset_confirmed_pairs"`
	BalancedRuns        int               `json:"balanced_runs"`
	CompletePairs       int               `json:"complete_pairs"`
	ChangedRuns         int               `json:"changed_runs"`
	NoChangeRuns        int               `json:"no_change_runs"`
	MixedRuns           int               `json:"mixed_runs"`
	UnknownPairs        int               `json:"unknown_pairs"`
	Outcome             ReplicatedOutcome `json:"outcome"`
}

// ReplicationStudyQuestions returns the stable study question catalog.
func ReplicationStudyQuestions() []StudyQuestion {
	return []StudyQuestion{
		{ID: StudyQuestionOutcome, Text: "What aggregate outcome did the independent runs produce?"},
		{ID: StudyQuestionSupport, Text: "Did every run have balanced order, confirmed resets, complete comparison support, and observed evidence?"},
		{ID: StudyQuestionConsistency, Text: "Did supported runs agree, disagree, or remain unknown?"},
	}
}

// AnswerReplicationStudyQuestion answers one fixed question against a
// verified study and summary.
func AnswerReplicationStudyQuestion(study ReplicationStudy, summary StudyVerificationSummary, questionID string) (StudyQuestionAnswer, error) {
	question, ok := replicationStudyQuestion(questionID)
	if !ok {
		return StudyQuestionAnswer{}, errors.New("trace replication study question ID is invalid")
	}
	expectedSummary, err := replicationStudySummary(study)
	if err != nil {
		return StudyQuestionAnswer{}, err
	}
	if expectedSummary != summary {
		return StudyQuestionAnswer{}, errors.New("trace replication study question study identity does not match summary")
	}
	return answerReplicationStudyQuestionFromSummary(summary, question), nil
}

// AnswerAllReplicationStudyQuestions answers the complete fixed catalog in
// order against a verified study and summary.
func AnswerAllReplicationStudyQuestions(study ReplicationStudy, summary StudyVerificationSummary) ([]StudyQuestionAnswer, error) {
	expectedSummary, err := replicationStudySummary(study)
	if err != nil {
		return nil, err
	}
	if expectedSummary != summary {
		return nil, errors.New("trace replication study question study identity does not match summary")
	}
	answers := make([]StudyQuestionAnswer, 0, len(ReplicationStudyQuestions()))
	for _, question := range ReplicationStudyQuestions() {
		answers = append(answers, answerReplicationStudyQuestionFromSummary(summary, question))
	}
	return answers, nil
}

// AskReplicationStudyQuestion answers one fixed question after verifying a
// saved study.
func AskReplicationStudyQuestion(path, questionID string) (StudyQuestionAnswer, error) {
	study, summary, err := ReadReplicationStudy(path)
	if err != nil {
		return StudyQuestionAnswer{}, err
	}
	return AnswerReplicationStudyQuestion(study, summary, questionID)
}

// AskAllReplicationStudyQuestions answers the complete fixed catalog after
// verifying a saved study.
func AskAllReplicationStudyQuestions(path string) ([]StudyQuestionAnswer, error) {
	study, summary, err := ReadReplicationStudy(path)
	if err != nil {
		return nil, err
	}
	return AnswerAllReplicationStudyQuestions(study, summary)
}

func answerReplicationStudyQuestionFromSummary(summary StudyVerificationSummary, question StudyQuestion) StudyQuestionAnswer {
	answer := StudyQuestionAnswer{
		SchemaVersion:       summary.SchemaVersion,
		QuestionID:          question.ID,
		Question:            question.Text,
		EvidenceState:       summary.EvidenceState,
		StudySHA256:         summary.StudySHA256,
		Runs:                summary.Runs,
		Pairs:               summary.Pairs,
		SupportedRuns:       summary.SupportedRuns,
		UnknownRuns:         summary.UnknownRuns,
		ResetConfirmedPairs: summary.ResetConfirmedPairs,
		BalancedRuns:        summary.BalancedRuns,
		CompletePairs:       summary.CompletePairs,
		ChangedRuns:         summary.ChangedRuns,
		NoChangeRuns:        summary.NoChangeRuns,
		MixedRuns:           summary.MixedRuns,
		UnknownPairs:        summary.UnknownPairs,
		Outcome:             summary.Outcome,
	}
	switch question.ID {
	case StudyQuestionOutcome:
		answer.Result = string(summary.Outcome)
		answer.Reason = summary.Reason
	case StudyQuestionSupport:
		if summary.EvidenceState == evidence.Observed {
			answer.Result = "supported"
			answer.Reason = "every replicated run has balanced order, confirmed resets, and complete comparison support"
		} else {
			answer.Result = "unknown"
			answer.Reason = "at least one replicated run lacks balanced order, reset, or comparison support"
		}
	case StudyQuestionConsistency:
		switch summary.Outcome {
		case ReplicatedChange, NoChangeObserved:
			answer.Result = "consistent"
			answer.Reason = "all supported replicated runs agree about the aggregate outcome"
		case MixedInconsistent:
			answer.Result = "inconsistent"
			answer.Reason = "supported replicated runs include mixed or disagreeing outcomes"
		default:
			answer.Result = "unknown"
			answer.Reason = "the study lacks enough supported runs to establish cross-run consistency"
		}
	}
	return answer
}

func replicationStudyQuestion(id string) (StudyQuestion, bool) {
	for _, question := range ReplicationStudyQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return StudyQuestion{}, false
}
