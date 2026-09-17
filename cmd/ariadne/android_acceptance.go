package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

type androidAcceptanceSaver func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error)
type androidAcceptanceVerifier func(string) (bundle.AndroidAcceptanceVerificationSummary, error)

func runAndroidAcceptanceSave(
	args []string,
	stdout, stderr io.Writer,
	save androidAcceptanceSaver,
) int {
	flags := flag.NewFlagSet("experiment acceptance save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	reviewSelfAttested := flags.Bool("review-self-attested", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 5 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	if !*reviewSelfAttested {
		_, _ = io.WriteString(stderr, "ariadne: experiment acceptance save: --review-self-attested is required\n")
		return 2
	}

	summary, err := save(
		flags.Arg(0),
		flags.Arg(1),
		flags.Arg(2),
		flags.Arg(3),
		flags.Arg(4),
		*reviewSelfAttested,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"android acceptance record saved\nworkflow: %s\nmanifest_name: %s\noutcome: %s\nevidence_state: %s\nquestion_id: %s\nquestion_state: %s\nreview: %s %s\nacceptance_sha256: %s\n",
		summary.Workflow,
		summary.ManifestName,
		summary.Outcome,
		summary.EvidenceState,
		summary.QuestionID,
		summary.QuestionState,
		summary.ReviewMethod,
		summary.ReviewStatus,
		summary.AcceptanceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAndroidAcceptanceVerify(
	args []string,
	stdout, stderr io.Writer,
	verify androidAcceptanceVerifier,
) int {
	flags := flag.NewFlagSet("experiment acceptance verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment acceptance verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.AcceptanceSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment acceptance verify: acceptance SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"android acceptance record structurally verified\nschema_version: %d\nworkflow: %s\nmanifest_name: %s\noutcome: %s\nevidence_state: %s\nquestion_id: %s\nquestion_state: %s\nreview: %s %s (%s)\nacceptance_sha256: %s\nnote: this verifies raw-value-free identities and the declared GET-only review contract; it does not prove target behavior beyond the checked artifacts\n",
		summary.SchemaVersion,
		summary.Workflow,
		summary.ManifestName,
		summary.Outcome,
		summary.EvidenceState,
		summary.QuestionID,
		summary.QuestionState,
		summary.ReviewMethod,
		summary.ReviewStatus,
		summary.ReviewPath,
		summary.AcceptanceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment acceptance verify: write output: %v\n", err)
		return 1
	}
	return 0
}
