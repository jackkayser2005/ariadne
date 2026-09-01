package adb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/provenance"
)

const (
	// ReplicatedRunSchemaVersion is the root replication receipt schema.
	ReplicatedRunSchemaVersion = 1
	// ReplicationOrderBaselineTreatment records baseline-first execution.
	ReplicationOrderBaselineTreatment = "baseline-treatment"
	// ReplicationOrderTreatmentBaseline records treatment-first execution.
	ReplicationOrderTreatmentBaseline = "treatment-baseline"
	// ReplicationResetPolicy records the reset applied before every session.
	ReplicationResetPolicy = "reset-before-each-session"
	// ReplicationSource is the canonical source identity for Android runs.
	ReplicationSource = "android"
	// ReplicationAdapter is the canonical adapter identity for Android runs.
	ReplicationAdapter = "android-experiment-001"
	// ReplicationAdapterVersion is the canonical Android adapter version.
	ReplicationAdapterVersion = 1
	// ReplicationScope is the fixed scope captured by the Android fixture.
	ReplicationScope = "all"
	// ReplicationStatusComplete records a successful pair or root.
	ReplicationStatusComplete = "complete"
	// ReplicationStatusIncomplete records a partial pair or root.
	ReplicationStatusIncomplete = "incomplete"
	maxReplicatedPairs          = 8
)

// ReplicatedRunRecord records the safe execution contract for a replicated run.
// It contains no persona values or captured observations.
type ReplicatedRunRecord struct {
	SchemaVersion    int                    `json:"schema_version"`
	ManifestName     string                 `json:"manifest_name"`
	DeclaredVariable string                 `json:"declared_variable"`
	PairsPerOrder    int                    `json:"pairs_per_order"`
	ResetPolicy      string                 `json:"reset_policy"`
	ProvenanceSHA256 string                 `json:"provenance_sha256,omitempty"`
	Status           string                 `json:"status"`
	CompletedPairs   int                    `json:"completed_pairs"`
	FailurePair      int                    `json:"failure_pair,omitempty"`
	FailureOrder     string                 `json:"failure_order,omitempty"`
	Pairs            []ReplicatedPairRecord `json:"pairs"`
}

// ReplicationProvenance returns the canonical adapter boundary for an
// authenticated Android replication run.
func ReplicationProvenance(manifestContractSHA256 string) (provenance.Contract, error) {
	contract := provenance.Contract{
		SchemaVersion:   provenance.SchemaVersion,
		Source:          ReplicationSource,
		Adapter:         ReplicationAdapter,
		AdapterVersion:  ReplicationAdapterVersion,
		ProcedureSHA256: manifestContractSHA256,
		Scope:           ReplicationScope,
	}
	if err := contract.Validate(); err != nil {
		return provenance.Contract{}, err
	}
	return contract, nil
}

// ReplicationProvenanceSHA256 returns the canonical adapter-boundary identity
// for an authenticated Android replication run.
func ReplicationProvenanceSHA256(manifestContractSHA256 string) (string, error) {
	contract, err := ReplicationProvenance(manifestContractSHA256)
	if err != nil {
		return "", err
	}
	return contract.SHA256()
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

// RunReplicated executes matched pairs in both orders, resetting before each session.
func RunReplicated(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	pairs int,
) error {
	return runReplicatedWithAuthenticated(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		pairs,
		runCommand,
		runInputCommand,
		newChallenge,
		time.Now,
	)
}

func runReplicatedWith(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	pairs int,
	run commandRunner,
	now func() time.Time,
) error {
	return runReplicatedWithMode(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		pairs,
		run,
		nil,
		now,
	)
}

func runReplicatedWithAuthenticated(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	pairs int,
	run commandRunner,
	writeInput inputCommandRunner,
	challenge challengeGenerator,
	now func() time.Time,
) error {
	return runReplicatedWithMode(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		pairs,
		run,
		&sessionAuthDependencies{
			writeInput: writeInput,
			challenge:  challenge,
		},
		now,
	)
}
func runReplicatedWithMode(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	pairs int,
	run commandRunner,
	authDependencies *sessionAuthDependencies,
	now func() time.Time,
) error {
	if pairs < 1 || pairs > maxReplicatedPairs {
		return fmt.Errorf("pairs must be between 1 and %d", maxReplicatedPairs)
	}
	if strings.TrimSpace(outputDir) == "" {
		return errors.New("output directory is required")
	}
	if err := validatePairConfig(
		binary,
		target,
		manifest,
		filepath.Join(outputDir, "pair-001-baseline-treatment"),
		orderedSessions(ReplicationOrderBaselineTreatment, manifest),
	); err != nil {
		return err
	}
	provenanceSHA256 := ""
	if authDependencies != nil {
		var err error
		provenanceSHA256, err = ReplicationProvenanceSHA256(manifest.ContractDigest())
		if err != nil {
			return fmt.Errorf("replication provenance: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	record := ReplicatedRunRecord{
		SchemaVersion:    ReplicatedRunSchemaVersion,
		ManifestName:     manifest.Name,
		DeclaredVariable: manifest.Variable,
		PairsPerOrder:    pairs,
		ResetPolicy:      ReplicationResetPolicy,
		ProvenanceSHA256: provenanceSHA256,
		Status:           ReplicationStatusIncomplete,
		Pairs:            make([]ReplicatedPairRecord, 0, pairs*2),
	}
	orders := []struct {
		name          string
		firstSession  string
		secondSession string
	}{
		{
			name:          ReplicationOrderBaselineTreatment,
			firstSession:  "baseline",
			secondSession: "treatment",
		},
		{
			name:          ReplicationOrderTreatmentBaseline,
			firstSession:  "treatment",
			secondSession: "baseline",
		},
	}
	for pair := 1; pair <= pairs; pair++ {
		for _, order := range orders {
			directory := fmt.Sprintf("pair-%03d-%s", pair, order.name)
			pairRecord := ReplicatedPairRecord{
				Pair:          pair,
				Order:         order.name,
				Directory:     directory,
				FirstSession:  order.firstSession,
				SecondSession: order.secondSession,
				Status:        ReplicationStatusIncomplete,
			}
			var pairAuth *sessionAuthDependencies
			if authDependencies != nil {
				dependencies := *authDependencies
				dependencies.order = order.name
				pairAuth = &dependencies
			}
			var err error
			if pairAuth == nil {
				err = runPairWithOrder(
					ctx,
					binary,
					target,
					manifest,
					filepath.Join(outputDir, directory),
					orderedSessions(order.name, manifest),
					run,
					now,
				)
			} else {
				err = runPairWithOrderAndAuth(
					ctx,
					binary,
					target,
					manifest,
					filepath.Join(outputDir, directory),
					orderedSessions(order.name, manifest),
					run,
					pairAuth,
					now,
				)
			}
			if err != nil {
				record.FailurePair = pair
				record.FailureOrder = order.name
				record.Pairs = append(record.Pairs, pairRecord)
				record.CompletedPairs = completedPairCount(record.Pairs)
				if writeErr := writeReplicatedRecord(outputDir, record); writeErr != nil {
					return errors.Join(err, writeErr)
				}
				return err
			}
			pairRecord.Status = ReplicationStatusComplete
			record.Pairs = append(record.Pairs, pairRecord)
			record.CompletedPairs = completedPairCount(record.Pairs)
		}
	}
	record.Status = ReplicationStatusComplete
	if err := writeReplicatedRecord(outputDir, record); err != nil {
		return err
	}
	return nil
}

func orderedSessions(order string, manifest experiment.Manifest) []sessionSpec {
	if order == ReplicationOrderTreatmentBaseline {
		return []sessionSpec{
			{kind: "treatment", persona: manifest.Treatment},
			{kind: "baseline", persona: manifest.Baseline},
		}
	}
	return []sessionSpec{
		{kind: "baseline", persona: manifest.Baseline},
		{kind: "treatment", persona: manifest.Treatment},
	}
}

func completedPairCount(pairs []ReplicatedPairRecord) int {
	count := 0
	for _, pair := range pairs {
		if pair.Status == ReplicationStatusComplete {
			count++
		}
	}
	return count
}

func writeReplicatedRecord(outputDir string, record ReplicatedRunRecord) error {
	if !validReplicationMetadata(record.ManifestName) ||
		!validReplicationMetadata(record.DeclaredVariable) ||
		(record.ProvenanceSHA256 != "" && !validSHA256(record.ProvenanceSHA256)) {
		return errors.New("replication metadata is invalid")
	}
	data, _ := json.MarshalIndent(record, "", "  ")
	data = append(data, '\n')
	path := filepath.Join(outputDir, "replication.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create replication metadata: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write replication metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync replication metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close replication metadata: %w", err)
	}
	remove = false
	return nil
}

func validReplicationMetadata(value string) bool {
	return value != "" &&
		len(value) <= 1024 &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
