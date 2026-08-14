package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		{"trace"},
		{"trace", "unknown"},
		{"trace", "verify"},
		{"trace", "compare"},
		{"trace", "study"},
		{"trace", "study", "save"},
		{"trace", "study", "verify"},
		{"browser"},
		{"browser", "unknown"},
		{"browser", "trace"},
		{"experiment"},
		{"experiment", "unknown"},
		{"experiment", "export"},
		{"experiment", "export", "verify"},
		{"experiment", "export", "ask"},
		{"experiment", "export", "finding"},
		{"experiment", "replicate"},
		{"experiment", "replicate", "verify"},
		{"experiment", "verify"},
		{"experiment", "finding"},
		{"experiment", "ask"},
		{"experiment", "ask-archive"},
		{"experiment", "ask-archive", "save"},
		{"experiment", "ask-archive", "compare"},
		{"experiment", "ask-archive", "compare-current"},
		{"experiment", "ask-archive", "transitions"},
		{"experiment", "ask-archive", "transitions", "questions", "extra"},
		{"experiment", "ask-archive", "transitions", "ask", "repeated"},
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

func TestRunBrowserTrace(t *testing.T) {
	input := filepath.Join(t.TempDir(), "browser-audit.json")
	data := []byte(`{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[{"channel":"network","kind":"request","destination":"analytics","fields":["region"]}]}`)
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json-%t", jsonOutput), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			output := filepath.Join(t.TempDir(), "browser-trace.json")
			args := []string{"browser", "trace", input, output}
			if jsonOutput {
				args = []string{"browser", "trace", "--json", input, output}
			}
			if exitCode := run(args, &stdout, &stderr); exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("run() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if jsonOutput {
				var summary map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
					t.Fatal(err)
				}
				if summary["scope"] != "outbound" || summary["events"] != float64(1) {
					t.Fatalf("summary = %#v", summary)
				}
			} else if !strings.Contains(stdout.String(), "browser trace complete") || !strings.Contains(stdout.String(), "events: 1") {
				t.Fatalf("human output = %q", stdout.String())
			}
		})
	}
}

func TestRunArchiveTransitionQuestions(t *testing.T) {
	for _, args := range [][]string{nil, {"--json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run(append([]string{"experiment", "ask-archive", "transitions", "questions"}, args...), &stdout, &stderr)
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("run() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if args == nil {
				want := "question catalog\n" +
					"- id: answer-state-transitions\n" +
					"  question: At which supplied boundaries did the bounded answer state change?\n" +
					"- id: answer-state-repeated-changes\n" +
					"  question: Did any safe archive entry change at more than one supplied boundary?\n" +
					"- id: answer-state-snapshot-summaries\n" +
					"  question: What bounded answer-state summary did each supplied reflection snapshot record?\n" +
					"- id: answer-state-summary-changes\n" +
					"  question: Did the bounded answer-state summary change at any supplied boundary?\n"
				if stdout.String() != want {
					t.Fatalf("run() stdout = %q, want %q", stdout.String(), want)
				}
				return
			}
			want := `[{"id":"answer-state-transitions","question":"At which supplied boundaries did the bounded answer state change?"},{"id":"answer-state-repeated-changes","question":"Did any safe archive entry change at more than one supplied boundary?"},{"id":"answer-state-snapshot-summaries","question":"What bounded answer-state summary did each supplied reflection snapshot record?"},{"id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?"}]` + "\n"
			if stdout.String() != want {
				t.Fatalf("run() stdout = %q, want %q", stdout.String(), want)
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

func TestRunExperimentReplicate(t *testing.T) {
	path := writeManifest(t, validManifest)
	outputDir := filepath.Join(t.TempDir(), "replicated")
	var stdout, stderr bytes.Buffer
	check := func(
		ctx context.Context,
		binary, device, packageName string,
	) (adb.Target, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check() context has no deadline")
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
	runReplicated := func(
		ctx context.Context,
		binary string,
		target adb.Target,
		manifest experiment.Manifest,
		output string,
		pairs int,
	) error {
		if _, ok := ctx.Deadline(); !ok ||
			binary != "custom-adb" ||
			target.Device != "emulator-5554" ||
			manifest.Name != "experiment-001-email" ||
			output != outputDir || pairs != 2 {
			t.Fatalf("runReplicated() arguments = %#v, %q, %#v, %q, %d", ctx, binary, target, output, pairs)
		}
		return nil
	}

	exitCode := runExperimentReplicate(
		[]string{
			"--adb", "custom-adb",
			"--device", "emulator-5554",
			"--package", "dev.ariadne.fixture",
			"--pairs", "2",
			"--output", outputDir,
			path,
		},
		&stdout,
		&stderr,
		check,
		runReplicated,
	)
	want := "experiment replication complete\n" +
		"name: experiment-001-email\n" +
		"pairs_per_order: 2\n" +
		"runs: 4\n" +
		"order: baseline-treatment, treatment-baseline\n" +
		"reset_policy: reset-before-each-session\n"
	if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("runExperimentReplicate() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunExperimentReplicateUsage(t *testing.T) {
	tests := [][]string{
		nil,
		{"--device", "emulator-5554", "--package", "dev.ariadne.fixture", "--output", "run", "manifest.json"},
		{"--device", "emulator-5554", "--package", "dev.ariadne.fixture", "--pairs", "0", "--output", "run", "manifest.json"},
		{"--device", "emulator-5554", "--package", "dev.ariadne.fixture", "--pairs", "1", "--output", "run"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runExperimentReplicate(
				args,
				&stdout,
				&stderr,
				func(context.Context, string, string, string) (adb.Target, error) {
					t.Fatal("check called for invalid usage")
					return adb.Target{}, nil
				},
				nil,
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runExperimentReplicate() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunReplicateVerify(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json-%t", jsonOutput), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"replicated"}
			if jsonOutput {
				args = []string{"--json", "replicated"}
			}
			exitCode := runReplicateVerify(
				args,
				&stdout,
				&stderr,
				func(path string) (bundle.ReplicatedExperimentSummary, error) {
					if path != "replicated" {
						t.Fatalf("verify path = %q", path)
					}
					return bundle.ReplicatedExperimentSummary{
						ManifestName:   "experiment-001-email",
						Outcome:        bundle.ReplicatedChange,
						EvidenceState:  "observed",
						Pairs:          2,
						PairsPerOrder:  1,
						CompletedPairs: 2,
						ChangedPairs:   2,
					}, nil
				},
			)
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("runReplicateVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if jsonOutput {
				var got bundle.ReplicatedExperimentSummary
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Outcome != bundle.ReplicatedChange || got.ChangedPairs != 2 {
					t.Fatalf("JSON summary = %#v", got)
				}
			} else if !strings.Contains(stdout.String(), "outcome: replicated-change") ||
				!strings.Contains(stdout.String(), "evidence_state: observed") {
				t.Fatalf("human summary = %q", stdout.String())
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
		SchemaVersion:         2,
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
		StateChanges: []bundle.ArchiveQuestionStateChange{{
			Directory:  "a-run",
			OlderState: "observed",
			NewerState: "unknown",
		}},
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
			"state_changes:\n" +
			"- directory: a-run\n" +
			"  older_state: observed\n" +
			"  newer_state: unknown\n" +
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
		want := `{"schema_version":2,"comparison_id":"answer-state-change","comparison_question":"Did the bounded answer state change between these saved reflection snapshots?","question_id":"counterfactual-change","question":"Did it change?","result":"changed","older_reflection_sha256":"` + strings.Repeat("a", 64) + `","newer_reflection_sha256":"` + strings.Repeat("b", 64) + `","compared":2,"changed":1,"older_only":0,"newer_only":1,"state_changes":[{"directory":"a-run","older_state":"observed","newer_state":"unknown"}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveCompare() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveCompareWriteFailure(t *testing.T) {
	comparison := bundle.ArchiveQuestionComparison{
		StateChanges: []bundle.ArchiveQuestionStateChange{{
			Directory:  "a-run",
			OlderState: "observed",
			NewerState: "unknown",
		}},
	}
	for _, test := range []struct {
		name   string
		failAt int
	}{
		{name: "state changes header", failAt: 2},
		{name: "state change entry", failAt: 3},
		{name: "note", failAt: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveCompare(
				[]string{"older.json", "newer.json"},
				&failAfterWriter{failAt: test.failAt},
				&stderr,
				func(string, string) (bundle.ArchiveQuestionComparison, error) { return comparison, nil },
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveCompare() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveCompareCurrent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	comparison := bundle.ArchiveQuestionComparison{
		SchemaVersion:         2,
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
		SchemaVersion:   3,
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		QuestionID:      "counterfactual-change",
		Question:        "Did it change?",
		OrderBasis:      "caller",
		Snapshots:       3,
		SnapshotSummaries: []bundle.ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("c", 64), Observed: 1, Unavailable: 1, Checked: 2},
		},
		Transitions: []bundle.ArchiveQuestionTransition{
			{
				FromReflectionSHA256: strings.Repeat("a", 64),
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              1,
				StateChanges: []bundle.ArchiveQuestionStateChange{{
					Directory:  "run-001",
					OlderState: "observed",
					NewerState: "unknown",
				}},
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
			"snapshot_summaries:\n" +
			"- snapshot: 1\n" +
			"  reflection_sha256: " + strings.Repeat("a", 64) + "\n" +
			"  observed: 1\n" +
			"  unknown: 0\n" +
			"  unavailable: 0\n" +
			"  checked: 1\n" +
			"- snapshot: 2\n" +
			"  reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  observed: 0\n" +
			"  unknown: 1\n" +
			"  unavailable: 0\n" +
			"  checked: 1\n" +
			"- snapshot: 3\n" +
			"  reflection_sha256: " + strings.Repeat("c", 64) + "\n" +
			"  observed: 1\n" +
			"  unknown: 0\n" +
			"  unavailable: 1\n" +
			"  checked: 2\n" +
			"- transition: 1\n" +
			"  from_reflection_sha256: " + strings.Repeat("a", 64) + "\n" +
			"  to_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  result: changed\n" +
			"  compared: 2\n" +
			"  changed: 1\n" +
			"  from_only: 0\n" +
			"  to_only: 0\n" +
			"  state_changes:\n" +
			"  - directory: run-001\n" +
			"    older_state: observed\n" +
			"    newer_state: unknown\n" +
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
		want := `{"schema_version":3,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","question":"Did it change?","order_basis":"caller","snapshots":3,"transitions":[{"from_reflection_sha256":"` + strings.Repeat("a", 64) + `","to_reflection_sha256":"` + strings.Repeat("b", 64) + `","result":"changed","compared":2,"changed":1,"from_only":0,"to_only":0,"state_changes":[{"directory":"run-001","older_state":"observed","newer_state":"unknown"}]},{"from_reflection_sha256":"` + strings.Repeat("b", 64) + `","to_reflection_sha256":"` + strings.Repeat("c", 64) + `","result":"incomparable","compared":1,"changed":0,"from_only":1,"to_only":2}],"snapshot_summaries":[{"reflection_sha256":"` + strings.Repeat("a", 64) + `","observed":1,"unknown":0,"unavailable":0,"checked":1},{"reflection_sha256":"` + strings.Repeat("b", 64) + `","observed":0,"unknown":1,"unavailable":0,"checked":1},{"reflection_sha256":"` + strings.Repeat("c", 64) + `","observed":1,"unknown":0,"unavailable":1,"checked":2}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitions() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAsk(t *testing.T) {
	answer := bundle.ArchiveQuestionTransitionHistoryAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-transitions",
		Question:                "At which supplied boundaries did the bounded answer state change?",
		Result:                  "changed",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             3,
		ChangedTransitions:      []int{1, 3},
		IncomparableTransitions: []int{2, 3},
		ChangedEntries: []bundle.ArchiveQuestionTransitionHistoryChange{
			{Transition: 1, FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64), Directory: "run-001", OlderState: "observed", NewerState: "unknown"},
			{Transition: 3, FromReflectionSHA256: strings.Repeat("d", 64), ToReflectionSHA256: strings.Repeat("e", 64), Directory: "run-002", OlderState: "unknown", NewerState: "unavailable"},
		},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAsk(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				if path != "history.json" {
					t.Fatalf("AskArchiveQuestionTransitionHistory() path = %q", path)
				}
				return answer, nil
			},
		)
		want := "archive reflection history question answered\n" +
			"question_id: answer-state-transitions\n" +
			"question: At which supplied boundaries did the bounded answer state change?\n" +
			"result: changed\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"transitions: 3\n" +
			"changed_transitions:\n" +
			"- 1\n" +
			"- 3\n" +
			"changed_entries:\n" +
			"- transition: 1\n" +
			"  from_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  to_reflection_sha256: " + strings.Repeat("c", 64) + "\n" +
			"  directory: run-001\n" +
			"  older_state: observed\n" +
			"  newer_state: unknown\n" +
			"- transition: 3\n" +
			"  from_reflection_sha256: " + strings.Repeat("d", 64) + "\n" +
			"  to_reflection_sha256: " + strings.Repeat("e", 64) + "\n" +
			"  directory: run-002\n" +
			"  older_state: unknown\n" +
			"  newer_state: unavailable\n" +
			"incomparable_transitions:\n" +
			"- 2\n" +
			"- 3\n" +
			"note: this answers only the verified history structure; it does not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAsk(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) { return answer, nil },
		)
		want := `{"schema_version":1,"question_id":"answer-state-transitions","question":"At which supplied boundaries did the bounded answer state change?","result":"changed","transition_history_sha256":"` + strings.Repeat("a", 64) + `","transitions":3,"changed_transitions":[1,3],"incomparable_transitions":[2,3],"changed_entries":[{"transition":1,"from_reflection_sha256":"` + strings.Repeat("b", 64) + `","to_reflection_sha256":"` + strings.Repeat("c", 64) + `","directory":"run-001","older_state":"observed","newer_state":"unknown"},{"transition":3,"from_reflection_sha256":"` + strings.Repeat("d", 64) + `","to_reflection_sha256":"` + strings.Repeat("e", 64) + `","directory":"run-002","older_state":"unknown","newer_state":"unavailable"}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskQuestion(t *testing.T) {
	transitionAnswer := bundle.ArchiveQuestionTransitionHistoryAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-transitions",
		Question:                "At which supplied boundaries did the bounded answer state change?",
		Result:                  "same",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             2,
		ChangedTransitions:      []int{},
		IncomparableTransitions: []int{},
		ChangedEntries:          []bundle.ArchiveQuestionTransitionHistoryChange{},
	}
	repeatedAnswer := bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-repeated-changes",
		Question:                "Did any safe archive entry change at more than one supplied boundary?",
		Result:                  "none",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             2,
		RepeatedEntries:         []bundle.ArchiveQuestionTransitionHistoryRepeatedChange{},
	}
	snapshotAnswer := bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-snapshot-summaries",
		Question:                "What bounded answer-state summary did each supplied reflection snapshot record?",
		Result:                  "available",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Snapshots:               2,
		SnapshotSummaries: []bundle.ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
		},
	}
	summaryAnswer := bundle.ArchiveQuestionTransitionHistorySummaryAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-summary-changes",
		Question:                "Did the bounded answer-state summary change at any supplied boundary?",
		Result:                  "changed",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             2,
		ChangedTransitions:      []int{1},
	}

	t.Run("transition ID", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskQuestion(
			[]string{"history.json", "answer-state-transitions"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				if path != "history.json" {
					t.Fatalf("transition asker path = %q", path)
				}
				return transitionAnswer, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				t.Fatal("repeated asker called for transition question")
				return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				t.Fatal("snapshot asker called for transition question")
				return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				t.Fatal("summary asker called for transition question")
				return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
			},
		)
		if exitCode != 0 || !strings.Contains(stdout.String(), "question_id: answer-state-transitions") || stderr.Len() != 0 {
			t.Fatalf("transition ID ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("repeated ID JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskQuestion(
			[]string{"--json", "history.json", "answer-state-repeated-changes"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				t.Fatal("transition asker called for repeated question")
				return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
			},
			func(path string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				if path != "history.json" {
					t.Fatalf("repeated asker path = %q", path)
				}
				return repeatedAnswer, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				t.Fatal("snapshot asker called for repeated question")
				return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				t.Fatal("summary asker called for repeated question")
				return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
			},
		)
		if exitCode != 0 || !strings.Contains(stdout.String(), `"question_id":"answer-state-repeated-changes"`) || !strings.Contains(stdout.String(), `"repeated_entries":[]`) || stderr.Len() != 0 {
			t.Fatalf("repeated ID ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("snapshot ID JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskQuestion(
			[]string{"--json", "history.json", "answer-state-snapshot-summaries"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				t.Fatal("transition asker called for snapshot question")
				return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				t.Fatal("repeated asker called for snapshot question")
				return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
			},
			func(path string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				if path != "history.json" {
					t.Fatalf("snapshot asker path = %q", path)
				}
				return snapshotAnswer, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				t.Fatal("summary asker called for snapshot question")
				return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
			},
		)
		if exitCode != 0 || !strings.Contains(stdout.String(), `"question_id":"answer-state-snapshot-summaries"`) || !strings.Contains(stdout.String(), `"snapshot_summaries":[`) || stderr.Len() != 0 {
			t.Fatalf("snapshot ID ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("summary ID JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskQuestion(
			[]string{"--json", "history.json", "answer-state-summary-changes"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				t.Fatal("transition asker called for summary question")
				return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				t.Fatal("repeated asker called for summary question")
				return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				t.Fatal("snapshot asker called for summary question")
				return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
			},
			func(path string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				if path != "history.json" {
					t.Fatalf("summary asker path = %q", path)
				}
				return summaryAnswer, nil
			},
		)
		if exitCode != 0 || !strings.Contains(stdout.String(), `"question_id":"answer-state-summary-changes"`) || !strings.Contains(stdout.String(), `"changed_transitions":[1]`) || stderr.Len() != 0 {
			t.Fatalf("summary ID ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("unknown ID", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskQuestion(
			[]string{"history.json", "not-a-question"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				t.Fatal("transition asker called for unknown question")
				return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				t.Fatal("repeated asker called for unknown question")
				return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				t.Fatal("snapshot asker called for unknown question")
				return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
			},
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				t.Fatal("summary asker called for unknown question")
				return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
			},
		)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment ask-archive transitions ask: question ID is invalid\n" {
			t.Fatalf("unknown ID ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("run routes ID", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := run([]string{"experiment", "ask-archive", "transitions", "ask", "history.json", "not-a-question"}, &stdout, &stderr)
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment ask-archive transitions ask: question ID is invalid\n" {
			t.Fatalf("run question ID route = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskSnapshots(t *testing.T) {
	answer := bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-snapshot-summaries",
		Question:                "What bounded answer-state summary did each supplied reflection snapshot record?",
		Result:                  "available",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Snapshots:               2,
		SnapshotSummaries: []bundle.ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("b", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("c", 64), Unknown: 1, Unavailable: 1, Checked: 2},
		},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSnapshots(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				if path != "history.json" {
					t.Fatalf("AskArchiveQuestionTransitionHistorySnapshots() path = %q", path)
				}
				return answer, nil
			},
		)
		want := "archive reflection snapshot-summary question answered\n" +
			"question_id: answer-state-snapshot-summaries\n" +
			"question: What bounded answer-state summary did each supplied reflection snapshot record?\n" +
			"result: available\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"snapshots: 2\n" +
			"snapshot_summaries:\n" +
			"- snapshot: 1\n" +
			"  reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"  observed: 1\n" +
			"  unknown: 0\n" +
			"  unavailable: 0\n" +
			"  checked: 1\n" +
			"- snapshot: 2\n" +
			"  reflection_sha256: " + strings.Repeat("c", 64) + "\n" +
			"  observed: 0\n" +
			"  unknown: 1\n" +
			"  unavailable: 1\n" +
			"  checked: 2\n" +
			"note: this answers only safe snapshot summaries; it does not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSnapshots(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) { return answer, nil },
		)
		want := `{"schema_version":1,"question_id":"answer-state-snapshot-summaries","question":"What bounded answer-state summary did each supplied reflection snapshot record?","result":"available","transition_history_sha256":"` + strings.Repeat("a", 64) + `","snapshots":2,"snapshot_summaries":[{"reflection_sha256":"` + strings.Repeat("b", 64) + `","observed":1,"unknown":0,"unavailable":0,"checked":1},{"reflection_sha256":"` + strings.Repeat("c", 64) + `","observed":0,"unknown":1,"unavailable":1,"checked":2}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskSnapshotsFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSnapshots(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
					t.Fatal("AskArchiveQuestionTransitionHistorySnapshots called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSnapshots(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
				return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSnapshots(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	for _, failAt := range []int{2, 3, 4} {
		t.Run(fmt.Sprintf("write detail output %d", failAt), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSnapshots(
				[]string{"history.json"},
				&failAfterWriter{failAt: failAt},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistorySnapshotAnswer{
						QuestionID: "answer-state-snapshot-summaries",
						Question:   "What bounded answer-state summary did each supplied reflection snapshot record?",
						Result:     "available",
						SnapshotSummaries: []bundle.ArchiveQuestionTransitionSnapshot{
							{ReflectionSHA256: strings.Repeat("b", 64)},
							{ReflectionSHA256: strings.Repeat("c", 64)},
						},
					}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskSnapshots() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskAll(t *testing.T) {
	round := bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer{
		SchemaVersion:           2,
		HistoryQuestionID:       "counterfactual-change",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Questions: []bundle.ArchiveQuestionTransitionHistoryQuestionRoundItem{
			{QuestionID: "answer-state-transitions", Question: "At which supplied boundaries did the bounded answer state change?", Result: "changed"},
			{QuestionID: "answer-state-repeated-changes", Question: "Did any safe archive entry change at more than one supplied boundary?", Result: "none"},
			{QuestionID: "answer-state-snapshot-summaries", Question: "What bounded answer-state summary did each supplied reflection snapshot record?", Result: "available"},
			{QuestionID: "answer-state-summary-changes", Question: "Did the bounded answer-state summary change at any supplied boundary?", Result: "changed"},
		},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAll(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
				if path != "history.json" {
					t.Fatalf("AskArchiveQuestionTransitionHistoryQuestionRound() path = %q", path)
				}
				return round, nil
			},
		)
		want := "archive reflection question round answered\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"questions:\n" +
			"- question_id: answer-state-transitions\n" +
			"  question: At which supplied boundaries did the bounded answer state change?\n" +
			"  result: changed\n" +
			"- question_id: answer-state-repeated-changes\n" +
			"  question: Did any safe archive entry change at more than one supplied boundary?\n" +
			"  result: none\n" +
			"- question_id: answer-state-snapshot-summaries\n" +
			"  question: What bounded answer-state summary did each supplied reflection snapshot record?\n" +
			"  result: available\n" +
			"- question_id: answer-state-summary-changes\n" +
			"  question: Did the bounded answer-state summary change at any supplied boundary?\n" +
			"  result: changed\n" +
			"note: this records fixed bounded question results only; inspect an individual question for details, and do not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAll(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) { return round, nil },
		)
		want := `{"schema_version":2,"history_question_id":"counterfactual-change","transition_history_sha256":"` + strings.Repeat("a", 64) + `","questions":[{"question_id":"answer-state-transitions","question":"At which supplied boundaries did the bounded answer state change?","result":"changed"},{"question_id":"answer-state-repeated-changes","question":"Did any safe archive entry change at more than one supplied boundary?","result":"none"},{"question_id":"answer-state-snapshot-summaries","question":"What bounded answer-state summary did each supplied reflection snapshot record?","result":"available"},{"question_id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?","result":"changed"}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskAllFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAll(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
					t.Fatal("AskArchiveQuestionTransitionHistoryQuestionRound called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAll(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
				return bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAll(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	for _, failAt := range []int{2, 6} {
		t.Run(fmt.Sprintf("write detail output %d", failAt), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAll(
				[]string{"history.json"},
				&failAfterWriter{failAt: failAt},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer{
						Questions: []bundle.ArchiveQuestionTransitionHistoryQuestionRoundItem{{QuestionID: "answer-state-transitions", Question: "At which supplied boundaries did the bounded answer state change?", Result: "same"}, {QuestionID: "answer-state-repeated-changes", Question: "Did any safe archive entry change at more than one supplied boundary?", Result: "none"}, {QuestionID: "answer-state-snapshot-summaries", Question: "What bounded answer-state summary did each supplied reflection snapshot record?", Result: "available"}, {QuestionID: "answer-state-summary-changes", Question: "Did the bounded answer-state summary change at any supplied boundary?", Result: "same"}},
					}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskAll() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskAllSave(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{
		SchemaVersion:           1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Questions:               4,
		RoundSHA256:             strings.Repeat("b", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllSave(
			[]string{"history.json", "round.json"},
			&stdout,
			&stderr,
			func(historyPath, roundPath string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				if historyPath != "history.json" || roundPath != "round.json" {
					t.Fatalf("SaveArchiveQuestionTransitionHistoryQuestionRound() args = %q, %q", historyPath, roundPath)
				}
				return summary, nil
			},
		)
		want := "archive reflection question round saved\n" +
			"schema_version: 1\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"questions: 4\n" +
			"round_sha256: " + strings.Repeat("b", 64) + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllSave(
			[]string{"--json", "history.json", "round.json"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				return summary, nil
			},
		)
		want := `{"schema_version":1,"transition_history_sha256":"` + strings.Repeat("a", 64) + `","questions":4,"round_sha256":"` + strings.Repeat("b", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskAllSaveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json"}, {"history.json", "round.json", "extra"}, {"--json=invalid", "history.json", "round.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllSave(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
					t.Fatal("SaveArchiveQuestionTransitionHistoryQuestionRound called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskAllSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("save error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllSave(
			[]string{"history.json", "round.json"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, errors.New("round path exists")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "round path exists") {
			t.Fatalf("runAskArchiveTransitionsAskAllSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json", "round.json"}, {"--json", "history.json", "round.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllSave(
				args,
				failingWriter{},
				&stderr,
				func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
					return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskAllSave() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskAllVerify(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{
		SchemaVersion:           1,
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Questions:               4,
		RoundSHA256:             strings.Repeat("b", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllVerify(
			[]string{"--expect-sha256", strings.Repeat("b", 64), "round.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				if path != "round.json" {
					t.Fatalf("VerifyArchiveQuestionTransitionHistoryQuestionRound() path = %q", path)
				}
				return summary, nil
			},
		)
		want := "archive reflection question round structurally verified\n" +
			"schema_version: 1\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"questions: 4\n" +
			"round_sha256: " + strings.Repeat("b", 64) + "\n" +
			"note: this verifies the raw-value-free question round contract; it does not re-verify the transition history or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllVerify(
			[]string{"--json", "round.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				return summary, nil
			},
		)
		want := `{"schema_version":1,"transition_history_sha256":"` + strings.Repeat("a", 64) + `","questions":4,"round_sha256":"` + strings.Repeat("b", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskAllVerifyFailures(t *testing.T) {
	verify := func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
		return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{
			SchemaVersion: 1, TransitionHistorySHA256: strings.Repeat("a", 64), Questions: 4, RoundSHA256: strings.Repeat("b", 64),
		}, nil
	}
	for _, args := range [][]string{nil, {"round.json", "extra"}, {"--json=invalid", "round.json"}, {"--expect-sha256", "bad", "round.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllVerify(args, &stdout, &stderr, func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
				t.Fatal("VerifyArchiveQuestionTransitionHistoryQuestionRound called for invalid usage")
				return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage && !strings.Contains(stderr.String(), "expect-sha256") {
				t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("verify error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllVerify([]string{"round.json"}, &stdout, &stderr, func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error) {
			return bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary{}, errors.New("round is invalid")
		})
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "round is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskAllVerify([]string{"--expect-sha256", strings.Repeat("c", 64), "round.json"}, &stdout, &stderr, verify)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "SHA-256 mismatch") {
			t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	for _, args := range [][]string{{"round.json"}, {"--json", "round.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskAllVerify(args, failingWriter{}, &stderr, verify)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskAllVerify() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskReceipt(t *testing.T) {
	receipt := bundle.ArchiveQuestionTransitionHistoryAnswerReceipt{
		SchemaVersion:           1,
		QuestionID:              "answer-state-summary-changes",
		Question:                "Did the bounded answer-state summary change at any supplied boundary?",
		Result:                  "changed",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Answer:                  json.RawMessage(`{"schema_version":1,"result":"changed","changed_transitions":[1]}`),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceipt(
			[]string{"history.json", "answer-state-summary-changes"},
			&stdout,
			&stderr,
			func(path, questionID string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
				if path != "history.json" || questionID != "answer-state-summary-changes" {
					t.Fatalf("AskArchiveQuestionTransitionHistoryReceipt() args = %q, %q", path, questionID)
				}
				return receipt, nil
			},
		)
		want := "archive reflection answer receipt\n" +
			"schema_version: 1\n" +
			"question_id: answer-state-summary-changes\n" +
			"question: Did the bounded answer-state summary change at any supplied boundary?\n" +
			"result: changed\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"note: use --json for the portable raw-value-free answer details; this does not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceipt() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceipt(
			[]string{"--json", "history.json", "answer-state-summary-changes"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
				return receipt, nil
			},
		)
		want := `{"schema_version":1,"question_id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?","result":"changed","transition_history_sha256":"` + strings.Repeat("a", 64) + `","answer":{"schema_version":1,"result":"changed","changed_transitions":[1]}}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceipt() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskReceiptFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json"}, {"history.json", "question", "extra"}, {"--json=invalid", "history.json", "question"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceipt(
				args,
				&stdout,
				&stderr,
				func(string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
					t.Fatal("AskArchiveQuestionTransitionHistoryReceipt called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceipt{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskReceipt() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceipt(
			[]string{"history.json", "question"},
			&stdout,
			&stderr,
			func(string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
				return bundle.ArchiveQuestionTransitionHistoryAnswerReceipt{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskReceipt() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json", "question"}, {"--json", "history.json", "question"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceipt(
				args,
				failingWriter{},
				&stderr,
				func(string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error) {
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceipt{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskReceipt() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskReceiptSave(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary{
		SchemaVersion:           1,
		QuestionID:              "answer-state-summary-changes",
		Question:                "Did the bounded answer-state summary change at any supplied boundary?",
		Result:                  "same",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		ReceiptSHA256:           strings.Repeat("b", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptSave(
			[]string{"history.json", "answer-state-summary-changes", "receipt.json"},
			&stdout,
			&stderr,
			func(historyPath, questionID, receiptPath string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error) {
				if historyPath != "history.json" || questionID != "answer-state-summary-changes" || receiptPath != "receipt.json" {
					t.Fatalf("SaveArchiveQuestionTransitionHistoryAnswerReceipt() args = %q, %q, %q", historyPath, questionID, receiptPath)
				}
				return summary, nil
			},
		)
		want := "archive reflection answer receipt saved\n" +
			"schema_version: 1\n" +
			"question_id: answer-state-summary-changes\n" +
			"question: Did the bounded answer-state summary change at any supplied boundary?\n" +
			"result: same\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"receipt_sha256: " + strings.Repeat("b", 64) + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceiptSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptSave(
			[]string{"--json", "history.json", "answer-state-summary-changes", "receipt.json"},
			&stdout,
			&stderr,
			func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error) {
				return summary, nil
			},
		)
		want := `{"schema_version":1,"question_id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?","result":"same","transition_history_sha256":"` + strings.Repeat("a", 64) + `","receipt_sha256":"` + strings.Repeat("b", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceiptSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskReceiptSaveFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "question"}, {"history.json", "question", "receipt.json", "extra"}, {"--json=invalid", "history.json", "question", "receipt.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceiptSave(
				args,
				&stdout,
				&stderr,
				func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error) {
					t.Fatal("SaveArchiveQuestionTransitionHistoryAnswerReceipt called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskReceiptSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("save error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptSave(
			[]string{"history.json", "question", "receipt.json"},
			&stdout,
			&stderr,
			func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error) {
				return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary{}, errors.New("receipt path exists")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "receipt path exists") {
			t.Fatalf("runAskArchiveTransitionsAskReceiptSave() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json", "question", "receipt.json"}, {"--json", "history.json", "question", "receipt.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceiptSave(
				args,
				failingWriter{},
				&stderr,
				func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error) {
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskReceiptSave() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskReceiptVerify(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{
		SchemaVersion:           1,
		QuestionID:              "answer-state-summary-changes",
		Question:                "Did the bounded answer-state summary change at any supplied boundary?",
		Result:                  "same",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		ReceiptSHA256:           strings.Repeat("b", 64),
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptVerify(
			[]string{"receipt.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
				if path != "receipt.json" {
					t.Fatalf("VerifyArchiveQuestionTransitionHistoryAnswerReceipt() path = %q", path)
				}
				return summary, nil
			},
		)
		want := "archive reflection answer receipt structurally verified\n" +
			"schema_version: 1\n" +
			"question_id: answer-state-summary-changes\n" +
			"question: Did the bounded answer-state summary change at any supplied boundary?\n" +
			"result: same\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"receipt_sha256: " + strings.Repeat("b", 64) + "\n" +
			"note: this verifies the raw-value-free receipt contract; it does not re-verify the transition history or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptVerify(
			[]string{"--json", "--expect-sha256", strings.Repeat("b", 64), "receipt.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
				return summary, nil
			},
		)
		want := `{"schema_version":1,"question_id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?","result":"same","transition_history_sha256":"` + strings.Repeat("a", 64) + `","receipt_sha256":"` + strings.Repeat("b", 64) + `"}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskReceiptVerifyFailures(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"receipt.json", "extra"},
		{"--json=invalid", "receipt.json"},
		{"--expect-sha256", "invalid", "receipt.json"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceiptVerify(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
					t.Fatal("VerifyArchiveQuestionTransitionHistoryAnswerReceipt called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || (strings.Contains(strings.Join(args, "_"), "expect-sha256") && stderr.String() == usage) {
				t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "expect-sha256") && stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() stderr = %q", stderr.String())
			}
		})
	}

	t.Run("verify error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptVerify(
			[]string{"receipt.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
				return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, errors.New("receipt is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "receipt is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	t.Run("digest mismatch", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskReceiptVerify(
			[]string{"--expect-sha256", strings.Repeat("a", 64), "receipt.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
				return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{ReceiptSHA256: strings.Repeat("b", 64)}, nil
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "SHA-256 mismatch") {
			t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"receipt.json"}, {"--json", "receipt.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskReceiptVerify(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error) {
					return bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskReceiptVerify() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskSummary(t *testing.T) {
	answer := bundle.ArchiveQuestionTransitionHistorySummaryAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-summary-changes",
		Question:                "Did the bounded answer-state summary change at any supplied boundary?",
		Result:                  "changed",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             2,
		ChangedTransitions:      []int{1},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSummary(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				if path != "history.json" {
					t.Fatalf("AskArchiveQuestionTransitionHistorySummary() path = %q", path)
				}
				return answer, nil
			},
		)
		want := "archive reflection summary-change question answered\n" +
			"question_id: answer-state-summary-changes\n" +
			"question: Did the bounded answer-state summary change at any supplied boundary?\n" +
			"result: changed\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"transitions: 2\n" +
			"changed_transitions:\n" +
			"- 1\n" +
			"note: this answers only safe snapshot-summary structure; it does not infer chronology or prove the underlying evidence\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSummary(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) { return answer, nil },
		)
		want := `{"schema_version":1,"question_id":"answer-state-summary-changes","question":"Did the bounded answer-state summary change at any supplied boundary?","result":"changed","transition_history_sha256":"` + strings.Repeat("a", 64) + `","transitions":2,"changed_transitions":[1]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskSummaryFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSummary(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
					t.Fatal("AskArchiveQuestionTransitionHistorySummary called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskSummary(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
				return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSummary(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	for _, failAt := range []int{2, 3} {
		t.Run(fmt.Sprintf("write detail output %d", failAt), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskSummary(
				[]string{"history.json"},
				&failAfterWriter{failAt: failAt},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistorySummaryAnswer{
						QuestionID:         "answer-state-summary-changes",
						Question:           "Did the bounded answer-state summary change at any supplied boundary?",
						Result:             "changed",
						ChangedTransitions: []int{1},
					}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskSummary() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskRepeated(t *testing.T) {
	answer := bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{
		SchemaVersion:           1,
		QuestionID:              "answer-state-repeated-changes",
		Question:                "Did any safe archive entry change at more than one supplied boundary?",
		Result:                  "repeated",
		TransitionHistorySHA256: strings.Repeat("a", 64),
		Transitions:             3,
		RepeatedEntries: []bundle.ArchiveQuestionTransitionHistoryRepeatedChange{{
			Directory: "run-001",
			Changes: []bundle.ArchiveQuestionTransitionHistoryChange{
				{Transition: 1, FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64), Directory: "run-001", OlderState: "observed", NewerState: "unknown"},
				{Transition: 3, FromReflectionSHA256: strings.Repeat("d", 64), ToReflectionSHA256: strings.Repeat("e", 64), Directory: "run-001", OlderState: "unknown", NewerState: "unavailable"},
			},
		}},
	}

	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskRepeated(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(path string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				if path != "history.json" {
					t.Fatalf("AskArchiveQuestionTransitionHistoryRepeated() path = %q", path)
				}
				return answer, nil
			},
		)
		want := "archive reflection repeated-change question answered\n" +
			"question_id: answer-state-repeated-changes\n" +
			"question: Did any safe archive entry change at more than one supplied boundary?\n" +
			"result: repeated\n" +
			"transition_history_sha256: " + strings.Repeat("a", 64) + "\n" +
			"transitions: 3\n" +
			"repeated_entries:\n" +
			"- directory: run-001\n" +
			"  changes:\n" +
			"  - transition: 1\n" +
			"    from_reflection_sha256: " + strings.Repeat("b", 64) + "\n" +
			"    to_reflection_sha256: " + strings.Repeat("c", 64) + "\n" +
			"    older_state: observed\n" +
			"    newer_state: unknown\n" +
			"  - transition: 3\n" +
			"    from_reflection_sha256: " + strings.Repeat("d", 64) + "\n" +
			"    to_reflection_sha256: " + strings.Repeat("e", 64) + "\n" +
			"    older_state: unknown\n" +
			"    newer_state: unavailable\n" +
			"note: this reports repeated verified state-change records only; it does not infer chronology or a trend\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskRepeated(
			[]string{"--json", "history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) { return answer, nil },
		)
		want := `{"schema_version":1,"question_id":"answer-state-repeated-changes","question":"Did any safe archive entry change at more than one supplied boundary?","result":"repeated","transition_history_sha256":"` + strings.Repeat("a", 64) + `","transitions":3,"repeated_entries":[{"directory":"run-001","changes":[{"transition":1,"from_reflection_sha256":"` + strings.Repeat("b", 64) + `","to_reflection_sha256":"` + strings.Repeat("c", 64) + `","directory":"run-001","older_state":"observed","newer_state":"unknown"},{"transition":3,"from_reflection_sha256":"` + strings.Repeat("d", 64) + `","to_reflection_sha256":"` + strings.Repeat("e", 64) + `","directory":"run-001","older_state":"unknown","newer_state":"unavailable"}]}]}` + "\n"
		if exitCode != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
}

func TestRunAskArchiveTransitionsAskRepeatedFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskRepeated(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
					t.Fatal("AskArchiveQuestionTransitionHistoryRepeated called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAskRepeated(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
				return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskRepeated(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	for _, failAt := range []int{2, 3, 4, 5} {
		t.Run(fmt.Sprintf("write detail output %d", failAt), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAskRepeated(
				[]string{"history.json"},
				&failAfterWriter{failAt: failAt},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer{
						QuestionID: "answer-state-repeated-changes",
						Question:   "Did any safe archive entry change at more than one supplied boundary?",
						Result:     "repeated",
						RepeatedEntries: []bundle.ArchiveQuestionTransitionHistoryRepeatedChange{{
							Directory: "run-001",
							Changes: []bundle.ArchiveQuestionTransitionHistoryChange{
								{Transition: 1},
								{Transition: 2},
							},
						}},
					}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAskRepeated() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsAskFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"history.json", "extra"}, {"--json=invalid", "history.json"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAsk(
				args,
				&stdout,
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
					t.Fatal("AskArchiveQuestionTransitionHistory called for invalid usage")
					return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
				},
			)
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
				t.Fatalf("runAskArchiveTransitionsAsk() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}

	t.Run("answer error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAskArchiveTransitionsAsk(
			[]string{"history.json"},
			&stdout,
			&stderr,
			func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
				return bundle.ArchiveQuestionTransitionHistoryAnswer{}, errors.New("history is invalid")
			},
		)
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "history is invalid") {
			t.Fatalf("runAskArchiveTransitionsAsk() = %d, stderr=%q", exitCode, stderr.String())
		}
	})

	for _, args := range [][]string{{"history.json"}, {"--json", "history.json"}} {
		t.Run("write output "+strings.Join(args, "_"), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAsk(
				args,
				failingWriter{},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryAnswer{}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAsk() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

	for _, failAt := range []int{2, 3, 4, 5, 6} {
		t.Run(fmt.Sprintf("write detail output %d", failAt), func(t *testing.T) {
			var stderr bytes.Buffer
			exitCode := runAskArchiveTransitionsAsk(
				[]string{"history.json"},
				&failAfterWriter{failAt: failAt},
				&stderr,
				func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error) {
					return bundle.ArchiveQuestionTransitionHistoryAnswer{
						QuestionID:              "answer-state-transitions",
						Question:                "At which supplied boundaries did the bounded answer state change?",
						Result:                  "changed",
						ChangedTransitions:      []int{1},
						IncomparableTransitions: []int{2},
					}, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitionsAsk() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}
}

func TestRunAskArchiveTransitionsSave(t *testing.T) {
	summary := bundle.ArchiveQuestionTransitionVerificationSummary{
		SchemaVersion:           2,
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
			"schema_version: 2\n" +
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
		want := `{"schema_version":2,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","order_basis":"caller","snapshots":2,"transitions":1,"transition_history_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
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

	for _, failAt := range []int{3, 4} {
		t.Run(fmt.Sprintf("write state change output %d", failAt), func(t *testing.T) {
			history := bundle.ArchiveQuestionTransitionHistory{
				HistoryID:       "answer-state-transitions",
				HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
				QuestionID:      "counterfactual-change",
				Question:        "Did it change?",
				OrderBasis:      "caller",
				Snapshots:       2,
				Transitions: []bundle.ArchiveQuestionTransition{{
					FromReflectionSHA256: strings.Repeat("a", 64),
					ToReflectionSHA256:   strings.Repeat("b", 64),
					Result:               "changed",
					Compared:             1,
					Changed:              1,
					StateChanges: []bundle.ArchiveQuestionStateChange{{
						Directory:  "run-001",
						OlderState: "observed",
						NewerState: "unknown",
					}},
				}},
			}
			var stderr bytes.Buffer
			stdout := &failAfterWriter{failAt: failAt}
			exitCode := runAskArchiveTransitions(
				[]string{"one.json", "two.json"},
				stdout,
				&stderr,
				func([]string) (bundle.ArchiveQuestionTransitionHistory, error) {
					return history, nil
				},
			)
			if exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
				t.Fatalf("runAskArchiveTransitions() = %d, stderr=%q", exitCode, stderr.String())
			}
		})
	}

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
		SchemaVersion:           2,
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
			"schema_version: 2\n" +
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
		want := `{"schema_version":2,"history_id":"answer-state-transitions","history_question":"At which supplied boundaries did the bounded answer state change?","question_id":"counterfactual-change","order_basis":"caller","snapshots":3,"transitions":2,"transition_history_sha256":"` + strings.Repeat("a", 64) + `"}` + "\n"
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
		SchemaVersion:           2,
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

	t.Run("acceptance flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--history", "history.json", "--acceptance", "acceptance.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with acceptance = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("question round comparison flags", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--round-first", "first-round.json", "--round-second", "second-round.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with question round comparison = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("trace archive flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--trace-archive", "trace-archive.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with trace archive = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("trace question round flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--trace-round", "trace-round.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with trace question round = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("trace replication flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--trace-replication", "trace-replication.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with trace replication = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
		}
	})

	t.Run("trace case flag", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var gotHandler http.Handler
		exitCode := runServe(
			[]string{"--trace-case", "trace-case.json", "archive-root"},
			&stdout,
			&stderr,
			func(_ string, handler http.Handler) error {
				gotHandler = handler
				return nil
			},
		)
		if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
			t.Fatalf("runServe() with trace case = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
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

	t.Run("acceptance requires history", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runServe([]string{"--acceptance", "acceptance.json", "archive-root"}, &stdout, &stderr, func(string, http.Handler) error {
			t.Fatal("server called without history")
			return nil
		})
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment serve: --acceptance requires --history\n" {
			t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("question round comparison requires both rounds", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runServe([]string{"--round-first", "first-round.json", "archive-root"}, &stdout, &stderr, func(string, http.Handler) error {
			t.Fatal("server called without both question rounds")
			return nil
		})
		if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment serve: --round-first and --round-second must be supplied together\n" {
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

type failAfterWriter struct {
	failAt int
	writes int
}

func (w *failAfterWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, errors.New("write failed")
	}
	return len(data), nil
}
