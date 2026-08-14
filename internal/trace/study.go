package trace

import (
	"bytes"
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
	replicationStudySchemaVersion = 1
	maxReplicationStudyRuns       = 8
	maxReplicationStudyBytes      = 4 << 20
)

const (
	// ReplicationStudyOrderBasis identifies the caller-supplied study order.
	// It is not a chronology claim.
	ReplicationStudyOrderBasis = "caller"
)

// StudyInput identifies one saved replication ledger and its matching
// question round. Input paths are used only while creating the study and are
// never retained in the portable artifact.
type StudyInput struct {
	LedgerPath string
	RoundPath  string
}

// ReplicationStudy is a portable collection of independently identified
// replication ledgers for one caller-supplied counterfactual commitment.
type ReplicationStudy struct {
	SchemaVersion  int                   `json:"schema_version"`
	ContrastSHA256 string                `json:"contrast_sha256"`
	OrderBasis     string                `json:"order_basis"`
	Runs           []ReplicationStudyRun `json:"runs"`
}

// ReplicationStudyRun retains one verified ledger and its matching fixed
// question round in caller order.
type ReplicationStudyRun struct {
	Position      int                      `json:"position"`
	Ledger        ReplicationLedger        `json:"ledger"`
	QuestionRound ReplicationQuestionRound `json:"question_round"`
}

// StudyVerificationSummary identifies a verified replication study while
// keeping aggregate outcome and evidence support separate.
type StudyVerificationSummary struct {
	SchemaVersion       int               `json:"schema_version"`
	ContrastSHA256      string            `json:"contrast_sha256"`
	OrderBasis          string            `json:"order_basis"`
	Runs                int               `json:"runs"`
	Pairs               int               `json:"pairs"`
	SupportedRuns       int               `json:"supported_runs"`
	UnknownRuns         int               `json:"unknown_runs"`
	ResetConfirmedPairs int               `json:"reset_confirmed_pairs"`
	BalancedRuns        int               `json:"balanced_runs"`
	CompletePairs       int               `json:"complete_pairs"`
	ChangedRuns         int               `json:"changed_runs"`
	NoChangeRuns        int               `json:"no_change_runs"`
	MixedRuns           int               `json:"mixed_runs"`
	UnknownPairs        int               `json:"unknown_pairs"`
	Outcome             ReplicatedOutcome `json:"outcome"`
	EvidenceState       evidence.State    `json:"evidence_state"`
	Reason              string            `json:"reason"`
	StudySHA256         string            `json:"study_sha256"`
}

// SaveReplicationStudy verifies and embeds at least two independently
// identified replication ledgers and their matching question rounds.
func SaveReplicationStudy(contrastSHA256 string, inputs []StudyInput, outputPath string) (StudyVerificationSummary, error) {
	if !ValidSHA256(contrastSHA256) {
		return StudyVerificationSummary{}, errors.New("trace replication study contrast_sha256 is invalid")
	}
	if len(inputs) < 2 || len(inputs) > maxReplicationStudyRuns {
		return StudyVerificationSummary{}, errors.New("trace replication study run count is invalid")
	}
	if strings.TrimSpace(outputPath) == "" {
		return StudyVerificationSummary{}, errors.New("trace replication study output path is required")
	}
	study := ReplicationStudy{
		SchemaVersion:  replicationStudySchemaVersion,
		ContrastSHA256: contrastSHA256,
		OrderBasis:     ReplicationStudyOrderBasis,
		Runs:           make([]ReplicationStudyRun, 0, len(inputs)),
	}
	seenLedgers := make(map[string]struct{}, len(inputs))
	seenRounds := make(map[string]struct{}, len(inputs))
	for index, input := range inputs {
		if strings.TrimSpace(input.LedgerPath) == "" || strings.TrimSpace(input.RoundPath) == "" {
			return StudyVerificationSummary{}, fmt.Errorf("trace replication study run %d paths are required", index+1)
		}
		ledger, ledgerSummary, err := ReadReplicationLedger(input.LedgerPath)
		if err != nil {
			return StudyVerificationSummary{}, fmt.Errorf("trace replication study run %d ledger: %w", index+1, err)
		}
		round, roundSummary, err := ReadReplicationQuestionRound(input.RoundPath)
		if err != nil {
			return StudyVerificationSummary{}, fmt.Errorf("trace replication study run %d question round: %w", index+1, err)
		}
		if round.LedgerSHA256 != ledgerSummary.LedgerSHA256 {
			return StudyVerificationSummary{}, fmt.Errorf("trace replication study run %d question round does not match ledger", index+1)
		}
		if _, exists := seenLedgers[ledgerSummary.LedgerSHA256]; exists {
			return StudyVerificationSummary{}, errors.New("trace replication study contains duplicate ledger identities")
		}
		if _, exists := seenRounds[roundSummary.RoundSHA256]; exists {
			return StudyVerificationSummary{}, errors.New("trace replication study contains duplicate question-round identities")
		}
		seenLedgers[ledgerSummary.LedgerSHA256] = struct{}{}
		seenRounds[roundSummary.RoundSHA256] = struct{}{}
		study.Runs = append(study.Runs, ReplicationStudyRun{
			Position:      index + 1,
			Ledger:        ledger,
			QuestionRound: round,
		})
	}
	if err := validateReplicationStudy(study); err != nil {
		return StudyVerificationSummary{}, err
	}
	data, err := json.Marshal(study)
	if err != nil {
		return StudyVerificationSummary{}, errors.New("trace replication study encoding failed")
	}
	if err := writeReplicationStudyExclusive(outputPath, append(data, '\n')); err != nil {
		return StudyVerificationSummary{}, fmt.Errorf("trace replication study: %w", err)
	}
	return replicationStudySummary(study)
}

// ReadReplicationStudy verifies and reads one portable replication study.
func ReadReplicationStudy(path string) (ReplicationStudy, StudyVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ReplicationStudy{}, StudyVerificationSummary{}, errors.New("trace replication study path is required")
	}
	data, err := readReplicationStudy(path)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, fmt.Errorf("trace replication study: %w", err)
	}
	study, err := DecodeReplicationStudy(data)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, err
	}
	summary, err := replicationStudySummary(study)
	if err != nil {
		return ReplicationStudy{}, StudyVerificationSummary{}, err
	}
	return study, summary, nil
}

// VerifyReplicationStudy verifies one saved study without reopening its
// source-specific input files.
func VerifyReplicationStudy(path string) (StudyVerificationSummary, error) {
	_, summary, err := ReadReplicationStudy(path)
	return summary, err
}

// DecodeReplicationStudy verifies one bounded study document.
func DecodeReplicationStudy(data []byte) (ReplicationStudy, error) {
	if len(data) == 0 {
		return ReplicationStudy{}, errors.New("trace replication study is empty")
	}
	if len(data) > maxReplicationStudyBytes {
		return ReplicationStudy{}, errors.New("trace replication study exceeds 4194304-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationStudy{}, errors.New("trace replication study has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var study ReplicationStudy
	if err := decoder.Decode(&study); err != nil {
		return ReplicationStudy{}, errors.New("trace replication study has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationStudy{}, errors.New("trace replication study has trailing data")
	}
	if err := validateReplicationStudy(study); err != nil {
		return ReplicationStudy{}, err
	}
	return study, nil
}

// ReplicationStudySHA256 returns the canonical identity of a valid study.
func ReplicationStudySHA256(study ReplicationStudy) (string, error) {
	if err := validateReplicationStudy(study); err != nil {
		return "", err
	}
	data, err := json.Marshal(study)
	if err != nil {
		return "", errors.New("trace replication study encoding failed")
	}
	return sha256Hex(data), nil
}

type replicationStudyProvenance struct {
	Source          string
	Adapter         string
	AdapterVersion  int
	ProcedureSHA256 string
	Scope           string
}

func replicationStudySummary(study ReplicationStudy) (StudyVerificationSummary, error) {
	if err := validateReplicationStudy(study); err != nil {
		return StudyVerificationSummary{}, err
	}
	summary := StudyVerificationSummary{
		SchemaVersion:  replicationStudySchemaVersion,
		ContrastSHA256: study.ContrastSHA256,
		OrderBasis:     study.OrderBasis,
		Runs:           len(study.Runs),
	}
	for _, run := range study.Runs {
		ledgerSummary, err := replicationLedgerSummary(run.Ledger)
		if err != nil {
			return StudyVerificationSummary{}, err
		}
		summary.Pairs += ledgerSummary.Pairs
		summary.ResetConfirmedPairs += ledgerSummary.ResetConfirmedPairs
		summary.CompletePairs += ledgerSummary.CompletePairs
		summary.UnknownPairs += ledgerSummary.UnknownPairs
		if ledgerSummary.OrderBalanced {
			summary.BalancedRuns++
		}
		if replicationStudyRunSupported(ledgerSummary) {
			summary.SupportedRuns++
			switch ledgerSummary.Outcome {
			case ReplicatedChange:
				summary.ChangedRuns++
			case NoChangeObserved:
				summary.NoChangeRuns++
			case MixedInconsistent:
				summary.MixedRuns++
			}
		} else {
			summary.UnknownRuns++
		}
	}
	summary.EvidenceState = evidence.Observed
	switch {
	case summary.UnknownRuns > 0:
		summary.Outcome = ReplicationUnknown
		summary.EvidenceState = evidence.Unknown
		summary.Reason = "at least one replicated run lacks balanced order, reset, complete capture, or comparison support"
	case summary.ChangedRuns == summary.Runs:
		summary.Outcome = ReplicatedChange
		summary.Reason = "every supported replicated run contains a safe category difference"
	case summary.NoChangeRuns == summary.Runs:
		summary.Outcome = NoChangeObserved
		summary.Reason = "no supported replicated run contains a safe category difference"
	case summary.MixedRuns == summary.Runs:
		summary.Outcome = MixedInconsistent
		summary.Reason = "every supported replicated run contains internally inconsistent pair outcomes"
	default:
		summary.Outcome = MixedInconsistent
		summary.Reason = "supported replicated runs disagree about safe category change"
	}
	digest, err := ReplicationStudySHA256(study)
	if err != nil {
		return StudyVerificationSummary{}, err
	}
	summary.StudySHA256 = digest
	return summary, nil
}

func replicationStudyRunSupported(summary ReplicationLedgerVerificationSummary) bool {
	return summary.OrderBalanced &&
		summary.ResetConfirmedPairs == summary.Pairs &&
		summary.CompletePairs == summary.Pairs &&
		summary.UnknownPairs == 0 &&
		summary.EvidenceState == evidence.Observed &&
		summary.Outcome != ReplicationUnknown
}

func validateReplicationStudy(study ReplicationStudy) error {
	if study.SchemaVersion != replicationStudySchemaVersion {
		return errors.New("trace replication study has unsupported schema_version")
	}
	if !ValidSHA256(study.ContrastSHA256) {
		return errors.New("trace replication study contrast_sha256 is invalid")
	}
	if study.OrderBasis != ReplicationStudyOrderBasis {
		return errors.New("trace replication study order_basis is invalid")
	}
	if len(study.Runs) < 2 || len(study.Runs) > maxReplicationStudyRuns {
		return errors.New("trace replication study run count is invalid")
	}
	seenLedgers := make(map[string]struct{}, len(study.Runs))
	seenRounds := make(map[string]struct{}, len(study.Runs))
	var reference replicationStudyProvenance
	for index, run := range study.Runs {
		if run.Position != index+1 {
			return errors.New("trace replication study run positions are invalid")
		}
		ledgerSummary, err := replicationLedgerSummary(run.Ledger)
		if err != nil {
			return fmt.Errorf("trace replication study run %d ledger is invalid", index+1)
		}
		roundSHA256, err := ReplicationQuestionRoundSHA256(run.QuestionRound)
		if err != nil {
			return fmt.Errorf("trace replication study run %d question round is invalid", index+1)
		}
		if run.QuestionRound.LedgerSHA256 != ledgerSummary.LedgerSHA256 {
			return fmt.Errorf("trace replication study run %d question round does not match ledger", index+1)
		}
		expectedAnswers, err := AnswerAllReplicationQuestions(run.Ledger, ledgerSummary)
		if err != nil {
			return fmt.Errorf("trace replication study run %d question answers are invalid", index+1)
		}
		if !slices.Equal(run.QuestionRound.Answers, expectedAnswers) {
			return fmt.Errorf("trace replication study run %d question answers do not match ledger", index+1)
		}
		if _, exists := seenLedgers[ledgerSummary.LedgerSHA256]; exists {
			return errors.New("trace replication study contains duplicate ledger identities")
		}
		if _, exists := seenRounds[roundSHA256]; exists {
			return errors.New("trace replication study contains duplicate question-round identities")
		}
		seenLedgers[ledgerSummary.LedgerSHA256] = struct{}{}
		seenRounds[roundSHA256] = struct{}{}
		provenance, err := replicationStudyRunProvenance(run.Ledger)
		if err != nil {
			return fmt.Errorf("trace replication study run %d provenance is invalid", index+1)
		}
		if index == 0 {
			reference = provenance
		} else if provenance != reference {
			return errors.New("trace replication study run provenance does not match")
		}
	}
	return nil
}

func replicationStudyRunProvenance(ledger ReplicationLedger) (replicationStudyProvenance, error) {
	if len(ledger.Pairs) == 0 {
		return replicationStudyProvenance{}, errors.New("replication ledger has no pairs")
	}
	pair := ledger.Pairs[0].Pair
	return replicationStudyProvenance{
		Source:          pair.Source,
		Adapter:         pair.Adapter,
		AdapterVersion:  pair.AdapterVersion,
		ProcedureSHA256: pair.ProcedureSHA256,
		Scope:           pair.Scope,
	}, nil
}

func readReplicationStudy(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("study path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("read study")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxReplicationStudyBytes+1))
	if err != nil || len(data) > maxReplicationStudyBytes {
		return nil, errors.New("read study")
	}
	return data, nil
}

func writeReplicationStudyExclusive(path string, data []byte) error {
	if len(data) > maxReplicationStudyBytes {
		return errors.New("study output exceeds limit")
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
