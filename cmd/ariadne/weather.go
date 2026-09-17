package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

type weatherVerifier func(string) (browser.WeatherReview, error)

func runWeather(args []string, stdout, stderr io.Writer) int {
	return runWeatherWithVerifier(args, stdout, stderr, browser.VerifyWeather)
}

func runWeatherWithVerifier(args []string, stdout, stderr io.Writer, verify weatherVerifier) int {
	verifyMode := len(args) > 0 && args[0] == "verify"
	if verifyMode {
		args = args[1:]
	}
	flags := flag.NewFlagSet("browser weather", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	driver := flags.String("driver", "", "")
	output := flags.String("output", "", "")
	expected := flags.String("expect-sha256", "", "")
	var driverArgs []string
	flags.Func("driver-arg", "", func(v string) error {
		driverArgs = append(driverArgs, v)
		return nil
	})
	if flags.Parse(args) != nil ||
		(verifyMode && flags.NArg() != 1) ||
		(!verifyMode && (flags.NArg() != 0 || *driver == "" || *output == "")) ||
		(*expected != "" && !trace.ValidSHA256(*expected)) {
		fmt.Fprintln(stderr, "ariadne: browser weather: invalid arguments")
		return 2
	}
	root := *output
	if verifyMode {
		root = flags.Arg(0)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		if err := browser.RunWeather(ctx, browser.WeatherInput{DriverPath: *driver, DriverArgs: driverArgs, OutputDir: root}); err != nil {
			fmt.Fprintln(stderr, "ariadne:", err)
			return 1
		}
	}
	review, err := verify(root)
	if err != nil {
		fmt.Fprintln(stderr, "ariadne: weather verification failed")
		return 1
	}
	if *expected != "" && review.ReceiptSHA256 != *expected {
		fmt.Fprintln(stderr, "ariadne: weather receipt identity mismatch")
		return 1
	}
	if *jsonOutput {
		if json.NewEncoder(stdout).Encode(review) != nil {
			return 1
		}
		return 0
	}
	if err := writeWeatherSummary(stdout, review); err != nil {
		fmt.Fprintf(stderr, "ariadne: browser weather: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeWeatherSummary(stdout io.Writer, review browser.WeatherReview) error {
	if err := writeCLIStatus(stdout, "Weather investigation verified.\n"); err != nil {
		return err
	}
	precise, preciseOK := weatherCandidate(review, "precise")
	if preciseOK && precise.ResponseBackedSessions > 0 {
		if _, err := fmt.Fprintf(stdout, "Location observed: %d of %d precise-location sessions had a matching request and response.\n", precise.ResponseBackedSessions, precise.SessionCount); err != nil {
			return err
		}
	} else if _, err := io.WriteString(stdout, "Location observed: unknown; no supported response-backed match was recorded.\n"); err != nil {
		return err
	}
	if err := writeWeatherTrail(stdout, precise, preciseOK); err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, "Forecast results:\n"); err != nil {
		return err
	}
	for _, candidate := range []struct {
		id    string
		label string
	}{
		{id: "precise", label: "  precise location"},
		{id: "coarse", label: "  city-level location"},
		{id: "denied", label: "  location access off"},
	} {
		evidence, ok := weatherCandidate(review, candidate.id)
		if !ok {
			if _, err := fmt.Fprintf(stdout, "%s: unknown\n", candidate.label); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s: %d/%d sessions showed a forecast\n", candidate.label, evidence.AvailableSessions, evidence.SessionCount); err != nil {
			return err
		}
	}
	if review.Ladder.SelectionState == "unknown" {
		if _, err := io.WriteString(stdout, "Minimum disclosure: unknown; the test could not see every channel.\n"); err != nil {
			return err
		}
	} else {
		selected := review.Ladder.SelectedCandidate
		if selected == "" {
			selected = "none"
		}
		if _, err := fmt.Fprintf(stdout, "Minimum disclosure: %s.\n", selected); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "Evidence state: %s\nSelection state: %s\nReceipt SHA-256: %s\n", review.Ladder.EvidenceState, review.Ladder.SelectionState, review.ReceiptSHA256); err != nil {
		return err
	}
	if preciseOK && len(precise.VisibilityGaps) > 0 {
		if _, err := fmt.Fprintf(stdout, "Visibility limits: %s\n", formatWeatherGaps(precise.VisibilityGaps)); err != nil {
			return err
		}
	}
	return nil
}

func writeWeatherTrail(stdout io.Writer, precise browser.WeatherCandidateEvidence, available bool) error {
	switch {
	case available && precise.ResponseBackedSessions > 0:
		_, err := fmt.Fprintf(stdout, "Information trail: browser -> Location -> Weather service (response-backed in %d/%d precise-location sessions).\n", precise.ResponseBackedSessions, precise.SessionCount)
		return err
	case available && precise.AttemptedSessions > 0:
		_, err := io.WriteString(stdout, "Information trail: browser attempted to send Location; a response-backed destination was not confirmed.\n")
		return err
	default:
		_, err := io.WriteString(stdout, "Information trail: unknown; no supported location path was recorded.\n")
		return err
	}
}

func formatWeatherGaps(gaps []browser.WeatherGapEvidence) string {
	parts := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		if gap.Reason == "" {
			parts = append(parts, gap.ID)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", gap.ID, gap.Reason))
	}
	return strings.Join(parts, ", ")
}

func weatherCandidate(review browser.WeatherReview, candidate string) (browser.WeatherCandidateEvidence, bool) {
	for _, evidence := range review.CandidateEvidence {
		if evidence.Candidate == candidate {
			return evidence, true
		}
	}
	return browser.WeatherCandidateEvidence{}, false
}
