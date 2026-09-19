package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

func profileFixture(t *testing.T, location string) (string, string) {
	t.Helper()
	root := t.TempDir()
	for i, role := range []string{"baseline", "treatment"} {
		bundle, private := comparisonFixture(t, i == 1, "baseline-treatment")
		if i == 1 {
			private.Trial.Location = location
		}
		_, err := SaveInvestigation(filepath.Join(root, role), CaptureResult{Journey: bundle.Journey, Destinations: private.Destinations, Steps: private.Steps, Trial: private.Trial})
		if err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(root, "baseline"), filepath.Join(root, "treatment")
}

func TestPrivateSiteProfileBindsRecordedControlsAndUserReports(t *testing.T) {
	base, trial := profileFixture(t, "deny")
	profile, err := BuildSiteProtectionProfile(base, trial, TaskConfirmation{"works", "broken"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.SiteOrigin != "https://site.invalid" || profile.Controls.Location != "deny" || len(profile.Controls.BlockOrigins) != 1 || profile.Test.Functionality.Treatment != "broken" || profile.Test.FunctionalityState != evidence.Claimed || profile.Test.AutomaticFunctionality != minimize.CandidateUnknown {
		t.Fatalf("profile: %+v", profile)
	}
	data, err := profile.PrivateJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSiteProtectionProfile(data)
	if err != nil || decoded.ProfileSHA256 != profile.ProfileSHA256 {
		t.Fatalf("profile round trip: %v", err)
	}
	if !strings.Contains(string(data), "collector.invalid") || strings.Contains(string(data), "@example.invalid") {
		t.Fatal("private profile fields changed")
	}
	path := filepath.Join(t.TempDir(), "profiles", "site.json")
	if err := SaveSiteProtectionProfile(path, profile); err != nil {
		t.Fatal(err)
	}
	if err := SaveSiteProtectionProfile(path, profile); err == nil {
		t.Fatal("profile overwrote an existing artifact")
	}
	if _, err := BuildSiteProtectionProfile("missing", trial, TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("accepted missing baseline")
	}
	if _, err := BuildSiteProtectionProfile(base, "missing", TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("accepted missing trial")
	}
	if _, err := BuildSiteProtectionProfile(base, base, TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("accepted wrong trial role")
	}
	_, approximate := profileFixture(t, "approximate")
	// Matching test fixtures use the same bounded pair identity, so the supported
	// control check is the boundary being exercised here.
	if _, err := BuildSiteProtectionProfile(base, approximate, TaskConfirmation{"works", "works"}); err == nil || !strings.Contains(err.Error(), "lab-only") {
		t.Fatalf("approximate location became persistent: %v", err)
	}
}

func TestSiteProfileRejectsTamperingMalformedAndInventedClaims(t *testing.T) {
	base, trial := profileFixture(t, "deny")
	original, err := BuildSiteProtectionProfile(base, trial, TaskConfirmation{"works", "works"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SiteProtectionProfile){
		func(p *SiteProtectionProfile) { p.SchemaVersion = 2 }, func(p *SiteProtectionProfile) { p.Kind = "portable" }, func(p *SiteProtectionProfile) { p.SiteOrigin = "https://site.invalid/path" }, func(p *SiteProtectionProfile) { p.Controls.Location = "approximate" }, func(p *SiteProtectionProfile) { p.Controls.BlockOrigins = nil }, func(p *SiteProtectionProfile) { p.Controls.BlockOrigins = []string{"https://bad.invalid/path"} }, func(p *SiteProtectionProfile) {
			p.Controls.BlockOrigins = []string{"https://b.invalid", "https://a.invalid"}
		}, func(p *SiteProtectionProfile) { p.Test.Order = "unknown" }, func(p *SiteProtectionProfile) { p.Test.ComparisonSHA256 = "bad" }, func(p *SiteProtectionProfile) { p.Test.AutomaticFunctionality = minimize.CandidateSufficient }, func(p *SiteProtectionProfile) { p.Test.FunctionalityState = evidence.Observed }, func(p *SiteProtectionProfile) { p.Test.Baseline.Requests = -1 }, func(p *SiteProtectionProfile) { p.Test.Treatment.Blocked = 2000 }, func(p *SiteProtectionProfile) { p.Test.Treatment.WebSocketFrames = 2049 }, func(p *SiteProtectionProfile) { p.Test.EvidenceState = evidence.Unknown }, func(p *SiteProtectionProfile) { p.Test.Reduction = "not-reduced" },
	} {
		profile := original
		mutate(&profile)
		profile.ProfileSHA256 = profile.identity()
		if profile.Validate() == nil {
			t.Fatal("accepted invalid profile contract")
		}
	}
	profile := original
	profile.Controls.Location = "unchanged"
	if profile.Validate() == nil {
		t.Fatal("accepted changed controls with stale identity")
	}
	profile.ProfileSHA256 = "changed"
	if _, err := profile.PrivateJSON(); err == nil {
		t.Fatal("serialized invalid profile")
	}
	if err := SaveSiteProtectionProfile(filepath.Join(t.TempDir(), "invalid.json"), profile); err == nil {
		t.Fatal("saved invalid profile")
	}
	for _, data := range [][]byte{nil, []byte(`{}`), []byte(`{"schema_version":1,"schema_version":1}`), []byte(`{"unexpected":1}`), []byte(strings.Repeat(" ", 64<<10+1))} {
		if _, err := DecodeSiteProtectionProfile(data); err == nil {
			t.Fatal("accepted malformed profile")
		}
	}
	data, _ := original.PrivateJSON()
	if _, err := DecodeSiteProtectionProfile(append(data, []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing profile data")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveSiteProtectionProfile(filepath.Join(file, "profile.json"), original); err == nil {
		t.Fatal("wrote through non-directory")
	}
}

func TestSiteProfilePreservesUnknownEvidenceAndRejectsUnboundSources(t *testing.T) {
	base, trial := profileFixture(t, "deny")
	profile, err := BuildSiteProtectionProfile(base, trial, TaskConfirmation{"unknown", "works"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Test.EvidenceState = evidence.Unknown
	profile.Test.Reduction = "unknown"
	profile.ProfileSHA256 = profile.identity()
	if err := profile.Validate(); err != nil {
		t.Fatal(err)
	}
	profile.Test.EvidenceState = evidence.Claimed
	profile.ProfileSHA256 = profile.identity()
	if profile.Validate() == nil {
		t.Fatal("accepted a claimed reduction as observation")
	}
	bundle, _, err := ReadInvestigation(base)
	if err != nil {
		t.Fatal(err)
	}
	export := filepath.Join(t.TempDir(), "export.json")
	data, _ := bundle.PortableJSON()
	if err := os.WriteFile(export, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSiteProtectionProfile(export, trial, TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("portable source invented private binding")
	}
	_, tc, err := readInvestigation(trial)
	if err != nil {
		t.Fatal(err)
	}
	trialBundle, _, err := ReadInvestigation(trial)
	if err != nil {
		t.Fatal(err)
	}
	tc.Trial.Location = "unchanged"
	tc.Trial.BlockOrigins = []string{}
	uncontrolled := filepath.Join(t.TempDir(), "uncontrolled")
	if _, err := SaveInvestigation(uncontrolled, CaptureResult{Journey: trialBundle.Journey, Destinations: tc.Destinations, Steps: tc.Steps, Trial: tc.Trial}); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSiteProtectionProfile(base, uncontrolled, TaskConfirmation{"works", "works"}); err == nil {
		t.Fatal("profile without tested controls")
	}
	profile, err = BuildSiteProtectionProfile(base, trial, TaskConfirmation{"works", "works"})
	if err != nil {
		t.Fatal(err)
	}
	profile.Controls.Location = "unchanged"
	profile.Controls.BlockOrigins = []string{}
	profile.ProfileSHA256 = profile.identity()
	if profile.Validate() == nil {
		t.Fatal("accepted empty protection")
	}
}
