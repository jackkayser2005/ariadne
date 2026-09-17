package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	replicationLedgerSchemaVersion = 1
	maxReplicationPairs            = maxArchiveEntries / 2
)

// ReplicationResetPolicy identifies the caller assertion retained by a
// source-neutral replication ledger. It does not prove that a reset occurred.
const ReplicationResetPolicy = "caller-confirmed-before-each-session"

// ReplicationPairInput identifies one already-produced matched pair. The
// caller supplies the reset assertion because this package does not control a
// source-specific runner.
type ReplicationPairInput struct {
	BaselineTracePath    string
	TreatmentTracePath   string
	BaselineSessionPath  string
	TreatmentSessionPath string
	ResetConfirmed       bool
}

// ReplicationPair retains one verified matched pair without source paths or
// captured values. Comparison is recomputed when a ledger is verified.
type ReplicationPair struct {
	Position         int                            `json:"position"`
	ResetConfirmed   bool                           `json:"reset_confirmed"`
	Pair             SessionPairVerificationSummary `json:"pair"`
	BaselineSession  Session                        `json:"baseline_session"`
	TreatmentSession Session                        `json:"treatment_session"`
	BaselineTrace    Document                       `json:"baseline_trace"`
	TreatmentTrace   Document                       `json:"treatment_trace"`
	Comparison       Comparison                     `json:"comparison"`
}

// ReplicationLedger is a portable, raw-value-free collection of matched
// counterfactual pairs. Pair order and reset assertions remain explicit.
type ReplicationLedger struct {
	SchemaVersion int               `json:"schema_version"`
	ResetPolicy   string            `json:"reset_policy"`
	Pairs         []ReplicationPair `json:"pairs"`
}

// ReplicationLedgerVerificationSummary identifies a verified ledger and its
// bounded aggregate classification.
type ReplicationLedgerVerificationSummary struct {
	SchemaVersion          int               `json:"schema_version"`
	ResetPolicy            string            `json:"reset_policy"`
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
	EvidenceState          evidence.State    `json:"evidence_state"`
	Reason                 string            `json:"reason"`
	LedgerSHA256           string            `json:"ledger_sha256"`
}

// ReplicationPairEvidenceState reports whether one retained pair has complete
// reset, capture, and comparison support. It does not classify the pair's
// outcome.
func ReplicationPairEvidenceState(pair ReplicationPair) evidence.State {
	if !pair.ResetConfirmed ||
		pair.Pair.BaselineCompleteness != Complete ||
		pair.Pair.TreatmentCompleteness != Complete ||
		len(pair.Comparison.Unknowns) > 0 {
		return evidence.Unknown
	}
	return evidence.Observed
}

// SaveReplicationLedger verifies and embeds one or more matched pairs
// without overwriting an existing output path.
func SaveReplicationLedger(inputs []ReplicationPairInput, outputPath string) (ReplicationLedgerVerificationSummary, error) {
	if len(inputs) == 0 || len(inputs) > maxReplicationPairs {
		return ReplicationLedgerVerificationSummary{}, errors.New("trace replication pair count is invalid")
	}
	if outputPath == "" {
		return ReplicationLedgerVerificationSummary{}, errors.New("trace replication ledger output path is required")
	}
	ledger := ReplicationLedger{
		SchemaVersion: replicationLedgerSchemaVersion,
		ResetPolicy:   ReplicationResetPolicy,
		Pairs:         make([]ReplicationPair, 0, len(inputs)),
	}
	for index, input := range inputs {
		pair, err := newReplicationPair(input)
		if err != nil {
			return ReplicationLedgerVerificationSummary{}, fmt.Errorf("trace replication ledger pair %d verification failed", index+1)
		}
		pair.Position = index + 1
		if len(ledger.Pairs) > 0 {
			if err := validateReplicationProvenance(ledger.Pairs[0], pair); err != nil {
				return ReplicationLedgerVerificationSummary{}, fmt.Errorf("trace replication ledger: %w", err)
			}
		}
		ledger.Pairs = append(ledger.Pairs, pair)
	}
	if err := validateReplicationLedger(ledger); err != nil {
		return ReplicationLedgerVerificationSummary{}, err
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		return ReplicationLedgerVerificationSummary{}, errors.New("trace replication ledger encoding failed")
	}
	if err := writeArchiveExclusive(outputPath, append(data, '\n')); err != nil {
		return ReplicationLedgerVerificationSummary{}, fmt.Errorf("trace replication ledger: %w", err)
	}
	return replicationLedgerSummary(ledger)
}

// ReadReplicationLedger verifies and reads one portable replication ledger.
func ReadReplicationLedger(path string) (ReplicationLedger, ReplicationLedgerVerificationSummary, error) {
	if path == "" {
		return ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, errors.New("trace replication ledger path is required")
	}
	data, err := readArchive(path)
	if err != nil {
		return ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, fmt.Errorf("trace replication ledger: %w", err)
	}
	ledger, err := DecodeReplicationLedger(data)
	if err != nil {
		return ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, err
	}
	summary, err := replicationLedgerSummary(ledger)
	if err != nil {
		return ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, err
	}
	return ledger, summary, nil
}

// VerifyReplicationLedger verifies one saved ledger without reopening its
// source-specific input files.
func VerifyReplicationLedger(path string) (ReplicationLedgerVerificationSummary, error) {
	_, summary, err := ReadReplicationLedger(path)
	return summary, err
}

// DecodeReplicationLedger verifies one bounded ledger document.
func DecodeReplicationLedger(data []byte) (ReplicationLedger, error) {
	if len(data) == 0 {
		return ReplicationLedger{}, errors.New("trace replication ledger is empty")
	}
	if len(data) > maxArchiveBytes {
		return ReplicationLedger{}, errors.New("trace replication ledger exceeds 1048576-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicationLedger{}, errors.New("trace replication ledger has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var ledger ReplicationLedger
	if err := decoder.Decode(&ledger); err != nil {
		return ReplicationLedger{}, errors.New("trace replication ledger has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicationLedger{}, errors.New("trace replication ledger has trailing data")
	}
	if err := validateReplicationLedger(ledger); err != nil {
		return ReplicationLedger{}, err
	}
	return ledger, nil
}

// ReplicationLedgerSHA256 returns the canonical identity of a valid ledger.
func ReplicationLedgerSHA256(ledger ReplicationLedger) (string, error) {
	if err := validateReplicationLedger(ledger); err != nil {
		return "", err
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		return "", errors.New("trace replication ledger encoding failed")
	}
	return sha256Hex(data), nil
}

func newReplicationPair(input ReplicationPairInput) (ReplicationPair, error) {
	if input.BaselineTracePath == "" || input.TreatmentTracePath == "" || input.BaselineSessionPath == "" || input.TreatmentSessionPath == "" {
		return ReplicationPair{}, errors.New("trace replication pair paths are required")
	}
	baselineTrace, err := Read(input.BaselineTracePath)
	if err != nil {
		return ReplicationPair{}, err
	}
	treatmentTrace, err := Read(input.TreatmentTracePath)
	if err != nil {
		return ReplicationPair{}, err
	}
	baselineSession, err := readAndDecodeSession(input.BaselineSessionPath)
	if err != nil {
		return ReplicationPair{}, err
	}
	treatmentSession, err := readAndDecodeSession(input.TreatmentSessionPath)
	if err != nil {
		return ReplicationPair{}, err
	}
	result, err := CompareSessionPair(input.BaselineSessionPath, input.BaselineTracePath, input.TreatmentSessionPath, input.TreatmentTracePath)
	if err != nil {
		return ReplicationPair{}, err
	}
	return ReplicationPair{
		ResetConfirmed:   input.ResetConfirmed,
		Pair:             result.Pair,
		BaselineSession:  baselineSession,
		TreatmentSession: treatmentSession,
		BaselineTrace:    baselineTrace,
		TreatmentTrace:   treatmentTrace,
		Comparison:       result.Comparison,
	}, nil
}

func readAndDecodeSession(path string) (Session, error) {
	data, err := readSession(path)
	if err != nil {
		return Session{}, err
	}
	return DecodeSession(data)
}

func validateReplicationLedger(ledger ReplicationLedger) error {
	if ledger.SchemaVersion != replicationLedgerSchemaVersion {
		return errors.New("trace replication ledger has unsupported schema_version")
	}
	if ledger.ResetPolicy != ReplicationResetPolicy {
		return errors.New("trace replication ledger reset_policy is invalid")
	}
	if len(ledger.Pairs) == 0 || len(ledger.Pairs) > maxReplicationPairs {
		return errors.New("trace replication ledger pair count is invalid")
	}
	for index, pair := range ledger.Pairs {
		if pair.Position != index+1 {
			return errors.New("trace replication ledger pair positions are invalid")
		}
		if err := validateReplicationPair(pair); err != nil {
			return fmt.Errorf("trace replication ledger pair %d: %w", index+1, err)
		}
		if index > 0 {
			if err := validateReplicationProvenance(ledger.Pairs[0], pair); err != nil {
				return fmt.Errorf("trace replication ledger: %w", err)
			}
		}
	}
	return nil
}

func validateReplicationPair(pair ReplicationPair) error {
	if err := validate(&pair.BaselineTrace); err != nil {
		return errors.New("baseline trace is invalid")
	}
	if err := validate(&pair.TreatmentTrace); err != nil {
		return errors.New("treatment trace is invalid")
	}
	baselineTraceSHA256, err := SHA256(pair.BaselineTrace)
	if err != nil {
		return errors.New("baseline trace identity is invalid")
	}
	treatmentTraceSHA256, err := SHA256(pair.TreatmentTrace)
	if err != nil {
		return errors.New("treatment trace identity is invalid")
	}
	if err := validateSessionBinding(pair.BaselineSession, pair.BaselineTrace, baselineTraceSHA256); err != nil {
		return errors.New("baseline session binding is invalid")
	}
	if err := validateSessionBinding(pair.TreatmentSession, pair.TreatmentTrace, treatmentTraceSHA256); err != nil {
		return errors.New("treatment session binding is invalid")
	}
	if err := validateSessionPairMetadata(pair.BaselineSession, pair.TreatmentSession); err != nil {
		return err
	}
	expectedPair, err := embeddedSessionPairSummary(pair.BaselineSession, pair.TreatmentSession)
	if err != nil {
		return err
	}
	if pair.Pair != expectedPair {
		return errors.New("session pair summary does not match embedded sessions")
	}
	expectedComparison, err := Compare(pair.BaselineTrace, pair.TreatmentTrace)
	if err != nil {
		return errors.New("trace comparison is invalid")
	}
	if !reflect.DeepEqual(pair.Comparison, expectedComparison) {
		return errors.New("trace comparison does not match embedded traces")
	}
	return nil
}

func embeddedSessionPairSummary(baseline, treatment Session) (SessionPairVerificationSummary, error) {
	baselineSessionSHA256, err := SessionSHA256(baseline)
	if err != nil {
		return SessionPairVerificationSummary{}, errors.New("baseline session identity is invalid")
	}
	treatmentSessionSHA256, err := SessionSHA256(treatment)
	if err != nil {
		return SessionPairVerificationSummary{}, errors.New("treatment session identity is invalid")
	}
	expectedPairSHA256, err := SessionPairSHA256(baseline.TraceSHA256, treatment.TraceSHA256, SessionPairInput{
		Adapter:         baseline.Adapter,
		AdapterVersion:  baseline.AdapterVersion,
		Source:          baseline.Source,
		ProcedureSHA256: baseline.ProcedureSHA256,
		Scope:           baseline.Scope,
		Order:           baseline.Order,
	})
	if err != nil {
		return SessionPairVerificationSummary{}, err
	}
	if baseline.PairSHA256 != expectedPairSHA256 || treatment.PairSHA256 != expectedPairSHA256 {
		return SessionPairVerificationSummary{}, errors.New("session pair identity is invalid")
	}
	return SessionPairVerificationSummary{
		SchemaVersion:          sessionSchemaVersion,
		PairSHA256:             expectedPairSHA256,
		Source:                 baseline.Source,
		Adapter:                baseline.Adapter,
		AdapterVersion:         baseline.AdapterVersion,
		ProcedureSHA256:        baseline.ProcedureSHA256,
		Scope:                  baseline.Scope,
		Order:                  baseline.Order,
		BaselineTraceSHA256:    baseline.TraceSHA256,
		TreatmentTraceSHA256:   treatment.TraceSHA256,
		BaselineCompleteness:   baseline.Completeness,
		TreatmentCompleteness:  treatment.Completeness,
		BaselineSessionSHA256:  baselineSessionSHA256,
		TreatmentSessionSHA256: treatmentSessionSHA256,
	}, nil
}

func validateReplicationProvenance(reference, candidate ReplicationPair) error {
	left, right := reference.Pair, candidate.Pair
	if left.Source != right.Source || left.Adapter != right.Adapter || left.AdapterVersion != right.AdapterVersion || left.ProcedureSHA256 != right.ProcedureSHA256 || left.Scope != right.Scope {
		return errors.New("replication pair provenance does not match")
	}
	return nil
}

func replicationLedgerSummary(ledger ReplicationLedger) (ReplicationLedgerVerificationSummary, error) {
	if err := validateReplicationLedger(ledger); err != nil {
		return ReplicationLedgerVerificationSummary{}, err
	}
	observations := make([]ReplicatedPairObservation, 0, len(ledger.Pairs))
	summary := ReplicationLedgerVerificationSummary{
		SchemaVersion: replicationLedgerSchemaVersion,
		ResetPolicy:   ledger.ResetPolicy,
		Pairs:         len(ledger.Pairs),
	}
	for _, pair := range ledger.Pairs {
		switch pair.Pair.Order {
		case OrderBaselineTreatment:
			summary.BaselineTreatmentPairs++
		case OrderTreatmentBaseline:
			summary.TreatmentBaselinePairs++
		}
		if pair.ResetConfirmed {
			summary.ResetConfirmedPairs++
		}
		if pair.Pair.BaselineCompleteness == Complete && pair.Pair.TreatmentCompleteness == Complete {
			summary.CompletePairs++
		}
		unknowns := len(pair.Comparison.Unknowns)
		state := ReplicationPairEvidenceState(pair)
		if state == evidence.Unknown && unknowns == 0 {
			unknowns++
		}
		observations = append(observations, ReplicatedPairObservation{
			Differences:   len(pair.Comparison.Differences),
			Unknowns:      unknowns,
			EvidenceState: state,
		})
	}
	classification := ClassifyReplicatedPairs(observations)
	summary.ChangedPairs = classification.ChangedPairs
	summary.NoChangePairs = classification.NoChangePairs
	summary.UnknownPairs = classification.UnknownPairs
	summary.Outcome = classification.Outcome
	summary.EvidenceState = classification.EvidenceState
	summary.OrderBalanced = summary.BaselineTreatmentPairs > 0 && summary.BaselineTreatmentPairs == summary.TreatmentBaselinePairs
	switch {
	case !summary.OrderBalanced:
		summary.Outcome = ReplicationUnknown
		summary.EvidenceState = evidence.Unknown
		summary.Reason = "equal nonzero counts of both explicit pair orders are required"
	case summary.UnknownPairs > 0:
		summary.Reason = "at least one retained pair lacks complete reset, capture, or comparison support"
	case summary.Outcome == ReplicatedChange:
		summary.Reason = "every retained pair contains a safe category difference"
	case summary.Outcome == NoChangeObserved:
		summary.Reason = "no retained pair contains a safe category difference"
	default:
		summary.Reason = "retained pairs disagree about safe category change"
	}
	digest, err := ReplicationLedgerSHA256(ledger)
	if err != nil {
		return ReplicationLedgerVerificationSummary{}, err
	}
	summary.LedgerSHA256 = digest
	return summary, nil
}
