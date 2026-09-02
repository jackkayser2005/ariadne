package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	archiveQuestionRoundSchemaVersion   = 1
	archiveQuestionReceiptSchemaVersion = 1
)

// ArchiveQuestionRound is a portable, raw-value-free answer round for one
// verified trace archive. It retains the archive identity and fixed answers,
// not the source archive or any input paths.
type ArchiveQuestionRound struct {
	SchemaVersion int                    `json:"schema_version"`
	OrderBasis    string                 `json:"order_basis"`
	ArchiveSHA256 string                 `json:"archive_sha256"`
	Entries       int                    `json:"entries"`
	Complete      int                    `json:"complete"`
	Partial       int                    `json:"partial"`
	Sources       []ArchiveSourceSummary `json:"sources"`
	Answers       []ArchiveAnswer        `json:"answers"`
}

// ArchiveQuestionRoundVerificationSummary identifies a valid saved answer
// round without reopening its source archive.
type ArchiveQuestionRoundVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	ArchiveSHA256 string `json:"archive_sha256"`
	Questions     int    `json:"questions"`
	RoundSHA256   string `json:"round_sha256"`
}

// ArchiveQuestionReceipt is one selected fixed answer bound to its archive
// and saved question-round identities.
type ArchiveQuestionReceipt struct {
	SchemaVersion int                    `json:"schema_version"`
	QuestionID    string                 `json:"question_id"`
	Question      string                 `json:"question"`
	Result        string                 `json:"result"`
	EvidenceState evidence.State         `json:"evidence_state"`
	Reason        string                 `json:"reason,omitempty"`
	ArchiveSHA256 string                 `json:"archive_sha256"`
	RoundSHA256   string                 `json:"round_sha256"`
	Entries       int                    `json:"entries"`
	Compared      int                    `json:"compared"`
	Changed       int                    `json:"changed"`
	Same          int                    `json:"same"`
	Unknown       int                    `json:"unknown"`
	Sources       []ArchiveSourceSummary `json:"sources"`
}

// ArchiveQuestionReceiptVerificationSummary identifies a valid saved answer
// receipt without reopening its source archive or question round.
type ArchiveQuestionReceiptVerificationSummary struct {
	SchemaVersion int            `json:"schema_version"`
	QuestionID    string         `json:"question_id"`
	Question      string         `json:"question"`
	Result        string         `json:"result"`
	EvidenceState evidence.State `json:"evidence_state"`
	ArchiveSHA256 string         `json:"archive_sha256"`
	RoundSHA256   string         `json:"round_sha256"`
	ReceiptSHA256 string         `json:"receipt_sha256"`
}

// SaveArchiveQuestionRound verifies an archive, asks every fixed question,
// and writes a portable answer round without overwriting an existing path.
func SaveArchiveQuestionRound(archivePath, roundPath string) (ArchiveQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ArchiveQuestionRoundVerificationSummary{}, errors.New("trace archive question round path is required")
	}
	archive, archiveSummary, err := ReadArchive(archivePath)
	if err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, err
	}
	round, err := AnswerArchiveQuestionRound(archive, archiveSummary)
	if err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ArchiveQuestionRoundSHA256(round)
	if err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, errors.New("trace archive question round encoding failed")
	}
	if err := writeArchiveExclusive(roundPath, append(data, '\n')); err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, fmt.Errorf("trace archive question round: %w", err)
	}
	return archiveQuestionRoundSummary(round, roundSHA256), nil
}

// AnswerArchiveQuestionRound answers every fixed question from one already
// verified archive and returns the portable round in memory.
func AnswerArchiveQuestionRound(archive Archive, summary ArchiveVerificationSummary) (ArchiveQuestionRound, error) {
	expectedSummary, err := archiveSummary(archive)
	if err != nil {
		return ArchiveQuestionRound{}, err
	}
	if expectedSummary.SchemaVersion != summary.SchemaVersion || expectedSummary.OrderBasis != summary.OrderBasis || expectedSummary.ArchiveSHA256 != summary.ArchiveSHA256 || expectedSummary.Entries != summary.Entries || expectedSummary.Complete != summary.Complete || expectedSummary.Partial != summary.Partial || !slices.Equal(expectedSummary.Sources, summary.Sources) {
		return ArchiveQuestionRound{}, errors.New("trace archive question round archive identity does not match summary")
	}
	return newArchiveQuestionRound(archive, summary)
}

// ReadArchiveQuestionRound verifies and reads a saved answer round.
func ReadArchiveQuestionRound(roundPath string) (ArchiveQuestionRound, ArchiveQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return ArchiveQuestionRound{}, ArchiveQuestionRoundVerificationSummary{}, errors.New("trace archive question round path is required")
	}
	data, err := readArchive(roundPath)
	if err != nil {
		return ArchiveQuestionRound{}, ArchiveQuestionRoundVerificationSummary{}, fmt.Errorf("trace archive question round: %w", err)
	}
	round, err := DecodeArchiveQuestionRound(data)
	if err != nil {
		return ArchiveQuestionRound{}, ArchiveQuestionRoundVerificationSummary{}, err
	}
	roundSHA256, err := ArchiveQuestionRoundSHA256(round)
	if err != nil {
		return ArchiveQuestionRound{}, ArchiveQuestionRoundVerificationSummary{}, err
	}
	return round, archiveQuestionRoundSummary(round, roundSHA256), nil
}

// VerifyArchiveQuestionRound verifies a saved answer round without reopening
// the source trace archive.
func VerifyArchiveQuestionRound(roundPath string) (ArchiveQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadArchiveQuestionRound(roundPath)
	return summary, err
}

// DecodeArchiveQuestionRound verifies one bounded answer-round document.
func DecodeArchiveQuestionRound(data []byte) (ArchiveQuestionRound, error) {
	if len(data) == 0 {
		return ArchiveQuestionRound{}, errors.New("trace archive question round is empty")
	}
	if len(data) > maxArchiveBytes {
		return ArchiveQuestionRound{}, errors.New("trace archive question round exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionRound{}, errors.New("trace archive question round has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round ArchiveQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return ArchiveQuestionRound{}, errors.New("trace archive question round has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionRound{}, errors.New("trace archive question round has trailing data")
	}
	if err := validateArchiveQuestionRound(round); err != nil {
		return ArchiveQuestionRound{}, err
	}
	return round, nil
}

// ArchiveQuestionRoundSHA256 returns the canonical identity of a valid round.
func ArchiveQuestionRoundSHA256(round ArchiveQuestionRound) (string, error) {
	if err := validateArchiveQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("trace archive question round encoding failed")
	}
	return sha256Hex(data), nil
}

// AskArchiveQuestionRound returns one fixed answer from a saved round.
func AskArchiveQuestionRound(roundPath, questionID string) (ArchiveAnswer, error) {
	round, _, err := ReadArchiveQuestionRound(roundPath)
	if err != nil {
		return ArchiveAnswer{}, err
	}
	index, ok := archiveQuestionIndex(questionID)
	if !ok {
		return ArchiveAnswer{}, errors.New("trace archive question ID is invalid")
	}
	return round.Answers[index], nil
}

// SaveArchiveQuestionReceipt selects one fixed answer from a saved round and
// writes a portable receipt without overwriting an existing path.
func SaveArchiveQuestionReceipt(roundPath, questionID, receiptPath string) (ArchiveQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return ArchiveQuestionReceiptVerificationSummary{}, errors.New("trace archive question receipt path is required")
	}
	round, roundSummary, err := ReadArchiveQuestionRound(roundPath)
	if err != nil {
		return ArchiveQuestionReceiptVerificationSummary{}, err
	}
	answer, err := answerFromRound(round, questionID)
	if err != nil {
		return ArchiveQuestionReceiptVerificationSummary{}, err
	}
	receipt := archiveQuestionReceiptFromAnswer(answer, roundSummary.RoundSHA256)
	receiptSHA256, err := ArchiveQuestionReceiptSHA256(receipt)
	if err != nil {
		return ArchiveQuestionReceiptVerificationSummary{}, err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return ArchiveQuestionReceiptVerificationSummary{}, errors.New("trace archive question receipt encoding failed")
	}
	if err := writeArchiveExclusive(receiptPath, append(data, '\n')); err != nil {
		return ArchiveQuestionReceiptVerificationSummary{}, fmt.Errorf("trace archive question receipt: %w", err)
	}
	return archiveQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// ReadArchiveQuestionReceipt verifies and reads a saved answer receipt.
func ReadArchiveQuestionReceipt(receiptPath string) (ArchiveQuestionReceipt, ArchiveQuestionReceiptVerificationSummary, error) {
	if strings.TrimSpace(receiptPath) == "" {
		return ArchiveQuestionReceipt{}, ArchiveQuestionReceiptVerificationSummary{}, errors.New("trace archive question receipt path is required")
	}
	data, err := readArchive(receiptPath)
	if err != nil {
		return ArchiveQuestionReceipt{}, ArchiveQuestionReceiptVerificationSummary{}, fmt.Errorf("trace archive question receipt: %w", err)
	}
	receipt, err := DecodeArchiveQuestionReceipt(data)
	if err != nil {
		return ArchiveQuestionReceipt{}, ArchiveQuestionReceiptVerificationSummary{}, err
	}
	receiptSHA256, err := ArchiveQuestionReceiptSHA256(receipt)
	if err != nil {
		return ArchiveQuestionReceipt{}, ArchiveQuestionReceiptVerificationSummary{}, err
	}
	return receipt, archiveQuestionReceiptSummary(receipt, receiptSHA256), nil
}

// VerifyArchiveQuestionReceipt verifies a saved answer receipt without
// reopening its source archive or question round.
func VerifyArchiveQuestionReceipt(receiptPath string) (ArchiveQuestionReceiptVerificationSummary, error) {
	_, summary, err := ReadArchiveQuestionReceipt(receiptPath)
	return summary, err
}

// DecodeArchiveQuestionReceipt verifies one bounded answer-receipt document.
func DecodeArchiveQuestionReceipt(data []byte) (ArchiveQuestionReceipt, error) {
	if len(data) == 0 {
		return ArchiveQuestionReceipt{}, errors.New("trace archive question receipt is empty")
	}
	if len(data) > maxArchiveBytes {
		return ArchiveQuestionReceipt{}, errors.New("trace archive question receipt exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionReceipt{}, errors.New("trace archive question receipt has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt ArchiveQuestionReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return ArchiveQuestionReceipt{}, errors.New("trace archive question receipt has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionReceipt{}, errors.New("trace archive question receipt has trailing data")
	}
	if err := validateArchiveQuestionReceipt(receipt); err != nil {
		return ArchiveQuestionReceipt{}, err
	}
	return receipt, nil
}

// ArchiveQuestionReceiptSHA256 returns the canonical identity of a valid
// answer receipt.
func ArchiveQuestionReceiptSHA256(receipt ArchiveQuestionReceipt) (string, error) {
	if err := validateArchiveQuestionReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("trace archive question receipt encoding failed")
	}
	return sha256Hex(data), nil
}

// AskArchiveQuestionReceipt returns a selected receipt derived from a saved
// round without reopening the source archive.
func AskArchiveQuestionReceipt(roundPath, questionID string) (ArchiveQuestionReceipt, error) {
	round, summary, err := ReadArchiveQuestionRound(roundPath)
	if err != nil {
		return ArchiveQuestionReceipt{}, err
	}
	answer, err := answerFromRound(round, questionID)
	if err != nil {
		return ArchiveQuestionReceipt{}, err
	}
	return archiveQuestionReceiptFromAnswer(answer, summary.RoundSHA256), nil
}

func newArchiveQuestionRound(archive Archive, summary ArchiveVerificationSummary) (ArchiveQuestionRound, error) {
	answers := make([]ArchiveAnswer, 0, len(ArchiveQuestions()))
	for _, question := range ArchiveQuestions() {
		answer, err := AnswerArchive(archive, summary.ArchiveSHA256, question.ID)
		if err != nil {
			return ArchiveQuestionRound{}, err
		}
		answers = append(answers, answer)
	}
	return ArchiveQuestionRound{
		SchemaVersion: archiveQuestionRoundSchemaVersion,
		OrderBasis:    summary.OrderBasis,
		ArchiveSHA256: summary.ArchiveSHA256,
		Entries:       summary.Entries,
		Complete:      summary.Complete,
		Partial:       summary.Partial,
		Sources:       append([]ArchiveSourceSummary(nil), summary.Sources...),
		Answers:       answers,
	}, nil
}

func archiveQuestionRoundSummary(round ArchiveQuestionRound, roundSHA256 string) ArchiveQuestionRoundVerificationSummary {
	return ArchiveQuestionRoundVerificationSummary{
		SchemaVersion: round.SchemaVersion,
		ArchiveSHA256: round.ArchiveSHA256,
		Questions:     len(round.Answers),
		RoundSHA256:   roundSHA256,
	}
}

func answerFromRound(round ArchiveQuestionRound, questionID string) (ArchiveAnswer, error) {
	index, ok := archiveQuestionIndex(questionID)
	if !ok {
		return ArchiveAnswer{}, errors.New("trace archive question ID is invalid")
	}
	return round.Answers[index], nil
}

func archiveQuestionReceiptFromAnswer(answer ArchiveAnswer, roundSHA256 string) ArchiveQuestionReceipt {
	return ArchiveQuestionReceipt{
		SchemaVersion: archiveQuestionReceiptSchemaVersion,
		QuestionID:    answer.QuestionID,
		Question:      answer.Question,
		Result:        answer.Result,
		EvidenceState: answer.EvidenceState,
		Reason:        answer.Reason,
		ArchiveSHA256: answer.ArchiveSHA256,
		RoundSHA256:   roundSHA256,
		Entries:       answer.Entries,
		Compared:      answer.Compared,
		Changed:       answer.Changed,
		Same:          answer.Same,
		Unknown:       answer.Unknown,
		Sources:       append([]ArchiveSourceSummary(nil), answer.Sources...),
	}
}

func archiveQuestionReceiptSummary(receipt ArchiveQuestionReceipt, receiptSHA256 string) ArchiveQuestionReceiptVerificationSummary {
	return ArchiveQuestionReceiptVerificationSummary{
		SchemaVersion: receipt.SchemaVersion,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		ArchiveSHA256: receipt.ArchiveSHA256,
		RoundSHA256:   receipt.RoundSHA256,
		ReceiptSHA256: receiptSHA256,
	}
}

func archiveQuestionIndex(questionID string) (int, bool) {
	for index, question := range ArchiveQuestions() {
		if question.ID == questionID {
			return index, true
		}
	}
	return 0, false
}

func validateArchiveQuestionRound(round ArchiveQuestionRound) error {
	if round.SchemaVersion != archiveQuestionRoundSchemaVersion {
		return errors.New("trace archive question round has unsupported schema_version")
	}
	if round.OrderBasis != "caller" {
		return errors.New("trace archive question round order_basis is invalid")
	}
	if !ValidSHA256(round.ArchiveSHA256) {
		return errors.New("trace archive question round archive_sha256 is invalid")
	}
	if round.Entries <= 0 || round.Complete < 0 || round.Partial < 0 || round.Complete+round.Partial != round.Entries {
		return errors.New("trace archive question round counts are invalid")
	}
	if err := validateArchiveSources(round.Sources, round.Entries); err != nil {
		return fmt.Errorf("trace archive question round sources: %w", err)
	}
	questions := ArchiveQuestions()
	if len(round.Answers) != len(questions) {
		return errors.New("trace archive question round answer count does not match catalog")
	}
	for index, answer := range round.Answers {
		if err := validateArchiveAnswer(answer, questions[index], round.ArchiveSHA256, round.Entries, round.Sources); err != nil {
			return fmt.Errorf("trace archive question round answer %d: %w", index+1, err)
		}
	}
	return nil
}

func validateArchiveQuestionReceipt(receipt ArchiveQuestionReceipt) error {
	if receipt.SchemaVersion != archiveQuestionReceiptSchemaVersion {
		return errors.New("trace archive question receipt has unsupported schema_version")
	}
	index, ok := archiveQuestionIndex(receipt.QuestionID)
	if !ok {
		return errors.New("trace archive question receipt question_id is invalid")
	}
	if err := validateArchiveAnswer(ArchiveAnswer{
		SchemaVersion: receipt.SchemaVersion,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		Reason:        receipt.Reason,
		ArchiveSHA256: receipt.ArchiveSHA256,
		Entries:       receipt.Entries,
		Compared:      receipt.Compared,
		Changed:       receipt.Changed,
		Same:          receipt.Same,
		Unknown:       receipt.Unknown,
		Sources:       receipt.Sources,
	}, ArchiveQuestions()[index], receipt.ArchiveSHA256, receipt.Entries, receipt.Sources); err != nil {
		return fmt.Errorf("trace archive question receipt answer: %w", err)
	}
	if !ValidSHA256(receipt.RoundSHA256) {
		return errors.New("trace archive question receipt round_sha256 is invalid")
	}
	return nil
}

func validateArchiveAnswer(answer ArchiveAnswer, question ArchiveQuestion, archiveSHA256 string, entries int, sources []ArchiveSourceSummary) error {
	if answer.SchemaVersion != archiveAnswerSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text {
		return errors.New("question identity is invalid")
	}
	if !ValidSHA256(answer.ArchiveSHA256) || answer.ArchiveSHA256 != archiveSHA256 {
		return errors.New("archive identity is invalid")
	}
	if answer.Entries != entries || answer.Compared < 0 || answer.Changed < 0 || answer.Same < 0 || answer.Unknown < 0 {
		return errors.New("answer counts are invalid")
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("evidence_state is invalid")
	}
	if !validArchiveAnswerResult(question.ID, answer.Result) {
		return errors.New("answer result is invalid")
	}
	if err := validateArchiveSources(answer.Sources, entries); err != nil {
		return fmt.Errorf("answer sources: %w", err)
	}
	if !slices.Equal(answer.Sources, sources) {
		return errors.New("answer sources do not match round sources")
	}
	return nil
}

func validArchiveAnswerResult(questionID, result string) bool {
	switch questionID {
	case ArchiveQuestionCoverage:
		return result == archiveResultComplete || result == archiveResultUnknown
	case ArchiveQuestionChange:
		return result == archiveResultChanged || result == archiveResultSame || result == archiveResultMixed || result == archiveResultUnknown
	case ArchiveQuestionSources:
		return result == archiveResultAvailable
	default:
		return false
	}
}

func validateArchiveSources(sources []ArchiveSourceSummary, entries int) error {
	if len(sources) == 0 {
		return errors.New("source summaries are empty")
	}
	copySources := append([]ArchiveSourceSummary(nil), sources...)
	slices.SortFunc(copySources, func(left, right ArchiveSourceSummary) int {
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
	})
	if !slices.Equal(copySources, sources) {
		return errors.New("source summaries are not sorted")
	}
	total := 0
	for index, source := range sources {
		if strings.TrimSpace(source.Source) == "" || strings.TrimSpace(source.Adapter) == "" || source.Entries <= 0 {
			return errors.New("source summary is invalid")
		}
		if index > 0 && sources[index-1].Source == source.Source && sources[index-1].Adapter == source.Adapter {
			return errors.New("source summaries contain duplicates")
		}
		total += source.Entries
	}
	if total != entries {
		return errors.New("source summary counts do not match entries")
	}
	return nil
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
