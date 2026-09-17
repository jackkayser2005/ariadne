package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	replicationStudyQuestionRoundSchemaVersion   = 1
	replicationStudyQuestionReceiptSchemaVersion = 1
)

// ReplicationStudyQuestionRound retains every fixed study answer without the
// study or any source-specific input paths.
type ReplicationStudyQuestionRound struct {
	SchemaVersion int                   `json:"schema_version"`
	StudySHA256   string                `json:"study_sha256"`
	Answers       []StudyQuestionAnswer `json:"answers"`
}

// ReplicationStudyQuestionRoundVerificationSummary identifies a valid saved
// study question round.
type ReplicationStudyQuestionRoundVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	StudySHA256   string `json:"study_sha256"`
	Questions     int    `json:"questions"`
	RoundSHA256   string `json:"round_sha256"`
}

// ReplicationStudyQuestionReceipt is one selected study answer bound to a
// study and complete question-round identity.
type ReplicationStudyQuestionReceipt struct {
	StudyQuestionAnswer
	RoundSHA256 string                        `json:"round_sha256"`
	Round       ReplicationStudyQuestionRound `json:"round"`
}

// ReplicationStudyQuestionReceiptVerificationSummary identifies a valid saved
// study question receipt.
type ReplicationStudyQuestionReceiptVerificationSummary struct {
	SchemaVersion int               `json:"schema_version"`
	QuestionID    string            `json:"question_id"`
	Question      string            `json:"question"`
	Result        string            `json:"result"`
	EvidenceState evidence.State    `json:"evidence_state"`
	StudySHA256   string            `json:"study_sha256"`
	RoundSHA256   string            `json:"round_sha256"`
	ReceiptSHA256 string            `json:"receipt_sha256"`
	Outcome       ReplicatedOutcome `json:"outcome"`
}

// AnswerReplicationStudyQuestionRound answers the complete fixed catalog and
// returns a portable round bound to the verified study identity.
func AnswerReplicationStudyQuestionRound(study ReplicationStudy, summary StudyVerificationSummary) (ReplicationStudyQuestionRound, error) {
	expectedSummary, err := replicationStudySummary(study)
	if err != nil {
		return ReplicationStudyQuestionRound{}, err
	}
	if expectedSummary != summary {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round study identity does not match summary")
	}
	answers, err := AnswerAllReplicationStudyQuestions(study, summary)
	if err != nil {
		return ReplicationStudyQuestionRound{}, err
	}
	round := ReplicationStudyQuestionRound{
		SchemaVersion: replicationStudyQuestionRoundSchemaVersion,
		StudySHA256:   summary.StudySHA256,
		Answers:       answers,
	}
	if err := validateReplicationStudyQuestionRound(round); err != nil {
		return ReplicationStudyQuestionRound{}, err
	}
	return round, nil
}

// SaveReplicationStudyQuestionRound verifies a study, answers every fixed
// question, and writes a portable round without overwriting an existing path.
func SaveReplicationStudyQuestionRound(studyPath, roundPath string) (ReplicationStudyQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ReplicationStudyQuestionRoundVerificationSummary{}, errors.New("trace replication study question round path is required")
	}
	study, summary, err := ReadReplicationStudy(studyPath)
	if err != nil {
		return ReplicationStudyQuestionRoundVerificationSummary{}, err
	}
	round, err := AnswerReplicationStudyQuestionRound(study, summary)
	if err != nil {
		return ReplicationStudyQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ReplicationStudyQuestionRoundSHA256(round)
	if err != nil {
		return ReplicationStudyQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return ReplicationStudyQuestionRoundVerificationSummary{}, errors.New("trace replication study question round encoding failed")
	}
	if err := writeArchiveExclusive(roundPath, append(data, '\n')); err != nil {
		return ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("trace replication study question round: %w", err)
	}
	return replicationStudyQuestionRoundSummary(round, roundSHA256), nil
}

// ReadReplicationStudyQuestionRound verifies and reads a saved study question
// round.
func ReadReplicationStudyQuestionRound(path string) (ReplicationStudyQuestionRound, ReplicationStudyQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, errors.New("trace replication study question round path is required")
	}
	data, err := readArchive(path)
	if err != nil {
		return ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, fmt.Errorf("trace replication study question round: %w", err)
	}
	round, err := DecodeReplicationStudyQuestionRound(data)
	if err != nil {
		return ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ReplicationStudyQuestionRoundSHA256(round)
	if err != nil {
		return ReplicationStudyQuestionRound{}, ReplicationStudyQuestionRoundVerificationSummary{}, err
	}
	return round, replicationStudyQuestionRoundSummary(round, roundSHA256), nil
}

// VerifyReplicationStudyQuestionRound verifies a saved round without
// reopening the source study.
func VerifyReplicationStudyQuestionRound(path string) (ReplicationStudyQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadReplicationStudyQuestionRound(path)
	return summary, err
}

// DecodeReplicationStudyQuestionRound verifies one bounded study round
// document.
func DecodeReplicationStudyQuestionRound(data []byte) (ReplicationStudyQuestionRound, error) {
	if len(data) == 0 {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round is empty")
	}
	if len(data) > maxArchiveBytes {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round ReplicationStudyQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationStudyQuestionRound{}, errors.New("trace replication study question round has trailing data")
	}
	if err := validateReplicationStudyQuestionRound(round); err != nil {
		return ReplicationStudyQuestionRound{}, err
	}
	return round, nil
}

// ReplicationStudyQuestionRoundSHA256 returns the canonical identity of a
// valid study question round.
func ReplicationStudyQuestionRoundSHA256(round ReplicationStudyQuestionRound) (string, error) {
	if err := validateReplicationStudyQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("trace replication study question round encoding failed")
	}
	return sha256Hex(data), nil
}

// AskReplicationStudyQuestionRound returns one fixed answer from a saved
// study question round.
func AskReplicationStudyQuestionRound(path, questionID string) (StudyQuestionAnswer, error) {
	round, _, err := ReadReplicationStudyQuestionRound(path)
	if err != nil {
		return StudyQuestionAnswer{}, err
	}
	return studyQuestionAnswerFromRound(round, questionID)
}

// SaveReplicationStudyQuestionReceipt selects one fixed answer from a saved
// study round and writes a portable receipt without overwriting an existing
// path.
func SaveReplicationStudyQuestionReceipt(roundPath, questionID, receiptPath string) (ReplicationStudyQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, errors.New("trace replication study question receipt path is required")
	}
	round, roundSummary, err := ReadReplicationStudyQuestionRound(roundPath)
	if err != nil {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, err
	}
	answer, err := studyQuestionAnswerFromRound(round, questionID)
	if err != nil {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, err
	}
	receipt := ReplicationStudyQuestionReceipt{
		StudyQuestionAnswer: answer,
		RoundSHA256:         roundSummary.RoundSHA256,
		Round:               round,
	}
	receiptSHA256, err := ReplicationStudyQuestionReceiptSHA256(receipt)
	if err != nil {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, errors.New("trace replication study question receipt encoding failed")
	}
	if err := writeArchiveExclusive(receiptPath, append(data, '\n')); err != nil {
		return ReplicationStudyQuestionReceiptVerificationSummary{}, fmt.Errorf("trace replication study question receipt: %w", err)
	}
	return replicationStudyQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// ReadReplicationStudyQuestionReceipt verifies and reads a saved study
// question receipt.
func ReadReplicationStudyQuestionReceipt(path string) (ReplicationStudyQuestionReceipt, ReplicationStudyQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ReplicationStudyQuestionReceipt{}, ReplicationStudyQuestionReceiptVerificationSummary{}, errors.New("trace replication study question receipt path is required")
	}
	data, err := readArchive(path)
	if err != nil {
		return ReplicationStudyQuestionReceipt{}, ReplicationStudyQuestionReceiptVerificationSummary{}, fmt.Errorf("trace replication study question receipt: %w", err)
	}
	receipt, err := DecodeReplicationStudyQuestionReceipt(data)
	if err != nil {
		return ReplicationStudyQuestionReceipt{}, ReplicationStudyQuestionReceiptVerificationSummary{}, err
	}
	receiptSHA256, err := ReplicationStudyQuestionReceiptSHA256(receipt)
	if err != nil {
		return ReplicationStudyQuestionReceipt{}, ReplicationStudyQuestionReceiptVerificationSummary{}, err
	}
	return receipt, replicationStudyQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// VerifyReplicationStudyQuestionReceipt verifies a saved receipt without
// reopening the source study or round.
func VerifyReplicationStudyQuestionReceipt(path string) (ReplicationStudyQuestionReceiptVerificationSummary, error) {
	_, summary, err := ReadReplicationStudyQuestionReceipt(path)
	return summary, err
}

// DecodeReplicationStudyQuestionReceipt verifies one bounded receipt.
func DecodeReplicationStudyQuestionReceipt(data []byte) (ReplicationStudyQuestionReceipt, error) {
	if len(data) == 0 {
		return ReplicationStudyQuestionReceipt{}, errors.New("trace replication study question receipt is empty")
	}
	if len(data) > maxArchiveBytes {
		return ReplicationStudyQuestionReceipt{}, errors.New("trace replication study question receipt exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationStudyQuestionReceipt{}, errors.New("trace replication study question receipt has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt ReplicationStudyQuestionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return ReplicationStudyQuestionReceipt{}, errors.New("trace replication study question receipt has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationStudyQuestionReceipt{}, errors.New("trace replication study question receipt has trailing data")
	}
	if err := validateReplicationStudyQuestionReceipt(receipt); err != nil {
		return ReplicationStudyQuestionReceipt{}, err
	}
	return receipt, nil
}

// ReplicationStudyQuestionReceiptSHA256 returns the canonical identity of a
// valid study question receipt.
func ReplicationStudyQuestionReceiptSHA256(receipt ReplicationStudyQuestionReceipt) (string, error) {
	if err := validateReplicationStudyQuestionReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("trace replication study question receipt encoding failed")
	}
	return sha256Hex(data), nil
}

// AskReplicationStudyQuestionReceipt returns one selected receipt from a
// saved study question round.
func AskReplicationStudyQuestionReceipt(roundPath, questionID string) (ReplicationStudyQuestionReceipt, error) {
	round, roundSummary, err := ReadReplicationStudyQuestionRound(roundPath)
	if err != nil {
		return ReplicationStudyQuestionReceipt{}, err
	}
	answer, err := studyQuestionAnswerFromRound(round, questionID)
	if err != nil {
		return ReplicationStudyQuestionReceipt{}, err
	}
	return ReplicationStudyQuestionReceipt{
		StudyQuestionAnswer: answer,
		RoundSHA256:         roundSummary.RoundSHA256,
		Round:               round,
	}, nil
}

func studyQuestionAnswerFromRound(round ReplicationStudyQuestionRound, questionID string) (StudyQuestionAnswer, error) {
	index, ok := replicationStudyQuestionIndex(questionID)
	if !ok {
		return StudyQuestionAnswer{}, errors.New("trace replication study question ID is invalid")
	}
	return round.Answers[index], nil
}

func replicationStudyQuestionRoundSummary(round ReplicationStudyQuestionRound, digest string) ReplicationStudyQuestionRoundVerificationSummary {
	return ReplicationStudyQuestionRoundVerificationSummary{
		SchemaVersion: round.SchemaVersion,
		StudySHA256:   round.StudySHA256,
		Questions:     len(round.Answers),
		RoundSHA256:   digest,
	}
}

func replicationStudyQuestionReceiptSummary(receipt ReplicationStudyQuestionReceipt, digest string) ReplicationStudyQuestionReceiptVerificationSummary {
	return ReplicationStudyQuestionReceiptVerificationSummary{
		SchemaVersion: receipt.SchemaVersion,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		StudySHA256:   receipt.StudySHA256,
		RoundSHA256:   receipt.RoundSHA256,
		ReceiptSHA256: digest,
		Outcome:       receipt.Outcome,
	}
}

func replicationStudyQuestionIndex(id string) (int, bool) {
	for index, question := range ReplicationStudyQuestions() {
		if question.ID == id {
			return index, true
		}
	}
	return 0, false
}

func validateReplicationStudyQuestionRound(round ReplicationStudyQuestionRound) error {
	if round.SchemaVersion != replicationStudyQuestionRoundSchemaVersion {
		return errors.New("trace replication study question round has unsupported schema_version")
	}
	if !ValidSHA256(round.StudySHA256) {
		return errors.New("trace replication study question round study_sha256 is invalid")
	}
	questions := ReplicationStudyQuestions()
	if len(round.Answers) != len(questions) {
		return errors.New("trace replication study question round answer count does not match catalog")
	}
	for index, answer := range round.Answers {
		if err := validateReplicationStudyQuestionAnswer(answer, questions[index], round.StudySHA256); err != nil {
			return fmt.Errorf("trace replication study question round answer %d: %w", index+1, err)
		}
		if index > 0 && !sameReplicationStudyAnswerMetrics(round.Answers[0], answer) {
			return errors.New("trace replication study question round answers disagree about study metrics")
		}
	}
	return nil
}

func validateReplicationStudyQuestionReceipt(receipt ReplicationStudyQuestionReceipt) error {
	if receipt.SchemaVersion != replicationStudyQuestionReceiptSchemaVersion {
		return errors.New("trace replication study question receipt has unsupported schema_version")
	}
	question, ok := replicationStudyQuestion(receipt.QuestionID)
	if !ok {
		return errors.New("trace replication study question receipt question_id is invalid")
	}
	if err := validateReplicationStudyQuestionAnswer(receipt.StudyQuestionAnswer, question, receipt.StudySHA256); err != nil {
		return fmt.Errorf("trace replication study question receipt answer: %w", err)
	}
	if !ValidSHA256(receipt.RoundSHA256) {
		return errors.New("trace replication study question receipt round_sha256 is invalid")
	}
	if err := validateReplicationStudyQuestionRound(receipt.Round); err != nil {
		return fmt.Errorf("trace replication study question receipt round: %w", err)
	}
	roundSHA256, err := ReplicationStudyQuestionRoundSHA256(receipt.Round)
	if err != nil || roundSHA256 != receipt.RoundSHA256 {
		return errors.New("trace replication study question receipt round identity does not match round")
	}
	answer, err := studyQuestionAnswerFromRound(receipt.Round, receipt.QuestionID)
	if err != nil || answer != receipt.StudyQuestionAnswer {
		return errors.New("trace replication study question receipt answer does not match round")
	}
	return nil
}

func validateReplicationStudyQuestionAnswer(answer StudyQuestionAnswer, question StudyQuestion, studySHA256 string) error {
	if answer.SchemaVersion != replicationStudyQuestionRoundSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text {
		return errors.New("question identity is invalid")
	}
	if !ValidSHA256(answer.StudySHA256) || answer.StudySHA256 != studySHA256 {
		return errors.New("study identity is invalid")
	}
	if answer.Runs < 2 || answer.Pairs <= 0 || answer.SupportedRuns < 0 || answer.SupportedRuns > answer.Runs || answer.UnknownRuns < 0 || answer.UnknownRuns > answer.Runs || answer.SupportedRuns+answer.UnknownRuns != answer.Runs {
		return errors.New("study run counts are invalid")
	}
	if answer.ResetConfirmedPairs < 0 || answer.ResetConfirmedPairs > answer.Pairs || answer.CompletePairs < 0 || answer.CompletePairs > answer.Pairs || answer.UnknownPairs < 0 || answer.UnknownPairs > answer.Pairs {
		return errors.New("study pair support counts are invalid")
	}
	if answer.BalancedRuns < 0 || answer.BalancedRuns > answer.Runs || answer.ChangedRuns < 0 || answer.NoChangeRuns < 0 || answer.MixedRuns < 0 || answer.ChangedRuns+answer.NoChangeRuns+answer.MixedRuns != answer.SupportedRuns {
		return errors.New("study outcome counts are invalid")
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("evidence_state is invalid")
	}
	if answer.Outcome != ReplicatedChange && answer.Outcome != NoChangeObserved && answer.Outcome != MixedInconsistent && answer.Outcome != ReplicationUnknown {
		return errors.New("answer outcome is invalid")
	}
	expectedResult, ok := replicationStudyQuestionResult(question.ID, answer.Outcome, answer.EvidenceState)
	if !ok || answer.Result != expectedResult {
		return errors.New("answer result does not match study outcome")
	}
	supported := answer.UnknownRuns == 0 && answer.SupportedRuns == answer.Runs && answer.BalancedRuns == answer.Runs && answer.ResetConfirmedPairs == answer.Pairs && answer.CompletePairs == answer.Pairs && answer.UnknownPairs == 0
	expectedEvidence := evidence.Unknown
	if supported {
		expectedEvidence = evidence.Observed
	}
	if answer.EvidenceState != expectedEvidence {
		return errors.New("answer evidence_state does not match study metrics")
	}
	expectedOutcome := ReplicationUnknown
	if supported {
		switch {
		case answer.ChangedRuns == answer.Runs:
			expectedOutcome = ReplicatedChange
		case answer.NoChangeRuns == answer.Runs:
			expectedOutcome = NoChangeObserved
		default:
			expectedOutcome = MixedInconsistent
		}
	}
	if answer.Outcome != expectedOutcome {
		return errors.New("answer outcome does not match study metrics")
	}
	if answer.Reason != replicationStudyQuestionReason(answer) {
		return errors.New("answer reason is invalid")
	}
	return nil
}

func sameReplicationStudyAnswerMetrics(left, right StudyQuestionAnswer) bool {
	return left.StudySHA256 == right.StudySHA256 &&
		left.EvidenceState == right.EvidenceState &&
		left.Runs == right.Runs &&
		left.Pairs == right.Pairs &&
		left.SupportedRuns == right.SupportedRuns &&
		left.UnknownRuns == right.UnknownRuns &&
		left.ResetConfirmedPairs == right.ResetConfirmedPairs &&
		left.BalancedRuns == right.BalancedRuns &&
		left.CompletePairs == right.CompletePairs &&
		left.ChangedRuns == right.ChangedRuns &&
		left.NoChangeRuns == right.NoChangeRuns &&
		left.MixedRuns == right.MixedRuns &&
		left.UnknownPairs == right.UnknownPairs &&
		left.Outcome == right.Outcome
}
