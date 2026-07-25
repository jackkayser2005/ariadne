package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validManifest = `{
	"schema_version": 1,
	"name": "experiment-001-email",
	"variable": "email",
	"baseline": {"email": "baseline@example.invalid", "region": "us-east"},
	"treatment": {"email": "treatment@example.invalid", "region": "us-east"}
}`

func TestRunValidate(t *testing.T) {
	path := writeManifest(t, validManifest)
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"validate", path}, &stdout, &stderr)

	const want = "valid manifest\n" +
		"name: experiment-001-email\n" +
		"schema_version: 1\n" +
		"variable: email\n" +
		"persona_fields: 2\n"
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if stdout.String() != want {
		t.Fatalf("run() stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run() stderr = %q", stderr.String())
	}
}

func TestRunUsage(t *testing.T) {
	tests := [][]string{
		nil,
		{"unknown"},
		{"validate"},
		{"validate", "one.json", "two.json"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := run(args, &stdout, &stderr)

			if exitCode != 2 {
				t.Fatalf("run() exit code = %d, want 2", exitCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("run() stdout = %q", stdout.String())
			}
			if stderr.String() != usage {
				t.Fatalf("run() stderr = %q, want %q", stderr.String(), usage)
			}
		})
	}
}

func TestRunReadFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"validate", filepath.Join(t.TempDir(), "missing.json")}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "open manifest") {
		t.Fatalf("run() stderr = %q, want open error", stderr.String())
	}
}

func TestRunDoesNotExposePersonaValues(t *testing.T) {
	const secret = "do-not-print-this-value"
	input := strings.Replace(validManifest, "baseline@example.invalid", secret, 1)
	input = strings.Replace(input, "treatment@example.invalid", secret, 1)
	path := writeManifest(t, input)
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"validate", path}, &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("run() exposed persona value: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunWriteFailure(t *testing.T) {
	path := writeManifest(t, validManifest)
	var stderr bytes.Buffer

	exitCode := run(
		[]string{"validate", path},
		failingWriter{},
		&stderr,
	)

	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("run() stderr = %q, want output error", stderr.String())
	}
}

func writeManifest(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
