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
type traceStudyQuestions func() []trace.StudyQuestion
type traceStudyAsker func(string, string) (trace.StudyQuestionAnswer, error)
type traceStudyAllAsker func(string) ([]trace.StudyQuestionAnswer, error)

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

func runTraceStudyQuestions(args []string, stdout, stderr io.Writer, questions traceStudyQuestions) int {
	flags := flag.NewFlagSet("trace study questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := questions()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace replication study question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceStudyAsk(args []string, stdout, stderr io.Writer, ask traceStudyAsker) int {
	flags := flag.NewFlagSet("trace study ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyAskAll(args []string, stdout, stderr io.Writer, askAll traceStudyAllAsker) int {
	flags := flag.NewFlagSet("trace study ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace replication study questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeTraceStudyAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func writeTraceStudySummary(stdout io.Writer, heading string, summary trace.StudyVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\ncontrast_sha256: %s\norder_basis: %s\nruns: %d\npairs: %d\nsupported_runs: %d\nunknown_runs: %d\nreset_confirmed_pairs: %d\nbalanced_runs: %d\ncomplete_pairs: %d\nchanged_runs: %d\nno_change_runs: %d\nmixed_runs: %d\nunknown_pairs: %d\noutcome: %s\nevidence_state: %s\nreason: %s\nstudy_sha256: %s\n", heading, summary.SchemaVersion, summary.ContrastSHA256, summary.OrderBasis, summary.Runs, summary.Pairs, summary.SupportedRuns, summary.UnknownRuns, summary.ResetConfirmedPairs, summary.BalancedRuns, summary.CompletePairs, summary.ChangedRuns, summary.NoChangeRuns, summary.MixedRuns, summary.UnknownPairs, summary.Outcome, summary.EvidenceState, summary.Reason, summary.StudySHA256)
	return err
}

func writeTraceStudyAnswer(stdout io.Writer, answer trace.StudyQuestionAnswer) error {
	_, err := fmt.Fprintf(stdout, "trace replication study question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nstudy_sha256: %s\nruns: %d\npairs: %d\nsupported_runs: %d\nunknown_runs: %d\nreset_confirmed_pairs: %d\nbalanced_runs: %d\ncomplete_pairs: %d\nchanged_runs: %d\nno_change_runs: %d\nmixed_runs: %d\nunknown_pairs: %d\nreason: %s\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.Outcome, answer.StudySHA256, answer.Runs, answer.Pairs, answer.SupportedRuns, answer.UnknownRuns, answer.ResetConfirmedPairs, answer.BalancedRuns, answer.CompletePairs, answer.ChangedRuns, answer.NoChangeRuns, answer.MixedRuns, answer.UnknownPairs, answer.Reason)
	return err
}

type traceStudyRoundSaver func(string, string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error)
type traceStudyRoundVerifier func(string) (trace.ReplicationStudyQuestionRoundVerificationSummary, error)
type traceStudyReceiptAsker func(string, string) (trace.ReplicationStudyQuestionReceipt, error)
type traceStudyReceiptSaver func(string, string, string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error)
type traceStudyReceiptVerifier func(string) (trace.ReplicationStudyQuestionReceiptVerificationSummary, error)

func runTraceStudyAskAllSave(args []string, stdout, stderr io.Writer, save traceStudyRoundSaver) int {
	flags := flag.NewFlagSet("trace study ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyRoundSummary(stdout, "trace replication study question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyAskAllVerify(args []string, stdout, stderr io.Writer, verify traceStudyRoundVerifier) int {
	flags := flag.NewFlagSet("trace study ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace study ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace study ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyRoundSummary(stdout, "trace replication study question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyAskReceipt(args []string, stdout, stderr io.Writer, ask traceStudyReceiptAsker) int {
	flags := flag.NewFlagSet("trace study ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyReceipt(stdout, "trace replication study question receipt", receipt, ""); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyAskReceiptSave(args []string, stdout, stderr io.Writer, save traceStudyReceiptSaver) int {
	flags := flag.NewFlagSet("trace study ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyReceiptSummary(stdout, "trace replication study question receipt saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceStudyAskReceiptVerify(args []string, stdout, stderr io.Writer, verify traceStudyReceiptVerifier) int {
	flags := flag.NewFlagSet("trace study ask receipt verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace study ask receipt verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ReceiptSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace study ask receipt verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceStudyReceiptSummary(stdout, "trace replication study question receipt verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace study ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceStudyRoundSummary(stdout io.Writer, heading string, summary trace.ReplicationStudyQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nstudy_sha256: %s\nquestions: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.StudySHA256, summary.Questions, summary.RoundSHA256)
	return err
}

func writeTraceStudyReceiptSummary(stdout io.Writer, heading string, summary trace.ReplicationStudyQuestionReceiptVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nstudy_sha256: %s\nround_sha256: %s\nreceipt_sha256: %s\n", heading, summary.SchemaVersion, summary.QuestionID, summary.Question, summary.Result, summary.EvidenceState, summary.Outcome, summary.StudySHA256, summary.RoundSHA256, summary.ReceiptSHA256)
	return err
}

func writeTraceStudyReceipt(stdout io.Writer, heading string, receipt trace.ReplicationStudyQuestionReceipt, receiptSHA256 string) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nstudy_sha256: %s\nround_sha256: %s\nruns: %d\npairs: %d\n", heading, receipt.SchemaVersion, receipt.QuestionID, receipt.Question, receipt.Result, receipt.EvidenceState, receipt.Outcome, receipt.StudySHA256, receipt.RoundSHA256, receipt.Runs, receipt.Pairs); err != nil {
		return err
	}
	if receiptSHA256 != "" {
		if _, err := fmt.Fprintf(stdout, "receipt_sha256: %s\n", receiptSHA256); err != nil {
			return err
		}
	}
	if receipt.Reason != "" {
		_, err := fmt.Fprintf(stdout, "reason: %s\n", receipt.Reason)
		return err
	}
	return nil
}
