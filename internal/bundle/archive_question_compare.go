package bundle

import (
	"errors"
	"fmt"
)

const (
	archiveQuestionComparisonSchemaVersion = 1
	archiveQuestionComparisonID            = "answer-state-change"
	archiveQuestionComparisonText          = "Did the bounded answer state change between these saved reflection snapshots?"
	archiveQuestionTransitionHistoryID     = "answer-state-transitions"
	archiveQuestionTransitionHistoryText   = "At which supplied boundaries did the bounded answer state change?"
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

// ArchiveQuestionTransitionHistory is a raw-value-free ledger of adjacent
// comparisons across caller-supplied verified reflections. It does not infer
// chronology, a trend, or the underlying evidence.
type ArchiveQuestionTransitionHistory struct {
	SchemaVersion   int                         `json:"schema_version"`
	HistoryID       string                      `json:"history_id"`
	HistoryQuestion string                      `json:"history_question"`
	QuestionID      string                      `json:"question_id"`
	Question        string                      `json:"question"`
	OrderBasis      string                      `json:"order_basis"`
	Snapshots       int                         `json:"snapshots"`
	Transitions     []ArchiveQuestionTransition `json:"transitions"`
}

// ArchiveQuestionTransition describes one adjacent bounded answer-state
// comparison without exposing raw values.
type ArchiveQuestionTransition struct {
	FromReflectionSHA256 string `json:"from_reflection_sha256"`
	ToReflectionSHA256   string `json:"to_reflection_sha256"`
	Result               string `json:"result"`
	Compared             int    `json:"compared"`
	Changed              int    `json:"changed"`
	FromOnly             int    `json:"from_only"`
	ToOnly               int    `json:"to_only"`
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
	return compareVerifiedArchiveQuestionReports(older, olderSummary, newer, newerSummary), nil
}

// CompareArchiveQuestionReportWithArchive verifies one saved reflection,
// re-asks its fixed question against the explicitly supplied current archive,
// and compares only their bounded answer states. The current reflection is
// derived in memory and is not persisted by this function.
func CompareArchiveQuestionReportWithArchive(olderPath, archiveRoot string) (ArchiveQuestionComparison, error) {
	older, olderSummary, err := readVerifiedArchiveQuestionReport(olderPath)
	if err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("older reflection: %w", err)
	}
	current, err := AskArchive(archiveRoot, older.QuestionID)
	if err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("current archive: %w", err)
	}
	if err := validateArchiveQuestionReport(current); err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("current archive reflection: %w", err)
	}
	currentSHA256, err := archiveQuestionReflectionSHA256(current)
	if err != nil {
		return ArchiveQuestionComparison{}, fmt.Errorf("current archive reflection: %w", err)
	}
	currentSummary := ArchiveQuestionVerificationSummary{
		SchemaVersion:    current.SchemaVersion,
		QuestionID:       current.QuestionID,
		Checked:          current.Summary.Checked,
		ReflectionSHA256: currentSHA256,
	}
	return compareVerifiedArchiveQuestionReports(older, olderSummary, current, currentSummary), nil
}

func compareVerifiedArchiveQuestionReports(
	older ArchiveQuestionReport,
	olderSummary ArchiveQuestionVerificationSummary,
	newer ArchiveQuestionReport,
	newerSummary ArchiveQuestionVerificationSummary,
) ArchiveQuestionComparison {
	comparison := ArchiveQuestionComparison{
		SchemaVersion:         archiveQuestionComparisonSchemaVersion,
		ComparisonID:          archiveQuestionComparisonID,
		ComparisonQuestion:    archiveQuestionComparisonText,
		QuestionID:            older.QuestionID,
		Question:              older.Question,
		OlderReflectionSHA256: olderSummary.ReflectionSHA256,
		NewerReflectionSHA256: newerSummary.ReflectionSHA256,
	}
	olderStates := archiveQuestionStates(older)
	newerStates := archiveQuestionStates(newer)
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
	return comparison
}

// CompareArchiveQuestionHistory verifies at least two saved reflections and
// compares each adjacent pair in caller-supplied order. Every report is
// verified once, and a question mismatch fails the whole operation.
func CompareArchiveQuestionHistory(reportPaths []string) (ArchiveQuestionTransitionHistory, error) {
	if len(reportPaths) < 2 {
		return ArchiveQuestionTransitionHistory{}, errors.New("at least two archive reflection reports are required")
	}

	reports := make([]ArchiveQuestionReport, len(reportPaths))
	summaries := make([]ArchiveQuestionVerificationSummary, len(reportPaths))
	history := ArchiveQuestionTransitionHistory{
		SchemaVersion:   archiveQuestionComparisonSchemaVersion,
		HistoryID:       archiveQuestionTransitionHistoryID,
		HistoryQuestion: archiveQuestionTransitionHistoryText,
		OrderBasis:      "caller",
		Snapshots:       len(reportPaths),
		Transitions:     make([]ArchiveQuestionTransition, 0, len(reportPaths)-1),
	}
	for index, reportPath := range reportPaths {
		report, summary, err := readVerifiedArchiveQuestionReport(reportPath)
		if err != nil {
			return ArchiveQuestionTransitionHistory{}, fmt.Errorf("reflection %d: %w", index+1, err)
		}
		reports[index] = report
		summaries[index] = summary
		if index == 0 {
			history.QuestionID = report.QuestionID
			history.Question = report.Question
		} else if report.QuestionID != history.QuestionID || report.Question != history.Question {
			return ArchiveQuestionTransitionHistory{}, fmt.Errorf("reflection %d: archive reflection questions do not match", index+1)
		}
	}
	for index := 1; index < len(reports); index++ {
		comparison := compareVerifiedArchiveQuestionReports(
			reports[index-1],
			summaries[index-1],
			reports[index],
			summaries[index],
		)
		history.Transitions = append(history.Transitions, ArchiveQuestionTransition{
			FromReflectionSHA256: comparison.OlderReflectionSHA256,
			ToReflectionSHA256:   comparison.NewerReflectionSHA256,
			Result:               comparison.Result,
			Compared:             comparison.Compared,
			Changed:              comparison.Changed,
			FromOnly:             comparison.OlderOnly,
			ToOnly:               comparison.NewerOnly,
		})
	}
	return history, nil
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
