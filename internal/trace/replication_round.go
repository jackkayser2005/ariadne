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
	replicationQuestionRoundSchemaVersion   = 1
	replicationQuestionReceiptSchemaVersion = 1
)

const (
	// ReplicationQuestionOutcome asks for the aggregate counterfactual result.
	ReplicationQuestionOutcome = "replication-outcome"
	// ReplicationQuestionSupport asks whether the retained evidence supports it.
	ReplicationQuestionSupport = "replication-support"
	// ReplicationQuestionConsistency asks whether both execution orders agree.
	ReplicationQuestionConsistency = "replication-consistency"
)

// ReplicationQuestion is one fixed, bounded question available for a ledger.
type ReplicationQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// ReplicationQuestions returns the stable replication question catalog.
func ReplicationQuestions() []ReplicationQuestion {
	return []ReplicationQuestion{
		{ID: ReplicationQuestionOutcome, Text: "What aggregate outcome was observed across the matched pairs?"},
		{ID: ReplicationQuestionSupport, Text: "Did every pair have confirmed resets and complete comparison support?"},
		{ID: ReplicationQuestionConsistency, Text: "Did the result agree across both execution orders?"},
	}
}

// ReplicationAnswer is a safe answer tied to one verified ledger identity.
// Result describes the question outcome; EvidenceState remains separate.
type ReplicationAnswer struct {
	SchemaVersion          int               `json:"schema_version"`
	QuestionID             string            `json:"question_id"`
	Question               string            `json:"question"`
	Result                 string            `json:"result"`
	EvidenceState          evidence.State    `json:"evidence_state"`
	Reason                 string            `json:"reason,omitempty"`
	LedgerSHA256           string            `json:"ledger_sha256"`
	Pairs                  int               `json:"pairs"`
	BaselineTreatmentPairs int               `json:"baseline_treatment_pairs"`
	TreatmentBaselinePairs int               `json:"treatment_baseline_pairs"`
	ResetConfirmedPairs    int               `json:"reset_confirmed_pairs"`
	CompletePairs          int               `json:"complete_pairs"`
	ChangedPairs           int               `json:"changed_pairs"`
	NoChangePairs          int               `json:"no_change_pairs"`
	UnknownPairs           int               `json:"unknown_pairs"`
	OrderBalanced          bool              `json:"order_balanced"`
	Outcome                ReplicatedOutcome `json:"outcome"`
}

// ReplicationQuestionRound retains every fixed answer without the ledger or
// any source-specific input paths.
type ReplicationQuestionRound struct {
	SchemaVersion int                 `json:"schema_version"`
	LedgerSHA256  string              `json:"ledger_sha256"`
	Answers       []ReplicationAnswer `json:"answers"`
}

// ReplicationQuestionRoundVerificationSummary identifies a valid saved round.
type ReplicationQuestionRoundVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	LedgerSHA256  string `json:"ledger_sha256"`
	Questions     int    `json:"questions"`
	RoundSHA256   string `json:"round_sha256"`
}

// ReplicationQuestionReceipt is one selected answer bound to a ledger and
// question-round identity.
type ReplicationQuestionReceipt struct {
	ReplicationAnswer
	RoundSHA256 string                   `json:"round_sha256"`
	Round       ReplicationQuestionRound `json:"round"`
}

// ReplicationQuestionReceiptVerificationSummary identifies a valid receipt.
type ReplicationQuestionReceiptVerificationSummary struct {
	SchemaVersion int               `json:"schema_version"`
	QuestionID    string            `json:"question_id"`
	Question      string            `json:"question"`
	Result        string            `json:"result"`
	EvidenceState evidence.State    `json:"evidence_state"`
	LedgerSHA256  string            `json:"ledger_sha256"`
	RoundSHA256   string            `json:"round_sha256"`
	ReceiptSHA256 string            `json:"receipt_sha256"`
	Outcome       ReplicatedOutcome `json:"outcome"`
}

// AnswerReplicationQuestion answers one fixed question against a verified
// ledger and summary.
func AnswerReplicationQuestion(ledger ReplicationLedger, summary ReplicationLedgerVerificationSummary, questionID string) (ReplicationAnswer, error) {
	question, ok := replicationQuestion(questionID)
	if !ok {
		return ReplicationAnswer{}, errors.New("trace replication question ID is invalid")
	}
	expectedSummary, err := replicationLedgerSummary(ledger)
	if err != nil {
		return ReplicationAnswer{}, err
	}
	if expectedSummary != summary {
		return ReplicationAnswer{}, errors.New("trace replication question ledger identity does not match summary")
	}
	return answerReplicationQuestionFromSummary(summary, question)
}

// AnswerAllReplicationQuestionsFromSummary answers the fixed catalog from a
// summary that has already been verified by its caller.
func AnswerAllReplicationQuestionsFromSummary(summary ReplicationLedgerVerificationSummary) ([]ReplicationAnswer, error) {
	return answerAllReplicationQuestionsFromSummary(summary)
}

func answerReplicationQuestionFromSummary(summary ReplicationLedgerVerificationSummary, question ReplicationQuestion) (ReplicationAnswer, error) {
	answer := replicationAnswerFromSummary(summary, question)
	switch question.ID {
	case ReplicationQuestionOutcome:
		answer.Result = string(summary.Outcome)
		answer.Reason = summary.Reason
	case ReplicationQuestionSupport:
		if summary.EvidenceState == evidence.Observed {
			answer.Result = "supported"
			answer.Reason = "every retained pair has a confirmed reset and complete comparison support"
		} else {
			answer.Result = "unknown"
			answer.Reason = "at least one retained pair lacks reset, capture, or comparison support"
		}
	case ReplicationQuestionConsistency:
		switch summary.Outcome {
		case ReplicatedChange, NoChangeObserved:
			answer.Result = "consistent"
			answer.Reason = "both explicit execution orders agree about the aggregate result"
		case MixedInconsistent:
			answer.Result = "inconsistent"
			answer.Reason = "retained pairs disagree about safe category change"
		default:
			answer.Result = "unknown"
			answer.Reason = "the aggregate cannot establish agreement across both execution orders"
		}
	default:
		return ReplicationAnswer{}, errors.New("trace replication question ID is invalid")
	}
	return answer, nil
}

// AnswerAllReplicationQuestions answers the complete fixed catalog in order.
func AnswerAllReplicationQuestions(ledger ReplicationLedger, summary ReplicationLedgerVerificationSummary) ([]ReplicationAnswer, error) {
	expectedSummary, err := replicationLedgerSummary(ledger)
	if err != nil {
		return nil, err
	}
	if expectedSummary != summary {
		return nil, errors.New("trace replication question ledger identity does not match summary")
	}
	return answerAllReplicationQuestionsFromSummary(summary)
}

func answerAllReplicationQuestionsFromSummary(summary ReplicationLedgerVerificationSummary) ([]ReplicationAnswer, error) {
	answers := make([]ReplicationAnswer, 0, len(ReplicationQuestions()))
	for _, question := range ReplicationQuestions() {
		answer, err := answerReplicationQuestionFromSummary(summary, question)
		if err != nil {
			return nil, err
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

// AskReplicationQuestion answers one fixed question after verifying a ledger.
func AskReplicationQuestion(path, questionID string) (ReplicationAnswer, error) {
	ledger, summary, err := ReadReplicationLedger(path)
	if err != nil {
		return ReplicationAnswer{}, err
	}
	return AnswerReplicationQuestion(ledger, summary, questionID)
}

// AskAllReplicationQuestions answers the complete fixed catalog after
// verifying a ledger.
func AskAllReplicationQuestions(path string) ([]ReplicationAnswer, error) {
	ledger, summary, err := ReadReplicationLedger(path)
	if err != nil {
		return nil, err
	}
	return AnswerAllReplicationQuestions(ledger, summary)
}

// SaveReplicationQuestionRound verifies a ledger, asks every fixed question,
// and writes a portable round without overwriting an existing path.
func SaveReplicationQuestionRound(ledgerPath, roundPath string) (ReplicationQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ReplicationQuestionRoundVerificationSummary{}, errors.New("trace replication question round path is required")
	}
	ledger, summary, err := ReadReplicationLedger(ledgerPath)
	if err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, err
	}
	answers, err := AnswerAllReplicationQuestions(ledger, summary)
	if err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, err
	}
	round := ReplicationQuestionRound{
		SchemaVersion: replicationQuestionRoundSchemaVersion,
		LedgerSHA256:  summary.LedgerSHA256,
		Answers:       answers,
	}
	roundSHA256, err := ReplicationQuestionRoundSHA256(round)
	if err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, errors.New("trace replication question round encoding failed")
	}
	if err := writeArchiveExclusive(roundPath, append(data, '\n')); err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, fmt.Errorf("trace replication question round: %w", err)
	}
	return replicationQuestionRoundSummary(round, roundSHA256), nil
}

// ReadReplicationQuestionRound verifies and reads a saved question round.
func ReadReplicationQuestionRound(path string) (ReplicationQuestionRound, ReplicationQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ReplicationQuestionRound{}, ReplicationQuestionRoundVerificationSummary{}, errors.New("trace replication question round path is required")
	}
	data, err := readArchive(path)
	if err != nil {
		return ReplicationQuestionRound{}, ReplicationQuestionRoundVerificationSummary{}, fmt.Errorf("trace replication question round: %w", err)
	}
	round, err := DecodeReplicationQuestionRound(data)
	if err != nil {
		return ReplicationQuestionRound{}, ReplicationQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ReplicationQuestionRoundSHA256(round)
	if err != nil {
		return ReplicationQuestionRound{}, ReplicationQuestionRoundVerificationSummary{}, err
	}
	return round, replicationQuestionRoundSummary(round, roundSHA256), nil
}

// VerifyReplicationQuestionRound verifies a saved round without reopening
// the source ledger.
func VerifyReplicationQuestionRound(path string) (ReplicationQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadReplicationQuestionRound(path)
	return summary, err
}

// DecodeReplicationQuestionRound verifies one bounded question-round document.
func DecodeReplicationQuestionRound(data []byte) (ReplicationQuestionRound, error) {
	if len(data) == 0 {
		return ReplicationQuestionRound{}, errors.New("trace replication question round is empty")
	}
	if len(data) > maxArchiveBytes {
		return ReplicationQuestionRound{}, errors.New("trace replication question round exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationQuestionRound{}, errors.New("trace replication question round has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round ReplicationQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return ReplicationQuestionRound{}, errors.New("trace replication question round has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationQuestionRound{}, errors.New("trace replication question round has trailing data")
	}
	if err := validateReplicationQuestionRound(round); err != nil {
		return ReplicationQuestionRound{}, err
	}
	return round, nil
}

// ReplicationQuestionRoundSHA256 returns the canonical identity of a valid round.
func ReplicationQuestionRoundSHA256(round ReplicationQuestionRound) (string, error) {
	if err := validateReplicationQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("trace replication question round encoding failed")
	}
	return sha256Hex(data), nil
}

// AskReplicationQuestionRound returns one fixed answer from a saved round.
func AskReplicationQuestionRound(path, questionID string) (ReplicationAnswer, error) {
	round, _, err := ReadReplicationQuestionRound(path)
	if err != nil {
		return ReplicationAnswer{}, err
	}
	index, ok := replicationQuestionIndex(questionID)
	if !ok {
		return ReplicationAnswer{}, errors.New("trace replication question ID is invalid")
	}
	return round.Answers[index], nil
}

// SaveReplicationQuestionReceipt selects one fixed answer from a saved round
// and writes a portable receipt without overwriting an existing path.
func SaveReplicationQuestionReceipt(roundPath, questionID, receiptPath string) (ReplicationQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return ReplicationQuestionReceiptVerificationSummary{}, errors.New("trace replication question receipt path is required")
	}
	round, roundSummary, err := ReadReplicationQuestionRound(roundPath)
	if err != nil {
		return ReplicationQuestionReceiptVerificationSummary{}, err
	}
	answer, err := replicationAnswerFromRound(round, questionID)
	if err != nil {
		return ReplicationQuestionReceiptVerificationSummary{}, err
	}
	receipt := ReplicationQuestionReceipt{ReplicationAnswer: answer, RoundSHA256: roundSummary.RoundSHA256, Round: round}
	receiptSHA256, err := ReplicationQuestionReceiptSHA256(receipt)
	if err != nil {
		return ReplicationQuestionReceiptVerificationSummary{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return ReplicationQuestionReceiptVerificationSummary{}, errors.New("trace replication question receipt encoding failed")
	}
	if err := writeArchiveExclusive(receiptPath, append(data, '\n')); err != nil {
		return ReplicationQuestionReceiptVerificationSummary{}, fmt.Errorf("trace replication question receipt: %w", err)
	}
	return replicationQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// ReadReplicationQuestionReceipt verifies and reads a saved receipt.
func ReadReplicationQuestionReceipt(path string) (ReplicationQuestionReceipt, ReplicationQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ReplicationQuestionReceipt{}, ReplicationQuestionReceiptVerificationSummary{}, errors.New("trace replication question receipt path is required")
	}
	data, err := readArchive(path)
	if err != nil {
		return ReplicationQuestionReceipt{}, ReplicationQuestionReceiptVerificationSummary{}, fmt.Errorf("trace replication question receipt: %w", err)
	}
	receipt, err := DecodeReplicationQuestionReceipt(data)
	if err != nil {
		return ReplicationQuestionReceipt{}, ReplicationQuestionReceiptVerificationSummary{}, err
	}
	receiptSHA256, err := ReplicationQuestionReceiptSHA256(receipt)
	if err != nil {
		return ReplicationQuestionReceipt{}, ReplicationQuestionReceiptVerificationSummary{}, err
	}
	return receipt, replicationQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// VerifyReplicationQuestionReceipt verifies a saved receipt without reopening
// the source ledger or question round.
func VerifyReplicationQuestionReceipt(path string) (ReplicationQuestionReceiptVerificationSummary, error) {
	_, summary, err := ReadReplicationQuestionReceipt(path)
	return summary, err
}

// DecodeReplicationQuestionReceipt verifies one bounded receipt document.
func DecodeReplicationQuestionReceipt(data []byte) (ReplicationQuestionReceipt, error) {
	if len(data) == 0 {
		return ReplicationQuestionReceipt{}, errors.New("trace replication question receipt is empty")
	}
	if len(data) > maxArchiveBytes {
		return ReplicationQuestionReceipt{}, errors.New("trace replication question receipt exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationQuestionReceipt{}, errors.New("trace replication question receipt has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt ReplicationQuestionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return ReplicationQuestionReceipt{}, errors.New("trace replication question receipt has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationQuestionReceipt{}, errors.New("trace replication question receipt has trailing data")
	}
	if err := validateReplicationQuestionReceipt(receipt); err != nil {
		return ReplicationQuestionReceipt{}, err
	}
	return receipt, nil
}

// ReplicationQuestionReceiptSHA256 returns the canonical identity of a valid receipt.
func ReplicationQuestionReceiptSHA256(receipt ReplicationQuestionReceipt) (string, error) {
	if err := validateReplicationQuestionReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("trace replication question receipt encoding failed")
	}
	return sha256Hex(data), nil
}

// AskReplicationQuestionReceipt returns one selected receipt from a saved round.
func AskReplicationQuestionReceipt(roundPath, questionID string) (ReplicationQuestionReceipt, error) {
	round, summary, err := ReadReplicationQuestionRound(roundPath)
	if err != nil {
		return ReplicationQuestionReceipt{}, err
	}
	answer, err := replicationAnswerFromRound(round, questionID)
	if err != nil {
		return ReplicationQuestionReceipt{}, err
	}
	return ReplicationQuestionReceipt{ReplicationAnswer: answer, RoundSHA256: summary.RoundSHA256, Round: round}, nil
}

func replicationAnswerFromSummary(summary ReplicationLedgerVerificationSummary, question ReplicationQuestion) ReplicationAnswer {
	return ReplicationAnswer{
		SchemaVersion:          replicationQuestionRoundSchemaVersion,
		QuestionID:             question.ID,
		Question:               question.Text,
		LedgerSHA256:           summary.LedgerSHA256,
		EvidenceState:          summary.EvidenceState,
		Pairs:                  summary.Pairs,
		BaselineTreatmentPairs: summary.BaselineTreatmentPairs,
		TreatmentBaselinePairs: summary.TreatmentBaselinePairs,
		ResetConfirmedPairs:    summary.ResetConfirmedPairs,
		CompletePairs:          summary.CompletePairs,
		ChangedPairs:           summary.ChangedPairs,
		NoChangePairs:          summary.NoChangePairs,
		UnknownPairs:           summary.UnknownPairs,
		OrderBalanced:          summary.OrderBalanced,
		Outcome:                summary.Outcome,
	}
}

func replicationQuestionRoundSummary(round ReplicationQuestionRound, digest string) ReplicationQuestionRoundVerificationSummary {
	return ReplicationQuestionRoundVerificationSummary{
		SchemaVersion: replicationQuestionRoundSchemaVersion,
		LedgerSHA256:  round.LedgerSHA256,
		Questions:     len(round.Answers),
		RoundSHA256:   digest,
	}
}

func replicationQuestionReceiptSummary(receipt ReplicationQuestionReceipt, digest string) ReplicationQuestionReceiptVerificationSummary {
	return ReplicationQuestionReceiptVerificationSummary{
		SchemaVersion: receipt.SchemaVersion,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		LedgerSHA256:  receipt.LedgerSHA256,
		RoundSHA256:   receipt.RoundSHA256,
		ReceiptSHA256: digest,
		Outcome:       receipt.Outcome,
	}
}

func replicationAnswerFromRound(round ReplicationQuestionRound, questionID string) (ReplicationAnswer, error) {
	index, ok := replicationQuestionIndex(questionID)
	if !ok {
		return ReplicationAnswer{}, errors.New("trace replication question ID is invalid")
	}
	return round.Answers[index], nil
}

func replicationQuestion(id string) (ReplicationQuestion, bool) {
	for _, question := range ReplicationQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return ReplicationQuestion{}, false
}

func replicationQuestionIndex(id string) (int, bool) {
	for index, question := range ReplicationQuestions() {
		if question.ID == id {
			return index, true
		}
	}
	return 0, false
}

func validateReplicationQuestionRound(round ReplicationQuestionRound) error {
	if round.SchemaVersion != replicationQuestionRoundSchemaVersion {
		return errors.New("trace replication question round has unsupported schema_version")
	}
	if !ValidSHA256(round.LedgerSHA256) {
		return errors.New("trace replication question round ledger_sha256 is invalid")
	}
	questions := ReplicationQuestions()
	if len(round.Answers) != len(questions) {
		return errors.New("trace replication question round answer count does not match catalog")
	}
	for index, answer := range round.Answers {
		if err := validateReplicationAnswer(answer, questions[index], round.LedgerSHA256); err != nil {
			return fmt.Errorf("trace replication question round answer %d: %w", index+1, err)
		}
		if index > 0 && !sameReplicationAnswerMetrics(round.Answers[0], answer) {
			return errors.New("trace replication question round answers disagree about ledger metrics")
		}
	}
	return nil
}

func validateReplicationQuestionReceipt(receipt ReplicationQuestionReceipt) error {
	if receipt.SchemaVersion != replicationQuestionReceiptSchemaVersion {
		return errors.New("trace replication question receipt has unsupported schema_version")
	}
	question, ok := replicationQuestion(receipt.QuestionID)
	if !ok {
		return errors.New("trace replication question receipt question_id is invalid")
	}
	if err := validateReplicationAnswer(receipt.ReplicationAnswer, question, receipt.LedgerSHA256); err != nil {
		return fmt.Errorf("trace replication question receipt answer: %w", err)
	}
	if !ValidSHA256(receipt.RoundSHA256) {
		return errors.New("trace replication question receipt round_sha256 is invalid")
	}
	if err := validateReplicationQuestionRound(receipt.Round); err != nil {
		return fmt.Errorf("trace replication question receipt round: %w", err)
	}
	roundSHA256, err := ReplicationQuestionRoundSHA256(receipt.Round)
	if err != nil || roundSHA256 != receipt.RoundSHA256 {
		return errors.New("trace replication question receipt round identity does not match round")
	}
	answer, err := replicationAnswerFromRound(receipt.Round, receipt.QuestionID)
	if err != nil || answer != receipt.ReplicationAnswer {
		return errors.New("trace replication question receipt answer does not match round")
	}
	return nil
}

func validateReplicationAnswer(answer ReplicationAnswer, question ReplicationQuestion, ledgerSHA256 string) error {
	if answer.SchemaVersion != replicationQuestionRoundSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text {
		return errors.New("question identity is invalid")
	}
	if !ValidSHA256(answer.LedgerSHA256) || answer.LedgerSHA256 != ledgerSHA256 {
		return errors.New("ledger identity is invalid")
	}
	if answer.Pairs <= 0 || answer.BaselineTreatmentPairs < 0 || answer.TreatmentBaselinePairs < 0 || answer.BaselineTreatmentPairs+answer.TreatmentBaselinePairs != answer.Pairs || answer.ResetConfirmedPairs < 0 || answer.ResetConfirmedPairs > answer.Pairs || answer.CompletePairs < 0 || answer.CompletePairs > answer.Pairs || answer.ChangedPairs < 0 || answer.NoChangePairs < 0 || answer.UnknownPairs < 0 || answer.ChangedPairs+answer.NoChangePairs+answer.UnknownPairs != answer.Pairs {
		return errors.New("answer counts are invalid")
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("evidence_state is invalid")
	}
	if !validReplicationQuestionResult(question.ID, answer.Result) {
		return errors.New("answer result is invalid")
	}
	if answer.Outcome != ReplicatedChange && answer.Outcome != NoChangeObserved && answer.Outcome != MixedInconsistent && answer.Outcome != ReplicationUnknown {
		return errors.New("answer outcome is invalid")
	}
	expectedResult, ok := replicationQuestionResult(question.ID, answer.Outcome, answer.EvidenceState)
	if !ok || answer.Result != expectedResult {
		return errors.New("answer result does not match ledger outcome")
	}
	expectedEvidence := evidence.Unknown
	supported := answer.ResetConfirmedPairs == answer.Pairs && answer.CompletePairs == answer.Pairs && answer.UnknownPairs == 0
	if answer.OrderBalanced && supported {
		expectedEvidence = evidence.Observed
	}
	if answer.EvidenceState != expectedEvidence {
		return errors.New("answer evidence_state does not match ledger metrics")
	}
	expectedOutcome := ReplicationUnknown
	if answer.OrderBalanced && supported {
		switch {
		case answer.ChangedPairs == answer.Pairs:
			expectedOutcome = ReplicatedChange
		case answer.NoChangePairs == answer.Pairs:
			expectedOutcome = NoChangeObserved
		default:
			expectedOutcome = MixedInconsistent
		}
	}
	if answer.Outcome != expectedOutcome {
		return errors.New("answer outcome does not match ledger metrics")
	}
	expectedReason := replicationQuestionReason(answer)
	if answer.Reason != expectedReason {
		return errors.New("answer reason is invalid")
	}
	if answer.OrderBalanced != (answer.BaselineTreatmentPairs > 0 && answer.BaselineTreatmentPairs == answer.TreatmentBaselinePairs) {
		return errors.New("answer order balance is invalid")
	}
	return nil
}

func sameReplicationAnswerMetrics(left, right ReplicationAnswer) bool {
	return left.LedgerSHA256 == right.LedgerSHA256 &&
		left.EvidenceState == right.EvidenceState &&
		left.Pairs == right.Pairs &&
		left.BaselineTreatmentPairs == right.BaselineTreatmentPairs &&
		left.TreatmentBaselinePairs == right.TreatmentBaselinePairs &&
		left.ResetConfirmedPairs == right.ResetConfirmedPairs &&
		left.CompletePairs == right.CompletePairs &&
		left.ChangedPairs == right.ChangedPairs &&
		left.NoChangePairs == right.NoChangePairs &&
		left.UnknownPairs == right.UnknownPairs &&
		left.OrderBalanced == right.OrderBalanced &&
		left.Outcome == right.Outcome
}

func validReplicationQuestionResult(questionID, result string) bool {
	switch questionID {
	case ReplicationQuestionOutcome:
		return result == string(ReplicatedChange) || result == string(NoChangeObserved) || result == string(MixedInconsistent) || result == string(ReplicationUnknown)
	case ReplicationQuestionSupport:
		return result == "supported" || result == "unknown"
	case ReplicationQuestionConsistency:
		return result == "consistent" || result == "inconsistent" || result == "unknown"
	default:
		return false
	}
}

func replicationQuestionReason(answer ReplicationAnswer) string {
	switch answer.QuestionID {
	case ReplicationQuestionOutcome:
		if !answer.OrderBalanced {
			return "equal nonzero counts of both explicit pair orders are required"
		}
		switch answer.Outcome {
		case ReplicatedChange:
			return "every retained pair contains a safe category difference"
		case NoChangeObserved:
			return "no retained pair contains a safe category difference"
		case MixedInconsistent:
			return "retained pairs disagree about safe category change"
		default:
			return "at least one retained pair lacks complete reset, capture, or comparison support"
		}
	case ReplicationQuestionSupport:
		if answer.EvidenceState == evidence.Observed {
			return "every retained pair has a confirmed reset and complete comparison support"
		}
		return "at least one retained pair lacks reset, capture, or comparison support"
	case ReplicationQuestionConsistency:
		switch answer.Outcome {
		case ReplicatedChange, NoChangeObserved:
			return "both explicit execution orders agree about the aggregate result"
		case MixedInconsistent:
			return "retained pairs disagree about safe category change"
		default:
			return "the aggregate cannot establish agreement across both execution orders"
		}
	default:
		return ""
	}
}

func replicationQuestionResult(questionID string, outcome ReplicatedOutcome, state evidence.State) (string, bool) {
	switch questionID {
	case ReplicationQuestionOutcome:
		return string(outcome), true
	case ReplicationQuestionSupport:
		if state == evidence.Observed {
			return "supported", true
		}
		return "unknown", true
	case ReplicationQuestionConsistency:
		switch outcome {
		case ReplicatedChange, NoChangeObserved:
			return "consistent", true
		case MixedInconsistent:
			return "inconsistent", true
		case ReplicationUnknown:
			return "unknown", true
		default:
			return "", false
		}
	default:
		return "", false
	}
}
