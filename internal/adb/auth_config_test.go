package adb

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/experiment"
)

func TestAuthenticatedPairRequiresInputDependenciesAndControl(t *testing.T) {
	manifest := sessionManifest()
	sessions := orderedSessions(ReplicationOrderBaselineTreatment, manifest)
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	if err := runPairWithOrderAndAuth(
		context.Background(),
		"adb",
		sessionTarget(),
		manifest,
		filepath.Join(t.TempDir(), "run"),
		sessions,
		run,
		&sessionAuthDependencies{},
		sequenceClock(),
	); err == nil || !strings.Contains(err.Error(), "dependencies are required") {
		t.Fatalf("missing authenticated dependencies error = %v", err)
	}

	manifest.SchemaVersion = 2
	dependencies := &sessionAuthDependencies{
		writeInput: func(context.Context, string, []byte, ...string) ([]byte, error) { return nil, nil },
		challenge:  func() (string, error) { return strings.Repeat("a", 64), nil },
	}
	if err := runPairWithOrderAndAuth(
		context.Background(),
		"adb",
		sessionTarget(),
		manifest,
		filepath.Join(t.TempDir(), "run"),
		sessions,
		run,
		dependencies,
		sequenceClock(),
	); err == nil || !strings.Contains(err.Error(), "declared fixture control") {
		t.Fatalf("missing authenticated control error = %v", err)
	}
}

func TestFixtureInputEncodingAndChallengeBoundaries(t *testing.T) {
	if validChallenge(strings.Repeat("g", 64)) {
		t.Fatal("validChallenge() accepted a non-hex challenge")
	}
	if _, err := fixtureInputChallenge([]byte(strings.Repeat("x", maxFixtureInputBytes+1))); err == nil {
		t.Fatal("fixtureInputChallenge() accepted oversized input")
	}
	if err := validateObservationChallenge(
		[]byte(`{"challenge":"`+strings.Repeat("a", 64)+`"}`),
		"not-a-challenge",
	); err == nil || !strings.Contains(err.Error(), "expected challenge is invalid") {
		t.Fatalf("validateObservationChallenge() error = %v", err)
	}

	input := fixtureInput{
		SchemaVersion:   fixtureInputSchemaVersion,
		PackageName:     "dev.ariadne.fixture",
		Challenge:       strings.Repeat("a", 64),
		Role:            "baseline",
		Order:           ReplicationOrderBaselineTreatment,
		ProcedureSHA256: strings.Repeat("b", 64),
		CollectorPort:   43210,
		Persona:         make(map[string]string, 64),
	}
	for index := 0; index < 64; index++ {
		input.Persona["field"+strings.Repeat("x", index)] = strings.Repeat("v", 1024)
	}
	if _, err := encodeFixtureInput(input); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("encodeFixtureInput() error = %v, want size limit", err)
	}
}

func TestMapsEqualRejectsMismatchedValues(t *testing.T) {
	if mapsEqual(experiment.Persona{"field": "one"}, experiment.Persona{"field": "two"}) {
		t.Fatal("mapsEqual() accepted mismatched values")
	}
	if mapsEqual(experiment.Persona{"field": "one"}, experiment.Persona{}) {
		t.Fatal("mapsEqual() accepted mismatched lengths")
	}
}
