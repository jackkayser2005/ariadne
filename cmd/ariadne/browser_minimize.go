package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

const browserFixtureMinimizationTimeout = 90 * time.Minute

type browserFixtureMinimizationRunner func(context.Context, browser.FixtureMinimizationInput) error
type browserFixtureMinimizationVerifier func(string) (minimize.LadderSummary, error)

func runBrowserFixtureMinimize(args []string, stdout, stderr io.Writer, run browserFixtureMinimizationRunner, verify browserFixtureMinimizationVerifier) int {
	flags := flag.NewFlagSet("browser fixture minimize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	planPath := flags.String("plan", "", "")
	procedurePath := flags.String("procedure", "", "")
	driverPath := flags.String("driver", "", "")
	pairs := flags.Int("pairs", 0, "")
	outputDir := flags.String("output", "", "")
	var driverArgs []string
	flags.Func("driver-arg", "", func(value string) error {
		driverArgs = append(driverArgs, value)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *planPath == "" || *procedurePath == "" || *driverPath == "" || *pairs < 1 || *outputDir == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), browserFixtureMinimizationTimeout)
	defer cancel()
	if err := run(ctx, browser.FixtureMinimizationInput{
		PlanPath:      *planPath,
		ProcedurePath: *procedurePath,
		DriverPath:    *driverPath,
		DriverArgs:    driverArgs,
		OutputDir:     *outputDir,
		Pairs:         *pairs,
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize: %v\n", err)
		return 1
	}
	summary, err := verify(*outputDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize: verify output: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(stdout,
		"browser fixture minimization complete\nplan: %s\nvariable: %s\ncriterion: %s\nselected_candidate: %s\nselection_state: %s\nevidence_state: %s\nprocedure_sha256: %s\n",
		summary.PlanName,
		summary.Variable,
		summary.FunctionalityCriterion,
		summary.SelectedCandidate,
		summary.SelectionState,
		summary.EvidenceState,
		summary.ProcedureSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize: write output: %v\n", err)
		return 1
	}
	return 0
}

func runBrowserFixtureMinimizeVerify(args []string, stdout, stderr io.Writer, verify browserFixtureMinimizationVerifier) int {
	flags := flag.NewFlagSet("browser fixture minimize verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(stdout,
		"browser fixture minimization verified\nplan: %s\nselected_candidate: %s\nselection_state: %s\nevidence_state: %s\nprocedure_sha256: %s\n",
		summary.PlanName,
		summary.SelectedCandidate,
		summary.SelectionState,
		summary.EvidenceState,
		summary.ProcedureSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser fixture minimize verify: write output: %v\n", err)
		return 1
	}
	return 0
}
