package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceReplicationSaver func([]trace.ReplicationPairInput, string) (trace.ReplicationLedgerVerificationSummary, error)
type traceReplicationVerifier func(string) (trace.ReplicationLedgerVerificationSummary, error)

func runTraceReplicationSave(args []string, stdout, stderr io.Writer, save traceReplicationSaver) int {
	flags := flag.NewFlagSet("trace replication save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	resetConfirmedPairs := make(map[int]bool)
	flags.Func("reset-confirmed", "", func(value string) error {
		pair, err := strconv.Atoi(value)
		if err != nil || pair < 1 {
			return fmt.Errorf("reset-confirmed pair must be a positive integer")
		}
		resetConfirmedPairs[pair] = true
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() < 5 || (flags.NArg()-1)%4 != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	pairCount := (flags.NArg() - 1) / 4
	for pair := range resetConfirmedPairs {
		if pair > pairCount {
			_, _ = io.WriteString(stderr, "ariadne: trace replication save: reset-confirmed pair is out of range\n")
			return 2
		}
	}
	inputs := make([]trace.ReplicationPairInput, 0, (flags.NArg()-1)/4)
	for index := 1; index < flags.NArg(); index += 4 {
		pair := (index-1)/4 + 1
		inputs = append(inputs, trace.ReplicationPairInput{
			BaselineTracePath:    flags.Arg(index),
			TreatmentTracePath:   flags.Arg(index + 1),
			BaselineSessionPath:  flags.Arg(index + 2),
			TreatmentSessionPath: flags.Arg(index + 3),
			ResetConfirmed:       resetConfirmedPairs[pair],
		})
	}
	summary, err := save(inputs, flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationSummary(stdout, "trace replication ledger saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationVerify(args []string, stdout, stderr io.Writer, verify traceReplicationVerifier) int {
	flags := flag.NewFlagSet("trace replication verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectSet := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectSet = true
		}
	})
	if expectSet && !trace.ValidSHA256(*expectSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: trace replication verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication verify: %v\n", err)
		return 1
	}
	if expectSet && summary.LedgerSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace replication verify: ledger SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationSummary(stdout, "trace replication ledger verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceReplicationSummary(stdout io.Writer, heading string, summary trace.ReplicationLedgerVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nreset_policy: %s\npairs: %d\nbaseline_treatment_pairs: %d\ntreatment_baseline_pairs: %d\nreset_confirmed_pairs: %d\ncomplete_pairs: %d\nchanged_pairs: %d\nno_change_pairs: %d\nunknown_pairs: %d\norder_balanced: %t\noutcome: %s\nevidence_state: %s\nreason: %s\nledger_sha256: %s\n", heading, summary.SchemaVersion, summary.ResetPolicy, summary.Pairs, summary.BaselineTreatmentPairs, summary.TreatmentBaselinePairs, summary.ResetConfirmedPairs, summary.CompletePairs, summary.ChangedPairs, summary.NoChangePairs, summary.UnknownPairs, summary.OrderBalanced, summary.Outcome, summary.EvidenceState, summary.Reason, summary.LedgerSHA256)
	return err
}
