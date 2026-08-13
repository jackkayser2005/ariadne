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
	Status           string                 `json:"status"`
	CompletedPairs   int                    `json:"completed_pairs"`
	FailurePair      int                    `json:"failure_pair,omitempty"`
	FailureOrder     string                 `json:"failure_order,omitempty"`
	Pairs            []ReplicatedPairRecord `json:"pairs"`
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
	return runReplicatedWith(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		pairs,
		runCommand,
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
			err := runPairWithOrder(
				ctx,
				binary,
				target,
				manifest,
				filepath.Join(outputDir, directory),
				orderedSessions(order.name, manifest),
				run,
				now,
			)
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
		!validReplicationMetadata(record.DeclaredVariable) {
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
