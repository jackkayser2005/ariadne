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
		SchemaVersion:       replicationStudyQuestionRoundSchemaVersion,
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
	answer.Result, _ = replicationStudyQuestionResult(question.ID, answer.Outcome, answer.EvidenceState)
	answer.Reason = replicationStudyQuestionReason(answer)
	return answer
}

func replicationStudyQuestionResult(questionID string, outcome ReplicatedOutcome, state evidence.State) (string, bool) {
	switch questionID {
	case StudyQuestionOutcome:
		return string(outcome), true
	case StudyQuestionSupport:
		if state == evidence.Observed {
			return "supported", true
		}
		return "unknown", true
	case StudyQuestionConsistency:
		switch outcome {
		case ReplicatedChange, NoChangeObserved:
			return "consistent", true
		case MixedInconsistent:
			return "inconsistent", true
		case ReplicationUnknown:
			return "unknown", true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

func replicationStudyQuestionReason(answer StudyQuestionAnswer) string {
	switch answer.QuestionID {
	case StudyQuestionOutcome:
		if answer.EvidenceState != evidence.Observed || answer.Outcome == ReplicationUnknown {
			return "at least one replicated run lacks balanced order, reset, complete capture, or comparison support"
		}
		switch answer.Outcome {
		case ReplicatedChange:
			return "every supported replicated run contains a safe category difference"
		case NoChangeObserved:
			return "no supported replicated run contains a safe category difference"
		case MixedInconsistent:
			if answer.MixedRuns == answer.Runs {
				return "every supported replicated run contains internally inconsistent pair outcomes"
			}
			return "supported replicated runs disagree about safe category change"
		default:
			return "at least one replicated run lacks balanced order, reset, complete capture, or comparison support"
		}
	case StudyQuestionSupport:
		if answer.EvidenceState == evidence.Observed {
			return "every replicated run has balanced order, confirmed resets, and complete comparison support"
		}
		return "at least one replicated run lacks balanced order, reset, complete capture, or comparison support"
	case StudyQuestionConsistency:
		switch answer.Outcome {
		case ReplicatedChange, NoChangeObserved:
			return "all supported replicated runs agree about the aggregate outcome"
		case MixedInconsistent:
			return "supported replicated runs include mixed or disagreeing outcomes"
		default:
			return "the study lacks enough supported runs to establish cross-run consistency"
		}
	default:
		return ""
	}
}
func replicationStudyQuestion(id string) (StudyQuestion, bool) {
	for _, question := range ReplicationStudyQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return StudyQuestion{}, false
}
