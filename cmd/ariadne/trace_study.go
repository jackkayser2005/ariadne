package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceStudySaver func(string, []trace.StudyInput, string) (trace.StudyVerificationSummary, error)
type traceStudyVerifier func(string) (trace.StudyVerificationSummary, error)

func runTraceStudySave(args []string, stdout, stderr io.Writer, save traceStudySaver) int {
	flags := flag.NewFlagSet("trace study save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	contrastSHA256 := flags.String("contrast-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() < 5 || (flags.NArg()-1)%2 != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	if !trace.ValidSHA256(*contrastSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: trace study save: contrast SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	inputs := make([]trace.StudyInput, 0, (flags.NArg()-1)/2)
	for index := 1; index < flags.NArg(); index += 2 {
		inputs = append(inputs, trace.StudyInput{LedgerPath: flags.Arg(index), RoundPath: flags.Arg(index + 1)})
	}
	summary, err := save(*contrastSHA256, inputs, flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudySummary(stdout, "trace replication study saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyVerify(args []string, stdout, stderr io.Writer, verify traceStudyVerifier) int {
	flags := flag.NewFlagSet("trace study verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace study verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study verify: %v\n", err)
		return 1
	}
	if expectSet && summary.StudySHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace study verify: study SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudySummary(stdout, "trace replication study verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceStudySummary(stdout io.Writer, heading string, summary trace.StudyVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\ncontrast_sha256: %s\norder_basis: %s\nruns: %d\npairs: %d\nsupported_runs: %d\nunknown_runs: %d\nreset_confirmed_pairs: %d\nbalanced_runs: %d\ncomplete_pairs: %d\nchanged_runs: %d\nno_change_runs: %d\nmixed_runs: %d\nunknown_pairs: %d\noutcome: %s\nevidence_state: %s\nreason: %s\nstudy_sha256: %s\n", heading, summary.SchemaVersion, summary.ContrastSHA256, summary.OrderBasis, summary.Runs, summary.Pairs, summary.SupportedRuns, summary.UnknownRuns, summary.ResetConfirmedPairs, summary.BalancedRuns, summary.CompletePairs, summary.ChangedRuns, summary.NoChangeRuns, summary.MixedRuns, summary.UnknownPairs, summary.Outcome, summary.EvidenceState, summary.Reason, summary.StudySHA256)
	return err
}
