package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceCaseRoundComparer func(string, string) (trace.CaseQuestionRoundComparison, error)

func runTraceCaseAskAllCompare(args []string, stdout, stderr io.Writer, compare traceCaseRoundComparer) int {
	flags := flag.NewFlagSet("trace case ask all compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	comparison, err := compare(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all compare: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(comparison); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all compare: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"trace case question round comparison complete\nschema_version: %d\ncomparison_id: %s\ncomparison_question: %s\norder_basis: %s\nresult: %s\nfirst_round_sha256: %s\nsecond_round_sha256: %s\nfirst_case_sha256: %s\nsecond_case_sha256: %s\ncompared: %d\nchanged: %d\nchanged_questions:\n",
		comparison.SchemaVersion,
		comparison.ComparisonID,
		comparison.ComparisonQuestion,
		comparison.OrderBasis,
		comparison.Result,
		comparison.FirstRoundSHA256,
		comparison.SecondRoundSHA256,
		comparison.FirstCaseSHA256,
		comparison.SecondCaseSHA256,
		comparison.Compared,
		comparison.Changed,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all compare: write output: %v\n", err)
		return 1
	}
	for _, change := range comparison.ChangedQuestions {
		if _, err := fmt.Fprintf(stdout, "- question_id: %s\n  first_result: %s\n  second_result: %s\n  first_evidence_state: %s\n  second_evidence_state: %s\n  change_kinds: %s\n", change.QuestionID, change.FirstResult, change.SecondResult, change.FirstEvidenceState, change.SecondEvidenceState, strings.Join(change.ChangeKinds, ",")); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all compare: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this compares fixed case question projections in caller order; it does not infer chronology, causality, or direction\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all compare: write output: %v\n", err)
		return 1
	}
	return 0
}
