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
	"time"
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
	maxOutputBytes  = 128 << 10

	treatmentStorageObservationUnknownReason = "treatment storage observation was not captured"
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
	ManifestName           string         `json:"manifest_name"`
	Differences            int            `json:"differences"`
	Unknowns               int            `json:"unknowns"`
	Question               string         `json:"-"`
	AnswerState            evidence.State `json:"-"`
	ManifestContractSHA256 string         `json:"-"`
	AriadneRevision        string         `json:"-"`
	AriadneModified        bool           `json:"-"`
	// RecordedAt is the verified baseline session start in UTC for current bundles.
	RecordedAt string `json:"-"`
	// TargetPackage is the verified package identity for the selected target.
	TargetPackage string `json:"-"`
	// TargetAndroidAPI is the verified Android API for the selected target.
	TargetAndroidAPI int `json:"-"`
	// TargetArchitecture is the verified architecture for the selected target.
	TargetArchitecture string `json:"-"`
	// TargetPackageVersionCode is the verified package version for the selected target.
	TargetPackageVersionCode uint64 `json:"-"`
	// TargetPackageSHA256 is the verified package digest for the selected target.
	TargetPackageSHA256 string `json:"-"`
	// Normalizations lists the verified, deterministic normalization steps for the bundle.
	Normalizations []string `json:"-"`
}

// Finding is the safe, raw-value-free view of one verified conclusion.
type Finding struct {
	Question       string         `json:"question"`
	AnswerState    evidence.State `json:"answer_state"`
	Kind           string         `json:"kind"`
	Classification string         `json:"classification,omitempty"`
	ID             string         `json:"id"`
	Field          string         `json:"field"`
	State          evidence.State `json:"state"`
	Reason         string         `json:"reason,omitempty"`
	Evidence       []string       `json:"evidence"`
}

// Answer is the deterministic result of one bounded bundle question.
type Answer struct {
	QuestionID string         `json:"question_id"`
	Question   string         `json:"question"`
	State      evidence.State `json:"answer_state"`
	Reason     string         `json:"reason,omitempty"`
	FindingIDs []string       `json:"finding_ids"`
}

// Question describes one bounded question available for a verified bundle.
type Question struct {
	ID   string `json:"id"`
	Text string `json:"question"`
}

// Questions returns the fixed question catalog in stable order.
func Questions() []Question {
	return []Question{
		{
			ID:   "counterfactual-change",
			Text: "Did changing the declared variable influence an observed output?",
		},
		{
			ID:   "capture-complete",
			Text: "Were all required observations captured for both sessions?",
		},
		{
			ID:   "source-integrity",
			Text: "Do the verified findings still match their source artifacts?",
		},
	}
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
	evidence, summary, err := buildDocument(runDir, true)
	if err != nil {
		return Summary{}, err
	}
	evidenceData, err := encodeDocument(evidence)
	if err != nil {
		return Summary{}, err
	}
	reportData, err := encodeReport(evidence)
	if err != nil {
		return Summary{}, err
	}
	if err := writeOutputs(runDir, evidenceData, reportData); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// Verify checks an existing evidence bundle without writing either output.
func Verify(runDir string) (Summary, error) {
	_, summary, err := verifyDocument(runDir)
	return summary, err
}

// Find verifies a bundle and returns one finding without observed values.
func Find(runDir, id string) (Finding, error) {
	if !validFindingID(id) {
		return Finding{}, errors.New("finding ID is invalid")
	}
	evidence, _, err := verifyDocument(runDir)
	if err != nil {
		return Finding{}, err
	}
	for _, difference := range evidence.Comparison.Differences {
		if difference.ID == id {
			return Finding{
				Question:       evidence.Question,
				AnswerState:    evidence.AnswerState,
				Kind:           "difference",
				Classification: difference.Kind,
				ID:             difference.ID,
				Field:          difference.Field,
				State:          difference.State,
				Evidence:       slices.Clone(difference.Evidence),
			}, nil
		}
	}
	for _, unknown := range evidence.Comparison.Unknowns {
		if unknown.ID == id {
			return Finding{
				Question:    evidence.Question,
				AnswerState: evidence.AnswerState,
				Kind:        "unknown",
				ID:          unknown.ID,
				Field:       unknown.Field,
				State:       unknown.State,
				Reason:      safeUnknownReason(unknown.Reason),
				Evidence:    slices.Clone(unknown.Evidence),
			}, nil
		}
	}
	return Finding{}, errors.New("finding not found")
}

// Ask verifies a bundle and answers one bounded question without observed values.
func Ask(runDir, questionID string) (Answer, error) {
	catalogQuestion, ok := questionForID(questionID)
	if !ok {
		return Answer{}, errors.New("question ID is invalid")
	}
	verified, _, err := verifyDocument(runDir)
	if err != nil {
		return Answer{}, err
	}
	if verified.SchemaVersion != 7 || verified.Comparison.SchemaVersion != 5 {
		return Answer{}, errors.New("question catalog requires current evidence schema")
	}
	findingIDs := comparisonFindingIDs(verified.Comparison)
	unknownReason := ""
	if verified.AnswerState == evidence.Unknown {
		unknownReason = comparisonUnknownReason(verified.Comparison)
	}
	switch questionID {
	case "counterfactual-change":
		return Answer{
			QuestionID: questionID,
			Question:   catalogQuestion.Text,
			State:      verified.AnswerState,
			Reason:     unknownReason,
			FindingIDs: findingIDs,
		}, nil
	case "capture-complete":
		state := evidence.Observed
		if len(verified.Comparison.Unknowns) > 0 {
			state = evidence.Unknown
		}
		return Answer{
			QuestionID: questionID,
			Question:   catalogQuestion.Text,
			State:      state,
			Reason:     unknownReason,
			FindingIDs: slices.Clone(findingIDs),
		}, nil
	case "source-integrity":
		return Answer{
			QuestionID: questionID,
			Question:   catalogQuestion.Text,
			State:      evidence.Observed,
			FindingIDs: slices.Clone(findingIDs),
		}, nil
	default:
		return Answer{}, errors.New("question ID is invalid")
	}
}

func comparisonUnknownReason(comparison analysis.Comparison) string {
	for _, unknown := range comparison.Unknowns {
		if reason := safeUnknownReason(unknown.Reason); reason != "" {
			return reason
		}
	}
	return ""
}

// safeUnknownReason exposes only reasons owned by the current verifier.
func safeUnknownReason(reason string) string {
	if reason == treatmentStorageObservationUnknownReason {
		return reason
	}
	return ""
}

func questionForID(id string) (Question, bool) {
	for _, question := range Questions() {
		if question.ID == id {
			return question, true
		}
	}
	return Question{}, false
}

func comparisonFindingIDs(comparison analysis.Comparison) []string {
	ids := make([]string, 0, len(comparison.Differences)+len(comparison.Unknowns))
	for _, difference := range comparison.Differences {
		ids = append(ids, difference.ID)
	}
	for _, unknown := range comparison.Unknowns {
		ids = append(ids, unknown.ID)
	}
	return ids
}

func verifyDocument(runDir string) (document, Summary, error) {
	if strings.TrimSpace(runDir) == "" {
		return document{}, Summary{}, errors.New("run directory is required")
	}
	existingEvidence, err := readFileBounded(filepath.Join(runDir, "evidence.json"), maxOutputBytes)
	if err != nil {
		return document{}, Summary{}, fmt.Errorf("evidence output: %w", err)
	}
	outputSchemaVersion, err := evidenceOutputSchema(existingEvidence)
	if err != nil {
		return document{}, Summary{}, fmt.Errorf("evidence output: %w", err)
	}
	evidence, summary, err := buildDocument(runDir, outputSchemaVersion == 7)
	if err != nil {
		return document{}, Summary{}, err
	}
	evidenceData, err := encodeDocument(evidence)
	if err != nil {
		return document{}, Summary{}, err
	}
	if !bytes.Equal(existingEvidence, evidenceData) {
		return document{}, Summary{}, errors.New("evidence output does not match verified artifacts")
	}
	existingReport, err := readFileBounded(filepath.Join(runDir, "report.md"), maxOutputBytes)
	if err != nil {
		return document{}, Summary{}, fmt.Errorf("report output: %w", err)
	}
	reportData, err := encodeReport(evidence)
	if err != nil {
		return document{}, Summary{}, err
	}
	if !bytes.Equal(existingReport, reportData) {
		return document{}, Summary{}, errors.New("report output does not match verified artifacts")
	}
	return evidence, summary, nil
}

func buildDocument(runDir string, includeFindingIDs bool) (document, Summary, error) {
	if strings.TrimSpace(runDir) == "" {
		return document{}, Summary{}, errors.New("run directory is required")
	}

	baseline, err := loadSession(runDir, "baseline")
	if err != nil {
		return document{}, Summary{}, err
	}
	treatment, err := loadSession(runDir, "treatment")
	if err != nil {
		return document{}, Summary{}, err
	}
	if err := validatePair(baseline.record, treatment.record); err != nil {
		return document{}, Summary{}, err
	}

	baselineNormalized, err := analysis.Normalize(
		bytes.NewReader(baseline.storage),
		bytes.NewReader(baseline.network),
	)
	if err != nil {
		return document{}, Summary{}, fmt.Errorf("baseline: %w", err)
	}
	var comparison analysis.Comparison
	var normalizations []string
	if sessionComplete(treatment.record) {
		treatmentNormalized, err := analysis.Normalize(
			bytes.NewReader(treatment.storage),
			bytes.NewReader(treatment.network),
		)
		if err != nil {
			return document{}, Summary{}, fmt.Errorf("treatment: %w", err)
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
			return document{}, Summary{}, fmt.Errorf("treatment: %w", err)
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
	recordedAt := ""
	if baseline.record.ManifestContractSHA256 != "" {
		evidenceSchemaVersion = 6
		question = "Did changing " + baseline.record.DeclaredVariable + " influence an observed output?"
		answerState = evidence.Observed
		recordedAt = baseline.record.StartedAt.UTC().Format(time.RFC3339Nano)
		if !sessionComplete(treatment.record) {
			answerState = evidence.Unknown
		}
		if includeFindingIDs {
			evidenceSchemaVersion = 7
			if err := assignFindingIDs(&comparison, artifacts); err != nil {
				return document{}, Summary{}, err
			}
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

	return evidence, Summary{
		ManifestName:             evidence.ManifestName,
		Differences:              len(comparison.Differences),
		Unknowns:                 len(comparison.Unknowns),
		Question:                 evidence.Question,
		AnswerState:              evidence.AnswerState,
		ManifestContractSHA256:   evidence.ManifestContractSHA256,
		AriadneRevision:          evidence.Target.AriadneRevision,
		AriadneModified:          evidence.Target.AriadneModified,
		RecordedAt:               recordedAt,
		TargetPackage:            evidence.Target.Package,
		TargetAndroidAPI:         evidence.Target.AndroidAPI,
		TargetArchitecture:       evidence.Target.Architecture,
		TargetPackageVersionCode: evidence.Target.PackageVersionCode,
		TargetPackageSHA256:      evidence.Target.PackageSHA256,
		Normalizations:           slices.Clone(evidence.Normalizations),
	}, nil
}

func evidenceOutputSchema(data []byte) (int, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return 0, fmt.Errorf("invalid schema_version: %w", err)
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&header); err != nil {
		return 0, fmt.Errorf("invalid schema_version: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, errors.New("invalid schema_version: trailing data")
	}
	if header.SchemaVersion != 4 && header.SchemaVersion != 6 && header.SchemaVersion != 7 {
		return 0, errors.New("unsupported schema_version")
	}
	return header.SchemaVersion, nil
}

func assignFindingIDs(comparison *analysis.Comparison, artifacts []artifact) error {
	digests := make(map[string]string, len(artifacts))
	for _, item := range artifacts {
		digests[item.Path] = item.SHA256
	}
	for index := range comparison.Differences {
		difference := &comparison.Differences[index]
		id, err := findingID(
			"difference",
			difference.Field,
			string(difference.State),
			difference.Kind,
			difference.Evidence,
			digests,
		)
		if err != nil {
			return err
		}
		difference.ID = id
	}
	for index := range comparison.Unknowns {
		unknown := &comparison.Unknowns[index]
		id, err := findingID(
			"unknown",
			unknown.Field,
			string(unknown.State),
			unknown.Reason,
			unknown.Evidence,
			digests,
		)
		if err != nil {
			return err
		}
		unknown.ID = id
	}
	comparison.SchemaVersion = 5
	return nil
}

func findingID(
	kind, field, state, qualifier string,
	references []string,
	digests map[string]string,
) (string, error) {
	if len(references) == 0 {
		return "", fmt.Errorf("finding %q has no evidence references", field)
	}
	parts := []string{"ariadne:finding:v1", kind, field, state, qualifier}
	for _, reference := range references {
		path, _, ok := strings.Cut(reference, "#")
		if !ok || path == "" {
			return "", fmt.Errorf("finding %q has invalid evidence reference", field)
		}
		digest, ok := digests[path]
		if !ok {
			return "", fmt.Errorf("finding %q references missing artifact %q", field, path)
		}
		parts = append(parts, reference, digest)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func encodeDocument(evidence document) ([]byte, error) {
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode evidence: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxOutputBytes {
		return nil, fmt.Errorf("evidence output exceeds %d-byte limit", maxOutputBytes)
	}
	return data, nil
}

func encodeReport(evidence document) ([]byte, error) {
	data := renderReport(evidence)
	if len(data) > maxOutputBytes {
		return nil, fmt.Errorf("report output exceeds %d-byte limit", maxOutputBytes)
	}
	return data, nil
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
			Reason:   treatmentStorageObservationUnknownReason,
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

func pathSafetyError(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are not allowed")
	}
	if info.Mode()&os.ModeIrregular != 0 {
		return errors.New("reparse points and other irregular path components are not allowed")
	}
	return nil
}

func lstatNoSymlinkPath(path string) (os.FileInfo, error) {
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
		if err := pathSafetyError(info); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func readFileBounded(path string, limit int64) ([]byte, error) {
	info, err := lstatNoSymlinkPath(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("open: regular file required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat after open: %w", err)
	}
	if !os.SameFile(info, openedInfo) {
		return nil, errors.New("open: path changed during verification")
	}
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

func validFindingID(value string) bool {
	const prefix = "sha256:"
	return strings.HasPrefix(value, prefix) && validDigest(value[len(prefix):])
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
		if difference.ID != "" {
			fmt.Fprintf(&report, "- Finding ID: %s\n", code(difference.ID))
		}
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
			if unknown.ID != "" {
				fmt.Fprintf(&report, "- Finding ID: %s\n", code(unknown.ID))
			}
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
