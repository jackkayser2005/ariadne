package trace

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
	"reflect"
	"slices"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	caseSchemaVersion              = 1
	caseQuestionRoundSchemaVersion = 1
	maxCaseBytes                   = 4 << 20
	maxCaseEntries                 = 8
	maxCaseSummaryEntries          = maxArchiveEntries * maxCaseEntries
)

const (
	// CaseEntryTraceArchive identifies one standalone trace archive entry.
	CaseEntryTraceArchive = "trace-archive"
	// CaseEntryTraceReplication identifies one replicated trace ledger entry.
	CaseEntryTraceReplication = "trace-replication"
)

const (
	// CaseQuestionSources asks which reviewed source boundaries are represented.
	CaseQuestionSources = "case-sources"
	// CaseQuestionOutcomes asks which replicated outcomes are retained.
	CaseQuestionOutcomes = "case-outcomes"
	// CaseQuestionSupport asks which retained conclusions remain unsupported.
	CaseQuestionSupport = "case-support"
)

// CaseInput identifies one verified artifact and its matching fixed question
// round. The paths are used only while creating the portable package.
type CaseInput struct {
	Kind              string
	ArtifactPath      string
	QuestionRoundPath string
}

// CasePackage is a bounded, caller-ordered collection of verified trace
// archives or replication ledgers. It embeds child artifacts and rounds so a
// later verifier never needs to reopen source-specific paths.
type CasePackage struct {
	SchemaVersion int         `json:"schema_version"`
	OrderBasis    string      `json:"order_basis"`
	Entries       []CaseEntry `json:"entries"`
}

// CaseEntry retains exactly one verified artifact and its matching question
// round. Embedded child documents remain raw-value-free by their own schema.
type CaseEntry struct {
	Position                 int                       `json:"position"`
	Kind                     string                    `json:"kind"`
	Archive                  *Archive                  `json:"archive,omitempty"`
	ArchiveQuestionRound     *ArchiveQuestionRound     `json:"archive_question_round,omitempty"`
	ReplicationLedger        *ReplicationLedger        `json:"replication_ledger,omitempty"`
	ReplicationQuestionRound *ReplicationQuestionRound `json:"replication_question_round,omitempty"`
}

// CaseSourceSummary identifies one reviewed source boundary represented in a
// case without exposing procedure paths, target identifiers, or values.
type CaseSourceSummary struct {
	Source  string `json:"source"`
	Adapter string `json:"adapter"`
	Entries int    `json:"entries"`
}

// CaseOutcomeSummary identifies one retained replicated result without
// exposing the embedded pair contents.
type CaseOutcomeSummary struct {
	Position      int               `json:"position"`
	Outcome       ReplicatedOutcome `json:"outcome"`
	EvidenceState evidence.State    `json:"evidence_state"`
	Pairs         int               `json:"pairs"`
	UnknownPairs  int               `json:"unknown_pairs"`
}

// CaseEntryVerificationSummary identifies one verified child artifact.
type CaseEntryVerificationSummary struct {
	Position            int                 `json:"position"`
	Kind                string              `json:"kind"`
	ArtifactSHA256      string              `json:"artifact_sha256"`
	QuestionRoundSHA256 string              `json:"question_round_sha256"`
	EvidenceState       evidence.State      `json:"evidence_state"`
	Complete            int                 `json:"complete,omitempty"`
	Partial             int                 `json:"partial,omitempty"`
	Pairs               int                 `json:"pairs,omitempty"`
	UnknownPairs        int                 `json:"unknown_pairs,omitempty"`
	Outcome             ReplicatedOutcome   `json:"outcome,omitempty"`
	Sources             []CaseSourceSummary `json:"sources"`
}

// CaseVerificationSummary identifies a verified case and its safe child
// summaries. Unknown child evidence is counted but does not invalidate the
// package itself.
type CaseVerificationSummary struct {
	SchemaVersion  int                            `json:"schema_version"`
	OrderBasis     string                         `json:"order_basis"`
	Entries        int                            `json:"entries"`
	Archives       int                            `json:"archives"`
	Replications   int                            `json:"replications"`
	UnknownEntries int                            `json:"unknown_entries"`
	Sources        []CaseSourceSummary            `json:"sources"`
	Outcomes       []CaseOutcomeSummary           `json:"outcomes"`
	EntrySummaries []CaseEntryVerificationSummary `json:"entry_summaries"`
	CaseSHA256     string                         `json:"case_sha256"`
}

// CaseQuestion is one fixed, bounded question available for a case.
type CaseQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// CaseQuestions returns the stable case question catalog.
func CaseQuestions() []CaseQuestion {
	return []CaseQuestion{
		{ID: CaseQuestionSources, Text: "Which reviewed source boundaries are represented?"},
		{ID: CaseQuestionOutcomes, Text: "Which replicated outcomes are retained?"},
		{ID: CaseQuestionSupport, Text: "Which retained conclusions remain unknown or incompletely supported?"},
	}
}

// CaseAnswer is a bounded answer tied to one verified case identity. Result
// describes the question outcome; EvidenceState remains separate.
type CaseAnswer struct {
	SchemaVersion  int                  `json:"schema_version"`
	QuestionID     string               `json:"question_id"`
	Question       string               `json:"question"`
	Result         string               `json:"result"`
	EvidenceState  evidence.State       `json:"evidence_state"`
	Reason         string               `json:"reason,omitempty"`
	CaseSHA256     string               `json:"case_sha256"`
	Entries        int                  `json:"entries"`
	Archives       int                  `json:"archives"`
	Replications   int                  `json:"replications"`
	UnknownEntries int                  `json:"unknown_entries"`
	Sources        []CaseSourceSummary  `json:"sources"`
	Outcomes       []CaseOutcomeSummary `json:"outcomes"`
}

// CaseQuestionRound is a durable answer set bound to one verified case.
type CaseQuestionRound struct {
	SchemaVersion int          `json:"schema_version"`
	OrderBasis    string       `json:"order_basis"`
	CaseSHA256    string       `json:"case_sha256"`
	Entries       int          `json:"entries"`
	Answers       []CaseAnswer `json:"answers"`
}

// CaseQuestionRoundVerificationSummary identifies one valid saved case round.
type CaseQuestionRoundVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	CaseSHA256    string `json:"case_sha256"`
	Questions     int    `json:"questions"`
	RoundSHA256   string `json:"round_sha256"`
}

// SaveCase verifies and embeds each supplied artifact and matching question
// round without overwriting an existing output path.
func SaveCase(inputs []CaseInput, outputPath string) (CaseVerificationSummary, error) {
	if len(inputs) == 0 || len(inputs) > maxCaseEntries {
		return CaseVerificationSummary{}, errors.New("trace case entry count is invalid")
	}
	if strings.TrimSpace(outputPath) == "" {
		return CaseVerificationSummary{}, errors.New("trace case output path is required")
	}
	casePackage := CasePackage{
		SchemaVersion: caseSchemaVersion,
		OrderBasis:    "caller",
		Entries:       make([]CaseEntry, 0, len(inputs)),
	}
	for index, input := range inputs {
		entry, err := readCaseEntry(input)
		if err != nil {
			return CaseVerificationSummary{}, fmt.Errorf("trace case entry %d verification failed", index+1)
		}
		entry.Position = index + 1
		casePackage.Entries = append(casePackage.Entries, entry)
	}
	if err := validateCase(casePackage); err != nil {
		return CaseVerificationSummary{}, err
	}
	data, err := marshalCase(casePackage)
	if err != nil {
		return CaseVerificationSummary{}, err
	}
	if err := writeCaseExclusive(outputPath, append(data, '\n')); err != nil {
		return CaseVerificationSummary{}, fmt.Errorf("trace case: %w", err)
	}
	return caseSummary(casePackage)
}

// ReadCase verifies and reads one portable case package.
func ReadCase(path string) (CasePackage, CaseVerificationSummary, error) {
	data, err := readCase(path)
	if err != nil {
		return CasePackage{}, CaseVerificationSummary{}, fmt.Errorf("trace case: %w", err)
	}
	casePackage, err := DecodeCase(data)
	if err != nil {
		return CasePackage{}, CaseVerificationSummary{}, err
	}
	summary, err := caseSummary(casePackage)
	if err != nil {
		return CasePackage{}, CaseVerificationSummary{}, err
	}
	return casePackage, summary, nil
}

// VerifyCase verifies one saved case package and returns only its safe summary.
func VerifyCase(path string) (CaseVerificationSummary, error) {
	_, summary, err := ReadCase(path)
	return summary, err
}

// DecodeCase verifies one bounded case package document.
func DecodeCase(data []byte) (CasePackage, error) {
	if len(data) == 0 {
		return CasePackage{}, errors.New("trace case is empty")
	}
	if len(data) > maxCaseBytes {
		return CasePackage{}, errors.New("trace case exceeds 4194304-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return CasePackage{}, errors.New("trace case has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var casePackage CasePackage
	if err := decoder.Decode(&casePackage); err != nil {
		return CasePackage{}, errors.New("trace case has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CasePackage{}, errors.New("trace case has trailing data")
	}
	if err := validateCase(casePackage); err != nil {
		return CasePackage{}, err
	}
	return casePackage, nil
}

// CaseSHA256 returns the canonical identity of a valid case package.
func CaseSHA256(casePackage CasePackage) (string, error) {
	data, err := marshalCase(casePackage)
	if err != nil {
		return "", err
	}
	return sha256HexCase(data), nil
}

// AnswerCase answers one fixed question against an already verified case and
// summary.
func AnswerCase(casePackage CasePackage, summary CaseVerificationSummary, questionID string) (CaseAnswer, error) {
	question, ok := caseQuestion(questionID)
	if !ok {
		return CaseAnswer{}, errors.New("trace case question ID is invalid")
	}
	expectedSummary, err := caseSummary(casePackage)
	if err != nil {
		return CaseAnswer{}, err
	}
	if !reflect.DeepEqual(expectedSummary, summary) {
		return CaseAnswer{}, errors.New("trace case question case identity does not match summary")
	}
	return answerCaseFromSummary(summary, question), nil
}

// AnswerAllCaseQuestions answers the complete fixed catalog.
func AnswerAllCaseQuestions(casePackage CasePackage, summary CaseVerificationSummary) ([]CaseAnswer, error) {
	expectedSummary, err := caseSummary(casePackage)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(expectedSummary, summary) {
		return nil, errors.New("trace case question case identity does not match summary")
	}
	return answerAllCaseQuestionsFromSummary(summary), nil
}

// AskCaseQuestion answers one fixed question after verifying a case.
func AskCaseQuestion(path, questionID string) (CaseAnswer, error) {
	casePackage, summary, err := ReadCase(path)
	if err != nil {
		return CaseAnswer{}, err
	}
	return AnswerCase(casePackage, summary, questionID)
}

// AskAllCaseQuestions answers all fixed questions after verifying a case.
func AskAllCaseQuestions(path string) ([]CaseAnswer, error) {
	casePackage, summary, err := ReadCase(path)
	if err != nil {
		return nil, err
	}
	return AnswerAllCaseQuestions(casePackage, summary)
}

// SaveCaseQuestionRound verifies a case, answers every fixed question, and
// writes a portable question round without overwriting an existing path.
func SaveCaseQuestionRound(casePath, roundPath string) (CaseQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(roundPath) == "" {
		return CaseQuestionRoundVerificationSummary{}, errors.New("trace case question round path is required")
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		return CaseQuestionRoundVerificationSummary{}, err
	}
	answers, err := AnswerAllCaseQuestions(casePackage, summary)
	if err != nil {
		return CaseQuestionRoundVerificationSummary{}, err
	}
	round := CaseQuestionRound{
		SchemaVersion: caseQuestionRoundSchemaVersion,
		OrderBasis:    summary.OrderBasis,
		CaseSHA256:    summary.CaseSHA256,
		Entries:       summary.Entries,
		Answers:       answers,
	}
	roundSHA256, err := CaseQuestionRoundSHA256(round)
	if err != nil {
		return CaseQuestionRoundVerificationSummary{}, err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return CaseQuestionRoundVerificationSummary{}, errors.New("trace case question round encoding failed")
	}
	if err := writeCaseExclusive(roundPath, append(data, '\n')); err != nil {
		return CaseQuestionRoundVerificationSummary{}, fmt.Errorf("trace case question round: %w", err)
	}
	return caseQuestionRoundSummary(round, roundSHA256), nil
}

// ReadCaseQuestionRound verifies and reads a saved case question round.
func ReadCaseQuestionRound(path string) (CaseQuestionRound, CaseQuestionRoundVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return CaseQuestionRound{}, CaseQuestionRoundVerificationSummary{}, errors.New("trace case question round path is required")
	}
	data, err := readCase(path)
	if err != nil {
		return CaseQuestionRound{}, CaseQuestionRoundVerificationSummary{}, fmt.Errorf("trace case question round: %w", err)
	}
	round, err := DecodeCaseQuestionRound(data)
	if err != nil {
		return CaseQuestionRound{}, CaseQuestionRoundVerificationSummary{}, err
	}
	digest, err := CaseQuestionRoundSHA256(round)
	if err != nil {
		return CaseQuestionRound{}, CaseQuestionRoundVerificationSummary{}, err
	}
	return round, caseQuestionRoundSummary(round, digest), nil
}

// VerifyCaseQuestionRound verifies a saved round without reopening its case.
func VerifyCaseQuestionRound(path string) (CaseQuestionRoundVerificationSummary, error) {
	_, summary, err := ReadCaseQuestionRound(path)
	return summary, err
}

// DecodeCaseQuestionRound verifies one bounded case question-round document.
func DecodeCaseQuestionRound(data []byte) (CaseQuestionRound, error) {
	if len(data) == 0 {
		return CaseQuestionRound{}, errors.New("trace case question round is empty")
	}
	if len(data) > maxCaseBytes {
		return CaseQuestionRound{}, errors.New("trace case question round exceeds 4194304-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return CaseQuestionRound{}, errors.New("trace case question round has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var round CaseQuestionRound
	if err := decoder.Decode(&round); err != nil {
		return CaseQuestionRound{}, errors.New("trace case question round has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseQuestionRound{}, errors.New("trace case question round has trailing data")
	}
	if err := validateCaseQuestionRound(round); err != nil {
		return CaseQuestionRound{}, err
	}
	return round, nil
}

// CaseQuestionRoundSHA256 returns the canonical identity of a valid case
// question round.
func CaseQuestionRoundSHA256(round CaseQuestionRound) (string, error) {
	if err := validateCaseQuestionRound(round); err != nil {
		return "", err
	}
	data, err := json.Marshal(round)
	if err != nil {
		return "", errors.New("trace case question round encoding failed")
	}
	return sha256HexCase(data), nil
}

func readCaseEntry(input CaseInput) (CaseEntry, error) {
	if strings.TrimSpace(input.ArtifactPath) == "" || strings.TrimSpace(input.QuestionRoundPath) == "" {
		return CaseEntry{}, errors.New("trace case entry paths are required")
	}
	switch input.Kind {
	case CaseEntryTraceArchive:
		archive, archiveSummary, err := ReadArchive(input.ArtifactPath)
		if err != nil {
			return CaseEntry{}, err
		}
		round, roundSummary, err := ReadArchiveQuestionRound(input.QuestionRoundPath)
		if err != nil {
			return CaseEntry{}, err
		}
		if err := validateCaseArchiveRound(archive, archiveSummary, round, roundSummary); err != nil {
			return CaseEntry{}, err
		}
		return CaseEntry{Kind: input.Kind, Archive: &archive, ArchiveQuestionRound: &round}, nil
	case CaseEntryTraceReplication:
		ledger, ledgerSummary, err := ReadReplicationLedger(input.ArtifactPath)
		if err != nil {
			return CaseEntry{}, err
		}
		round, roundSummary, err := ReadReplicationQuestionRound(input.QuestionRoundPath)
		if err != nil {
			return CaseEntry{}, err
		}
		if err := validateCaseReplicationRound(ledger, ledgerSummary, round, roundSummary); err != nil {
			return CaseEntry{}, err
		}
		return CaseEntry{Kind: input.Kind, ReplicationLedger: &ledger, ReplicationQuestionRound: &round}, nil
	default:
		return CaseEntry{}, errors.New("trace case entry kind is invalid")
	}
}

func validateCase(casePackage CasePackage) error {
	if casePackage.SchemaVersion != caseSchemaVersion {
		return errors.New("trace case has unsupported schema_version")
	}
	if casePackage.OrderBasis != "caller" {
		return errors.New("trace case order_basis is invalid")
	}
	if len(casePackage.Entries) == 0 || len(casePackage.Entries) > maxCaseEntries {
		return errors.New("trace case entries are invalid")
	}
	seen := make(map[string]struct{}, len(casePackage.Entries))
	for index, entry := range casePackage.Entries {
		if entry.Position != index+1 {
			return errors.New("trace case entry positions are invalid")
		}
		artifactSHA256, err := validateCaseEntry(entry)
		if err != nil {
			return fmt.Errorf("trace case entry %d: %w", index+1, err)
		}
		if _, ok := seen[artifactSHA256]; ok {
			return errors.New("trace case entries must have distinct artifact identities")
		}
		seen[artifactSHA256] = struct{}{}
	}
	return nil
}

func validateCaseEntry(entry CaseEntry) (string, error) {
	variants := 0
	if entry.Archive != nil || entry.ArchiveQuestionRound != nil {
		variants++
	}
	if entry.ReplicationLedger != nil || entry.ReplicationQuestionRound != nil {
		variants++
	}
	if variants != 1 {
		return "", errors.New("entry must contain exactly one artifact kind")
	}
	switch entry.Kind {
	case CaseEntryTraceArchive:
		if entry.Archive == nil || entry.ArchiveQuestionRound == nil || entry.ReplicationLedger != nil || entry.ReplicationQuestionRound != nil {
			return "", errors.New("archive entry fields are invalid")
		}
		summary, err := archiveSummary(*entry.Archive)
		if err != nil {
			return "", err
		}
		roundSummary, err := archiveQuestionRoundSummaryChecked(*entry.ArchiveQuestionRound)
		if err != nil {
			return "", err
		}
		if err := validateCaseArchiveRound(*entry.Archive, summary, *entry.ArchiveQuestionRound, roundSummary); err != nil {
			return "", err
		}
		return summary.ArchiveSHA256, nil
	case CaseEntryTraceReplication:
		if entry.ReplicationLedger == nil || entry.ReplicationQuestionRound == nil || entry.Archive != nil || entry.ArchiveQuestionRound != nil {
			return "", errors.New("replication entry fields are invalid")
		}
		summary, err := replicationLedgerSummary(*entry.ReplicationLedger)
		if err != nil {
			return "", err
		}
		roundSummary, err := replicationQuestionRoundSummaryChecked(*entry.ReplicationQuestionRound)
		if err != nil {
			return "", err
		}
		if err := validateCaseReplicationRound(*entry.ReplicationLedger, summary, *entry.ReplicationQuestionRound, roundSummary); err != nil {
			return "", err
		}
		return summary.LedgerSHA256, nil
	default:
		return "", errors.New("entry kind is invalid")
	}
}

func validateCaseArchiveRound(archive Archive, summary ArchiveVerificationSummary, round ArchiveQuestionRound, roundSummary ArchiveQuestionRoundVerificationSummary) error {
	if roundSummary.ArchiveSHA256 != summary.ArchiveSHA256 || roundSummary.Questions != len(ArchiveQuestions()) {
		return errors.New("archive question round identity is invalid")
	}
	digest, err := ArchiveQuestionRoundSHA256(round)
	if err != nil || digest != roundSummary.RoundSHA256 {
		return errors.New("archive question round identity is invalid")
	}
	expected, err := newArchiveQuestionRound(archive, summary)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(round, expected) {
		return errors.New("archive question round does not match archive")
	}
	return nil
}

func validateCaseReplicationRound(ledger ReplicationLedger, summary ReplicationLedgerVerificationSummary, round ReplicationQuestionRound, roundSummary ReplicationQuestionRoundVerificationSummary) error {
	if roundSummary.LedgerSHA256 != summary.LedgerSHA256 || roundSummary.Questions != len(ReplicationQuestions()) {
		return errors.New("replication question round identity is invalid")
	}
	digest, err := ReplicationQuestionRoundSHA256(round)
	if err != nil || digest != roundSummary.RoundSHA256 {
		return errors.New("replication question round identity is invalid")
	}
	answers, err := AnswerAllReplicationQuestionsFromSummary(summary)
	if err != nil {
		return err
	}
	expected := ReplicationQuestionRound{SchemaVersion: replicationQuestionRoundSchemaVersion, LedgerSHA256: summary.LedgerSHA256, Answers: answers}
	if !reflect.DeepEqual(round, expected) {
		return errors.New("replication question round does not match ledger")
	}
	return nil
}

func archiveQuestionRoundSummaryChecked(round ArchiveQuestionRound) (ArchiveQuestionRoundVerificationSummary, error) {
	digest, err := ArchiveQuestionRoundSHA256(round)
	if err != nil {
		return ArchiveQuestionRoundVerificationSummary{}, err
	}
	return archiveQuestionRoundSummary(round, digest), nil
}

func replicationQuestionRoundSummaryChecked(round ReplicationQuestionRound) (ReplicationQuestionRoundVerificationSummary, error) {
	digest, err := ReplicationQuestionRoundSHA256(round)
	if err != nil {
		return ReplicationQuestionRoundVerificationSummary{}, err
	}
	return replicationQuestionRoundSummary(round, digest), nil
}

func caseSummary(casePackage CasePackage) (CaseVerificationSummary, error) {
	if err := validateCase(casePackage); err != nil {
		return CaseVerificationSummary{}, err
	}
	summary := CaseVerificationSummary{
		SchemaVersion:  caseSchemaVersion,
		OrderBasis:     casePackage.OrderBasis,
		Entries:        len(casePackage.Entries),
		Sources:        make([]CaseSourceSummary, 0),
		Outcomes:       make([]CaseOutcomeSummary, 0),
		EntrySummaries: make([]CaseEntryVerificationSummary, 0, len(casePackage.Entries)),
	}
	sourceCounts := make(map[string]CaseSourceSummary)
	for _, entry := range casePackage.Entries {
		entrySummary, err := caseEntrySummary(entry)
		if err != nil {
			return CaseVerificationSummary{}, err
		}
		summary.EntrySummaries = append(summary.EntrySummaries, entrySummary)
		if entrySummary.EvidenceState == evidence.Unknown {
			summary.UnknownEntries++
		}
		if entry.Kind == CaseEntryTraceArchive {
			summary.Archives++
		} else {
			summary.Replications++
			summary.Outcomes = append(summary.Outcomes, CaseOutcomeSummary{
				Position: entry.Position, Outcome: entrySummary.Outcome, EvidenceState: entrySummary.EvidenceState,
				Pairs: entrySummary.Pairs, UnknownPairs: entrySummary.UnknownPairs,
			})
		}
		for _, source := range entrySummary.Sources {
			key := source.Source + "\x00" + source.Adapter
			current := sourceCounts[key]
			current.Source = source.Source
			current.Adapter = source.Adapter
			current.Entries += source.Entries
			sourceCounts[key] = current
		}
	}
	for _, source := range sourceCounts {
		summary.Sources = append(summary.Sources, source)
	}
	slices.SortFunc(summary.Sources, compareCaseSources)
	slices.SortFunc(summary.Outcomes, func(left, right CaseOutcomeSummary) int { return left.Position - right.Position })
	data, err := marshalCase(casePackage)
	if err != nil {
		return CaseVerificationSummary{}, err
	}
	summary.CaseSHA256 = sha256HexCase(data)
	return summary, nil
}

func caseEntrySummary(entry CaseEntry) (CaseEntryVerificationSummary, error) {
	result := CaseEntryVerificationSummary{Position: entry.Position, Kind: entry.Kind, Sources: make([]CaseSourceSummary, 0)}
	switch entry.Kind {
	case CaseEntryTraceArchive:
		summary, err := archiveSummary(*entry.Archive)
		if err != nil {
			return CaseEntryVerificationSummary{}, err
		}
		roundSummary, err := archiveQuestionRoundSummaryChecked(*entry.ArchiveQuestionRound)
		if err != nil {
			return CaseEntryVerificationSummary{}, err
		}
		result.ArtifactSHA256 = summary.ArchiveSHA256
		result.QuestionRoundSHA256 = roundSummary.RoundSHA256
		result.Complete = summary.Complete
		result.Partial = summary.Partial
		result.Sources = caseSourcesFromArchive(*entry.Archive)
		result.EvidenceState = caseRoundEvidenceStateArchive(*entry.ArchiveQuestionRound)
	case CaseEntryTraceReplication:
		summary, err := replicationLedgerSummary(*entry.ReplicationLedger)
		if err != nil {
			return CaseEntryVerificationSummary{}, err
		}
		roundSummary, err := replicationQuestionRoundSummaryChecked(*entry.ReplicationQuestionRound)
		if err != nil {
			return CaseEntryVerificationSummary{}, err
		}
		result.ArtifactSHA256 = summary.LedgerSHA256
		result.QuestionRoundSHA256 = roundSummary.RoundSHA256
		result.Pairs = summary.Pairs
		result.UnknownPairs = summary.UnknownPairs
		result.Outcome = summary.Outcome
		result.EvidenceState = summary.EvidenceState
		result.Sources = caseSourcesFromLedger(*entry.ReplicationLedger)
	default:
		return CaseEntryVerificationSummary{}, errors.New("case entry kind is invalid")
	}
	return result, nil
}

func caseSourcesFromArchive(archive Archive) []CaseSourceSummary {
	sources := make([]CaseSourceSummary, 0)
	for _, source := range archiveSources(archive) {
		sources = append(sources, CaseSourceSummary{Source: source.Source, Adapter: source.Adapter, Entries: source.Entries})
	}
	return sources
}

func caseSourcesFromLedger(ledger ReplicationLedger) []CaseSourceSummary {
	counts := make(map[string]CaseSourceSummary)
	for _, pair := range ledger.Pairs {
		key := pair.Pair.Source + "\x00" + pair.Pair.Adapter
		current := counts[key]
		current.Source = pair.Pair.Source
		current.Adapter = pair.Pair.Adapter
		current.Entries++
		counts[key] = current
	}
	result := make([]CaseSourceSummary, 0, len(counts))
	for _, source := range counts {
		result = append(result, source)
	}
	slices.SortFunc(result, compareCaseSources)
	return result
}

func compareCaseSources(left, right CaseSourceSummary) int {
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

func caseRoundEvidenceStateArchive(round ArchiveQuestionRound) evidence.State {
	for _, answer := range round.Answers {
		if answer.EvidenceState != evidence.Observed {
			return evidence.Unknown
		}
	}
	return evidence.Observed
}

func answerCaseFromSummary(summary CaseVerificationSummary, question CaseQuestion) CaseAnswer {
	answer := CaseAnswer{
		SchemaVersion:  caseQuestionRoundSchemaVersion,
		QuestionID:     question.ID,
		Question:       question.Text,
		CaseSHA256:     summary.CaseSHA256,
		Entries:        summary.Entries,
		Archives:       summary.Archives,
		Replications:   summary.Replications,
		UnknownEntries: summary.UnknownEntries,
		EvidenceState:  caseEvidenceState(summary),
		Sources:        append([]CaseSourceSummary(nil), summary.Sources...),
		Outcomes:       append([]CaseOutcomeSummary(nil), summary.Outcomes...),
	}
	switch question.ID {
	case CaseQuestionSources:
		answer.Result = "available"
	case CaseQuestionOutcomes:
		if summary.Replications == 0 {
			answer.Result = "unknown"
			answer.EvidenceState = evidence.Unknown
		} else {
			answer.Result = "available"
		}
	case CaseQuestionSupport:
		if summary.UnknownEntries == 0 {
			answer.Result = "supported"
		} else {
			answer.Result = "unknown"
		}
	}
	answer.Reason = caseAnswerReason(answer)
	return answer
}

func answerAllCaseQuestionsFromSummary(summary CaseVerificationSummary) []CaseAnswer {
	answers := make([]CaseAnswer, 0, len(CaseQuestions()))
	for _, question := range CaseQuestions() {
		answers = append(answers, answerCaseFromSummary(summary, question))
	}
	return answers
}

func caseEvidenceState(summary CaseVerificationSummary) evidence.State {
	if summary.UnknownEntries > 0 {
		return evidence.Unknown
	}
	return evidence.Observed
}

func caseQuestion(id string) (CaseQuestion, bool) {
	for _, question := range CaseQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return CaseQuestion{}, false
}

func caseQuestionRoundSummary(round CaseQuestionRound, digest string) CaseQuestionRoundVerificationSummary {
	return CaseQuestionRoundVerificationSummary{SchemaVersion: round.SchemaVersion, CaseSHA256: round.CaseSHA256, Questions: len(round.Answers), RoundSHA256: digest}
}

func validateCaseQuestionRound(round CaseQuestionRound) error {
	if round.SchemaVersion != caseQuestionRoundSchemaVersion {
		return errors.New("trace case question round has unsupported schema_version")
	}
	if round.OrderBasis != "caller" {
		return errors.New("trace case question round order_basis is invalid")
	}
	if !ValidSHA256(round.CaseSHA256) {
		return errors.New("trace case question round case_sha256 is invalid")
	}
	if round.Entries <= 0 || round.Entries > maxCaseEntries || len(round.Answers) != len(CaseQuestions()) {
		return errors.New("trace case question round counts are invalid")
	}
	for index, answer := range round.Answers {
		question := CaseQuestions()[index]
		if err := validateCaseAnswer(answer, question, round.CaseSHA256, round.Entries); err != nil {
			return fmt.Errorf("trace case question round answer %d: %w", index+1, err)
		}
	}
	if err := validateCaseAnswerConsistency(round.Answers); err != nil {
		return err
	}
	return nil
}

func validateCaseAnswerConsistency(answers []CaseAnswer) error {
	base := answers[0]
	for _, answer := range answers[1:] {
		if answer.Entries != base.Entries || answer.Archives != base.Archives || answer.Replications != base.Replications || answer.UnknownEntries != base.UnknownEntries || !slices.Equal(answer.Sources, base.Sources) || !slices.Equal(answer.Outcomes, base.Outcomes) {
			return errors.New("case question round answers disagree about shared summaries")
		}
	}
	return nil
}

func validateCaseAnswer(answer CaseAnswer, question CaseQuestion, caseSHA256 string, entries int) error {
	if answer.SchemaVersion != caseQuestionRoundSchemaVersion || answer.QuestionID != question.ID || answer.Question != question.Text || answer.CaseSHA256 != caseSHA256 || answer.Entries != entries {
		return errors.New("case answer identity is invalid")
	}
	if answer.Archives < 0 || answer.Replications < 0 || answer.Archives+answer.Replications != answer.Entries || answer.UnknownEntries < 0 || answer.UnknownEntries > answer.Entries {
		return errors.New("case answer counts are invalid")
	}
	if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
		return errors.New("case answer evidence_state is invalid")
	}
	if answer.Result != "available" && answer.Result != "supported" && answer.Result != "unknown" {
		return errors.New("case answer result is invalid")
	}
	if len(answer.Sources) == 0 {
		return errors.New("case answer sources are empty")
	}
	for index, source := range answer.Sources {
		if err := validateCaseSourceSummary(source); err != nil {
			return fmt.Errorf("case answer source %d is invalid", index+1)
		}
		if index > 0 && compareCaseSources(answer.Sources[index-1], source) >= 0 {
			return errors.New("case answer sources are not in canonical order")
		}
	}
	if len(answer.Outcomes) != answer.Replications {
		return errors.New("case answer outcomes are invalid")
	}
	for index, outcome := range answer.Outcomes {
		if err := validateCaseOutcomeSummary(outcome, entries); err != nil {
			return fmt.Errorf("case answer outcome %d is invalid", index+1)
		}
		if index > 0 && answer.Outcomes[index-1].Position >= outcome.Position {
			return errors.New("case answer outcomes are not in caller order")
		}
	}
	expectedResult, expectedEvidence := caseAnswerExpectation(answer)
	if answer.Result != expectedResult {
		return errors.New("case answer result does not match metrics")
	}
	if answer.EvidenceState != expectedEvidence {
		return errors.New("case answer evidence_state does not match metrics")
	}
	if answer.Reason != caseAnswerReason(answer) {
		return errors.New("case answer reason is invalid")
	}
	return nil
}

func validateCaseSourceSummary(source CaseSourceSummary) error {
	expectedSource, ok := adapterSource(source.Adapter)
	if !ok || source.Source != expectedSource || source.Entries <= 0 || source.Entries > maxCaseSummaryEntries {
		return errors.New("source summary values are invalid")
	}
	return nil
}

func validateCaseOutcomeSummary(outcome CaseOutcomeSummary, entries int) error {
	if outcome.Position <= 0 || outcome.Position > entries || outcome.Pairs <= 0 || outcome.Pairs > maxReplicationPairs || outcome.UnknownPairs < 0 || outcome.UnknownPairs > outcome.Pairs {
		return errors.New("outcome summary counts are invalid")
	}
	if outcome.Outcome != ReplicatedChange && outcome.Outcome != NoChangeObserved && outcome.Outcome != MixedInconsistent && outcome.Outcome != ReplicationUnknown {
		return errors.New("outcome summary outcome is invalid")
	}
	if outcome.EvidenceState != evidence.Observed && outcome.EvidenceState != evidence.Unknown {
		return errors.New("outcome summary evidence_state is invalid")
	}
	return nil
}

func caseAnswerExpectation(answer CaseAnswer) (string, evidence.State) {
	evidenceState := evidence.Observed
	if answer.UnknownEntries > 0 || (answer.QuestionID == CaseQuestionOutcomes && answer.Replications == 0) {
		evidenceState = evidence.Unknown
	}
	switch answer.QuestionID {
	case CaseQuestionSources:
		return "available", evidenceState
	case CaseQuestionOutcomes:
		if answer.Replications == 0 {
			return "unknown", evidence.Unknown
		}
		return "available", evidenceState
	case CaseQuestionSupport:
		if answer.UnknownEntries == 0 {
			return "supported", evidenceState
		}
		return "unknown", evidenceState
	default:
		return "", evidence.Unknown
	}
}

func caseAnswerReason(answer CaseAnswer) string {
	switch answer.QuestionID {
	case CaseQuestionSources:
		return "verified source and adapter boundaries are represented without source paths or values"
	case CaseQuestionOutcomes:
		if answer.Replications == 0 {
			return "the case retains no replicated trace ledger"
		}
		return "replicated outcomes are retained as bounded child summaries"
	case CaseQuestionSupport:
		if answer.UnknownEntries == 0 {
			return "every retained child question round reports observed evidence support"
		}
		return "at least one retained child conclusion remains unknown or incompletely supported"
	default:
		return ""
	}
}

func marshalCase(casePackage CasePackage) ([]byte, error) {
	if err := validateCase(casePackage); err != nil {
		return nil, err
	}
	data, err := json.Marshal(casePackage)
	if err != nil {
		return nil, errors.New("trace case encoding failed")
	}
	if len(data) > maxCaseBytes {
		return nil, errors.New("trace case exceeds 4194304-byte limit")
	}
	return data, nil
}

func sha256HexCase(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func readCase(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("case path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("read case")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCaseBytes+1))
	if err != nil || len(data) > maxCaseBytes {
		return nil, errors.New("read case")
	}
	return data, nil
}

func writeCaseExclusive(path string, data []byte) error {
	if len(data) > maxCaseBytes {
		return errors.New("case output exceeds limit")
	}
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("create output directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create output")
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.New("write output")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync output")
	}
	if err := file.Close(); err != nil {
		return errors.New("close output")
	}
	remove = false
	return nil
}
