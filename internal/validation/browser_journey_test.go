package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestValidateGuidedInvestigation(t *testing.T) {
	journey := trace.NewJourney()
	if err := journey.Append(trace.JourneyObservation{
		Kind: "request", Context: "network", Destination: "d1", Reference: "r1",
		Matches: []trace.JourneyMatch{{Marker: "m1", Category: "email", Encoding: "exact"}},
	}); err != nil {
		t.Fatal(err)
	}
	result := browser.CaptureResult{
		Journey:      journey,
		Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://private-fixture.invalid"}},
		Steps:        []browser.InvestigationStep{{Kind: "input", Selector: "input:nth-of-type(1)", Marker: "m1"}},
	}
	path := filepath.Join(t.TempDir(), "saved")
	bundle, err := browser.SaveInvestigation(path, result)
	if err != nil {
		t.Fatal(err)
	}
	check := func(path string) {
		t.Helper()
		report := Validate(path)
		if report.ArtifactKind != KindBrowserJourney || report.Overall != StatusWarning ||
			report.Identity != bundle.Receipt.JourneySHA256 || report.EvidenceState != evidence.Observed ||
			tierStatus(report, TierStructural) != StatusPass || tierStatus(report, TierIntegrity) != StatusPass ||
			tierStatus(report, TierBoundary) != StatusUnavailable || tierStatus(report, TierReplay) != StatusUnavailable {
			t.Fatalf("guided validation = %#v", report)
		}
		data, _ := json.Marshal(report)
		if strings.Contains(string(data), "private-fixture.invalid") || strings.Contains(string(data), "input:nth-of-type") {
			t.Fatalf("validation exposed private context: %s", data)
		}
	}
	check(path)
	portable, err := bundle.PortableJSON()
	if err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(t.TempDir(), "renamed-export.json")
	if err := os.WriteFile(exportPath, portable, 0o600); err != nil {
		t.Fatal(err)
	}
	check(exportPath)

	result.Journey.AddGap("worker-unavailable")
	partialPath := filepath.Join(t.TempDir(), "partial")
	if _, err := browser.SaveInvestigation(partialPath, result); err != nil {
		t.Fatal(err)
	}
	partial := Validate(partialPath)
	if partial.ArtifactKind != KindBrowserJourney || partial.Overall != StatusUnknown ||
		partial.EvidenceState != evidence.Unknown || tierStatus(partial, TierReplay) != StatusUnknown {
		t.Fatalf("partial guided validation = %#v", partial)
	}

	if err := os.WriteFile(filepath.Join(path, "weather.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindUnknown)
	if err := os.Remove(filepath.Join(path, "weather.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "journey.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindBrowserJourney)
	malformedPath := filepath.Join(t.TempDir(), "ariadne-evidence.json")
	if err := os.WriteFile(malformedPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(malformedPath), KindBrowserJourney)
}
