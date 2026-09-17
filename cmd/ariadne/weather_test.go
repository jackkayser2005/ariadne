package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func TestWeatherCLIRejectsInvalidOrUnavailableInputs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"browser", "weather"}, 2},
		{[]string{"browser", "weather", "--unknown"}, 2},
		{[]string{"browser", "weather", "verify"}, 2},
		{[]string{"browser", "weather", "verify", "--expect-sha256", "bad", t.TempDir()}, 2},
		{[]string{"browser", "weather", "verify", t.TempDir()}, 1},
		{[]string{"browser", "weather", "--driver", "relative", "--output", t.TempDir()}, 1},
	} {
		var out, err bytes.Buffer
		if code := run(tc.args, &out, &err); code != tc.code {
			t.Errorf("%v = %d (%s)", tc.args, code, err.String())
		}
	}
}
func TestWeatherCLIHumanAndJSONOutput(t *testing.T) {
	review := browser.WeatherReview{
		Ladder: minimize.LadderSummary{
			SelectionState: minimize.SelectionUnknown,
			EvidenceState:  "unknown",
		},
		ReceiptSHA256: strings.Repeat("a", 64),
		CandidateEvidence: []browser.WeatherCandidateEvidence{
			{Candidate: "precise", SessionCount: 4, AvailableSessions: 4, ResponseBackedSessions: 4, VisibilityGaps: []browser.WeatherGapEvidence{{ID: "blocked-origin"}}},
			{Candidate: "coarse", SessionCount: 2, AvailableSessions: 2},
			{Candidate: "denied", SessionCount: 2, UnavailableSessions: 2},
		},
	}
	for _, jsonOutput := range []bool{false, true} {
		t.Run(map[bool]string{false: "human", true: "json"}[jsonOutput], func(t *testing.T) {
			args := []string{"verify", "run"}
			if jsonOutput {
				args = []string{"verify", "--json", "run"}
			}
			var stdout, stderr bytes.Buffer
			exitCode := runWeatherWithVerifier(args, &stdout, &stderr, func(path string) (browser.WeatherReview, error) {
				if path != "run" {
					t.Fatalf("verify path = %q", path)
				}
				return review, nil
			})
			if exitCode != 0 || stderr.Len() != 0 {
				t.Fatalf("runWeatherWithVerifier() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
			if jsonOutput {
				var got browser.WeatherReview
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.ReceiptSHA256 != review.ReceiptSHA256 || got.Ladder.SelectionState != minimize.SelectionUnknown {
					t.Fatalf("JSON review = %#v", got)
				}
			} else {
				for _, marker := range []string{
					"Weather investigation verified.",
					"Location observed: 4 of 4",
					"city-level location: 2/2",
					"location access off: 0/2",
					"Minimum disclosure: unknown",
					"Visibility limits: blocked-origin",
					"Receipt SHA-256: " + review.ReceiptSHA256,
				} {
					if !strings.Contains(stdout.String(), marker) {
						t.Fatalf("human summary missing %q: %s", marker, stdout.String())
					}
				}
				if strings.Contains(stdout.String(), "https://") || strings.Contains(stdout.String(), "38.889484") {
					t.Fatalf("human summary exposed source details: %s", stdout.String())
				}
			}
		})
	}
}
