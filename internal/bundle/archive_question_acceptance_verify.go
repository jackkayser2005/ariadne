package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const archiveQuestionTransitionHistoryAcceptanceSchemaVersion = 1

// ArchiveQuestionTransitionHistoryAcceptanceRecord binds one selected fixed
// question to a verified question round and answer receipt. It records safe
// identities only and does not prove that a UI driver performed the selection.
type ArchiveQuestionTransitionHistoryAcceptanceRecord struct {
	SchemaVersion           int    `json:"schema_version"`
	TransitionHistorySHA256 string `json:"transition_history_sha256"`
	QuestionRoundSHA256     string `json:"question_round_sha256"`
	QuestionID              string `json:"question_id"`
	ReceiptSHA256           string `json:"receipt_sha256"`
}

// ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary describes a
// structurally valid acceptance record and its canonical content identity.
type ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary struct {
	SchemaVersion           int    `json:"schema_version"`
	TransitionHistorySHA256 string `json:"transition_history_sha256"`
	QuestionRoundSHA256     string `json:"question_round_sha256"`
	QuestionID              string `json:"question_id"`
	ReceiptSHA256           string `json:"receipt_sha256"`
	AcceptanceSHA256        string `json:"acceptance_sha256"`
}

// ArchiveQuestionTransitionHistoryAcceptanceRecordSHA256 returns the
// canonical identity of one valid raw-value-free acceptance record.
func ArchiveQuestionTransitionHistoryAcceptanceRecordSHA256(record ArchiveQuestionTransitionHistoryAcceptanceRecord) (string, error) {
	if err := validateArchiveQuestionTransitionHistoryAcceptanceRecord(record); err != nil {
		return "", fmt.Errorf("archive question acceptance record: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("canonicalize acceptance record: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyArchiveQuestionTransitionHistoryAcceptanceRecord checks a saved
// acceptance record without reopening its round or receipt. It verifies the
// record contract and canonical identity, not the UI interaction itself.
func VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(recordPath string) (ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
	if strings.TrimSpace(recordPath) == "" {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("archive question acceptance record path is required")
	}
	data, err := readFileBounded(recordPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, fmt.Errorf("archive question acceptance record: %w", err)
	}
	record, err := decodeArchiveQuestionTransitionHistoryAcceptanceRecord(data)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, fmt.Errorf("archive question acceptance record: %w", err)
	}
	acceptanceSHA256, err := ArchiveQuestionTransitionHistoryAcceptanceRecordSHA256(record)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, err
	}
	return archiveQuestionTransitionHistoryAcceptanceVerificationSummary(record, acceptanceSHA256), nil
}

func decodeArchiveQuestionTransitionHistoryAcceptanceRecord(data []byte) (ArchiveQuestionTransitionHistoryAcceptanceRecord, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceRecord{}, err
	}
	var record ArchiveQuestionTransitionHistoryAcceptanceRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceRecord{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionTransitionHistoryAcceptanceRecord{}, errors.New("trailing data")
	}
	if err := validateArchiveQuestionTransitionHistoryAcceptanceRecordJSON(data); err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceRecord{}, err
	}
	return record, nil
}

func validateArchiveQuestionTransitionHistoryAcceptanceRecordJSON(data []byte) error {
	_, err := archiveQuestionObject(data, []string{
		"schema_version",
		"transition_history_sha256",
		"question_round_sha256",
		"question_id",
		"receipt_sha256",
	}, nil)
	return err
}

func validateArchiveQuestionTransitionHistoryAcceptanceRecord(record ArchiveQuestionTransitionHistoryAcceptanceRecord) error {
	if record.SchemaVersion != archiveQuestionTransitionHistoryAcceptanceSchemaVersion {
		return errors.New("unsupported schema_version")
	}
	if !validDigest(record.TransitionHistorySHA256) {
		return errors.New("transition_history_sha256 is invalid")
	}
	if !validDigest(record.QuestionRoundSHA256) {
		return errors.New("question_round_sha256 is invalid")
	}
	if _, ok := archiveQuestionTransitionHistoryQuestionForID(record.QuestionID); !ok {
		return errors.New("question ID is invalid")
	}
	if !validDigest(record.ReceiptSHA256) {
		return errors.New("receipt_sha256 is invalid")
	}
	return nil
}

// SaveArchiveQuestionTransitionHistoryAcceptanceRecord verifies a retained
// question round and receipt, confirms that they describe the same fixed
// question and history, and writes their identity binding without overwrite.
func SaveArchiveQuestionTransitionHistoryAcceptanceRecord(roundPath, receiptPath, recordPath string) (ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
	if strings.TrimSpace(recordPath) == "" {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("archive question acceptance record path is required")
	}
	round, roundSummary, err := readArchiveQuestionTransitionHistoryQuestionRound(roundPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, fmt.Errorf("question round: %w", err)
	}
	receipt, receiptSummary, err := readArchiveQuestionTransitionHistoryAnswerReceipt(receiptPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, fmt.Errorf("answer receipt: %w", err)
	}
	if roundSummary.TransitionHistorySHA256 != receiptSummary.TransitionHistorySHA256 {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("question round and answer receipt history identities do not match")
	}
	matched := false
	for _, question := range round.Questions {
		if question.QuestionID != receipt.QuestionID {
			continue
		}
		matched = true
		if question.Result != receipt.Result {
			return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("question round and answer receipt results do not match")
		}
		break
	}
	if !matched {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, errors.New("answer receipt question is not present in question round")
	}
	record := ArchiveQuestionTransitionHistoryAcceptanceRecord{
		SchemaVersion:           archiveQuestionTransitionHistoryAcceptanceSchemaVersion,
		TransitionHistorySHA256: roundSummary.TransitionHistorySHA256,
		QuestionRoundSHA256:     roundSummary.RoundSHA256,
		QuestionID:              receipt.QuestionID,
		ReceiptSHA256:           receiptSummary.ReceiptSHA256,
	}
	acceptanceSHA256, err := ArchiveQuestionTransitionHistoryAcceptanceRecordSHA256(record)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, fmt.Errorf("encode acceptance record: %w", err)
	}
	if err := writeExclusive(recordPath, append(data, '\n')); err != nil {
		return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{}, err
	}
	return archiveQuestionTransitionHistoryAcceptanceVerificationSummary(record, acceptanceSHA256), nil
}

func archiveQuestionTransitionHistoryAcceptanceVerificationSummary(record ArchiveQuestionTransitionHistoryAcceptanceRecord, acceptanceSHA256 string) ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary {
	return ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary{
		SchemaVersion:           record.SchemaVersion,
		TransitionHistorySHA256: record.TransitionHistorySHA256,
		QuestionRoundSHA256:     record.QuestionRoundSHA256,
		QuestionID:              record.QuestionID,
		ReceiptSHA256:           record.ReceiptSHA256,
		AcceptanceSHA256:        acceptanceSHA256,
	}
}
