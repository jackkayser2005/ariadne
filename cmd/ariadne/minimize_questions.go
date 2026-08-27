package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

type minimizationQuestionLister func() []minimize.MinimizationQuestion
type minimizationQuestionAsker func(string, string) (minimize.MinimizationQuestionAnswer, error)
type minimizationQuestionAllAsker func(string) ([]minimize.MinimizationQuestionAnswer, error)
type minimizationQuestionRoundSaver func(string, string) (minimize.MinimizationQuestionRoundVerificationSummary, error)
type minimizationQuestionRoundVerifier func(string) (minimize.MinimizationQuestionRoundVerificationSummary, error)
type minimizationQuestionReceiptAsker func(string, string) (minimize.MinimizationQuestionReceipt, error)
type minimizationQuestionReceiptSaver func(string, string, string) (minimize.MinimizationQuestionReceiptVerificationSummary, error)
type minimizationQuestionReceiptVerifier func(string) (minimize.MinimizationQuestionReceiptVerificationSummary, error)

func runMinimizationQuestions(args []string, stdout, stderr io.Writer, list minimizationQuestionLister) int {
	flags := flag.NewFlagSet("experiment minimize questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := list()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "minimization question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runMinimizationAsk(args []string, stdout, stderr io.Writer, ask minimizationQuestionAsker) int {
	flags := flag.NewFlagSet("experiment minimize ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runMinimizationAskAll(args []string, stdout, stderr io.Writer, askAll minimizationQuestionAllAsker) int {
	flags := flag.NewFlagSet("experiment minimize ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "minimization questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeMinimizationQuestionAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runMinimizationAskAllSave(args []string, stdout, stderr io.Writer, save minimizationQuestionRoundSaver) int {
	flags := flag.NewFlagSet("experiment minimize ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionRoundSummary(stdout, "minimization question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runMinimizationAskAllVerify(args []string, stdout, stderr io.Writer, verify minimizationQuestionRoundVerifier) int {
	flags := flag.NewFlagSet("experiment minimize ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: experiment minimize ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment minimize ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionRoundSummary(stdout, "minimization question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runMinimizationAskReceipt(args []string, stdout, stderr io.Writer, ask minimizationQuestionReceiptAsker) int {
	flags := flag.NewFlagSet("experiment minimize ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionReceipt(stdout, "minimization question receipt", receipt, ""); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runMinimizationAskReceiptSave(args []string, stdout, stderr io.Writer, save minimizationQuestionReceiptSaver) int {
	flags := flag.NewFlagSet("experiment minimize ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionReceiptSummary(stdout, "minimization question receipt saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runMinimizationAskReceiptVerify(args []string, stdout, stderr io.Writer, verify minimizationQuestionReceiptVerifier) int {
	flags := flag.NewFlagSet("experiment minimize ask receipt verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: experiment minimize ask receipt verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ReceiptSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment minimize ask receipt verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeMinimizationQuestionReceiptSummary(stdout, "minimization question receipt verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment minimize ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeMinimizationQuestionAnswer(stdout io.Writer, answer minimize.MinimizationQuestionAnswer) error {
	selected := answer.SelectedCandidate
	if selected == "" {
		selected = "none"
	}
	_, err := fmt.Fprintf(stdout, "minimization question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\nselection_state: %s\nselected_candidate: %s\nminimization_sha256: %s\ncandidate_count: %d\nsupported_candidates: %d\nunknown_candidates: %d\nreason: %s\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.SelectionState, selected, answer.MinimizationSHA256, answer.CandidateCount, answer.SupportedCandidates, answer.UnknownCandidates, answer.Reason)
	return err
}

func writeMinimizationQuestionRoundSummary(stdout io.Writer, heading string, summary minimize.MinimizationQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nminimization_sha256: %s\nquestions: %d\ncandidates: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.MinimizationSHA256, summary.Questions, summary.Candidates, summary.RoundSHA256)
	return err
}

func writeMinimizationQuestionReceipt(stdout io.Writer, heading string, receipt minimize.MinimizationQuestionReceipt, receiptSHA256 string) error {
	selected := receipt.SelectedCandidate
	if selected == "" {
		selected = "none"
	}
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\nselection_state: %s\nselected_candidate: %s\nminimization_sha256: %s\nround_sha256: %s\ncandidate_count: %d\nsupported_candidates: %d\nunknown_candidates: %d\nreason: %s\n", heading, receipt.SchemaVersion, receipt.QuestionID, receipt.Question, receipt.Result, receipt.EvidenceState, receipt.SelectionState, selected, receipt.MinimizationSHA256, receipt.RoundSHA256, receipt.CandidateCount, receipt.SupportedCandidates, receipt.UnknownCandidates, receipt.Reason); err != nil {
		return err
	}
	if receiptSHA256 != "" {
		_, err := fmt.Fprintf(stdout, "receipt_sha256: %s\n", receiptSHA256)
		return err
	}
	return nil
}

func writeMinimizationQuestionReceiptSummary(stdout io.Writer, heading string, summary minimize.MinimizationQuestionReceiptVerificationSummary) error {
	selected := summary.SelectedCandidate
	if selected == "" {
		selected = "none"
	}
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\nselection_state: %s\nselected_candidate: %s\nminimization_sha256: %s\nround_sha256: %s\nreceipt_sha256: %s\ncandidate_count: %d\nsupported_candidates: %d\nunknown_candidates: %d\n", heading, summary.SchemaVersion, summary.QuestionID, summary.Question, summary.Result, summary.EvidenceState, summary.SelectionState, selected, summary.MinimizationSHA256, summary.RoundSHA256, summary.ReceiptSHA256, summary.CandidateCount, summary.SupportedCandidates, summary.UnknownCandidates)
	return err
}
