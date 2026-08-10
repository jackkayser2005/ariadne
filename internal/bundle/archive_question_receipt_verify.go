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
		return validateArchiveQuestionTransitionHistoryChangedEntriesJSON(answer["changed_entries"])
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
		return nil
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
		return nil
	case archiveQuestionTransitionHistorySummaryQuestionID:
		answer, err := archiveQuestionObject(data, []string{
			"schema_version", "question_id", "question", "result",
			"transition_history_sha256", "transitions", "changed_transitions",
		}, nil)
		if err != nil {
			return err
		}
		_, err = archiveQuestionArray(answer["changed_transitions"], "changed_transitions")
		return err
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
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistoryRepeatedQuestionID:
		var answer ArchiveQuestionTransitionHistoryRepeatedAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistorySnapshotQuestionID:
		var answer ArchiveQuestionTransitionHistorySnapshotAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
			return err
		}
		return validateArchiveQuestionTransitionHistoryReceiptAnswerMetadata(receipt, answer.SchemaVersion, answer.QuestionID, answer.Question, answer.Result, answer.TransitionHistorySHA256)
	case archiveQuestionTransitionHistorySummaryQuestionID:
		var answer ArchiveQuestionTransitionHistorySummaryAnswer
		if err := decodeArchiveQuestionTransitionHistoryReceiptAnswer(receipt.Answer, &answer); err != nil {
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
