package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type sourceAdapterRunner func(string, string, []string, string) (trace.SourceAdapterRunSummary, error)
type sourceAdapterVerifier func(string) (trace.SourceAdapterRunSummary, error)

func sourceAdapterMeaning(summary trace.SourceAdapterRunSummary) string {
	switch {
	case summary.Receipt.Completeness == trace.Partial:
		return "The source reported some reviewed labels, but incomplete coverage means missing observations are unknown."
	case summary.Receipt.Completeness == trace.Complete && summary.Receipt.Events == 0:
		return "No supported observations were reported; that is not proof that no information left the source."
	case summary.Receipt.Completeness == trace.Complete:
		return "The source reported only reviewed labels from supported channels; raw values are omitted."
	default:
		return "The report contains only supported labels; missing or unsupported activity remains unknown."
	}
}

func runTraceAdapter(args []string, stdout, stderr io.Writer, run sourceAdapterRunner) int {
	flags := flag.NewFlagSet("trace adapter run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	procedurePath := flags.String("procedure", "", "")
	driverPath := flags.String("driver", "", "")
	outputDir := flags.String("output", "", "")
	var driverArgs []string
	flags.Func("driver-arg", "", func(value string) error {
		driverArgs = append(driverArgs, value)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *procedurePath == "" || *driverPath == "" || *outputDir == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := run(*procedurePath, *driverPath, driverArgs, *outputDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter run: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter run: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"source adapter run complete\nmeaning: %s\nadapter: %s\nsource: %s\nscope: %s\ncompleteness: %s\nevents: %d\nprocedure_sha256: %s\nexecutable_sha256: %s\nchallenge_sha256: %s\ntrace_sha256: %s\nsession_sha256: %s\nreceipt_sha256: %s\n",
		sourceAdapterMeaning(summary),
		summary.Receipt.Adapter,
		summary.Receipt.Source,
		summary.Receipt.Scope,
		summary.Receipt.Completeness,
		summary.Receipt.Events,
		summary.Receipt.ProcedureSHA256,
		summary.Receipt.ExecutableSHA256,
		summary.Receipt.ChallengeSHA256,
		summary.Receipt.TraceSHA256,
		summary.Receipt.SessionSHA256,
		summary.ReceiptSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter run: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceAdapterVerify(args []string, stdout, stderr io.Writer, verify sourceAdapterVerifier) int {
	flags := flag.NewFlagSet("trace adapter verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectSHA256Set := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectSHA256Set = true
		}
	})
	if expectSHA256Set && !trace.ValidSHA256(*expectSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: trace adapter verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter verify: %v\n", err)
		return 1
	}
	if expectSHA256Set && *expectSHA256 != summary.ReceiptSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace adapter verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"source adapter run verified\nmeaning: %s\nadapter: %s\nsource: %s\nscope: %s\ncompleteness: %s\nevents: %d\nprocedure_sha256: %s\nexecutable_sha256: %s\nchallenge_sha256: %s\ntrace_sha256: %s\nsession_sha256: %s\nreceipt_sha256: %s\n",
		sourceAdapterMeaning(summary),
		summary.Receipt.Adapter,
		summary.Receipt.Source,
		summary.Receipt.Scope,
		summary.Receipt.Completeness,
		summary.Receipt.Events,
		summary.Receipt.ProcedureSHA256,
		summary.Receipt.ExecutableSHA256,
		summary.Receipt.ChallengeSHA256,
		summary.Receipt.TraceSHA256,
		summary.Receipt.SessionSHA256,
		summary.ReceiptSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace adapter verify: write output: %v\n", err)
		return 1
	}
	return 0
}
