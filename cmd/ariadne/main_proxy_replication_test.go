package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/proxy"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunProxyReplicate(t *testing.T) {
	want := proxy.ReplicationSummary{
		SchemaVersion:           1,
		Adapter:                 proxy.Adapter,
		AdapterVersion:          proxy.AdapterVersion,
		Scope:                   "outbound",
		ResetPolicy:             proxy.ProxyReplicationResetPolicy,
		ControlledArgumentCount: 1,
		ConditionValuesWithheld: true,
		ExecutionIdentitySHA256: strings.Repeat("b", 64),
		ReceiptSHA256:           strings.Repeat("c", 64),
		Pairs:                   2,
		PairsPerOrder:           1,
		Outcome:                 trace.ReplicatedChange,
		EvidenceState:           "observed",
	}
	run := func(ctx context.Context, input proxy.ReplicationInput) error {
		if ctx.Err() != nil || input.ProcedurePath != "procedure.json" || input.ProgramPath != "program.exe" || input.OutputDir != "out" || input.Pairs != 1 || input.BaselineArg != "baseline" || input.TreatmentArg != "treatment" || !reflect.DeepEqual(input.SharedArgs, []string{"--mode", "fixture"}) {
			t.Fatalf("proxy replication input = %#v", input)
		}
		return nil
	}
	verify := func(root string) (proxy.ReplicationSummary, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, nil
	}
	args := []string{"--procedure", "procedure.json", "--program", "program.exe", "--shared-arg", "--mode", "--shared-arg", "fixture", "--baseline-arg", "baseline", "--treatment-arg", "treatment", "--pairs", "1", "--output", "out"}
	var stdout, stderr bytes.Buffer
	if exitCode := runProxyReplicate(args, &stdout, &stderr, run, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("proxy replication = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, text := range []string{"proxy replication complete", "outcome: replicated-change", "evidence_state: observed", "reset_policy: " + proxy.ProxyReplicationResetPolicy, "controlled_argument_count: 1", "condition_values_withheld: true"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human output missing %q: %s", text, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyReplicate(append([]string{"--json"}, args...), &stdout, &stderr, run, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON proxy replication = %d, stderr=%q", exitCode, stderr.String())
	}
	var got proxy.ReplicationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON proxy replication = %#v, err=%v; want %#v", got, err, want)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyReplicate([]string{"--procedure", "procedure.json", "--program", "program.exe", "--pairs", "1", "--output", "out"}, &stdout, &stderr, run, verify); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("invalid proxy replication = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runProxyReplicate(args, &stdout, &stderr, func(context.Context, proxy.ReplicationInput) error { return errors.New("replication failed safely") }, verify); exitCode != 1 || !strings.Contains(stderr.String(), "replication failed safely") {
		t.Fatalf("failed proxy replication = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicate(args, &stdout, &stderr, run, func(string) (proxy.ReplicationSummary, error) {
		return proxy.ReplicationSummary{}, errors.New("verification failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verification failed safely") {
		t.Fatalf("verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicate(append([]string{"--json"}, args...), browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicate(args, browserErrorWriter{}, &stderr, run, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunProxyReplicateVerify(t *testing.T) {
	want := proxy.ReplicationSummary{
		SchemaVersion:           1,
		ExecutionIdentitySHA256: strings.Repeat("b", 64),
		ReceiptSHA256:           strings.Repeat("c", 64),
		Outcome:                 trace.NoChangeObserved,
		EvidenceState:           "unknown",
		Pairs:                   2,
		PairsPerOrder:           1,
	}
	verify := func(root string) (proxy.ReplicationSummary, error) {
		if root != "out" {
			t.Fatalf("verify root = %q", root)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runProxyReplicateVerify([]string{"--json", "out"}, &stdout, &stderr, verify); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON proxy verification = %d, stderr=%q", exitCode, stderr.String())
	}
	var got proxy.ReplicationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON proxy verification = %#v, err=%v; want %#v", got, err, want)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runProxyReplicateVerify([]string{"out"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "outcome: no-change-observed") || !strings.Contains(stdout.String(), "evidence_state: unknown") {
		t.Fatalf("human proxy verification = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runProxyReplicateVerify(nil, &stdout, &stderr, verify); exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("invalid proxy verification = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicateVerify([]string{"out"}, &stdout, &stderr, func(string) (proxy.ReplicationSummary, error) {
		return proxy.ReplicationSummary{}, errors.New("verify failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verify failed safely") {
		t.Fatalf("proxy verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicateVerify([]string{"--json", "out"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON proxy verification write failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runProxyReplicateVerify([]string{"out"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human proxy verification write failure = %d, stderr=%q", exitCode, stderr.String())
	}
}
