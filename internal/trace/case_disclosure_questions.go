package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	caseDisclosureQuestionRoundSchemaVersion   = 1
	caseDisclosureQuestionReceiptSchemaVersion = 1
)

const (
	// CaseDisclosureQuestionCoverage asks whether every retained trace declared complete coverage.
	CaseDisclosureQuestionCoverage = "disclosure-map-coverage"
	// CaseDisclosureQuestionOverlap asks which categories appeared across source or adapter boundaries.
	CaseDisclosureQuestionOverlap = "cross-boundary-category-overlap"
)

const (
	caseDisclosureResultComplete   = "complete"
	caseDisclosureResultOverlap    = "overlap-observed"
	caseDisclosureResultNoOverlap  = "no-overlap-observed"
	caseDisclosureResultUnknown    = "unknown"
	caseDisclosureEvidenceUnknown  = evidence.Unknown
	caseDisclosureEvidenceObserved = evidence.Observed
)

// CaseDisclosureQuestion is one fixed, bounded question available for a
// verified case disclosure map.
type CaseDisclosureQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// CaseDisclosureQuestions returns the stable disclosure-map question catalog.
func CaseDisclosureQuestions() []CaseDisclosureQuestion {
	return []CaseDisclosureQuestion{
		{ID: CaseDisclosureQuestionCoverage, Text: "Did every retained trace declare complete coverage?"},
		{ID: CaseDisclosureQuestionOverlap, Text: "Which reviewed categories appeared across multiple source or adapter boundaries?"},
	}
}

// CaseDisclosureBoundary identifies one reviewed source and adapter boundary.
type CaseDisclosureBoundary struct {
	Source  string `json:"source"`
	Adapter string `json:"adapter"`
}

// CaseDisclosureCategorySummary lists the reviewed boundaries for one safe
// category label.
type CaseDisclosureCategorySummary struct {
	Category   string                   `json:"category"`
	Boundaries []CaseDisclosureBoundary `json:"boundaries"`
}

// CaseDisclosureQuestionAnswer is a raw-value-free answer tied to one case.
// Question result and evidence state remain separate fields.
type CaseDisclosureQuestionAnswer struct {
	SchemaVersion         int                             `json:"schema_version"`
	QuestionID            string                          `json:"question_id"`
	Question              string                          `json:"question"`
	Result                string                          `json:"result"`
	EvidenceState         evidence.State                  `json:"evidence_state"`
	Reason                string                          `json:"reason,omitempty"`
	CaseSHA256            string                          `json:"case_sha256"`
	Traces                int                             `json:"traces"`
	CoverageState         evidence.State                  `json:"coverage_state"`
	Categories            []CaseDisclosureCategorySummary `json:"categories"`
	OverlappingCategories []string                        `json:"overlapping_categories"`
}

// CaseDisclosureQuestionRound retains every fixed disclosure-map answer
// without retaining the verified case or any source-specific paths.
type CaseDisclosureQuestionRound struct {
	SchemaVersion int                             `json:"schema_version"`
	OrderBasis    string                          `json:"order_basis"`
	CaseSHA256    string                          `json:"case_sha256"`
	Traces        int                             `json:"traces"`
	CoverageState evidence.State                  `json:"coverage_state"`
	Categories    []CaseDisclosureCategorySummary `json:"categories"`
	Answers       []CaseDisclosureQuestionAnswer  `json:"answers"`
}

// CaseDisclosureQuestionRoundVerificationSummary identifies a valid saved
// disclosure-map question round.
type CaseDisclosureQuestionRoundVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	CaseSHA256    string `json:"case_sha256"`
	Questions     int    `json:"questions"`
	RoundSHA256   string `json:"round_sha256"`
}

// CaseDisclosureQuestionReceipt is one selected answer bound to a complete
// disclosure-map question round.
type CaseDisclosureQuestionReceipt struct {
	CaseDisclosureQuestionAnswer
	RoundSHA256 string                      `json:"round_sha256"`
	Round       CaseDisclosureQuestionRound `json:"round"`
}

// CaseDisclosureQuestionReceiptVerificationSummary identifies a valid saved
// disclosure-map answer receipt.
type CaseDisclosureQuestionReceiptVerificationSummary struct {
	SchemaVersion int            `json:"schema_version"`
	QuestionID    string         `json:"question_id"`
	Question      string         `json:"question"`
	Result        string         `json:"result"`
	EvidenceState evidence.State `json:"evidence_state"`
	CaseSHA256    string         `json:"case_sha256"`
	RoundSHA256   string         `json:"round_sha256"`
	ReceiptSHA256 string         `json:"receipt_sha256"`
}

// AnswerCaseDisclosureQuestionRound answers the complete fixed catalog from
// an already verified case and summary.
func AnswerCaseDisclosureQuestionRound(casePackage CasePackage, summary CaseVerificationSummary) (CaseDisclosureQuestionRound, error) {
	expectedSummary, err := caseSummary(casePackage)
	if err != nil {
		return CaseDisclosureQuestionRound{}, err
	}
	if !reflect.DeepEqual(expectedSummary, summary) {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round case identity does not match summary")
	}
	mapResult, err := BuildCaseDisclosureMap(casePackage, summary)
	if err != nil {
		return CaseDisclosureQuestionRound{}, err
	}
	categories := disclosureQuestionCategories(mapResult)
	answers := make([]CaseDisclosureQuestionAnswer, 0, len(CaseDisclosureQuestions()))
	for _, question := range CaseDisclosureQuestions() {
		answers = append(answers, answerCaseDisclosureQuestion(mapResult, categories, question))
	}
	round := CaseDisclosureQuestionRound{
		SchemaVersion: caseDisclosureQuestionRoundSchemaVersion,
		OrderBasis:    summary.OrderBasis,
		CaseSHA256:    summary.CaseSHA256,
		Traces:        mapResult.Traces,
		CoverageState: mapResult.CoverageState,
		Categories:    cloneDisclosureCategories(categories),
		Answers:       answers,
	}
	if err := validateCaseDisclosureQuestionRound(round); err != nil {
		return CaseDisclosureQuestionRound{}, err
	}
	return round, nil
}

// SaveCaseDisclosureQuestionRound verifies a case, answers every fixed
// question, and writes a portable round without overwriting an existing path.
func SaveCaseDisclosureQuestionRound(casePath, roundPath string) (CaseDisclosureQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("trace disclosure-map question round path is required")
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		return CaseDisclosureQuestionRoundVerificationSummary{}, err
	}
	round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
	if err != nil {
		return CaseDisclosureQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := CaseDisclosureQuestionRoundSHA256(round)
	if err != nil {
		return CaseDisclosureQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("trace disclosure-map question round encoding failed")
	}
	if err := writeCaseExclusive(roundPath, append(data, '\n')); err != nil {
		return CaseDisclosureQuestionRoundVerificationSummary{}, fmt.Errorf("trace disclosure-map question round: %w", err)
	}
	return caseDisclosureQuestionRoundSummary(round, roundSHA256), nil
}

// ReadCaseDisclosureQuestionRound verifies and reads a saved disclosure-map
// question round.
func ReadCaseDisclosureQuestionRound(path string) (CaseDisclosureQuestionRound, CaseDisclosureQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return CaseDisclosureQuestionRound{}, CaseDisclosureQuestionRoundVerificationSummary{}, errors.New("trace disclosure-map question round path is required")
	}
	data, err := readCase(path)
	if err != nil {
		return CaseDisclosureQuestionRound{}, CaseDisclosureQuestionRoundVerificationSummary{}, fmt.Errorf("trace disclosure-map question round: %w", err)
	}
	round, err := DecodeCaseDisclosureQuestionRound(data)
	if err != nil {
		return CaseDisclosureQuestionRound{}, CaseDisclosureQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := CaseDisclosureQuestionRoundSHA256(round)
	if err != nil {
		return CaseDisclosureQuestionRound{}, CaseDisclosureQuestionRoundVerificationSummary{}, err
	}
	return round, caseDisclosureQuestionRoundSummary(round, roundSHA256), nil
}

// VerifyCaseDisclosureQuestionRound verifies a saved round without reopening
// the source case.
func VerifyCaseDisclosureQuestionRound(path string) (CaseDisclosureQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadCaseDisclosureQuestionRound(path)
	return summary, err
}

// DecodeCaseDisclosureQuestionRound verifies one bounded question-round
// document.
func DecodeCaseDisclosureQuestionRound(data []byte) (CaseDisclosureQuestionRound, error) {
	if len(data) == 0 {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round is empty")
	}
	if len(data) > maxCaseBytes {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round exceeds 4194304-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round CaseDisclosureQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseDisclosureQuestionRound{}, errors.New("trace disclosure-map question round has trailing data")
	}
	if err := validateCaseDisclosureQuestionRound(round); err != nil {
		return CaseDisclosureQuestionRound{}, err
	}
	return round, nil
}

// CaseDisclosureQuestionRoundSHA256 returns the canonical identity of a valid
// disclosure-map question round.
func CaseDisclosureQuestionRoundSHA256(round CaseDisclosureQuestionRound) (string, error) {
	if err := validateCaseDisclosureQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("trace disclosure-map question round encoding failed")
	}
	return sha256HexCase(data), nil
}

// AskCaseDisclosureQuestionRound returns one fixed answer from a saved round.
func AskCaseDisclosureQuestionRound(path, questionID string) (CaseDisclosureQuestionAnswer, error) {
	round, _, err := ReadCaseDisclosureQuestionRound(path)
	if err != nil {
		return CaseDisclosureQuestionAnswer{}, err
	}
	return caseDisclosureQuestionAnswerFromRound(round, questionID)
}

// SaveCaseDisclosureQuestionReceipt selects one fixed answer from a saved
// round and writes a portable receipt without overwriting an existing path.
func SaveCaseDisclosureQuestionReceipt(roundPath, questionID, receiptPath string) (CaseDisclosureQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, errors.New("trace disclosure-map question receipt path is required")
	}
	round, roundSummary, err := ReadCaseDisclosureQuestionRound(roundPath)
	if err != nil {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, err
	}
	answer, err := caseDisclosureQuestionAnswerFromRound(round, questionID)
	if err != nil {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, err
	}
	receipt := CaseDisclosureQuestionReceipt{
		CaseDisclosureQuestionAnswer: answer,
		RoundSHA256:                  roundSummary.RoundSHA256,
		Round:                        round,
	}
	receiptSHA256, err := CaseDisclosureQuestionReceiptSHA256(receipt)
	if err != nil {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, errors.New("trace disclosure-map question receipt encoding failed")
	}
	if err := writeCaseExclusive(receiptPath, append(data, '\n')); err != nil {
		return CaseDisclosureQuestionReceiptVerificationSummary{}, fmt.Errorf("trace disclosure-map question receipt: %w", err)
	}
	return caseDisclosureQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// ReadCaseDisclosureQuestionReceipt verifies and reads a saved answer receipt.
func ReadCaseDisclosureQuestionReceipt(path string) (CaseDisclosureQuestionReceipt, CaseDisclosureQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return CaseDisclosureQuestionReceipt{}, CaseDisclosureQuestionReceiptVerificationSummary{}, errors.New("trace disclosure-map question receipt path is required")
	}
	data, err := readCase(path)
	if err != nil {
		return CaseDisclosureQuestionReceipt{}, CaseDisclosureQuestionReceiptVerificationSummary{}, fmt.Errorf("trace disclosure-map question receipt: %w", err)
	}
	receipt, err := DecodeCaseDisclosureQuestionReceipt(data)
	if err != nil {
		return CaseDisclosureQuestionReceipt{}, CaseDisclosureQuestionReceiptVerificationSummary{}, err
	}
	receiptSHA256, err := CaseDisclosureQuestionReceiptSHA256(receipt)
	if err != nil {
		return CaseDisclosureQuestionReceipt{}, CaseDisclosureQuestionReceiptVerificationSummary{}, err
	}
	return receipt, caseDisclosureQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// VerifyCaseDisclosureQuestionReceipt verifies a saved receipt without
// reopening the source case or question round.
func VerifyCaseDisclosureQuestionReceipt(path string) (CaseDisclosureQuestionReceiptVerificationSummary, error) {
	_, summary, err := ReadCaseDisclosureQuestionReceipt(path)
	return summary, err
}

// DecodeCaseDisclosureQuestionReceipt verifies one bounded answer receipt.
func DecodeCaseDisclosureQuestionReceipt(data []byte) (CaseDisclosureQuestionReceipt, error) {
	if len(data) == 0 {
		return CaseDisclosureQuestionReceipt{}, errors.New("trace disclosure-map question receipt is empty")
	}
	if len(data) > maxCaseBytes {
		return CaseDisclosureQuestionReceipt{}, errors.New("trace disclosure-map question receipt exceeds 4194304-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return CaseDisclosureQuestionReceipt{}, errors.New("trace disclosure-map question receipt has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt CaseDisclosureQuestionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return CaseDisclosureQuestionReceipt{}, errors.New("trace disclosure-map question receipt has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseDisclosureQuestionReceipt{}, errors.New("trace disclosure-map question receipt has trailing data")
	}
	if err := validateCaseDisclosureQuestionReceipt(receipt); err != nil {
		return CaseDisclosureQuestionReceipt{}, err
	}
	return receipt, nil
}

// CaseDisclosureQuestionReceiptSHA256 returns the canonical identity of a
// valid disclosure-map question receipt.
func CaseDisclosureQuestionReceiptSHA256(receipt CaseDisclosureQuestionReceipt) (string, error) {
	if err := validateCaseDisclosureQuestionReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("trace disclosure-map question receipt encoding failed")
	}
	return sha256HexCase(data), nil
}

// AskCaseDisclosureQuestionReceipt returns one selected receipt from a saved
// disclosure-map question round.
func AskCaseDisclosureQuestionReceipt(roundPath, questionID string) (CaseDisclosureQuestionReceipt, error) {
	round, roundSummary, err := ReadCaseDisclosureQuestionRound(roundPath)
	if err != nil {
		return CaseDisclosureQuestionReceipt{}, err
	}
	answer, err := caseDisclosureQuestionAnswerFromRound(round, questionID)
	if err != nil {
		return CaseDisclosureQuestionReceipt{}, err
	}
	return CaseDisclosureQuestionReceipt{
		CaseDisclosureQuestionAnswer: answer,
		RoundSHA256:                  roundSummary.RoundSHA256,
		Round:                        round,
	}, nil
}

func answerCaseDisclosureQuestion(mapResult CaseDisclosureMap, categories []CaseDisclosureCategorySummary, question CaseDisclosureQuestion) CaseDisclosureQuestionAnswer {
	answer := CaseDisclosureQuestionAnswer{
		SchemaVersion:         caseDisclosureQuestionRoundSchemaVersion,
		QuestionID:            question.ID,
		Question:              question.Text,
		CaseSHA256:            mapResult.CaseSHA256,
		Traces:                mapResult.Traces,
		CoverageState:         mapResult.CoverageState,
		Categories:            cloneDisclosureCategories(categories),
		OverlappingCategories: disclosureOverlapCategories(categories),
		EvidenceState:         caseDisclosureEvidenceUnknown,
		Result:                caseDisclosureResultUnknown,
	}
	switch question.ID {
	case CaseDisclosureQuestionCoverage:
		if mapResult.CoverageState == evidence.Observed {
			answer.Result = caseDisclosureResultComplete
			answer.EvidenceState = caseDisclosureEvidenceObserved
			answer.Reason = "every retained trace declared complete coverage"
		} else {
			answer.Reason = "partial trace coverage prevents a complete-coverage conclusion"
		}
	case CaseDisclosureQuestionOverlap:
		switch {
		case len(answer.OverlappingCategories) > 0:
			answer.Result = caseDisclosureResultOverlap
			answer.EvidenceState = caseDisclosureEvidenceObserved
			answer.Reason = "one or more reviewed categories appeared across multiple source or adapter boundaries"
		case mapResult.CoverageState == evidence.Unknown:
			answer.Reason = "no cross-boundary overlap was observed, but partial coverage prevents a no-overlap conclusion"
		default:
			answer.Result = caseDisclosureResultNoOverlap
			answer.EvidenceState = caseDisclosureEvidenceObserved
			answer.Reason = "no reviewed category appeared across multiple source or adapter boundaries"
		}
	}
	return answer
}

func disclosureQuestionCategories(mapResult CaseDisclosureMap) []CaseDisclosureCategorySummary {
	categories := make([]CaseDisclosureCategorySummary, 0, len(mapResult.Categories))
	for _, category := range mapResult.Categories {
		boundaries := make([]CaseDisclosureBoundary, 0)
		seen := make(map[string]struct{})
		for _, observation := range category.Observations {
			key := observation.Source + "\x00" + observation.Adapter
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			boundaries = append(boundaries, CaseDisclosureBoundary{Source: observation.Source, Adapter: observation.Adapter})
		}
		slices.SortFunc(boundaries, compareDisclosureBoundaries)
		categories = append(categories, CaseDisclosureCategorySummary{Category: category.Category, Boundaries: boundaries})
	}
	return categories
}

func disclosureOverlapCategories(categories []CaseDisclosureCategorySummary) []string {
	overlaps := make([]string, 0)
	for _, category := range categories {
		if len(category.Boundaries) >= 2 {
			overlaps = append(overlaps, category.Category)
		}
	}
	return overlaps
}

func cloneDisclosureCategories(categories []CaseDisclosureCategorySummary) []CaseDisclosureCategorySummary {
	clone := make([]CaseDisclosureCategorySummary, 0, len(categories))
	for _, category := range categories {
		clone = append(clone, CaseDisclosureCategorySummary{
			Category:   category.Category,
			Boundaries: append([]CaseDisclosureBoundary(nil), category.Boundaries...),
		})
	}
	return clone
}

func caseDisclosureQuestionAnswerFromRound(round CaseDisclosureQuestionRound, questionID string) (CaseDisclosureQuestionAnswer, error) {
	index, ok := caseDisclosureQuestionIndex(questionID)
	if !ok {
		return CaseDisclosureQuestionAnswer{}, errors.New("trace disclosure-map question ID is invalid")
	}
	return round.Answers[index], nil
}

func caseDisclosureQuestionRoundSummary(round CaseDisclosureQuestionRound, digest string) CaseDisclosureQuestionRoundVerificationSummary {
	return CaseDisclosureQuestionRoundVerificationSummary{
		SchemaVersion: round.SchemaVersion,
		CaseSHA256:    round.CaseSHA256,
		Questions:     len(round.Answers),
		RoundSHA256:   digest,
	}
}

func caseDisclosureQuestionReceiptSummary(receipt CaseDisclosureQuestionReceipt, digest string) CaseDisclosureQuestionReceiptVerificationSummary {
	return CaseDisclosureQuestionReceiptVerificationSummary{
		SchemaVersion: receipt.SchemaVersion,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		CaseSHA256:    receipt.CaseSHA256,
		RoundSHA256:   receipt.RoundSHA256,
		ReceiptSHA256: digest,
	}
}

func caseDisclosureQuestionIndex(questionID string) (int, bool) {
	for index, question := range CaseDisclosureQuestions() {
		if question.ID == questionID {
			return index, true
		}
	}
	return 0, false
}

func validateCaseDisclosureQuestionRound(round CaseDisclosureQuestionRound) error {
	if round.SchemaVersion != caseDisclosureQuestionRoundSchemaVersion {
		return errors.New("trace disclosure-map question round has unsupported schema_version")
	}
	if round.OrderBasis != "caller" {
		return errors.New("trace disclosure-map question round order_basis is invalid")
	}
	if !ValidSHA256(round.CaseSHA256) {
		return errors.New("trace disclosure-map question round case_sha256 is invalid")
	}
	if round.Traces <= 0 || round.Traces > maxCaseSummaryEntries {
		return errors.New("trace disclosure-map question round trace count is invalid")
	}
	if err := validateDisclosureCoverageState(round.CoverageState); err != nil {
		return err
	}
	if err := validateDisclosureCategories(round.Categories); err != nil {
		return err
	}
	questions := CaseDisclosureQuestions()
	if len(round.Answers) != len(questions) {
		return errors.New("trace disclosure-map question round answer count does not match catalog")
	}
	for index, answer := range round.Answers {
		if err := validateCaseDisclosureQuestionAnswer(answer, questions[index], round.CaseSHA256, round.Traces, round.CoverageState, round.Categories); err != nil {
			return fmt.Errorf("trace disclosure-map question round answer %d: %w", index+1, err)
		}
	}
	return nil
}

func validateCaseDisclosureQuestionReceipt(receipt CaseDisclosureQuestionReceipt) error {
	if receipt.SchemaVersion != caseDisclosureQuestionReceiptSchemaVersion {
		return errors.New("trace disclosure-map question receipt has unsupported schema_version")
	}
	question, ok := caseDisclosureQuestion(receipt.QuestionID)
	if !ok {
		return errors.New("trace disclosure-map question receipt question_id is invalid")
	}
	if err := validateCaseDisclosureQuestionAnswer(receipt.CaseDisclosureQuestionAnswer, question, receipt.CaseSHA256, receipt.Traces, receipt.CoverageState, receipt.Categories); err != nil {
		return fmt.Errorf("trace disclosure-map question receipt answer: %w", err)
	}
	if !ValidSHA256(receipt.RoundSHA256) {
		return errors.New("trace disclosure-map question receipt round_sha256 is invalid")
	}
	if err := validateCaseDisclosureQuestionRound(receipt.Round); err != nil {
		return fmt.Errorf("trace disclosure-map question receipt round: %w", err)
	}
	roundSHA256, err := CaseDisclosureQuestionRoundSHA256(receipt.Round)
	if err != nil || roundSHA256 != receipt.RoundSHA256 {
		return errors.New("trace disclosure-map question receipt round identity does not match round")
	}
	answer, err := caseDisclosureQuestionAnswerFromRound(receipt.Round, receipt.QuestionID)
	if err != nil || !reflect.DeepEqual(answer, receipt.CaseDisclosureQuestionAnswer) {
		return errors.New("trace disclosure-map question receipt answer does not match round")
	}
	return nil
}

func validateCaseDisclosureQuestionAnswer(answer CaseDisclosureQuestionAnswer, question CaseDisclosureQuestion, caseSHA256 string, traces int, coverageState evidence.State, categories []CaseDisclosureCategorySummary) error {
	if answer.SchemaVersion != caseDisclosureQuestionRoundSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text {
		return errors.New("question identity is invalid")
	}
	if !ValidSHA256(answer.CaseSHA256) || answer.CaseSHA256 != caseSHA256 {
		return errors.New("case identity is invalid")
	}
	if answer.Traces != traces || answer.CoverageState != coverageState || !reflect.DeepEqual(answer.Categories, categories) {
		return errors.New("answer map summary does not match round")
	}
	if err := validateDisclosureCoverageState(answer.CoverageState); err != nil {
		return err
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("evidence_state is invalid")
	}
	expectedOverlaps := disclosureOverlapCategories(categories)
	if !slices.Equal(answer.OverlappingCategories, expectedOverlaps) {
		return errors.New("overlapping categories are invalid")
	}
	if answer.Reason != disclosureQuestionReason(answer) {
		return errors.New("answer reason is invalid")
	}
	expectedResult, expectedEvidence := disclosureQuestionResult(answer.QuestionID, answer.CoverageState, expectedOverlaps)
	if answer.Result != expectedResult || answer.EvidenceState != expectedEvidence {
		return errors.New("answer result does not match map state")
	}
	return nil
}

func validateDisclosureCoverageState(state evidence.State) error {
	if state != evidence.Observed && state != evidence.Unknown {
		return errors.New("coverage_state is invalid")
	}
	return nil
}

func validateDisclosureCategories(categories []CaseDisclosureCategorySummary) error {
	if categories == nil || len(categories) > maxCaseSummaryEntries {
		return errors.New("disclosure categories are invalid")
	}
	copyCategories := cloneDisclosureCategories(categories)
	slices.SortFunc(copyCategories, compareDisclosureCategories)
	if !reflect.DeepEqual(copyCategories, categories) {
		return errors.New("disclosure categories are not sorted")
	}
	for index, category := range categories {
		if !validField(category.Category) || category.Boundaries == nil || len(category.Boundaries) == 0 {
			return errors.New("disclosure category is invalid")
		}
		if index > 0 && categories[index-1].Category == category.Category {
			return errors.New("disclosure categories contain duplicates")
		}
		copyBoundaries := append([]CaseDisclosureBoundary(nil), category.Boundaries...)
		slices.SortFunc(copyBoundaries, compareDisclosureBoundaries)
		if !reflect.DeepEqual(copyBoundaries, category.Boundaries) {
			return errors.New("disclosure category boundaries are not sorted")
		}
		for boundaryIndex, boundary := range category.Boundaries {
			if !validSource(boundary.Source) {
				return errors.New("disclosure category source is invalid")
			}
			if !validAdapterSource(boundary.Adapter, boundary.Source) {
				return errors.New("disclosure category adapter is invalid")
			}
			if boundaryIndex > 0 && category.Boundaries[boundaryIndex-1] == boundary {
				return errors.New("disclosure category boundaries contain duplicates")
			}
		}
	}
	return nil
}

func disclosureQuestionResult(questionID string, coverageState evidence.State, overlaps []string) (string, evidence.State) {
	switch questionID {
	case CaseDisclosureQuestionCoverage:
		if coverageState == evidence.Observed {
			return caseDisclosureResultComplete, evidence.Observed
		}
		return caseDisclosureResultUnknown, evidence.Unknown
	case CaseDisclosureQuestionOverlap:
		if len(overlaps) > 0 {
			return caseDisclosureResultOverlap, evidence.Observed
		}
		if coverageState == evidence.Unknown {
			return caseDisclosureResultUnknown, evidence.Unknown
		}
		return caseDisclosureResultNoOverlap, evidence.Observed
	default:
		return caseDisclosureResultUnknown, evidence.Unknown
	}
}

func disclosureQuestionReason(answer CaseDisclosureQuestionAnswer) string {
	result, _ := disclosureQuestionResult(answer.QuestionID, answer.CoverageState, answer.OverlappingCategories)
	switch answer.QuestionID {
	case CaseDisclosureQuestionCoverage:
		if result == caseDisclosureResultComplete {
			return "every retained trace declared complete coverage"
		}
		return "partial trace coverage prevents a complete-coverage conclusion"
	case CaseDisclosureQuestionOverlap:
		switch result {
		case caseDisclosureResultOverlap:
			return "one or more reviewed categories appeared across multiple source or adapter boundaries"
		case caseDisclosureResultUnknown:
			return "no cross-boundary overlap was observed, but partial coverage prevents a no-overlap conclusion"
		default:
			return "no reviewed category appeared across multiple source or adapter boundaries"
		}
	default:
		return ""
	}
}

func caseDisclosureQuestion(questionID string) (CaseDisclosureQuestion, bool) {
	for _, question := range CaseDisclosureQuestions() {
		if question.ID == questionID {
			return question, true
		}
	}
	return CaseDisclosureQuestion{}, false
}

func compareDisclosureCategories(left, right CaseDisclosureCategorySummary) int {
	if left.Category < right.Category {
		return -1
	}
	if left.Category > right.Category {
		return 1
	}
	return 0
}

func compareDisclosureBoundaries(left, right CaseDisclosureBoundary) int {
	if left.Source != right.Source {
		if left.Source < right.Source {
			return -1
		}
		return 1
	}
	if left.Adapter < right.Adapter {
		return -1
	}
	if left.Adapter > right.Adapter {
		return 1
	}
	return 0
}
