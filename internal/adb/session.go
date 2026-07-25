package adb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackkayser2005/ariadne/internal/experiment"
)

const sessionSchemaVersion = 1

// SessionRecord describes one isolated fixture execution without persona values.
type SessionRecord struct {
	SchemaVersion    int          `json:"schema_version"`
	Kind             string       `json:"kind"`
	ManifestName     string       `json:"manifest_name"`
	DeclaredVariable string       `json:"declared_variable"`
	PersonaFields    int          `json:"persona_fields"`
	ADBVersion       string       `json:"adb_version"`
	Device           string       `json:"device"`
	Package          string       `json:"package"`
	StartedAt        time.Time    `json:"started_at"`
	FinishedAt       time.Time    `json:"finished_at"`
	Steps            []StepRecord `json:"steps"`
	Artifacts        []Artifact   `json:"artifacts,omitempty"`
}

// StepRecord describes one ADB command without its arguments or output.
type StepRecord struct {
	Name       string    `json:"name"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Status     string    `json:"status"`
	ExitCode   int       `json:"exit_code"`
}

// Artifact identifies one captured file and its origin.
type Artifact struct {
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Path      string `json:"path"`
	SizeBytes int    `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// RunPair executes isolated baseline and treatment fixture sessions.
func RunPair(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
) error {
	return runPairWith(ctx, binary, target, manifest, outputDir, runCommand, time.Now)
}

func runPairWith(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	run commandRunner,
	now func() time.Time,
) error {
	if !validSelection(binary) {
		return errors.New("adb binary is invalid")
	}
	if !validSelection(target.Device) {
		return errors.New("device is invalid")
	}
	if !validSelection(target.Package) {
		return errors.New("package is invalid")
	}
	if !validSelection(target.Version) {
		return errors.New("adb version is invalid")
	}
	if err := manifest.Validate(); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if strings.TrimSpace(outputDir) == "" {
		return errors.New("output directory is required")
	}
	if err := validatePersonaForShell(manifest.Baseline); err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	if err := validatePersonaForShell(manifest.Treatment); err != nil {
		return fmt.Errorf("treatment: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	sessions := []struct {
		kind    string
		persona experiment.Persona
	}{
		{kind: "baseline", persona: manifest.Baseline},
		{kind: "treatment", persona: manifest.Treatment},
	}
	for _, session := range sessions {
		if err := runSession(
			ctx,
			binary,
			target,
			manifest,
			outputDir,
			session.kind,
			session.persona,
			run,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func runSession(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir, kind string,
	persona experiment.Persona,
	run commandRunner,
	now func() time.Time,
) error {
	sessionDir := filepath.Join(outputDir, kind)
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		return fmt.Errorf("%s: create session directory: %w", kind, err)
	}

	record := SessionRecord{
		SchemaVersion:    sessionSchemaVersion,
		Kind:             kind,
		ManifestName:     manifest.Name,
		DeclaredVariable: manifest.Variable,
		PersonaFields:    len(persona),
		ADBVersion:       target.Version,
		Device:           target.Device,
		Package:          target.Package,
		StartedAt:        now().UTC(),
	}

	reset, output, err := runStep(
		ctx,
		run,
		now,
		"reset",
		binary,
		"-s", target.Device,
		"shell", "pm", "clear", target.Package,
	)
	record.Steps = append(record.Steps, reset)
	if err == nil && strings.TrimSpace(string(output)) != "Success" {
		record.Steps[len(record.Steps)-1].Status = "error"
		err = errors.New("reset output was not recognized")
	}
	if err != nil {
		return finishSession(sessionDir, &record, now, fmt.Errorf("%s: reset package: %w", kind, err))
	}

	args := []string{
		"-s", target.Device,
		"shell", "am", "start", "-W", "-S",
		"-n", target.Package + "/.MainActivity",
	}
	keys := make([]string, 0, len(persona))
	for key := range persona {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--es", key, persona[key])
	}

	start, _, err := runStep(ctx, run, now, "start", binary, args...)
	record.Steps = append(record.Steps, start)
	if err != nil {
		return finishSession(sessionDir, &record, now, fmt.Errorf("%s: start fixture: %w", kind, err))
	}

	capture, output, err := runStep(
		ctx,
		run,
		now,
		"capture_storage",
		binary,
		"-s", target.Device,
		"exec-out", "run-as", target.Package,
		"cat", "files/observation.json",
	)
	record.Steps = append(record.Steps, capture)
	if err != nil {
		return finishSession(sessionDir, &record, now, fmt.Errorf("%s: capture storage: %w", kind, err))
	}
	artifact, err := writeStorageObservation(sessionDir, output)
	if err != nil {
		record.Steps[len(record.Steps)-1].Status = "error"
		return finishSession(sessionDir, &record, now, fmt.Errorf("%s: capture storage: %w", kind, err))
	}
	record.Artifacts = append(record.Artifacts, artifact)
	return finishSession(sessionDir, &record, now, nil)
}

func writeStorageObservation(sessionDir string, data []byte) (Artifact, error) {
	trimmed := bytes.TrimSpace(data)
	if len(data) == 0 {
		return Artifact{}, errors.New("empty observation")
	}
	if len(data) > maxOutputBytes {
		return Artifact{}, fmt.Errorf("observation exceeds %d-byte limit", maxOutputBytes)
	}
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(data) {
		return Artifact{}, errors.New("observation is not a valid JSON object")
	}

	const relativePath = "observations/storage.json"
	directory := filepath.Join(sessionDir, "observations")
	if err := os.Mkdir(directory, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create observation directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, filepath.FromSlash(relativePath)), data, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write observation: %w", err)
	}

	sum := sha256.Sum256(data)
	return Artifact{
		Kind:      "android_private_storage",
		Source:    "files/observation.json",
		Path:      relativePath,
		SizeBytes: len(data),
		SHA256:    fmt.Sprintf("%x", sum),
	}, nil
}

func runStep(
	ctx context.Context,
	run commandRunner,
	now func() time.Time,
	name, binary string,
	args ...string,
) (StepRecord, []byte, error) {
	step := StepRecord{Name: name, StartedAt: now().UTC()}
	output, err := run(ctx, binary, args...)
	step.FinishedAt = now().UTC()
	step.ExitCode = commandExitCode(err)
	step.Status = "ok"
	if err != nil {
		step.Status = "error"
	}
	return step, output, err
}

func finishSession(
	sessionDir string,
	record *SessionRecord,
	now func() time.Time,
	sessionErr error,
) error {
	record.FinishedAt = now().UTC()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), data, 0o600); err != nil {
		return fmt.Errorf("write session metadata: %w", err)
	}
	return sessionErr
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func validatePersonaForShell(persona experiment.Persona) error {
	for key, value := range persona {
		if !safeShellToken(key) || !safeShellToken(value) {
			return fmt.Errorf("persona field %q cannot be passed safely", key)
		}
	}
	return nil
}

func safeShellToken(value string) bool {
	if value == "" || len(value) > 1024 {
		return false
	}
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && !strings.ContainsRune("._@:+-", character) {
			return false
		}
	}
	return true
}
