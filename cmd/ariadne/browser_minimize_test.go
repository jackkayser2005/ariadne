package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestRunBrowserFixtureMinimize(t *testing.T) {
	want := minimize.LadderSummary{
		PlanName:          "browser-account-minimize",
		Variable:          "account-id",
		SelectedCandidate: "omitted",
		SelectionState:    minimize.SelectionSelected,
		EvidenceState:     "observed",
	}
	run := func(ctx context.Context, input browser.FixtureMinimizationInput) error {
		if ctx.Err() != nil || input.PlanPath != "plan.json" || input.ProcedurePath != "procedure.json" || input.DriverPath != "driver.exe" || input.OutputDir != "out" || input.Pairs != 2 || len(input.DriverArgs) != 2 || input.DriverArgs[0] != "driver.mjs" || input.DriverArgs[1] != "--fixture" {
			t.Fatalf("browser minimization input = %#v", input)
		}
		return nil
	}
	verify := func(root string) (minimize.LadderSummary, string, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, strings.Repeat("a", 64), nil
	}
	expectedSHA256 := strings.Repeat("a", 64)
	args := []string{"--plan", "plan.json", "--procedure", "procedure.json", "--driver", "driver.exe", "--driver-arg", "driver.mjs", "--driver-arg", "--fixture", "--pairs", "2", "--output", "out"}
	var stdout, stderr bytes.Buffer
	if exitCode := runBrowserFixtureMinimize(args, &stdout, &stderr, run, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("human browser minimization = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, text := range []string{"browser fixture minimization complete", "selected_candidate: omitted", "selection_state: selected", "evidence_state: observed", "receipt_sha256: " + expectedSHA256} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human output missing %q: %s", text, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserFixtureMinimize(append([]string{"--json"}, args...), &stdout, &stderr, run, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON browser minimization = %d, stderr=%q", exitCode, stderr.String())
	}
	var got minimize.LadderSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) || !strings.Contains(stdout.String(), `"receipt_sha256":"`+expectedSHA256+`"`) {
		t.Fatalf("JSON browser minimization = %#v, err=%v", got, err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserFixtureMinimize([]string{"--plan", "plan.json", "--procedure", "procedure.json", "--driver", "driver.exe", "--pairs", "1"}, &stdout, &stderr, run, verify); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid browser minimization usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserFixtureMinimize(args, &stdout, &stderr, func(context.Context, browser.FixtureMinimizationInput) error { return errors.New("run failed safely") }, verify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "run failed safely") {
		t.Fatalf("browser minimization run failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserFixtureMinimize(args, &stdout, &stderr, run, func(string) (minimize.LadderSummary, string, error) {
		return minimize.LadderSummary{}, "", errors.New("verify failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verify failed safely") {
		t.Fatalf("browser minimization verify failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureMinimize(append([]string{"--json"}, args...), browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("browser minimization JSON write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureMinimize(args, browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("browser minimization human write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunBrowserFixtureMinimizeExpectedIdentityFailures(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := runBrowserFixtureMinimizeVerify([]string{"--expect-sha256", "bad", "out"}, &stdout, &stderr, func(string) (minimize.LadderSummary, string, error) {
		t.Fatal("verify callback ran for invalid expected digest")
		return minimize.LadderSummary{}, "", nil
	}); exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "64-character SHA-256") {
		t.Fatalf("invalid expected browser identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserFixtureMinimizeVerify([]string{"--expect-sha256", strings.Repeat("b", 64), "out"}, &stdout, &stderr, func(string) (minimize.LadderSummary, string, error) {
		return minimize.LadderSummary{}, strings.Repeat("a", 64), nil
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "does not match expected identity") {
		t.Fatalf("mismatched browser identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
func TestRunBrowserFixtureMinimizeVerify(t *testing.T) {
	want := minimize.LadderSummary{PlanName: "browser-account-minimize", SelectedCandidate: "omitted", SelectionState: minimize.SelectionSelected, EvidenceState: "observed"}
	expectedSHA256 := strings.Repeat("a", 64)
	verify := func(root string) (minimize.LadderSummary, string, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, strings.Repeat("a", 64), nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runBrowserFixtureMinimizeVerify([]string{"--json", "--expect-sha256", expectedSHA256, "out"}, &stdout, &stderr, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON browser minimization verification = %d, stderr=%q", exitCode, stderr.String())
	}
	var got minimize.LadderSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) || !strings.Contains(stdout.String(), `"receipt_sha256":"`+expectedSHA256+`"`) {
		t.Fatalf("JSON browser minimization verification = %#v, err=%v", got, err)
	}
	stdout.Reset()
	if exitCode := runBrowserFixtureMinimizeVerify([]string{"--expect-sha256", expectedSHA256, "out"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "browser fixture minimization verified") {
		t.Fatalf("human browser minimization verification = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runBrowserFixtureMinimizeVerify(nil, &stdout, &stderr, verify); exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("invalid browser minimization verification = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureMinimizeVerify([]string{"out"}, &stdout, &stderr, func(string) (minimize.LadderSummary, string, error) {
		return minimize.LadderSummary{}, "", errors.New("verify failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verify failed safely") {
		t.Fatalf("browser minimization verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
}
