package bundle

import (
	"errors"
	"fmt"
)

const (
	archiveQuestionComparisonSchemaVersion = 1
	archiveQuestionComparisonID            = "answer-state-change"
	archiveQuestionComparisonText          = "Did the bounded answer state change between these saved reflection snapshots?"
)

// ArchiveQuestionComparison is a raw-value-free comparison of two verified
// archive-question reflections. It describes answer-state structure only; it
// does not infer a trend or prove the underlying evidence.
type ArchiveQuestionComparison struct {
	SchemaVersion         int    `json:"schema_version"`
	ComparisonID          string `json:"comparison_id"`
	ComparisonQuestion    string `json:"comparison_question"`
	QuestionID            string `json:"question_id"`
	Question              string `json:"question"`
	Result                string `json:"result"`
	OlderReflectionSHA256 string `json:"older_reflection_sha256"`
	NewerReflectionSHA256 string `json:"newer_reflection_sha256"`
	Compared              int    `json:"compared"`
	Changed               int    `json:"changed"`
	OlderOnly             int    `json:"older_only"`
	NewerOnly             int    `json:"newer_only"`
}

// CompareArchiveQuestionReports compares only verified answer states from
// two saved reflections. A changing archive membership is incomparable rather
// than evidence of a change.
func CompareArchiveQuestionReports(olderPath, newerPath string) (ArchiveQuestionComparison, error) {
	older, olderSummary, err := readVerifiedArchiveQuestionReport(olderPath)
	if err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("older reflection: %w", err)
	}
	newer, newerSummary, err := readVerifiedArchiveQuestionReport(newerPath)
	if err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("newer reflection: %w", err)
	}
	if older.QuestionID != newer.QuestionID || older.Question != newer.Question {
		return ArchiveQuestionComparison{}, errors.New("archive reflection questions do not match")
	}

	olderStates := archiveQuestionStates(older)
	newerStates := archiveQuestionStates(newer)
	comparison := ArchiveQuestionComparison{
		SchemaVersion:         archiveQuestionComparisonSchemaVersion,
		ComparisonID:          archiveQuestionComparisonID,
		ComparisonQuestion:    archiveQuestionComparisonText,
		QuestionID:            older.QuestionID,
		Question:              older.Question,
		OlderReflectionSHA256: olderSummary.ReflectionSHA256,
		NewerReflectionSHA256: newerSummary.ReflectionSHA256,
	}
	for directory, olderState := range olderStates {
		newerState, ok := newerStates[directory]
		if !ok {
			comparison.OlderOnly++
			continue
		}
		comparison.Compared++
		if olderState != newerState {
			comparison.Changed++
		}
	}
	for directory := range newerStates {
		if _, ok := olderStates[directory]; !ok {
			comparison.NewerOnly++
		}
	}
	if comparison.OlderOnly != 0 || comparison.NewerOnly != 0 {
		comparison.Result = "incomparable"
	} else if comparison.Changed != 0 {
		comparison.Result = "changed"
	} else {
		comparison.Result = "same"
	}
	return comparison, nil
}

func archiveQuestionStates(report ArchiveQuestionReport) map[string]string {
	states := make(map[string]string, len(report.Results))
	for _, result := range report.Results {
		state := "unavailable"
		if result.Available {
			state = string(result.Answer.State)
		}
		states[result.Directory] = state
	}
	return states
}
