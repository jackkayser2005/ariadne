package browser

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
	// BrowserReplicationSchemaVersion is the browser fixture receipt schema.
	BrowserReplicationSchemaVersion = 2
	// BrowserReplicationLegacySchemaVersion remains readable for existing
	// fixture receipts that did not bind an explicit candidate.
	BrowserReplicationLegacySchemaVersion = 1
	// BrowserReplicationAdapter identifies the fixed local browser fixture.
	BrowserReplicationAdapter = "browser-local-fixture"
	// BrowserReplicationAdapterVersion identifies the fixture adapter contract.
	BrowserReplicationAdapterVersion = 2
	// BrowserReplicationLegacyAdapterVersion identifies the original fixture
	// contract, which remains verifiable for existing receipts.
	BrowserReplicationLegacyAdapterVersion = 1
	// BrowserReplicationResetPolicy records the isolation applied to every session.
	BrowserReplicationResetPolicy = "fresh-ephemeral-profile-before-each-session"
	// BrowserReplicationStatusComplete records a completed receipt or pair.
	BrowserReplicationStatusComplete = "complete"
	// BrowserReplicationStatusIncomplete records an interrupted receipt or pair.
	BrowserReplicationStatusIncomplete = "incomplete"
	maxBrowserReplicationPairs         = 8
	maxBrowserReplicationBytes         = 64 << 10
)

const (
	// BrowserFixtureReferenceCandidate is the fully disclosed fixture input.
	BrowserFixtureReferenceCandidate = "reference"
	// BrowserFixtureOmittedCandidate is the first reduced fixture input.
	BrowserFixtureOmittedCandidate = "omitted"
)

// FixtureReplicationInput describes one fixed, local browser counterfactual.
// The procedure and driver are explicitly selected; the runner supplies the
// baseline/treatment variant to the fixed fixture driver itself.
type FixtureReplicationInput struct {
	ProcedurePath string
	DriverPath    string
	DriverArgs    []string
	OutputDir     string
	Pairs         int
	// CandidateID binds an explicit treatment candidate to every child pair.
	// Empty preserves the legacy role-based fixture behavior.
	CandidateID string
}

// ReplicatedRunRecord records safe execution metadata without captured values.
type ReplicatedRunRecord struct {
	SchemaVersion   int                    `json:"schema_version"`
	Adapter         string                 `json:"adapter"`
	AdapterVersion  int                    `json:"adapter_version"`
	ProcedureSHA256 string                 `json:"procedure_sha256"`
	Scope           string                 `json:"scope"`
	CandidateID     string                 `json:"candidate_id,omitempty"`
	PairsPerOrder   int                    `json:"pairs_per_order"`
	ResetPolicy     string                 `json:"reset_policy"`
	Status          string                 `json:"status"`
	CompletedPairs  int                    `json:"completed_pairs"`
	FailurePair     int                    `json:"failure_pair,omitempty"`
	FailureOrder    string                 `json:"failure_order,omitempty"`
	Pairs           []ReplicatedPairRecord `json:"pairs"`
}

// ReplicatedPairRecord identifies one matched pair and its execution order.
type ReplicatedPairRecord struct {
	Pair          int    `json:"pair"`
	Order         string `json:"order"`
	Directory     string `json:"directory"`
	FirstSession  string `json:"first_session"`
	SecondSession string `json:"second_session"`
	Status        string `json:"status"`
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

// BrowserReplicationSummary is a raw-value-free aggregate verification result.
type BrowserReplicationSummary struct {
	SchemaVersion   int                             `json:"schema_version"`
	Adapter         string                          `json:"adapter"`
	AdapterVersion  int                             `json:"adapter_version"`
	ProcedureSHA256 string                          `json:"procedure_sha256"`
	Scope           string                          `json:"scope"`
	CandidateID     string                          `json:"candidate_id,omitempty"`
	ResetPolicy     string                          `json:"reset_policy"`
	ReceiptSHA256   string                          `json:"receipt_sha256"`
	Pairs           int                             `json:"pairs"`
	PairsPerOrder   int                             `json:"pairs_per_order"`
	BaselinePairs   int                             `json:"baseline_treatment_pairs"`
	TreatmentPairs  int                             `json:"treatment_baseline_pairs"`
	Outcome         portabletrace.ReplicatedOutcome `json:"outcome"`
	EvidenceState   evidence.State                  `json:"evidence_state"`
	CompletedPairs  int                             `json:"completed_pairs"`
	ChangedPairs    int                             `json:"changed_pairs"`
	NoChangePairs   int                             `json:"no_change_pairs"`
	UnknownPairs    int                             `json:"unknown_pairs"`
	PairSummaries   []ReplicatedPairSummary         `json:"pair_summaries"`
}

type fixtureCapture func(string, string, []string, string) (CaptureSummary, error)

// RunFixtureReplicated runs matched baseline/treatment pairs in both orders.
// Each Capture call creates a fresh browser profile, and the receipt records
// the observed order separately from the aggregate outcome.
func RunFixtureReplicated(ctx context.Context, input FixtureReplicationInput) error {
	return runFixtureReplicatedWith(ctx, input, Capture)
}

func runFixtureReplicatedWith(ctx context.Context, input FixtureReplicationInput, capture fixtureCapture) error {
	procedure, procedureSHA256, err := validateFixtureReplicationInput(input)
	if err != nil {
		return err
	}
	if capture == nil {
		return errors.New("browser fixture capture is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(input.OutputDir), 0o700); err != nil {
		return errors.New("create browser replication parent")
	}
	if err := os.Mkdir(input.OutputDir, 0o700); err != nil {
		return errors.New("create browser replication output")
	}

	record := ReplicatedRunRecord{
		SchemaVersion:   BrowserReplicationSchemaVersion,
		Adapter:         BrowserReplicationAdapter,
		AdapterVersion:  BrowserReplicationAdapterVersion,
		ProcedureSHA256: procedureSHA256,
		Scope:           procedure.Scope,
		CandidateID:     input.CandidateID,
		PairsPerOrder:   input.Pairs,
		ResetPolicy:     BrowserReplicationResetPolicy,
		Status:          BrowserReplicationStatusIncomplete,
		Pairs:           make([]ReplicatedPairRecord, 0, input.Pairs*2),
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
			if err := os.Mkdir(filepath.Join(input.OutputDir, directory), 0o700); err != nil {
				return writeFixtureReplicationFailure(input.OutputDir, record, pair, order.name, errors.New("create browser replication pair"))
			}
			if err := ctx.Err(); err != nil {
				return writeFixtureReplicationFailure(input.OutputDir, record, pair, order.name, err)
			}
			pairRecord := ReplicatedPairRecord{
				Pair:          pair,
				Order:         order.name,
				Directory:     directory,
				FirstSession:  order.firstSession,
				SecondSession: order.secondSession,
				Status:        BrowserReplicationStatusIncomplete,
			}
			if err := runFixturePair(ctx, input, procedureSHA256, directory, order.name, capture); err != nil {
				record.FailurePair = pair
				record.FailureOrder = order.name
				record.Pairs = append(record.Pairs, pairRecord)
				record.CompletedPairs = completedBrowserPairCount(record.Pairs)
				if writeErr := writeFixtureReplicationRecord(input.OutputDir, record); writeErr != nil {
					return errors.Join(err, writeErr)
				}
				return err
			}
			pairRecord.Status = BrowserReplicationStatusComplete
			record.Pairs = append(record.Pairs, pairRecord)
			record.CompletedPairs = completedBrowserPairCount(record.Pairs)
		}
	}
	record.Status = BrowserReplicationStatusComplete
	if err := writeFixtureReplicationRecord(input.OutputDir, record); err != nil {
		return err
	}
	return nil
}

// VerifyFixtureReplicated verifies a browser fixture receipt and its bound pairs.
func VerifyFixtureReplicated(rootDir string) (BrowserReplicationSummary, error) {
	return verifyFixtureReplicatedWithFields(rootDir, nil)
}

func verifyFixtureReplicatedWithFields(rootDir string, ignoredFields map[string]struct{}) (BrowserReplicationSummary, error) {
	record, receiptData, err := readFixtureReplicationRecord(rootDir)
	if err != nil {
		return BrowserReplicationSummary{}, err
	}
	if err := validateFixtureReplicationRecord(rootDir, record); err != nil {
		return BrowserReplicationSummary{}, err
	}

	totalPairs := record.PairsPerOrder * 2
	pairSummaries := make([]ReplicatedPairSummary, 0, totalPairs)
	byKey := make(map[string]ReplicatedPairSummary, len(record.Pairs))
	for _, pair := range record.Pairs {
		result := ReplicatedPairSummary{
			Pair:          pair.Pair,
			Order:         pair.Order,
			Outcome:       "unknown",
			EvidenceState: evidence.Unknown,
		}
		if pair.Status == BrowserReplicationStatusComplete {
			result, err = verifyFixturePairWithFields(rootDir, record, pair, ignoredFields)
			if err != nil {
				return BrowserReplicationSummary{}, err
			}
		}
		byKey[browserReplicationPairKey(pair.Pair, pair.Order)] = result
	}
	for pair := 1; pair <= record.PairsPerOrder; pair++ {
		for _, order := range []string{portabletrace.OrderBaselineTreatment, portabletrace.OrderTreatmentBaseline} {
			result, ok := byKey[browserReplicationPairKey(pair, order)]
			if !ok {
				result = ReplicatedPairSummary{Pair: pair, Order: order, Outcome: "unknown", EvidenceState: evidence.Unknown}
			}
			pairSummaries = append(pairSummaries, result)
		}
	}

	observations := make([]portabletrace.ReplicatedPairObservation, 0, len(pairSummaries))
	for _, pair := range pairSummaries {
		observations = append(observations, portabletrace.ReplicatedPairObservation{
			Differences:   pair.Differences,
			Unknowns:      pair.Unknowns,
			EvidenceState: pair.EvidenceState,
		})
	}
	classification := portabletrace.ClassifyReplicatedPairs(observations)
	return BrowserReplicationSummary{
		SchemaVersion:   record.SchemaVersion,
		Adapter:         record.Adapter,
		AdapterVersion:  record.AdapterVersion,
		ProcedureSHA256: record.ProcedureSHA256,
		Scope:           record.Scope,
		CandidateID:     record.CandidateID,
		ResetPolicy:     record.ResetPolicy,
		ReceiptSHA256:   digestBrowserReplication(receiptData),
		Pairs:           totalPairs,
		PairsPerOrder:   record.PairsPerOrder,
		BaselinePairs:   record.PairsPerOrder,
		TreatmentPairs:  record.PairsPerOrder,
		Outcome:         classification.Outcome,
		EvidenceState:   classification.EvidenceState,
		CompletedPairs:  record.CompletedPairs,
		ChangedPairs:    classification.ChangedPairs,
		NoChangePairs:   classification.NoChangePairs,
		UnknownPairs:    classification.UnknownPairs,
		PairSummaries:   pairSummaries,
	}, nil
}

func validateFixtureReplicationInput(input FixtureReplicationInput) (Procedure, string, error) {
	if strings.TrimSpace(input.ProcedurePath) == "" || strings.TrimSpace(input.DriverPath) == "" || strings.TrimSpace(input.OutputDir) == "" {
		return Procedure{}, "", errors.New("browser fixture replication paths and driver are required")
	}
	if input.Pairs < 1 || input.Pairs > maxBrowserReplicationPairs {
		return Procedure{}, "", fmt.Errorf("pairs must be between 1 and %d", maxBrowserReplicationPairs)
	}
	for _, argument := range input.DriverArgs {
		if argument == "--fixture-variant" || strings.HasPrefix(argument, "--fixture-variant=") {
			return Procedure{}, "", errors.New("fixture variant is controlled by the replication runner")
		}
		if argument == "--fixture-candidate" || strings.HasPrefix(argument, "--fixture-candidate=") {
			return Procedure{}, "", errors.New("fixture candidate is controlled by the replication runner")
		}
	}
	if input.CandidateID != "" && !validFixtureCandidate(input.CandidateID) {
		return Procedure{}, "", errors.New("fixture candidate is invalid")
	}
	procedure, _, err := ReadProcedure(input.ProcedurePath)
	if err != nil {
		return Procedure{}, "", fmt.Errorf("browser fixture procedure: %w", err)
	}
	if procedure.ProcedureID != BrowserLocalFixtureProcedureID || procedure.Scope != "outbound" {
		return Procedure{}, "", errors.New("browser fixture procedure is not supported")
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		return Procedure{}, "", errors.New("browser fixture procedure identity failed")
	}
	return procedure, procedureSHA256, nil
}

func runFixturePair(ctx context.Context, input FixtureReplicationInput, procedureSHA256, directory, order string, capture fixtureCapture) error {
	paths := browserReplicationPaths(filepath.Join(input.OutputDir, directory))
	if order == portabletrace.OrderBaselineTreatment {
		if err := runFixtureSession(ctx, input, procedureSHA256, "baseline", paths.baselineTrace, capture); err != nil {
			return err
		}
		if err := runFixtureSession(ctx, input, procedureSHA256, "treatment", paths.treatmentTrace, capture); err != nil {
			return err
		}
	} else {
		if err := runFixtureSession(ctx, input, procedureSHA256, "treatment", paths.treatmentTrace, capture); err != nil {
			return err
		}
		if err := runFixtureSession(ctx, input, procedureSHA256, "baseline", paths.baselineTrace, capture); err != nil {
			return err
		}
	}
	if _, err := portabletrace.SaveSessionPair(
		paths.baselineTrace,
		paths.treatmentTrace,
		paths.baselineSession,
		paths.treatmentSession,
		portabletrace.SessionPairInput{
			Adapter:         BrowserReplicationAdapter,
			AdapterVersion:  BrowserReplicationAdapterVersion,
			ProcedureSHA256: procedureSHA256,
			Scope:           "outbound",
			Order:           order,
			CandidateID:     input.CandidateID,
		},
	); err != nil {
		return fmt.Errorf("browser fixture pair: %w", err)
	}
	return nil
}

func runFixtureSession(ctx context.Context, input FixtureReplicationInput, procedureSHA256, variant, outputPath string, capture fixtureCapture) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	args := append([]string(nil), input.DriverArgs...)
	args = append(args, "--fixture-variant", variant)
	if input.CandidateID != "" {
		candidate := BrowserFixtureReferenceCandidate
		if variant == "treatment" {
			candidate = input.CandidateID
		}
		args = append(args, "--fixture-candidate", candidate)
	}
	summary, err := capture(input.ProcedurePath, input.DriverPath, args, outputPath)
	if err != nil {
		return fmt.Errorf("browser fixture %s session: %w", variant, err)
	}
	if summary.ProcedureSHA256 != procedureSHA256 {
		return errors.New("browser fixture procedure identity changed")
	}
	return nil
}

func verifyFixturePairWithFields(rootDir string, record ReplicatedRunRecord, pair ReplicatedPairRecord, ignoredFields map[string]struct{}) (ReplicatedPairSummary, error) {
	paths := browserReplicationPaths(filepath.Join(rootDir, pair.Directory))
	var comparison portabletrace.SessionPairComparison
	var err error
	if len(ignoredFields) == 0 {
		comparison, err = portabletrace.CompareSessionPairWithCandidate(paths.baselineSession, paths.baselineTrace, paths.treatmentSession, paths.treatmentTrace, record.CandidateID)
	} else {
		comparison, err = compareFixtureSessionPairIgnoringFields(paths, ignoredFields, record.CandidateID)
	}
	if err != nil {
		return ReplicatedPairSummary{}, fmt.Errorf("browser replication pair %d %s: %w", pair.Pair, pair.Order, err)
	}
	if comparison.Pair.Adapter != record.Adapter || comparison.Pair.AdapterVersion != record.AdapterVersion ||
		comparison.Pair.ProcedureSHA256 != record.ProcedureSHA256 || comparison.Pair.Scope != record.Scope ||
		comparison.Pair.Order != pair.Order {
		return ReplicatedPairSummary{}, errors.New("browser replication pair provenance disagrees")
	}
	state := evidence.Observed
	if comparison.Comparison.BaselineCompleteness != portabletrace.Complete ||
		comparison.Comparison.TreatmentCompleteness != portabletrace.Complete ||
		len(comparison.Comparison.Unknowns) > 0 {
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
	switch {
	case state == evidence.Unknown || result.Unknowns > 0:
		result.Outcome = "unknown"
	case result.Differences > 0:
		result.Outcome = "changed"
	default:
		result.Outcome = "same"
	}
	return result, nil
}

type browserReplicationPathsValue struct {
	baselineTrace, treatmentTrace     string
	baselineSession, treatmentSession string
}

func browserReplicationPaths(directory string) browserReplicationPathsValue {
	return browserReplicationPathsValue{
		baselineTrace:    filepath.Join(directory, "baseline", "trace.json"),
		treatmentTrace:   filepath.Join(directory, "treatment", "trace.json"),
		baselineSession:  filepath.Join(directory, "baseline", "session.json"),
		treatmentSession: filepath.Join(directory, "treatment", "session.json"),
	}
}

func writeFixtureReplicationFailure(outputDir string, record ReplicatedRunRecord, pair int, order string, runErr error) error {
	record.FailurePair = pair
	record.FailureOrder = order
	record.Pairs = append(record.Pairs, ReplicatedPairRecord{
		Pair:          pair,
		Order:         order,
		Directory:     fmt.Sprintf("pair-%03d-%s", pair, order),
		FirstSession:  firstBrowserReplicationSession(order),
		SecondSession: secondBrowserReplicationSession(order),
		Status:        BrowserReplicationStatusIncomplete,
	})
	record.CompletedPairs = completedBrowserPairCount(record.Pairs)
	if writeErr := writeFixtureReplicationRecord(outputDir, record); writeErr != nil {
		return errors.Join(runErr, writeErr)
	}
	return runErr
}

func firstBrowserReplicationSession(order string) string {
	if order == portabletrace.OrderTreatmentBaseline {
		return portabletrace.RoleTreatment
	}
	return portabletrace.RoleBaseline
}

func secondBrowserReplicationSession(order string) string {
	if order == portabletrace.OrderTreatmentBaseline {
		return portabletrace.RoleBaseline
	}
	return portabletrace.RoleTreatment
}

func completedBrowserPairCount(pairs []ReplicatedPairRecord) int {
	count := 0
	for _, pair := range pairs {
		if pair.Status == BrowserReplicationStatusComplete {
			count++
		}
	}
	return count
}

func writeFixtureReplicationRecord(rootDir string, record ReplicatedRunRecord) error {
	if err := validateFixtureReplicationRecord(rootDir, record); err != nil {
		return err
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return errors.New("encode browser replication metadata")
	}
	data = append(data, '\n')
	file, err := os.OpenFile(filepath.Join(rootDir, "replication.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create browser replication metadata")
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(filepath.Join(rootDir, "replication.json"))
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.New("write browser replication metadata")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync browser replication metadata")
	}
	if err := file.Close(); err != nil {
		return errors.New("close browser replication metadata")
	}
	remove = false
	return nil
}

func readFixtureReplicationRecord(rootDir string) (ReplicatedRunRecord, []byte, error) {
	if strings.TrimSpace(rootDir) == "" {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication directory is required")
	}
	rootInfo, err := lstatBrowserReplicationPath(rootDir)
	if err != nil || !rootInfo.IsDir() {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication directory is invalid")
	}
	data, err := readBrowserReplicationFile(filepath.Join(rootDir, "replication.json"), maxBrowserReplicationBytes)
	if err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication metadata is unreadable")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication metadata is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record ReplicatedRunRecord
	if err := decoder.Decode(&record); err != nil {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication metadata is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ReplicatedRunRecord{}, nil, errors.New("browser replication metadata has trailing data")
	}
	return record, data, nil
}

func validateFixtureReplicationRecord(rootDir string, record ReplicatedRunRecord) error {
	legacy := record.SchemaVersion == BrowserReplicationLegacySchemaVersion && record.AdapterVersion == BrowserReplicationLegacyAdapterVersion && record.CandidateID == ""
	current := record.SchemaVersion == BrowserReplicationSchemaVersion && record.AdapterVersion == BrowserReplicationAdapterVersion
	if record.Adapter != BrowserReplicationAdapter || (!legacy && !current) ||
		!portabletrace.ValidSHA256(record.ProcedureSHA256) || record.Scope != "outbound" ||
		record.PairsPerOrder < 1 || record.PairsPerOrder > maxBrowserReplicationPairs ||
		record.ResetPolicy != BrowserReplicationResetPolicy {
		return errors.New("browser replication metadata is invalid")
	}
	if record.CandidateID != "" && !validFixtureCandidate(record.CandidateID) {
		return errors.New("browser replication candidate is invalid")
	}
	if record.Status != BrowserReplicationStatusComplete && record.Status != BrowserReplicationStatusIncomplete {
		return errors.New("browser replication metadata status is invalid")
	}
	if record.CompletedPairs < 0 || record.CompletedPairs > record.PairsPerOrder*2 {
		return errors.New("browser replication metadata completed_pairs is invalid")
	}
	seen := make(map[string]struct{}, len(record.Pairs))
	completeCount := 0
	failureMatches := false
	for _, pair := range record.Pairs {
		if pair.Pair < 1 || pair.Pair > record.PairsPerOrder ||
			(pair.Order != portabletrace.OrderBaselineTreatment && pair.Order != portabletrace.OrderTreatmentBaseline) ||
			pair.Directory != fmt.Sprintf("pair-%03d-%s", pair.Pair, pair.Order) ||
			pair.FirstSession != firstBrowserReplicationSession(pair.Order) ||
			pair.SecondSession != secondBrowserReplicationSession(pair.Order) ||
			(pair.Status != BrowserReplicationStatusComplete && pair.Status != BrowserReplicationStatusIncomplete) {
			return errors.New("browser replication pair metadata is invalid")
		}
		key := browserReplicationPairKey(pair.Pair, pair.Order)
		if _, ok := seen[key]; ok {
			return errors.New("browser replication pair metadata contains a duplicate")
		}
		seen[key] = struct{}{}
		if pair.Status == BrowserReplicationStatusComplete {
			completeCount++
		}
		info, err := lstatBrowserReplicationPath(filepath.Join(rootDir, pair.Directory))
		if err != nil || !info.IsDir() {
			return errors.New("browser replication pair directory is invalid")
		}
		if record.Status == BrowserReplicationStatusIncomplete && pair.Pair == record.FailurePair && pair.Order == record.FailureOrder && pair.Status == BrowserReplicationStatusIncomplete {
			failureMatches = true
		}
	}
	if completeCount != record.CompletedPairs {
		return errors.New("browser replication metadata completed_pairs disagrees")
	}
	if record.Status == BrowserReplicationStatusComplete {
		if len(record.Pairs) != record.PairsPerOrder*2 || record.CompletedPairs != len(record.Pairs) || record.FailurePair != 0 || record.FailureOrder != "" {
			return errors.New("complete browser replication metadata is incomplete")
		}
	} else if record.FailurePair < 1 || record.FailurePair > record.PairsPerOrder || !failureMatches {
		return errors.New("incomplete browser replication metadata is missing its failure")
	}
	return nil
}

func browserReplicationPairKey(pair int, order string) string {
	return fmt.Sprintf("%d:%s", pair, order)
}

func readBrowserReplicationFile(path string, limit int64) ([]byte, error) {
	info, err := lstatBrowserReplicationPath(path)
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

func lstatBrowserReplicationPath(path string) (os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimPrefix(absolute, volume)
	current := volume
	separator := string(filepath.Separator)
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

func digestBrowserReplication(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
