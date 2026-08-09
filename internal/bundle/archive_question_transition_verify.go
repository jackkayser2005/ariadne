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

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

// ArchiveQuestionTransitionVerificationSummary describes a structurally valid
// saved transition ledger. It does not prove the underlying evidence or the
// historical interpretation of the supplied order.
type ArchiveQuestionTransitionVerificationSummary struct {
	SchemaVersion           int    `json:"schema_version"`
	HistoryID               string `json:"history_id"`
	HistoryQuestion         string `json:"history_question"`
	QuestionID              string `json:"question_id"`
	OrderBasis              string `json:"order_basis"`
	Snapshots               int    `json:"snapshots"`
	Transitions             int    `json:"transitions"`
	TransitionHistorySHA256 string `json:"transition_history_sha256"`
}

// VerifyArchiveQuestionTransitionHistory checks a saved transition ledger
// without requiring its source reflection reports. It validates the derived
// contract, not the truth of the underlying evidence or chronology.
func VerifyArchiveQuestionTransitionHistory(historyPath string) (ArchiveQuestionTransitionVerificationSummary, error) {
	_, summary, err := readVerifiedArchiveQuestionTransitionHistory(historyPath)
	return summary, err
}

// ReadArchiveQuestionTransitionHistory returns a structurally verified,
// raw-value-free transition ledger and its content summary.
func ReadArchiveQuestionTransitionHistory(historyPath string) (ArchiveQuestionTransitionHistory, ArchiveQuestionTransitionVerificationSummary, error) {
	return readVerifiedArchiveQuestionTransitionHistory(historyPath)
}

func readVerifiedArchiveQuestionTransitionHistory(historyPath string) (ArchiveQuestionTransitionHistory, ArchiveQuestionTransitionVerificationSummary, error) {
	if strings.TrimSpace(historyPath) == "" {
		return ArchiveQuestionTransitionHistory{}, ArchiveQuestionTransitionVerificationSummary{}, errors.New("archive question transition history path is required")
	}
	data, err := readFileBounded(historyPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionTransitionHistory{}, ArchiveQuestionTransitionVerificationSummary{}, fmt.Errorf("archive question transition history: %w", err)
	}
	history, err := decodeArchiveQuestionTransitionHistory(data)
	if err != nil {
		return ArchiveQuestionTransitionHistory{}, ArchiveQuestionTransitionVerificationSummary{}, fmt.Errorf("archive question transition history: %w", err)
	}
	if err := validateArchiveQuestionTransitionHistory(history); err != nil {
		return ArchiveQuestionTransitionHistory{}, ArchiveQuestionTransitionVerificationSummary{}, fmt.Errorf("archive question transition history: %w", err)
	}
	historySHA256, err := archiveQuestionTransitionHistorySHA256(history)
	if err != nil {
		return ArchiveQuestionTransitionHistory{}, ArchiveQuestionTransitionVerificationSummary{}, fmt.Errorf("archive question transition history: %w", err)
	}
	return history, ArchiveQuestionTransitionVerificationSummary{
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

func decodeArchiveQuestionTransitionHistory(data []byte) (ArchiveQuestionTransitionHistory, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionTransitionHistory{}, err
	}
	var history ArchiveQuestionTransitionHistory
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return ArchiveQuestionTransitionHistory{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionTransitionHistory{}, errors.New("trailing data")
	}
	if err := validateArchiveQuestionTransitionHistoryJSON(data); err != nil {
		return ArchiveQuestionTransitionHistory{}, err
	}
	return history, nil
}

func validateArchiveQuestionTransitionHistoryJSON(data []byte) error {
	root, err := archiveQuestionObject(data, []string{
		"schema_version",
		"history_id",
		"history_question",
		"question_id",
		"question",
		"order_basis",
		"snapshots",
		"transitions",
	}, nil)
	if err != nil {
		return err
	}
	transitions, err := archiveQuestionArray(root["transitions"], "transitions")
	if err != nil {
		return err
	}
	for _, rawTransition := range transitions {
		transitionObject, err := archiveQuestionObject(rawTransition, []string{
			"from_reflection_sha256",
			"to_reflection_sha256",
			"result",
			"compared",
			"changed",
			"from_only",
			"to_only",
		}, []string{"state_changes"})
		if err != nil {
			return fmt.Errorf("transition: %w", err)
		}
		if stateChanges, ok := transitionObject["state_changes"]; ok {
			items, err := archiveQuestionArray(stateChanges, "state_changes")
			if err != nil {
				return fmt.Errorf("transition: %w", err)
			}
			for _, rawStateChange := range items {
				if _, err := archiveQuestionObject(rawStateChange, []string{"directory", "older_state", "newer_state"}, nil); err != nil {
					return fmt.Errorf("transition state change: %w", err)
				}
			}
		}
	}
	return nil
}

func validateArchiveQuestionTransitionHistory(history ArchiveQuestionTransitionHistory) error {
	if history.SchemaVersion != archiveQuestionTransitionHistoryLegacySchemaVersion && history.SchemaVersion != archiveQuestionTransitionHistorySchemaVersion {
		return errors.New("unsupported schema_version")
	}
	if history.HistoryID != archiveQuestionTransitionHistoryID {
		return errors.New("history ID is invalid")
	}
	if history.HistoryQuestion != archiveQuestionTransitionHistoryText {
		return errors.New("history question does not match contract")
	}
	catalogQuestion, ok := questionForID(history.QuestionID)
	if !ok {
		return errors.New("question ID is invalid")
	}
	if history.Question != catalogQuestion.Text {
		return errors.New("question text does not match catalog")
	}
	if history.OrderBasis != "caller" {
		return errors.New("order_basis is invalid")
	}
	if history.Snapshots < 2 {
		return errors.New("snapshots must be at least two")
	}
	if len(history.Transitions) != history.Snapshots-1 {
		return errors.New("transition count does not match snapshots")
	}
	for _, transition := range history.Transitions {
		if !validDigest(transition.FromReflectionSHA256) || !validDigest(transition.ToReflectionSHA256) {
			return errors.New("transition reflection identity is invalid")
		}
		if transition.Compared < 0 || transition.Changed < 0 || transition.FromOnly < 0 || transition.ToOnly < 0 || transition.Changed > transition.Compared {
			return errors.New("transition counts are invalid")
		}
		switch transition.Result {
		case "same":
			if transition.Changed != 0 || transition.FromOnly != 0 || transition.ToOnly != 0 {
				return errors.New("same transition counts are invalid")
			}
		case "changed":
			if transition.Changed == 0 || transition.FromOnly != 0 || transition.ToOnly != 0 {
				return errors.New("changed transition counts are invalid")
			}
		case "incomparable":
			if transition.FromOnly == 0 && transition.ToOnly == 0 {
				return errors.New("incomparable transition counts are invalid")
			}
		default:
			return errors.New("transition result is invalid")
		}
		if history.SchemaVersion == archiveQuestionTransitionHistoryLegacySchemaVersion {
			if len(transition.StateChanges) != 0 {
				return errors.New("legacy transition state changes are unsupported")
			}
			continue
		}
		if len(transition.StateChanges) != transition.Changed {
			return errors.New("transition state change count does not match changed count")
		}
		previousDirectory := ""
		for _, stateChange := range transition.StateChanges {
			if !validArchiveEntryName(stateChange.Directory) || stateChange.Directory <= previousDirectory {
				return errors.New("transition state change directory ordering is invalid")
			}
			if !validArchiveQuestionState(stateChange.OlderState) || !validArchiveQuestionState(stateChange.NewerState) {
				return errors.New("transition state change state is invalid")
			}
			if stateChange.OlderState == stateChange.NewerState {
				return errors.New("transition state change does not change state")
			}
			previousDirectory = stateChange.Directory
		}
	}
	return nil
}

func validArchiveQuestionState(state string) bool {
	switch state {
	case string(evidence.Observed), string(evidence.Unknown), "unavailable":
		return true
	default:
		return false
	}
}

func archiveQuestionTransitionHistorySHA256(history ArchiveQuestionTransitionHistory) (string, error) {
	data, err := json.Marshal(history)
	if err != nil {
		return "", fmt.Errorf("canonicalize: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
