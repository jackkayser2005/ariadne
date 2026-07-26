package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/experiment"
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
		{"android"},
		{"android", "unknown"},
		{"experiment"},
		{"experiment", "unknown"},
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

func TestRunExperiment(t *testing.T) {
	path := writeManifest(t, validManifest)
	outputDir := filepath.Join(t.TempDir(), "run")
	var stdout, stderr bytes.Buffer

	check := func(
		ctx context.Context,
		binary, device, packageName string,
	) (adb.Target, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check() context has no deadline")
		}
		if binary != "custom-adb" ||
			device != "emulator-5554" ||
			packageName != "dev.ariadne.fixture" {
			t.Fatalf("check() arguments = %q, %q, %q", binary, device, packageName)
		}
		return adb.Target{
			Version:            "1.0.41",
			Device:             device,
			Package:            packageName,
			AndroidAPI:         35,
			Architecture:       "x86_64",
			PackageVersionCode: 1,
			PackageSHA256:      strings.Repeat("a", 64),
		}, nil
	}
	runPair := func(
		_ context.Context,
		binary string,
		target adb.Target,
		manifest experiment.Manifest,
		output string,
	) error {
		if binary != "custom-adb" ||
			target.Device != "emulator-5554" ||
			manifest.Name != "experiment-001-email" ||
			output != outputDir {
			t.Fatalf(
				"runPair() arguments = %q, %#v, %#v, %q",
				binary,
				target,
				manifest,
				output,
			)
		}
		return nil
	}

	exitCode := runExperiment(
		[]string{
			"--adb", "custom-adb",
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"--output", outputDir,
			path,
		},
		&stdout,
		&stderr,
		check,
		runPair,
	)

	const want = "experiment complete\nname: experiment-001-email\nruns: 2\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runExperiment() = %d, stdout=%q, stderr=%q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunExperimentUsage(t *testing.T) {
	tests := [][]string{
		nil,
		{"--device", "emulator-5554"},
		{"--package", "dev.ariadne.fixture"},
		{"--output", "run"},
		{
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"--output", "run",
		},
		{
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"--output", "run",
			"one.json", "two.json",
		},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExperiment(
				args,
				&stdout,
				&stderr,
				func(context.Context, string, string, string) (adb.Target, error) {
					t.Fatal("check called for invalid usage")
					return adb.Target{}, nil
				},
				func(
					context.Context,
					string,
					adb.Target,
					experiment.Manifest,
					string,
				) error {
					t.Fatal("runPair called for invalid usage")
					return nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf(
					"runExperiment() = %d, stdout=%q, stderr=%q",
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}
}

func TestRunExperimentFailures(t *testing.T) {
	validArgs := func(path, output string) []string {
		return []string{
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"--output", output,
			path,
		}
	}
	target := adb.Target{
		Version: "1.0.41",
		Device:  "emulator-5554",
		Package: "dev.ariadne.fixture",
	}

	t.Run("missing manifest", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExperiment(
			validArgs(filepath.Join(t.TempDir(), "missing.json"), "run"),
			&stdout,
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				t.Fatal("check called for missing manifest")
				return adb.Target{}, nil
			},
			nil,
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "open manifest") {
			t.Fatalf("runExperiment() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("target check", func(t *testing.T) {
		path := writeManifest(t, validManifest)
		var stdout, stderr bytes.Buffer
		exitCode := runExperiment(
			validArgs(path, "run"),
			&stdout,
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				return adb.Target{}, errors.New("device unavailable")
			},
			nil,
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "device unavailable") {
			t.Fatalf("runExperiment() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("session", func(t *testing.T) {
		const secret = "do-not-print-persona"
		input := strings.Replace(validManifest, "baseline@example.invalid", secret, 1)
		path := writeManifest(t, input)
		var stdout, stderr bytes.Buffer
		exitCode := runExperiment(
			validArgs(path, "run"),
			&stdout,
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				return target, nil
			},
			func(
				context.Context,
				string,
				adb.Target,
				experiment.Manifest,
				string,
			) error {
				return errors.New("session failed")
			},
		)
		if exitCode != 1 ||
			!strings.Contains(stderr.String(), "session failed") ||
			strings.Contains(stderr.String(), secret) {
			t.Fatalf("runExperiment() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		path := writeManifest(t, validManifest)
		var stderr bytes.Buffer
		exitCode := runExperiment(
			validArgs(path, "run"),
			failingWriter{},
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				return target, nil
			},
			func(
				context.Context,
				string,
				adb.Target,
				experiment.Manifest,
				string,
			) error {
				return nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runExperiment() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunReport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runReport(
		[]string{"run-directory"},
		&stdout,
		&stderr,
		func(path string) (bundle.Summary, error) {
			if path != "run-directory" {
				t.Fatalf("write() path = %q", path)
			}
			return bundle.Summary{
				ManifestName: "experiment-001-email",
				Differences:  1,
			}, nil
		},
	)
	const want = "evidence bundle complete\n" +
		"name: experiment-001-email\n" +
		"differences: 1\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runReport() = %d, stdout=%q, stderr=%q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunReportFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"one", "two"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runReport(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.Summary, error) {
					t.Fatal("write called for invalid usage")
					return bundle.Summary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf(
					"runReport() = %d, stdout=%q, stderr=%q",
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}

	t.Run("bundle error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runReport(
			[]string{"run"},
			&stdout,
			&stderr,
			func(string) (bundle.Summary, error) {
				return bundle.Summary{}, errors.New("invalid bundle")
			},
		)
		if exitCode != 1 ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), "invalid bundle") {
			t.Fatalf(
				"runReport() = %d, stdout=%q, stderr=%q",
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runReport(
			[]string{"run"},
			failingWriter{},
			&stderr,
			func(string) (bundle.Summary, error) {
				return bundle.Summary{ManifestName: "experiment-001"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runReport() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunAndroidCheck(t *testing.T) {
	check := func(
		ctx context.Context,
		binary, device, packageName string,
	) (adb.Target, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check() context has no deadline")
		}
		if binary != "custom-adb" ||
			device != "emulator-5554" ||
			packageName != "dev.ariadne.fixture" {
			t.Fatalf(
				"check() arguments = %q, %q, %q",
				binary,
				device,
				packageName,
			)
		}
		return adb.Target{
			Version:            "1.0.41",
			Device:             device,
			Package:            packageName,
			AndroidAPI:         35,
			Architecture:       "x86_64",
			PackageVersionCode: 1,
			PackageSHA256:      strings.Repeat("a", 64),
		}, nil
	}
	var stdout, stderr bytes.Buffer

	exitCode := runAndroidCheck(
		[]string{
			"--adb", "custom-adb",
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
		},
		&stdout,
		&stderr,
		check,
	)

	want := "android target ready\n" +
		"adb_version: 1.0.41\n" +
		"device: emulator-5554\n" +
		"android_api: 35\n" +
		"architecture: x86_64\n" +
		"package: dev.ariadne.fixture\n" +
		"package_version_code: 1\n" +
		"package_sha256: " + strings.Repeat("a", 64) + "\n" +
		"ariadne_revision: unknown\n" +
		"ariadne_modified: false\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runAndroidCheck() = %d, stdout=%q, stderr=%q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunAndroidCheckFailures(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing device", args: []string{"--package", "dev.ariadne.fixture"}},
		{name: "missing package", args: []string{"--device", "emulator-5554"}},
		{name: "unknown flag", args: []string{"--unknown"}},
		{name: "positional argument", args: []string{
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"extra",
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAndroidCheck(
				test.args,
				&stdout,
				&stderr,
				func(context.Context, string, string, string) (adb.Target, error) {
					t.Fatal("check called for invalid usage")
					return adb.Target{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf(
					"runAndroidCheck() = %d, stdout=%q, stderr=%q",
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}

	t.Run("check error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidCheck(
			[]string{
				"--device", "emulator-5554",
				"--package", "dev.ariadne.fixture",
			},
			&stdout,
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				return adb.Target{}, errors.New("adb unavailable")
			},
		)
		if exitCode != 1 ||
			stdout.Len() != 0 ||
			!strings.Contains(stderr.String(), "adb unavailable") {
			t.Fatalf(
				"runAndroidCheck() = %d, stdout=%q, stderr=%q",
				exitCode,
				stdout.String(),
				stderr.String(),
			)
		}
	})

	t.Run("write error", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runAndroidCheck(
			[]string{
				"--device", "emulator-5554",
				"--package", "dev.ariadne.fixture",
			},
			failingWriter{},
			&stderr,
			func(context.Context, string, string, string) (adb.Target, error) {
				return adb.Target{
					Version: "1.0.41",
					Device:  "emulator-5554",
					Package: "dev.ariadne.fixture",
				}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf(
				"runAndroidCheck() = %d, stderr=%q",
				exitCode,
				stderr.String(),
			)
		}
	})
}

func TestIdentityFromSettings(t *testing.T) {
	revision := strings.Repeat("a", 40)
	gotRevision, modified := identityFromSettings([]debug.BuildSetting{
		{Key: "vcs.revision", Value: revision},
		{Key: "vcs.modified", Value: "true"},
	})
	if gotRevision != revision || !modified {
		t.Fatalf("identityFromSettings() = %q, %t", gotRevision, modified)
	}

	gotRevision, modified = identityFromSettings([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "invalid"},
		{Key: "vcs.modified", Value: "false"},
	})
	if gotRevision != "unknown" || modified {
		t.Fatalf("identityFromSettings() = %q, %t", gotRevision, modified)
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
