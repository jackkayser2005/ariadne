package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceReplicationCommands(t *testing.T) {
	summary := trace.ReplicationLedgerVerificationSummary{
		SchemaVersion:          1,
		ResetPolicy:            trace.ReplicationResetPolicy,
		Pairs:                  2,
		BaselineTreatmentPairs: 1,
		TreatmentBaselinePairs: 1,
		ResetConfirmedPairs:    2,
		CompletePairs:          2,
		ChangedPairs:           2,
		OrderBalanced:          true,
		Outcome:                trace.ReplicatedChange,
		EvidenceState:          "observed",
		Reason:                 "every retained pair contains a safe category difference",
		LedgerSHA256:           strings.Repeat("a", 64),
	}
	args := []string{"--reset-confirmed", "1", "--reset-confirmed", "2", "ledger.json", "b1.json", "t1.json", "bs1.json", "ts1.json", "b2.json", "t2.json", "bs2.json", "ts2.json"}
	save := func(inputs []trace.ReplicationPairInput, output string) (trace.ReplicationLedgerVerificationSummary, error) {
		if output != "ledger.json" || len(inputs) != 2 || !inputs[0].ResetConfirmed || inputs[0].BaselineTracePath != "b1.json" || inputs[1].TreatmentSessionPath != "ts2.json" {
			t.Fatalf("save inputs = %#v, output = %q", inputs, output)
		}
		return summary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceReplicationSave(args, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication ledger saved") || !strings.Contains(stdout.String(), "outcome: replicated-change") || stderr.Len() != 0 {
		t.Fatalf("save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationSave(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var saved trace.ReplicationLedgerVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil || saved != summary {
		t.Fatalf("JSON save = %#v, err=%v", saved, err)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceReplicationSave([]string{"--reset-confirmed", "1", "ledger.json", "b1.json", "t1.json", "bs1.json", "ts1.json", "b2.json", "t2.json", "bs2.json", "ts2.json"}, &stdout, &stderr, func(inputs []trace.ReplicationPairInput, output string) (trace.ReplicationLedgerVerificationSummary, error) {
		if output != "ledger.json" || len(inputs) != 2 || !inputs[0].ResetConfirmed || inputs[1].ResetConfirmed {
			t.Fatalf("mixed reset inputs = %#v, output = %q", inputs, output)
		}
		return summary, nil
	}); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("mixed reset save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	verify := func(path string) (trace.ReplicationLedgerVerificationSummary, error) {
		if path != "ledger.json" {
			t.Fatalf("verify path = %q", path)
		}
		return summary, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceReplicationVerify([]string{"--expect-sha256", summary.LedgerSHA256, "ledger.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication ledger verified") || stderr.Len() != 0 {
		t.Fatalf("verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationVerify([]string{"--json", "ledger.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var verified trace.ReplicationLedgerVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verified); err != nil || verified != summary {
		t.Fatalf("JSON verify = %#v, err=%v", verified, err)
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "save usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationSave([]string{"ledger.json", "b.json", "t.json", "bs.json"}, out, errOut, save)
		}},
		{name: "verify usage", call: func(out, errOut *bytes.Buffer) int { return runTraceReplicationVerify(nil, out, errOut, verify) }},
		{name: "invalid expected ledger", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationVerify([]string{"--expect-sha256=bad", "ledger.json"}, out, errOut, verify)
		}},
		{name: "reset pair out of range", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationSave([]string{"--reset-confirmed", "3", "ledger.json", "b.json", "t.json", "bs.json", "ts.json", "b2.json", "t2.json", "bs2.json", "ts2.json"}, out, errOut, save)
		}},
		{name: "invalid reset pair", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationSave([]string{"--reset-confirmed", "zero", "ledger.json", "b.json", "t.json", "bs.json", "ts.json"}, out, errOut, save)
		}},
		{name: "save error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationSave(args, out, errOut, func([]trace.ReplicationPairInput, string) (trace.ReplicationLedgerVerificationSummary, error) {
				return trace.ReplicationLedgerVerificationSummary{}, errors.New("save failed safely")
			})
		}},
		{name: "verify error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationVerify([]string{"ledger.json"}, out, errOut, func(string) (trace.ReplicationLedgerVerificationSummary, error) {
				return trace.ReplicationLedgerVerificationSummary{}, errors.New("verify failed safely")
			})
		}},
		{name: "expected mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runTraceReplicationVerify([]string{"--expect-sha256=" + strings.Repeat("b", 64), "ledger.json"}, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}

	if exitCode := runTraceReplicationSave(args, failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceReplicationSave(append([]string{"--json"}, args...), failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceReplicationVerify([]string{"ledger.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceReplicationVerify([]string{"--json", "ledger.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestWriteTraceReplicationSummary(t *testing.T) {
	if err := writeTraceReplicationSummary(failingWriter{}, "ledger", trace.ReplicationLedgerVerificationSummary{}); err == nil {
		t.Fatal("writeTraceReplicationSummary() accepted a failing writer")
	}
}
