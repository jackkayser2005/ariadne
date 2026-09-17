package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceArchiveRoundSaver func(string, string) (trace.ArchiveQuestionRoundVerificationSummary, error)
type traceArchiveRoundVerifier func(string) (trace.ArchiveQuestionRoundVerificationSummary, error)
type traceArchiveReceiptAsker func(string, string) (trace.ArchiveQuestionReceipt, error)
type traceArchiveReceiptSaver func(string, string, string) (trace.ArchiveQuestionReceiptVerificationSummary, error)
type traceArchiveReceiptVerifier func(string) (trace.ArchiveQuestionReceiptVerificationSummary, error)

func runTraceArchiveAskAllSave(args []string, stdout, stderr io.Writer, save traceArchiveRoundSaver) int {
	flags := flag.NewFlagSet("trace archive ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveRoundSummary(stdout, "trace archive question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveAskAllVerify(args []string, stdout, stderr io.Writer, verify traceArchiveRoundVerifier) int {
	flags := flag.NewFlagSet("trace archive ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace archive ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace archive ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveRoundSummary(stdout, "trace archive question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveAskReceipt(args []string, stdout, stderr io.Writer, ask traceArchiveReceiptAsker) int {
	flags := flag.NewFlagSet("trace archive ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveReceipt(stdout, "trace archive question receipt", receipt, ""); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveAskReceiptSave(args []string, stdout, stderr io.Writer, save traceArchiveReceiptSaver) int {
	flags := flag.NewFlagSet("trace archive ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveReceiptSummary(stdout, "trace archive question receipt saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveAskReceiptVerify(args []string, stdout, stderr io.Writer, verify traceArchiveReceiptVerifier) int {
	flags := flag.NewFlagSet("trace archive ask receipt verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace archive ask receipt verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ReceiptSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace archive ask receipt verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveReceiptSummary(stdout, "trace archive question receipt verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceArchiveRoundSummary(stdout io.Writer, heading string, summary trace.ArchiveQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\narchive_sha256: %s\nquestions: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.ArchiveSHA256, summary.Questions, summary.RoundSHA256)
	return err
}

func writeTraceArchiveReceiptSummary(stdout io.Writer, heading string, summary trace.ArchiveQuestionReceiptVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\narchive_sha256: %s\nround_sha256: %s\nreceipt_sha256: %s\n", heading, summary.SchemaVersion, summary.QuestionID, summary.Question, summary.Result, summary.EvidenceState, summary.ArchiveSHA256, summary.RoundSHA256, summary.ReceiptSHA256)
	return err
}

func writeTraceArchiveReceipt(stdout io.Writer, heading string, receipt trace.ArchiveQuestionReceipt, receiptSHA256 string) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\narchive_sha256: %s\nround_sha256: %s\nentries: %d\ncompared: %d\nchanged: %d\nsame: %d\nunknown: %d\n", heading, receipt.SchemaVersion, receipt.QuestionID, receipt.Question, receipt.Result, receipt.EvidenceState, receipt.ArchiveSHA256, receipt.RoundSHA256, receipt.Entries, receipt.Compared, receipt.Changed, receipt.Same, receipt.Unknown); err != nil {
		return err
	}
	if receiptSHA256 != "" {
		if _, err := fmt.Fprintf(stdout, "receipt_sha256: %s\n", receiptSHA256); err != nil {
			return err
		}
	}
	if receipt.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "reason: %s\n", receipt.Reason); err != nil {
			return err
		}
	}
	return nil
}
