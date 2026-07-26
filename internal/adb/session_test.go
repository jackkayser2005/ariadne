package adb

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/experiment"
)

func TestRunPair(t *testing.T) {
	manifest := sessionManifest()
	target := sessionTarget()
	var calls [][]string
	captures := [][]byte{
		[]byte(`{"schema_version":1,"region":"us-east","variant":"standard"}`),
		[]byte(`{"schema_version":1,"region":"us-east","variant":"personalized"}`),
	}
	run := func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			return nil, nil
		}
		if args[3] == "am" {
			if err := postFixtureObservation(args, captures[0]); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if args[2] == "exec-out" {
			output := captures[0]
			captures = captures[1:]
			return output, nil
		}
		return []byte("Status: ok\n"), nil
	}
	now := sequenceClock()
	outputDir := filepath.Join(t.TempDir(), "run")

	err := runPairWith(
		context.Background(),
		"adb",
		target,
		manifest,
		outputDir,
		run,
		now,
	)
	if err != nil {
		t.Fatalf("runPairWith() error = %v", err)
	}

	wantCalls := [][]string{
		{"adb", "-s", "emulator-5554", "shell", "pm", "clear", "dev.ariadne.fixture"},
		{"adb", "-s", "emulator-5554", "reverse", "tcp:<port>", "tcp:<port>"},
		{
			"adb", "-s", "emulator-5554", "shell", "am", "start", "-W", "-S",
			"-n", "dev.ariadne.fixture/.MainActivity",
			"--es", "email", "baseline@example.invalid",
			"--es", "region", "us-east",
			"--ei", "collector_port", "<port>",
		},
		{
			"adb", "-s", "emulator-5554", "exec-out", "run-as",
			"dev.ariadne.fixture", "cat", "files/observation.json",
		},
		{"adb", "-s", "emulator-5554", "reverse", "--remove", "tcp:<port>"},
		{"adb", "-s", "emulator-5554", "shell", "pm", "clear", "dev.ariadne.fixture"},
		{"adb", "-s", "emulator-5554", "reverse", "tcp:<port>", "tcp:<port>"},
		{
			"adb", "-s", "emulator-5554", "shell", "am", "start", "-W", "-S",
			"-n", "dev.ariadne.fixture/.MainActivity",
			"--es", "email", "treatment@example.invalid",
			"--es", "region", "us-east",
			"--ei", "collector_port", "<port>",
		},
		{
			"adb", "-s", "emulator-5554", "exec-out", "run-as",
			"dev.ariadne.fixture", "cat", "files/observation.json",
		},
		{"adb", "-s", "emulator-5554", "reverse", "--remove", "tcp:<port>"},
	}
	if normalized := normalizeNetworkPorts(calls); !reflect.DeepEqual(normalized, wantCalls) {
		t.Fatalf("runPairWith() calls = %#v, want %#v", calls, wantCalls)
	}

	for _, kind := range []string{"baseline", "treatment"} {
		observation, err := os.ReadFile(
			filepath.Join(outputDir, kind, "observations", "storage.json"),
		)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(observation)
		network, err := os.ReadFile(
			filepath.Join(outputDir, kind, "observations", "network.json"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(network), base64.StdEncoding.EncodeToString(observation)) {
			t.Fatalf("%s network observation = %s", kind, network)
		}

		data, err := os.ReadFile(filepath.Join(outputDir, kind, "session.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `"schema_version": 4`) ||
			!strings.Contains(text, `"kind": "`+kind+`"`) ||
			!strings.Contains(text, `"volatile_fields": [`) ||
			!strings.Contains(text, `"request_id"`) ||
			!strings.Contains(text, `"status": "complete"`) ||
			strings.Contains(text, `"failure_stage"`) ||
			!strings.Contains(text, `"exit_code": 0`) ||
			!strings.Contains(text, `"source": "files/observation.json"`) ||
			!strings.Contains(text, `"source": "POST /observe"`) ||
			!strings.Contains(text, `"sha256": "`+fmt.Sprintf("%x", sum)+`"`) {
			t.Fatalf("%s session metadata = %s", kind, text)
		}
		for _, value := range []string{
			"baseline@example.invalid",
			"treatment@example.invalid",
			"us-east",
		} {
			if strings.Contains(text, value) {
				t.Fatalf("%s session metadata exposed persona value %q", kind, value)
			}
		}
	}
}

func TestRunPairRejectsInvalidStorageObservation(t *testing.T) {
	tests := []struct {
		name   string
		output []byte
		want   string
	}{
		{name: "empty", want: "empty observation"},
		{name: "malformed", output: []byte(`{"variant":`), want: "valid JSON object"},
		{name: "non-object", output: []byte(`["standard"]`), want: "valid JSON object"},
		{
			name:   "oversized",
			output: []byte(`{"value":"` + strings.Repeat("x", maxOutputBytes) + `"}`),
			want:   "exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "run")
			calls := 0
			run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
				calls++
				if args[3] == "pm" {
					return []byte("Success\n"), nil
				}
				if args[2] == "reverse" {
					return nil, nil
				}
				if args[3] == "am" {
					if err := postFixtureObservation(args, []byte(`{}`)); err != nil {
						return nil, err
					}
					return []byte("Status: ok\n"), nil
				}
				if args[2] == "exec-out" {
					return test.output, nil
				}
				return []byte("Status: ok\n"), nil
			}

			err := runPairWith(
				context.Background(),
				"adb",
				sessionTarget(),
				sessionManifest(),
				outputDir,
				run,
				sequenceClock(),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runPairWith() error = %v, want containing %q", err, test.want)
			}
			if calls != 5 || directoryExists(filepath.Join(outputDir, "treatment")) {
				t.Fatalf("runPairWith() calls = %d, treatment directory exists", calls)
			}

			data, readErr := os.ReadFile(filepath.Join(outputDir, "baseline", "session.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !strings.Contains(string(data), `"name": "capture_storage"`) ||
				!strings.Contains(string(data), `"status": "error"`) ||
				!strings.Contains(string(data), `"status": "incomplete"`) ||
				!strings.Contains(string(data), `"failure_stage": "capture_storage"`) {
				t.Fatalf("failure metadata = %s", data)
			}
		})
	}
}

func TestRunPairRecordsStorageCaptureFailure(t *testing.T) {
	const secret = "do-not-print-captured-output"
	outputDir := filepath.Join(t.TempDir(), "run")
	calls := 0
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		if args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			return nil, nil
		}
		if args[3] == "am" {
			if err := postFixtureObservation(args, []byte(`{}`)); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if args[2] == "exec-out" {
			return []byte(secret), errors.New("exit status 1")
		}
		return []byte("Status: ok\n"), nil
	}

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		run,
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "capture storage") {
		t.Fatalf("runPairWith() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("runPairWith() exposed observation output: %v", err)
	}
	if calls != 5 || directoryExists(filepath.Join(outputDir, "treatment")) {
		t.Fatalf("runPairWith() calls = %d, treatment directory exists", calls)
	}
}

func TestRunPairCleansNetworkMappingAfterMissingObservation(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "run")
	var calls [][]string
	run := func(ctx context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			if args[3] == "--remove" && ctx.Err() != nil {
				t.Fatalf("cleanup context error = %v", ctx.Err())
			}
			return nil, nil
		}
		return []byte("Status: ok\n"), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := runPairWith(
		ctx,
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		run,
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "capture network") {
		t.Fatalf("runPairWith() error = %v", err)
	}
	if len(calls) != 4 ||
		calls[len(calls)-1][2] != "reverse" ||
		calls[len(calls)-1][3] != "--remove" {
		t.Fatalf("runPairWith() calls = %#v", calls)
	}

	data, readErr := os.ReadFile(filepath.Join(outputDir, "baseline", "session.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	text := string(data)
	if !strings.Contains(text, `"name": "capture_network"`) ||
		!strings.Contains(text, `"name": "disconnect_network"`) ||
		!strings.Contains(text, `"status": "error"`) ||
		!strings.Contains(text, `"status": "incomplete"`) ||
		!strings.Contains(text, `"failure_stage": "capture_network"`) {
		t.Fatalf("failure metadata = %s", data)
	}
}

func TestRunPairCleansNetworkMappingAfterConnectFailure(t *testing.T) {
	const secret = "do-not-print-reverse-output"
	outputDir := filepath.Join(t.TempDir(), "run")
	var calls [][]string
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" && args[3] != "--remove" {
			return []byte(secret), errors.New("exit status 1")
		}
		if args[2] == "reverse" && args[3] == "--remove" {
			return nil, nil
		}
		t.Fatal("fixture started after reverse failure")
		return nil, nil
	}

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		run,
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "connect network collector") {
		t.Fatalf("runPairWith() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("runPairWith() exposed reverse output: %v", err)
	}
	if len(calls) != 3 ||
		calls[len(calls)-1][2] != "reverse" ||
		calls[len(calls)-1][3] != "--remove" {
		t.Fatalf("runPairWith() calls = %#v", calls)
	}
}

func TestRunPairReportsNetworkCleanupFailure(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "run")
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			if args[3] == "--remove" {
				return nil, errors.New("exit status 1")
			}
			return nil, nil
		}
		if args[3] == "am" {
			if err := postFixtureObservation(args, []byte(`{}`)); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if args[2] == "exec-out" {
			return []byte(`{}`), nil
		}
		return nil, errors.New("unexpected command")
	}

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		run,
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "disconnect network collector") {
		t.Fatalf("runPairWith() error = %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(outputDir, "baseline", "session.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"name": "disconnect_network"`) ||
		!strings.Contains(string(data), `"status": "error"`) ||
		!strings.Contains(string(data), `"status": "incomplete"`) ||
		!strings.Contains(string(data), `"failure_stage": "disconnect_network"`) {
		t.Fatalf("failure metadata = %s", data)
	}
}

func TestRunPairRejectsUnsafePersonaBeforeCreatingOutput(t *testing.T) {
	manifest := sessionManifest()
	const secret = "value with spaces"
	manifest.Treatment["email"] = secret
	outputDir := filepath.Join(t.TempDir(), "run")
	called := false

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		manifest,
		outputDir,
		func(context.Context, string, ...string) ([]byte, error) {
			called = true
			return nil, nil
		},
		time.Now,
	)
	if err == nil || !strings.Contains(err.Error(), `field "email"`) {
		t.Fatalf("runPairWith() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("runPairWith() exposed persona value: %v", err)
	}
	if called || directoryExists(outputDir) {
		t.Fatal("runPairWith() changed external state for unsafe input")
	}
}

func TestRunPairRejectsInvalidConfiguration(t *testing.T) {
	validTarget := sessionTarget()
	invalidAPI := validTarget
	invalidAPI.AndroidAPI = 0
	invalidArchitecture := validTarget
	invalidArchitecture.Architecture = ""
	invalidVersionCode := validTarget
	invalidVersionCode.PackageVersionCode = 0
	invalidPackageDigest := validTarget
	invalidPackageDigest.PackageSHA256 = "invalid"
	invalidRevision := validTarget
	invalidRevision.AriadneRevision = "invalid"
	tests := []struct {
		name      string
		binary    string
		target    Target
		manifest  experiment.Manifest
		outputDir string
		want      string
	}{
		{
			name:      "binary",
			target:    validTarget,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "binary",
		},
		{
			name:      "device",
			binary:    "adb",
			target:    Target{Version: "1.0.41", Package: "dev.ariadne.fixture"},
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "device",
		},
		{
			name:      "package",
			binary:    "adb",
			target:    Target{Version: "1.0.41", Device: "emulator-5554"},
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "package",
		},
		{
			name:      "version",
			binary:    "adb",
			target:    Target{Device: "emulator-5554", Package: "dev.ariadne.fixture"},
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "version",
		},
		{
			name:      "Android API",
			binary:    "adb",
			target:    invalidAPI,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "Android API",
		},
		{
			name:      "architecture",
			binary:    "adb",
			target:    invalidArchitecture,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "architecture",
		},
		{
			name:      "package version",
			binary:    "adb",
			target:    invalidVersionCode,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "package version",
		},
		{
			name:      "package digest",
			binary:    "adb",
			target:    invalidPackageDigest,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "SHA-256",
		},
		{
			name:      "Ariadne revision",
			binary:    "adb",
			target:    invalidRevision,
			manifest:  sessionManifest(),
			outputDir: "run",
			want:      "revision",
		},
		{
			name:      "manifest",
			binary:    "adb",
			target:    validTarget,
			outputDir: "run",
			want:      "manifest",
		},
		{
			name:     "output",
			binary:   "adb",
			target:   validTarget,
			manifest: sessionManifest(),
			want:     "output",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RunPair(
				context.Background(),
				test.binary,
				test.target,
				test.manifest,
				test.outputDir,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunPair() error = %v, want containing %q", err, test.want)
			}
		})
	}

	t.Run("oversized persona value", func(t *testing.T) {
		manifest := sessionManifest()
		manifest.Baseline["email"] = strings.Repeat("a", 1025)
		err := RunPair(context.Background(), "adb", validTarget, manifest, "run")
		if err == nil || !strings.Contains(err.Error(), `field "email"`) {
			t.Fatalf("RunPair() error = %v", err)
		}
	})
}

func TestRunPairRefusesExistingOutput(t *testing.T) {
	outputDir := t.TempDir()

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("adb invoked for existing output")
			return nil, nil
		},
		time.Now,
	)
	if err == nil || !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("runPairWith() error = %v", err)
	}
}

func TestRunPairReportsOutputParentFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := RunPair(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		filepath.Join(parent, "run"),
	)
	if err == nil || !strings.Contains(err.Error(), "output parent") {
		t.Fatalf("RunPair() error = %v", err)
	}
}

func TestRunPairRecordsFailureAndStops(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "run")
	calls := 0
	const secret = "raw-secret-output"
	run := func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return []byte(secret), errors.New("exit status 7")
	}

	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		run,
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "reset package") {
		t.Fatalf("runPairWith() error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("runPairWith() exposed command output: %v", err)
	}
	if calls != 1 || directoryExists(filepath.Join(outputDir, "treatment")) {
		t.Fatalf("runPairWith() calls = %d, treatment directory exists", calls)
	}

	data, readErr := os.ReadFile(filepath.Join(outputDir, "baseline", "session.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), `"status": "error"`) ||
		!strings.Contains(string(data), `"status": "incomplete"`) ||
		!strings.Contains(string(data), `"failure_stage": "reset"`) ||
		!strings.Contains(string(data), `"exit_code": -1`) ||
		strings.Contains(string(data), secret) {
		t.Fatalf("failure metadata = %s", data)
	}
}

func TestRunPairRejectsUnexpectedResetOutput(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "run")
	err := runPairWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		func(context.Context, string, ...string) ([]byte, error) {
			return []byte("unexpected"), nil
		},
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "not recognized") {
		t.Fatalf("runPairWith() error = %v", err)
	}
}

func TestFinishSessionReportsWriteFailure(t *testing.T) {
	record := SessionRecord{}
	err := finishSession(
		filepath.Join(t.TempDir(), "missing"),
		&record,
		time.Now,
		"",
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "write session metadata") {
		t.Fatalf("finishSession() error = %v", err)
	}
}

func TestFinishSessionRejectsInvalidFailureStage(t *testing.T) {
	err := finishSession(
		t.TempDir(),
		&SessionRecord{},
		time.Now,
		"",
		errors.New("failed"),
	)
	if err == nil || !strings.Contains(err.Error(), "failure stage") {
		t.Fatalf("finishSession() error = %v", err)
	}
}

func TestCommandExitCode(t *testing.T) {
	command := exec.Command(
		os.Args[0],
		"-test.run=TestADBHelperProcess",
		"--",
		"exit-seven",
	)
	err := command.Run()
	if got := commandExitCode(err); got != 7 {
		t.Fatalf("commandExitCode() = %d, want 7", got)
	}
}

func sessionManifest() experiment.Manifest {
	return experiment.Manifest{
		SchemaVersion: experiment.CurrentSchemaVersion,
		Name:          "experiment-001-email",
		Variable:      "email",
		Baseline: experiment.Persona{
			"email":  "baseline@example.invalid",
			"region": "us-east",
		},
		Treatment: experiment.Persona{
			"email":  "treatment@example.invalid",
			"region": "us-east",
		},
		VolatileFields: []string{"request_id"},
	}
}

func sessionTarget() Target {
	return Target{
		Version:            "1.0.41",
		Device:             "emulator-5554",
		Package:            "dev.ariadne.fixture",
		AndroidAPI:         35,
		Architecture:       "x86_64",
		PackageVersionCode: 1,
		PackageSHA256:      strings.Repeat("a", 64),
		AriadneRevision:    strings.Repeat("b", 40),
	}
}

func sequenceClock() func() time.Time {
	next := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		current := next
		next = next.Add(time.Second)
		return current
	}
}

func directoryExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func postFixtureObservation(args []string, body []byte) error {
	for index := range args {
		if args[index] != "collector_port" || index+1 >= len(args) {
			continue
		}
		response, err := http.Post(
			"http://127.0.0.1:"+args[index+1]+"/observe",
			"application/json",
			strings.NewReader(string(body)),
		)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return fmt.Errorf("collector status %d", response.StatusCode)
		}
		return nil
	}
	return errors.New("collector port not found")
}

func normalizeNetworkPorts(calls [][]string) [][]string {
	normalized := make([][]string, len(calls))
	for callIndex, call := range calls {
		normalized[callIndex] = append([]string(nil), call...)
		for argumentIndex, argument := range normalized[callIndex] {
			if strings.HasPrefix(argument, "tcp:") {
				normalized[callIndex][argumentIndex] = "tcp:<port>"
			}
			if argumentIndex > 0 &&
				normalized[callIndex][argumentIndex-1] == "collector_port" {
				normalized[callIndex][argumentIndex] = "<port>"
			}
		}
	}
	return normalized
}
