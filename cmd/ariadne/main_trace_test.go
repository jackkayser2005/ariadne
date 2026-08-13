package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceVerify(t *testing.T) {
	path := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	})
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"trace", "verify", path}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "trace verified\nscope: outbound\ncompleteness: complete\nevents: 1\ntrace_sha256: ") || stderr.Len() != 0 {
		t.Fatalf("trace verify output = %q, stderr = %q", stdout.String(), stderr.String())
	}

	summary, err := trace.Verify(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "verify", "--json", "--expect-sha256", summary.TraceSHA256, path}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("JSON run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var got trace.VerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != summary || stderr.Len() != 0 {
		t.Fatalf("JSON trace verify = %#v, want %#v; stderr=%q", got, summary, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "verify", "--expect-sha256", strings.Repeat("f", 64), path}, &stdout, &stderr); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "does not match expected identity") {
		t.Fatalf("mismatched identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "verify", "--expect-sha256=", path}, &stdout, &stderr); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "64 lowercase hexadecimal") {
		t.Fatalf("empty expected identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceCompare(t *testing.T) {
	baseline := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	})
	treatment := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region", "account-id"},
		}},
	})
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"trace", "compare", baseline, treatment}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	for _, want := range []string{"trace compared", "scope: outbound", "differences: 1", "source: browser", "destination: analytics", "change: changed", "state: observed"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("trace compare output missing %q: %s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("trace compare stderr = %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "compare", "--json", baseline, treatment}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("JSON run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	var comparison trace.Comparison
	if err := json.Unmarshal(stdout.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if len(comparison.Differences) != 1 || comparison.Differences[0].State != "observed" || stderr.Len() != 0 {
		t.Fatalf("JSON trace compare = %#v, stderr=%q", comparison, stderr.String())
	}
}

func TestRunExperimentTrace(t *testing.T) {
	summary := trace.VerificationSummary{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "all",
		Completeness:  trace.Complete,
		Events:        2,
		TraceSHA256:   strings.Repeat("a", 64),
	}
	save := func(runDir, session, tracePath string) (trace.VerificationSummary, error) {
		if runDir != "run" || session != "baseline" || tracePath != "trace.json" {
			t.Fatalf("save args = %q, %q, %q", runDir, session, tracePath)
		}
		return summary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runExperimentTrace([]string{"--session", "baseline", "run", "trace.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("runExperimentTrace() = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "experiment trace complete\nsession: baseline\nscope: all\ncompleteness: complete\nevents: 2\ntrace_sha256: "+strings.Repeat("a", 64)) || stderr.Len() != 0 {
		t.Fatalf("experiment trace output = %q, stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runExperimentTrace([]string{"--json", "--session", "baseline", "run", "trace.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON runExperimentTrace() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.VerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != summary || stderr.Len() != 0 {
		t.Fatalf("JSON experiment trace = %#v, want %#v; stderr=%q", got, summary, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runExperimentTrace([]string{"--session", "baseline", "run"}, &stdout, &stderr, save); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid experiment trace usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceReportsVerificationAndCoverageErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"trace", "verify", missing}, &stdout, &stderr); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "read trace") {
		t.Fatalf("trace verify error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	baseline := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	})
	treatment := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Partial,
		Events:        []trace.Event{},
	})
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "compare", baseline, treatment}, &stdout, &stderr); exitCode != 0 || !strings.Contains(stdout.String(), "unknowns: 1") || !strings.Contains(stdout.String(), "state: unknown") || stderr.Len() != 0 {
		t.Fatalf("partial trace compare = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func writeTraceFile(t *testing.T, document trace.Document) string {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "trace.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
