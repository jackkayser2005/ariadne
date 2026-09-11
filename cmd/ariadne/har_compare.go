package main

import (
	"flag"
	"fmt"
	"github.com/jackkayser2005/ariadne/internal/browser"
	"io"
)

func runCompareHAR(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browser compare-har", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	origin := flags.String("origin", "", "")
	rules := flags.String("test-values", "", "")
	output := flags.String("output", "", "")
	if flags.Parse(args) != nil || flags.NArg() != 2 || *origin == "" || *rules == "" || *output == "" {
		fmt.Fprintln(stderr, "ariadne: browser compare-har: require --origin, --test-values, --output and two captures")
		return 2
	}
	if err := browser.SaveHARComparisonReport(flags.Arg(0), flags.Arg(1), *origin, *output, *rules); err != nil {
		fmt.Fprintln(stderr, "ariadne: browser compare-har:", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, "Saved a redacted capture comparison. Differences are observations, not privacy conclusions."); err != nil {
		return 1
	}
	return 0
}
