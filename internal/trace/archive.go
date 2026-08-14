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
	"slices"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	archiveSchemaVersion = 1
	maxArchiveBytes      = 1 << 20
	maxArchiveEntries    = 32
)

const (
	// ArchiveQuestionCoverage asks whether every retained trace declared complete coverage.
	ArchiveQuestionCoverage = "trace-coverage"
	// ArchiveQuestionChange asks whether safe tracking categories changed across the retained traces.
	ArchiveQuestionChange = "trace-change"
	// ArchiveQuestionSources asks which reviewed source adapters are represented.
	ArchiveQuestionSources = "trace-sources"
)

const (
	archiveResultComplete  = "complete"
	archiveResultChanged   = "changed"
	archiveResultSame      = "same"
	archiveResultMixed     = "mixed"
	archiveResultAvailable = "available"
	archiveResultUnknown   = "unknown"
)

// ArchiveInput identifies one already verified standalone trace and session.
// The caller's input order is retained as the archive order; it is not treated
// as chronology.
type ArchiveInput struct {
	TracePath   string
	SessionPath string
}

// Archive is a portable, raw-value-free sequence of standalone trace snapshots.
// It can contain reviewed traces from different source adapters.
type Archive struct {
	SchemaVersion int            `json:"schema_version"`
	OrderBasis    string         `json:"order_basis"`
	Entries       []ArchiveEntry `json:"entries"`
}

// ArchiveEntry retains one normalized trace and its provenance envelope.
type ArchiveEntry struct {
	Position int      `json:"position"`
	Session  Session  `json:"session"`
	Trace    Document `json:"trace"`
}

// ArchiveSourceSummary counts safe reviewed source identities represented in an archive.
type ArchiveSourceSummary struct {
	Source  string `json:"source"`
	Adapter string `json:"adapter"`
	Entries int    `json:"entries"`
}

// ArchiveVerificationSummary identifies a valid trace archive without exposing
// input paths or captured values.
type ArchiveVerificationSummary struct {
	SchemaVersion int                    `json:"schema_version"`
	OrderBasis    string                 `json:"order_basis"`
	Entries       int                    `json:"entries"`
	Complete      int                    `json:"complete"`
	Partial       int                    `json:"partial"`
	Sources       []ArchiveSourceSummary `json:"sources"`
	ArchiveSHA256 string                 `json:"archive_sha256"`
}

// ArchiveQuestion is one fixed, bounded question available for an archive.
type ArchiveQuestion struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// ArchiveQuestions returns the stable archive question catalog.
func ArchiveQuestions() []ArchiveQuestion {
	return []ArchiveQuestion{
		{ID: ArchiveQuestionCoverage, Text: "Did every retained trace declare complete coverage?"},
		{ID: ArchiveQuestionChange, Text: "Did the safe tracking categories change across the retained traces?"},
		{ID: ArchiveQuestionSources, Text: "Which reviewed source adapters are represented?"},
	}
}

// ArchiveAnswer is a bounded answer tied to one verified archive identity.
// Result describes the question outcome; EvidenceState remains the separate
// qualification of the support available for that outcome.
type ArchiveAnswer struct {
	SchemaVersion int                    `json:"schema_version"`
	QuestionID    string                 `json:"question_id"`
	Question      string                 `json:"question"`
	Result        string                 `json:"result"`
	EvidenceState evidence.State         `json:"evidence_state"`
	Reason        string                 `json:"reason,omitempty"`
	ArchiveSHA256 string                 `json:"archive_sha256"`
	Entries       int                    `json:"entries"`
	Compared      int                    `json:"compared"`
	Changed       int                    `json:"changed"`
	Same          int                    `json:"same"`
	Unknown       int                    `json:"unknown"`
	Sources       []ArchiveSourceSummary `json:"sources"`
}

// SaveArchive verifies and writes a portable archive without overwriting an
// existing output path.
func SaveArchive(inputs []ArchiveInput, outputPath string) (ArchiveVerificationSummary, error) {
	if len(inputs) == 0 || len(inputs) > maxArchiveEntries {
		return ArchiveVerificationSummary{}, errors.New("trace archive entry count is invalid")
	}
	if strings.TrimSpace(outputPath) == "" {
		return ArchiveVerificationSummary{}, errors.New("trace archive output path is required")
	}

	archive := Archive{
		SchemaVersion: archiveSchemaVersion,
		OrderBasis:    "caller",
		Entries:       make([]ArchiveEntry, 0, len(inputs)),
	}
	for index, input := range inputs {
		document, err := Read(input.TracePath)
		if err != nil {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d trace: %w", index+1, err)
		}
		sessionData, err := readSession(input.SessionPath)
		if err != nil {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d session: %w", index+1, err)
		}
		session, err := DecodeSession(sessionData)
		if err != nil {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d session: %w", index+1, err)
		}
		traceSHA256, err := SHA256(document)
		if err != nil {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d trace: %w", index+1, err)
		}
		if err := validateSessionBinding(session, document, traceSHA256); err != nil {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d: %w", index+1, err)
		}
		if session.Role != RoleStandalone || session.Order != OrderStandalone {
			return ArchiveVerificationSummary{}, fmt.Errorf("trace archive entry %d must be standalone", index+1)
		}
		archive.Entries = append(archive.Entries, ArchiveEntry{
			Position: index + 1,
			Session:  session,
			Trace:    document,
		})
	}

	data, err := marshalArchive(archive)
	if err != nil {
		return ArchiveVerificationSummary{}, err
	}
	if err := writeArchiveExclusive(outputPath, append(data, '\n')); err != nil {
		return ArchiveVerificationSummary{}, fmt.Errorf("trace archive: %w", err)
	}
	return archiveSummary(archive)
}

// ReadArchive verifies and reads one portable trace archive.
func ReadArchive(path string) (Archive, ArchiveVerificationSummary, error) {
	data, err := readArchive(path)
	if err != nil {
		return Archive{}, ArchiveVerificationSummary{}, fmt.Errorf("trace archive: %w", err)
	}
	archive, err := DecodeArchive(data)
	if err != nil {
		return Archive{}, ArchiveVerificationSummary{}, err
	}
	summary, err := archiveSummary(archive)
	return archive, summary, err
}

// VerifyArchive verifies one portable trace archive and returns only its safe summary.
func VerifyArchive(path string) (ArchiveVerificationSummary, error) {
	_, summary, err := ReadArchive(path)
	return summary, err
}

// DecodeArchive verifies one bounded archive document.
func DecodeArchive(data []byte) (Archive, error) {
	if len(data) == 0 {
		return Archive{}, errors.New("trace archive is empty")
	}
	if len(data) > maxArchiveBytes {
		return Archive{}, errors.New("trace archive exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Archive{}, errors.New("trace archive has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var archive Archive
	if err := decoder.Decode(&archive); err != nil {
		return Archive{}, errors.New("trace archive has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Archive{}, errors.New("trace archive has trailing data")
	}
	if err := validateArchive(&archive); err != nil {
		return Archive{}, err
	}
	return archive, nil
}

// ArchiveSHA256 returns the canonical identity of a valid archive.
func ArchiveSHA256(archive Archive) (string, error) {
	if err := validateArchive(&archive); err != nil {
		return "", err
	}
	data, err := marshalArchive(archive)
	if err != nil {
		return "", err
	}
	return sha256Bytes(data), nil
}

// AskArchive answers one fixed question after re-verifying the archive.
func AskArchive(path, questionID string) (ArchiveAnswer, error) {
	archive, summary, err := ReadArchive(path)
	if err != nil {
		return ArchiveAnswer{}, err
	}
	return AnswerArchive(archive, summary.ArchiveSHA256, questionID)
}

// AnswerArchive answers one fixed question against an already verified archive.
func AnswerArchive(archive Archive, archiveSHA256, questionID string) (ArchiveAnswer, error) {
	question, ok := archiveQuestion(questionID)
	if !ok {
		return ArchiveAnswer{}, errors.New("trace archive question ID is invalid")
	}
	if err := validateArchive(&archive); err != nil {
		return ArchiveAnswer{}, err
	}
	if !ValidSHA256(archiveSHA256) {
		return ArchiveAnswer{}, errors.New("trace archive identity is invalid")
	}
	expectedSHA256, err := ArchiveSHA256(archive)
	if err != nil {
		return ArchiveAnswer{}, err
	}
	if expectedSHA256 != archiveSHA256 {
		return ArchiveAnswer{}, errors.New("trace archive identity does not match archive")
	}
	answer := ArchiveAnswer{
		SchemaVersion: archiveSchemaVersion,
		QuestionID:    question.ID,
		Question:      question.Text,
		EvidenceState: evidence.Observed,
		ArchiveSHA256: archiveSHA256,
		Entries:       len(archive.Entries),
		Sources:       archiveSources(archive),
	}
	switch questionID {
	case ArchiveQuestionCoverage:
		for _, entry := range archive.Entries {
			if entry.Trace.Completeness != Complete {
				answer.Result = archiveResultUnknown
				answer.EvidenceState = evidence.Unknown
				answer.Unknown = 1
				answer.Reason = "at least one retained trace declares partial coverage"
				return answer, nil
			}
		}
		answer.Result = archiveResultComplete
		answer.Reason = "every retained trace declares complete coverage"
		return answer, nil
	case ArchiveQuestionSources:
		answer.Result = archiveResultAvailable
		answer.Reason = "source identities come from verified session envelopes"
		return answer, nil
	case ArchiveQuestionChange:
		return answerArchiveChange(answer, archive)
	default:
		return ArchiveAnswer{}, errors.New("trace archive question ID is invalid")
	}
}

// AskAllArchive answers the complete fixed question catalog in stable order.
func AskAllArchive(path string) ([]ArchiveAnswer, error) {
	archive, summary, err := ReadArchive(path)
	if err != nil {
		return nil, err
	}
	questions := ArchiveQuestions()
	answers := make([]ArchiveAnswer, 0, len(questions))
	for _, question := range questions {
		answer, err := AnswerArchive(archive, summary.ArchiveSHA256, question.ID)
		if err != nil {
			return nil, err
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

func answerArchiveChange(answer ArchiveAnswer, archive Archive) (ArchiveAnswer, error) {
	if len(archive.Entries) < 2 {
		answer.Result = archiveResultUnknown
		answer.EvidenceState = evidence.Unknown
		answer.Unknown = 1
		answer.Reason = "at least two retained traces are required"
		return answer, nil
	}
	for index := 1; index < len(archive.Entries); index++ {
		before, after := archive.Entries[index-1], archive.Entries[index]
		answer.Compared++
		if !sameComparisonBoundary(before, after) {
			answer.Unknown++
			continue
		}
		if before.Trace.Completeness != Complete || after.Trace.Completeness != Complete {
			answer.Unknown++
			continue
		}
		comparison, err := Compare(before.Trace, after.Trace)
		if err != nil {
			return ArchiveAnswer{}, fmt.Errorf("trace archive comparison: %w", err)
		}
		if len(comparison.Unknowns) > 0 {
			answer.Unknown++
			continue
		}
		if len(comparison.Differences) > 0 {
			answer.Changed++
		} else {
			answer.Same++
		}
	}
	switch {
	case answer.Unknown > 0:
		answer.Result = archiveResultUnknown
		answer.EvidenceState = evidence.Unknown
		answer.Reason = "an adjacent trace boundary is incomplete or has incompatible reviewed provenance"
	case answer.Changed == answer.Compared:
		answer.Result = archiveResultChanged
		answer.Reason = "every comparable adjacent boundary contains a safe category difference"
	case answer.Same == answer.Compared:
		answer.Result = archiveResultSame
		answer.Reason = "no comparable adjacent boundary contains a safe category difference"
	default:
		answer.Result = archiveResultMixed
		answer.Reason = "comparable adjacent boundaries disagree about safe category change"
	}
	return answer, nil
}

func sameComparisonBoundary(before, after ArchiveEntry) bool {
	return before.Session.Source == after.Session.Source &&
		before.Session.Adapter == after.Session.Adapter &&
		before.Session.AdapterVersion == after.Session.AdapterVersion &&
		before.Session.ProcedureSHA256 == after.Session.ProcedureSHA256 &&
		before.Trace.Scope == after.Trace.Scope
}

func archiveQuestion(id string) (ArchiveQuestion, bool) {
	for _, question := range ArchiveQuestions() {
		if question.ID == id {
			return question, true
		}
	}
	return ArchiveQuestion{}, false
}

func archiveSources(archive Archive) []ArchiveSourceSummary {
	counts := make(map[string]ArchiveSourceSummary)
	for _, entry := range archive.Entries {
		key := entry.Session.Source + "\x00" + entry.Session.Adapter
		current := counts[key]
		current.Source = entry.Session.Source
		current.Adapter = entry.Session.Adapter
		current.Entries++
		counts[key] = current
	}
	result := make([]ArchiveSourceSummary, 0, len(counts))
	for _, source := range counts {
		result = append(result, source)
	}
	slices.SortFunc(result, func(left, right ArchiveSourceSummary) int {
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
	return result
}

func archiveSummary(archive Archive) (ArchiveVerificationSummary, error) {
	data, err := marshalArchive(archive)
	if err != nil {
		return ArchiveVerificationSummary{}, err
	}
	digest := sha256Bytes(data)
	summary := ArchiveVerificationSummary{
		SchemaVersion: archive.SchemaVersion,
		OrderBasis:    archive.OrderBasis,
		Entries:       len(archive.Entries),
		Sources:       archiveSources(archive),
		ArchiveSHA256: digest,
	}
	for _, entry := range archive.Entries {
		if entry.Trace.Completeness == Complete {
			summary.Complete++
		} else {
			summary.Partial++
		}
	}
	return summary, nil
}

func validateArchive(archive *Archive) error {
	if archive.SchemaVersion != archiveSchemaVersion {
		return errors.New("trace archive has unsupported schema_version")
	}
	if archive.OrderBasis != "caller" {
		return errors.New("trace archive order_basis is invalid")
	}
	if archive.Entries == nil || len(archive.Entries) == 0 || len(archive.Entries) > maxArchiveEntries {
		return errors.New("trace archive entries are invalid")
	}
	for index := range archive.Entries {
		entry := &archive.Entries[index]
		if entry.Position != index+1 {
			return errors.New("trace archive entry positions are invalid")
		}
		if entry.Session.Role != RoleStandalone || entry.Session.Order != OrderStandalone {
			return errors.New("trace archive entries must be standalone")
		}
		if err := validate(&entry.Trace); err != nil {
			return fmt.Errorf("trace archive entry %d trace: %w", index+1, err)
		}
		traceSHA256, err := SHA256(entry.Trace)
		if err != nil {
			return fmt.Errorf("trace archive entry %d trace: %w", index+1, err)
		}
		if err := validateSessionBinding(entry.Session, entry.Trace, traceSHA256); err != nil {
			return fmt.Errorf("trace archive entry %d: %w", index+1, err)
		}
	}
	return nil
}

func marshalArchive(archive Archive) ([]byte, error) {
	if err := validateArchive(&archive); err != nil {
		return nil, err
	}
	data, err := json.Marshal(archive)
	if err != nil {
		return nil, errors.New("trace archive encoding failed")
	}
	if len(data) > maxArchiveBytes {
		return nil, errors.New("trace archive exceeds 1048576-byte limit")
	}
	return data, nil
}

func readArchive(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("archive path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("read archive")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxArchiveBytes+1))
	if err != nil || len(data) > maxArchiveBytes {
		return nil, errors.New("read archive")
	}
	return data, nil
}

func writeArchiveExclusive(path string, data []byte) error {
	if len(data) > maxArchiveBytes {
		return errors.New("archive output exceeds limit")
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

func sha256Bytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
