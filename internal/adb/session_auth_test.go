package adb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/experiment"
)

func TestRunPairWithAuthenticatedInputBoundary(t *testing.T) {
	manifest := sessionManifest()
	manifest.SchemaVersion = 3
	manifest.TapResourceID = "dev.ariadne.fixture:id/observe_button"
	target := sessionTarget()
	challenges := []string{strings.Repeat("0123456789abcdef", 4), strings.Repeat("fedcba9876543210", 4)}
	challengeIndex := 0
	currentInput := fixtureInput{}
	var inputDocuments [][]byte
	var starts [][]string
	cleanupCalls := 0

	ui := []byte(`<hierarchy><node resource-id="dev.ariadne.fixture:id/observe_button" bounds="[100,200][300,400]" /></hierarchy>`)

	writeInput := func(_ context.Context, _ string, data []byte, args ...string) ([]byte, error) {
		var input fixtureInput
		if err := json.Unmarshal(data, &input); err != nil {
			return nil, err
		}
		if err := validateFixtureInput(input); err != nil {
			return nil, err
		}
		currentInput = input
		inputDocuments = append(inputDocuments, append([]byte(nil), data...))
		if strings.Join(args, " ") != "-s emulator-5554 shell -T run-as dev.ariadne.fixture sh -c 'cat > files/ariadne-input.json'" {
			return nil, fmt.Errorf("unexpected private input command: %v", args)
		}
		return nil, nil
	}

	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 3 && args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if len(args) > 2 && args[2] == "reverse" {
			return nil, nil
		}
		if len(args) > 3 && args[3] == "am" {
			starts = append(starts, append([]string(nil), args...))
			body := []byte(fmt.Sprintf(
				`{"schema_version":1,"challenge":"%s","region":"us-east","request_id":"request-%s","variant":"standard"}`,
				currentInput.Challenge,
				currentInput.Role,
			))
			response, err := http.Post(
				"http://127.0.0.1:"+fmt.Sprint(currentInput.CollectorPort)+"/observe",
				"application/json",
				strings.NewReader(string(body)),
			)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				return nil, fmt.Errorf("collector status %d", response.StatusCode)
			}
			return []byte("Status: ok\n"), nil
		}
		if len(args) > 3 && args[3] == "uiautomator" {
			return ui, nil
		}
		if len(args) > 3 && args[3] == "cat" {
			return ui, nil
		}
		if len(args) > 3 && args[3] == "input" {
			return nil, nil
		}

		if len(args) > 2 && args[2] == "exec-out" {
			return []byte(fmt.Sprintf(
				`{"schema_version":1,"challenge":"%s","region":"us-east","request_id":"request-%s","variant":"standard"}`,
				currentInput.Challenge,
				currentInput.Role,
			)), nil
		}
		if len(args) > 3 && args[3] == "run-as" && contains(args, "rm") {
			cleanupCalls++
		}
		return []byte("Status: ok\n"), nil
	}

	challenge := func() (string, error) {
		value := challenges[challengeIndex]
		challengeIndex++
		return value, nil
	}
	outputDir := filepath.Join(t.TempDir(), "run")
	if err := runPairWithAuthenticated(
		context.Background(),
		"adb",
		target,
		manifest,
		outputDir,
		run,
		writeInput,
		challenge,
		sequenceClock(),
	); err != nil {
		t.Fatalf("runPairWithAuthenticated() error = %v", err)
	}

	if len(inputDocuments) != 2 || cleanupCalls != 2 || len(starts) != 2 {
		t.Fatalf("input documents = %d, cleanup calls = %d, starts = %d", len(inputDocuments), cleanupCalls, len(starts))
	}
	for _, start := range starts {
		if contains(start, "run-as") || !contains(start, "am") || !contains(start, "-S") {
			t.Fatalf("authenticated start did not use the shell-authorized launcher: %v", start)
		}
	}

	for index, start := range starts {
		text := strings.Join(start, " ")
		for _, secret := range []string{
			manifest.Baseline["email"],
			manifest.Treatment["email"],
			challenges[index],
		} {
			if strings.Contains(text, secret) {
				t.Fatalf("start command exposed %q: %v", secret, start)
			}
		}
		if contains(start, "collector_port") {
			t.Fatalf("start command exposed collector port: %v", start)
		}
	}

	for index, kind := range []string{"baseline", "treatment"} {
		data, err := os.ReadFile(filepath.Join(outputDir, kind, "session.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `"schema_version": 8`) ||
			!strings.Contains(text, `"role": "`+kind+`"`) ||
			!strings.Contains(text, `"order": "baseline-treatment"`) ||
			!strings.Contains(text, `"procedure_sha256": "`+manifest.ContractDigest()+`"`) ||
			strings.Contains(text, challenges[index]) ||
			strings.Contains(text, manifest.Baseline["email"]) ||
			strings.Contains(text, manifest.Treatment["email"]) {
			t.Fatalf("authenticated session metadata exposed unsafe data: %s", text)
		}
	}
}

func TestValidateObservationChallengeRejectsUnverifiableEvidence(t *testing.T) {
	expected := strings.Repeat("a", 64)
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing", body: `{"schema_version":1}`, want: "missing"},
		{name: "mismatch", body: `{"challenge":"` + strings.Repeat("b", 64) + `"}`, want: "does not match"},
		{name: "invalid", body: `{"challenge":"not-a-challenge"}`, want: "invalid"},
		{name: "duplicate", body: `{"challenge":"` + expected + `","challenge":"` + expected + `"}`, want: "invalid JSON"},
		{name: "trailing", body: `{"challenge":"` + expected + `"} {}`, want: "trailing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateObservationChallenge([]byte(test.body), expected)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateObservationChallenge() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestFixtureInputChallengeRejectsMalformedInput(t *testing.T) {
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
	data, err := encodeFixtureInput(valid)
	if err != nil {
		t.Fatal(err)
	}
	if challenge, err := fixtureInputChallenge(data); err != nil || challenge != valid.Challenge {
		t.Fatalf("fixtureInputChallenge() = %q, %v", challenge, err)
	}
	for _, malformed := range [][]byte{
		[]byte(strings.Replace(
			string(data),
			`"challenge":"`+valid.Challenge+`","role"`,
			`"challenge":"`+valid.Challenge+`","challenge":"`+valid.Challenge+`","role"`,
			1)),
		[]byte(strings.Replace(string(data), `"persona":`, `"unexpected":"value","persona":`, 1)),
		[]byte(strings.Replace(string(data), "\n", "\n{}", 1)),
	} {
		if _, err := fixtureInputChallenge(malformed); err == nil {
			t.Fatalf("fixtureInputChallenge() accepted malformed input: %s", malformed)
		}
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
