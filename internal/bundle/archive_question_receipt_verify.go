package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

// ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary describes
// a structurally valid saved answer receipt. It does not prove the underlying
// evidence or re-verify the transition history named by the receipt.
type ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary struct {
	SchemaVersion           int    `json:"schema_version"`
	QuestionID              string `json:"question_id"`
	Question                string `json:"question"`
	Result                  string `json:"result"`
	TransitionHistorySHA256 string `json:"transition_history_sha256"`
	ReceiptSHA256           string `json:"receipt_sha256"`
}

// ArchiveQuestionTransitionHistoryAnswerReceiptSHA256 returns the canonical
// identity of one valid raw-value-free answer receipt.
func ArchiveQuestionTransitionHistoryAnswerReceiptSHA256(receipt ArchiveQuestionTransitionHistoryAnswerReceipt) (string, error) {
	if err := validateArchiveQuestionTransitionHistoryAnswerReceipt(receipt); err != nil {
		return "", fmt.Errorf("archive question answer receipt: %w", err)
	}
	return archiveQuestionTransitionHistoryAnswerReceiptSHA256(receipt)
}

// VerifyArchiveQuestionTransitionHistoryAnswerReceipt checks a saved
// raw-value-free answer receipt without requiring its source history. It
// validates the receipt contract and canonical identity, not the underlying
// evidence or the history referenced by its digest.
func VerifyArchiveQuestionTransitionHistoryAnswerReceipt(receiptPath string) (ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
	_, summary, err := readArchiveQuestionTransitionHistoryAnswerReceipt(receiptPath)
	return summary, err
}

func readArchiveQuestionTransitionHistoryAnswerReceipt(receiptPath string) (ArchiveQuestionTransitionHistoryAnswerReceipt, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, errors.New("archive question answer receipt path is required")
	}
	data, err := readFileBounded(receiptPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, fmt.Errorf("archive question answer receipt: %w", err)
	}
	receipt, err := decodeArchiveQuestionTransitionHistoryAnswerReceipt(data)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, fmt.Errorf("archive question answer receipt: %w", err)
	}
	if err := validateArchiveQuestionTransitionHistoryAnswerReceipt(receipt); err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, fmt.Errorf("archive question answer receipt: %w", err)
	}
	receiptSHA256, err := archiveQuestionTransitionHistoryAnswerReceiptSHA256(receipt)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, fmt.Errorf("archive question answer receipt: %w", err)
	}
	return receipt, ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{
		SchemaVersion:           receipt.SchemaVersion,
		QuestionID:              receipt.QuestionID,
		Question:                receipt.Question,
		Result:                  receipt.Result,
		TransitionHistorySHA256: receipt.TransitionHistorySHA256,
		ReceiptSHA256:           receiptSHA256,
	}, nil
}

func decodeArchiveQuestionTransitionHistoryAnswerReceipt(data []byte) (ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, err
	}
	var receipt ArchiveQuestionTransitionHistoryAnswerReceipt
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, errors.New("trailing data")
	}
	if err := validateArchiveQuestionTransitionHistoryAnswerReceiptJSON(data); err != nil {
		return ArchiveQuestionTransitionHistoryAnswerReceipt{}, err
	}
	return receipt, nil
}

func validateArchiveQuestionTransitionHistoryAnswerReceiptJSON(data []byte) error {
	root, err := archiveQuestionObject(data, []string{
		"schema_version",
		"question_id",
		"question",
		"result",
		"transition_history_sha256",
		"answer",
	}, nil)
	if err != nil {
		return err
	}
	var questionID string
	if err := json.Unmarshal(root["question_id"], &questionID); err != nil {
		return fmt.Errorf("question_id: %w", err)
	}
	if err := validateArchiveQuestionTransitionHistoryAnswerJSON(root["answer"], questionID); err != nil {
		return fmt.Errorf("answer: %w", err)
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryAnswerJSON(data []byte, questionID string) error {
	switch questionID {
	case archiveQuestionTransitionHistoryID:
		answer, err := archiveQuestionObject(data, []string{
			"schema_version", "question_id", "question", "result",
			"transition_history_sha256", "transitions", "changed_transitions",
			"incomparable_transitions", "changed_entries",
		}, nil)
		if err != nil {
			return err
		}
		if _, err := archiveQuestionArray(answer["changed_transitions"], "changed_transitions"); err != nil {
			return err
		}
		if _, err := archiveQuestionArray(answer["incomparable_transitions"], "incomparable_transitions"); err != nil {
			return err
		}
		if err := validateArchiveQuestionTransitionHistoryChangedEntriesJSON(answer["changed_entries"]); err != nil {
			return err
		}
		var typed ArchiveQuestionTransitionHistoryAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(data, &typed); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryAnswer(typed)
	case archiveQuestionTransitionHistoryRepeatedQuestionID:
		answer, err := archiveQuestionObject(data, []string{
			"schema_version", "question_id", "question", "result",
			"transition_history_sha256", "transitions", "repeated_entries",
		}, nil)
		if err != nil {
			return err
		}
		repeatedEntries, err := archiveQuestionArray(answer["repeated_entries"], "repeated_entries")
		if err != nil {
			return err
		}
		for _, rawEntry := range repeatedEntries {
			entry, err := archiveQuestionObject(rawEntry, []string{"directory", "changes"}, nil)
			if err != nil {
				return fmt.Errorf("repeated entry: %w", err)
			}
			if err := validateArchiveQuestionTransitionHistoryChangedEntriesJSON(entry["changes"]); err != nil {
				return fmt.Errorf("repeated entry changes: %w", err)
			}
		}
		var typed ArchiveQuestionTransitionHistoryRepeatedAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(data, &typed); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryRepeatedAnswer(typed)
	case archiveQuestionTransitionHistorySnapshotQuestionID:
		answer, err := archiveQuestionObject(data, []string{
			"schema_version", "question_id", "question", "result",
			"transition_history_sha256", "snapshots", "snapshot_summaries",
		}, nil)
		if err != nil {
			return err
		}
		summaries, err := archiveQuestionArray(answer["snapshot_summaries"], "snapshot_summaries")
		if err != nil {
			return err
		}
		for _, rawSummary := range summaries {
			if _, err := archiveQuestionObject(rawSummary, []string{
				"reflection_sha256", "observed", "unknown", "unavailable", "checked",
			}, nil); err != nil {
				return fmt.Errorf("snapshot summary: %w", err)
			}
		}
		var typed ArchiveQuestionTransitionHistorySnapshotAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(data, &typed); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistorySnapshotAnswer(typed)
	case archiveQuestionTransitionHistorySummaryQuestionID:
		answer, err := archiveQuestionObject(data, []string{
			"schema_version", "question_id", "question", "result",
			"transition_history_sha256", "transitions", "changed_transitions",
		}, nil)
		if err != nil {
			return err
		}
		if _, err = archiveQuestionArray(answer["changed_transitions"], "changed_transitions"); err != nil {
			return err
		}
		var typed ArchiveQuestionTransitionHistorySummaryAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(data, &typed); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistorySummaryAnswer(typed)
	default:
		return errors.New("question ID is invalid")
	}
}

func validateArchiveQuestionTransitionHistoryChangedEntriesJSON(data []byte) error {
	entries, err := archiveQuestionArray(data, "changed_entries")
	if err != nil {
		return err
	}
	for _, rawEntry := range entries {
		if _, err := archiveQuestionObject(rawEntry, []string{
			"transition", "from_reflection_sha256", "to_reflection_sha256",
			"directory", "older_state", "newer_state",
		}, nil); err != nil {
			return fmt.Errorf("changed entry: %w", err)
		}
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryAnswerMetadataValues(
	schemaVersion int,
	questionID, question, result, historySHA256, expectedQuestionID string,
) error {
	if schemaVersion != archiveQuestionTransitionHistoryAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistoryRepeatedAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistorySnapshotAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistorySummaryAnswerSchemaVersion {
		return errors.New("answer schema_version is invalid")
	}
	if questionID != expectedQuestionID {
		return errors.New("answer question ID is invalid")
	}
	expectedQuestion, ok := archiveQuestionTransitionHistoryQuestionForID(expectedQuestionID)
	if !ok || question != expectedQuestion.Text {
		return errors.New("answer question text is invalid")
	}
	if !validArchiveQuestionTransitionHistoryReceiptResult(expectedQuestionID, result) {
		return errors.New("answer result is invalid")
	}
	if !validDigest(historySHA256) {
		return errors.New("answer transition_history_sha256 is invalid")
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryAnswer(answer ArchiveQuestionTransitionHistoryAnswer) error {
	if err := validateArchiveQuestionTransitionHistoryAnswerMetadataValues(
		answer.SchemaVersion,
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		archiveQuestionTransitionHistoryID,
	); err != nil {
		return err
	}
	if answer.Transitions < 1 {
		return errors.New("answer transitions is invalid")
	}
	if err := validateArchiveQuestionTransitionHistoryTransitionIndexes(answer.ChangedTransitions, answer.Transitions, "changed_transitions"); err != nil {
		return err
	}
	if err := validateArchiveQuestionTransitionHistoryTransitionIndexes(answer.IncomparableTransitions, answer.Transitions, "incomparable_transitions"); err != nil {
		return err
	}
	incomparable := make(map[int]struct{}, len(answer.IncomparableTransitions))
	for _, index := range answer.IncomparableTransitions {
		incomparable[index] = struct{}{}
	}
	for _, index := range answer.ChangedTransitions {
		if _, ok := incomparable[index]; ok {
			return errors.New("answer transition indexes overlap")
		}
	}
	expectedResult := "same"
	if len(answer.ChangedTransitions) > 0 {
		expectedResult = "changed"
	} else if len(answer.IncomparableTransitions) > 0 {
		expectedResult = "incomparable"
	}
	if answer.Result != expectedResult {
		return errors.New("answer result does not match detail")
	}
	changed := make(map[int]struct{}, len(answer.ChangedTransitions))
	for _, index := range answer.ChangedTransitions {
		changed[index] = struct{}{}
	}
	entriesByTransition := make(map[int]int, len(changed))
	reflectionByTransition := make(map[int][2]string, len(changed))
	lastTransition := 0
	lastDirectory := ""
	for _, entry := range answer.ChangedEntries {
		if _, ok := changed[entry.Transition]; !ok {
			return errors.New("answer changed entry transition is invalid")
		}
		if entry.Transition < lastTransition || (entry.Transition == lastTransition && entry.Directory <= lastDirectory) {
			return errors.New("answer changed entry ordering is invalid")
		}
		if !validDigest(entry.FromReflectionSHA256) || !validDigest(entry.ToReflectionSHA256) {
			return errors.New("answer changed entry reflection identity is invalid")
		}
		identity := [2]string{entry.FromReflectionSHA256, entry.ToReflectionSHA256}
		if previous, ok := reflectionByTransition[entry.Transition]; ok && previous != identity {
			return errors.New("answer transition reflection identity is inconsistent")
		}
		reflectionByTransition[entry.Transition] = identity
		if !validArchiveEntryName(entry.Directory) {
			return errors.New("answer changed entry directory is invalid")
		}
		if !validArchiveQuestionState(entry.OlderState) || !validArchiveQuestionState(entry.NewerState) || entry.OlderState == entry.NewerState {
			return errors.New("answer changed entry state is invalid")
		}
		entriesByTransition[entry.Transition]++
		lastTransition = entry.Transition
		lastDirectory = entry.Directory
	}
	for _, index := range answer.ChangedTransitions {
		if entriesByTransition[index] == 0 {
			return errors.New("answer changed entry count does not match changed transitions")
		}
	}
	if len(answer.ChangedEntries) > 0 && len(answer.ChangedTransitions) == 0 {
		return errors.New("answer changed entry count does not match changed transitions")
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryTransitionIndexes(indexes []int, transitions int, field string) error {
	previous := 0
	for _, index := range indexes {
		if index < 1 || index > transitions || index <= previous {
			return fmt.Errorf("answer %s ordering or range is invalid", field)
		}
		previous = index
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer ArchiveQuestionTransitionHistoryRepeatedAnswer) error {
	if err := validateArchiveQuestionTransitionHistoryAnswerMetadataValues(
		answer.SchemaVersion,
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		archiveQuestionTransitionHistoryRepeatedQuestionID,
	); err != nil {
		return err
	}
	if answer.Transitions < 1 {
		return errors.New("answer transitions is invalid")
	}
	if answer.Result != "repeated" && len(answer.RepeatedEntries) != 0 {
		return errors.New("answer repeated entries do not match result")
	}
	if answer.Result == "repeated" && len(answer.RepeatedEntries) == 0 {
		return errors.New("answer repeated entries do not match result")
	}
	previousDirectory := ""
	reflectionByTransition := make(map[int][2]string)
	for _, entry := range answer.RepeatedEntries {
		if !validArchiveEntryName(entry.Directory) || entry.Directory <= previousDirectory {
			return errors.New("answer repeated entry directory ordering is invalid")
		}
		if len(entry.Changes) < 2 {
			return errors.New("answer repeated entry needs multiple changes")
		}
		previousTransition := 0
		for _, change := range entry.Changes {
			if change.Directory != entry.Directory || change.Transition < 1 || change.Transition > answer.Transitions || change.Transition <= previousTransition {
				return errors.New("answer repeated entry changes are invalid")
			}
			if !validDigest(change.FromReflectionSHA256) || !validDigest(change.ToReflectionSHA256) {
				return errors.New("answer repeated entry reflection identity is invalid")
			}
			identity := [2]string{change.FromReflectionSHA256, change.ToReflectionSHA256}
			if previous, ok := reflectionByTransition[change.Transition]; ok && previous != identity {
				return errors.New("answer transition reflection identity is inconsistent")
			}
			reflectionByTransition[change.Transition] = identity
			if !validArchiveQuestionState(change.OlderState) || !validArchiveQuestionState(change.NewerState) || change.OlderState == change.NewerState {
				return errors.New("answer repeated entry state is invalid")
			}
			previousTransition = change.Transition
		}
		previousDirectory = entry.Directory
	}
	return nil
}

func validateArchiveQuestionTransitionHistorySnapshotAnswer(answer ArchiveQuestionTransitionHistorySnapshotAnswer) error {
	if err := validateArchiveQuestionTransitionHistoryAnswerMetadataValues(
		answer.SchemaVersion,
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		archiveQuestionTransitionHistorySnapshotQuestionID,
	); err != nil {
		return err
	}
	if answer.Snapshots < 2 {
		return errors.New("answer snapshots is invalid")
	}
	if answer.Result == "unavailable" {
		if len(answer.SnapshotSummaries) != 0 {
			return errors.New("answer snapshot summaries do not match result")
		}
		return nil
	}
	if len(answer.SnapshotSummaries) != answer.Snapshots {
		return errors.New("answer snapshot summary count does not match snapshots")
	}
	for index, summary := range answer.SnapshotSummaries {
		if err := validateArchiveQuestionTransitionSnapshot(summary); err != nil {
			return fmt.Errorf("answer snapshot %d: %w", index+1, err)
		}
	}
	return nil
}

func validateArchiveQuestionTransitionHistorySummaryAnswer(answer ArchiveQuestionTransitionHistorySummaryAnswer) error {
	if err := validateArchiveQuestionTransitionHistoryAnswerMetadataValues(
		answer.SchemaVersion,
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		archiveQuestionTransitionHistorySummaryQuestionID,
	); err != nil {
		return err
	}
	if answer.Transitions < 1 {
		return errors.New("answer transitions is invalid")
	}
	if err := validateArchiveQuestionTransitionHistoryTransitionIndexes(answer.ChangedTransitions, answer.Transitions, "changed_transitions"); err != nil {
		return err
	}
	expectedResult := "same"
	if len(answer.ChangedTransitions) > 0 {
		expectedResult = "changed"
	}
	if answer.Result != "unavailable" && answer.Result != expectedResult {
		return errors.New("answer result does not match detail")
	}
	if answer.Result == "unavailable" && len(answer.ChangedTransitions) != 0 {
		return errors.New("answer changed transitions do not match result")
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryAnswerReceipt(receipt ArchiveQuestionTransitionHistoryAnswerReceipt) error {
	if receipt.SchemaVersion != archiveQuestionTransitionHistoryAnswerReceiptSchemaVersion {
		return errors.New("unsupported schema_version")
	}
	question, ok := archiveQuestionTransitionHistoryQuestionForID(receipt.QuestionID)
	if !ok {
		return errors.New("question ID is invalid")
	}
	if receipt.Question != question.Text {
		return errors.New("question text does not match catalog")
	}
	if !validDigest(receipt.TransitionHistorySHA256) {
		return errors.New("transition_history_sha256 is invalid")
	}
	if len(bytes.TrimSpace(receipt.Answer)) == 0 || bytes.Equal(bytes.TrimSpace(receipt.Answer), []byte("null")) {
		return errors.New("answer is required")
	}

	switch receipt.QuestionID {
	case archiveQuestionTransitionHistoryID:
		var answer ArchiveQuestionTransitionHistoryAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		if err := validateArchiveQuestionTransitionHistoryAnswer(answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistoryRepeatedQuestionID:
		var answer ArchiveQuestionTransitionHistoryRepeatedAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		if err := validateArchiveQuestionTransitionHistoryRepeatedAnswer(answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistorySnapshotQuestionID:
		var answer ArchiveQuestionTransitionHistorySnapshotAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		if err := validateArchiveQuestionTransitionHistorySnapshotAnswer(answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistorySummaryQuestionID:
		var answer ArchiveQuestionTransitionHistorySummaryAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		if err := validateArchiveQuestionTransitionHistorySummaryAnswer(answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	default:
		return errors.New("question ID is invalid")
	}
}

func decodeArchiveQuestionTransitionHistoryReceiptAnswer(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode answer: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("answer has trailing data")
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(
	receipt ArchiveQuestionTransitionHistoryAnswerReceipt,
	schemaVersion int,
	questionID, question, result, historySHA256 string,
) error {
	if schemaVersion != archiveQuestionTransitionHistoryAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistoryRepeatedAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistorySnapshotAnswerSchemaVersion &&
		schemaVersion != archiveQuestionTransitionHistorySummaryAnswerSchemaVersion {
		return errors.New("answer schema_version is invalid")
	}
	if questionID != receipt.QuestionID || question != receipt.Question {
		return errors.New("answer question does not match receipt")
	}
	if !validArchiveQuestionTransitionHistoryReceiptResult(receipt.QuestionID, result) {
		return errors.New("answer result is invalid")
	}
	if result != receipt.Result {
		return errors.New("answer result does not match receipt")
	}
	if !validDigest(historySHA256) || historySHA256 != receipt.TransitionHistorySHA256 {
		return errors.New("answer transition_history_sha256 does not match receipt")
	}
	return nil
}

func validArchiveQuestionTransitionHistoryReceiptResult(questionID, result string) bool {
	switch questionID {
	case archiveQuestionTransitionHistoryID:
		return result == "same" || result == "changed" || result == "incomparable"
	case archiveQuestionTransitionHistoryRepeatedQuestionID:
		return result == "unavailable" || result == "none" || result == "repeated"
	case archiveQuestionTransitionHistorySnapshotQuestionID:
		return result == "unavailable" || result == "available"
	case archiveQuestionTransitionHistorySummaryQuestionID:
		return result == "unavailable" || result == "same" || result == "changed"
	default:
		return false
	}
}
