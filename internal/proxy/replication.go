package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	// ProxyReplicationSchemaVersion is the proxy replication receipt schema.
	ProxyReplicationSchemaVersion = 1
	// ProxyReplicationResetPolicy records the isolation applied before every
	// capture. It does not assert reset of any remote or target state.
	ProxyReplicationResetPolicy = "fresh-process-and-loopback-proxy-before-each-session"
	// ProxyReplicationStatusComplete records a completed receipt or pair.
	ProxyReplicationStatusComplete = "complete"
	// ProxyReplicationStatusIncomplete records an interrupted receipt or pair.
	ProxyReplicationStatusIncomplete = "incomplete"
	maxProxyReplicationPairs         = 8
	maxProxyReplicationBytes         = 64 << 10
)

// ReplicationInput describes one matched proxy counterfactual. The runner
// appends BaselineArg or TreatmentArg as the final argument; those values and
// all other process arguments remain outside the receipt.
type ReplicationInput struct {
	ProcedurePath string
	ProgramPath   string
	SharedArgs    []string
	BaselineArg   string
	TreatmentArg  string
	OutputDir     string
	Pairs         int
}

// ReplicatedRunRecord records safe execution metadata without process paths,
// arguments, condition values, credentials, authorities, or traffic.
type ReplicatedRunRecord struct {
	SchemaVersion              int                    `json:"schema_version"`
	Adapter                    string                 `json:"adapter"`
	AdapterVersion             int                    `json:"adapter_version"`
	Scope                      string                 `json:"scope"`
	PairsPerOrder              int                    `json:"pairs_per_order"`
	ResetPolicy                string                 `json:"reset_policy"`
	ControlledArgumentPosition string                 `json:"controlled_argument_position"`
	ControlledArgumentCount    int                    `json:"controlled_argument_count"`
	ConditionValuesWithheld    bool                   `json:"condition_values_withheld"`
	ExecutionIdentitySHA256    string                 `json:"execution_identity_sha256"`
	Status                     string                 `json:"status"`
	CompletedPairs             int                    `json:"completed_pairs"`
	FailurePair                int                    `json:"failure_pair,omitempty"`
	FailureOrder               string                 `json:"failure_order,omitempty"`
	Pairs                      []ReplicatedPairRecord `json:"pairs"`
}

// ReplicatedPairRecord identifies one ordered matched pair and its safe
// verified identities.
type ReplicatedPairRecord struct {
	Pair                   int    `json:"pair"`
	Order                  string `json:"order"`
	Directory              string `json:"directory"`
	FirstSession           string `json:"first_session"`
	SecondSession          string `json:"second_session"`
	Status                 string `json:"status"`
	BaselineTraceSHA256    string `json:"baseline_trace_sha256,omitempty"`
	TreatmentTraceSHA256   string `json:"treatment_trace_sha256,omitempty"`
	BaselineSessionSHA256  string `json:"baseline_session_sha256,omitempty"`
	TreatmentSessionSHA256 string `json:"treatment_session_sha256,omitempty"`
	PairSHA256             string `json:"pair_sha256,omitempty"`
}

// ReplicatedPairSummary is the safe verification result for one ordered pair.
type ReplicatedPairSummary struct {
	Pair          int            `json:"pair"`
	Order         string         `json:"order"`
	Outcome       string         `json:"outcome"`
	EvidenceState evidence.State `json:"evidence_state"`
	Differences   int            `json:"differences"`
	Unknowns      int            `json:"unknowns"`
	PairSHA256    string         `json:"pair_sha256,omitempty"`
}

// ReplicationSummary is a raw-value-free aggregate verification result.
type ReplicationSummary struct {
	SchemaVersion           int                             `json:"schema_version"`
	Adapter                 string                          `json:"adapter"`
	AdapterVersion          int                             `json:"adapter_version"`
	Scope                   string                          `json:"scope"`
	ResetPolicy             string                          `json:"reset_policy"`
	ControlledArgumentCount int                             `json:"controlled_argument_count"`
	ConditionValuesWithheld bool                            `json:"condition_values_withheld"`
	ExecutionIdentitySHA256 string                          `json:"execution_identity_sha256"`
	ReceiptSHA256           string                          `json:"receipt_sha256"`
	Pairs                   int                             `json:"pairs"`
	PairsPerOrder           int                             `json:"pairs_per_order"`
	BaselinePairs           int                             `json:"baseline_treatment_pairs"`
	TreatmentPairs          int                             `json:"treatment_baseline_pairs"`
	Outcome                 portabletrace.ReplicatedOutcome `json:"outcome"`
	EvidenceState           evidence.State                  `json:"evidence_state"`
	CompletedPairs          int                             `json:"completed_pairs"`
	ChangedPairs            int                             `json:"changed_pairs"`
	NoChangePairs           int                             `json:"no_change_pairs"`
	UnknownPairs            int                             `json:"unknown_pairs"`
	PairSummaries           []ReplicatedPairSummary         `json:"pair_summaries"`
}

type proxyCapture func(string, string, []string, string) (CaptureSummary, error)

// RunReplicated runs matched baseline/treatment pairs in both orders. Every
// session calls Capture, which creates a fresh process, listener, credential,
// and cleanup boundary.
func RunReplicated(ctx context.Context, input ReplicationInput) error {
	if strings.TrimSpace(input.ProcedurePath) == "" || strings.TrimSpace(input.ProgramPath) == "" || strings.TrimSpace(input.OutputDir) == "" {
		return runReplicatedWith(ctx, input, Capture, hashExecutable)
	}
	stagedProgram, executionIdentity, cleanup, err := stageExecutable(input.ProgramPath)
	if err != nil {
		return err
	}
	defer cleanup()
	input.ProgramPath = stagedProgram
	return runReplicatedWith(ctx, input, Capture, func(string) (string, error) {
		return executionIdentity, nil
	})
}

func runReplicatedWith(ctx context.Context, input ReplicationInput, capture proxyCapture, executableIdentity func(string) (string, error)) error {
	procedure, procedureSHA256, executionIdentity, err := validateReplicationInput(input, executableIdentity)
	if err != nil {
		return err
	}
	if capture == nil {
		return errors.New("proxy capture is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(input.OutputDir), 0o700); err != nil {
		return errors.New("create proxy replication parent")
	}
	if err := os.Mkdir(input.OutputDir, 0o700); err != nil {
		return errors.New("create proxy replication output")
	}

	record := ReplicatedRunRecord{
		SchemaVersion:              ProxyReplicationSchemaVersion,
		Adapter:                    Adapter,
		AdapterVersion:             AdapterVersion,
		Scope:                      procedure.Scope,
		PairsPerOrder:              input.Pairs,
		ResetPolicy:                ProxyReplicationResetPolicy,
		ControlledArgumentPosition: "final",
		ControlledArgumentCount:    1,
		ConditionValuesWithheld:    true,
		ExecutionIdentitySHA256:    executionIdentity,
		Status:                     ProxyReplicationStatusIncomplete,
		Pairs:                      make([]ReplicatedPairRecord, 0, input.Pairs*2),
	}
	orders := []struct {
		name          string
		firstSession  string
		secondSession string
	}{
		{name: portabletrace.OrderBaselineTreatment, firstSession: portabletrace.RoleBaseline, secondSession: portabletrace.RoleTreatment},
		{name: portabletrace.OrderTreatmentBaseline, firstSession: portabletrace.RoleTreatment, secondSession: portabletrace.RoleBaseline},
	}
	for pair := 1; pair <= input.Pairs; pair++ {
		for _, order := range orders {
			directory := fmt.Sprintf("pair-%03d-%s", pair, order.name)
			pairRecord := ReplicatedPairRecord{
				Pair:          pair,
				Order:         order.name,
				Directory:     directory,
				FirstSession:  order.firstSession,
				SecondSession: order.secondSession,
				Status:        ProxyReplicationStatusIncomplete,
			}
			pairDirectory := filepath.Join(input.OutputDir, directory)
			if err := os.Mkdir(pairDirectory, 0o700); err != nil {
				return writeReplicationFailure(input.OutputDir, record, pairRecord, err)
			}
			if err := os.Mkdir(filepath.Join(pairDirectory, "baseline"), 0o700); err != nil {
				return writeReplicationFailure(input.OutputDir, record, pairRecord, errors.New("create proxy replication baseline"))
			}
			if err := os.Mkdir(filepath.Join(pairDirectory, "treatment"), 0o700); err != nil {
				return writeReplicationFailure(input.OutputDir, record, pairRecord, errors.New("create proxy replication treatment"))
			}
			if err := ctx.Err(); err != nil {
				return writeReplicationFailure(input.OutputDir, record, pairRecord, err)
			}

			pairSummary, err := runProxyPair(ctx, input, procedureSHA256, pairDirectory, order.name, capture)
			if err != nil {
				return writeReplicationFailure(input.OutputDir, record, pairRecord, err)
			}
			pairRecord.Status = ProxyReplicationStatusComplete
			pairRecord.BaselineTraceSHA256 = pairSummary.BaselineTraceSHA256
			pairRecord.TreatmentTraceSHA256 = pairSummary.TreatmentTraceSHA256
			pairRecord.BaselineSessionSHA256 = pairSummary.BaselineSessionSHA256
			pairRecord.TreatmentSessionSHA256 = pairSummary.TreatmentSessionSHA256
			pairRecord.PairSHA256 = pairSummary.PairSHA256
			record.Pairs = append(record.Pairs, pairRecord)
			record.CompletedPairs = completedReplicationPairCount(record.Pairs)
		}
	}
	record.Status = ProxyReplicationStatusComplete
	return writeReplicationRecord(input.OutputDir, record)
}

// VerifyReplicated verifies a proxy replication receipt and recomputes every
// complete pair from its bound trace and session files.
func VerifyReplicated(rootDir string) (ReplicationSummary, error) {
	record, receiptData, err := readReplicationRecord(rootDir)
	if err != nil {
		return ReplicationSummary{}, err
	}
	if err := validateReplicationRecord(rootDir, record); err != nil {
		return ReplicationSummary{}, err
	}

	totalPairs := record.PairsPerOrder * 2
	pairSummaries := make([]ReplicatedPairSummary, 0, totalPairs)
	byKey := make(map[string]ReplicatedPairSummary, len(record.Pairs))
	expectedProcedureSHA256 := ""
	for _, pair := range record.Pairs {
		result := ReplicatedPairSummary{Pair: pair.Pair, Order: pair.Order, Outcome: "unknown", EvidenceState: evidence.Unknown}
		if pair.Status == ProxyReplicationStatusComplete {
			var observedProcedureSHA256 string
			result, observedProcedureSHA256, err = verifyProxyPair(rootDir, record, pair, expectedProcedureSHA256)
			if err != nil {
				return ReplicationSummary{}, err
			}
			if expectedProcedureSHA256 == "" {
				expectedProcedureSHA256 = observedProcedureSHA256
			}
		}
		byKey[replicationPairKey(pair.Pair, pair.Order)] = result
	}
	for pair := 1; pair <= record.PairsPerOrder; pair++ {
		for _, order := range []string{portabletrace.OrderBaselineTreatment, portabletrace.OrderTreatmentBaseline} {
			result, ok := byKey[replicationPairKey(pair, order)]
			if !ok {
				result = ReplicatedPairSummary{Pair: pair, Order: order, Outcome: "unknown", EvidenceState: evidence.Unknown}
			}
			pairSummaries = append(pairSummaries, result)
		}
	}
	classification := classifyProxyPairs(pairSummaries)
	return ReplicationSummary{
		SchemaVersion:           record.SchemaVersion,
		Adapter:                 record.Adapter,
		AdapterVersion:          record.AdapterVersion,
		Scope:                   record.Scope,
		ResetPolicy:             record.ResetPolicy,
		ControlledArgumentCount: record.ControlledArgumentCount,
		ConditionValuesWithheld: record.ConditionValuesWithheld,
		ExecutionIdentitySHA256: record.ExecutionIdentitySHA256,
		ReceiptSHA256:           digestReplication(receiptData),
		Pairs:                   totalPairs,
		PairsPerOrder:           record.PairsPerOrder,
		BaselinePairs:           record.PairsPerOrder,
		TreatmentPairs:          record.PairsPerOrder,
		Outcome:                 classification.Outcome,
		EvidenceState:           classification.EvidenceState,
		CompletedPairs:          record.CompletedPairs,
		ChangedPairs:            classification.ChangedPairs,
		NoChangePairs:           classification.NoChangePairs,
		UnknownPairs:            classification.UnknownPairs,
		PairSummaries:           pairSummaries,
	}, nil
}

func validateReplicationInput(input ReplicationInput, executableIdentity func(string) (string, error)) (Procedure, string, string, error) {
	if strings.TrimSpace(input.ProcedurePath) == "" || strings.TrimSpace(input.ProgramPath) == "" || strings.TrimSpace(input.OutputDir) == "" {
		return Procedure{}, "", "", errors.New("proxy replication paths and program are required")
	}
	if input.Pairs < 1 || input.Pairs > maxProxyReplicationPairs {
		return Procedure{}, "", "", fmt.Errorf("pairs must be between 1 and %d", maxProxyReplicationPairs)
	}
	if input.BaselineArg == input.TreatmentArg {
		return Procedure{}, "", "", errors.New("baseline and treatment arguments must differ")
	}
	procedure, _, err := ReadProcedure(input.ProcedurePath)
	if err != nil {
		return Procedure{}, "", "", fmt.Errorf("proxy replication procedure: %w", err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		return Procedure{}, "", "", errors.New("proxy replication procedure identity failed")
	}
	if err := validateProgram(input.ProgramPath, append(append([]string(nil), input.SharedArgs...), input.BaselineArg)); err != nil {
		return Procedure{}, "", "", err
	}
	if err := validateProgram(input.ProgramPath, append(append([]string(nil), input.SharedArgs...), input.TreatmentArg)); err != nil {
		return Procedure{}, "", "", err
	}
	if executableIdentity == nil {
		return Procedure{}, "", "", errors.New("proxy executable identity is required")
	}
	executionIdentity, err := executableIdentity(input.ProgramPath)
	if err != nil || !portabletrace.ValidSHA256(executionIdentity) {
		return Procedure{}, "", "", errors.New("proxy executable identity failed")
	}
	return procedure, procedureSHA256, executionIdentity, nil
}

func runProxyPair(ctx context.Context, input ReplicationInput, procedureSHA256, directory, order string, capture proxyCapture) (portabletrace.SessionPairVerificationSummary, error) {
	paths := replicationPaths(directory)
	if order == portabletrace.OrderBaselineTreatment {
		if err := runProxySession(ctx, input, procedureSHA256, portabletrace.RoleBaseline, paths.baselineTrace, capture); err != nil {
			return portabletrace.SessionPairVerificationSummary{}, err
		}
		if err := runProxySession(ctx, input, procedureSHA256, portabletrace.RoleTreatment, paths.treatmentTrace, capture); err != nil {
			return portabletrace.SessionPairVerificationSummary{}, err
		}
	} else {
		if err := runProxySession(ctx, input, procedureSHA256, portabletrace.RoleTreatment, paths.treatmentTrace, capture); err != nil {
			return portabletrace.SessionPairVerificationSummary{}, err
		}
		if err := runProxySession(ctx, input, procedureSHA256, portabletrace.RoleBaseline, paths.baselineTrace, capture); err != nil {
			return portabletrace.SessionPairVerificationSummary{}, err
		}
	}
	result, err := portabletrace.SaveSessionPair(paths.baselineTrace, paths.treatmentTrace, paths.baselineSession, paths.treatmentSession, portabletrace.SessionPairInput{
		Adapter:         Adapter,
		AdapterVersion:  AdapterVersion,
		ProcedureSHA256: procedureSHA256,
		Scope:           "outbound",
		Order:           order,
	})
	if err != nil {
		return portabletrace.SessionPairVerificationSummary{}, fmt.Errorf("proxy replication pair: %w", err)
	}
	return result, nil
}

func runProxySession(ctx context.Context, input ReplicationInput, procedureSHA256, role, outputPath string, capture proxyCapture) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	args := append([]string(nil), input.SharedArgs...)
	if role == portabletrace.RoleBaseline {
		args = append(args, input.BaselineArg)
	} else {
		args = append(args, input.TreatmentArg)
	}
	summary, err := capture(input.ProcedurePath, input.ProgramPath, args, outputPath)
	if err != nil {
		return fmt.Errorf("proxy %s session: %w", role, err)
	}
	if summary.ProcedureSHA256 != procedureSHA256 {
		return errors.New("proxy procedure identity changed")
	}
	if summary.Trace.Completeness == "" || !portabletrace.ValidSHA256(summary.Trace.TraceSHA256) {
		return errors.New("proxy capture trace identity is invalid")
	}
	return nil
}

func verifyProxyPair(rootDir string, record ReplicatedRunRecord, pair ReplicatedPairRecord, expectedProcedureSHA256 string) (ReplicatedPairSummary, string, error) {
	paths := replicationPaths(filepath.Join(rootDir, pair.Directory))
	for _, path := range []string{paths.baselineTrace, paths.treatmentTrace, paths.baselineSession, paths.treatmentSession} {
		info, err := lstatReplicationPath(path)
		if err != nil || !info.Mode().IsRegular() {
			return ReplicatedPairSummary{}, "", fmt.Errorf("proxy replication pair %d %s: pair file is invalid", pair.Pair, pair.Order)
		}
	}
	comparison, err := portabletrace.CompareSessionPair(paths.baselineSession, paths.baselineTrace, paths.treatmentSession, paths.treatmentTrace)
	if err != nil {
		return ReplicatedPairSummary{}, "", fmt.Errorf("proxy replication pair %d %s: %w", pair.Pair, pair.Order, err)
	}
	if comparison.Pair.Adapter != record.Adapter || comparison.Pair.AdapterVersion != record.AdapterVersion ||
		(expectedProcedureSHA256 != "" && comparison.Pair.ProcedureSHA256 != expectedProcedureSHA256) || comparison.Pair.Scope != record.Scope || comparison.Pair.Order != pair.Order ||
		comparison.Pair.BaselineTraceSHA256 != pair.BaselineTraceSHA256 || comparison.Pair.TreatmentTraceSHA256 != pair.TreatmentTraceSHA256 ||
		comparison.Pair.BaselineSessionSHA256 != pair.BaselineSessionSHA256 || comparison.Pair.TreatmentSessionSHA256 != pair.TreatmentSessionSHA256 ||
		comparison.Pair.PairSHA256 != pair.PairSHA256 {
		return ReplicatedPairSummary{}, "", errors.New("proxy replication pair provenance disagrees")
	}
	state := evidence.Observed
	if comparison.Pair.BaselineCompleteness != portabletrace.Complete || comparison.Pair.TreatmentCompleteness != portabletrace.Complete {
		state = evidence.Unknown
	}
	result := ReplicatedPairSummary{
		Pair:          pair.Pair,
		Order:         pair.Order,
		EvidenceState: state,
		Differences:   len(comparison.Comparison.Differences),
		Unknowns:      len(comparison.Comparison.Unknowns),
		PairSHA256:    comparison.Pair.PairSHA256,
	}
	if result.Unknowns > 0 {
		result.EvidenceState = evidence.Unknown
		result.Outcome = "unknown"
	} else if result.Differences > 0 {
		result.Outcome = "changed"
	} else {
		result.Outcome = "same"
	}
	return result, comparison.Pair.ProcedureSHA256, nil
}

type proxyPairClassification struct {
	Outcome       portabletrace.ReplicatedOutcome
	EvidenceState evidence.State
	ChangedPairs  int
	NoChangePairs int
	UnknownPairs  int
}

func classifyProxyPairs(pairs []ReplicatedPairSummary) proxyPairClassification {
	result := proxyPairClassification{EvidenceState: evidence.Observed}
	if len(pairs) == 0 {
		result.Outcome = portabletrace.ReplicationUnknown
		result.EvidenceState = evidence.Unknown
		return result
	}
	for _, pair := range pairs {
		if pair.EvidenceState != evidence.Observed {
			result.EvidenceState = evidence.Unknown
		}
		switch pair.Outcome {
		case "changed":
			result.ChangedPairs++
		case "same":
			result.NoChangePairs++
		default:
			result.UnknownPairs++
		}
	}
	switch {
	case result.UnknownPairs > 0:
		result.Outcome = portabletrace.ReplicationUnknown
	case result.ChangedPairs == len(pairs):
		result.Outcome = portabletrace.ReplicatedChange
	case result.NoChangePairs == len(pairs):
		result.Outcome = portabletrace.NoChangeObserved
	default:
		result.Outcome = portabletrace.MixedInconsistent
	}
	return result
}

type replicationPathsValue struct {
	baselineTrace, treatmentTrace     string
	baselineSession, treatmentSession string
}

func replicationPaths(directory string) replicationPathsValue {
	return replicationPathsValue{
		baselineTrace:    filepath.Join(directory, "baseline", "trace.json"),
		treatmentTrace:   filepath.Join(directory, "treatment", "trace.json"),
		baselineSession:  filepath.Join(directory, "baseline", "session.json"),
		treatmentSession: filepath.Join(directory, "treatment", "session.json"),
	}
}

func writeReplicationFailure(outputDir string, record ReplicatedRunRecord, pair ReplicatedPairRecord, runErr error) error {
	record.FailurePair = pair.Pair
	record.FailureOrder = pair.Order
	record.Pairs = append(record.Pairs, pair)
	record.CompletedPairs = completedReplicationPairCount(record.Pairs)
	if writeErr := writeReplicationRecord(outputDir, record); writeErr != nil {
		return errors.Join(runErr, writeErr)
	}
	return runErr
}

func completedReplicationPairCount(pairs []ReplicatedPairRecord) int {
	count := 0
	for _, pair := range pairs {
		if pair.Status == ProxyReplicationStatusComplete {
			count++
		}
	}
	return count
}

func writeReplicationRecord(rootDir string, record ReplicatedRunRecord) error {
	if err := validateReplicationRecord(rootDir, record); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return errors.New("encode proxy replication metadata")
	}
	data = append(data, '\n')
	path := filepath.Join(rootDir, "replication.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create proxy replication metadata")
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.New("write proxy replication metadata")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync proxy replication metadata")
	}
	if err := file.Close(); err != nil {
		return errors.New("close proxy replication metadata")
	}
	remove = false
	return nil
}

func readReplicationRecord(rootDir string) (ReplicatedRunRecord, []byte, error) {
	if strings.TrimSpace(rootDir) == "" {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication directory is required")
	}
	rootInfo, err := lstatReplicationPath(rootDir)
	if err != nil || !rootInfo.IsDir() {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication directory is invalid")
	}
	data, err := readReplicationFile(filepath.Join(rootDir, "replication.json"), maxProxyReplicationBytes)
	if err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication metadata is unreadable")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication metadata is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record ReplicatedRunRecord
	if err := decoder.Decode(&record); err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication metadata is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicatedRunRecord{}, nil, errors.New("proxy replication metadata has trailing data")
	}
	return record, data, nil
}

func validateReplicationRecord(rootDir string, record ReplicatedRunRecord) error {
	if record.SchemaVersion != ProxyReplicationSchemaVersion || record.Adapter != Adapter || record.AdapterVersion != AdapterVersion ||
		record.Scope != "outbound" || record.PairsPerOrder < 1 || record.PairsPerOrder > maxProxyReplicationPairs ||
		record.ResetPolicy != ProxyReplicationResetPolicy || record.ControlledArgumentPosition != "final" || record.ControlledArgumentCount != 1 ||
		!record.ConditionValuesWithheld || !portabletrace.ValidSHA256(record.ExecutionIdentitySHA256) {
		return errors.New("proxy replication metadata is invalid")
	}
	if record.Status != ProxyReplicationStatusComplete && record.Status != ProxyReplicationStatusIncomplete {
		return errors.New("proxy replication metadata status is invalid")
	}
	totalPairs := record.PairsPerOrder * 2
	if record.CompletedPairs < 0 || record.CompletedPairs > totalPairs || len(record.Pairs) == 0 || len(record.Pairs) > totalPairs {
		return errors.New("proxy replication metadata pair count is invalid")
	}
	seen := make(map[string]struct{}, len(record.Pairs))
	completeCount := 0
	failureMatches := 0
	for index, pair := range record.Pairs {
		expectedPair := index/2 + 1
		expectedOrder := portabletrace.OrderBaselineTreatment
		if index%2 == 1 {
			expectedOrder = portabletrace.OrderTreatmentBaseline
		}
		if pair.Pair != expectedPair || pair.Order != expectedOrder || pair.Directory != fmt.Sprintf("pair-%03d-%s", pair.Pair, pair.Order) ||
			pair.FirstSession != firstReplicationSession(pair.Order) || pair.SecondSession != secondReplicationSession(pair.Order) ||
			(pair.Status != ProxyReplicationStatusComplete && pair.Status != ProxyReplicationStatusIncomplete) {
			return errors.New("proxy replication pair metadata is invalid")
		}
		key := replicationPairKey(pair.Pair, pair.Order)
		if _, exists := seen[key]; exists {
			return errors.New("proxy replication pair metadata contains a duplicate")
		}
		seen[key] = struct{}{}
		pairDirectory := filepath.Join(rootDir, pair.Directory)
		info, err := lstatReplicationPath(pairDirectory)
		if pair.Status == ProxyReplicationStatusComplete || !(record.Status == ProxyReplicationStatusIncomplete && index == len(record.Pairs)-1) {
			if err != nil || !info.IsDir() {
				return errors.New("proxy replication pair directory is invalid")
			}
		} else if err == nil && !info.IsDir() {
			return errors.New("proxy replication pair directory is invalid")
		}
		if pair.Status == ProxyReplicationStatusComplete {
			completeCount++
			if !portabletrace.ValidSHA256(pair.BaselineTraceSHA256) || !portabletrace.ValidSHA256(pair.TreatmentTraceSHA256) ||
				!portabletrace.ValidSHA256(pair.BaselineSessionSHA256) || !portabletrace.ValidSHA256(pair.TreatmentSessionSHA256) || !portabletrace.ValidSHA256(pair.PairSHA256) {
				return errors.New("proxy replication pair identities are invalid")
			}
		} else if pair.BaselineTraceSHA256 != "" || pair.TreatmentTraceSHA256 != "" || pair.BaselineSessionSHA256 != "" || pair.TreatmentSessionSHA256 != "" || pair.PairSHA256 != "" {
			return errors.New("incomplete proxy replication pair has identities")
		}
		if record.Status == ProxyReplicationStatusIncomplete && pair.Pair == record.FailurePair && pair.Order == record.FailureOrder && pair.Status == ProxyReplicationStatusIncomplete {
			failureMatches++
		}
	}
	if completeCount != record.CompletedPairs {
		return errors.New("proxy replication metadata completed_pairs disagrees")
	}
	if record.Status == ProxyReplicationStatusComplete {
		if len(record.Pairs) != totalPairs || record.CompletedPairs != totalPairs || record.FailurePair != 0 || record.FailureOrder != "" {
			return errors.New("complete proxy replication metadata is incomplete")
		}
	} else if record.FailurePair < 1 || record.FailurePair > record.PairsPerOrder ||
		(record.FailureOrder != portabletrace.OrderBaselineTreatment && record.FailureOrder != portabletrace.OrderTreatmentBaseline) || failureMatches != 1 || record.Pairs[len(record.Pairs)-1].Status != ProxyReplicationStatusIncomplete {
		return errors.New("incomplete proxy replication metadata is missing its failure")
	}
	return nil
}

func firstReplicationSession(order string) string {
	if order == portabletrace.OrderTreatmentBaseline {
		return portabletrace.RoleTreatment
	}
	return portabletrace.RoleBaseline
}

func secondReplicationSession(order string) string {
	if order == portabletrace.OrderTreatmentBaseline {
		return portabletrace.RoleBaseline
	}
	return portabletrace.RoleTreatment
}

func replicationPairKey(pair int, order string) string {
	return fmt.Sprintf("%d:%s", pair, order)
}

func readReplicationFile(path string, limit int64) ([]byte, error) {
	info, err := lstatReplicationPath(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("regular file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open file")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, errors.New("file changed during verification")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("file exceeds limit")
	}
	return data, nil
}

func lstatReplicationPath(path string) (os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	separator := string(filepath.Separator)
	current := volume
	if strings.HasPrefix(remainder, separator) {
		current += separator
		remainder = strings.TrimPrefix(remainder, separator)
	}
	if remainder == "" {
		return os.Lstat(current)
	}
	var info os.FileInfo
	for _, component := range strings.Split(remainder, separator) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			return nil, errors.New("unsafe path")
		}
	}
	return info, nil
}

func hashExecutable(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func stageExecutable(path string) (string, string, func(), error) {
	if err := validateProgram(path, nil); err != nil {
		return "", "", func() {}, err
	}
	temporaryDirectory, err := os.MkdirTemp("", "ariadne-proxy-replication-")
	if err != nil {
		return "", "", func() {}, errors.New("create proxy executable staging directory")
	}
	stagedName := "program" + filepath.Ext(path)
	cleanup := func() {
		_ = os.Remove(filepath.Join(temporaryDirectory, stagedName))
		_ = os.Remove(temporaryDirectory)
	}
	stagedPath := filepath.Join(temporaryDirectory, stagedName)
	source, err := os.Open(path)
	if err != nil {
		cleanup()
		return "", "", func() {}, errors.New("open proxy executable")
	}
	defer source.Close()
	target, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		cleanup()
		return "", "", func() {}, errors.New("create staged proxy executable")
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(target, hash), source)
	if copyErr == nil {
		copyErr = target.Sync()
	}
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		cleanup()
		return "", "", func() {}, errors.New("stage proxy executable")
	}
	return stagedPath, hex.EncodeToString(hash.Sum(nil)), cleanup, nil
}

func digestReplication(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
