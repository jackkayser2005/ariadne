package bundle

import "fmt"

const (
	archiveQuestionTransitionHistoryQuestionRoundComparisonSchemaVersion = 1
	archiveQuestionTransitionHistoryQuestionRoundComparisonID            = "question-round-answer-change"
	archiveQuestionTransitionHistoryQuestionRoundComparisonText          = "Did the bounded answer results change between these retained question rounds?"
)

// ArchiveQuestionTransitionHistoryQuestionRoundComparison is a raw-value-free
// comparison of two retained fixed-question rounds in caller-supplied order.
// It does not infer chronology or prove the underlying evidence.
type ArchiveQuestionTransitionHistoryQuestionRoundComparison struct {
	SchemaVersion                 int                                                   `json:"schema_version"`
	ComparisonID                  string                                                `json:"comparison_id"`
	ComparisonQuestion            string                                                `json:"comparison_question"`
	OrderBasis                    string                                                `json:"order_basis"`
	Result                        string                                                `json:"result"`
	FirstRoundSHA256              string                                                `json:"first_round_sha256"`
	SecondRoundSHA256             string                                                `json:"second_round_sha256"`
	FirstTransitionHistorySHA256  string                                                `json:"first_transition_history_sha256"`
	SecondTransitionHistorySHA256 string                                                `json:"second_transition_history_sha256"`
	Compared                      int                                                   `json:"compared"`
	Changed                       int                                                   `json:"changed"`
	ChangedQuestions              []ArchiveQuestionTransitionHistoryQuestionRoundChange `json:"changed_questions"`
}

// ArchiveQuestionTransitionHistoryQuestionRoundChange identifies one fixed
// question whose bounded result differs between two retained rounds.
type ArchiveQuestionTransitionHistoryQuestionRoundChange struct {
	QuestionID   string `json:"question_id"`
	FirstResult  string `json:"first_result"`
	SecondResult string `json:"second_result"`
}

// CompareArchiveQuestionTransitionHistoryQuestionRounds verifies two saved
// question rounds and compares only their fixed bounded results. The caller's
// order is retained as first and second; no chronology is inferred.
func CompareArchiveQuestionTransitionHistoryQuestionRounds(firstPath, secondPath string) (ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
	first, firstSummary, err := readArchiveQuestionTransitionHistoryQuestionRound(firstPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, fmt.Errorf("first question round: %w", err)
	}
	second, secondSummary, err := readArchiveQuestionTransitionHistoryQuestionRound(secondPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundComparison{}, fmt.Errorf("second question round: %w", err)
	}

	comparison := ArchiveQuestionTransitionHistoryQuestionRoundComparison{
		SchemaVersion:                 archiveQuestionTransitionHistoryQuestionRoundComparisonSchemaVersion,
		ComparisonID:                  archiveQuestionTransitionHistoryQuestionRoundComparisonID,
		ComparisonQuestion:            archiveQuestionTransitionHistoryQuestionRoundComparisonText,
		OrderBasis:                    "caller",
		FirstRoundSHA256:              firstSummary.RoundSHA256,
		SecondRoundSHA256:             secondSummary.RoundSHA256,
		FirstTransitionHistorySHA256:  first.TransitionHistorySHA256,
		SecondTransitionHistorySHA256: second.TransitionHistorySHA256,
		Compared:                      len(first.Questions),
		ChangedQuestions:              make([]ArchiveQuestionTransitionHistoryQuestionRoundChange, 0),
	}
	for index, firstQuestion := range first.Questions {
		secondQuestion := second.Questions[index]
		if firstQuestion.Result == secondQuestion.Result {
			continue
		}
		comparison.ChangedQuestions = append(comparison.ChangedQuestions, ArchiveQuestionTransitionHistoryQuestionRoundChange{
			QuestionID:   firstQuestion.QuestionID,
			FirstResult:  firstQuestion.Result,
			SecondResult: secondQuestion.Result,
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
