package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceArchiveSaver func([]trace.ArchiveInput, string) (trace.ArchiveVerificationSummary, error)
type traceAdapterArchiveSaver func([]string, string) (trace.ArchiveVerificationSummary, error)
type traceArchiveVerifier func(string) (trace.ArchiveVerificationSummary, error)
type traceArchiveQuestions func() []trace.ArchiveQuestion
type traceArchiveAsker func(string, string) (trace.ArchiveAnswer, error)
type traceArchiveAllAsker func(string) ([]trace.ArchiveAnswer, error)

func runTraceArchiveCreate(args []string, stdout, stderr io.Writer, save traceArchiveSaver, runSavers ...traceAdapterArchiveSaver) int {
	flags := flag.NewFlagSet("trace archive create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	var tracePaths, sessionPaths, runDirs []string
	flags.Func("trace", "", func(value string) error {
		tracePaths = append(tracePaths, value)
		return nil
	})
	flags.Func("session", "", func(value string) error {
		sessionPaths = append(sessionPaths, value)
		return nil
	})
	flags.Func("run", "", func(value string) error {
		runDirs = append(runDirs, value)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 ||
		(len(tracePaths) == 0 && len(runDirs) == 0) ||
		len(tracePaths) != len(sessionPaths) ||
		(len(tracePaths) > 0 && len(runDirs) > 0) {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	var summary trace.ArchiveVerificationSummary
	var err error
	if len(runDirs) > 0 {
		if len(runSavers) == 0 || runSavers[0] == nil {
			_, _ = io.WriteString(stderr, "ariadne: trace archive create: adapter-run archive support is unavailable\n")
			return 1
		}
		summary, err = runSavers[0](runDirs, flags.Arg(0))
	} else {
		inputs := make([]trace.ArchiveInput, len(tracePaths))
		for index := range tracePaths {
			inputs[index] = trace.ArchiveInput{TracePath: tracePaths[index], SessionPath: sessionPaths[index]}
		}
		summary, err = save(inputs, flags.Arg(0))
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive create: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive create: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveSummary(stdout, "trace archive complete", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive create: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveVerify(args []string, stdout, stderr io.Writer, verify traceArchiveVerifier) int {
	flags := flag.NewFlagSet("trace archive verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace archive verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive verify: %v\n", err)
		return 1
	}
	if expectSet && summary.ArchiveSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace archive verify: archive SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveSummary(stdout, "trace archive verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveQuestions(args []string, stdout, stderr io.Writer, questions traceArchiveQuestions) int {
	flags := flag.NewFlagSet("trace archive questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := questions()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace archive question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceArchiveAsk(args []string, stdout, stderr io.Writer, ask traceArchiveAsker) int {
	flags := flag.NewFlagSet("trace archive ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceArchiveAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceArchiveAskAll(args []string, stdout, stderr io.Writer, askAll traceArchiveAllAsker) int {
	flags := flag.NewFlagSet("trace archive ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace archive questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeTraceArchiveAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace archive ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func writeTraceArchiveSummary(stdout io.Writer, heading string, summary trace.ArchiveVerificationSummary) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\norder_basis: %s\nentries: %d\ncomplete: %d\npartial: %d\narchive_sha256: %s\n", heading, summary.SchemaVersion, summary.OrderBasis, summary.Entries, summary.Complete, summary.Partial, summary.ArchiveSHA256); err != nil {
		return err
	}
	for _, source := range summary.Sources {
		if _, err := fmt.Fprintf(stdout, "- source: %s\n  adapter: %s\n  entries: %d\n", source.Source, source.Adapter, source.Entries); err != nil {
			return err
		}
	}
	return nil
}

func writeTraceArchiveAnswer(stdout io.Writer, answer trace.ArchiveAnswer) error {
	if _, err := fmt.Fprintf(stdout, "trace archive question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\narchive_sha256: %s\nentries: %d\ncompared: %d\nchanged: %d\nsame: %d\nunknown: %d\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.ArchiveSHA256, answer.Entries, answer.Compared, answer.Changed, answer.Same, answer.Unknown); err != nil {
		return err
	}
	if strings.TrimSpace(answer.Reason) != "" {
		if _, err := fmt.Fprintf(stdout, "reason: %s\n", answer.Reason); err != nil {
			return err
		}
	}
	for _, source := range answer.Sources {
		if _, err := fmt.Fprintf(stdout, "- source: %s\n  adapter: %s\n  entries: %d\n", source.Source, source.Adapter, source.Entries); err != nil {
			return err
		}
	}
	return nil
}
