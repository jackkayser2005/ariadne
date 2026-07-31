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
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/analysis"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
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
	"interact",
	"capture_network",
	"capture_storage",
	"disconnect_network",
}

var legacyExpectedSteps = []string{
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
	Unknowns     int
}

type document struct {
	SchemaVersion          int                 `json:"schema_version"`
	ManifestName           string              `json:"manifest_name"`
	DeclaredVariable       string              `json:"declared_variable"`
	ManifestContractSHA256 string              `json:"manifest_contract_sha256,omitempty"`
	Question               string              `json:"question,omitempty"`
	AnswerState            evidence.State      `json:"answer_state,omitempty"`
	Target                 target              `json:"target"`
	Normalizations         []string            `json:"normalizations"`
	Artifacts              []artifact          `json:"artifacts"`
	Comparison             analysis.Comparison `json:"comparison"`
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
	var comparison analysis.Comparison
	var normalizations []string
	if sessionComplete(treatment.record) {
		treatmentNormalized, err := analysis.Normalize(
			bytes.NewReader(treatment.storage),
			bytes.NewReader(treatment.network),
		)
		if err != nil {
			return Summary{}, fmt.Errorf("treatment: %w", err)
		}
		comparison = analysis.Compare(
			baselineNormalized,
			treatmentNormalized,
			baseline.record.VolatileFields,
		)
		normalizations = []string{
			"decoded network body_base64",
			"required storage and network payload equality per session",
			"removed HTTP transport fields from semantic comparison",
		}
		for _, field := range comparison.NormalizedFields {
			normalizations = append(
				normalizations,
				"removed declared volatile observation field "+field+" from comparison",
			)
		}
	} else {
		treatmentAvailable, err := analysis.NormalizeNetwork(bytes.NewReader(treatment.network))
		if err != nil {
			return Summary{}, fmt.Errorf("treatment: %w", err)
		}
		comparison = incompleteTreatmentComparison(baselineNormalized, treatmentAvailable)
		normalizations = []string{
			"decoded available network body_base64",
			"required baseline storage and network payload equality",
			"withheld semantic comparison without treatment storage",
		}
	}

	artifacts := []artifact{baseline.metadata}
	artifacts = append(artifacts, baseline.artifacts...)
	artifacts = append(artifacts, treatment.metadata)
	artifacts = append(artifacts, treatment.artifacts...)
	evidenceSchemaVersion := 4
	question := ""
	answerState := evidence.State("")
	if baseline.record.ManifestContractSHA256 != "" {
		evidenceSchemaVersion = 6
		question = "Did changing " + baseline.record.DeclaredVariable + " influence an observed output?"
		answerState = evidence.Observed
		if !sessionComplete(treatment.record) {
			answerState = evidence.Unknown
		}
	}
	evidence := document{
		SchemaVersion:          evidenceSchemaVersion,
		ManifestName:           baseline.record.ManifestName,
		DeclaredVariable:       baseline.record.DeclaredVariable,
		ManifestContractSHA256: baseline.record.ManifestContractSHA256,
		Question:               question,
		AnswerState:            answerState,
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
		Normalizations: normalizations,
		Artifacts:      artifacts,
		Comparison:     comparison,
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
		Unknowns:     len(comparison.Unknowns),
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
	expectedCount := len(expected)
	if !sessionComplete(record) {
		expectedCount = 1
	}
	if len(record.Artifacts) != expectedCount {
		return loadedSession{}, fmt.Errorf(
			"%s: session metadata: expected %d artifacts",
			kind,
			expectedCount,
		)
	}
	for _, wanted := range expected[:expectedCount] {
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
			"schema_version":           {},
			"kind":                     {},
			"manifest_name":            {},
			"declared_variable":        {},
			"persona_fields":           {},
			"volatile_fields":          {},
			"tap_resource_id":          {},
			"manifest_contract_sha256": {},
			"adb_version":              {},
			"device":                   {},
			"package":                  {},
			"android_api":              {},
			"architecture":             {},
			"package_version_code":     {},
			"package_sha256":           {},
			"ariadne_revision":         {},
			"ariadne_modified":         {},
			"status":                   {},
			"failure_stage":            {},
			"started_at":               {},
			"finished_at":              {},
			"steps":                    {},
			"artifacts":                {},
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
	if (record.SchemaVersion != 2 &&
		record.SchemaVersion != 3 &&
		record.SchemaVersion != 4 &&
		record.SchemaVersion != 5 &&
		record.SchemaVersion != 6) ||
		record.Kind != kind {
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
	if record.SchemaVersion < 4 && len(record.VolatileFields) > 0 {
		return errors.New("legacy session volatile_fields is invalid")
	}
	if err := experiment.ValidateVolatileFields(record.VolatileFields); err != nil {
		return err
	}
	if record.SchemaVersion < 5 && record.TapResourceID != "" {
		return errors.New("legacy session tap_resource_id is invalid")
	}
	if record.SchemaVersion >= 5 && !experiment.ValidResourceID(record.TapResourceID) {
		return errors.New("tap_resource_id is invalid")
	}
	if record.SchemaVersion < 6 && record.ManifestContractSHA256 != "" {
		return errors.New("legacy session manifest_contract_sha256 is invalid")
	}
	if record.SchemaVersion >= 6 && !validDigest(record.ManifestContractSHA256) {
		return errors.New("manifest_contract_sha256 is invalid")
	}
	if !slices.IsSorted(record.VolatileFields) {
		return errors.New("volatile_fields are not canonical")
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
	if record.SchemaVersion == 2 {
		if record.Status != "" || record.FailureStage != "" {
			return errors.New("legacy session outcome is invalid")
		}
	} else {
		switch record.Status {
		case "complete":
			if record.FailureStage != "" {
				return errors.New("complete session failure_stage is invalid")
			}
		case "incomplete":
			if !adb.ValidFailureStage(record.FailureStage) {
				return errors.New("incomplete session failure_stage is invalid")
			}
			if record.FailureStage != "capture_storage" {
				return fmt.Errorf("session is incomplete at %s", record.FailureStage)
			}
		default:
			return errors.New("session status is invalid")
		}
	}
	steps := expectedSteps
	if record.SchemaVersion < 5 {
		steps = legacyExpectedSteps
	}
	if len(record.Steps) != len(steps) {
		return errors.New("step sequence is incomplete")
	}
	previous := record.StartedAt
	for index, expected := range steps {
		step := record.Steps[index]
		statusValid := step.Status == "ok" && step.ExitCode == 0
		if !sessionComplete(record) && expected == "capture_storage" {
			statusValid = step.Status == "error" && step.ExitCode != 0
		}
		if step.Name != expected ||
			!statusValid ||
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
	if !sessionComplete(baseline) ||
		baseline.SchemaVersion != treatment.SchemaVersion ||
		baseline.ManifestName != treatment.ManifestName ||
		baseline.DeclaredVariable != treatment.DeclaredVariable ||
		baseline.PersonaFields != treatment.PersonaFields ||
		!slices.Equal(baseline.VolatileFields, treatment.VolatileFields) ||
		baseline.ADBVersion != treatment.ADBVersion ||
		baseline.Device != treatment.Device ||
		baseline.Package != treatment.Package ||
		baseline.AndroidAPI != treatment.AndroidAPI ||
		baseline.Architecture != treatment.Architecture ||
		baseline.PackageVersionCode != treatment.PackageVersionCode ||
		baseline.PackageSHA256 != treatment.PackageSHA256 ||
		baseline.AriadneRevision != treatment.AriadneRevision ||
		baseline.AriadneModified != treatment.AriadneModified ||
		baseline.TapResourceID != treatment.TapResourceID ||
		baseline.ManifestContractSHA256 != treatment.ManifestContractSHA256 ||
		treatment.StartedAt.Before(baseline.FinishedAt) {
		return errors.New("baseline and treatment session metadata disagree")
	}
	return nil
}

func sessionComplete(record adb.SessionRecord) bool {
	return record.SchemaVersion == 2 || record.Status == "complete"
}

func incompleteTreatmentComparison(baseline, treatment analysis.Session) analysis.Comparison {
	fields := make(map[string]struct{}, len(baseline.Fields)+len(treatment.Fields))
	for field := range baseline.Fields {
		fields[field] = struct{}{}
	}
	for field := range treatment.Fields {
		fields[field] = struct{}{}
	}

	unknowns := make([]analysis.Unknown, 0, len(fields))
	for _, field := range slices.Sorted(maps.Keys(fields)) {
		references := make([]string, 0, 3)
		if _, ok := baseline.Fields[field]; ok {
			references = append(
				references,
				"baseline/observations/storage.json#/"+field,
				"baseline/observations/network.json#decoded-body/"+field,
			)
		}
		if _, ok := treatment.Fields[field]; ok {
			references = append(
				references,
				"treatment/observations/network.json#decoded-body/"+field,
			)
		}
		unknowns = append(unknowns, analysis.Unknown{
			Field:    field,
			State:    evidence.Unknown,
			Reason:   "treatment storage observation was not captured",
			Evidence: references,
		})
	}
	return analysis.Comparison{
		SchemaVersion:    4,
		UnchangedFields:  make([]string, 0),
		NormalizedFields: make([]string, 0),
		Differences:      make([]analysis.Difference, 0),
		Unknowns:         unknowns,
	}
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
	if evidence.ManifestContractSHA256 != "" {
		fmt.Fprintf(
			&report,
			"- Manifest contract SHA-256: %s\n",
			code(evidence.ManifestContractSHA256),
		)
	}
	if evidence.Question != "" {
		fmt.Fprintf(&report, "- Question: %s\n", code(evidence.Question))
		fmt.Fprintf(&report, "- Answer state: %s\n", code(string(evidence.AnswerState)))
	}
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
	fmt.Fprintf(&report, "- Unknown conclusions: %d\n", len(evidence.Comparison.Unknowns))

	report.WriteString("\n## Findings\n")
	if len(evidence.Comparison.Differences) == 0 {
		if len(evidence.Comparison.Unknowns) == 0 {
			report.WriteString("\nNo observed differences.\n")
		} else {
			report.WriteString("\nNo counterfactual difference was established.\n")
		}
	}
	for _, difference := range evidence.Comparison.Differences {
		fmt.Fprintf(&report, "\n### %s\n\n", code(difference.Field))
		fmt.Fprintf(&report, "- State: %s\n", code(string(difference.State)))
		fmt.Fprintf(&report, "- Kind: %s\n", code(difference.Kind))
		if difference.Kind != "added" {
			fmt.Fprintf(&report, "- Baseline: %s\n", code(difference.Baseline))
		}
		if difference.Kind != "removed" {
			fmt.Fprintf(&report, "- Treatment: %s\n", code(difference.Treatment))
		}
		report.WriteString("- Evidence:\n")
		for _, reference := range difference.Evidence {
			fmt.Fprintf(&report, "  - %s\n", code(reference))
		}
	}

	if len(evidence.Comparison.Unknowns) > 0 {
		report.WriteString("\n## Unknowns\n")
		for _, unknown := range evidence.Comparison.Unknowns {
			fmt.Fprintf(&report, "\n### %s\n\n", code(unknown.Field))
			fmt.Fprintf(&report, "- State: %s\n", code(string(unknown.State)))
			fmt.Fprintf(&report, "- Reason: %s\n", html.EscapeString(unknown.Reason))
			report.WriteString("- Available evidence:\n")
			for _, reference := range unknown.Evidence {
				fmt.Fprintf(&report, "  - %s\n", code(reference))
			}
		}
	}

	report.WriteString("\n## Stable Fields\n")
	if len(evidence.Comparison.UnchangedFields) == 0 {
		report.WriteString("\nNo stable fields were established.")
	}
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
