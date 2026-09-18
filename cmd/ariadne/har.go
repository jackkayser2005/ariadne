package main

import (
	"flag"
	"fmt"
	"github.com/jackkayser2005/ariadne/internal/browser"
	"io"
)

func runHAR(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browser inspect-har", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	origin := flags.String("origin", "", "")
	output := flags.String("output", "", "")
	rules := flags.String("test-values", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 1 || *origin == "" || *output == "" {
		fmt.Fprintln(stderr, "ariadne: browser inspect-har: require --origin <origin> --output <new.html> <capture.har>")
		return 2
	}
	if err := browser.SaveHARReport(flags.Arg(0), *origin, *output, *rules); err != nil {
		fmt.Fprintln(stderr, "ariadne: browser inspect-har:", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Saved a redacted browser inventory. Field-name clues are not confirmed disclosures."); err != nil {
		return 1
	}
	return 0
}
