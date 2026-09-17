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
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunBrowserCapture(t *testing.T) {
	want := browser.CaptureSummary{
		ProcedureSHA256: strings.Repeat("a", 64),
		Trace: trace.VerificationSummary{
			SchemaVersion: 1,
			Redacted:      true,
			Scope:         "outbound",
			Completeness:  trace.Complete,
			Events:        1,
			TraceSHA256:   strings.Repeat("b", 64),
		},
	}
	capture := func(procedure, driver string, args []string, output string) (browser.CaptureSummary, error) {
		if procedure != "procedure.json" || driver != "driver.exe" || output != "trace.json" || len(args) != 2 || args[0] != "driver-script.mjs" || args[1] != "--fixture" {
			t.Fatalf("capture args = %q, %q, %#v, %q", procedure, driver, args, output)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runBrowserCapture([]string{"--procedure", "procedure.json", "--driver", "driver.exe", "--driver-arg", "driver-script.mjs", "--driver-arg", "--fixture", "trace.json"}, &stdout, &stderr, capture); exitCode != 0 {
		t.Fatalf("runBrowserCapture() = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, text := range []string{"browser capture complete", "procedure_sha256: " + want.ProcedureSHA256, "scope: outbound", "completeness: complete", "events: 1", "trace_sha256: " + want.Trace.TraceSHA256} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human output missing %q: %s", text, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("human stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserCapture([]string{"--json", "--procedure", "procedure.json", "--driver", "driver.exe", "--driver-arg", "driver-script.mjs", "--driver-arg", "--fixture", "trace.json"}, &stdout, &stderr, capture); exitCode != 0 {
		t.Fatalf("JSON runBrowserCapture() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got browser.CaptureSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProcedureSHA256 != want.ProcedureSHA256 || got.Trace != want.Trace || stderr.Len() != 0 {
		t.Fatalf("JSON capture = %#v, want %#v; stderr=%q", got, want, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserCapture([]string{"--procedure", "procedure.json", "--driver", "driver.exe"}, &stdout, &stderr, capture); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid capture usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runBrowserCapture([]string{"--procedure", "", "--driver", "driver.exe", "trace.json"}, &stdout, &stderr, capture); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("missing procedure = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	failing := func(string, string, []string, string) (browser.CaptureSummary, error) {
		return browser.CaptureSummary{}, errors.New("driver failed safely")
	}
	if exitCode := runBrowserCapture([]string{"--procedure", "procedure.json", "--driver", "driver.exe", "trace.json"}, &stdout, &stderr, failing); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "driver failed safely") {
		t.Fatalf("capture failure = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunBrowserFixtureReplicate(t *testing.T) {
	want := browser.BrowserReplicationSummary{
		SchemaVersion:   1,
		Adapter:         browser.BrowserReplicationAdapter,
		ProcedureSHA256: strings.Repeat("a", 64),
		Scope:           "outbound",
		ResetPolicy:     browser.BrowserReplicationResetPolicy,
		ReceiptSHA256:   strings.Repeat("b", 64),
		Pairs:           2,
		PairsPerOrder:   1,
		Outcome:         trace.ReplicatedChange,
		EvidenceState:   "observed",
	}
	run := func(ctx context.Context, input browser.FixtureReplicationInput) error {
		if ctx.Err() != nil || input.ProcedurePath != "procedure.json" || input.DriverPath != "driver.exe" || input.OutputDir != "out" || input.Pairs != 1 || len(input.DriverArgs) != 2 || input.DriverArgs[0] != "driver-script.mjs" || input.DriverArgs[1] != "--fixture" {
			t.Fatalf("fixture replication input = %#v", input)
		}
		return nil
	}
	verify := func(root string) (browser.BrowserReplicationSummary, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--procedure", "procedure.json", "--driver", "driver.exe", "--driver-arg", "driver-script.mjs", "--driver-arg", "--fixture", "--pairs", "1", "--output", "out"}
	if exitCode := runBrowserFixtureReplicate(args, &stdout, &stderr, run, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("fixture replication = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, text := range []string{"browser fixture replication complete", "outcome: replicated-change", "evidence_state: observed", "reset_policy: " + browser.BrowserReplicationResetPolicy} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human output missing %q: %s", text, stdout.String())
		}
	}
	stdout.Reset()
	if exitCode := runBrowserFixtureReplicate(append([]string{"--json"}, args...), &stdout, &stderr, run, verify); exitCode != 0 {
		t.Fatalf("JSON fixture replication = %d, stderr=%q", exitCode, stderr.String())
	}
	var got browser.BrowserReplicationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON fixture replication = %#v, err=%v", got, err)
	}
	stdout.Reset()
	if exitCode := runBrowserFixtureReplicate([]string{"--procedure", "procedure.json", "--driver", "driver.exe", "--pairs", "1"}, &stdout, &stderr, run, verify); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid fixture replication = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runBrowserFixtureReplicate(args, &stdout, &stderr, func(context.Context, browser.FixtureReplicationInput) error {
		return errors.New("replication failed safely")
	}, verify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "replication failed safely") {
		t.Fatalf("failed fixture replication = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runBrowserFixtureReplicate(args, &stdout, &stderr, run, func(string) (browser.BrowserReplicationSummary, error) {
		return browser.BrowserReplicationSummary{}, errors.New("verification failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verification failed safely") {
		t.Fatalf("verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureReplicate(append([]string{"--json"}, args...), browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureReplicate(args, browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunBrowserFixtureReplicateVerify(t *testing.T) {
	want := browser.BrowserReplicationSummary{SchemaVersion: 1, Outcome: trace.NoChangeObserved, EvidenceState: "observed"}
	verify := func(root string) (browser.BrowserReplicationSummary, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runBrowserFixtureReplicateVerify([]string{"--json", "out"}, &stdout, &stderr, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON fixture verification = %d, stderr=%q", exitCode, stderr.String())
	}
	var got browser.BrowserReplicationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON fixture verification = %#v, err=%v", got, err)
	}
	stdout.Reset()
	if exitCode := runBrowserFixtureReplicateVerify([]string{"out"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "outcome: no-change-observed") {
		t.Fatalf("human fixture verification = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runBrowserFixtureReplicateVerify(nil, &stdout, &stderr, verify); exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("invalid fixture verification = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureReplicateVerify([]string{"out"}, &stdout, &stderr, func(string) (browser.BrowserReplicationSummary, error) {
		return browser.BrowserReplicationSummary{}, errors.New("verify failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verify failed safely") {
		t.Fatalf("fixture verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureReplicateVerify([]string{"--json", "out"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON verification write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runBrowserFixtureReplicateVerify([]string{"out"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human verification write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

type browserErrorWriter struct{}

func (browserErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
