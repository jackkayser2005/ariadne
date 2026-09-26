package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/securefs"
)

func recordPair(ctx context.Context, options browser.CaptureOptions, duration time.Duration, root, order string, output io.Writer) error {
	if !slices.Contains([]string{"baseline-treatment", "treatment-baseline"}, order) || duration <= 0 || duration > 20*time.Minute || !slices.Contains([]string{"unchanged", "deny", "approximate"}, options.Location) || len(options.BlockOrigins) > 64 {
		return errors.New("paired recording controls are invalid")
	}
	if _, err := browser.InvestigationURL(options.URL); err != nil {
		return err
	}
	for _, origin := range options.BlockOrigins {
		u, err := browser.InvestigationURL(origin)
		if err != nil || u.Scheme+"://"+u.Host != origin {
			return errors.New("blocked destinations must be exact HTTP(S) origins")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := securefs.MkdirExclusive(root, 0700); err != nil {
		return errors.New("paired output directory already exists or is unsafe")
	}
	identity := browser.NewTrialIdentity(order)
	options.Markers = browser.GenerateMarkers()
	roles := []string{"baseline", "treatment"}
	if order == "treatment-baseline" {
		slices.Reverse(roles)
	}
	for _, role := range roles {
		if _, err := fmt.Fprintf(output, "Starting %s. Repeat the same task in each recording.\n", role); err != nil {
			return err
		}
		current := options
		currentIdentity := identity
		currentIdentity.Role = role
		current.Trial = &currentIdentity
		if role == "baseline" {
			current.Location = "unchanged"
			current.BlockOrigins = nil
		}
		if err := recordTimed(ctx, current, duration, filepath.Join(root, role), output); err != nil {
			return fmt.Errorf("paired recording incomplete; any completed run remains in %s: %w", root, err)
		}
	}
	_, err := fmt.Fprintf(output, "Compare with: ariadne compare %q %q\n", filepath.Join(root, "baseline"), filepath.Join(root, "treatment"))
	return err
}

func runJourneyCompare(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	baselineTask := flags.String("baseline-task", "unknown", "")
	trialTask := flags.String("trial-task", "unknown", "")
	profilePath := flags.String("profile", "", "")
	if err := flags.Parse(args); err != nil {
		_, _ = io.WriteString(stderr, guidedUsage)
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 2 || !slices.Contains([]string{"unknown", "works", "broken"}, *baselineTask) || !slices.Contains([]string{"unknown", "works", "broken"}, *trialTask) {
		_, _ = io.WriteString(stderr, guidedUsage)
		return 2
	}
	confirmation := browser.TaskConfirmation{Baseline: *baselineTask, Treatment: *trialTask}
	comparison, err := browser.CompareInvestigations(flags.Arg(0), flags.Arg(1), confirmation)
	if err == nil && *profilePath != "" {
		var profile browser.SiteProtectionProfile
		profile, err = browser.BuildSiteProtectionProfile(flags.Arg(0), flags.Arg(1), confirmation)
		if err == nil && profile.Test.ComparisonSHA256 != comparison.SHA256() {
			err = errors.New("comparison sources changed during profile export")
		}
		if err == nil {
			err = browser.SaveSiteProtectionProfile(*profilePath, profile)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "ariadne: compare:", err)
		return 1
	}
	if *jsonOutput {
		err = json.NewEncoder(stdout).Encode(comparison)
	} else {
		_, err = fmt.Fprintf(stdout, "Supported marker-bearing observations (baseline -> trial):\nRequests attempted: %d -> %d\nResponses observed: %d -> %d\nAttempts blocked: %d -> %d\nText WebSocket frames sent: %d -> %d\nReduction: %s (%s)\nUser reports: baseline %s, trial %s (%s)\nAutomatic functionality verification: %s\n", comparison.Baseline.Requests, comparison.Treatment.Requests, comparison.Baseline.Responses, comparison.Treatment.Responses, comparison.Baseline.Blocked, comparison.Treatment.Blocked, comparison.Baseline.WebSocketFrames, comparison.Treatment.WebSocketFrames, comparison.Reduction, comparison.EvidenceState, confirmation.Baseline, confirmation.Treatment, comparison.FunctionalityState, comparison.AutomaticFunctionality)
		for _, unknown := range comparison.Unknowns {
			if err == nil {
				_, err = fmt.Fprintln(stdout, "Unknown:", unknown)
			}
		}
		if err == nil && *profilePath != "" {
			_, err = fmt.Fprintln(stdout, "Private protection profile saved:", *profilePath)
		}
	}
	if err != nil {
		return 1
	}
	return 0
}
