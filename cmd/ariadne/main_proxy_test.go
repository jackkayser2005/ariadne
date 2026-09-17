package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/proxy"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunProxyCapture(t *testing.T) {
	want := proxy.CaptureSummary{
		ProcedureSHA256: strings.Repeat("a", 64),
		Trace: trace.VerificationSummary{
			SchemaVersion: 1,
			Redacted:      true,
			Scope:         "outbound",
			Completeness:  trace.Partial,
			Events:        1,
			TraceSHA256:   strings.Repeat("b", 64),
		},
	}
	capture := func(procedure, program string, args []string, output string) (proxy.CaptureSummary, error) {
		if procedure != "procedure.json" || program != "program.exe" || output != "trace.json" || len(args) != 2 || args[0] != "--fixture" || args[1] != "one" {
			t.Fatalf("capture args = %q, %q, %#v, %q", procedure, program, args, output)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	args := []string{"--procedure", "procedure.json", "--program", "program.exe", "--program-arg", "--fixture", "--program-arg", "one", "trace.json"}
	if exitCode := runProxyCapture(args, &stdout, &stderr, capture); exitCode != 0 {
		t.Fatalf("runProxyCapture() = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, text := range []string{"proxy capture complete", "procedure_sha256: " + want.ProcedureSHA256, "scope: outbound", "completeness: partial", "events: 1", "trace_sha256: " + want.Trace.TraceSHA256} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human output missing %q: %s", text, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("human stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyCapture(append([]string{"--json"}, args...), &stdout, &stderr, capture); exitCode != 0 {
		t.Fatalf("JSON runProxyCapture() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got proxy.CaptureSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != want || stderr.Len() != 0 {
		t.Fatalf("JSON capture = %#v, want %#v; stderr=%q", got, want, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyCapture([]string{"--procedure", "procedure.json", "--program", "program.exe"}, &stdout, &stderr, capture); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid capture usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyCapture([]string{"--procedure", "", "--program", "program.exe", "trace.json"}, &stdout, &stderr, capture); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("missing procedure = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	failing := func(string, string, []string, string) (proxy.CaptureSummary, error) {
		return proxy.CaptureSummary{}, errors.New("program failed safely")
	}
	if exitCode := runProxyCapture(args, &stdout, &stderr, failing); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "program failed safely") {
		t.Fatalf("capture failure = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
