package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceReplicationQuestions func() []trace.ReplicationQuestion
type traceReplicationAsker func(string, string) (trace.ReplicationAnswer, error)
type traceReplicationAllAsker func(string) ([]trace.ReplicationAnswer, error)
type traceReplicationRoundSaver func(string, string) (trace.ReplicationQuestionRoundVerificationSummary, error)
type traceReplicationRoundVerifier func(string) (trace.ReplicationQuestionRoundVerificationSummary, error)
type traceReplicationReceiptAsker func(string, string) (trace.ReplicationQuestionReceipt, error)
type traceReplicationReceiptSaver func(string, string, string) (trace.ReplicationQuestionReceiptVerificationSummary, error)
type traceReplicationReceiptVerifier func(string) (trace.ReplicationQuestionReceiptVerificationSummary, error)

func runTraceReplicationQuestions(args []string, stdout, stderr io.Writer, questions traceReplicationQuestions) int {
	flags := flag.NewFlagSet("trace replication questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := questions()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace replication question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceReplicationAsk(args []string, stdout, stderr io.Writer, ask traceReplicationAsker) int {
	flags := flag.NewFlagSet("trace replication ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationAskAll(args []string, stdout, stderr io.Writer, askAll traceReplicationAllAsker) int {
	flags := flag.NewFlagSet("trace replication ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace replication questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeTraceReplicationAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceReplicationAskAllSave(args []string, stdout, stderr io.Writer, save traceReplicationRoundSaver) int {
	flags := flag.NewFlagSet("trace replication ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationRoundSummary(stdout, "trace replication question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationAskAllVerify(args []string, stdout, stderr io.Writer, verify traceReplicationRoundVerifier) int {
	flags := flag.NewFlagSet("trace replication ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace replication ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace replication ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationRoundSummary(stdout, "trace replication question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationAskReceipt(args []string, stdout, stderr io.Writer, ask traceReplicationReceiptAsker) int {
	flags := flag.NewFlagSet("trace replication ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationReceipt(stdout, "trace replication question receipt", receipt, ""); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationAskReceiptSave(args []string, stdout, stderr io.Writer, save traceReplicationReceiptSaver) int {
	flags := flag.NewFlagSet("trace replication ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationReceiptSummary(stdout, "trace replication question receipt saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceReplicationAskReceiptVerify(args []string, stdout, stderr io.Writer, verify traceReplicationReceiptVerifier) int {
	flags := flag.NewFlagSet("trace replication ask receipt verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace replication ask receipt verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ReceiptSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace replication ask receipt verify: receipt SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceReplicationReceiptSummary(stdout, "trace replication question receipt verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace replication ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceReplicationRoundSummary(stdout io.Writer, heading string, summary trace.ReplicationQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nledger_sha256: %s\nquestions: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.LedgerSHA256, summary.Questions, summary.RoundSHA256)
	return err
}

func writeTraceReplicationReceiptSummary(stdout io.Writer, heading string, summary trace.ReplicationQuestionReceiptVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nledger_sha256: %s\nround_sha256: %s\nreceipt_sha256: %s\n", heading, summary.SchemaVersion, summary.QuestionID, summary.Question, summary.Result, summary.EvidenceState, summary.Outcome, summary.LedgerSHA256, summary.RoundSHA256, summary.ReceiptSHA256)
	return err
}

func writeTraceReplicationAnswer(stdout io.Writer, answer trace.ReplicationAnswer) error {
	if _, err := fmt.Fprintf(stdout, "trace replication question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nledger_sha256: %s\npairs: %d\nbaseline_treatment_pairs: %d\ntreatment_baseline_pairs: %d\nreset_confirmed_pairs: %d\ncomplete_pairs: %d\nchanged_pairs: %d\nno_change_pairs: %d\nunknown_pairs: %d\norder_balanced: %t\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.Outcome, answer.LedgerSHA256, answer.Pairs, answer.BaselineTreatmentPairs, answer.TreatmentBaselinePairs, answer.ResetConfirmedPairs, answer.CompletePairs, answer.ChangedPairs, answer.NoChangePairs, answer.UnknownPairs, answer.OrderBalanced); err != nil {
		return err
	}
	if strings.TrimSpace(answer.Reason) != "" {
		_, err := fmt.Fprintf(stdout, "reason: %s\n", answer.Reason)
		return err
	}
	return nil
}

func writeTraceReplicationReceipt(stdout io.Writer, heading string, receipt trace.ReplicationQuestionReceipt, receiptSHA256 string) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\noutcome: %s\nledger_sha256: %s\nround_sha256: %s\npairs: %d\nchanged_pairs: %d\nno_change_pairs: %d\nunknown_pairs: %d\n", heading, receipt.SchemaVersion, receipt.QuestionID, receipt.Question, receipt.Result, receipt.EvidenceState, receipt.Outcome, receipt.LedgerSHA256, receipt.RoundSHA256, receipt.Pairs, receipt.ChangedPairs, receipt.NoChangePairs, receipt.UnknownPairs); err != nil {
		return err
	}
	if receiptSHA256 != "" {
		if _, err := fmt.Fprintf(stdout, "receipt_sha256: %s\n", receiptSHA256); err != nil {
			return err
		}
	}
	if strings.TrimSpace(receipt.Reason) != "" {
		_, err := fmt.Fprintf(stdout, "reason: %s\n", receipt.Reason)
		return err
	}
	return nil
}
