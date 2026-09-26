package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func comparisonFixture(t *testing.T, blocked bool, order string) (JourneyBundle, privateJourneyContext) {
	t.Helper()
	journey := trace.NewJourney()
	match := []trace.JourneyMatch{{Marker: "m1", Category: "email", Encoding: "exact"}}
	for _, observation := range []trace.JourneyObservation{{Kind: "input", Context: "page", Destination: "d1", Reference: "r1", Matches: match}, {Kind: "request", Context: "network", Destination: "d2", Reference: "r2", Matches: match}} {
		if err := journey.Append(observation); err != nil {
			t.Fatal(err)
		}
	}
	kind := "response"
	responseMatches := []trace.JourneyMatch{}
	if blocked {
		kind = "blocked"
		responseMatches = match
	}
	if err := journey.Append(trace.JourneyObservation{Kind: kind, Context: "network", Destination: "d2", Reference: "r2", Matches: responseMatches}); err != nil {
		t.Fatal(err)
	}
	bundle, err := BuildJourneyBundle(journey)
	if err != nil {
		t.Fatal(err)
	}
	identity := TrialIdentity{PairID: strings.Repeat("a", 64), InputSetID: strings.Repeat("b", 64), Role: "baseline", Order: order}
	started := int64(1)
	blocks := []string{}
	if blocked {
		identity.Role = "treatment"
		started = 2
		blocks = []string{"https://collector.invalid"}
	}
	if order == "treatment-baseline" {
		started = 3 - started
	}
	settings := &TrialSettings{SchemaVersion: 1, Identity: identity, StartedAt: started, Browser: "Chrome/153.0.0.0", Platform: "windows", Location: "unchanged", BlockOrigins: blocks}
	private := privateJourneyContext{SchemaVersion: 2, JourneySHA256: bundle.Receipt.JourneySHA256, Destinations: []DestinationName{{"d1", "https://site.invalid"}, {"d2", "https://collector.invalid"}}, Steps: []InvestigationStep{{Kind: "input", Selector: "input:nth-of-type(1)", Marker: "m1"}}, Trial: settings}
	if err := private.validate(bundle); err != nil {
		t.Fatal(err)
	}
	return bundle, private
}

func TestJourneyComparisonReductionAndUserFunctionalityStaySeparate(t *testing.T) {
	for _, order := range []string{"baseline-treatment", "treatment-baseline"} {
		t.Run(order, func(t *testing.T) {
			baseline, baseContext := comparisonFixture(t, false, order)
			treatment, trialContext := comparisonFixture(t, true, order)
			for _, status := range []string{"works", "broken", "unknown"} {
				comparison, err := compareJourneys(baseline, baseContext, treatment, trialContext, TaskConfirmation{Baseline: "works", Treatment: status})
				if err != nil {
					t.Fatal(err)
				}
				if !comparison.PairBound || comparison.Order != order || comparison.Reduction != "reduced" || comparison.EvidenceState != evidence.Observed || comparison.Baseline.Responses != 1 || comparison.Treatment.Blocked != 1 {
					t.Fatalf("comparison: %+v", comparison)
				}
				if comparison.AutomaticFunctionality != minimize.CandidateUnknown {
					t.Fatal("user report became automatic verification")
				}
				want := evidence.Claimed
				if status == "unknown" {
					want = evidence.Unknown
				}
				if comparison.FunctionalityState != want {
					t.Fatal("user confirmation state")
				}
				data, _ := json.Marshal(comparison)
				if strings.Contains(string(data), "site.invalid") || strings.Contains(string(data), "collector.invalid") {
					t.Fatal("comparison leaked private names")
				}
				if !journeyDigest(comparison.SHA256()) {
					t.Fatal("comparison identity")
				}
			}
		})
	}
}

func TestJourneyComparisonIncompleteOrUnboundEvidenceRemainsUnknown(t *testing.T) {
	for _, scenario := range []string{"partial", "legacy", "different-steps", "no-control", "empty-baseline", "unchanged"} {
		t.Run(scenario, func(t *testing.T) {
			base, bc := comparisonFixture(t, false, "baseline-treatment")
			trial, tc := comparisonFixture(t, true, "baseline-treatment")
			switch scenario {
			case "partial":
				trial.Journey.AddGap("worker-unavailable")
				trial, _ = BuildJourneyBundle(trial.Journey)
				tc.JourneySHA256 = trial.Receipt.JourneySHA256
			case "legacy":
				bc.SchemaVersion = 1
				bc.Trial = nil
				tc.SchemaVersion = 1
				tc.Trial = nil
			case "different-steps":
				tc.Steps = []InvestigationStep{}
			case "no-control":
				tc.Trial.BlockOrigins = []string{}
			case "empty-baseline":
				base, _ = BuildJourneyBundle(trace.NewJourney())
				bc.JourneySHA256 = base.Receipt.JourneySHA256
			case "unchanged":
				trial = base
				tc.JourneySHA256 = trial.Receipt.JourneySHA256
			}
			comparison, err := compareJourneys(base, bc, trial, tc, TaskConfirmation{"unknown", "unknown"})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "unchanged" {
				if comparison.Reduction != "not-reduced" {
					t.Fatal(comparison.Reduction)
				}
				return
			}
			if comparison.Reduction != "unknown" || comparison.EvidenceState != evidence.Unknown || len(comparison.Unknowns) == 0 {
				t.Fatalf("incomplete became conclusive: %+v", comparison)
			}
		})
	}
}

func TestJourneyComparisonRejectsPairDriftAndTampering(t *testing.T) {
	for _, mutate := range []func(*privateJourneyContext){
		func(c *privateJourneyContext) { c.Trial.Identity.PairID = strings.Repeat("c", 64) }, func(c *privateJourneyContext) { c.Trial.Identity.InputSetID = strings.Repeat("c", 64) }, func(c *privateJourneyContext) { c.Trial.Identity.Role = "baseline" }, func(c *privateJourneyContext) { c.Trial.Identity.Order = "treatment-baseline" }, func(c *privateJourneyContext) { c.Trial.StartedAt = 1 }, func(c *privateJourneyContext) { c.Trial.Browser = "Chrome/152.0.0.0" }, func(c *privateJourneyContext) { c.Trial.Platform = "linux" }, func(c *privateJourneyContext) { c.Destinations[0].Origin = "https://other.invalid" }, func(c *privateJourneyContext) { c.JourneySHA256 = "invalid" },
	} {
		base, bc := comparisonFixture(t, false, "baseline-treatment")
		trial, tc := comparisonFixture(t, true, "baseline-treatment")
		mutate(&tc)
		if _, err := compareJourneys(base, bc, trial, tc, TaskConfirmation{"works", "works"}); err == nil {
			t.Fatal("accepted mismatched pair")
		}
	}
	base, bc := comparisonFixture(t, false, "baseline-treatment")
	trial, tc := comparisonFixture(t, true, "baseline-treatment")
	if _, err := compareJourneys(base, bc, trial, tc, TaskConfirmation{"automatically-verified", "works"}); err == nil {
		t.Fatal("accepted forged automatic result")
	}
	trial.Receipt.JourneySHA256 = "changed"
	if _, err := compareJourneys(base, bc, trial, tc, TaskConfirmation{"unknown", "unknown"}); err == nil {
		t.Fatal("accepted tampering")
	}
}

func TestJourneyComparisonReadsPrivateBindingsAndExportsNone(t *testing.T) {
	root := t.TempDir()
	for i, role := range []string{"baseline", "treatment"} {
		bundle, private := comparisonFixture(t, i == 1, "baseline-treatment")
		_, err := SaveInvestigation(filepath.Join(root, role), CaptureResult{Journey: bundle.Journey, Destinations: private.Destinations, Steps: private.Steps, Trial: private.Trial})
		if err != nil {
			t.Fatal(err)
		}
	}
	comparison, err := CompareInvestigations(filepath.Join(root, "baseline"), filepath.Join(root, "treatment"), TaskConfirmation{"works", "works"})
	if err != nil || !comparison.PairBound {
		t.Fatalf("read comparison: %v", err)
	}
	bundle, _, err := ReadInvestigation(filepath.Join(root, "treatment"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := bundle.PortableJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "pair_id") || strings.Contains(string(data), "collector.invalid") || strings.Contains(string(data), "Chrome/") {
		t.Fatal("private trial settings leaked into portable evidence")
	}
	for _, paths := range [][2]string{{"missing", "treatment"}, {"baseline", "missing"}} {
		if _, err := CompareInvestigations(filepath.Join(root, paths[0]), filepath.Join(root, paths[1]), TaskConfirmation{"unknown", "unknown"}); err == nil {
			t.Fatal("missing source accepted")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "treatment", "private-context.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareInvestigations(filepath.Join(root, "baseline"), filepath.Join(root, "treatment"), TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("tampered private settings accepted")
	}
}

func TestJourneyDisclosureRejectsContradictoryReferences(t *testing.T) {
	for _, scenario := range []string{"duplicate", "unbound", "destination", "contradictory"} {
		bundle, _ := comparisonFixture(t, false, "baseline-treatment")
		journey := bundle.Journey
		switch scenario {
		case "duplicate":
			journey.Observations = append(journey.Observations, journey.Observations[1])
		case "unbound":
			journey.Observations[2].Reference = "r9"
			journey.Observations[2].Matches = journey.Observations[1].Matches
		case "destination":
			journey.Observations[2].Destination = "d1"
		case "contradictory":
			blocked := journey.Observations[2]
			blocked.Kind = "blocked"
			journey.Observations = append(journey.Observations, blocked)
		}
		if _, err := journeyDisclosure(journey); err == nil {
			t.Errorf("accepted %s", scenario)
		}
	}
	bundle, _ := comparisonFixture(t, false, "baseline-treatment")
	journey := bundle.Journey
	for _, observation := range []trace.JourneyObservation{{Kind: "blocked", Reference: "r99"}, {Kind: "request", Reference: "r3", Destination: "d1"}, {Kind: "response", Reference: "r3", Destination: "d1"}, {Kind: "websocket-sent", Matches: journey.Observations[1].Matches}} {
		journey.Observations = append(journey.Observations, observation)
	}
	count, err := journeyDisclosure(journey)
	if err != nil || count.WebSocketFrames != 1 || count.Requests != 1 {
		t.Fatalf("supported counts: %+v %v", count, err)
	}
}

func TestTrialSettingsRejectUnboundControlsAndLegacyUpgrade(t *testing.T) {
	identity := NewTrialIdentity("baseline-treatment")
	if identity.validate() != nil || identity.PairID == NewTrialIdentity("baseline-treatment").PairID {
		t.Fatal("trial identities not fresh")
	}
	settings, err := newTrialSettings(identity, "Chrome/153.0.0.0", CaptureOptions{Location: "unchanged"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TrialSettings){func(s *TrialSettings) { s.SchemaVersion = 2 }, func(s *TrialSettings) { s.StartedAt = 0 }, func(s *TrialSettings) { s.Browser = "private-unsafe" }, func(s *TrialSettings) { s.Platform = "unknown" }, func(s *TrialSettings) { s.Location = "invalid" }, func(s *TrialSettings) { s.Location = "deny" }, func(s *TrialSettings) { s.BlockOrigins = nil }, func(s *TrialSettings) { s.Identity.Role = "invalid" }, func(s *TrialSettings) { s.Identity.InputSetID = "" }} {
		copy := *settings
		mutate(&copy)
		if copy.validate() == nil {
			t.Fatal("accepted invalid trial settings")
		}
	}
	identity.Role = "treatment"
	settings, err = newTrialSettings(identity, "Chrome/153.0.0.0", CaptureOptions{Location: "deny", BlockOrigins: []string{"https://b.invalid", "https://a.invalid", "https://a.invalid"}})
	if err != nil || !slices.Equal(settings.BlockOrigins, []string{"https://a.invalid", "https://b.invalid"}) {
		t.Fatalf("canonical controls: %+v %v", settings, err)
	}
	for _, origins := range [][]string{{"https://a.invalid/path"}, {"https://b.invalid", "https://a.invalid"}} {
		settings.BlockOrigins = origins
		if settings.validate() == nil {
			t.Fatal("accepted invalid origins")
		}
	}
	bundle, private := comparisonFixture(t, false, "baseline-treatment")
	private.SchemaVersion = 1
	if private.validate(bundle) == nil {
		t.Fatal("legacy context invented trial semantics")
	}
	private.SchemaVersion = 2
	private.Trial = nil
	if private.validate(bundle) == nil {
		t.Fatal("new context omitted trial settings")
	}
}
