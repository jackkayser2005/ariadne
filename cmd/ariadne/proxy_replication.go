package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackkayser2005/ariadne/internal/proxy"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

const proxyReplicationTimeout = 80 * time.Minute

type proxyReplicationRunner func(context.Context, proxy.ReplicationInput) error
type proxyReplicationVerifier func(string) (proxy.ReplicationSummary, error)

func runProxyReplicate(args []string, stdout, stderr io.Writer, run proxyReplicationRunner, verify proxyReplicationVerifier) int {
	flags := flag.NewFlagSet("proxy replicate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	procedurePath := flags.String("procedure", "", "")
	programPath := flags.String("program", "", "")
	baselineArg := flags.String("baseline-arg", "", "")
	treatmentArg := flags.String("treatment-arg", "", "")
	pairs := flags.Int("pairs", 0, "")
	outputDir := flags.String("output", "", "")
	var sharedArgs []string
	flags.Func("shared-arg", "", func(value string) error {
		sharedArgs = append(sharedArgs, value)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *procedurePath == "" || *programPath == "" || *pairs < 1 || *outputDir == "" || *baselineArg == "" || *treatmentArg == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), proxyReplicationTimeout)
	defer cancel()
	if err := run(ctx, proxy.ReplicationInput{
		ProcedurePath: *procedurePath,
		ProgramPath:   *programPath,
		SharedArgs:    sharedArgs,
		BaselineArg:   *baselineArg,
		TreatmentArg:  *treatmentArg,
		OutputDir:     *outputDir,
		Pairs:         *pairs,
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate: %v\n", err)
		return 1
	}
	summary, err := verify(*outputDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate: verify output: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"proxy replication complete\nexecution_identity_sha256: %s\npairs_per_order: %d\nruns: %d\norder: %s, %s\nreset_policy: %s\ncontrolled_argument_count: %d\ncondition_values_withheld: %t\noutcome: %s\nevidence_state: %s\n",
		summary.ExecutionIdentitySHA256,
		summary.PairsPerOrder,
		summary.Pairs,
		trace.OrderBaselineTreatment,
		trace.OrderTreatmentBaseline,
		summary.ResetPolicy,
		summary.ControlledArgumentCount,
		summary.ConditionValuesWithheld,
		summary.Outcome,
		summary.EvidenceState,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate: write output: %v\n", err)
		return 1
	}
	return 0
}

func runProxyReplicateVerify(args []string, stdout, stderr io.Writer, verify proxyReplicationVerifier) int {
	flags := flag.NewFlagSet("proxy replicate verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"proxy replication verified\nexecution_identity_sha256: %s\nreceipt_sha256: %s\noutcome: %s\nevidence_state: %s\npairs: %d\npairs_per_order: %d\ncompleted_pairs: %d\nchanged_pairs: %d\nno_change_pairs: %d\nunknown_pairs: %d\n",
		summary.ExecutionIdentitySHA256,
		summary.ReceiptSHA256,
		summary.Outcome,
		summary.EvidenceState,
		summary.Pairs,
		summary.PairsPerOrder,
		summary.CompletedPairs,
		summary.ChangedPairs,
		summary.NoChangePairs,
		summary.UnknownPairs,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: proxy replicate verify: write output: %v\n", err)
		return 1
	}
	return 0
}
