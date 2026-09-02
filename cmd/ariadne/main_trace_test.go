package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestRunTraceSessionCreateAndVerify(t *testing.T) {
	summary := trace.SessionVerificationSummary{
		SchemaVersion:   1,
		TraceSHA256:     strings.Repeat("a", 64),
		Source:          "browser",
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("b", 64),
		Scope:           "outbound",
		Completeness:    trace.Complete,
		Role:            trace.RoleStandalone,
		Order:           trace.OrderStandalone,
		PairSHA256:      "",
		SessionSHA256:   strings.Repeat("d", 64),
	}
	save := func(tracePath, sessionPath string, input trace.SessionInput) (trace.SessionVerificationSummary, error) {
		if tracePath != "trace.json" || sessionPath != "session.json" || input.Adapter != summary.Adapter || input.AdapterVersion != summary.AdapterVersion || input.Source != summary.Source || input.ProcedureSHA256 != summary.ProcedureSHA256 || input.Role != trace.RoleStandalone || input.Order != trace.OrderStandalone || input.PairSHA256 != "" {
			t.Fatalf("save args = %q, %q, %#v", tracePath, sessionPath, input)
		}
		return summary, nil
	}
	verify := func(sessionPath, tracePath string) (trace.SessionVerificationSummary, error) {
		if sessionPath != "session.json" || tracePath != "trace.json" {
			t.Fatalf("verify args = %q, %q", sessionPath, tracePath)
		}
		return summary, nil
	}
	var stdout, stderr bytes.Buffer
	args := []string{
		"--adapter", summary.Adapter,
		"--adapter-version", "1",
		"--source", summary.Source,
		"--procedure-sha256", summary.ProcedureSHA256,
		"trace.json", "session.json",
	}
	if exitCode := runTraceSessionCreate(args, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("runTraceSessionCreate() = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{
		"trace session complete", "source: browser", "adapter: browser-redacted-audit", "role: standalone", "order: standalone", "session_sha256: " + summary.SessionSHA256,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("session create output missing %q: %q", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("session create output = %q, stderr=%q", stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionCreate(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON runTraceSessionCreate() = %d, stderr=%q", exitCode, stderr.String())
	}
	var created trace.SessionVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created != summary || stderr.Len() != 0 {
		t.Fatalf("JSON session create = %#v, want %#v; stderr=%q", created, summary, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionCreate([]string{"--adapter", summary.Adapter, "trace.json", "session.json"}, &stdout, &stderr, save); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid session create usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"session.json", "trace.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("runTraceSessionVerify() = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "trace session verified") || !strings.Contains(stdout.String(), "source: browser") || !strings.Contains(stdout.String(), "trace_sha256: "+summary.TraceSHA256) || stderr.Len() != 0 {
		t.Fatalf("session verify output = %q, stderr=%q", stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"--json", "--expect-sha256", summary.SessionSHA256, "session.json", "trace.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON runTraceSessionVerify() = %d, stderr=%q", exitCode, stderr.String())
	}
	var verified trace.SessionVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verified); err != nil {
		t.Fatal(err)
	}
	if verified != summary || stderr.Len() != 0 {
		t.Fatalf("JSON session verify = %#v, want %#v; stderr=%q", verified, summary, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"--expect-sha256", strings.Repeat("e", 64), "session.json", "trace.json"}, &stdout, &stderr, verify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "does not match expected identity") {
		t.Fatalf("mismatched session identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"--expect-sha256=", "session.json", "trace.json"}, &stdout, &stderr, verify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "64 lowercase hexadecimal") {
		t.Fatalf("empty expected session identity = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"session.json"}, &stdout, &stderr, verify); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid session verify usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	badSave := func(string, string, trace.SessionInput) (trace.SessionVerificationSummary, error) {
		return trace.SessionVerificationSummary{}, errors.New("save failed")
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionCreate(args, &stdout, &stderr, badSave); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "save failed") {
		t.Fatalf("session create error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	badVerify := func(string, string) (trace.SessionVerificationSummary, error) {
		return trace.SessionVerificationSummary{}, errors.New("verify failed")
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"session.json", "trace.json"}, &stdout, &stderr, badVerify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "verify failed") {
		t.Fatalf("session verify error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionCreate(append([]string{"--json"}, args...), failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("session create write error = %d, stderr=%q", exitCode, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionVerify([]string{"--json", "session.json", "trace.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("session verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceSessionDispatch(t *testing.T) {
	tracePath := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	})
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	procedureSHA256 := strings.Repeat("a", 64)
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{
		"trace", "session", "create", "--json", "--adapter", "browser-redacted-audit",
		"--procedure-sha256", procedureSHA256, tracePath, sessionPath,
	}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run session create = %d, stderr=%q", exitCode, stderr.String())
	}
	var created trace.SessionVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Source != "browser" || created.Adapter != "browser-redacted-audit" || !trace.ValidSHA256(created.SessionSHA256) || stderr.Len() != 0 {
		t.Fatalf("created session = %#v, stderr=%q", created, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{
		"trace", "session", "verify", "--json", "--expect-sha256", created.SessionSHA256, sessionPath, tracePath,
	}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run session verify = %d, stderr=%q", exitCode, stderr.String())
	}
	var verified trace.SessionVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verified); err != nil {
		t.Fatal(err)
	}
	if verified != created || stderr.Len() != 0 {
		t.Fatalf("verified session = %#v, want %#v; stderr=%q", verified, created, stderr.String())
	}
}

func TestRunTraceSessionPairVerify(t *testing.T) {
	summary := trace.SessionPairVerificationSummary{
		SchemaVersion:          1,
		PairSHA256:             strings.Repeat("a", 64),
		Source:                 "browser",
		Adapter:                "browser-redacted-audit",
		AdapterVersion:         1,
		ProcedureSHA256:        strings.Repeat("b", 64),
		Scope:                  "outbound",
		Order:                  trace.OrderTreatmentBaseline,
		BaselineTraceSHA256:    strings.Repeat("c", 64),
		TreatmentTraceSHA256:   strings.Repeat("d", 64),
		BaselineCompleteness:   trace.Complete,
		TreatmentCompleteness:  trace.Partial,
		BaselineSessionSHA256:  strings.Repeat("e", 64),
		TreatmentSessionSHA256: strings.Repeat("f", 64),
	}
	verify := func(baselineSession, baselineTrace, treatmentSession, treatmentTrace string) (trace.SessionPairVerificationSummary, error) {
		if baselineSession != "baseline-session.json" || baselineTrace != "baseline-trace.json" || treatmentSession != "treatment-session.json" || treatmentTrace != "treatment-trace.json" {
			t.Fatalf("pair verify args = %q, %q, %q, %q", baselineSession, baselineTrace, treatmentSession, treatmentTrace)
		}
		return summary, nil
	}
	args := []string{"baseline-session.json", "baseline-trace.json", "treatment-session.json", "treatment-trace.json"}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceSessionPairVerify(args, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("runTraceSessionPairVerify() = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{"trace session pair verified", "source: browser", "order: treatment-baseline", "baseline_completeness: complete", "treatment_completeness: partial", "pair_sha256: " + summary.PairSHA256} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("pair output missing %q: %q", want, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairVerify(append([]string{"--json"}, args...), &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON runTraceSessionPairVerify() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.SessionPairVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != summary || stderr.Len() != 0 {
		t.Fatalf("JSON pair verify = %#v, want %#v; stderr=%q", got, summary, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairVerify([]string{"only-one"}, &stdout, &stderr, verify); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid pair verify usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	badVerify := func(string, string, string, string) (trace.SessionPairVerificationSummary, error) {
		return trace.SessionPairVerificationSummary{}, errors.New("pair failed")
	}
	if exitCode := runTraceSessionPairVerify(args, &stdout, &stderr, badVerify); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "pair failed") {
		t.Fatalf("pair verification error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairVerify(append([]string{"--json"}, args...), failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("pair write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceSessionPairCreate(t *testing.T) {
	summary := trace.SessionPairVerificationSummary{
		SchemaVersion:          1,
		PairSHA256:             strings.Repeat("a", 64),
		Source:                 "browser",
		Adapter:                "browser-redacted-audit",
		AdapterVersion:         1,
		ProcedureSHA256:        strings.Repeat("b", 64),
		Scope:                  "outbound",
		Order:                  trace.OrderBaselineTreatment,
		BaselineTraceSHA256:    strings.Repeat("c", 64),
		TreatmentTraceSHA256:   strings.Repeat("d", 64),
		BaselineCompleteness:   trace.Complete,
		TreatmentCompleteness:  trace.Complete,
		BaselineSessionSHA256:  strings.Repeat("e", 64),
		TreatmentSessionSHA256: strings.Repeat("f", 64),
	}
	save := func(baselineTrace, treatmentTrace, baselineSession, treatmentSession string, input trace.SessionPairInput) (trace.SessionPairVerificationSummary, error) {
		if baselineTrace != "baseline-trace.json" || treatmentTrace != "treatment-trace.json" || baselineSession != "baseline-session.json" || treatmentSession != "treatment-session.json" || input.Adapter != summary.Adapter || input.AdapterVersion != summary.AdapterVersion || input.Source != summary.Source || input.ProcedureSHA256 != summary.ProcedureSHA256 || input.Order != summary.Order {
			t.Fatalf("pair create args = %q, %q, %q, %q, %#v", baselineTrace, treatmentTrace, baselineSession, treatmentSession, input)
		}
		return summary, nil
	}
	args := []string{"--adapter", summary.Adapter, "--adapter-version", "1", "--source", summary.Source, "--procedure-sha256", summary.ProcedureSHA256, "--order", summary.Order, "baseline-trace.json", "treatment-trace.json", "baseline-session.json", "treatment-session.json"}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceSessionPairCreate(args, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("runTraceSessionPairCreate() = %d, stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "trace session pair complete") || !strings.Contains(stdout.String(), "pair_sha256: "+summary.PairSHA256) || stderr.Len() != 0 {
		t.Fatalf("pair create output = %q, stderr=%q", stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCreate(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON runTraceSessionPairCreate() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.SessionPairVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != summary || stderr.Len() != 0 {
		t.Fatalf("JSON pair create = %#v, want %#v; stderr=%q", got, summary, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCreate([]string{"--adapter", summary.Adapter, "baseline-trace.json"}, &stdout, &stderr, save); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid pair create usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	badSave := func(string, string, string, string, trace.SessionPairInput) (trace.SessionPairVerificationSummary, error) {
		return trace.SessionPairVerificationSummary{}, errors.New("pair create failed")
	}
	if exitCode := runTraceSessionPairCreate(args, &stdout, &stderr, badSave); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "pair create failed") {
		t.Fatalf("pair create error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCreate(append([]string{"--json"}, args...), failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("pair create write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceSessionPairCompare(t *testing.T) {
	result := trace.SessionPairComparison{
		SchemaVersion: 1,
		Pair: trace.SessionPairVerificationSummary{
			SchemaVersion: 1, PairSHA256: strings.Repeat("a", 64), Source: "browser", Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("b", 64), Scope: "outbound", Order: trace.OrderBaselineTreatment,
			BaselineTraceSHA256: strings.Repeat("c", 64), TreatmentTraceSHA256: strings.Repeat("d", 64), BaselineCompleteness: trace.Complete, TreatmentCompleteness: trace.Complete, BaselineSessionSHA256: strings.Repeat("e", 64), TreatmentSessionSHA256: strings.Repeat("f", 64),
		},
		Comparison: trace.Comparison{SchemaVersion: 1, Scope: "outbound", Unchanged: []trace.Event{}, Differences: []trace.EventChange{}, Unknowns: []trace.Unknown{}},
	}
	compare := func(baselineSession, baselineTrace, treatmentSession, treatmentTrace string) (trace.SessionPairComparison, error) {
		if baselineSession != "baseline-session.json" || baselineTrace != "baseline-trace.json" || treatmentSession != "treatment-session.json" || treatmentTrace != "treatment-trace.json" {
			t.Fatalf("pair compare args = %q, %q, %q, %q", baselineSession, baselineTrace, treatmentSession, treatmentTrace)
		}
		return result, nil
	}
	args := []string{"baseline-session.json", "baseline-trace.json", "treatment-session.json", "treatment-trace.json"}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceSessionPairCompare(args, &stdout, &stderr, compare); exitCode != 0 {
		t.Fatalf("runTraceSessionPairCompare() = %d, stderr=%q", exitCode, stderr.String())
	}
	for _, want := range []string{"trace session pair compared", "pair_sha256: " + result.Pair.PairSHA256, "source: browser", "scope: outbound", "differences: 0"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("pair compare output missing %q: %q", want, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCompare(append([]string{"--json"}, args...), &stdout, &stderr, compare); exitCode != 0 {
		t.Fatalf("JSON runTraceSessionPairCompare() = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.SessionPairComparison
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != result.SchemaVersion || got.Pair != result.Pair || stderr.Len() != 0 {
		t.Fatalf("JSON pair compare = %#v, want %#v; stderr=%q", got, result, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCompare([]string{"only-one"}, &stdout, &stderr, compare); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid pair compare usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	badCompare := func(string, string, string, string) (trace.SessionPairComparison, error) {
		return trace.SessionPairComparison{}, errors.New("comparison failed")
	}
	if exitCode := runTraceSessionPairCompare(args, &stdout, &stderr, badCompare); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "comparison failed") {
		t.Fatalf("pair compare error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceSessionPairCompare(append([]string{"--json"}, args...), failingWriter{}, &stderr, compare); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("pair compare write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceSessionPairDispatch(t *testing.T) {
	baselineTrace := writeTraceFile(t, trace.Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: trace.Complete,
		Events: []trace.Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
	})
	treatmentTrace := writeTraceFile(t, trace.Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: trace.Complete,
		Events: []trace.Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id", "region"}}},
	})
	root := t.TempDir()
	baselineSession := filepath.Join(root, "baseline-session.json")
	treatmentSession := filepath.Join(root, "treatment-session.json")
	procedureSHA256 := strings.Repeat("b", 64)
	var createStdout, createStderr bytes.Buffer
	if exitCode := run([]string{"trace", "session", "pair", "create", "--json", "--adapter", "browser-redacted-audit", "--procedure-sha256", procedureSHA256, "--order", trace.OrderBaselineTreatment, baselineTrace, treatmentTrace, baselineSession, treatmentSession}, &createStdout, &createStderr); exitCode != 0 {
		t.Fatalf("run pair session create = %d, stderr=%q", exitCode, createStderr.String())
	}
	var created trace.SessionPairVerificationSummary
	if err := json.Unmarshal(createStdout.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if exitCode := run([]string{"trace", "session", "pair", "verify", "--json", baselineSession, baselineTrace, treatmentSession, treatmentTrace}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run pair session verify = %d, stderr=%q", exitCode, stderr.String())
	}
	var summary trace.SessionPairVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary != created || summary.BaselineTraceSHA256 == summary.TreatmentTraceSHA256 || stderr.Len() != 0 {
		t.Fatalf("run pair summary = %#v, stderr=%q", summary, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "session", "pair", "compare", "--json", baselineSession, baselineTrace, treatmentSession, treatmentTrace}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("run pair session compare = %d, stderr=%q", exitCode, stderr.String())
	}
	var comparison trace.SessionPairComparison
	if err := json.Unmarshal(stdout.Bytes(), &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.Pair != created || len(comparison.Comparison.Differences) != 1 || stderr.Len() != 0 {
		t.Fatalf("run pair comparison = %#v, stderr=%q", comparison, stderr.String())
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
