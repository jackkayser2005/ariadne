package adb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	"github.com/jackkayser2005/ariadne/internal/provenance"
)

// SessionBindingSHA256 returns the canonical safe identity of an authenticated
// Android session. It excludes persona values and captured payloads.
func SessionBindingSHA256(record SessionRecord) (string, error) {
	adapterProvenance, err := ReplicationProvenanceSHA256(record.ManifestContractSHA256)
	if err != nil {
		return "", fmt.Errorf("session provenance: %w", err)
	}
	steps := make([]provenance.StepBinding, 0, len(record.Steps))
	for _, step := range record.Steps {
		steps = append(steps, provenance.StepBinding{
			Name:              step.Name,
			StartedAt:         step.StartedAt,
			FinishedAt:        step.FinishedAt,
			Status:            step.Status,
			ExitCode:          step.ExitCode,
			UIHierarchySHA256: step.UIHierarchySHA256,
		})
	}
	artifacts := make([]provenance.ArtifactBinding, 0, len(record.Artifacts))
	for _, artifact := range record.Artifacts {
		artifacts = append(artifacts, provenance.ArtifactBinding{
			Kind:      artifact.Kind,
			Source:    artifact.Source,
			Path:      artifact.Path,
			SizeBytes: artifact.SizeBytes,
			SHA256:    artifact.SHA256,
		})
	}
	binding := provenance.SessionBinding{
		SchemaVersion:          provenance.BindingSchemaVersion,
		Kind:                   provenance.SessionBindingKind,
		Source:                 ReplicationSource,
		Adapter:                ReplicationAdapter,
		AdapterVersion:         ReplicationAdapterVersion,
		Scope:                  ReplicationScope,
		ManifestName:           record.ManifestName,
		DeclaredVariable:       record.DeclaredVariable,
		PersonaFields:          record.PersonaFields,
		VolatileFields:         append([]string(nil), record.VolatileFields...),
		TapResourceID:          record.TapResourceID,
		ManifestContractSHA256: record.ManifestContractSHA256,
		ProvenanceSHA256:       adapterProvenance,
		ChallengeCommitment:    record.ChallengeCommitment,
		Role:                   record.Role,
		Order:                  record.Order,
		ProcedureSHA256:        record.ProcedureSHA256,
		ResetPolicy:            record.ResetPolicy,
		Target: provenance.TargetBinding{
			ADBVersion:         record.ADBVersion,
			DeviceSHA256:       provenance.SHA256String(record.Device),
			Package:            record.Package,
			AndroidAPI:         record.AndroidAPI,
			Architecture:       record.Architecture,
			PackageVersionCode: record.PackageVersionCode,
			PackageSHA256:      record.PackageSHA256,
			AriadneRevision:    record.AriadneRevision,
			AriadneModified:    record.AriadneModified,
		},
		Status:       record.Status,
		FailureStage: record.FailureStage,
		StartedAt:    record.StartedAt,
		FinishedAt:   record.FinishedAt,
		Steps:        steps,
		Artifacts:    artifacts,
	}
	return binding.SHA256()
}

// ReplicatedPairBindingSHA256 returns the canonical identity of one ordered
// pair and its two authenticated session bindings.
func ReplicatedPairBindingSHA256(record ReplicatedRunRecord, pair ReplicatedPairRecord) (string, error) {
	binding := provenance.PairBinding{
		SchemaVersion:              provenance.BindingSchemaVersion,
		Kind:                       provenance.PairBindingKind,
		Source:                     ReplicationSource,
		Adapter:                    ReplicationAdapter,
		AdapterVersion:             ReplicationAdapterVersion,
		Scope:                      ReplicationScope,
		ManifestName:               record.ManifestName,
		DeclaredVariable:           record.DeclaredVariable,
		ManifestContractSHA256:     record.ManifestContractSHA256,
		ProvenanceSHA256:           record.ProvenanceSHA256,
		ProcedureSHA256:            record.ManifestContractSHA256,
		ResetPolicy:                record.ResetPolicy,
		Pair:                       pair.Pair,
		Order:                      pair.Order,
		Directory:                  pair.Directory,
		FirstSession:               pair.FirstSession,
		SecondSession:              pair.SecondSession,
		FirstSessionBindingSHA256:  pair.FirstSessionBindingSHA256,
		SecondSessionBindingSHA256: pair.SecondSessionBindingSHA256,
	}
	return binding.SHA256()
}

// ReplicatedBindingSHA256 returns the canonical identity of a complete
// authenticated replication root. The root binding does not include evidence
// output, which is produced and verified after device execution.
func ReplicatedBindingSHA256(record ReplicatedRunRecord) (string, error) {
	pairs := make([]provenance.PairReference, 0, len(record.Pairs))
	for _, pair := range record.Pairs {
		pairs = append(pairs, provenance.PairReference{
			Pair:          pair.Pair,
			Order:         pair.Order,
			BindingSHA256: pair.BindingSHA256,
		})
	}
	binding := provenance.ReplicationBinding{
		SchemaVersion:          provenance.BindingSchemaVersion,
		Kind:                   provenance.ReplicationBindingKind,
		Source:                 ReplicationSource,
		Adapter:                ReplicationAdapter,
		AdapterVersion:         ReplicationAdapterVersion,
		Scope:                  ReplicationScope,
		ManifestName:           record.ManifestName,
		DeclaredVariable:       record.DeclaredVariable,
		ManifestContractSHA256: record.ManifestContractSHA256,
		ProvenanceSHA256:       record.ProvenanceSHA256,
		PairsPerOrder:          record.PairsPerOrder,
		ResetPolicy:            record.ResetPolicy,
		Pairs:                  pairs,
	}
	return binding.SHA256()
}

func readSessionBinding(sessionDir, kind string) (string, error) {
	if kind != "baseline" && kind != "treatment" {
		return "", errors.New("authenticated session kind is invalid")
	}
	path := filepath.Join(sessionDir, kind, "session.json")
	data, err := readSessionBindingFile(path, maxOutputBytes)
	if err != nil {
		return "", fmt.Errorf("read %s session: %w", kind, err)
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return "", errors.New("session metadata is invalid")
	}
	var record SessionRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return "", errors.New("session metadata is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("session metadata has trailing data")
	}
	if record.SchemaVersion != AuthenticatedSessionSchemaVersion || record.Kind != kind || record.BindingSHA256 == "" {
		return "", errors.New("authenticated session binding is unavailable")
	}
	expected, err := SessionBindingSHA256(record)
	if err != nil || expected != record.BindingSHA256 {
		return "", errors.New("authenticated session binding is invalid")
	}
	return record.BindingSHA256, nil
}

func readSessionBindingFile(path string, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("session metadata limit is invalid")
	}
	info, err := lstatSessionPath(path)
	if err != nil {
		return nil, errors.New("session metadata is unavailable")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("session metadata must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("session metadata is unavailable")
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, errors.New("session metadata changed during verification")
	}
	currentInfo, err := lstatSessionPath(path)
	if err != nil || !currentInfo.Mode().IsRegular() || !os.SameFile(currentInfo, openedInfo) {
		return nil, errors.New("session metadata changed during verification")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("session metadata exceeds %d-byte limit", limit)
	}
	return data, nil
}

func lstatSessionPath(path string) (os.FileInfo, error) {
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
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			return nil, errors.New("unsafe session metadata path")
		}
	}
	return info, nil
}

func bindReplicatedPair(outputDir string, record ReplicatedRunRecord, pair ReplicatedPairRecord) (ReplicatedPairRecord, error) {
	first, err := readSessionBinding(filepath.Join(outputDir, pair.Directory), pair.FirstSession)
	if err != nil {
		return pair, err
	}
	second, err := readSessionBinding(filepath.Join(outputDir, pair.Directory), pair.SecondSession)
	if err != nil {
		return pair, err
	}
	pair.FirstSessionBindingSHA256 = first
	pair.SecondSessionBindingSHA256 = second
	pair.BindingSHA256, err = ReplicatedPairBindingSHA256(record, pair)
	if err != nil {
		return pair, fmt.Errorf("pair provenance: %w", err)
	}
	return pair, nil
}
