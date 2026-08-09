package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
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

	manifest, err := experiment.Decode(strings.NewReader(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	contractDigest := manifest.ContractDigest()
	want := "valid manifest\n" +
		"name: experiment-001-email\n" +
		"schema_version: 1\n" +
		"variable: email\n" +
		"persona_fields: 2\n" +
		"manifest_contract_sha256: " + contractDigest + "\n"
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
		{"experiment", "export"},
		{"experiment", "export", "verify"},
		{"experiment", "export", "ask"},
		{"experiment", "export", "finding"},
		{"experiment", "verify"},
		{"experiment", "finding"},
		{"experiment", "ask"},
		{"experiment", "ask-archive"},
		{"experiment", "ask-archive", "save"},
		{"experiment", "ask-archive", "compare"},
		{"experiment", "ask-archive", "compare-current"},
		{"experiment", "ask-archive", "transitions"},
		{"experiment", "ask-archive", "transitions", "save"},
		{"experiment", "ask-archive", "transitions", "verify"},
		{"experiment", "ask-archive", "verify"},
		{"experiment", "questions", "extra"},
		{"experiment", "list"},
		{"experiment", "serve"},
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
				Unknowns:     0,
			}, nil
		},
	)
	const want = "evidence bundle complete\n" +
		"name: experiment-001-email\n" +
		"differences: 1\n" +
		"unknowns: 0\n"
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

func TestRunExport(t *testing.T) {
	var stdout, stderr bytes.Buffer
	digest := strings.Repeat("a", 64)
	exitCode := runExport(
		[]string{"run-directory", "export.json"},
		&stdout,
		&stderr,
		func(runDir, exportPath string) (bundle.ExportSummary, error) {
			if runDir != "run-directory" || exportPath != "export.json" {
				t.Fatalf("Export() arguments = %q, %q", runDir, exportPath)
			}
			return bundle.ExportSummary{SourceEvidenceSHA256: digest, ExportSHA256: digest}, nil
		},
	)
	want := "redacted export complete\nsource_evidence_sha256: " + digest + "\nexport_sha256: " + digest + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runExport() = %d, stdout=%q, stderr=%q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunExportFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"run"}, {"run", "export", "extra"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExport(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.ExportSummary, error) {
					t.Fatal("export called for invalid usage")
					return bundle.ExportSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf(
					"runExport() = %d, stdout=%q, stderr=%q",
					exitCode,
					stdout.String(),
					stderr.String(),
				)
			}
		})
	}

	t.Run("export error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExport(
			[]string{"run", "export.json"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ExportSummary, error) {
				return bundle.ExportSummary{}, errors.New("invalid bundle")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid bundle") {
			t.Fatalf("runExport() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runExport(
			[]string{"run", "export.json"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.ExportSummary, error) {
				return bundle.ExportSummary{SourceEvidenceSHA256: strings.Repeat("a", 64), ExportSHA256: strings.Repeat("a", 64)}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runExport() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunExportVerify(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var stdout, stderr bytes.Buffer
	exitCode := runExportVerify(
		[]string{"export.json"},
		&stdout,
		&stderr,
		func(path string) (bundle.ExportVerificationSummary, error) {
			if path != "export.json" {
				t.Fatalf("VerifyExport() path = %q", path)
			}
			return bundle.ExportVerificationSummary{SchemaVersion: 1, SourceEvidenceSHA256: digest, ExportSHA256: digest}, nil
		},
	)
	want := "redacted export verified\nschema_version: 1\nsource_evidence_sha256: " + digest + "\nexport_sha256: " + digest + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExportVerifyJSON(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var stdout, stderr bytes.Buffer
	exitCode := runExportVerify(
		[]string{"--json", "export.json"},
		&stdout,
		&stderr,
		func(string) (bundle.ExportVerificationSummary, error) {
			return bundle.ExportVerificationSummary{SchemaVersion: 1, SourceEvidenceSHA256: digest, ExportSHA256: digest}, nil
		},
	)
	want := `{"schema_version":1,"source_evidence_sha256":"` + digest + `","export_sha256":"` + digest + `"}` + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExportVerifyExpectedIdentity(t *testing.T) {
	expected := strings.Repeat("a", 64)

	t.Run("match", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExportVerify(
			[]string{"--expect-sha256", expected, "export.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ExportVerificationSummary, error) {
				return bundle.ExportVerificationSummary{SchemaVersion: 1, ExportSHA256: expected}, nil
			},
		)
		if exitCode != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, value := range []string{"bad", ""} {
		t.Run("invalid "+value, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExportVerify(
				[]string{"--expect-sha256=" + value, "export.json"},
				&stdout,
				&stderr,
				func(string) (bundle.ExportVerificationSummary, error) {
					t.Fatal("VerifyExport called for invalid expected identity")
					return bundle.ExportVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "expect-sha256 must be") {
				t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("mismatch", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExportVerify(
			[]string{"--expect-sha256", strings.Repeat("b", 64), "export.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ExportVerificationSummary, error) {
				return bundle.ExportVerificationSummary{SchemaVersion: 1, ExportSHA256: expected}, nil
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "redacted export SHA-256 mismatch") {
			t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunExportVerifyFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"one", "two"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExportVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ExportVerificationSummary, error) {
					t.Fatal("verify called for invalid usage")
					return bundle.ExportVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("verify error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExportVerify(
			[]string{"export.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ExportVerificationSummary, error) {
				return bundle.ExportVerificationSummary{}, errors.New("invalid export")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid export") {
			t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runExportVerify(
			[]string{"export.json"},
			failingWriter{},
			&stderr,
			func(string) (bundle.ExportVerificationSummary, error) {
				return bundle.ExportVerificationSummary{SchemaVersion: 1}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runExportVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("invalid json flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExportVerify(
			[]string{"--json=invalid", "export.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ExportVerificationSummary, error) {
				t.Fatal("verify called for invalid JSON flag")
				return bundle.ExportVerificationSummary{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("runExportVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunExportAsk(t *testing.T) {
	findingID := "sha256:" + strings.Repeat("a", 64)
	var stdout, stderr bytes.Buffer
	exitCode := runExportAsk(
		[]string{"export.json", "counterfactual-change"},
		&stdout,
		&stderr,
		func(path, questionID string) (bundle.Answer, error) {
			if path != "export.json" || questionID != "counterfactual-change" {
				t.Fatalf("AskExport() arguments = %q, %q", path, questionID)
			}
			return bundle.Answer{
				QuestionID: "counterfactual-change",
				Question:   "Did changing the declared variable influence an observed output?",
				State:      "observed",
				FindingIDs: []string{findingID},
			}, nil
		},
	)
	want := "question answered\n" +
		"id: counterfactual-change\n" +
		"question: Did changing the declared variable influence an observed output?\n" +
		"answer_state: observed\n" +
		"findings:\n" +
		"- " + findingID + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runExportAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runExportAsk(
		[]string{"--json", "export.json", "counterfactual-change"},
		&stdout,
		&stderr,
		func(string, string) (bundle.Answer, error) {
			return bundle.Answer{
				QuestionID: "counterfactual-change",
				Question:   "Did changing the declared variable influence an observed output?",
				State:      "observed",
				FindingIDs: []string{findingID},
			}, nil
		},
	)
	wantJSON := `{"question_id":"counterfactual-change","question":"Did changing the declared variable influence an observed output?","answer_state":"observed","finding_ids":["` + findingID + `"]}` + "\n"
	if exitCode != 0 || stdout.String() != wantJSON || stderr.Len() != 0 {
		t.Fatalf("runExportAsk() JSON = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExportAskFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"export.json"}, {"export.json", "question", "extra"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExportAsk(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.Answer, error) {
					t.Fatal("ask called for invalid usage")
					return bundle.Answer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runExportAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("export error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runExportAsk(
			[]string{"export.json", "counterfactual-change"},
			&stdout,
			&stderr,
			func(string, string) (bundle.Answer, error) {
				return bundle.Answer{}, errors.New("invalid export")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "experiment export ask: invalid export") {
			t.Fatalf("runExportAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunExportFinding(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	var stdout, stderr bytes.Buffer
	exitCode := runExportFinding(
		[]string{"export.json", id},
		&stdout,
		&stderr,
		func(path, findingID string) (bundle.Finding, error) {
			if path != "export.json" || findingID != id {
				t.Fatalf("FindExport() arguments = %q, %q", path, findingID)
			}
			return bundle.Finding{
				Question:       "Did changing the declared variable influence an observed output?",
				AnswerState:    "observed",
				Kind:           "difference",
				Classification: "changed",
				ID:             id,
				Field:          "variant",
				State:          "observed",
				Evidence:       []string{"baseline/observations/storage.json#/variant"},
			}, nil
		},
	)
	want := "finding verified\n" +
		"question: Did changing the declared variable influence an observed output?\n" +
		"answer_state: observed\n" +
		"kind: difference\n" +
		"classification: changed\n" +
		"id: " + id + "\n" +
		"field: variant\n" +
		"state: observed\n" +
		"evidence:\n" +
		"- baseline/observations/storage.json#/variant\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runExportFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExportFindingFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runExportFinding(
		[]string{"export.json"},
		&stdout,
		&stderr,
		func(string, string) (bundle.Finding, error) {
			t.Fatal("find called for invalid usage")
			return bundle.Finding{}, nil
		},
	)
	if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
		t.Fatalf("runExportFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunVerify(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runVerify(
		[]string{"run-directory"},
		&stdout,
		&stderr,
		func(path string) (bundle.Summary, error) {
			if path != "run-directory" {
				t.Fatalf("verify() path = %q", path)
			}
			return bundle.Summary{
				ManifestName:           "experiment-001-email",
				Differences:            1,
				Unknowns:               0,
				Question:               "private question",
				AnswerState:            "observed",
				ManifestContractSHA256: strings.Repeat("c", 64),
				AriadneRevision:        strings.Repeat("b", 40),
				AriadneModified:        true,
			}, nil
		},
	)
	const want = "evidence bundle verified\n" +
		"name: experiment-001-email\n" +
		"differences: 1\n" +
		"unknowns: 0\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf(
			"runVerify() = %d, stdout=%q, stderr=%q",
			exitCode,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestRunVerifyJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runVerify(
		[]string{"--json", "run-directory"},
		&stdout,
		&stderr,
		func(path string) (bundle.Summary, error) {
			if path != "run-directory" {
				t.Fatalf("verify() path = %q", path)
			}
			return bundle.Summary{
				ManifestName: "experiment-001-email",
				Differences:  1,
				Unknowns:     0,
			}, nil
		},
	)
	const want = `{"manifest_name":"experiment-001-email","differences":1,"unknowns":0}` + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunVerifyFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"one", "two"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.Summary, error) {
					t.Fatal("verify called for invalid usage")
					return bundle.Summary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("bundle error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runVerify(
			[]string{"run"},
			&stdout,
			&stderr,
			func(string) (bundle.Summary, error) {
				return bundle.Summary{}, errors.New("invalid bundle")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid bundle") {
			t.Fatalf("runVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runVerify(
			[]string{"run"},
			failingWriter{},
			&stderr,
			func(string) (bundle.Summary, error) {
				return bundle.Summary{ManifestName: "experiment-001"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("invalid json flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runVerify(
			[]string{"--json=invalid", "run"},
			&stdout,
			&stderr,
			func(string) (bundle.Summary, error) {
				t.Fatal("verify called for invalid JSON flag")
				return bundle.Summary{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("runVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runVerify(
			[]string{"--json", "run"},
			failingWriter{},
			&stderr,
			func(string) (bundle.Summary, error) {
				return bundle.Summary{ManifestName: "experiment-001"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunFinding(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runFinding(
		[]string{"run-directory", "sha256:" + strings.Repeat("a", 64)},
		&stdout,
		&stderr,
		func(runDir, id string) (bundle.Finding, error) {
			if runDir != "run-directory" || id != "sha256:"+strings.Repeat("a", 64) {
				t.Fatalf("Find() arguments = %q, %q", runDir, id)
			}
			return bundle.Finding{
				Question:       "Did changing email influence an observed output?",
				AnswerState:    "observed",
				Kind:           "difference",
				Classification: "changed",
				ID:             id,
				Field:          "variant",
				State:          "observed",
				Evidence:       []string{"baseline/observations/storage.json#/variant"},
			}, nil
		},
	)
	const want = "finding verified\n" +
		"question: Did changing email influence an observed output?\n" +
		"answer_state: observed\n" +
		"kind: difference\n" +
		"classification: changed\n" +
		"id: sha256:" +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
		"field: variant\n" +
		"state: observed\n" +
		"evidence:\n" +
		"- baseline/observations/storage.json#/variant\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunFindingJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	id := "sha256:" + strings.Repeat("a", 64)
	exitCode := runFinding(
		[]string{"--json", "run-directory", id},
		&stdout,
		&stderr,
		func(runDir, findingID string) (bundle.Finding, error) {
			if runDir != "run-directory" || findingID != id {
				t.Fatalf("Find() arguments = %q, %q", runDir, findingID)
			}
			return bundle.Finding{
				Question:       "Did changing the declared variable influence an observed output?",
				AnswerState:    "observed",
				Kind:           "difference",
				Classification: "changed",
				ID:             findingID,
				Field:          "variant",
				State:          "observed",
				Evidence:       []string{"baseline/observations/storage.json#/variant"},
			}, nil
		},
	)
	want := `{"question":"Did changing the declared variable influence an observed output?","answer_state":"observed","kind":"difference","classification":"changed","id":"` + id + `","field":"variant","state":"observed","evidence":["baseline/observations/storage.json#/variant"]}` + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunFindingFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"run"}, {"run", "one", "two"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runFinding(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.Finding, error) {
					t.Fatal("find called for invalid usage")
					return bundle.Finding{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("bundle error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runFinding(
			[]string{"run", "id"},
			&stdout,
			&stderr,
			func(string, string) (bundle.Finding, error) {
				return bundle.Finding{}, errors.New("invalid bundle")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid bundle") {
			t.Fatalf("runFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runFinding(
			[]string{"run", "id"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.Finding, error) {
				return bundle.Finding{Question: "question", AnswerState: "observed", Kind: "difference", ID: "id", Field: "field", State: "observed"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runFinding() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("invalid json flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runFinding(
			[]string{"--json=invalid", "run", "id"},
			&stdout,
			&stderr,
			func(string, string) (bundle.Finding, error) {
				t.Fatal("find called for invalid JSON flag")
				return bundle.Finding{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("runFinding() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runFinding(
			[]string{"--json", "run", "id"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.Finding, error) {
				return bundle.Finding{Question: "question", AnswerState: "observed", Kind: "difference", ID: "id", Field: "field", State: "observed"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runFinding() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunAsk(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := runAsk(
		[]string{"run-directory", "capture-complete"},
		&stdout,
		&stderr,
		func(runDir, questionID string) (bundle.Answer, error) {
			if runDir != "run-directory" || questionID != "capture-complete" {
				t.Fatalf("Ask() arguments = %q, %q", runDir, questionID)
			}
			return bundle.Answer{
				QuestionID: "capture-complete",
				Question:   "Were all required observations captured for both sessions?",
				State:      "observed",
				FindingIDs: []string{"sha256:" + strings.Repeat("a", 64)},
			}, nil
		},
	)
	want := "question answered\n" +
		"id: capture-complete\n" +
		"question: Were all required observations captured for both sessions?\n" +
		"answer_state: observed\n" +
		"findings:\n" +
		"- sha256:" + strings.Repeat("a", 64) + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunAskJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	findingID := "sha256:" + strings.Repeat("a", 64)
	exitCode := runAsk(
		[]string{"--json", "run-directory", "capture-complete"},
		&stdout,
		&stderr,
		func(runDir, questionID string) (bundle.Answer, error) {
			if runDir != "run-directory" || questionID != "capture-complete" {
				t.Fatalf("Ask() arguments = %q, %q", runDir, questionID)
			}
			return bundle.Answer{
				QuestionID: "capture-complete",
				Question:   "Were all required observations captured for both sessions?",
				State:      "observed",
				FindingIDs: []string{findingID},
			}, nil
		},
	)
	want := `{"question_id":"capture-complete","question":"Were all required observations captured for both sessions?","answer_state":"observed","finding_ids":["` + findingID + `"]}` + "\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunAskFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"run"}, {"run", "one", "two"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAsk(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.Answer, error) {
					t.Fatal("ask called for invalid usage")
					return bundle.Answer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("bundle error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAsk(
			[]string{"run", "question"},
			&stdout,
			&stderr,
			func(string, string) (bundle.Answer, error) {
				return bundle.Answer{}, errors.New("invalid bundle")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid bundle") {
			t.Fatalf("runAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runAsk(
			[]string{"run", "question"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.Answer, error) {
				return bundle.Answer{QuestionID: "question", Question: "question", State: "observed"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runAsk() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("invalid json flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAsk(
			[]string{"--json=invalid", "run", "question"},
			&stdout,
			&stderr,
			func(string, string) (bundle.Answer, error) {
				t.Fatal("ask called for invalid JSON flag")
				return bundle.Answer{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("runAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runAsk(
			[]string{"--json", "run", "question"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.Answer, error) {
				return bundle.Answer{QuestionID: "question", Question: "question", State: "observed"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runAsk() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunAskArchive(t *testing.T) {
	findingID := "sha256:" + strings.Repeat("a", 64)
	report := bundle.ArchiveQuestionReport{
		SchemaVersion: 2,
		QuestionID:    "counterfactual-change",
		Question:      "Did it change?",
		Summary: bundle.ArchiveQuestionSummary{
			Observed:    1,
			Unavailable: 1,
			Checked:     2,
		},
		Results: []bundle.ArchiveQuestionResult{
			{
				Directory:    "old-run",
				ManifestName: "current",
				RecordedAt:   "2026-07-24T12:00:00Z",
				Provenance: &bundle.ArchiveQuestionProvenance{
					ManifestContractSHA256: strings.Repeat("c", 64),
					SourceEvidenceSHA256:   strings.Repeat("d", 64),
					AriadneRevision:        strings.Repeat("b", 40),
				},
				Answer: &bundle.Answer{
					QuestionID: "counterfactual-change",
					Question:   "Did it change?",
					State:      "observed",
					FindingIDs: []string{findingID},
				},
				Available: true,
			},
			{
				Directory:    "legacy-run",
				ManifestName: "legacy",
			},
		},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchive(
			[]string{"archive-root", "counterfactual-change"},
			&stdout,
			&stderr,
			func(root, questionID string) (bundle.ArchiveQuestionReport, error) {
				if root != "archive-root" || questionID != "counterfactual-change" {
					t.Fatalf("AskArchive() arguments = %q, %q", root, questionID)
				}
				return report, nil
			},
		)
		want := "archive question answered\n" +
			"id: counterfactual-change\n" +
			"question: Did it change?\n" +
			"observed: 1\n" +
			"unknown: 0\n" +
			"unavailable: 1\n" +
			"checked: 2\n" +
			"results:\n" +
			"- directory: old-run\n" +
			"  manifest_name: current\n" +
			"  recorded_at: 2026-07-24T12:00:00Z\n" +
			"  answer_state: observed\n" +
			"  findings:\n" +
			"  - " + findingID + "\n" +
			"- directory: legacy-run\n" +
			"  manifest_name: legacy\n" +
			"  answer_state: unavailable\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchive() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchive(
			[]string{"--json", "archive-root", "counterfactual-change"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionReport, error) { return report, nil },
		)
		want := `{"schema_version":2,"question_id":"counterfactual-change","question":"Did it change?","summary":{"observed":1,"unknown":0,"unavailable":1,"checked":2},"results":[{"directory":"old-run","manifest_name":"current","recorded_at":"2026-07-24T12:00:00Z","provenance":{"manifest_contract_sha256":"` + strings.Repeat("c", 64) + `","source_evidence_sha256":"` + strings.Repeat("d", 64) + `","ariadne_revision":"` + strings.Repeat("b", 40) + `","ariadne_modified":false},"answer":{"question_id":"counterfactual-change","question":"Did it change?","answer_state":"observed","finding_ids":["` + findingID + `"]},"available":true},{"directory":"legacy-run","manifest_name":"legacy","available":false}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchive() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveSave(t *testing.T) {
	summary := bundle.ArchiveQuestionVerificationSummary{
		SchemaVersion:    2,
		QuestionID:       "counterfactual-change",
		Checked:          2,
		ReflectionSHA256: strings.Repeat("a", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveSave(
			[]string{"archive-root", "counterfactual-change", "reflection.json"},
			&stdout,
			&stderr,
			func(root, questionID, reportPath string) (bundle.ArchiveQuestionVerificationSummary, error) {
				if root != "archive-root" || questionID != "counterfactual-change" || reportPath != "reflection.json" {
					t.Fatalf("SaveArchiveQuestionReport() arguments = %q, %q, %q", root, questionID, reportPath)
				}
				return summary, nil
			},
		)
		want := "archive question saved\n" +
			"schema_version: 2\n" +
			"question_id: counterfactual-change\n" +
			"checked: 2\n" +
			"reflection_sha256: " + strings.Repeat("a", 64) + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveSave(
			[]string{"--json", "archive-root", "counterfactual-change", "reflection.json"},
			&stdout,
			&stderr,
			func(string, string, string) (bundle.ArchiveQuestionVerificationSummary, error) { return summary, nil },
		)
		want := `{"schema_version":2,"question_id":"counterfactual-change","checked":2,"reflection_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveSaveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"archive-root", "question"}, {"archive-root", "question", "report.json", "extra"}, {"--json=invalid", "archive-root", "question", "report.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveSave(
				args,
				&stdout,
				&stderr,
				func(string, string, string) (bundle.ArchiveQuestionVerificationSummary, error) {
					t.Fatal("SaveArchiveQuestionReport called for invalid usage")
					return bundle.ArchiveQuestionVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("save error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveSave(
			[]string{"archive-root", "question", "report.json"},
			&stdout,
			&stderr,
			func(string, string, string) (bundle.ArchiveQuestionVerificationSummary, error) {
				return bundle.ArchiveQuestionVerificationSummary{}, errors.New("report path is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "report path is invalid") {
			t.Fatalf("runAskArchiveSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{
		{"archive-root", "question", "report.json"},
		{"--json", "archive-root", "question", "report.json"},
	} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveSave(
				args,
				failingWriter{},
				&stderr,
				func(string, string, string) (bundle.ArchiveQuestionVerificationSummary, error) {
					return bundle.ArchiveQuestionVerificationSummary{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveSave() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveCompare(t *testing.T) {
	comparison := bundle.ArchiveQuestionComparison{
		SchemaVersion:         1,
		ComparisonID:          "answer-state-change",
		ComparisonQuestion:    "Did the bounded answer state change between these saved reflection snapshots?",
		QuestionID:            "counterfactual-change",
		Question:              "Did it change?",
		Result:                "changed",
		OlderReflectionSHA256: strings.Repeat("a", 64),
		NewerReflectionSHA256: strings.Repeat("b", 64),
		Compared:              2,
		Changed:               1,
		OlderOnly:             0,
		NewerOnly:             1,
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveCompare(
			[]string{"older.json", "newer.json"},
			&stdout,
			&stderr,
			func(older, newer string) (bundle.ArchiveQuestionComparison, error) {
				if older != "older.json" || newer != "newer.json" {
					t.Fatalf("CompareArchiveQuestionReports() paths = %q, %q", older, newer)
				}
				return comparison, nil
			},
		)
		want := "archive reflection comparison complete\n" +
			"comparison_id: answer-state-change\n" +
			"comparison_question: Did the bounded answer state change between these saved reflection snapshots?\n" +
			"question_id: counterfactual-change\n" +
			"question: Did it change?\n" +
			"result: changed\n" +
			"older_reflection_sha256: " + strings.Repeat("a", 64) + "\n" +
			"newer_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"compared: 2\n" +
			"changed: 1\n" +
			"older_only: 0\n" +
			"newer_only: 1\n" +
			"note: this compares only bounded answer states; it does not infer a trend or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveCompare(
			[]string{"--json", "older.json", "newer.json"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionComparison, error) { return comparison, nil },
		)
		want := `{"schema_version":1,"comparison_id":"answer-state-change","comparison_question":"Did the bounded answer state change between these saved reflection snapshots?","question_id":"counterfactual-change","question":"Did it change?","result":"changed","older_reflection_sha256":"` + strings.Repeat("a", 64) + `","newer_reflection_sha256":"` + strings.Repeat("b", 64) + `","compared":2,"changed":1,"older_only":0,"newer_only":1}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveCompareCurrent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	comparison := bundle.ArchiveQuestionComparison{
		SchemaVersion:         1,
		ComparisonID:          "answer-state-change",
		ComparisonQuestion:    "Did the bounded answer state change between these saved reflection snapshots?",
		QuestionID:            "counterfactual-change",
		Question:              "Did it change?",
		Result:                "same",
		OlderReflectionSHA256: strings.Repeat("a", 64),
		NewerReflectionSHA256: strings.Repeat("b", 64),
	}
	exitCode := runAskArchiveCompareCurrent(
		[]string{"older.json", "archive-root"},
		&stdout,
		&stderr,
		func(older, archiveRoot string) (bundle.ArchiveQuestionComparison, error) {
			if older != "older.json" || archiveRoot != "archive-root" {
				t.Fatalf("CompareArchiveQuestionReportWithArchive() paths = %q, %q", older, archiveRoot)
			}
			return comparison, nil
		},
	)
	if exitCode != 0 || !strings.Contains(stdout.String(), "archive reflection comparison complete") || stderr.Len() != 0 {
		t.Fatalf("runAskArchiveCompareCurrent() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunAskArchiveCompareFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"older.json"}, {"older.json", "newer.json", "extra"}, {"--json=invalid", "older.json", "newer.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveCompare(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.ArchiveQuestionComparison, error) {
					t.Fatal("CompareArchiveQuestionReports called for invalid usage")
					return bundle.ArchiveQuestionComparison{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("comparison error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveCompare(
			[]string{"older.json", "newer.json"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionComparison, error) {
				return bundle.ArchiveQuestionComparison{}, errors.New("questions do not match")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "questions do not match") {
			t.Fatalf("runAskArchiveCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{{"older.json", "newer.json"}, {"--json", "older.json", "newer.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveCompare(
				args,
				failingWriter{},
				&stderr,
				func(string, string) (bundle.ArchiveQuestionComparison, error) {
					return bundle.ArchiveQuestionComparison{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveCompare() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitions(t *testing.T) {
	history := bundle.ArchiveQuestionTransitionHistory{
		SchemaVersion:   1,
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		QuestionID:      "counterfactual-change",
		Question:        "Did it change?",
		OrderBasis:      "caller",
		Snapshots:       3,
		Transitions: []bundle.ArchiveQuestionTransition{
			{
				FromReflectionSHA256: strings.Repeat("a", 64),
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              1,
			},
			{
				FromReflectionSHA256: strings.Repeat("b", 64),
				ToReflectionSHA256:   strings.Repeat("c", 64),
				Result:               "incomparable",
				Compared:             1,
				FromOnly:             1,
				ToOnly:               2,
			},
		},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitions(
			[]string{"one.json", "two.json", "three.json"},
			&stdout,
			&stderr,
			func(paths []string) (bundle.ArchiveQuestionTransitionHistory, error) {
				if strings.Join(paths, ",") != "one.json,two.json,three.json" {
					t.Fatalf("CompareArchiveQuestionHistory() paths = %q", paths)
				}
				return history, nil
			},
		)
		want := "archive reflection transitions complete\n" +
			"history_id: answer-state-transitions\n" +
			"history_question: At which supplied boundaries did the bounded answer state change?\n" +
			"question_id: counterfactual-change\n" +
			"question: Did it change?\n" +
			"order_basis: caller\n" +
			"snapshots: 3\n" +
			"transitions: 2\n" +
			"- transition: 1\n" +
			"  from_reflection_sha256: " + strings.Repeat("a", 64) + "\n" +
			"  to_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  result: changed\n" +
			"  compared: 2\n" +
			"  changed: 1\n" +
			"  from_only: 0\n" +
			"  to_only: 0\n" +
			"- transition: 2\n" +
			"  from_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  to_reflection_sha256: " + strings.Repeat("c", 64) + "\n" +
			"  result: incomparable\n" +
			"  compared: 1\n" +
			"  changed: 0\n" +
			"  from_only: 1\n" +
			"  to_only: 2\n" +
			"note: transitions follow caller-supplied order; incomparable membership is not a change claim, and this does not infer a trend or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitions(
			[]string{"--json", "one.json", "two.json", "three.json"},
			&stdout,
			&stderr,
			func([]string) (bundle.ArchiveQuestionTransitionHistory, error) { return history, nil },
		)
		want := `{"schema_version":1,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","question":"Did it change?","order_basis":"caller","snapshots":3,"transitions":[{"from_reflection_sha256":"` + strings.Repeat("a", 64) + `","to_reflection_sha256":"` + strings.Repeat("b", 64) + `","result":"changed","compared":2,"changed":1,"from_only":0,"to_only":0},{"from_reflection_sha256":"` + strings.Repeat("b", 64) + `","to_reflection_sha256":"` + strings.Repeat("c", 64) + `","result":"incomparable","compared":1,"changed":0,"from_only":1,"to_only":2}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsSave(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           1,
		HistoryID:               "answer-state-transitions",
		HistoryQuestion:         "At which supplied boundaries did the bounded answer state change?",
		QuestionID:              "counterfactual-change",
		OrderBasis:              "caller",
		Snapshots:               2,
		Transitions:             1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsSave(
			[]string{"one.json", "two.json", "history.json"},
			&stdout,
			&stderr,
			func(paths []string, historyPath string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				if strings.Join(paths, ",") != "one.json,two.json" || historyPath != "history.json" {
					t.Fatalf("SaveArchiveQuestionTransitionHistory() arguments = %q, %q", paths, historyPath)
				}
				return summary, nil
			},
		)
		want := "archive reflection transitions saved\n" +
			"schema_version: 1\n" +
			"history_id: answer-state-transitions\n" +
			"history_question: At which supplied boundaries did the bounded answer state change?\n" +
			"question_id: counterfactual-change\n" +
			"order_basis: caller\n" +
			"snapshots: 2\n" +
			"transitions: 1\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsSave(
			[]string{"--json", "one.json", "two.json", "history.json"},
			&stdout,
			&stderr,
			func([]string, string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				return summary, nil
			},
		)
		want := `{"schema_version":1,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","order_basis":"caller","snapshots":2,"transitions":1,"transition_history_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsSaveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"one.json", "history.json"}, {"--json=invalid", "one.json", "two.json", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsSave(
				args,
				&stdout,
				&stderr,
				func([]string, string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
					t.Fatal("SaveArchiveQuestionTransitionHistory called for invalid usage")
					return bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("save error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsSave(
			[]string{"one.json", "two.json", "history.json"},
			&stdout,
			&stderr,
			func([]string, string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				return bundle.ArchiveQuestionTransitionVerificationSummary{}, errors.New("history path is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history path is invalid") {
			t.Fatalf("runAskArchiveTransitionsSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{
		{"one.json", "two.json", "history.json"},
		{"--json", "one.json", "two.json", "history.json"},
	} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsSave(
				args,
				failingWriter{},
				&stderr,
				func([]string, string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
					return bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsSave() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"one.json"}, {"--json=invalid", "one.json", "two.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitions(
				args,
				&stdout,
				&stderr,
				func([]string) (bundle.ArchiveQuestionTransitionHistory, error) {
					t.Fatal("CompareArchiveQuestionHistory called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistory{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("transition history error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitions(
			[]string{"one.json", "two.json"},
			&stdout,
			&stderr,
			func([]string) (bundle.ArchiveQuestionTransitionHistory, error) {
				return bundle.ArchiveQuestionTransitionHistory{}, errors.New("questions do not match")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "questions do not match") {
			t.Fatalf("runAskArchiveTransitions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{{"one.json", "two.json"}, {"--json", "one.json", "two.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitions(
				args,
				failingWriter{},
				&stderr,
				func([]string) (bundle.ArchiveQuestionTransitionHistory, error) {
					return bundle.ArchiveQuestionTransitionHistory{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitions() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"archive-root"}, {"archive-root", "question", "extra"}, {"--json=invalid", "archive-root", "question"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchive(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.ArchiveQuestionReport, error) {
					t.Fatal("AskArchive called for invalid usage")
					return bundle.ArchiveQuestionReport{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchive() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("archive error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchive(
			[]string{"archive-root", "question"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionReport, error) {
				return bundle.ArchiveQuestionReport{}, errors.New("archive unavailable")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "archive unavailable") {
			t.Fatalf("runAskArchive() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runAskArchive(
			[]string{"archive-root", "question"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.ArchiveQuestionReport, error) {
				return bundle.ArchiveQuestionReport{QuestionID: "question", Question: "question"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runAskArchive() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runAskArchive(
			[]string{"--json", "archive-root", "question"},
			failingWriter{},
			&stderr,
			func(string, string) (bundle.ArchiveQuestionReport, error) {
				return bundle.ArchiveQuestionReport{QuestionID: "question", Question: "question"}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runAskArchive() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsVerify(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           1,
		HistoryID:               "answer-state-transitions",
		HistoryQuestion:         "At which supplied boundaries did the bounded answer state change?",
		QuestionID:              "counterfactual-change",
		OrderBasis:              "caller",
		Snapshots:               3,
		Transitions:             2,
		TransitionHistorySHA256: strings.Repeat("a", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsVerify(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				if path != "history.json" {
					t.Fatalf("VerifyArchiveQuestionTransitionHistory() path = %q", path)
				}
				return summary, nil
			},
		)
		want := "archive reflection transitions structurally verified\n" +
			"schema_version: 1\n" +
			"history_id: answer-state-transitions\n" +
			"history_question: At which supplied boundaries did the bounded answer state change?\n" +
			"question_id: counterfactual-change\n" +
			"order_basis: caller\n" +
			"snapshots: 3\n" +
			"transitions: 2\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"note: this verifies the derived transition contract; it does not prove the underlying evidence or chronology\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsVerify(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) { return summary, nil },
		)
		want := `{"schema_version":1,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","order_basis":"caller","snapshots":3,"transitions":2,"transition_history_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsVerify(
			[]string{"--expect-sha256", strings.Repeat("a", 64), "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) { return summary, nil },
		)
		if exitCode != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsVerifyFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
					t.Fatal("VerifyArchiveQuestionTransitionHistory called for invalid usage")
					return bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("verification error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsVerify(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				return bundle.ArchiveQuestionTransitionVerificationSummary{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{
		{"--expect-sha256", "bad", "history.json"},
		{"--expect-sha256=", "history.json"},
	} {
		t.Run("invalid expected identity "+strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
					t.Fatal("VerifyArchiveQuestionTransitionHistory called for invalid expected identity")
					return bundle.ArchiveQuestionTransitionVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "expect-sha256 must be") {
				t.Fatalf("runAskArchiveTransitionsVerify() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	t.Run("mismatched expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsVerify(
			[]string{"--expect-sha256", strings.Repeat("b", 64), "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
				return summaryForTransitionVerifyTest(), nil
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "transition history SHA-256 mismatch") {
			t.Fatalf("runAskArchiveTransitionsVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsVerify(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error) {
					return summaryForTransitionVerifyTest(), nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsVerify() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func summaryForTransitionVerifyTest() bundle.ArchiveQuestionTransitionVerificationSummary {
	return bundle.ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           1,
		HistoryID:               "answer-state-transitions",
		HistoryQuestion:         "At which supplied boundaries did the bounded answer state change?",
		QuestionID:              "counterfactual-change",
		OrderBasis:              "caller",
		Snapshots:               2,
		Transitions:             1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
	}
}

func TestRunAskArchiveVerify(t *testing.T) {
	summary := bundle.ArchiveQuestionVerificationSummary{
		SchemaVersion:    2,
		QuestionID:       "counterfactual-change",
		Checked:          3,
		ReflectionSHA256: strings.Repeat("a", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"report.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionVerificationSummary, error) {
				if path != "report.json" {
					t.Fatalf("VerifyArchiveQuestionReport() path = %q", path)
				}
				return summary, nil
			},
		)
		want := "archive question report structurally verified\n" +
			"schema_version: 2\n" +
			"question_id: counterfactual-change\n" +
			"checked: 3\n" +
			"reflection_sha256: " + strings.Repeat("a", 64) + "\n" +
			"note: this identifies the canonical safe reflection content; it does not prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"--json", "report.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionVerificationSummary, error) { return summary, nil },
		)
		want := `{"schema_version":2,"question_id":"counterfactual-change","checked":3,"reflection_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		expected := strings.Repeat("a", 64)
		exitCode := runAskArchiveVerify(
			[]string{"--expect-sha256", expected, "report.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionVerificationSummary, error) {
				if path != "report.json" {
					t.Fatalf("VerifyArchiveQuestionReport() path = %q", path)
				}
				return summary, nil
			},
		)
		if exitCode != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveVerifyFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"report.json", "extra"}, {"--json=invalid", "report.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
					t.Fatal("VerifyArchiveQuestionReport called for invalid usage")
					return bundle.ArchiveQuestionVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("verification error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"report.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
				return bundle.ArchiveQuestionVerificationSummary{}, errors.New("report is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "report is invalid") {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("invalid expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"--expect-sha256", "bad", "report.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
				t.Fatal("VerifyArchiveQuestionReport called for invalid expected identity")
				return bundle.ArchiveQuestionVerificationSummary{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "expect-sha256 must be") {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("empty expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"--expect-sha256=", "report.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
				t.Fatal("VerifyArchiveQuestionReport called for empty expected identity")
				return bundle.ArchiveQuestionVerificationSummary{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "expect-sha256 must be") {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("mismatched expected identity", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveVerify(
			[]string{"--expect-sha256", strings.Repeat("b", 64), "report.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
				return bundle.ArchiveQuestionVerificationSummary{ReflectionSHA256: strings.Repeat("a", 64)}, nil
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "reflection SHA-256 mismatch") {
			t.Fatalf("runAskArchiveVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{{"report.json"}, {"--json", "report.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveVerify(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionVerificationSummary, error) {
					return bundle.ArchiveQuestionVerificationSummary{SchemaVersion: 2, QuestionID: "question"}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveVerify() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunQuestions(t *testing.T) {
	questions := []bundle.Question{
		{ID: "counterfactual-change", Text: "Did changing the declared variable influence an observed output?"},
		{ID: "capture-complete", Text: "Were all required observations captured for both sessions?"},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runQuestions(
			nil,
			&stdout,
			&stderr,
			func() []bundle.Question { return questions },
		)
		want := "question catalog\n" +
			"- id: counterfactual-change\n" +
			"  question: Did changing the declared variable influence an observed output?\n" +
			"- id: capture-complete\n" +
			"  question: Were all required observations captured for both sessions?\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runQuestions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runQuestions(
			[]string{"--json"},
			&stdout,
			&stderr,
			func() []bundle.Question { return questions },
		)
		want := `[{"id":"counterfactual-change","question":"Did changing the declared variable influence an observed output?"},{"id":"capture-complete","question":"Were all required observations captured for both sessions?"}]` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runQuestions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunQuestionsFailures(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"--json", "extra"}, {"--json=invalid"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runQuestions(
				args,
				&stdout,
				&stderr,
				func() []bundle.Question {
					t.Fatal("list called for invalid usage")
					return nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runQuestions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runQuestions(nil, failingWriter{}, &stderr, func() []bundle.Question {
			return []bundle.Question{{ID: "question", Text: "question"}}
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runQuestions() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runQuestions([]string{"--json"}, failingWriter{}, &stderr, func() []bundle.Question {
			return []bundle.Question{{ID: "question", Text: "question"}}
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runQuestions() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunList(t *testing.T) {
	entries := []bundle.ArchiveEntry{
		{Directory: "a-run", ManifestName: "experiment-001-email", Differences: 1},
		{Directory: "z-run", ManifestName: "experiment-001-email", Unknowns: 3},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runList(
			[]string{"archive-root"},
			&stdout,
			&stderr,
			func(root string) ([]bundle.ArchiveEntry, error) {
				if root != "archive-root" {
					t.Fatalf("Index() root = %q", root)
				}
				return entries, nil
			},
		)
		want := "archived bundles\n" +
			"- directory: a-run\n" +
			"  manifest_name: experiment-001-email\n" +
			"  differences: 1\n" +
			"  unknowns: 0\n" +
			"- directory: z-run\n" +
			"  manifest_name: experiment-001-email\n" +
			"  differences: 0\n" +
			"  unknowns: 3\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runList() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runList(
			[]string{"--json", "archive-root"},
			&stdout,
			&stderr,
			func(string) ([]bundle.ArchiveEntry, error) { return entries, nil },
		)
		want := `[{"directory":"a-run","manifest_name":"experiment-001-email","differences":1,"unknowns":0},{"directory":"z-run","manifest_name":"experiment-001-email","differences":0,"unknowns":3}]` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runList() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunListFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"archive-root", "extra"}, {"--json=invalid", "archive-root"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runList(
				args,
				&stdout,
				&stderr,
				func(string) ([]bundle.ArchiveEntry, error) {
					t.Fatal("Index called for invalid usage")
					return nil, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runList() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("archive error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runList(
			[]string{"archive-root"},
			&stdout,
			&stderr,
			func(string) ([]bundle.ArchiveEntry, error) { return nil, errors.New("archive entry is invalid") },
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "archive entry is invalid") {
			t.Fatalf("runList() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("write output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runList(
			[]string{"archive-root"},
			failingWriter{},
			&stderr,
			func(string) ([]bundle.ArchiveEntry, error) {
				return []bundle.ArchiveEntry{{Directory: "run"}}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runList() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("write json output", func(t *testing.T) {
		var stderr bytes.Buffer
		exitCode := runList(
			[]string{"--json", "archive-root"},
			failingWriter{},
			&stderr,
			func(string) ([]bundle.ArchiveEntry, error) {
				return []bundle.ArchiveEntry{{Directory: "run"}}, nil
			},
		)
		if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
			t.Fatalf("runList() = %d, stderr=%q", exitCode, stderr.String())
		}
	})
}

func TestRunServe(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var address string
	var handler http.Handler
	exitCode := runServe(
		[]string{"--addr", "127.0.0.1:9090", "archive-root"},
		&stdout,
		&stderr,
		func(gotAddress string, gotHandler http.Handler) error {
			address = gotAddress
			handler = gotHandler
			return nil
		},
	)
	if exitCode != 0 || address != "127.0.0.1:9090" || handler == nil ||
		stdout.String() != "ariadne: review UI listening at http://127.0.0.1:9090/\n" || stderr.Len() != 0 {
		t.Fatalf("runServe() = %d, address=%q, handler=%v, stdout=%q, stderr=%q", exitCode, address, handler, stdout.String(), stderr.String())
	}

	t.Run("history flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--history", "history.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with history = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("reflection flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--reflection", "reflection.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with reflection = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("export flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--export", "export.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with export = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})
}

func TestRunServeFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"archive-root", "extra"}, {"--unknown", "archive-root"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runServe(args, &stdout, &stderr, func(string, http.Handler) error {
				t.Fatal("server called for invalid usage")
				return nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("server error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runServe([]string{"archive-root"}, &stdout, &stderr, func(string, http.Handler) error {
			return errors.New("listen failed")
		})
		if exitCode != 1 || !strings.Contains(stderr.String(), "listen failed") || stdout.Len() == 0 {
			t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("non-loopback address", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runServe([]string{"--addr", "0.0.0.0:9090", "archive-root"}, &stdout, &stderr, func(string, http.Handler) error {
			t.Fatal("server called for non-loopback address")
			return nil
		})
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment serve: address must use a loopback IP\n" {
			t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestLoopbackAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8787", "[::1]:8787"} {
		if !loopbackAddress(address) {
			t.Errorf("loopbackAddress(%q) = false", address)
		}
	}
	for _, address := range []string{"0.0.0.0:8787", "192.168.1.10:8787", ":8787", "127.0.0.1"} {
		if loopbackAddress(address) {
			t.Errorf("loopbackAddress(%q) = true", address)
		}
	}
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
