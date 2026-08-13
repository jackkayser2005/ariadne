package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
