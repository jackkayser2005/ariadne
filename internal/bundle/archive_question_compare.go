package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	archiveQuestionComparisonSchemaVersion                      = 2
	archiveQuestionTransitionHistoryLegacySchemaVersion         = 1
	archiveQuestionTransitionHistorySchemaVersion               = 2
	archiveQuestionTransitionHistoryAnswerSchemaVersion         = 1
	archiveQuestionTransitionHistoryRepeatedAnswerSchemaVersion = 1
	archiveQuestionComparisonID                                 = "answer-state-change"
	archiveQuestionComparisonText                               = "Did the bounded answer state change between these saved reflection snapshots?"
	archiveQuestionTransitionHistoryID                          = "answer-state-transitions"
	archiveQuestionTransitionHistoryText                        = "At which supplied boundaries did the bounded answer state change?"
	archiveQuestionTransitionHistoryRepeatedQuestionID          = "answer-state-repeated-changes"
	archiveQuestionTransitionHistoryRepeatedQuestionText        = "Did any safe archive entry change at more than one supplied boundary?"
)

// ArchiveQuestionComparison is a raw-value-free comparison of two verified
// archive-question reflections. It describes answer-state structure only; it
// does not infer a trend or prove the underlying evidence.
type ArchiveQuestionComparison struct {
	SchemaVersion         int                          `json:"schema_version"`
	ComparisonID          string                       `json:"comparison_id"`
	ComparisonQuestion    string                       `json:"comparison_question"`
	QuestionID            string                       `json:"question_id"`
	Question              string                       `json:"question"`
	Result                string                       `json:"result"`
	OlderReflectionSHA256 string                       `json:"older_reflection_sha256"`
	NewerReflectionSHA256 string                       `json:"newer_reflection_sha256"`
	Compared              int                          `json:"compared"`
	Changed               int                          `json:"changed"`
	OlderOnly             int                          `json:"older_only"`
	NewerOnly             int                          `json:"newer_only"`
	StateChanges          []ArchiveQuestionStateChange `json:"state_changes"`
}

// ArchiveQuestionStateChange identifies one common archive entry whose
// bounded answer state changed between two verified reflections.
type ArchiveQuestionStateChange struct {
	Directory  string `json:"directory"`
	OlderState string `json:"older_state"`
	NewerState string `json:"newer_state"`
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
	FromReflectionSHA256 string                       `json:"from_reflection_sha256"`
	ToReflectionSHA256   string                       `json:"to_reflection_sha256"`
	Result               string                       `json:"result"`
	Compared             int                          `json:"compared"`
	Changed              int                          `json:"changed"`
	FromOnly             int                          `json:"from_only"`
	ToOnly               int                          `json:"to_only"`
	StateChanges         []ArchiveQuestionStateChange `json:"state_changes,omitempty"`
}

// ArchiveQuestionTransitionHistoryAnswer is a raw-value-free answer derived
// from one verified transition history. It describes the ledger structure,
// not the underlying evidence or chronology.
type ArchiveQuestionTransitionHistoryAnswer struct {
	SchemaVersion           int                                      `json:"schema_version"`
	QuestionID              string                                   `json:"question_id"`
	Question                string                                   `json:"question"`
	Result                  string                                   `json:"result"`
	TransitionHistorySHA256 string                                   `json:"transition_history_sha256"`
	Transitions             int                                      `json:"transitions"`
	ChangedTransitions      []int                                    `json:"changed_transitions"`
	IncomparableTransitions []int                                    `json:"incomparable_transitions"`
	ChangedEntries          []ArchiveQuestionTransitionHistoryChange `json:"changed_entries"`
}

// ArchiveQuestionTransitionHistoryChange identifies one safe changed entry
// and the adjacent transition where it changed.
type ArchiveQuestionTransitionHistoryChange struct {
	Transition           int    `json:"transition"`
	FromReflectionSHA256 string `json:"from_reflection_sha256"`
	ToReflectionSHA256   string `json:"to_reflection_sha256"`
	Directory            string `json:"directory"`
	OlderState           string `json:"older_state"`
	NewerState           string `json:"newer_state"`
}

// ArchiveQuestionTransitionHistoryRepeatedAnswer is a raw-value-free answer
// about repeated state-change records in one verified transition history.
type ArchiveQuestionTransitionHistoryRepeatedAnswer struct {
	SchemaVersion           int                                              `json:"schema_version"`
	QuestionID              string                                           `json:"question_id"`
	Question                string                                           `json:"question"`
	Result                  string                                           `json:"result"`
	TransitionHistorySHA256 string                                           `json:"transition_history_sha256"`
	Transitions             int                                              `json:"transitions"`
	RepeatedEntries         []ArchiveQuestionTransitionHistoryRepeatedChange `json:"repeated_entries"`
}

// ArchiveQuestionTransitionHistoryRepeatedChange groups the safe state-change
// records for one archive entry when it changed at multiple boundaries.
type ArchiveQuestionTransitionHistoryRepeatedChange struct {
	Directory string                                   `json:"directory"`
	Changes   []ArchiveQuestionTransitionHistoryChange `json:"changes"`
}

// ArchiveQuestionTransitionHistoryQuestions returns the fixed questions
// available for a verified transition history in stable order.
func ArchiveQuestionTransitionHistoryQuestions() []Question {
	return []Question{
		{ID: archiveQuestionTransitionHistoryID, Text: archiveQuestionTransitionHistoryText},
		{ID: archiveQuestionTransitionHistoryRepeatedQuestionID, Text: archiveQuestionTransitionHistoryRepeatedQuestionText},
	}
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
		StateChanges:          make([]ArchiveQuestionStateChange, 0),
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
			comparison.StateChanges = append(comparison.StateChanges, ArchiveQuestionStateChange{
				Directory:  directory,
				OlderState: olderState,
				NewerState: newerState,
			})
		}
	}
	for directory := range newerStates {
		if _, ok := olderStates[directory]; !ok {
			comparison.NewerOnly++
		}
	}
	sort.Slice(comparison.StateChanges, func(i, j int) bool {
		return comparison.StateChanges[i].Directory < comparison.StateChanges[j].Directory
	})
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
		SchemaVersion:   archiveQuestionTransitionHistorySchemaVersion,
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
			StateChanges:         append([]ArchiveQuestionStateChange(nil), comparison.StateChanges...),
		})
	}
	return history, nil
}

// AnswerArchiveQuestionTransitionHistory answers the fixed question carried
// by a verified transition history. The history and SHA-256 must come from a
// successful verification call.
func AnswerArchiveQuestionTransitionHistory(history ArchiveQuestionTransitionHistory, historySHA256 string) ArchiveQuestionTransitionHistoryAnswer {
	changedTransitions := make([]int, 0)
	incomparableTransitions := make([]int, 0)
	for index, transition := range history.Transitions {
		if transition.Changed > 0 {
			changedTransitions = append(changedTransitions, index+1)
		}
		if transition.Result == "incomparable" {
			incomparableTransitions = append(incomparableTransitions, index+1)
		}
	}
	result := "same"
	if len(changedTransitions) > 0 {
		result = "changed"
	} else if len(incomparableTransitions) > 0 {
		result = "incomparable"
	}
	return ArchiveQuestionTransitionHistoryAnswer{
		SchemaVersion:           archiveQuestionTransitionHistoryAnswerSchemaVersion,
		QuestionID:              history.HistoryID,
		Question:                history.HistoryQuestion,
		Result:                  result,
		TransitionHistorySHA256: historySHA256,
		Transitions:             len(history.Transitions),
		ChangedTransitions:      changedTransitions,
		IncomparableTransitions: incomparableTransitions,
		ChangedEntries:          archiveQuestionTransitionHistoryChangedEntries(history),
	}
}

func archiveQuestionTransitionHistoryChangedEntries(history ArchiveQuestionTransitionHistory) []ArchiveQuestionTransitionHistoryChange {
	changedEntries := make([]ArchiveQuestionTransitionHistoryChange, 0)
	for index, transition := range history.Transitions {
		for _, stateChange := range transition.StateChanges {
			changedEntries = append(changedEntries, ArchiveQuestionTransitionHistoryChange{
				Transition:           index + 1,
				FromReflectionSHA256: transition.FromReflectionSHA256,
				ToReflectionSHA256:   transition.ToReflectionSHA256,
				Directory:            stateChange.Directory,
				OlderState:           stateChange.OlderState,
				NewerState:           stateChange.NewerState,
			})
		}
	}
	return changedEntries
}

// AnswerArchiveQuestionTransitionHistoryRepeated answers whether any safe
// archive entry changed at multiple supplied boundaries. Legacy histories do
// not contain the state-change records needed for this question.
func AnswerArchiveQuestionTransitionHistoryRepeated(history ArchiveQuestionTransitionHistory, historySHA256 string) ArchiveQuestionTransitionHistoryRepeatedAnswer {
	answer := ArchiveQuestionTransitionHistoryRepeatedAnswer{
		SchemaVersion:           archiveQuestionTransitionHistoryRepeatedAnswerSchemaVersion,
		QuestionID:              archiveQuestionTransitionHistoryRepeatedQuestionID,
		Question:                archiveQuestionTransitionHistoryRepeatedQuestionText,
		TransitionHistorySHA256: historySHA256,
		Transitions:             len(history.Transitions),
		RepeatedEntries:         make([]ArchiveQuestionTransitionHistoryRepeatedChange, 0),
	}
	if history.SchemaVersion != archiveQuestionTransitionHistorySchemaVersion {
		answer.Result = "unavailable"
		return answer
	}
	changesByDirectory := make(map[string][]ArchiveQuestionTransitionHistoryChange)
	for _, change := range archiveQuestionTransitionHistoryChangedEntries(history) {
		changesByDirectory[change.Directory] = append(changesByDirectory[change.Directory], change)
	}
	directories := make([]string, 0, len(changesByDirectory))
	for directory := range changesByDirectory {
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	for _, directory := range directories {
		changes := changesByDirectory[directory]
		if len(changes) > 1 {
			answer.RepeatedEntries = append(answer.RepeatedEntries, ArchiveQuestionTransitionHistoryRepeatedChange{
				Directory: directory,
				Changes:   changes,
			})
		}
	}
	if len(answer.RepeatedEntries) > 0 {
		answer.Result = "repeated"
	} else {
		answer.Result = "none"
	}
	return answer
}

// AskArchiveQuestionTransitionHistory verifies a saved transition history
// and answers its fixed raw-value-free question.
func AskArchiveQuestionTransitionHistory(historyPath string) (ArchiveQuestionTransitionHistoryAnswer, error) {
	history, summary, err := ReadArchiveQuestionTransitionHistory(historyPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAnswer{}, err
	}
	return AnswerArchiveQuestionTransitionHistory(history, summary.TransitionHistorySHA256), nil
}

// AskArchiveQuestionTransitionHistoryRepeated verifies a saved transition
// history and answers its repeated-change question.
func AskArchiveQuestionTransitionHistoryRepeated(historyPath string) (ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
	history, summary, err := ReadArchiveQuestionTransitionHistory(historyPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryRepeatedAnswer{}, err
	}
	return AnswerArchiveQuestionTransitionHistoryRepeated(history, summary.TransitionHistorySHA256), nil
}

// SaveArchiveQuestionTransitionHistory writes one validated transition ledger
// without overwriting an existing path.
func SaveArchiveQuestionTransitionHistory(reportPaths []string, historyPath string) (ArchiveQuestionTransitionVerificationSummary, error) {
	if strings.TrimSpace(historyPath) == "" {
		return ArchiveQuestionTransitionVerificationSummary{}, errors.New("archive question transition history path is required")
	}
	history, err := CompareArchiveQuestionHistory(reportPaths)
	if err != nil {
		return ArchiveQuestionTransitionVerificationSummary{}, err
	}
	if err := validateArchiveQuestionTransitionHistory(history); err != nil {
		return ArchiveQuestionTransitionVerificationSummary{}, err
	}
	historySHA256, err := archiveQuestionTransitionHistorySHA256(history)
	if err != nil {
		return ArchiveQuestionTransitionVerificationSummary{}, err
	}
	data, err := json.Marshal(history)
	if err != nil {
		return ArchiveQuestionTransitionVerificationSummary{}, err
	}
	data = append(data, '\n')
	if err := writeExclusive(historyPath, data); err != nil {
		return ArchiveQuestionTransitionVerificationSummary{}, err
	}
	return ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           history.SchemaVersion,
		HistoryID:               history.HistoryID,
		HistoryQuestion:         history.HistoryQuestion,
		QuestionID:              history.QuestionID,
		OrderBasis:              history.OrderBasis,
		Snapshots:               history.Snapshots,
		Transitions:             len(history.Transitions),
		TransitionHistorySHA256: historySHA256,
	}, nil
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
