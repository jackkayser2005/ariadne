package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceAdapter(t *testing.T) {
	summary := sourceAdapterCLISummary()
	var stdout, stderr bytes.Buffer
	var gotProcedure, gotDriver, gotOutput string
	var gotArgs []string
	if exitCode := runTraceAdapter([]string{
		"--json", "--procedure", "procedure.json", "--driver", "driver.exe",
		"--driver-arg", "one", "--driver-arg=two", "--output", "run",
	}, &stdout, &stderr, func(procedure, driver string, args []string, output string) (trace.SourceAdapterRunSummary, error) {
		gotProcedure, gotDriver, gotOutput, gotArgs = procedure, driver, output, append([]string(nil), args...)
		return summary, nil
	}); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("runTraceAdapter() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decoded trace.SourceAdapterRunSummary
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != summary || gotProcedure != "procedure.json" || gotDriver != "driver.exe" || gotOutput != "run" || strings.Join(gotArgs, ",") != "one,two" {
		t.Fatalf("decoded = %#v, args = %q %q %q %#v", decoded, gotProcedure, gotDriver, gotOutput, gotArgs)
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapter([]string{"--procedure", "procedure.json", "--driver", "driver.exe", "--output", "run"}, &stdout, &stderr, func(string, string, []string, string) (trace.SourceAdapterRunSummary, error) {
		return summary, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "source adapter run complete") || !strings.Contains(stdout.String(), "receipt_sha256: "+summary.ReceiptSHA256) || stderr.Len() != 0 {
		t.Fatalf("human output = %d, %q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapter([]string{"--procedure", "procedure.json", "--driver", "driver.exe", "--output", "run"}, &stdout, &stderr, func(string, string, []string, string) (trace.SourceAdapterRunSummary, error) {
		return trace.SourceAdapterRunSummary{}, errors.New("driver failed")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "trace adapter run: driver failed") {
		t.Fatalf("runner error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, args := range [][]string{
		nil,
		{"--procedure", "procedure.json", "--driver", "driver.exe"},
		{"--procedure", "procedure.json", "--driver", "driver.exe", "--output", "run", "extra"},
		{"--json=invalid", "--procedure", "procedure.json", "--driver", "driver.exe", "--output", "run"},
	} {
		stdout.Reset()
		stderr.Reset()
		if exitCode := runTraceAdapter(args, &stdout, &stderr, func(string, string, []string, string) (trace.SourceAdapterRunSummary, error) {
			return summary, nil
		}); exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("invalid run args %#v = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
}

func TestRunTraceAdapterVerify(t *testing.T) {
	summary := sourceAdapterCLISummary()
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceAdapterVerify([]string{"--json", "--expect-sha256", summary.ReceiptSHA256, "run"}, &stdout, &stderr, func(path string) (trace.SourceAdapterRunSummary, error) {
		if path != "run" {
			t.Fatalf("verify path = %q", path)
		}
		return summary, nil
	}); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var decoded trace.SourceAdapterRunSummary
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != summary {
		t.Fatalf("decoded = %#v, want %#v", decoded, summary)
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapterVerify([]string{"run"}, &stdout, &stderr, func(string) (trace.SourceAdapterRunSummary, error) {
		return summary, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "source adapter run verified") || stderr.Len() != 0 {
		t.Fatalf("human verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapterVerify([]string{"--expect-sha256", strings.Repeat("f", 64), "run"}, &stdout, &stderr, func(string) (trace.SourceAdapterRunSummary, error) {
		return summary, nil
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "does not match expected identity") {
		t.Fatalf("mismatched verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapterVerify([]string{"--expect-sha256", "bad", "run"}, &stdout, &stderr, func(string) (trace.SourceAdapterRunSummary, error) {
		return summary, nil
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "64 lowercase hexadecimal") {
		t.Fatalf("invalid expected verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceAdapterVerify([]string{"run"}, &stdout, &stderr, func(string) (trace.SourceAdapterRunSummary, error) {
		return trace.SourceAdapterRunSummary{}, errors.New("run is invalid")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "trace adapter verify: run is invalid") {
		t.Fatalf("verifier error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, args := range [][]string{nil, {"run", "extra"}, {"--json=invalid", "run"}} {
		stdout.Reset()
		stderr.Reset()
		if exitCode := runTraceAdapterVerify(args, &stdout, &stderr, func(string) (trace.SourceAdapterRunSummary, error) {
			return summary, nil
		}); exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("invalid verify args %#v = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
}

func sourceAdapterCLISummary() trace.SourceAdapterRunSummary {
	return trace.SourceAdapterRunSummary{
		ReceiptSHA256: strings.Repeat("a", 64),
		Receipt: trace.SourceAdapterReceipt{
			Adapter: "external-desktop-v1", Source: "desktop", Scope: "outbound", Completeness: trace.Complete,
			ProcedureSHA256: strings.Repeat("b", 64), ExecutableSHA256: strings.Repeat("c", 64), ChallengeSHA256: strings.Repeat("d", 64),
			TraceSHA256: strings.Repeat("e", 64), SessionSHA256: strings.Repeat("f", 64),
		},
	}
}
