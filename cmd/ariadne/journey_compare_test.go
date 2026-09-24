package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestJourneyCompareCLIReportsUncertaintyAndExportsPrivateProfile(t *testing.T) {
	root := t.TempDir()
	identity := browser.NewTrialIdentity("baseline-treatment")
	for i, role := range []string{"baseline", "treatment"} {
		id := identity
		id.Role = role
		location := "unchanged"
		if i == 1 {
			location = "deny"
		}
		settings := &browser.TrialSettings{SchemaVersion: 1, Identity: id, StartedAt: int64(i + 1), Browser: "Chrome/153.0.0.0", Platform: "windows", Location: location, BlockOrigins: []string{}}
		journey := trace.NewJourney()
		journey.AddGap("instrumentation-unavailable")
		if _, err := browser.SaveInvestigation(filepath.Join(root, role), browser.CaptureResult{Journey: journey, Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://site.invalid"}}, Steps: []browser.InvestigationStep{}, Trial: settings}); err != nil {
			t.Fatal(err)
		}
	}
	baseline, trial := filepath.Join(root, "baseline"), filepath.Join(root, "treatment")
	var out, stderr bytes.Buffer
	profilePath := filepath.Join(root, "private.json")
	if code := run([]string{"compare", "--json", "--baseline-task", "works", "--trial-task", "broken", "--profile", profilePath, baseline, trial}, &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var comparison browser.JourneyComparison
	if err := json.Unmarshal(out.Bytes(), &comparison); err != nil || comparison.Reduction != "unknown" || comparison.Functionality.Treatment != "broken" {
		t.Fatal("incorrect comparison", err)
	}
	if strings.Contains(out.String(), "site.invalid") {
		t.Fatal("comparison output leaked names")
	}
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := browser.DecodeSiteProtectionProfile(data); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runJourneyCompare([]string{baseline, trial}, &out, &stderr); code != 0 || !strings.Contains(out.String(), "Automatic functionality verification: unknown") {
		t.Fatal("missing user/automatic distinction")
	}
	for _, args := range [][]string{{"--bad"}, {}, {baseline}, {"--trial-task", "verified", baseline, trial}} {
		if code := runJourneyCompare(args, &out, &stderr); code != 2 {
			t.Fatal("accepted invalid flags", args)
		}
	}
	if code := runJourneyCompare([]string{"--help"}, &out, &stderr); code != 0 {
		t.Fatal(code)
	}
	if code := runJourneyCompare([]string{"--profile", profilePath, baseline, trial}, &out, &stderr); code != 1 {
		t.Fatal("profile overwrite accepted")
	}
	if code := runJourneyCompare([]string{"--profile", filepath.Join(root, "second-private.json"), baseline, trial}, &out, &stderr); code != 0 {
		t.Fatal(stderr.String())
	}
	if err := os.WriteFile(filepath.Join(trial, "private-context.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runJourneyCompare([]string{baseline, trial}, &out, &stderr); code != 1 {
		t.Fatal("tampered comparison accepted")
	}
}

func TestJourneyPairCLIRejectsInvalidControlsBeforeRecording(t *testing.T) {
	for _, args := range [][]string{{"--pair"}, {"--pair", "--duration", "0s"}, {"--order", "treatment-baseline"}, {"--pair", "--duration", "1s", "--order", "wrong", "https://site.invalid"}} {
		if code := runGuided("investigate", args, io.Discard, io.Discard, nil); code != 2 {
			t.Fatal("invalid pair flags accepted")
		}
	}
	options := browser.CaptureOptions{URL: "https://site.invalid", Location: "deny"}
	for _, scenario := range []string{"order", "duration", "location", "origin", "url", "exists", "cancelled"} {
		current := options
		order := "baseline-treatment"
		duration := time.Second
		root := filepath.Join(t.TempDir(), "pair")
		ctx := context.Background()
		switch scenario {
		case "order":
			order = "invalid"
		case "duration":
			duration = 0
		case "location":
			current.Location = "actual"
		case "origin":
			current.BlockOrigins = []string{"https://site.invalid/path"}
		case "url":
			current.URL = "file:///secret"
		case "exists":
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
		case "cancelled":
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		if err := recordPair(ctx, current, duration, root, order, io.Discard); err == nil {
			t.Fatal("accepted", scenario)
		}
	}
}

func TestJourneyPairTimedCLIRecordsBothExecutionOrders(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("requires installed Chrome/Edge")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<title>Paired terminal fixture</title>")
	}))
	defer site.Close()
	for _, order := range []string{"baseline-treatment", "treatment-baseline"} {
		root := filepath.Join(t.TempDir(), "pair")
		if err := recordPair(context.Background(), browser.CaptureOptions{URL: site.URL, Location: "deny", Headless: true}, 50*time.Millisecond, root, order, io.Discard); err != nil {
			t.Fatal(err)
		}
		comparison, err := browser.CompareInvestigations(filepath.Join(root, "baseline"), filepath.Join(root, "treatment"), browser.TaskConfirmation{Baseline: "unknown", Treatment: "unknown"})
		if err != nil || !comparison.PairBound || comparison.Order != order || comparison.Reduction != "unknown" {
			t.Fatalf("order %s: %+v %v", order, comparison, err)
		}
	}
}
