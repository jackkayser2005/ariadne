package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReportFromAndroidRun(t *testing.T) {
	summary := bundle.Summary{
		EvidenceSHA256: strings.Repeat("a", 64),
		AnswerState:    evidence.Observed,
		Authenticated:  true,
	}
	report := reportFromAndroidRun(summary)
	if report.ArtifactKind != KindAndroidRun ||
		report.Overall != StatusPass ||
		report.Identity != summary.EvidenceSHA256 ||
		report.EvidenceState != evidence.Observed ||
		report.Outcome != "" ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("authenticated report = %#v", report)
	}

	summary.Authenticated = false
	report = reportFromAndroidRun(summary)
	if report.Overall != StatusWarning ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("legacy report = %#v", report)
	}

	summary.Unknowns = 1
	summary.AnswerState = evidence.Unknown
	report = reportFromAndroidRun(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown {
		t.Fatalf("incomplete report = %#v", report)
	}
}

func TestValidateRejectsMalformedStandaloneAndroidRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "evidence.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "report.md"), []byte("not a report"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"baseline", "treatment"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	report := Validate(root)
	assertRejected(t, report, KindAndroidRun)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) {
		t.Fatalf("report leaked input path: %s", encoded)
	}
}

func TestValidateRejectsAmbiguousStandaloneAndroidRun(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"evidence.json", "report.md", "replication.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"baseline", "treatment"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	report := Validate(root)
	assertRejected(t, report, KindUnknown)
}
