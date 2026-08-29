package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceCaseAssembler func(string, string) (trace.CaseAssemblySummary, error)
type traceCaseAssemblyVerifier func(string) (trace.CaseAssemblySummary, error)

func runTraceCaseAssemble(args []string, stdout, stderr io.Writer, assemble traceCaseAssembler) int {
	flags := flag.NewFlagSet("trace case assemble", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	planPath := flags.String("plan", "", "")
	outputDir := flags.String("output", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *planPath == "" || *outputDir == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := assemble(*planPath, *outputDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseAssemblySummary(stdout, summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseAssembleVerify(args []string, stdout, stderr io.Writer, verify traceCaseAssemblyVerifier) int {
	flags := flag.NewFlagSet("trace case assemble verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseAssemblyVerificationSummary(stdout, summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case assemble verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceCaseAssemblySummary(stdout io.Writer, summary trace.CaseAssemblySummary) error {
	return writeTraceCaseAssemblySummaryWithHeading(stdout, "trace case assembly complete", summary)
}

func writeTraceCaseAssemblyVerificationSummary(stdout io.Writer, summary trace.CaseAssemblySummary) error {
	return writeTraceCaseAssemblySummaryWithHeading(stdout, "trace case assembly verified", summary)
}

func writeTraceCaseAssemblySummaryWithHeading(stdout io.Writer, heading string, summary trace.CaseAssemblySummary) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nentries: %d\ncase_sha256: %s\ndisclosure_round_sha256: %s\ncoverage_state: %s\n", heading, summary.SchemaVersion, summary.Entries, summary.CaseSHA256, summary.DisclosureRoundSHA256, summary.CoverageState); err != nil {
		return err
	}
	for _, question := range summary.Questions {
		if _, err := fmt.Fprintf(stdout, "- question_id: %s\n  result: %s\n  evidence_state: %s\n", question.QuestionID, question.Result, question.EvidenceState); err != nil {
			return err
		}
	}
	return nil
}
