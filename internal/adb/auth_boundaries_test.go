package adb

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/experiment"
)

func TestNewChallengeAndAuthenticatedObservation(t *testing.T) {
	challenge, err := newChallenge()
	if err != nil {
		t.Fatal(err)
	}
	if !validChallenge(challenge) {
		t.Fatalf("newChallenge() = %q, want a valid challenge", challenge)
	}

	body := []byte(`{"challenge":"` + challenge + `"}`)
	encoded := base64.StdEncoding.EncodeToString(body)
	if err := validateAuthenticatedObservation(collector.Observation{BodyBase64: encoded}, challenge); err != nil {
		t.Fatalf("validateAuthenticatedObservation() error = %v", err)
	}
	if err := validateAuthenticatedObservation(collector.Observation{BodyBase64: "not-base64"}, challenge); err == nil {
		t.Fatal("validateAuthenticatedObservation() error = nil for malformed base64")
	}
}

func TestValidateFixtureInputRejectsBoundaryValues(t *testing.T) {
	valid := fixtureInput{
		SchemaVersion:   fixtureInputSchemaVersion,
		PackageName:     "dev.ariadne.fixture",
		Challenge:       strings.Repeat("a", 64),
		Role:            "baseline",
		Order:           ReplicationOrderBaselineTreatment,
		ProcedureSHA256: strings.Repeat("b", 64),
		CollectorPort:   43210,
		Persona:         experiment.Persona{"email": "baseline@example.invalid", "region": "us-east"},
	}
	tooManyPersonaFields := make(experiment.Persona, 65)
	for index := 0; index < 65; index++ {
		tooManyPersonaFields["field"+strings.Repeat("x", index)] = "value"
	}
	tests := []struct {
		name   string
		mutate func(*fixtureInput)
	}{
		{name: "schema", mutate: func(input *fixtureInput) { input.SchemaVersion = 0 }},
		{name: "package", mutate: func(input *fixtureInput) { input.PackageName = "" }},
		{name: "package shell punctuation", mutate: func(input *fixtureInput) { input.PackageName = "dev.ariadne.fixture;id" }},
		{name: "challenge", mutate: func(input *fixtureInput) { input.Challenge = strings.Repeat("a", 63) }},
		{name: "role", mutate: func(input *fixtureInput) { input.Role = "observer" }},
		{name: "order", mutate: func(input *fixtureInput) { input.Order = "random" }},
		{name: "procedure", mutate: func(input *fixtureInput) { input.ProcedureSHA256 = "not-a-digest" }},
		{name: "port-low", mutate: func(input *fixtureInput) { input.CollectorPort = 0 }},
		{name: "port-high", mutate: func(input *fixtureInput) { input.CollectorPort = 65_536 }},
		{name: "persona-empty", mutate: func(input *fixtureInput) { input.Persona = nil }},
		{name: "persona-large", mutate: func(input *fixtureInput) { input.Persona = tooManyPersonaFields }},
		{name: "persona-shell", mutate: func(input *fixtureInput) { input.Persona = experiment.Persona{"email": "bad\nvalue"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if err := validateFixtureInput(input); err == nil {
				t.Fatal("validateFixtureInput() error = nil")
			}
		})
	}
}

func TestRunInputCommand(t *testing.T) {
	t.Setenv("ARIADNE_INPUT_HELPER", "1")
	input := []byte("private fixture input")
	output, err := runInputCommand(
		context.Background(),
		os.Args[0],
		input,
		"-test.run=TestRunInputCommandHelper",
	)
	if err != nil {
		t.Fatalf("runInputCommand() error = %v", err)
	}
	if string(output) != string(input) {
		t.Fatalf("runInputCommand() output = %q, want %q", output, input)
	}
	if _, err := runInputCommand(context.Background(), os.Args[0]+".missing", input); err == nil {
		t.Fatal("runInputCommand() error = nil for missing binary")
	}
	if _, err := runInputCommand(context.Background(), os.Args[0], []byte(strings.Repeat("x", maxFixtureInputBytes+1))); err == nil {
		t.Fatal("runInputCommand() error = nil for oversized input")
	}
}

func TestRunInputCommandHelper(t *testing.T) {
	if os.Getenv("ARIADNE_INPUT_HELPER") != "1" {
		return
	}
	if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestAuthenticatedReplicationDispatchRejectsInvalidCount(t *testing.T) {
	manifest := sessionManifest()
	manifest.SchemaVersion = 3
	manifest.TapResourceID = "dev.ariadne.fixture:id/observe_button"
	run := func(context.Context, string, ...string) ([]byte, error) { return nil, nil }
	writeInput := func(context.Context, string, []byte, ...string) ([]byte, error) { return nil, nil }
	challenge := func() (string, error) { return strings.Repeat("a", 64), nil }
	if err := runReplicatedWithAuthenticated(
		context.Background(),
		"adb",
		sessionTarget(),
		manifest,
		t.TempDir(),
		0,
		run,
		writeInput,
		challenge,
		sequenceClock(),
	); err == nil {
		t.Fatal("runReplicatedWithAuthenticated() error = nil")
	}
	if err := RunReplicated(context.Background(), "adb", sessionTarget(), manifest, t.TempDir(), 0); err == nil {
		t.Fatal("RunReplicated() error = nil")
	}
}
