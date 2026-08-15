package adb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/experiment"
)

const sessionSchemaVersion = 7
const networkObservationTimeout = 5 * time.Second
const networkCleanupTimeout = 5 * time.Second
const uiHierarchySettleTimeout = 2 * time.Second
const uiHierarchyRetryInterval = 100 * time.Millisecond

var errFixtureControlNotUnique = errors.New("fixture control was not found uniquely")

const (
	sessionStatusComplete   = "complete"
	sessionStatusIncomplete = "incomplete"
)

// SessionRecord describes one isolated fixture execution without persona values.
type SessionRecord struct {
	SchemaVersion          int          `json:"schema_version"`
	Kind                   string       `json:"kind"`
	ManifestName           string       `json:"manifest_name"`
	DeclaredVariable       string       `json:"declared_variable"`
	PersonaFields          int          `json:"persona_fields"`
	VolatileFields         []string     `json:"volatile_fields,omitempty"`
	TapResourceID          string       `json:"tap_resource_id,omitempty"`
	ManifestContractSHA256 string       `json:"manifest_contract_sha256,omitempty"`
	ChallengeCommitment    string       `json:"challenge_commitment,omitempty"`
	Role                   string       `json:"role,omitempty"`
	Order                  string       `json:"order,omitempty"`
	ProcedureSHA256        string       `json:"procedure_sha256,omitempty"`
	ADBVersion             string       `json:"adb_version"`
	Device                 string       `json:"device"`
	Package                string       `json:"package"`
	AndroidAPI             int          `json:"android_api"`
	Architecture           string       `json:"architecture"`
	PackageVersionCode     uint64       `json:"package_version_code"`
	PackageSHA256          string       `json:"package_sha256"`
	AriadneRevision        string       `json:"ariadne_revision"`
	AriadneModified        bool         `json:"ariadne_modified"`
	Status                 string       `json:"status"`
	FailureStage           string       `json:"failure_stage,omitempty"`
	StartedAt              time.Time    `json:"started_at"`
	FinishedAt             time.Time    `json:"finished_at"`
	Steps                  []StepRecord `json:"steps"`
	Artifacts              []Artifact   `json:"artifacts,omitempty"`
}

// StepRecord describes one session operation without arguments or output.
type StepRecord struct {
	Name              string    `json:"name"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	Status            string    `json:"status"`
	ExitCode          int       `json:"exit_code"`
	UIHierarchySHA256 string    `json:"ui_hierarchy_sha256,omitempty"`
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
	return runPairWithAuthenticated(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		runCommand,
		runInputCommand,
		newChallenge,
		time.Now,
	)
}

func runPairWithAuthenticated(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	run commandRunner,
	writeInput inputCommandRunner,
	challenge challengeGenerator,
	now func() time.Time,
) error {
	return runPairWithOrderAndAuth(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		[]sessionSpec{
			{kind: "baseline", persona: manifest.Baseline},
			{kind: "treatment", persona: manifest.Treatment},
		},
		run,
		&sessionAuthDependencies{
			order:      ReplicationOrderBaselineTreatment,
			writeInput: writeInput,
			challenge:  challenge,
		},
		now,
	)
}

type sessionSpec struct {
	kind    string
	persona experiment.Persona
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
	return runPairWithOrder(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		[]sessionSpec{
			{kind: "baseline", persona: manifest.Baseline},
			{kind: "treatment", persona: manifest.Treatment},
		},
		run,
		now,
	)
}

func runPairWithOrder(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	sessions []sessionSpec,
	run commandRunner,
	now func() time.Time,
) error {
	return runPairWithOrderAndAuth(
		ctx,
		binary,
		target,
		manifest,
		outputDir,
		sessions,
		run,
		nil,
		now,
	)
}
func runPairWithOrderAndAuth(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	sessions []sessionSpec,
	run commandRunner,
	authDependencies *sessionAuthDependencies,
	now func() time.Time,
) error {
	if err := validatePairConfig(binary, target, manifest, outputDir, sessions); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o700); err != nil {
		return fmt.Errorf("create output parent: %w", err)
	}
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if authDependencies != nil && (authDependencies.writeInput == nil || authDependencies.challenge == nil) {
		return errors.New("authenticated session dependencies are required")
	}
	if authDependencies != nil && !experiment.ValidResourceID(manifest.TapResourceID) {
		return errors.New("authenticated sessions require a declared fixture control")
	}

	for _, session := range sessions {
		var auth *sessionAuth
		if authDependencies != nil {
			auth = authDependencies.forSession()
		}
		if err := runSessionWithAuth(
			ctx,
			binary,
			target,
			manifest,
			outputDir,
			session.kind,
			session.persona,
			run,
			now,
			auth,
		); err != nil {
			return err
		}
	}
	return nil
}

func validatePairConfig(
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir string,
	sessions []sessionSpec,
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
	if target.AndroidAPI < 1 || target.AndroidAPI > 999 {
		return errors.New("Android API is invalid")
	}
	if !validSelection(target.Architecture) {
		return errors.New("architecture is invalid")
	}
	if target.PackageVersionCode == 0 {
		return errors.New("package version code is invalid")
	}
	if !validSHA256(target.PackageSHA256) {
		return errors.New("package SHA-256 is invalid")
	}
	if !validAriadneRevision(target.AriadneRevision) {
		return errors.New("Ariadne revision is invalid")
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
	if len(sessions) != 2 {
		return errors.New("session order must contain baseline and treatment")
	}
	seen := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if (session.kind != "baseline" && session.kind != "treatment") ||
			!samePersona(session.kind, session.persona, manifest) {
			return errors.New("session order is invalid")
		}
		if _, ok := seen[session.kind]; ok {
			return errors.New("session order contains a duplicate")
		}
		seen[session.kind] = struct{}{}
	}
	if len(seen) != 2 {
		return errors.New("session order must contain baseline and treatment")
	}
	return nil
}

func samePersona(kind string, persona experiment.Persona, manifest experiment.Manifest) bool {
	if kind == "baseline" {
		return mapsEqual(persona, manifest.Baseline)
	}
	return mapsEqual(persona, manifest.Treatment)
}

func mapsEqual(left, right experiment.Persona) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func runSessionWithAuth(
	ctx context.Context,
	binary string,
	target Target,
	manifest experiment.Manifest,
	outputDir, kind string,
	persona experiment.Persona,
	run commandRunner,
	now func() time.Time,
	auth *sessionAuth,
) error {
	sessionDir := filepath.Join(outputDir, kind)
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		return fmt.Errorf("%s: create session directory: %w", kind, err)
	}

	schemaVersion := sessionSchemaVersion
	manifestContractSHA256 := ""
	if manifest.TapResourceID == "" {
		schemaVersion = 4
	} else {
		manifestContractSHA256 = manifest.ContractDigest()
	}
	record := SessionRecord{
		SchemaVersion:          schemaVersion,
		Kind:                   kind,
		ManifestName:           manifest.Name,
		DeclaredVariable:       manifest.Variable,
		PersonaFields:          len(persona),
		VolatileFields:         experiment.CanonicalVolatileFields(manifest.VolatileFields),
		TapResourceID:          manifest.TapResourceID,
		ManifestContractSHA256: manifestContractSHA256,
		ADBVersion:             target.Version,
		Device:                 target.Device,
		Package:                target.Package,
		AndroidAPI:             target.AndroidAPI,
		Architecture:           target.Architecture,
		PackageVersionCode:     target.PackageVersionCode,
		PackageSHA256:          target.PackageSHA256,
		AriadneRevision:        target.AriadneRevision,
		AriadneModified:        target.AriadneModified,
		StartedAt:              now().UTC(),
	}
	if auth != nil {
		if auth.writeInput == nil || auth.challenge == nil {
			return finishSession(sessionDir, &record, now, "start", errors.New("authenticated session dependencies are required"))
		}
		if auth.order != ReplicationOrderBaselineTreatment && auth.order != ReplicationOrderTreatmentBaseline {
			return finishSession(sessionDir, &record, now, "start", errors.New("authenticated session order is invalid"))
		}
		challenge, err := auth.challenge()
		if err != nil {
			return finishSession(sessionDir, &record, now, "start", errors.New("create session challenge"))
		}
		if !validChallenge(challenge) {
			return finishSession(sessionDir, &record, now, "start", errors.New("generated session challenge is invalid"))
		}
		auth.challengeValue = challenge
		record.SchemaVersion = authenticatedSessionSchema
		record.ManifestContractSHA256 = manifest.ContractDigest()
		record.ChallengeCommitment = challengeCommitment(challenge)
		record.Role = kind
		record.Order = auth.order
		record.ProcedureSHA256 = record.ManifestContractSHA256
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
		return finishSession(
			sessionDir,
			&record,
			now,
			"reset",
			fmt.Errorf("%s: reset package: %w", kind, err),
		)
	}

	networkCollector, err := collector.Start()
	if err != nil {
		return finishSession(
			sessionDir,
			&record,
			now,
			"connect_network",
			fmt.Errorf("%s: start network collector: %w", kind, err),
		)
	}

	port := strconv.Itoa(networkCollector.Port())
	connect, _, err := runStep(
		ctx,
		run,
		now,
		"connect_network",
		binary,
		"-s", target.Device,
		"reverse", "tcp:"+port, "tcp:"+port,
	)
	record.Steps = append(record.Steps, connect)
	var sessionErr error
	failureStage := ""
	if err != nil {
		sessionErr = fmt.Errorf("%s: connect network collector: %w", kind, err)
		failureStage = "connect_network"
	}

	if sessionErr == nil {
		sessionErr = func() error {
			if auth != nil {
				inputData, err := encodeFixtureInput(fixtureInput{
					SchemaVersion:   fixtureInputSchemaVersion,
					PackageName:     target.Package,
					Challenge:       auth.challengeValue,
					Role:            kind,
					Order:           auth.order,
					ProcedureSHA256: record.ProcedureSHA256,
					CollectorPort:   networkCollector.Port(),
					Persona:         persona,
				})
				if err != nil {
					failureStage = "start"
					return fmt.Errorf("%s: encode fixture input: %w", kind, err)
				}
				if _, err := auth.writeInput(ctx, binary, inputData, fixtureInputArgs(target)...); err != nil {
					failureStage = "start"
					return fmt.Errorf("%s: write private fixture input: %w", kind, err)
				}
			}

			args := []string{
				"-s", target.Device,
				"shell", "am", "start", "-W", "-S",
				"-n", target.Package + "/.MainActivity",
			}
			if auth == nil {
				keys := make([]string, 0, len(persona))
				for key := range persona {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					args = append(args, "--es", key, persona[key])
				}
				args = append(args, "--ei", "collector_port", port)
			}

			start, _, err := runStep(ctx, run, now, "start", binary, args...)
			record.Steps = append(record.Steps, start)
			if err != nil {
				failureStage = "start"
				return fmt.Errorf("%s: start fixture: %w", kind, err)
			}

			if manifest.TapResourceID != "" {
				interact, err := runInteractionStep(
					ctx,
					run,
					now,
					binary,
					target.Device,
					manifest.TapResourceID,
				)
				record.Steps = append(record.Steps, interact)
				if err != nil {
					failureStage = "interact"
					return fmt.Errorf("%s: interact with fixture: %w", kind, err)
				}
			}

			captureNetwork := StepRecord{
				Name:      "capture_network",
				StartedAt: now().UTC(),
				Status:    "ok",
			}
			waitContext, cancel := context.WithTimeout(ctx, networkObservationTimeout)
			observation, err := networkCollector.Wait(waitContext)
			cancel()
			captureNetwork.FinishedAt = now().UTC()
			if err != nil {
				captureNetwork.Status = "error"
				captureNetwork.ExitCode = -1
			}
			record.Steps = append(record.Steps, captureNetwork)
			if err != nil {
				failureStage = "capture_network"
				return fmt.Errorf("%s: capture network: %w", kind, err)
			}
			data, err := json.MarshalIndent(observation, "", "  ")
			if err != nil {
				record.Steps[len(record.Steps)-1].Status = "error"
				record.Steps[len(record.Steps)-1].ExitCode = -1
				failureStage = "capture_network"
				return fmt.Errorf("%s: encode network observation: %w", kind, err)
			}
			data = append(data, '\n')
			artifact, err := writeArtifact(
				sessionDir,
				"observations/network.json",
				"http_request",
				"POST /observe",
				data,
			)
			if err != nil {
				record.Steps[len(record.Steps)-1].Status = "error"
				record.Steps[len(record.Steps)-1].ExitCode = -1
				failureStage = "capture_network"
				return fmt.Errorf("%s: capture network: %w", kind, err)
			}
			record.Artifacts = append(record.Artifacts, artifact)

			if auth != nil {
				if err := validateAuthenticatedObservation(observation, auth.challengeValue); err != nil {
					record.Steps[len(record.Steps)-1].Status = "error"
					record.Steps[len(record.Steps)-1].ExitCode = -1
					failureStage = "capture_network"
					record.Steps = append(record.Steps, StepRecord{
						Name:       "capture_storage",
						StartedAt:  now().UTC(),
						FinishedAt: now().UTC(),
						Status:     "error",
						ExitCode:   -1,
					})
					return fmt.Errorf("%s: validate network authentication: %w", kind, err)
				}
			}

			captureStorage, output, err := runStep(
				ctx,
				run,
				now,
				"capture_storage",
				binary,
				"-s", target.Device,
				"exec-out", "run-as", target.Package,
				"cat", "files/observation.json",
			)
			record.Steps = append(record.Steps, captureStorage)
			if err != nil {
				failureStage = "capture_storage"
				return fmt.Errorf("%s: capture storage: %w", kind, err)
			}
			if auth != nil {
				if err := validateObservationChallenge(output, auth.challengeValue); err != nil {
					record.Steps[len(record.Steps)-1].Status = "error"
					record.Steps[len(record.Steps)-1].ExitCode = -1
					failureStage = "capture_storage"
					return fmt.Errorf("%s: validate storage authentication: %w", kind, err)
				}
			}
			artifact, err = writeStorageObservation(sessionDir, output)
			if err != nil {
				record.Steps[len(record.Steps)-1].Status = "error"
				record.Steps[len(record.Steps)-1].ExitCode = -1
				failureStage = "capture_storage"
				return fmt.Errorf("%s: capture storage: %w", kind, err)
			}
			record.Artifacts = append(record.Artifacts, artifact)
			return nil
		}()
	}

	if auth != nil {
		inputCleanupContext, cancelInputCleanup := context.WithTimeout(
			context.WithoutCancel(ctx),
			networkCleanupTimeout,
		)
		cleanupErr := removeFixtureInput(inputCleanupContext, run, binary, target)
		cancelInputCleanup()
		if cleanupErr != nil {
			inputCleanupFailure := fmt.Errorf("%s: remove private fixture input: %w", kind, cleanupErr)
			sessionErr = errors.Join(sessionErr, inputCleanupFailure)
			if failureStage == "" {
				failureStage = "cleanup_input"
			}
		}
	}

	cleanupContext, cancelCleanup := context.WithTimeout(
		context.WithoutCancel(ctx),
		networkCleanupTimeout,
	)
	disconnect, _, disconnectErr := runStep(
		cleanupContext,
		run,
		now,
		"disconnect_network",
		binary,
		"-s", target.Device,
		"reverse", "--remove", "tcp:"+port,
	)
	cancelCleanup()
	record.Steps = append(record.Steps, disconnect)
	if disconnectErr != nil {
		cleanupErr := fmt.Errorf("%s: disconnect network collector: %w", kind, disconnectErr)
		sessionErr = errors.Join(sessionErr, cleanupErr)
		if failureStage == "" {
			failureStage = "disconnect_network"
		}
	}
	if err := networkCollector.Close(); err != nil {
		sessionErr = errors.Join(
			sessionErr,
			fmt.Errorf("%s: close network collector: %w", kind, err),
		)
		if failureStage == "" {
			failureStage = "disconnect_network"
		}
	}
	return finishSession(sessionDir, &record, now, failureStage, sessionErr)
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

	return writeArtifact(
		sessionDir,
		"observations/storage.json",
		"android_private_storage",
		"files/observation.json",
		data,
	)
}

func writeArtifact(
	sessionDir, relativePath, kind, source string,
	data []byte,
) (Artifact, error) {
	directory := filepath.Dir(filepath.Join(sessionDir, filepath.FromSlash(relativePath)))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create observation directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, filepath.FromSlash(relativePath)), data, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write observation: %w", err)
	}

	sum := sha256.Sum256(data)
	return Artifact{
		Kind:      kind,
		Source:    source,
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

const uiDumpPath = "/sdcard/ariadne-ui.xml"

type uiHierarchy struct {
	Nodes []uiNode `xml:"node"`
}

type uiNode struct {
	ResourceID string   `xml:"resource-id,attr"`
	Bounds     string   `xml:"bounds,attr"`
	Nodes      []uiNode `xml:"node"`
}

func runInteractionStep(
	ctx context.Context,
	run commandRunner,
	now func() time.Time,
	binary, device, resourceID string,
) (StepRecord, error) {
	step := StepRecord{
		Name:      "interact",
		StartedAt: now().UTC(),
		Status:    "ok",
	}
	finish := func(err error) (StepRecord, error) {
		step.FinishedAt = now().UTC()
		if err != nil {
			step.Status = "error"
			if step.ExitCode == 0 {
				step.ExitCode = -1
			}
		}
		return step, err
	}
	cleanup := func() error {
		cleanupContext, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			networkCleanupTimeout,
		)
		defer cancel()
		_, err := run(
			cleanupContext,
			binary,
			"-s", device,
			"shell", "rm", "--", uiDumpPath,
		)
		return err
	}

	var coordinates [2]int
	uiHierarchySHA256 := ""
	settleDeadline := time.Now().Add(uiHierarchySettleTimeout)
	for {
		if _, err := run(
			ctx,
			binary,
			"-s", device,
			"shell", "uiautomator", "dump", "--compressed", uiDumpPath,
		); err != nil {
			step.ExitCode = commandExitCode(err)
			return finish(errors.New("dump UI hierarchy"))
		}

		dump, err := run(
			ctx,
			binary,
			"-s", device,
			"shell", "cat", uiDumpPath,
		)
		if err != nil {
			_ = cleanup()
			step.ExitCode = commandExitCode(err)
			return finish(errors.New("read UI hierarchy"))
		}

		coordinates, err = tapCoordinates(dump, resourceID)
		if err == nil {
			sum := sha256.Sum256(dump)
			uiHierarchySHA256 = fmt.Sprintf("%x", sum)
		}
		cleanupErr := cleanup()
		if err == nil {
			if cleanupErr != nil {
				step.ExitCode = commandExitCode(cleanupErr)
				return finish(errors.New("remove UI hierarchy"))
			}
			step.UIHierarchySHA256 = uiHierarchySHA256
			break
		}
		if !errors.Is(err, errFixtureControlNotUnique) || time.Now().After(settleDeadline) {
			return finish(err)
		}
		if cleanupErr != nil {
			step.ExitCode = commandExitCode(cleanupErr)
			return finish(errors.New("remove UI hierarchy"))
		}
		select {
		case <-ctx.Done():
			return finish(ctx.Err())
		case <-time.After(uiHierarchyRetryInterval):
		}
	}

	if _, err := run(
		ctx,
		binary,
		"-s", device,
		"shell", "input", "tap",
		strconv.Itoa(coordinates[0]),
		strconv.Itoa(coordinates[1]),
	); err != nil {
		step.ExitCode = commandExitCode(err)
		return finish(errors.New("tap fixture control"))
	}
	return finish(nil)
}

func tapCoordinates(data []byte, resourceID string) ([2]int, error) {
	if resourceID == "" || len(data) > 256<<10 {
		return [2]int{}, errors.New("UI hierarchy is invalid")
	}
	var hierarchy uiHierarchy
	decoder := xml.NewDecoder(io.LimitReader(bytes.NewReader(data), 256<<10+1))
	if err := decoder.Decode(&hierarchy); err != nil {
		return [2]int{}, errors.New("UI hierarchy is invalid")
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return [2]int{}, errors.New("UI hierarchy is invalid")
	}
	var match uiNode
	count := 0
	var visit func([]uiNode)
	visit = func(nodes []uiNode) {
		for index := range nodes {
			node := nodes[index]
			if node.ResourceID == resourceID {
				count++
				if count == 1 {
					match = node
				}
			}
			visit(node.Nodes)
		}
	}
	visit(hierarchy.Nodes)
	if count != 1 {
		return [2]int{}, errFixtureControlNotUnique
	}
	return parseBounds(match.Bounds)
}

func parseBounds(value string) ([2]int, error) {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '[' || r == ']' || r == ','
	})
	if len(parts) != 4 {
		return [2]int{}, errors.New("fixture control bounds are invalid")
	}
	values := make([]int, len(parts))
	for index, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 || parsed > 10000 {
			return [2]int{}, errors.New("fixture control bounds are invalid")
		}
		values[index] = parsed
	}
	if values[2] <= values[0] || values[3] <= values[1] {
		return [2]int{}, errors.New("fixture control bounds are invalid")
	}
	return [2]int{(values[0] + values[2]) / 2, (values[1] + values[3]) / 2}, nil
}

func finishSession(
	sessionDir string,
	record *SessionRecord,
	now func() time.Time,
	failureStage string,
	sessionErr error,
) error {
	if sessionErr == nil {
		record.Status = sessionStatusComplete
		record.FailureStage = ""
	} else {
		if !ValidFailureStage(failureStage) {
			return errors.New("record incomplete session: failure stage is invalid")
		}
		record.Status = sessionStatusIncomplete
		record.FailureStage = failureStage
	}
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

// ValidFailureStage reports whether stage is a bounded session failure category.
func ValidFailureStage(stage string) bool {
	switch stage {
	case "reset",
		"connect_network",
		"start",
		"interact",
		"capture_network",
		"capture_storage",
		"cleanup_input",
		"disconnect_network":
		return true
	default:
		return false
	}
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
