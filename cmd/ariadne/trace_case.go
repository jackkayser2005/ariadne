package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

type traceCaseSaver func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error)
type traceCaseVerifier func(string) (trace.CaseVerificationSummary, error)
type traceCaseQuestions func() []trace.CaseQuestion
type traceCaseAsker func(string, string) (trace.CaseAnswer, error)
type traceCaseAllAsker func(string) ([]trace.CaseAnswer, error)
type traceCaseRoundSaver func(string, string) (trace.CaseQuestionRoundVerificationSummary, error)
type traceCaseRoundVerifier func(string) (trace.CaseQuestionRoundVerificationSummary, error)
type traceCaseMapper func(string) (trace.CaseDisclosureMap, error)

func mapTraceCase(path string) (trace.CaseDisclosureMap, error) {
	casePackage, summary, err := trace.ReadCase(path)
	if err != nil {
		return trace.CaseDisclosureMap{}, err
	}
	return trace.BuildCaseDisclosureMap(casePackage, summary)
}

func runTraceCaseMap(args []string, stdout, stderr io.Writer, mapCase traceCaseMapper) int {
	flags := flag.NewFlagSet("trace case map", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	result, err := mapCase(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case map: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseMap(stdout, result); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case map: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseSave(args []string, stdout, stderr io.Writer, save traceCaseSaver) int {
	flags := flag.NewFlagSet("trace case save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() < 4 || (flags.NArg()-1)%3 != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	inputs := make([]trace.CaseInput, 0, (flags.NArg()-1)/3)
	for index := 1; index < flags.NArg(); index += 3 {
		inputs = append(inputs, trace.CaseInput{Kind: flags.Arg(index), ArtifactPath: flags.Arg(index + 1), QuestionRoundPath: flags.Arg(index + 2)})
	}
	summary, err := save(inputs, flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseSummary(stdout, "trace case saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseVerify(args []string, stdout, stderr io.Writer, verify traceCaseVerifier) int {
	flags := flag.NewFlagSet("trace case verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace case verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case verify: %v\n", err)
		return 1
	}
	if expectSet && summary.CaseSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace case verify: case SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseSummary(stdout, "trace case verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseQuestions(args []string, stdout, stderr io.Writer, questions traceCaseQuestions) int {
	flags := flag.NewFlagSet("trace case questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	catalog := questions()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(catalog); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace case question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case questions: write output: %v\n", err)
		return 1
	}
	for _, question := range catalog {
		if _, err := fmt.Fprintf(stdout, "- %s: %s\n", question.ID, question.Text); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceCaseAsk(args []string, stdout, stderr io.Writer, ask traceCaseAsker) int {
	flags := flag.NewFlagSet("trace case ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseAnswer(stdout, answer); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseAskAll(args []string, stdout, stderr io.Writer, askAll traceCaseAllAsker) int {
	flags := flag.NewFlagSet("trace case ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	answers, err := askAll(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answers); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "trace case questions answered\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all: write output: %v\n", err)
		return 1
	}
	for _, answer := range answers {
		if err := writeTraceCaseAnswer(stdout, answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceCaseAskAllSave(args []string, stdout, stderr io.Writer, save traceCaseRoundSaver) int {
	flags := flag.NewFlagSet("trace case ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseRoundSummary(stdout, "trace case question round saved", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCaseAskAllVerify(args []string, stdout, stderr io.Writer, verify traceCaseRoundVerifier) int {
	flags := flag.NewFlagSet("trace case ask all verify", flag.ContinueOnError)
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
		_, _ = io.WriteString(stderr, "ariadne: trace case ask all verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all verify: %v\n", err)
		return 1
	}
	if expectSet && summary.RoundSHA256 != *expectSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace case ask all verify: question round SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceCaseRoundSummary(stdout, "trace case question round verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace case ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceCaseSummary(stdout io.Writer, heading string, summary trace.CaseVerificationSummary) error {
	if _, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\norder_basis: %s\nentries: %d\narchives: %d\nreplications: %d\nunknown_entries: %d\ncase_sha256: %s\n", heading, summary.SchemaVersion, summary.OrderBasis, summary.Entries, summary.Archives, summary.Replications, summary.UnknownEntries, summary.CaseSHA256); err != nil {
		return err
	}
	for _, source := range summary.Sources {
		if _, err := fmt.Fprintf(stdout, "- source: %s\n  adapter: %s\n  entries: %d\n", source.Source, source.Adapter, source.Entries); err != nil {
			return err
		}
	}
	for _, outcome := range summary.Outcomes {
		if _, err := fmt.Fprintf(stdout, "- position: %d\n  outcome: %s\n  evidence_state: %s\n  pairs: %d\n  unknown_pairs: %d\n", outcome.Position, outcome.Outcome, outcome.EvidenceState, outcome.Pairs, outcome.UnknownPairs); err != nil {
			return err
		}
	}
	return nil
}

func writeTraceCaseAnswer(stdout io.Writer, answer trace.CaseAnswer) error {
	if _, err := fmt.Fprintf(stdout, "trace case question answered\nquestion_id: %s\nquestion: %s\nresult: %s\nevidence_state: %s\ncase_sha256: %s\nentries: %d\narchives: %d\nreplications: %d\nunknown_entries: %d\n", answer.QuestionID, answer.Question, answer.Result, answer.EvidenceState, answer.CaseSHA256, answer.Entries, answer.Archives, answer.Replications, answer.UnknownEntries); err != nil {
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
	for _, outcome := range answer.Outcomes {
		if _, err := fmt.Fprintf(stdout, "- position: %d\n  outcome: %s\n  evidence_state: %s\n  pairs: %d\n  unknown_pairs: %d\n", outcome.Position, outcome.Outcome, outcome.EvidenceState, outcome.Pairs, outcome.UnknownPairs); err != nil {
			return err
		}
	}
	return nil
}

func writeTraceCaseRoundSummary(stdout io.Writer, heading string, summary trace.CaseQuestionRoundVerificationSummary) error {
	_, err := fmt.Fprintf(stdout, "%s\nschema_version: %d\ncase_sha256: %s\nquestions: %d\nround_sha256: %s\n", heading, summary.SchemaVersion, summary.CaseSHA256, summary.Questions, summary.RoundSHA256)
	return err
}

func writeTraceCaseMap(stdout io.Writer, result trace.CaseDisclosureMap) error {
	if _, err := fmt.Fprintf(stdout, "trace case disclosure map\nschema_version: %d\ncase_sha256: %s\ntraces: %d\ncoverage_state: %s\n", result.SchemaVersion, result.CaseSHA256, result.Traces, result.CoverageState); err != nil {
		return err
	}
	for _, category := range result.Categories {
		if _, err := fmt.Fprintf(stdout, "- category: %s\n", category.Category); err != nil {
			return err
		}
		for _, observation := range category.Observations {
			if _, err := fmt.Fprintf(stdout, "  source: %s\n  adapter: %s\n  channel: %s\n  kind: %s\n  destination: %s\n  trace_count: %d\n  evidence_state: %s\n", observation.Source, observation.Adapter, observation.Channel, observation.Kind, observation.Destination, observation.TraceCount, observation.EvidenceState); err != nil {
				return err
			}
		}
	}
	return nil
}
