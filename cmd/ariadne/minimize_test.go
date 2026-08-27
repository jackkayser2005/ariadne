package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

const validMinimizationPlan = `{
  "schema_version": 1,
  "name": "android-location-minimize",
  "variable": "location",
  "reference_candidate": "exact",
  "functionality_criterion": "all-non-disclosure-fields-equal-v1",
  "tap_resource_id": "dev.ariadne.fixture:id/observe_button",
  "base_persona": {"email":"baseline@example.invalid","region":"us-east"},
  "candidates": [
    {"id":"exact","value":"37.7749-122.4194"},
    {"id":"city","value":"san-francisco"},
    {"id":"omitted","omitted":true}
  ]
}`

func TestRunExperimentMinimize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, []byte(validMinimizationPlan), 0o600); err != nil {
		t.Fatal(err)
	}
	target := adb.Target{Device: "emulator-5554", Package: "dev.ariadne.fixture"}
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonOutput], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{
				"--adb", "custom-adb",
				"--device", "emulator-5554",
				"--package", "dev.ariadne.fixture",
				"--pairs", "2",
				"--output", filepath.Join(t.TempDir(), "run"),
				path,
			}
			if jsonOutput {
				args = append([]string{"--json"}, args...)
			}
			exitCode := runExperimentMinimize(
				args,
				&stdout,
				&stderr,
				func(ctx context.Context, binary, device, packageName string) (adb.Target, error) {
					if _, ok := ctx.Deadline(); !ok || binary != "custom-adb" || device != target.Device || packageName != target.Package {
						t.Fatalf("check arguments = %q, %q, %q", binary, device, packageName)
					}
					return target, nil
				},
				func(ctx context.Context, binary string, gotTarget adb.Target, plan minimize.MinimizationPlan, output string, pairs int) (minimize.MinimizationSummary, error) {
					if _, ok := ctx.Deadline(); !ok || binary != "custom-adb" || gotTarget.Device != target.Device || plan.Name != "android-location-minimize" || pairs != 2 || output == "" {
						t.Fatalf("execute arguments = %q, %#v, %#v, %q, %d", binary, gotTarget, plan, output, pairs)
					}
					return minimize.MinimizationSummary{
						PlanName:          plan.Name,
						Variable:          plan.Variable,
						SelectionState:    minimize.SelectionSelected,
						SelectedCandidate: "omitted",
						EvidenceState:     evidence.Observed,
					}, nil
				},
			)
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("runExperimentMinimize() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if jsonOutput {
				var summary minimize.MinimizationSummary
				if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
					t.Fatal(err)
				}
				if summary.SelectedCandidate != "omitted" || summary.EvidenceState != evidence.Observed {
					t.Fatalf("JSON summary = %#v", summary)
				}
			} else if !strings.Contains(stdout.String(), "selection_state: selected") ||
				!strings.Contains(stdout.String(), "selected_candidate: omitted") {
				t.Fatalf("human summary = %q", stdout.String())
			}
		})
	}
}

func TestRunMinimizationVerify(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonOutput], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"run"}
			if jsonOutput {
				args = []string{"--json", "run"}
			}
			exitCode := runMinimizationVerify(args, &stdout, &stderr, func(path string) (minimize.MinimizationSummary, error) {
				if path != "run" {
					t.Fatalf("verify path = %q", path)
				}
				return minimize.MinimizationSummary{
					PlanName:       "android-location-minimize",
					Variable:       "location",
					SelectionState: minimize.SelectionUnknown,
					EvidenceState:  evidence.Unknown,
				}, nil
			})
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("runMinimizationVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if jsonOutput {
				var summary minimize.MinimizationSummary
				if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
					t.Fatal(err)
				}
				if summary.SelectionState != minimize.SelectionUnknown {
					t.Fatalf("JSON summary = %#v", summary)
				}
			} else if !strings.Contains(stdout.String(), "minimization verified") ||
				!strings.Contains(stdout.String(), "selected_candidate: none") {
				t.Fatalf("human summary = %q", stdout.String())
			}
		})
	}
}

func TestRunMinimizationUsageAndFailures(t *testing.T) {
	for _, args := range [][]string{
		{"experiment", "minimize"},
		{"experiment", "minimize", "verify"},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode != 2 || stderr.String() != usage || stdout.Len() != 0 {
			t.Fatalf("run(%v) = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	exitCode := runExperimentMinimize(
		[]string{"--device", "device", "--package", "package", "--pairs", "1", "--output", "run", filepath.Join(t.TempDir(), "missing.json")},
		&stdout,
		&stderr,
		func(context.Context, string, string, string) (adb.Target, error) {
			return adb.Target{}, errors.New("check must not run")
		},
		nil,
	)
	if exitCode != 1 || !strings.Contains(stderr.String(), "open plan") {
		t.Fatalf("missing plan = %d, stderr=%q", exitCode, stderr.String())
	}
}
