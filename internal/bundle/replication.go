package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	replicatedPairOutcomeChanged = "changed"
	replicatedPairOutcomeSame    = "same"
	replicatedPairOutcomeUnknown = "unknown"
)

// ReplicatedOutcome classifies the aggregate result independently of evidence.State.
type ReplicatedOutcome = portabletrace.ReplicatedOutcome

const (
	ReplicatedChange   = portabletrace.ReplicatedChange
	NoChangeObserved   = portabletrace.NoChangeObserved
	MixedInconsistent  = portabletrace.MixedInconsistent
	ReplicationUnknown = portabletrace.ReplicationUnknown
)

// ReplicatedPairSummary is the safe result for one ordered pair.
type ReplicatedPairSummary struct {
	Pair           int            `json:"pair"`
	Order          string         `json:"order"`
	Outcome        string         `json:"outcome"`
	EvidenceState  evidence.State `json:"evidence_state"`
	Differences    int            `json:"differences"`
	Unknowns       int            `json:"unknowns"`
	EvidenceSHA256 string         `json:"evidence_sha256,omitempty"`
}

// ReplicatedExperimentSummary is a raw-value-free aggregate verification result.
type ReplicatedExperimentSummary struct {
	SchemaVersion          int                     `json:"schema_version"`
	ManifestName           string                  `json:"manifest_name"`
	DeclaredVariable       string                  `json:"declared_variable"`
	ReceiptSHA256          string                  `json:"receipt_sha256"`
	Pairs                  int                     `json:"pairs"`
	PairsPerOrder          int                     `json:"pairs_per_order"`
	BaselineTreatmentPairs int                     `json:"baseline_treatment_pairs"`
	TreatmentBaselinePairs int                     `json:"treatment_baseline_pairs"`
	Outcome                ReplicatedOutcome       `json:"outcome"`
	EvidenceState          evidence.State          `json:"evidence_state"`
	CompletedPairs         int                     `json:"completed_pairs"`
	ChangedPairs           int                     `json:"changed_pairs"`
	NoChangePairs          int                     `json:"no_change_pairs"`
	UnknownPairs           int                     `json:"unknown_pairs"`
	PairSummaries          []ReplicatedPairSummary `json:"pair_summaries"`
}

// ReplicatedPairObservation is retained as the bundle-facing name for the
// source-neutral trace classification contract.
type ReplicatedPairObservation = portabletrace.ReplicatedPairObservation

// ReplicatedClassification is retained as the bundle-facing aggregate type.
type ReplicatedClassification = portabletrace.ReplicatedClassification

// ClassifyReplicatedPairs forwards to the source-neutral trace classifier.
func ClassifyReplicatedPairs(pairs []ReplicatedPairObservation) ReplicatedClassification {
	return portabletrace.ClassifyReplicatedPairs(pairs)
}

// VerifyReplicated verifies the safe replication receipt and each complete pair.
// Incomplete executions return a successful structural summary with unknown outcome.
func VerifyReplicated(rootDir string) (ReplicatedExperimentSummary, error) {
	record, receiptData, err := readReplicatedRecord(rootDir)
	if err != nil {
		return ReplicatedExperimentSummary{}, err
	}
	if err := validateReplicatedRecord(rootDir, record); err != nil {
		return ReplicatedExperimentSummary{}, err
	}

	totalPairs := record.PairsPerOrder * 2
	pairSummaries := make([]ReplicatedPairSummary, 0, totalPairs)
	byKey := make(map[string]ReplicatedPairSummary, len(record.Pairs))
	var provenance *Summary
	for _, pair := range record.Pairs {
		result := ReplicatedPairSummary{
			Pair:          pair.Pair,
			Order:         pair.Order,
			Outcome:       replicatedPairOutcomeUnknown,
			EvidenceState: evidence.Unknown,
		}
		if pair.Status == adb.ReplicationStatusComplete {
			summary, err := verifyReplicatedPair(filepath.Join(rootDir, pair.Directory), pair)
			if err != nil {
				return ReplicatedExperimentSummary{}, err
			}
			if summary.ManifestName != record.ManifestName ||
				summary.DeclaredVariable != record.DeclaredVariable {
				return ReplicatedExperimentSummary{}, errors.New("replication pair manifest metadata disagrees")
			}
			if provenance == nil {
				copy := summary
				provenance = &copy
			} else if !sameReplicatedProvenance(*provenance, summary) {
				return ReplicatedExperimentSummary{}, errors.New("replication pair provenance disagrees")
			}
			result.Differences = summary.Differences
			result.Unknowns = summary.Unknowns
			result.EvidenceSHA256 = summary.EvidenceSHA256
			result.EvidenceState = replicatedEvidenceState(summary)
			if result.Unknowns > 0 || result.EvidenceState == evidence.Unknown {
				result.Outcome = replicatedPairOutcomeUnknown
			} else if result.Differences > 0 {
				result.Outcome = replicatedPairOutcomeChanged
			} else {
				result.Outcome = replicatedPairOutcomeSame
			}
		}
		byKey[replicatedPairKey(pair.Pair, pair.Order)] = result
	}
	for pair := 1; pair <= record.PairsPerOrder; pair++ {
		for _, order := range []string{
			adb.ReplicationOrderBaselineTreatment,
			adb.ReplicationOrderTreatmentBaseline,
		} {
			key := replicatedPairKey(pair, order)
			result, ok := byKey[key]
			if !ok {
				result = ReplicatedPairSummary{
					Pair:          pair,
					Order:         order,
					Outcome:       replicatedPairOutcomeUnknown,
					EvidenceState: evidence.Unknown,
				}
			}
			pairSummaries = append(pairSummaries, result)
		}
	}

	result := ReplicatedExperimentSummary{
		SchemaVersion:          adb.ReplicatedRunSchemaVersion,
		ManifestName:           record.ManifestName,
		DeclaredVariable:       record.DeclaredVariable,
		ReceiptSHA256:          digestSHA256(receiptData),
		Pairs:                  totalPairs,
		PairsPerOrder:          record.PairsPerOrder,
		BaselineTreatmentPairs: record.PairsPerOrder,
		TreatmentBaselinePairs: record.PairsPerOrder,
		CompletedPairs:         record.CompletedPairs,
		PairSummaries:          pairSummaries,
	}
	observations := make([]ReplicatedPairObservation, 0, len(pairSummaries))
	for _, pair := range pairSummaries {
		observations = append(observations, ReplicatedPairObservation{
			Differences:   pair.Differences,
			Unknowns:      pair.Unknowns,
			EvidenceState: pair.EvidenceState,
		})
	}
	classification := ClassifyReplicatedPairs(observations)
	result.EvidenceState = classification.EvidenceState
	result.Outcome = classification.Outcome
	result.ChangedPairs = classification.ChangedPairs
	result.NoChangePairs = classification.NoChangePairs
	result.UnknownPairs = classification.UnknownPairs
	return result, nil
}

func readReplicatedRecord(rootDir string) (adb.ReplicatedRunRecord, []byte, error) {
	if strings.TrimSpace(rootDir) == "" {
		return adb.ReplicatedRunRecord{}, nil, errors.New("replicated run directory is required")
	}
	data, err := readFileBounded(filepath.Join(rootDir, "replication.json"), maxOutputBytes)
	if err != nil {
		return adb.ReplicatedRunRecord{}, nil, fmt.Errorf("replication metadata: %w", err)
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return adb.ReplicatedRunRecord{}, nil, fmt.Errorf("replication metadata: %w", err)
	}
	var record adb.ReplicatedRunRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return adb.ReplicatedRunRecord{}, nil, fmt.Errorf("replication metadata: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return adb.ReplicatedRunRecord{}, nil, errors.New("replication metadata: trailing data")
	}
	return record, data, nil
}

func validateReplicatedRecord(rootDir string, record adb.ReplicatedRunRecord) error {
	if record.SchemaVersion != adb.ReplicatedRunSchemaVersion ||
		!validMetadataValue(record.ManifestName) ||
		!validMetadataValue(record.DeclaredVariable) {
		return errors.New("replication metadata is invalid")
	}
	if record.PairsPerOrder < 1 || record.PairsPerOrder > 8 ||
		record.ResetPolicy != adb.ReplicationResetPolicy {
		return errors.New("replication metadata configuration is invalid")
	}
	switch record.Status {
	case adb.ReplicationStatusComplete, adb.ReplicationStatusIncomplete:
	default:
		return errors.New("replication metadata status is invalid")
	}
	if record.CompletedPairs < 0 || record.CompletedPairs > record.PairsPerOrder*2 {
		return errors.New("replication metadata completed_pairs is invalid")
	}
	seen := make(map[string]struct{}, len(record.Pairs))
	completeCount := 0
	failureMatches := false
	for _, pair := range record.Pairs {
		if pair.Pair < 1 || pair.Pair > record.PairsPerOrder ||
			(pair.Order != adb.ReplicationOrderBaselineTreatment &&
				pair.Order != adb.ReplicationOrderTreatmentBaseline) {
			return errors.New("replication pair metadata is invalid")
		}
		expectedDirectory := fmt.Sprintf("pair-%03d-%s", pair.Pair, pair.Order)
		if pair.Directory != expectedDirectory ||
			!validPairSessions(pair) ||
			(pair.Status != adb.ReplicationStatusComplete &&
				pair.Status != adb.ReplicationStatusIncomplete) {
			return errors.New("replication pair metadata is invalid")
		}
		key := replicatedPairKey(pair.Pair, pair.Order)
		if _, ok := seen[key]; ok {
			return errors.New("replication pair metadata contains a duplicate")
		}
		seen[key] = struct{}{}
		if pair.Status == adb.ReplicationStatusComplete {
			completeCount++
		}
		info, err := lstatNoSymlinkPath(filepath.Join(rootDir, pair.Directory))
		if err != nil || !info.IsDir() {
			return errors.New("replication pair directory is invalid")
		}
		if record.Status == adb.ReplicationStatusIncomplete &&
			pair.Pair == record.FailurePair && pair.Order == record.FailureOrder &&
			pair.Status == adb.ReplicationStatusIncomplete {
			failureMatches = true
		}
	}
	if completeCount != record.CompletedPairs {
		return errors.New("replication metadata completed_pairs disagrees")
	}
	if record.Status == adb.ReplicationStatusComplete {
		if len(record.Pairs) != record.PairsPerOrder*2 ||
			record.CompletedPairs != len(record.Pairs) ||
			record.FailurePair != 0 || record.FailureOrder != "" {
			return errors.New("complete replication metadata is incomplete")
		}
	} else {
		if record.FailurePair < 1 || record.FailurePair > record.PairsPerOrder ||
			!failureMatches {
			return errors.New("incomplete replication metadata is missing its failure")
		}
	}
	return nil
}

func validPairSessions(pair adb.ReplicatedPairRecord) bool {
	if pair.Order == adb.ReplicationOrderBaselineTreatment {
		return pair.FirstSession == "baseline" && pair.SecondSession == "treatment"
	}
	return pair.FirstSession == "treatment" && pair.SecondSession == "baseline"
}

func verifyReplicatedPair(pairDir string, pair adb.ReplicatedPairRecord) (Summary, error) {
	first, err := loadSession(pairDir, pair.FirstSession)
	if err != nil {
		return Summary{}, fmt.Errorf("replication pair %d %s: %w", pair.Pair, pair.Order, err)
	}
	second, err := loadSession(pairDir, pair.SecondSession)
	if err != nil {
		return Summary{}, fmt.Errorf("replication pair %d %s: %w", pair.Pair, pair.Order, err)
	}
	if first.record.SchemaVersion >= 8 &&
		(first.record.Order != pair.Order || second.record.Order != pair.Order) {
		return Summary{}, errors.New("replication pair authenticated order disagrees with receipt")
	}
	if !first.record.StartedAt.Before(second.record.StartedAt) ||
		second.record.StartedAt.Before(first.record.FinishedAt) {
		return Summary{}, errors.New("replication pair session order is invalid")
	}
	_, summary, _, err := verifyDocumentWithOutput(pairDir)
	if err != nil {
		return Summary{}, fmt.Errorf("replication pair %d %s: %w", pair.Pair, pair.Order, err)
	}
	return summary, nil
}

func digestSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func replicatedEvidenceState(summary Summary) evidence.State {
	if summary.Unknowns > 0 || summary.AnswerState == evidence.Unknown {
		return evidence.Unknown
	}
	if summary.AnswerState.Valid() {
		return summary.AnswerState
	}
	return evidence.Unknown
}

func aggregateEvidenceState(pairs []ReplicatedPairSummary) evidence.State {
	for _, pair := range pairs {
		if pair.EvidenceState == evidence.Unknown {
			return evidence.Unknown
		}
	}
	for _, pair := range pairs {
		if pair.EvidenceState != evidence.Observed {
			return evidence.Unknown
		}
	}
	return evidence.Observed
}

func replicatedPairKey(pair int, order string) string {
	return fmt.Sprintf("%d:%s", pair, order)
}

func sameReplicatedProvenance(left, right Summary) bool {
	return left.ManifestName == right.ManifestName &&
		left.DeclaredVariable == right.DeclaredVariable &&
		left.ManifestContractSHA256 == right.ManifestContractSHA256 &&
		left.Question == right.Question &&
		left.TargetADBVersion == right.TargetADBVersion &&
		left.TargetDevice == right.TargetDevice &&
		left.TargetPackage == right.TargetPackage &&
		left.TargetAndroidAPI == right.TargetAndroidAPI &&
		left.TargetArchitecture == right.TargetArchitecture &&
		left.TargetPackageVersionCode == right.TargetPackageVersionCode &&
		left.TargetPackageSHA256 == right.TargetPackageSHA256 &&
		left.AriadneRevision == right.AriadneRevision &&
		left.AriadneModified == right.AriadneModified
}
