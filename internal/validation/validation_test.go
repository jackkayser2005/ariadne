package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/minimize"
)

const testManifest = `{
	"schema_version": 1,
	"name": "validation-test",
	"variable": "location",
	"baseline": {"location": "exact", "region": "us-east"},
	"treatment": {"location": "city", "region": "us-east"}
}`

func TestValidateManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(testManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := experiment.Decode(strings.NewReader(testManifest))
	if err != nil {
		t.Fatal(err)
	}

	report := Validate(path)
	if report.ArtifactKind != KindManifest ||
		report.Overall != StatusWarning ||
		report.Identity != manifest.ContractDigest() ||
		report.EvidenceState != evidence.Unknown ||
		report.Outcome != "" ||
		report.Reason != ReasonValidationIncomplete {
		t.Fatalf("report = %#v", report)
	}
	if got := tierStatus(report, TierStructural); got != StatusPass {
		t.Fatalf("structural status = %q", got)
	}
	if got := tierStatus(report, TierIntegrity); got != StatusPass {
		t.Fatalf("integrity status = %q", got)
	}
	if got := tierStatus(report, TierBoundary); got != StatusUnavailable {
		t.Fatalf("boundary status = %q", got)
	}
	if got := tierStatus(report, TierReplay); got != StatusUnavailable {
		t.Fatalf("replay status = %q", got)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "exact") || strings.Contains(string(data), "city") {
		t.Fatalf("report exposed persona values: %s", data)
	}
}

func TestValidateRejectsMalformedAndAmbiguousArtifacts(t *testing.T) {
	t.Run("malformed manifest", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "manifest.json")
		if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		report := Validate(path)
		assertRejected(t, report, KindManifest)
	})

	t.Run("malformed replication", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "replication")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "replication.json"), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		report := Validate(root)
		assertRejected(t, report, KindAndroidReplication)
	})

	t.Run("malformed minimization", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "minimization")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "minimization.json"), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		report := Validate(root)
		assertRejected(t, report, KindAndroidMinimization)
	})

	t.Run("ambiguous directory", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"replication.json", "minimization.json"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		report := Validate(root)
		assertRejected(t, report, KindUnknown)
	})
}

func TestValidateUnavailableArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		reason string
	}{
		{name: "empty", reason: ReasonArtifactUnavailable},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing"), reason: ReasonArtifactUnavailable},
		{name: "unsupported file", path: filepath.Join(t.TempDir(), "other.txt"), reason: ReasonUnsupportedArtifact},
		{name: "unsupported directory", path: t.TempDir(), reason: ReasonUnsupportedArtifact},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "unsupported file" {
				if err := os.WriteFile(test.path, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			report := Validate(test.path)
			if report.Overall != StatusUnavailable || report.Reason != test.reason || report.ArtifactKind != KindUnknown {
				t.Fatalf("report = %#v", report)
			}
		})
	}
}

func TestValidateRejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.WriteFile(target, []byte(testManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	assertRejected(t, Validate(link), KindUnknown)
}

func TestValidateRejectsAncestorSymlink(t *testing.T) {
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "manifest.json")
	if err := os.WriteFile(target, []byte(testManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(t.TempDir(), "linked-parent")
	if err := os.Symlink(targetDir, linkParent); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}

	report := Validate(filepath.Join(linkParent, "manifest.json"))
	assertRejected(t, report, KindManifest)
}
func TestReportFromReplication(t *testing.T) {
	base := bundle.ReplicatedExperimentSummary{
		ReceiptSHA256:    strings.Repeat("a", 64),
		ProvenanceSHA256: strings.Repeat("b", 64),
		Pairs:            2,
		CompletedPairs:   2,
		Outcome:          bundle.ReplicatedChange,
		EvidenceState:    evidence.Observed,
	}
	report := reportFromReplication(base)
	if report.Overall != StatusPass ||
		report.ArtifactKind != KindAndroidReplication ||
		report.Outcome != string(bundle.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonVerified {
		t.Fatalf("complete report = %#v", report)
	}
	if tierStatus(report, TierBoundary) != StatusPass || tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete tiers = %#v", report.Tiers)
	}

	legacy := base
	legacy.ProvenanceSHA256 = ""
	report = reportFromReplication(legacy)
	if report.Overall != StatusWarning || tierStatus(report, TierBoundary) != StatusUnavailable ||
		report.Reason != ReasonProvenanceUnavailable {
		t.Fatalf("legacy report = %#v", report)
	}

	incomplete := base
	incomplete.CompletedPairs = 1
	incomplete.UnknownPairs = 1
	incomplete.Outcome = bundle.ReplicationUnknown
	report = reportFromReplication(incomplete)
	if report.Overall != StatusUnknown || tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("incomplete report = %#v", report)
	}
}

func TestReportFromMinimization(t *testing.T) {
	provenance := strings.Repeat("c", 64)
	summary := minimize.MinimizationSummary{
		EvidenceState:     evidence.Observed,
		SelectionState:    minimize.SelectionSelected,
		SelectedCandidate: "city",
		CandidateResults: []minimize.CandidateResult{
			testCandidate("city", provenance, 2, 2, bundle.NoChangeObserved),
			testCandidate("omitted", strings.Repeat("e", 64), 2, 2, bundle.NoChangeObserved),
		},
	}
	report := reportFromMinimization(summary, strings.Repeat("d", 64))
	if report.Overall != StatusPass ||
		report.SelectionState != string(minimize.SelectionSelected) ||
		report.SelectedCandidate != "city" ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	legacy := summary
	legacy.CandidateResults = append([]minimize.CandidateResult(nil), summary.CandidateResults...)
	for index := range legacy.CandidateResults {
		legacy.CandidateResults[index].ProvenanceSHA256 = ""
	}
	report = reportFromMinimization(legacy, "legacy")
	if report.Overall != StatusWarning || tierStatus(report, TierBoundary) != StatusUnavailable {
		t.Fatalf("legacy report = %#v", report)
	}

	mixed := summary
	mixed.CandidateResults = append([]minimize.CandidateResult(nil), summary.CandidateResults...)
	mixed.CandidateResults[1].ProvenanceSHA256 = ""
	report = reportFromMinimization(mixed, "mixed")
	if report.Overall != StatusFail || tierStatus(report, TierBoundary) != StatusFail ||
		report.Reason != ReasonProvenanceInconsistent {
		t.Fatalf("mixed report = %#v", report)
	}

	incomplete := summary
	incomplete.CandidateResults = append([]minimize.CandidateResult(nil), summary.CandidateResults...)
	incomplete.CandidateResults[1].CompletedPairs = 1
	incomplete.CandidateResults[1].Outcome = bundle.ReplicationUnknown
	report = reportFromMinimization(incomplete, "incomplete")
	if report.Overall != StatusUnknown || tierStatus(report, TierReplay) != StatusUnknown {
		t.Fatalf("incomplete report = %#v", report)
	}

	empty := summary
	empty.CandidateResults = nil
	report = reportFromMinimization(empty, "empty")
	if report.Overall != StatusFail || report.Reason != ReasonArtifactRejected {
		t.Fatalf("empty report = %#v", report)
	}
}

func TestFinalizeStatusMapping(t *testing.T) {
	report := verifiedReport(KindManifest)
	setTier(&report, TierBoundary, StatusWarning, ReasonValidationIncomplete)
	if got := finalize(report); got.Overall != StatusWarning || got.Reason != ReasonValidationIncomplete {
		t.Fatalf("warning report = %#v", got)
	}

	report = Report{
		Tiers: []TierResult{
			{Tier: TierStructural, Status: StatusUnavailable, Reason: ReasonNotApplicable},
		},
	}
	if got := finalize(report); got.Overall != StatusUnavailable || got.Reason != ReasonNotChecked {
		t.Fatalf("unavailable report = %#v", got)
	}

	if got := tierReason(TierResult{}, ReasonVerified); got != ReasonVerified {
		t.Fatalf("fallback reason = %q", got)
	}
	setTier(&report, TierReplay, StatusPass, ReasonVerified)
	if tierStatus(report, TierReplay) != "" {
		t.Fatalf("unknown tier should not be added: %#v", report.Tiers)
	}
}

func testCandidate(id, provenance string, pairs, completed int, outcome bundle.ReplicatedOutcome) minimize.CandidateResult {
	return minimize.CandidateResult{
		ID:               id,
		ProvenanceSHA256: provenance,
		Pairs:            pairs,
		CompletedPairs:   completed,
		Outcome:          outcome,
		EvidenceState:    evidence.Observed,
	}
}

func tierStatus(report Report, wanted Tier) Status {
	for _, tier := range report.Tiers {
		if tier.Tier == wanted {
			return tier.Status
		}
	}
	return ""
}

func assertRejected(t *testing.T, report Report, kind ArtifactKind) {
	t.Helper()
	if report.ArtifactKind != kind || report.Overall != StatusFail || report.Reason != ReasonArtifactRejected {
		t.Fatalf("rejected report = %#v", report)
	}
	if tierStatus(report, TierStructural) != StatusFail || tierStatus(report, TierIntegrity) != StatusFail {
		t.Fatalf("rejected tiers = %#v", report.Tiers)
	}
}
