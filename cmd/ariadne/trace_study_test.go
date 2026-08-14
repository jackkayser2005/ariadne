package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceStudyCommands(t *testing.T) {
	summary := trace.StudyVerificationSummary{
		SchemaVersion:       1,
		ContrastSHA256:      strings.Repeat("a", 64),
		OrderBasis:          trace.ReplicationStudyOrderBasis,
		Runs:                2,
		Pairs:               4,
		SupportedRuns:       2,
		ResetConfirmedPairs: 4,
		BalancedRuns:        2,
		CompletePairs:       4,
		ChangedRuns:         2,
		MixedRuns:           0,
		Outcome:             trace.ReplicatedChange,
		EvidenceState:       "observed",
		Reason:              "every supported replicated run contains a safe category difference",
		StudySHA256:         strings.Repeat("b", 64),
	}
	args := []string{"--contrast-sha256", summary.ContrastSHA256, "study.json", "ledger-1.json", "round-1.json", "ledger-2.json", "round-2.json"}
	save := func(contrast string, inputs []trace.StudyInput, output string) (trace.StudyVerificationSummary, error) {
		if contrast != summary.ContrastSHA256 || output != "study.json" || len(inputs) != 2 || inputs[0].LedgerPath != "ledger-1.json" || inputs[1].RoundPath != "round-2.json" {
			t.Fatalf("save inputs = %#v, contrast = %q, output = %q", inputs, contrast, output)
		}
		return summary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceStudySave(args, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study saved") || !strings.Contains(stdout.String(), "outcome: replicated-change") || stderr.Len() != 0 {
		t.Fatalf("save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudySave(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var saved trace.StudyVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &saved); err != nil || saved != summary {
		t.Fatalf("JSON save = %#v, err=%v", saved, err)
	}

	verify := func(path string) (trace.StudyVerificationSummary, error) {
		if path != "study.json" {
			t.Fatalf("verify path = %q", path)
		}
		return summary, nil
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"--expect-sha256", summary.StudySHA256, "study.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication study verified") || stderr.Len() != 0 {
		t.Fatalf("verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceStudyVerify([]string{"--json", "study.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var verified trace.StudyVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &verified); err != nil || verified != summary {
		t.Fatalf("JSON verify = %#v, err=%v", verified, err)
	}

	for _, test := range []struct {
		name string
		call func(*bytes.Buffer, *bytes.Buffer) int
	}{
		{name: "save usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave([]string{"study.json", "ledger.json", "round.json"}, out, errOut, save)
		}},
		{name: "invalid contrast", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave([]string{"--contrast-sha256=bad", "study.json", "ledger.json", "round.json", "ledger-2.json", "round-2.json"}, out, errOut, save)
		}},
		{name: "verify usage", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify(nil, out, errOut, verify)
		}},
		{name: "invalid expected study", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"--expect-sha256=bad", "study.json"}, out, errOut, verify)
		}},
		{name: "save error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudySave(args, out, errOut, func(string, []trace.StudyInput, string) (trace.StudyVerificationSummary, error) {
				return trace.StudyVerificationSummary{}, errors.New("save failed safely")
			})
		}},
		{name: "verify error", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"study.json"}, out, errOut, func(string) (trace.StudyVerificationSummary, error) {
				return trace.StudyVerificationSummary{}, errors.New("verify failed safely")
			})
		}},
		{name: "expected mismatch", call: func(out, errOut *bytes.Buffer) int {
			return runTraceStudyVerify([]string{"--expect-sha256=" + strings.Repeat("c", 64), "study.json"}, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}

	stderr.Reset()
	if exitCode := runTraceStudySave(args, failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudySave(append([]string{"--json"}, args...), failingWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON save write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"study.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceStudyVerify([]string{"--json", "study.json"}, failingWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON verify write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestWriteTraceStudySummary(t *testing.T) {
	if err := writeTraceStudySummary(failingWriter{}, "study", trace.StudyVerificationSummary{}); err == nil {
		t.Fatal("writeTraceStudySummary() accepted a failing writer")
	}
}
