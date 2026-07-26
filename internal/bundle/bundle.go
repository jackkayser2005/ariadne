// Package bundle verifies experiment artifacts and writes evidence output.
package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/analysis"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	maxSessionBytes = 64 << 10
	maxStorageBytes = 64 << 10
	maxNetworkBytes = 96 << 10
)

var expectedSteps = []string{
	"reset",
	"connect_network",
	"start",
	"capture_network",
	"capture_storage",
	"disconnect_network",
}

// Summary describes a completed evidence bundle without observed values.
type Summary struct {
	ManifestName string
	Differences  int
}

type document struct {
	SchemaVersion    int                 `json:"schema_version"`
	ManifestName     string              `json:"manifest_name"`
	DeclaredVariable string              `json:"declared_variable"`
	Target           target              `json:"target"`
	Normalizations   []string            `json:"normalizations"`
	Artifacts        []artifact          `json:"artifacts"`
	Comparison       analysis.Comparison `json:"comparison"`
}

type target struct {
	ADBVersion         string `json:"adb_version"`
	Device             string `json:"device"`
	AndroidAPI         int    `json:"android_api"`
	Architecture       string `json:"architecture"`
	Package            string `json:"package"`
	PackageVersionCode uint64 `json:"package_version_code"`
	PackageSHA256      string `json:"package_sha256"`
	AriadneRevision    string `json:"ariadne_revision"`
	AriadneModified    bool   `json:"ariadne_modified"`
}

type artifact struct {
	Path      string `json:"path"`
	SizeBytes int    `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

type loadedSession struct {
	record    adb.SessionRecord
	metadata  artifact
	network   []byte
	storage   []byte
	artifacts []artifact
}

// Write verifies runDir and creates evidence.json and report.md without overwriting.
func Write(runDir string) (Summary, error) {
	if strings.TrimSpace(runDir) == "" {
		return Summary{}, errors.New("run directory is required")
	}

	baseline, err := loadSession(runDir, "baseline")
	if err != nil {
		return Summary{}, err
	}
	treatment, err := loadSession(runDir, "treatment")
	if err != nil {
		return Summary{}, err
	}
	if err := validatePair(baseline.record, treatment.record); err != nil {
		return Summary{}, err
	}

	baselineNormalized, err := analysis.Normalize(
		bytes.NewReader(baseline.storage),
		bytes.NewReader(baseline.network),
	)
	if err != nil {
		return Summary{}, fmt.Errorf("baseline: %w", err)
	}
	treatmentNormalized, err := analysis.Normalize(
		bytes.NewReader(treatment.storage),
		bytes.NewReader(treatment.network),
	)
	if err != nil {
		return Summary{}, fmt.Errorf("treatment: %w", err)
	}
	comparison := analysis.Compare(baselineNormalized, treatmentNormalized)

	artifacts := []artifact{baseline.metadata}
	artifacts = append(artifacts, baseline.artifacts...)
	artifacts = append(artifacts, treatment.metadata)
	artifacts = append(artifacts, treatment.artifacts...)
	evidence := document{
		SchemaVersion:    2,
		ManifestName:     baseline.record.ManifestName,
		DeclaredVariable: baseline.record.DeclaredVariable,
		Target: target{
			ADBVersion:         baseline.record.ADBVersion,
			Device:             baseline.record.Device,
			AndroidAPI:         baseline.record.AndroidAPI,
			Architecture:       baseline.record.Architecture,
			Package:            baseline.record.Package,
			PackageVersionCode: baseline.record.PackageVersionCode,
			PackageSHA256:      baseline.record.PackageSHA256,
			AriadneRevision:    baseline.record.AriadneRevision,
			AriadneModified:    baseline.record.AriadneModified,
		},
		Normalizations: []string{
			"decoded network body_base64",
			"required storage and network payload equality per session",
			"removed HTTP transport fields from semantic comparison",
		},
		Artifacts:  artifacts,
		Comparison: comparison,
	}

	evidenceData, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return Summary{}, fmt.Errorf("encode evidence: %w", err)
	}
	evidenceData = append(evidenceData, '\n')
	reportData := renderReport(evidence)
	if err := writeOutputs(runDir, evidenceData, reportData); err != nil {
		return Summary{}, err
	}
	return Summary{
		ManifestName: baseline.record.ManifestName,
		Differences:  len(comparison.Differences),
	}, nil
}

func loadSession(runDir, kind string) (loadedSession, error) {
	sessionPath := filepath.Join(runDir, kind, "session.json")
	sessionData, err := readFileBounded(sessionPath, maxSessionBytes)
	if err != nil {
		return loadedSession{}, fmt.Errorf("%s: session metadata: %w", kind, err)
	}
	var record adb.SessionRecord
	if err := decodeSession(sessionData, &record); err != nil {
		return loadedSession{}, fmt.Errorf("%s: session metadata: %w", kind, err)
	}
	if err := validateSession(record, kind); err != nil {
		return loadedSession{}, fmt.Errorf("%s: session metadata: %w", kind, err)
	}

	loaded := loadedSession{
		record:   record,
		metadata: artifactFor(kind+"/session.json", sessionData),
	}
	expected := []struct {
		path   string
		kind   string
		source string
		limit  int64
		output *[]byte
	}{
		{
			path:   "observations/network.json",
			kind:   "http_request",
			source: "POST /observe",
			limit:  maxNetworkBytes,
			output: &loaded.network,
		},
		{
			path:   "observations/storage.json",
			kind:   "android_private_storage",
			source: "files/observation.json",
			limit:  maxStorageBytes,
			output: &loaded.storage,
		},
	}
	if len(record.Artifacts) != len(expected) {
		return loadedSession{}, fmt.Errorf("%s: session metadata: expected two artifacts", kind)
	}
	for _, wanted := range expected {
		metadata, ok := artifactByPath(record.Artifacts, wanted.path)
		if !ok ||
			metadata.Kind != wanted.kind ||
			metadata.Source != wanted.source ||
			!validDigest(metadata.SHA256) ||
			metadata.SizeBytes < 0 {
			return loadedSession{}, fmt.Errorf("%s: artifact %q metadata is invalid", kind, wanted.path)
		}
		data, err := readFileBounded(
			filepath.Join(runDir, kind, filepath.FromSlash(wanted.path)),
			wanted.limit,
		)
		if err != nil {
			return loadedSession{}, fmt.Errorf("%s: artifact %q: %w", kind, wanted.path, err)
		}
		sum := sha256.Sum256(data)
		if len(data) != metadata.SizeBytes || hex.EncodeToString(sum[:]) != metadata.SHA256 {
			return loadedSession{}, fmt.Errorf("%s: artifact %q integrity check failed", kind, wanted.path)
		}
		*wanted.output = data
		loaded.artifacts = append(
			loaded.artifacts,
			artifactFor(kind+"/"+wanted.path, data),
		)
	}
	return loaded, nil
}

func decodeSession(data []byte, record *adb.SessionRecord) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty input")
	}
	if !utf8.Valid(data) {
		return errors.New("input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		allowed := map[string]struct{}{
			"schema_version":       {},
			"kind":                 {},
			"manifest_name":        {},
			"declared_variable":    {},
			"persona_fields":       {},
			"adb_version":          {},
			"device":               {},
			"package":              {},
			"android_api":          {},
			"architecture":         {},
			"package_version_code": {},
			"package_sha256":       {},
			"ariadne_revision":     {},
			"ariadne_modified":     {},
			"started_at":           {},
			"finished_at":          {},
			"steps":                {},
			"artifacts":            {},
		}
		for field := range fields {
			if _, ok := allowed[field]; !ok {
				return fmt.Errorf("unknown field %q", field)
			}
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(record); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func validateSession(record adb.SessionRecord, kind string) error {
	if record.SchemaVersion != 2 || record.Kind != kind {
		return errors.New("schema_version or kind is invalid")
	}
	for name, value := range map[string]string{
		"manifest_name":     record.ManifestName,
		"declared_variable": record.DeclaredVariable,
		"adb_version":       record.ADBVersion,
		"device":            record.Device,
		"package":           record.Package,
		"architecture":      record.Architecture,
		"package_sha256":    record.PackageSHA256,
		"ariadne_revision":  record.AriadneRevision,
	} {
		if !validMetadataValue(value) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	if record.PersonaFields < 1 {
		return errors.New("persona_fields is invalid")
	}
	if record.AndroidAPI < 1 || record.AndroidAPI > 999 {
		return errors.New("android_api is invalid")
	}
	if record.PackageVersionCode == 0 {
		return errors.New("package_version_code is invalid")
	}
	if !validDigest(record.PackageSHA256) {
		return errors.New("package_sha256 is invalid")
	}
	if !validRevision(record.AriadneRevision) {
		return errors.New("ariadne_revision is invalid")
	}
	if record.StartedAt.IsZero() ||
		record.FinishedAt.IsZero() ||
		record.FinishedAt.Before(record.StartedAt) {
		return errors.New("session timestamps are invalid")
	}
	if len(record.Steps) != len(expectedSteps) {
		return errors.New("step sequence is incomplete")
	}
	previous := record.StartedAt
	for index, expected := range expectedSteps {
		step := record.Steps[index]
		if step.Name != expected ||
			step.Status != "ok" ||
			step.ExitCode != 0 ||
			step.StartedAt.IsZero() ||
			step.FinishedAt.IsZero() ||
			step.StartedAt.Before(previous) ||
			step.FinishedAt.Before(step.StartedAt) ||
			record.FinishedAt.Before(step.FinishedAt) {
			return fmt.Errorf("step %q is invalid", expected)
		}
		previous = step.FinishedAt
	}
	return nil
}

func validatePair(baseline, treatment adb.SessionRecord) error {
	if baseline.ManifestName != treatment.ManifestName ||
		baseline.DeclaredVariable != treatment.DeclaredVariable ||
		baseline.PersonaFields != treatment.PersonaFields ||
		baseline.ADBVersion != treatment.ADBVersion ||
		baseline.Device != treatment.Device ||
		baseline.Package != treatment.Package ||
		baseline.AndroidAPI != treatment.AndroidAPI ||
		baseline.Architecture != treatment.Architecture ||
		baseline.PackageVersionCode != treatment.PackageVersionCode ||
		baseline.PackageSHA256 != treatment.PackageSHA256 ||
		baseline.AriadneRevision != treatment.AriadneRevision ||
		baseline.AriadneModified != treatment.AriadneModified ||
		treatment.StartedAt.Before(baseline.FinishedAt) {
		return errors.New("baseline and treatment session metadata disagree")
	}
	return nil
}

func artifactByPath(artifacts []adb.Artifact, path string) (adb.Artifact, bool) {
	var found adb.Artifact
	count := 0
	for _, artifact := range artifacts {
		if artifact.Path == path {
			found = artifact
			count++
		}
	}
	return found, count == 1
}

func readFileBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("exceeds %d-byte limit", limit)
	}
	return data, nil
}

func artifactFor(path string, data []byte) artifact {
	sum := sha256.Sum256(data)
	return artifact{
		Path:      path,
		SizeBytes: len(data),
		SHA256:    hex.EncodeToString(sum[:]),
	}
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validMetadataValue(value string) bool {
	return value != "" &&
		len(value) <= 1024 &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func validRevision(value string) bool {
	if value == "unknown" {
		return true
	}
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func renderReport(evidence document) []byte {
	var report strings.Builder
	report.WriteString("# Evidence Report\n\n")
	fmt.Fprintf(&report, "- Manifest: %s\n", code(evidence.ManifestName))
	fmt.Fprintf(&report, "- Declared variable: %s\n", code(evidence.DeclaredVariable))
	fmt.Fprintf(&report, "- Device: %s\n", code(evidence.Target.Device))
	fmt.Fprintf(&report, "- Android API: %d\n", evidence.Target.AndroidAPI)
	fmt.Fprintf(&report, "- Architecture: %s\n", code(evidence.Target.Architecture))
	fmt.Fprintf(&report, "- Package: %s\n", code(evidence.Target.Package))
	fmt.Fprintf(&report, "- Package version code: %d\n", evidence.Target.PackageVersionCode)
	fmt.Fprintf(&report, "- Package SHA-256: %s\n", code(evidence.Target.PackageSHA256))
	fmt.Fprintf(&report, "- Ariadne revision: %s\n", code(evidence.Target.AriadneRevision))
	fmt.Fprintf(&report, "- Ariadne modified: %t\n", evidence.Target.AriadneModified)
	fmt.Fprintf(&report, "- Verified artifacts: %d\n", len(evidence.Artifacts))
	fmt.Fprintf(&report, "- Observed differences: %d\n", len(evidence.Comparison.Differences))

	report.WriteString("\n## Findings\n")
	if len(evidence.Comparison.Differences) == 0 {
		report.WriteString("\nNo observed differences.\n")
	}
	for _, difference := range evidence.Comparison.Differences {
		fmt.Fprintf(&report, "\n### %s\n\n", code(difference.Field))
		fmt.Fprintf(&report, "- State: %s\n", code(string(difference.State)))
		fmt.Fprintf(&report, "- Baseline: %s\n", code(difference.Baseline))
		fmt.Fprintf(&report, "- Treatment: %s\n", code(difference.Treatment))
		report.WriteString("- Evidence:\n")
		for _, reference := range difference.Evidence {
			fmt.Fprintf(&report, "  - %s\n", code(reference))
		}
	}

	report.WriteString("\n## Stable Fields\n")
	for _, field := range evidence.Comparison.UnchangedFields {
		fmt.Fprintf(&report, "\n- %s", code(field))
	}
	report.WriteString("\n\n## Normalization\n")
	for _, normalization := range evidence.Normalizations {
		fmt.Fprintf(&report, "\n- %s", html.EscapeString(normalization))
	}
	report.WriteString("\n\n## Limitations\n\n")
	report.WriteString("- Covers fixture private storage and one loopback HTTP request per session.\n")
	report.WriteString("- Persona values are excluded from session metadata and this report.\n")
	return []byte(report.String())
}

func code(value string) string {
	return "<code>" + html.EscapeString(value) + "</code>"
}

func writeOutputs(runDir string, evidence, report []byte) error {
	evidencePath := filepath.Join(runDir, "evidence.json")
	reportPath := filepath.Join(runDir, "report.md")
	for _, path := range []string{evidencePath, reportPath} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("output already exists: %s", filepath.Base(path))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect output %s: %w", filepath.Base(path), err)
		}
	}
	if err := writeExclusive(reportPath, report); err != nil {
		return err
	}
	if err := writeExclusive(evidencePath, evidence); err != nil {
		_ = os.Remove(reportPath)
		return err
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	remove = false
	return nil
}
