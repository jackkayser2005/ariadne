package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceCaseDisclosureQuestionLister func() []trace.CaseDisclosureQuestion
type traceCaseDisclosureQuestionAsker func(string, string) (trace.CaseDisclosureQuestionAnswer, error)
type traceCaseDisclosureQuestionAllAsker func(string) ([]trace.CaseDisclosureQuestionAnswer, error)
type traceCaseDisclosureQuestionRoundSaver func(string, string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error)
type traceCaseDisclosureQuestionRoundVerifier func(string) (trace.CaseDisclosureQuestionRoundVerificationSummary, error)
type traceCaseDisclosureQuestionReceiptAsker func(string, string) (trace.CaseDisclosureQuestionReceipt, error)
type traceCaseDisclosureQuestionReceiptSaver func(string, string, string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error)
type traceCaseDisclosureQuestionReceiptVerifier func(string) (trace.CaseDisclosureQuestionReceiptVerificationSummary, error)

func runTraceCaseMapQuestions(args []string, stdout, stderr io.Writer, questions traceCaseDisclosureQuestionLister) int {
	flags := flag.NewFlagSet("trace case map questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := questions()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace case disclosure question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceCaseMapAsk(args []string, stdout, stderr io.Writer, ask traceCaseDisclosureQuestionAsker) int {
	flags := flag.NewFlagSet("trace case map ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseMapAskAll(args []string, stdout, stderr io.Writer, askAll traceCaseDisclosureQuestionAllAsker) int {
	flags := flag.NewFlagSet("trace case map ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace case disclosure questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeTraceCaseDisclosureAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceCaseMapAskAllSave(args []string, stdout, stderr io.Writer, save traceCaseDisclosureQuestionRoundSaver) int {
	flags := flag.NewFlagSet("trace case map ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureRoundSummary(stdout, "trace case disclosure question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseMapAskAllVerify(args []string, stdout, stderr io.Writer, verify traceCaseDisclosureQuestionRoundVerifier) int {
	flags := flag.NewFlagSet("trace case map ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace case map ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace case map ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureRoundSummary(stdout, "trace case disclosure question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseMapAskReceipt(args []string, stdout, stderr io.Writer, ask traceCaseDisclosureQuestionReceiptAsker) int {
	flags := flag.NewFlagSet("trace case map ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureReceipt(stdout, "trace case disclosure question receipt", receipt, ""); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseMapAskReceiptSave(args []string, stdout, stderr io.Writer, save traceCaseDisclosureQuestionReceiptSaver) int {
	flags := flag.NewFlagSet("trace case map ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureReceiptSummary(stdout, "trace case disclosure question receipt saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseMapAskReceiptVerify(args []string, stdout, stderr io.Writer, verify traceCaseDisclosureQuestionReceiptVerifier) int {
	flags := flag.NewFlagSet("trace case map ask receipt verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace case map ask receipt verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ReceiptSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace case map ask receipt verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseDisclosureReceiptSummary(stdout, "trace case disclosure question receipt verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceCaseDisclosureAnswer(stdout io.Writer, answer trace.CaseDisclosureQuestionAnswer) error {
	if _, err := fmt.Fprintf(stdout, "trace case disclosure question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\ncase_sha256: %s\ntraces: %d\ncoverage_state: %s\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.CaseSHA256, answer.Traces, answer.CoverageState); err != nil {
		return err
	}
	if strings.TrimSpace(answer.Reason) != "" {
		if _, err := fmt.Fprintf(stdout, "reason: %s\n", answer.Reason); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "overlapping_categories: %s\n", strings.Join(answer.OverlappingCategories, ",")); err != nil {
		return err
	}
	for _, category := range answer.Categories {
		if _, err := fmt.Fprintf(stdout, "- category: %s\n", category.Category); err != nil {
			return err
		}
		for _, boundary := range category.Boundaries {
			if _, err := fmt.Fprintf(stdout, "  source: %s\n  adapter: %s\n", boundary.Source, boundary.Adapter); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeTraceCaseDisclosureRoundSummary(stdout io.Writer, heading string, summary trace.CaseDisclosureQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\ncase_sha256: %s\nquestions: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.CaseSHA256, summary.Questions, summary.RoundSHA256)
	return err
}

func writeTraceCaseDisclosureReceiptSummary(stdout io.Writer, heading string, summary trace.CaseDisclosureQuestionReceiptVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\ncase_sha256: %s\nround_sha256: %s\nreceipt_sha256: %s\n", heading, summary.SchemaVersion, summary.QuestionID, summary.Question, summary.Result, summary.EvidenceState, summary.CaseSHA256, summary.RoundSHA256, summary.ReceiptSHA256)
	return err
}

func writeTraceCaseDisclosureReceipt(stdout io.Writer, heading string, receipt trace.CaseDisclosureQuestionReceipt, receiptSHA256 string) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\ncase_sha256: %s\nround_sha256: %s\ntraces: %d\ncoverage_state: %s\n", heading, receipt.SchemaVersion, receipt.QuestionID, receipt.Question, receipt.Result, receipt.EvidenceState, receipt.CaseSHA256, receipt.RoundSHA256, receipt.Traces, receipt.CoverageState); err != nil {
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
	if _, err := fmt.Fprintf(stdout, "overlapping_categories: %s\n", strings.Join(receipt.OverlappingCategories, ",")); err != nil {
		return err
	}
	return nil
}
