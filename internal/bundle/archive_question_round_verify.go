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

// ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary describes
// one structurally valid saved question round. It does not prove the
// underlying evidence or the historical interpretation of the supplied order.
type ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary struct {
	SchemaVersion           int    `json:"schema_version"`
	TransitionHistorySHA256 string `json:"transition_history_sha256"`
	Questions               int    `json:"questions"`
	RoundSHA256             string `json:"round_sha256"`
}

// ArchiveQuestionTransitionHistoryQuestionRoundSHA256 returns the canonical
// identity of one valid raw-value-free question round.
func ArchiveQuestionTransitionHistoryQuestionRoundSHA256(round ArchiveQuestionTransitionHistoryQuestionRoundAnswer) (string, error) {
	if err := validateArchiveQuestionTransitionHistoryQuestionRound(round); err != nil {
		return "", fmt.Errorf("archive question round: %w", err)
	}
	return archiveQuestionTransitionHistoryQuestionRoundSHA256(round)
}

// VerifyArchiveQuestionTransitionHistoryQuestionRound checks a saved
// raw-value-free question round without requiring its source history. It
// validates the fixed question catalog and canonical identity, not the
// underlying evidence or the history referenced by its digest.
func VerifyArchiveQuestionTransitionHistoryQuestionRound(roundPath string) (ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
	_, summary, err := readArchiveQuestionTransitionHistoryQuestionRound(roundPath)
	return summary, err
}

func readArchiveQuestionTransitionHistoryQuestionRound(roundPath string) (ArchiveQuestionTransitionHistoryQuestionRoundAnswer, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, errors.New("archive question round path is required")
	}
	data, err := readFileBounded(roundPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, fmt.Errorf("archive question round: %w", err)
	}
	round, err := decodeArchiveQuestionTransitionHistoryQuestionRound(data)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, fmt.Errorf("archive question round: %w", err)
	}
	roundSHA256, err := ArchiveQuestionTransitionHistoryQuestionRoundSHA256(round)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, err
	}
	return round, ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{
		SchemaVersion:           round.SchemaVersion,
		TransitionHistorySHA256: round.TransitionHistorySHA256,
		Questions:               len(round.Questions),
		RoundSHA256:             roundSHA256,
	}, nil
}

func decodeArchiveQuestionTransitionHistoryQuestionRound(data []byte) (ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, err
	}
	var round ArchiveQuestionTransitionHistoryQuestionRoundAnswer
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&round); err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, errors.New("trailing data")
	}
	if err := validateArchiveQuestionTransitionHistoryQuestionRoundJSON(data); err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, err
	}
	return round, nil
}

func validateArchiveQuestionTransitionHistoryQuestionRoundJSON(data []byte) error {
	root, err := archiveQuestionObject(data, []string{
		"schema_version",
		"transition_history_sha256",
		"questions",
	}, nil)
	if err != nil {
		return err
	}
	questions, err := archiveQuestionArray(root["questions"], "questions")
	if err != nil {
		return err
	}
	for _, rawQuestion := range questions {
		if _, err := archiveQuestionObject(rawQuestion, []string{
			"question_id", "question", "result",
		}, nil); err != nil {
			return fmt.Errorf("question: %w", err)
		}
	}
	return nil
}

func validateArchiveQuestionTransitionHistoryQuestionRound(round ArchiveQuestionTransitionHistoryQuestionRoundAnswer) error {
	if round.SchemaVersion != archiveQuestionTransitionHistoryQuestionRoundSchemaVersion {
		return errors.New("unsupported schema_version")
	}
	if !validDigest(round.TransitionHistorySHA256) {
		return errors.New("transition_history_sha256 is invalid")
	}
	questions := ArchiveQuestionTransitionHistoryQuestions()
	if len(round.Questions) != len(questions) {
		return errors.New("question count does not match catalog")
	}
	for index, item := range round.Questions {
		question := questions[index]
		if item.QuestionID != question.ID {
			return errors.New("question ID or order does not match catalog")
		}
		if item.Question != question.Text {
			return errors.New("question text does not match catalog")
		}
		if !validArchiveQuestionTransitionHistoryReceiptResult(item.QuestionID, item.Result) {
			return errors.New("question result is invalid")
		}
	}
	return nil
}

func archiveQuestionTransitionHistoryQuestionRoundSHA256(round ArchiveQuestionTransitionHistoryQuestionRoundAnswer) (string, error) {
	data, err := json.Marshal(round)
	if err != nil {
		return "", fmt.Errorf("canonicalize question round: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// SaveArchiveQuestionTransitionHistoryQuestionRound verifies a saved
// transition history, asks every fixed question, and writes the portable
// question round without overwriting an existing path.
func SaveArchiveQuestionTransitionHistoryQuestionRound(historyPath, roundPath string) (ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, errors.New("archive question round path is required")
	}
	round, err := AskArchiveQuestionTransitionHistoryQuestionRound(historyPath)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ArchiveQuestionTransitionHistoryQuestionRoundSHA256(round)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, fmt.Errorf("encode question round: %w", err)
	}
	data = append(data, '\n')
	if err := writeExclusive(roundPath, data); err != nil {
		return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, err
	}
	return ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{
		SchemaVersion:           round.SchemaVersion,
		TransitionHistorySHA256: round.TransitionHistorySHA256,
		Questions:               len(round.Questions),
		RoundSHA256:             roundSHA256,
	}, nil
}
