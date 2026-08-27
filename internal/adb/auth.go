package adb

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	fixtureInputSchemaVersion  = 1
	fixtureInputDirectory      = "files"
	fixtureInputPath           = "files/ariadne-input.json"
	maxFixtureInputBytes       = 32 << 10
	challengeSize              = 32
	authenticatedSessionSchema = 8
)

type fixtureInput struct {
	SchemaVersion   int                `json:"schema_version"`
	PackageName     string             `json:"package_name"`
	Challenge       string             `json:"challenge"`
	Role            string             `json:"role"`
	Order           string             `json:"order"`
	ProcedureSHA256 string             `json:"procedure_sha256"`
	CollectorPort   int                `json:"collector_port"`
	Persona         experiment.Persona `json:"persona"`
}

type inputCommandRunner func(context.Context, string, []byte, ...string) ([]byte, error)
type challengeGenerator func() (string, error)

type sessionAuth struct {
	order          string
	writeInput     inputCommandRunner
	challenge      challengeGenerator
	challengeValue string
}

type sessionAuthDependencies struct {
	order      string
	writeInput inputCommandRunner
	challenge  challengeGenerator
}

func (dependencies sessionAuthDependencies) forSession() *sessionAuth {
	return &sessionAuth{
		order:      dependencies.order,
		writeInput: dependencies.writeInput,
		challenge:  dependencies.challenge,
	}
}

func newChallenge() (string, error) {
	value := make([]byte, challengeSize)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", errors.New("create session challenge")
	}
	return hex.EncodeToString(value), nil
}

func challengeCommitment(challenge string) string {
	digest := sha256.Sum256([]byte(challenge))
	return hex.EncodeToString(digest[:])
}

func encodeFixtureInput(input fixtureInput) ([]byte, error) {
	if err := validateFixtureInput(input); err != nil {
		return nil, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, errors.New("encode fixture input")
	}
	if len(data) > maxFixtureInputBytes {
		return nil, fmt.Errorf("fixture input exceeds %d-byte limit", maxFixtureInputBytes)
	}
	return append(data, '\n'), nil
}

func validateFixtureInput(input fixtureInput) error {
	if input.SchemaVersion != fixtureInputSchemaVersion {
		return errors.New("fixture input schema_version is invalid")
	}
	if !validSelection(input.PackageName) {
		return errors.New("fixture input package_name is invalid")
	}
	if !validChallenge(input.Challenge) {
		return errors.New("fixture input challenge is invalid")
	}
	if input.Role != "baseline" && input.Role != "treatment" {
		return errors.New("fixture input role is invalid")
	}
	if input.Order != ReplicationOrderBaselineTreatment && input.Order != ReplicationOrderTreatmentBaseline {
		return errors.New("fixture input order is invalid")
	}
	if !validSHA256(input.ProcedureSHA256) {
		return errors.New("fixture input procedure_sha256 is invalid")
	}
	if input.CollectorPort < 1 || input.CollectorPort > 65_535 {
		return errors.New("fixture input collector_port is invalid")
	}
	if len(input.Persona) == 0 || len(input.Persona) > 64 {
		return errors.New("fixture input persona is invalid")
	}
	if err := validatePersonaForShell(input.Persona); err != nil {
		return fmt.Errorf("fixture input persona: %w", err)
	}
	return nil
}

func validChallenge(value string) bool {
	if len(value) != challengeSize*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func runInputCommand(ctx context.Context, binary string, input []byte, args ...string) ([]byte, error) {
	if len(input) > maxFixtureInputBytes {
		return nil, errors.New("fixture input exceeds 32768-byte limit")
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin = bytes.NewReader(input)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, errors.New("open adb input output")
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, maxOutputBytes+1))
	if len(output) > maxOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("adb output exceeds 65536-byte limit")
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("read adb output: %w", readErr)
	}
	if err := command.Wait(); err != nil {
		return nil, err
	}
	return output, nil
}

func fixtureInputArgs(target Target) []string {
	return []string{
		"-s", target.Device,
		"shell", "-T", "run-as", target.Package, "sh", "-c",
		"'cat > " + fixtureInputPath + "'",
	}
}

func ensureFixtureInputDirectory(ctx context.Context, run commandRunner, binary string, target Target) error {
	_, err := run(
		ctx,
		binary,
		"-s", target.Device,
		"shell", "run-as", target.Package,
		"mkdir", "-p", fixtureInputDirectory,
	)
	return err
}

func verifyFixtureInput(ctx context.Context, run commandRunner, binary string, target Target) error {
	_, err := run(
		ctx,
		binary,
		"-s", target.Device,
		"shell", "run-as", target.Package,
		"ls", "-l", fixtureInputPath,
	)
	return err
}

func removeFixtureInput(ctx context.Context, run commandRunner, binary string, target Target) error {
	_, err := run(
		ctx,
		binary,
		"-s", target.Device,
		"shell", "run-as", target.Package,
		"rm", "-f", fixtureInputPath,
	)
	return err
}

func validateAuthenticatedObservation(observation collector.Observation, expected string) error {
	body, err := base64.StdEncoding.Strict().DecodeString(observation.BodyBase64)
	if err != nil {
		return errors.New("network challenge evidence is invalid")
	}
	return validateObservationChallenge(body, expected)
}

func validateObservationChallenge(data []byte, expected string) error {
	if !validChallenge(expected) {
		return errors.New("expected challenge is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return errors.New("challenge evidence has invalid JSON structure")
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&fields); err != nil {
		return errors.New("challenge evidence is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("challenge evidence has trailing data")
	}
	rawChallenge, ok := fields["challenge"]
	if !ok {
		return errors.New("challenge evidence is missing")
	}
	var challenge string
	if err := json.Unmarshal(rawChallenge, &challenge); err != nil || !validChallenge(challenge) {
		return errors.New("challenge evidence is invalid")
	}
	if challenge != expected {
		return errors.New("challenge evidence does not match session")
	}
	return nil
}

func fixtureInputChallenge(data []byte) (string, error) {
	if len(data) > maxFixtureInputBytes {
		return "", errors.New("fixture input exceeds 32768-byte limit")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return "", errors.New("fixture input has invalid JSON structure")
	}
	var input fixtureInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return "", errors.New("fixture input is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("fixture input has trailing data")
	}
	if err := validateFixtureInput(input); err != nil {
		return "", err
	}
	return input.Challenge, nil
}
