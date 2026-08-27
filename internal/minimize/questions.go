package minimize

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// QuestionRoundSchemaVersion is the minimization question-round schema.
	QuestionRoundSchemaVersion = 1
	// QuestionReceiptSchemaVersion is the minimization question-receipt schema.
	QuestionReceiptSchemaVersion = 1
	// MinimizationQuestionSelection asks for the bounded ladder decision.
	MinimizationQuestionSelection = "minimum-tested-selection"
	// MinimizationQuestionSupport asks whether every candidate is supported.
	MinimizationQuestionSupport = "minimization-support"

	questionArtifactMaxBytes = 128 << 10
)

// MinimizationQuestion is one fixed, bounded question for a verified ladder.
type MinimizationQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// MinimizationCandidateProjection is the safe candidate projection retained
// by a question round. It omits local directories, manifest names, and input
// values while preserving the result and its support counts.
type MinimizationCandidateProjection struct {
	ID             string                   `json:"id"`
	Classification CandidateClassification  `json:"classification"`
	Outcome        bundle.ReplicatedOutcome `json:"outcome"`
	EvidenceState  evidence.State           `json:"evidence_state"`
	ReceiptSHA256  string                   `json:"receipt_sha256"`
	Pairs          int                      `json:"pairs"`
	PairsPerOrder  int                      `json:"pairs_per_order"`
	CompletedPairs int                      `json:"completed_pairs"`
	ChangedPairs   int                      `json:"changed_pairs"`
	NoChangePairs  int                      `json:"no_change_pairs"`
	UnknownPairs   int                      `json:"unknown_pairs"`
}

// MinimizationQuestionAnswer is a safe answer bound to a verified
// minimization receipt. Result and selection state are decisions; candidate
// outcomes remain in the round projection and are not folded into them.
type MinimizationQuestionAnswer struct {
	SchemaVersion       int            `json:"schema_version"`
	QuestionID          string         `json:"question_id"`
	Question            string         `json:"question"`
	Result              string         `json:"result"`
	EvidenceState       evidence.State `json:"evidence_state"`
	Reason              string         `json:"reason"`
	MinimizationSHA256  string         `json:"minimization_sha256"`
	SelectionState      SelectionState `json:"selection_state"`
	SelectedCandidate   string         `json:"selected_candidate,omitempty"`
	CandidateCount      int            `json:"candidate_count"`
	SupportedCandidates int            `json:"supported_candidates"`
	UnknownCandidates   int            `json:"unknown_candidates"`
}

// MinimizationQuestionRound retains every fixed answer and safe candidate
// projection without retaining the minimization receipt or source inputs.
type MinimizationQuestionRound struct {
	SchemaVersion      int                               `json:"schema_version"`
	MinimizationSHA256 string                            `json:"minimization_sha256"`
	Candidates         []MinimizationCandidateProjection `json:"candidates"`
	Answers            []MinimizationQuestionAnswer      `json:"answers"`
}

// MinimizationQuestionRoundVerificationSummary identifies a valid saved
// question round without reopening the source minimization run.
type MinimizationQuestionRoundVerificationSummary struct {
	SchemaVersion      int    `json:"schema_version"`
	MinimizationSHA256 string `json:"minimization_sha256"`
	Questions          int    `json:"questions"`
	Candidates         int    `json:"candidates"`
	RoundSHA256        string `json:"round_sha256"`
}

// MinimizationQuestionReceipt is one selected answer bound to a complete
// question round and its minimization identity.
type MinimizationQuestionReceipt struct {
	MinimizationQuestionAnswer
	RoundSHA256 string                    `json:"round_sha256"`
	Round       MinimizationQuestionRound `json:"round"`
}

// MinimizationQuestionReceiptVerificationSummary identifies a valid selected
// answer without reopening the source minimization run or question round.
type MinimizationQuestionReceiptVerificationSummary struct {
	SchemaVersion       int            `json:"schema_version"`
	QuestionID          string         `json:"question_id"`
	Question            string         `json:"question"`
	Result              string         `json:"result"`
	EvidenceState       evidence.State `json:"evidence_state"`
	MinimizationSHA256  string         `json:"minimization_sha256"`
	RoundSHA256         string         `json:"round_sha256"`
	ReceiptSHA256       string         `json:"receipt_sha256"`
	SelectionState      SelectionState `json:"selection_state"`
	SelectedCandidate   string         `json:"selected_candidate,omitempty"`
	CandidateCount      int            `json:"candidate_count"`
	SupportedCandidates int            `json:"supported_candidates"`
	UnknownCandidates   int            `json:"unknown_candidates"`
}

// MinimizationQuestions returns the stable minimization question catalog.
func MinimizationQuestions() []MinimizationQuestion {
	return []MinimizationQuestion{
		{ID: MinimizationQuestionSelection, Text: "What minimum tested sufficient disclosure, if any, did this ladder establish?"},
		{ID: MinimizationQuestionSupport, Text: "Did every candidate have complete replicated support and observed evidence?"},
	}
}

// AnswerMinimizationQuestion answers one fixed question against a verified
// minimization summary and its canonical receipt identity.
func AnswerMinimizationQuestion(summary MinimizationSummary, receiptSHA256, questionID string) (MinimizationQuestionAnswer, error) {
	question, ok := minimizationQuestion(questionID)
	if !ok {
		return MinimizationQuestionAnswer{}, errors.New("minimization question ID is invalid")
	}
	if err := validateSummary(summary); err != nil {
		return MinimizationQuestionAnswer{}, fmt.Errorf("minimization question summary: %w", err)
	}
	if !validDigest(receiptSHA256) {
		return MinimizationQuestionAnswer{}, errors.New("minimization question receipt identity is invalid")
	}
	return minimizationQuestionAnswer(summary, receiptSHA256, question), nil
}

// AnswerAllMinimizationQuestions answers the complete fixed catalog in order.
func AnswerAllMinimizationQuestions(summary MinimizationSummary, receiptSHA256 string) ([]MinimizationQuestionAnswer, error) {
	if err := validateSummary(summary); err != nil {
		return nil, fmt.Errorf("minimization question summary: %w", err)
	}
	if !validDigest(receiptSHA256) {
		return nil, errors.New("minimization question receipt identity is invalid")
	}
	answers := make([]MinimizationQuestionAnswer, 0, len(MinimizationQuestions()))
	for _, question := range MinimizationQuestions() {
		answers = append(answers, minimizationQuestionAnswer(summary, receiptSHA256, question))
	}
	return answers, nil
}

// AskMinimizationQuestion verifies a saved minimization run before answering.
func AskMinimizationQuestion(path, questionID string) (MinimizationQuestionAnswer, error) {
	summary, receiptSHA256, err := VerifyWithIdentity(path)
	if err != nil {
		return MinimizationQuestionAnswer{}, err
	}
	return AnswerMinimizationQuestion(summary, receiptSHA256, questionID)
}

// AskAllMinimizationQuestions verifies a saved run before answering every
// fixed question.
func AskAllMinimizationQuestions(path string) ([]MinimizationQuestionAnswer, error) {
	summary, receiptSHA256, err := VerifyWithIdentity(path)
	if err != nil {
		return nil, err
	}
	return AnswerAllMinimizationQuestions(summary, receiptSHA256)
}

// AnswerMinimizationQuestionRound builds a portable round from an already
// verified minimization summary and receipt identity.
func AnswerMinimizationQuestionRound(summary MinimizationSummary, receiptSHA256 string) (MinimizationQuestionRound, error) {
	if err := validateSummary(summary); err != nil {
		return MinimizationQuestionRound{}, fmt.Errorf("minimization question summary: %w", err)
	}
	if !validDigest(receiptSHA256) {
		return MinimizationQuestionRound{}, errors.New("minimization question receipt identity is invalid")
	}
	answers, err := AnswerAllMinimizationQuestions(summary, receiptSHA256)
	if err != nil {
		return MinimizationQuestionRound{}, err
	}
	candidates := make([]MinimizationCandidateProjection, 0, len(summary.CandidateResults))
	for _, result := range summary.CandidateResults {
		candidates = append(candidates, candidateProjection(result))
	}
	round := MinimizationQuestionRound{
		SchemaVersion:      QuestionRoundSchemaVersion,
		MinimizationSHA256: receiptSHA256,
		Candidates:         candidates,
		Answers:            answers,
	}
	if err := validateQuestionRound(round); err != nil {
		return MinimizationQuestionRound{}, err
	}
	return round, nil
}

// SaveMinimizationQuestionRound verifies a minimization run and writes a
// portable question round without overwriting an existing path.
func SaveMinimizationQuestionRound(minimizationPath, roundPath string) (MinimizationQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return MinimizationQuestionRoundVerificationSummary{}, errors.New("minimization question round path is required")
	}
	summary, receiptSHA256, err := VerifyWithIdentity(minimizationPath)
	if err != nil {
		return MinimizationQuestionRoundVerificationSummary{}, err
	}
	round, err := AnswerMinimizationQuestionRound(summary, receiptSHA256)
	if err != nil {
		return MinimizationQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return MinimizationQuestionRoundVerificationSummary{}, errors.New("minimization question round encoding failed")
	}
	if err := writeQuestionArtifact(roundPath, append(data, '\n'), "minimization question round"); err != nil {
		return MinimizationQuestionRoundVerificationSummary{}, err
	}
	digest, err := MinimizationQuestionRoundSHA256(round)
	if err != nil {
		return MinimizationQuestionRoundVerificationSummary{}, err
	}
	return questionRoundSummary(round, digest), nil
}

// ReadMinimizationQuestionRound verifies and reads a saved question round.
func ReadMinimizationQuestionRound(path string) (MinimizationQuestionRound, MinimizationQuestionRoundVerificationSummary, error) {
	data, err := readQuestionArtifact(path, "minimization question round")
	if err != nil {
		return MinimizationQuestionRound{}, MinimizationQuestionRoundVerificationSummary{}, err
	}
	round, err := DecodeMinimizationQuestionRound(data)
	if err != nil {
		return MinimizationQuestionRound{}, MinimizationQuestionRoundVerificationSummary{}, err
	}
	digest, err := MinimizationQuestionRoundSHA256(round)
	if err != nil {
		return MinimizationQuestionRound{}, MinimizationQuestionRoundVerificationSummary{}, err
	}
	return round, questionRoundSummary(round, digest), nil
}

// VerifyMinimizationQuestionRound verifies a saved round without reopening
// the source minimization run.
func VerifyMinimizationQuestionRound(path string) (MinimizationQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadMinimizationQuestionRound(path)
	return summary, err
}

// DecodeMinimizationQuestionRound verifies one bounded question-round value.
func DecodeMinimizationQuestionRound(data []byte) (MinimizationQuestionRound, error) {
	if len(data) == 0 {
		return MinimizationQuestionRound{}, errors.New("minimization question round is empty")
	}
	if len(data) > questionArtifactMaxBytes {
		return MinimizationQuestionRound{}, fmt.Errorf("minimization question round exceeds %d-byte limit", questionArtifactMaxBytes)
	}
	if !utf8.Valid(data) {
		return MinimizationQuestionRound{}, errors.New("minimization question round must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return MinimizationQuestionRound{}, fmt.Errorf("minimization question round: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round MinimizationQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return MinimizationQuestionRound{}, fmt.Errorf("minimization question round: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MinimizationQuestionRound{}, errors.New("minimization question round: trailing data")
	}
	if err := validateQuestionRound(round); err != nil {
		return MinimizationQuestionRound{}, fmt.Errorf("minimization question round: %w", err)
	}
	return round, nil
}

// MinimizationQuestionRoundSHA256 returns the canonical identity of a valid
// question round.
func MinimizationQuestionRoundSHA256(round MinimizationQuestionRound) (string, error) {
	if err := validateQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("minimization question round encoding failed")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// AskMinimizationQuestionReceipt returns one selected receipt from a saved
// minimization question round.
func AskMinimizationQuestionReceipt(roundPath, questionID string) (MinimizationQuestionReceipt, error) {
	round, summary, err := ReadMinimizationQuestionRound(roundPath)
	if err != nil {
		return MinimizationQuestionReceipt{}, err
	}
	answer, err := minimizationQuestionAnswerFromRound(round, questionID)
	if err != nil {
		return MinimizationQuestionReceipt{}, err
	}
	return MinimizationQuestionReceipt{MinimizationQuestionAnswer: answer, RoundSHA256: summary.RoundSHA256, Round: round}, nil
}

// SaveMinimizationQuestionReceipt selects one fixed answer and writes a
// portable receipt without overwriting an existing path.
func SaveMinimizationQuestionReceipt(roundPath, questionID, receiptPath string) (MinimizationQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return MinimizationQuestionReceiptVerificationSummary{}, errors.New("minimization question receipt path is required")
	}
	receipt, err := AskMinimizationQuestionReceipt(roundPath, questionID)
	if err != nil {
		return MinimizationQuestionReceiptVerificationSummary{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return MinimizationQuestionReceiptVerificationSummary{}, errors.New("minimization question receipt encoding failed")
	}
	if err := writeQuestionArtifact(receiptPath, append(data, '\n'), "minimization question receipt"); err != nil {
		return MinimizationQuestionReceiptVerificationSummary{}, err
	}
	digest, err := MinimizationQuestionReceiptSHA256(receipt)
	if err != nil {
		return MinimizationQuestionReceiptVerificationSummary{}, err
	}
	return questionReceiptSummary(receipt, digest), nil
}

// ReadMinimizationQuestionReceipt verifies and reads a selected receipt.
func ReadMinimizationQuestionReceipt(path string) (MinimizationQuestionReceipt, MinimizationQuestionReceiptVerificationSummary, error) {
	data, err := readQuestionArtifact(path, "minimization question receipt")
	if err != nil {
		return MinimizationQuestionReceipt{}, MinimizationQuestionReceiptVerificationSummary{}, err
	}
	receipt, err := DecodeMinimizationQuestionReceipt(data)
	if err != nil {
		return MinimizationQuestionReceipt{}, MinimizationQuestionReceiptVerificationSummary{}, err
	}
	digest, err := MinimizationQuestionReceiptSHA256(receipt)
	if err != nil {
		return MinimizationQuestionReceipt{}, MinimizationQuestionReceiptVerificationSummary{}, err
	}
	return receipt, questionReceiptSummary(receipt, digest), nil
}

// VerifyMinimizationQuestionReceipt verifies a receipt without reopening its
// source minimization run or question round.
func VerifyMinimizationQuestionReceipt(path string) (MinimizationQuestionReceiptVerificationSummary, error) {
	_, summary, err := ReadMinimizationQuestionReceipt(path)
	return summary, err
}

// DecodeMinimizationQuestionReceipt verifies one bounded selected receipt.
func DecodeMinimizationQuestionReceipt(data []byte) (MinimizationQuestionReceipt, error) {
	if len(data) == 0 {
		return MinimizationQuestionReceipt{}, errors.New("minimization question receipt is empty")
	}
	if len(data) > questionArtifactMaxBytes {
		return MinimizationQuestionReceipt{}, fmt.Errorf("minimization question receipt exceeds %d-byte limit", questionArtifactMaxBytes)
	}
	if !utf8.Valid(data) {
		return MinimizationQuestionReceipt{}, errors.New("minimization question receipt must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return MinimizationQuestionReceipt{}, fmt.Errorf("minimization question receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt MinimizationQuestionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return MinimizationQuestionReceipt{}, fmt.Errorf("minimization question receipt: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MinimizationQuestionReceipt{}, errors.New("minimization question receipt: trailing data")
	}
	if err := validateQuestionReceipt(receipt); err != nil {
		return MinimizationQuestionReceipt{}, fmt.Errorf("minimization question receipt: %w", err)
	}
	return receipt, nil
}

// MinimizationQuestionReceiptSHA256 returns the canonical identity of a valid
// selected receipt.
func MinimizationQuestionReceiptSHA256(receipt MinimizationQuestionReceipt) (string, error) {
	if err := validateQuestionReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("minimization question receipt encoding failed")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func minimizationQuestionAnswer(summary MinimizationSummary, receiptSHA256 string, question MinimizationQuestion) MinimizationQuestionAnswer {
	supported, unknown := candidateSupportCounts(summary.CandidateResults)
	evidenceState := evidence.Unknown
	if unknown == 0 {
		evidenceState = evidence.Observed
	}
	answer := MinimizationQuestionAnswer{
		SchemaVersion:       QuestionRoundSchemaVersion,
		QuestionID:          question.ID,
		Question:            question.Text,
		EvidenceState:       evidenceState,
		MinimizationSHA256:  receiptSHA256,
		SelectionState:      summary.SelectionState,
		SelectedCandidate:   summary.SelectedCandidate,
		CandidateCount:      len(summary.CandidateResults),
		SupportedCandidates: supported,
		UnknownCandidates:   unknown,
	}
	switch question.ID {
	case MinimizationQuestionSelection:
		answer.Result = string(summary.SelectionState)
	case MinimizationQuestionSupport:
		answer.Result = "supported"
		if unknown > 0 {
			answer.Result = "unknown"
		}
	}
	answer.Reason = minimizationQuestionReason(answer)
	return answer
}

func minimizationQuestionReason(answer MinimizationQuestionAnswer) string {
	switch answer.QuestionID {
	case MinimizationQuestionSelection:
		switch answer.SelectionState {
		case SelectionSelected:
			return "the tested ladder established a sufficient candidate at its least-disclosing selected position"
		case SelectionNoSufficient:
			return "every tested candidate changed the fixed functionality criterion"
		default:
			return "at least one candidate lacks complete replicated support or observed evidence"
		}
	case MinimizationQuestionSupport:
		if answer.Result == "supported" {
			return "every candidate has complete replicated support and observed evidence"
		}
		return "at least one candidate lacks complete replicated support or observed evidence"
	default:
		return ""
	}
}

func candidateProjection(result CandidateResult) MinimizationCandidateProjection {
	return MinimizationCandidateProjection{
		ID:             result.ID,
		Classification: result.Classification,
		Outcome:        result.Outcome,
		EvidenceState:  result.EvidenceState,
		ReceiptSHA256:  result.ReceiptSHA256,
		Pairs:          result.Pairs,
		PairsPerOrder:  result.PairsPerOrder,
		CompletedPairs: result.CompletedPairs,
		ChangedPairs:   result.ChangedPairs,
		NoChangePairs:  result.NoChangePairs,
		UnknownPairs:   result.UnknownPairs,
	}
}

func candidateSupportCounts(results []CandidateResult) (supported, unknown int) {
	for _, result := range results {
		if result.CompletedPairs == result.Pairs && result.EvidenceState == evidence.Observed && result.Outcome != bundle.ReplicationUnknown {
			supported++
		} else {
			unknown++
		}
	}
	return supported, unknown
}

func minimizationQuestion(id string) (MinimizationQuestion, bool) {
	for _, question := range MinimizationQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return MinimizationQuestion{}, false
}

func minimizationQuestionAnswerFromRound(round MinimizationQuestionRound, questionID string) (MinimizationQuestionAnswer, error) {
	index := -1
	for i, question := range MinimizationQuestions() {
		if question.ID == questionID {
			index = i
			break
		}
	}
	if index < 0 {
		return MinimizationQuestionAnswer{}, errors.New("minimization question ID is invalid")
	}
	return round.Answers[index], nil
}

func questionRoundSummary(round MinimizationQuestionRound, digest string) MinimizationQuestionRoundVerificationSummary {
	return MinimizationQuestionRoundVerificationSummary{
		SchemaVersion:      round.SchemaVersion,
		MinimizationSHA256: round.MinimizationSHA256,
		Questions:          len(round.Answers),
		Candidates:         len(round.Candidates),
		RoundSHA256:        digest,
	}
}

func questionReceiptSummary(receipt MinimizationQuestionReceipt, digest string) MinimizationQuestionReceiptVerificationSummary {
	return MinimizationQuestionReceiptVerificationSummary{
		SchemaVersion:       receipt.SchemaVersion,
		QuestionID:          receipt.QuestionID,
		Question:            receipt.Question,
		Result:              receipt.Result,
		EvidenceState:       receipt.EvidenceState,
		MinimizationSHA256:  receipt.MinimizationSHA256,
		RoundSHA256:         receipt.RoundSHA256,
		ReceiptSHA256:       digest,
		SelectionState:      receipt.SelectionState,
		SelectedCandidate:   receipt.SelectedCandidate,
		CandidateCount:      receipt.CandidateCount,
		SupportedCandidates: receipt.SupportedCandidates,
		UnknownCandidates:   receipt.UnknownCandidates,
	}
}

func validateQuestionRound(round MinimizationQuestionRound) error {
	if round.SchemaVersion != QuestionRoundSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", round.SchemaVersion)
	}
	if !validDigest(round.MinimizationSHA256) {
		return errors.New("minimization_sha256 is invalid")
	}
	if len(round.Candidates) < 1 || len(round.Candidates) >= maxCandidates {
		return errors.New("candidate count is invalid")
	}
	seen := make(map[string]struct{}, len(round.Candidates))
	for index, candidate := range round.Candidates {
		if err := validateCandidateProjection(candidate); err != nil {
			return fmt.Errorf("candidate %d: %w", index+1, err)
		}
		if _, ok := seen[candidate.ID]; ok {
			return errors.New("candidate IDs are duplicated")
		}
		seen[candidate.ID] = struct{}{}
	}
	questions := MinimizationQuestions()
	if len(round.Answers) != len(questions) {
		return errors.New("answer count does not match catalog")
	}
	supported, unknown := candidateSupportCountsProjection(round.Candidates)
	selection, selected := selectionForProjections(round.Candidates)
	for index, answer := range round.Answers {
		if err := validateQuestionAnswer(answer, questions[index], round.MinimizationSHA256, len(round.Candidates), supported, unknown, selection, selected); err != nil {
			return fmt.Errorf("answer %d: %w", index+1, err)
		}
	}
	return nil
}

func classifyCandidateProjection(candidate MinimizationCandidateProjection) (bundle.ReplicatedOutcome, CandidateClassification) {
	if candidate.CompletedPairs != candidate.Pairs || candidate.UnknownPairs > 0 || candidate.EvidenceState != evidence.Observed {
		return bundle.ReplicationUnknown, CandidateUnknown
	}
	switch {
	case candidate.ChangedPairs == candidate.Pairs:
		return bundle.ReplicatedChange, CandidateInsufficient
	case candidate.NoChangePairs == candidate.Pairs:
		return bundle.NoChangeObserved, CandidateSufficient
	default:
		return bundle.MixedInconsistent, CandidateMixedInconsistent
	}
}

func validateCandidateProjection(candidate MinimizationCandidateProjection) error {
	if !validIdentifier(candidate.ID, maxCandidateID) {
		return errors.New("id is invalid")
	}
	if !validDigest(candidate.ReceiptSHA256) {
		return errors.New("receipt_sha256 is invalid")
	}
	if candidate.PairsPerOrder < 1 || candidate.PairsPerOrder > 8 || candidate.Pairs != candidate.PairsPerOrder*2 || candidate.CompletedPairs < 0 || candidate.CompletedPairs > candidate.Pairs || candidate.ChangedPairs < 0 || candidate.NoChangePairs < 0 || candidate.UnknownPairs < 0 || candidate.ChangedPairs+candidate.NoChangePairs+candidate.UnknownPairs != candidate.Pairs {
		return errors.New("counts are invalid")
	}
	if !candidate.EvidenceState.Valid() {
		return errors.New("evidence_state is invalid")
	}
	switch candidate.Outcome {
	case bundle.ReplicatedChange, bundle.NoChangeObserved, bundle.MixedInconsistent, bundle.ReplicationUnknown:
	default:
		return errors.New("outcome is invalid")
	}
	switch candidate.Classification {
	case CandidateSufficient, CandidateInsufficient, CandidateMixedInconsistent, CandidateUnknown:
	default:
		return errors.New("classification is invalid")
	}
	expectedOutcome, expectedClassification := classifyCandidateProjection(candidate)
	if candidate.Outcome != expectedOutcome {
		return errors.New("outcome disagrees with counts")
	}
	if candidate.Classification != expectedClassification {
		return errors.New("classification disagrees with outcome")
	}
	return nil
}

func validateQuestionAnswer(answer MinimizationQuestionAnswer, question MinimizationQuestion, minimizationSHA256 string, candidateCount, supported, unknown int, selection SelectionState, selected string) error {
	if answer.SchemaVersion != QuestionRoundSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text || answer.MinimizationSHA256 != minimizationSHA256 {
		return errors.New("question identity is invalid")
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("evidence_state is invalid")
	}
	if answer.SelectionState != selection || answer.SelectedCandidate != selected || answer.CandidateCount != candidateCount || answer.SupportedCandidates != supported || answer.UnknownCandidates != unknown {
		return errors.New("shared answer metrics are invalid")
	}
	if answer.SelectionState != SelectionSelected && answer.SelectedCandidate != "" {
		return errors.New("selected_candidate is invalid")
	}
	if supported < 0 || unknown < 0 || supported+unknown != candidateCount {
		return errors.New("candidate support counts are invalid")
	}
	expectedEvidence := evidence.Unknown
	if unknown == 0 {
		expectedEvidence = evidence.Observed
	}
	if answer.EvidenceState != expectedEvidence {
		return errors.New("answer evidence state does not match candidate support")
	}
	switch answer.QuestionID {
	case MinimizationQuestionSelection:
		if answer.Result != string(selection) {
			return errors.New("selection result is invalid")
		}
	case MinimizationQuestionSupport:
		expected := "supported"
		if unknown > 0 {
			expected = "unknown"
		}
		if answer.Result != expected {
			return errors.New("support result is invalid")
		}
	default:
		return errors.New("question ID is invalid")
	}
	if answer.Reason != minimizationQuestionReason(answer) {
		return errors.New("answer reason is invalid")
	}
	return nil
}

func selectionForProjections(candidates []MinimizationCandidateProjection) (SelectionState, string) {
	selection := SelectionNoSufficient
	selected := ""
	for _, candidate := range candidates {
		if candidate.Classification == CandidateUnknown || candidate.Classification == CandidateMixedInconsistent {
			selection = SelectionUnknown
		}
		if candidate.Classification == CandidateSufficient {
			selected = candidate.ID
		}
	}
	if selection != SelectionUnknown && selected != "" {
		selection = SelectionSelected
	}
	if selection != SelectionSelected {
		selected = ""
	}
	return selection, selected
}

func candidateSupportCountsProjection(candidates []MinimizationCandidateProjection) (supported, unknown int) {
	for _, candidate := range candidates {
		if candidate.CompletedPairs == candidate.Pairs && candidate.EvidenceState == evidence.Observed && candidate.Outcome != bundle.ReplicationUnknown {
			supported++
		} else {
			unknown++
		}
	}
	return supported, unknown
}

func validateQuestionReceipt(receipt MinimizationQuestionReceipt) error {
	if receipt.SchemaVersion != QuestionReceiptSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", receipt.SchemaVersion)
	}
	if err := validateQuestionRound(receipt.Round); err != nil {
		return fmt.Errorf("round: %w", err)
	}
	if !validDigest(receipt.RoundSHA256) {
		return errors.New("round_sha256 is invalid")
	}
	roundSHA256, err := MinimizationQuestionRoundSHA256(receipt.Round)
	if err != nil || roundSHA256 != receipt.RoundSHA256 {
		return errors.New("round identity does not match round")
	}
	index := -1
	for i, question := range MinimizationQuestions() {
		if question.ID == receipt.QuestionID {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("question_id is invalid")
	}
	answer := receipt.Round.Answers[index]
	if receipt.MinimizationQuestionAnswer != answer {
		return errors.New("answer does not match round")
	}
	return nil
}

func readQuestionArtifact(path, kind string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%s path is required", kind)
	}
	data, err := bundle.ReadBoundedFile(path, questionArtifactMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	return data, nil
}

func writeQuestionArtifact(path string, data []byte, kind string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%s path is required", kind)
	}
	if len(data) > questionArtifactMaxBytes {
		return fmt.Errorf("%s exceeds %d-byte limit", kind, questionArtifactMaxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("%s: create output directory: %w", kind, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("%s: create output: %w", kind, err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("%s: write output: %w", kind, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("%s: sync output: %w", kind, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("%s: close output: %w", kind, err)
	}
	remove = false
	return nil
}
