package adb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/provenance"
	"github.com/jackkayser2005/ariadne/internal/securefs"
)

const (
	// ReplicatedRunSchemaVersion is the root replication receipt schema.
	ReplicatedRunSchemaVersion = 1
	// LegacyAuthenticatedReplicatedRunSchemaVersion is the authenticated root schema
	// that aliases the manifest digest as its procedure identity.
	LegacyAuthenticatedReplicatedRunSchemaVersion = 2
	// AuthenticatedReplicatedRunSchemaVersion is the current independently procedure-bound root schema.
	AuthenticatedReplicatedRunSchemaVersion = 3
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
	SchemaVersion          int                    `json:"schema_version"`
	ManifestName           string                 `json:"manifest_name"`
	DeclaredVariable       string                 `json:"declared_variable"`
	ManifestContractSHA256 string                 `json:"manifest_contract_sha256,omitempty"`
	ProcedureSHA256        string                 `json:"procedure_sha256,omitempty"`
	PairsPerOrder          int                    `json:"pairs_per_order"`
	ResetPolicy            string                 `json:"reset_policy"`
	ProvenanceSHA256       string                 `json:"provenance_sha256,omitempty"`
	BindingSHA256          string                 `json:"binding_sha256,omitempty"`
	Status                 string                 `json:"status"`
	CompletedPairs         int                    `json:"completed_pairs"`
	FailurePair            int                    `json:"failure_pair,omitempty"`
	FailureOrder           string                 `json:"failure_order,omitempty"`
	Pairs                  []ReplicatedPairRecord `json:"pairs"`
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

// ReplicationProvenanceWithProcedure returns Android provenance with the
// manifest contract and reviewed execution procedure represented separately.
func ReplicationProvenanceWithProcedure(manifestContractSHA256, procedureSHA256 string) (provenance.Contract, error) {
	contract := provenance.Contract{
		SchemaVersion:          provenance.SchemaVersion,
		Source:                 ReplicationSource,
		Adapter:                ReplicationAdapter,
		AdapterVersion:         ReplicationAdapterVersion,
		ProcedureSHA256:        procedureSHA256,
		ManifestContractSHA256: manifestContractSHA256,
		Scope:                  ReplicationScope,
	}
	if err := contract.Validate(); err != nil {
		return provenance.Contract{}, err
	}
	return contract, nil
}

// ReplicationProvenanceSHA256WithProcedure returns the independently
// manifest- and procedure-bound Android provenance identity.
func ReplicationProvenanceSHA256WithProcedure(manifestContractSHA256, procedureSHA256 string) (string, error) {
	contract, err := ReplicationProvenanceWithProcedure(manifestContractSHA256, procedureSHA256)
	if err != nil {
		return "", err
	}
	return contract.SHA256()
}

// ReplicatedPairRecord identifies one matched pair and its execution order.
type ReplicatedPairRecord struct {
	Pair                       int    `json:"pair"`
	Order                      string `json:"order"`
	Directory                  string `json:"directory"`
	FirstSession               string `json:"first_session"`
	SecondSession              string `json:"second_session"`
	FirstSessionBindingSHA256  string `json:"first_session_binding_sha256,omitempty"`
	SecondSessionBindingSHA256 string `json:"second_session_binding_sha256,omitempty"`
	BindingSHA256              string `json:"binding_sha256,omitempty"`
	Status                     string `json:"status"`
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
	recordSchemaVersion := ReplicatedRunSchemaVersion
	manifestContractSHA256 := ""
	procedureSHA256 := ""
	provenanceSHA256 := ""
	var err error
	if authDependencies != nil {
		recordSchemaVersion = AuthenticatedReplicatedRunSchemaVersion
		manifestContractSHA256 = manifest.ContractDigest()
		procedureSHA256, err = AndroidProcedureSHA256()
		if err != nil {
			return fmt.Errorf("replication procedure: %w", err)
		}
		provenanceSHA256, err = ReplicationProvenanceSHA256WithProcedure(manifestContractSHA256, procedureSHA256)
		if err != nil {
			return fmt.Errorf("replication provenance: %w", err)
		}
	}
	if err := securefs.MkdirAll(filepath.Dir(outputDir), 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	if err := securefs.MkdirExclusive(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	record := ReplicatedRunRecord{
		SchemaVersion:          recordSchemaVersion,
		ManifestName:           manifest.Name,
		DeclaredVariable:       manifest.Variable,
		ManifestContractSHA256: manifestContractSHA256,
		ProcedureSHA256:        procedureSHA256,
		PairsPerOrder:          pairs,
		ResetPolicy:            ReplicationResetPolicy,
		ProvenanceSHA256:       provenanceSHA256,
		Status:                 ReplicationStatusIncomplete,
		Pairs:                  make([]ReplicatedPairRecord, 0, pairs*2),
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
			if err == nil && authDependencies != nil {
				pairRecord, err = bindReplicatedPair(outputDir, record, pairRecord)
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
	if authDependencies != nil {
		var err error
		record.BindingSHA256, err = ReplicatedBindingSHA256(record)
		if err != nil {
			return fmt.Errorf("replication provenance: %w", err)
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
	if record.SchemaVersion != ReplicatedRunSchemaVersion &&
		record.SchemaVersion != LegacyAuthenticatedReplicatedRunSchemaVersion &&
		record.SchemaVersion != AuthenticatedReplicatedRunSchemaVersion {
		return errors.New("replication schema version is invalid")
	}
	if !validReplicationMetadata(record.ManifestName) ||
		!validReplicationMetadata(record.DeclaredVariable) {
		return errors.New("replication metadata is invalid")
	}
	if record.SchemaVersion == AuthenticatedReplicatedRunSchemaVersion {
		if !validSHA256(record.ManifestContractSHA256) ||
			!validSHA256(record.ProcedureSHA256) ||
			!validSHA256(record.ProvenanceSHA256) ||
			(record.Status == ReplicationStatusComplete && !validSHA256(record.BindingSHA256)) ||
			(record.Status == ReplicationStatusIncomplete && record.BindingSHA256 != "") {
			return errors.New("authenticated replication binding is invalid")
		}
	} else if record.SchemaVersion == LegacyAuthenticatedReplicatedRunSchemaVersion {
		if !validSHA256(record.ManifestContractSHA256) ||
			record.ProcedureSHA256 != "" ||
			!validSHA256(record.ProvenanceSHA256) ||
			(record.Status == ReplicationStatusComplete && !validSHA256(record.BindingSHA256)) ||
			(record.Status == ReplicationStatusIncomplete && record.BindingSHA256 != "") {
			return errors.New("legacy authenticated replication binding is invalid")
		}
	} else if record.ManifestContractSHA256 != "" || record.ProcedureSHA256 != "" || record.BindingSHA256 != "" {
		return errors.New("legacy replication binding fields are invalid")
	} else if record.ProvenanceSHA256 != "" && !validSHA256(record.ProvenanceSHA256) {
		return errors.New("replication metadata provenance is invalid")
	}
	for _, pair := range record.Pairs {
		expectedDirectory := fmt.Sprintf("pair-%03d-%s", pair.Pair, pair.Order)
		if pair.Pair < 1 || pair.Pair > maxReplicatedPairs ||
			(pair.Order != ReplicationOrderBaselineTreatment &&
				pair.Order != ReplicationOrderTreatmentBaseline) ||
			pair.Directory != expectedDirectory ||
			!validPairSessionsForOrder(pair) ||
			(pair.Status != ReplicationStatusComplete && pair.Status != ReplicationStatusIncomplete) {
			return errors.New("replication pair metadata is invalid")
		}
		if record.SchemaVersion >= LegacyAuthenticatedReplicatedRunSchemaVersion {
			if pair.Status == ReplicationStatusComplete &&
				(!validSHA256(pair.FirstSessionBindingSHA256) ||
					!validSHA256(pair.SecondSessionBindingSHA256) ||
					!validSHA256(pair.BindingSHA256)) {
				return errors.New("authenticated replication pair binding is invalid")
			}
			if pair.Status == ReplicationStatusIncomplete &&
				(pair.FirstSessionBindingSHA256 != "" ||
					pair.SecondSessionBindingSHA256 != "" ||
					pair.BindingSHA256 != "") {
				return errors.New("incomplete replication pair binding is invalid")
			}
		} else if pair.FirstSessionBindingSHA256 != "" ||
			pair.SecondSessionBindingSHA256 != "" ||
			pair.BindingSHA256 != "" {
			return errors.New("legacy replication pair binding fields are invalid")
		}
	}
	data, _ := json.MarshalIndent(record, "", "  ")
	data = append(data, '\n')
	path := filepath.Join(outputDir, "replication.json")
	if err := securefs.WriteExclusiveExistingParent(path, data, 0o600); err != nil {
		return fmt.Errorf("create replication metadata: %w", err)
	}
	return nil
}

func validPairSessionsForOrder(pair ReplicatedPairRecord) bool {
	if pair.Order == ReplicationOrderBaselineTreatment {
		return pair.FirstSession == "baseline" && pair.SecondSession == "treatment"
	}
	return pair.FirstSession == "treatment" && pair.SecondSession == "baseline"
}
func validReplicationMetadata(value string) bool {
	return value != "" &&
		len(value) <= 1024 &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
