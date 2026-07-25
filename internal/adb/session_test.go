package adb

import (
	"context"
	"errors"
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
	target := Target{
		Version: "1.0.41",
		Device:  "emulator-5554",
		Package: "dev.ariadne.fixture",
	}
	var calls [][]string
	run := func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		if args[3] == "pm" {
			return []byte("Success\n"), nil
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
		{
			"adb", "-s", "emulator-5554", "shell", "am", "start", "-W", "-S",
			"-n", "dev.ariadne.fixture/.MainActivity",
			"--es", "email", "baseline@example.invalid",
			"--es", "region", "us-east",
		},
		{"adb", "-s", "emulator-5554", "shell", "pm", "clear", "dev.ariadne.fixture"},
		{
			"adb", "-s", "emulator-5554", "shell", "am", "start", "-W", "-S",
			"-n", "dev.ariadne.fixture/.MainActivity",
			"--es", "email", "treatment@example.invalid",
			"--es", "region", "us-east",
		},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("runPairWith() calls = %#v, want %#v", calls, wantCalls)
	}

	for _, kind := range []string{"baseline", "treatment"} {
		data, err := os.ReadFile(filepath.Join(outputDir, kind, "session.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, `"kind": "`+kind+`"`) ||
			!strings.Contains(text, `"exit_code": 0`) {
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

func TestRunPairRejectsUnsafePersonaBeforeCreatingOutput(t *testing.T) {
	manifest := sessionManifest()
	const secret = "value with spaces"
	manifest.Treatment["email"] = secret
	outputDir := filepath.Join(t.TempDir(), "run")
	called := false

	err := runPairWith(
		context.Background(),
		"adb",
		Target{
			Version: "1.0.41",
			Device:  "emulator-5554",
			Package: "dev.ariadne.fixture",
		},
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
	validTarget := Target{
		Version: "1.0.41",
		Device:  "emulator-5554",
		Package: "dev.ariadne.fixture",
	}
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
		Target{
			Version: "1.0.41",
			Device:  "emulator-5554",
			Package: "dev.ariadne.fixture",
		},
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
		Target{
			Version: "1.0.41",
			Device:  "emulator-5554",
			Package: "dev.ariadne.fixture",
		},
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
		Target{
			Version: "1.0.41",
			Device:  "emulator-5554",
			Package: "dev.ariadne.fixture",
		},
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
		Target{
			Version: "1.0.41",
			Device:  "emulator-5554",
			Package: "dev.ariadne.fixture",
		},
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
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "write session metadata") {
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
