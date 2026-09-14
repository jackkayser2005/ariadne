package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/proxy"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

const testManifest = `{
	"schema_version": 1,
	"name": "validation-test",
	"variable": "location",
	"baseline": {"location": "exact", "region": "us-east"},
	"treatment": {"location": "city", "region": "us-east"}
}`

func writeAndroidAcceptanceValidationRecord(t *testing.T) (string, bundle.AndroidAcceptanceRecord) {
	t.Helper()
	contract := strings.Repeat("c", 64)
	provenance, err := adb.ReplicationProvenanceSHA256(contract)
	if err != nil {
		t.Fatal(err)
	}
	evidenceSHA256 := strings.Repeat("d", 64)
	record := bundle.AndroidAcceptanceRecord{
		SchemaVersion:                  1,
		Workflow:                       "experiment-001-emulator",
		ManifestName:                   "experiment-001-email",
		DeclaredVariable:               "email",
		ManifestContractSHA256:         contract,
		Package:                        "dev.ariadne.fixture",
		AndroidAPI:                     35,
		Architecture:                   "x86_64",
		PackageVersionCode:             1,
		PackageSHA256:                  strings.Repeat("a", 64),
		AriadneRevision:                strings.Repeat("b", 40),
		RunEvidenceSHA256:              evidenceSHA256,
		RunReportSHA256:                strings.Repeat("e", 64),
		RunDifferences:                 1,
		RunUnknowns:                    0,
		ReplicationReceiptSHA256:       strings.Repeat("f", 64),
		ReplicationProvenanceSHA256:    provenance,
		ReplicationBindingSHA256:       strings.Repeat("8", 64),
		Outcome:                        bundle.ReplicatedChange,
		EvidenceState:                  evidence.Observed,
		PairsPerOrder:                  1,
		CompletedPairs:                 2,
		ChangedPairs:                   2,
		NoChangePairs:                  0,
		UnknownPairs:                   0,
		ExportSourceEvidenceSHA256:     evidenceSHA256,
		ExportSHA256:                   strings.Repeat("9", 64),
		ReflectionSHA256:               strings.Repeat("7", 64),
		ReflectionSourceEvidenceSHA256: evidenceSHA256,
		QuestionID:                     "counterfactual-change",
		QuestionState:                  evidence.Observed,
		ReviewMethod:                   "GET",
		ReviewPath:                     "/",
		ReviewStatus:                   "self-attested",
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "experiment-001-acceptance.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, record
}

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

func writeArchiveQuestionValidationReport(t *testing.T) (string, bundle.ArchiveQuestionVerificationSummary) {
	t.Helper()
	report := bundle.ArchiveQuestionReport{
		SchemaVersion: 2,
		QuestionID:    "counterfactual-change",
		Question:      "Did changing the declared variable influence an observed output?",
		Summary:       bundle.ArchiveQuestionSummary{},
		Results:       []bundle.ArchiveQuestionResult{},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "reflection.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := bundle.VerifyArchiveQuestionReport(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, summary
}

func TestValidateArchiveQuestion(t *testing.T) {
	path, summary := writeArchiveQuestionValidationReport(t)
	report := Validate(path)
	if report.ArtifactKind != KindArchiveQuestion ||
		report.Overall != StatusWarning ||
		report.Identity != summary.ReflectionSHA256 ||
		report.EvidenceState != evidence.Unknown ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierStructural) != StatusPass ||
		tierStatus(report, TierIntegrity) != StatusPass ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusUnavailable {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateRejectsMalformedArchiveQuestion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive-question.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindArchiveQuestion)
}

func writeArchiveQuestionTransitionHistoryValidationRecord(t *testing.T) (string, bundle.ArchiveQuestionTransitionVerificationSummary) {
	t.Helper()
	history := bundle.ArchiveQuestionTransitionHistory{
		SchemaVersion:   2,
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		QuestionID:      "counterfactual-change",
		Question:        "Did changing the declared variable influence an observed output?",
		OrderBasis:      "caller",
		Snapshots:       2,
		Transitions: []bundle.ArchiveQuestionTransition{{
			FromReflectionSHA256: strings.Repeat("a", 64),
			ToReflectionSHA256:   strings.Repeat("b", 64),
			Result:               "changed",
			Compared:             1,
			Changed:              1,
			StateChanges: []bundle.ArchiveQuestionStateChange{{
				Directory:  "run-001",
				OlderState: "observed",
				NewerState: "unknown",
			}},
		}},
	}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "archive-question-transitions.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := bundle.VerifyArchiveQuestionTransitionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, summary
}

func TestValidateArchiveQuestionTransitionHistory(t *testing.T) {
	path, summary := writeArchiveQuestionTransitionHistoryValidationRecord(t)
	report := Validate(path)
	if report.ArtifactKind != KindArchiveQuestionTransitionHistory ||
		report.Overall != StatusWarning ||
		report.Identity != summary.TransitionHistorySHA256 ||
		report.EvidenceState != evidence.Unknown ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierStructural) != StatusPass ||
		tierStatus(report, TierIntegrity) != StatusPass ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusUnavailable {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateRejectsMalformedArchiveQuestionTransitionHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transition-history.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindArchiveQuestionTransitionHistory)
}

func TestValidateAndroidAcceptance(t *testing.T) {
	path, record := writeAndroidAcceptanceValidationRecord(t)
	summary, err := bundle.VerifyAndroidAcceptanceRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(path)
	if report.ArtifactKind != KindAndroidAcceptance ||
		report.Overall != StatusWarning ||
		report.Identity != summary.AcceptanceSHA256 ||
		report.Outcome != string(record.Outcome) ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonValidationIncomplete ||
		tierStatus(report, TierStructural) != StatusPass ||
		tierStatus(report, TierIntegrity) != StatusPass ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusUnavailable {
		t.Fatalf("report = %#v", report)
	}
}

func TestReportFromAndroidAcceptanceWithoutBinding(t *testing.T) {
	report := reportFromAndroidAcceptance(bundle.AndroidAcceptanceVerificationSummary{
		AcceptanceSHA256: strings.Repeat("a", 64),
		Outcome:          bundle.ReplicatedChange,
		EvidenceState:    evidence.Observed,
	})
	if report.Overall != StatusWarning ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusUnavailable ||
		report.Reason != ReasonProvenanceUnavailable {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateRejectsMalformedAndroidAcceptance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acceptance.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindAndroidAcceptance)
}

func TestValidateHAR(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.har")
	data := []byte("{\"log\":{\"version\":\"1.2\",\"entries\":[{\"request\":{\"url\":\"https://example.test/?email=secret\"}}]}}")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := browser.VerifyHAR(path)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(path)
	if report.ArtifactKind != KindBrowserHAR || report.Overall != StatusWarning ||
		report.Identity != summary.HARFileSHA256 || report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierBoundary) != StatusUnavailable || tierStatus(report, TierReplay) != StatusUnavailable {
		t.Fatalf("report = %#v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "example.test") {
		t.Fatalf("report exposed HAR content: %s", encoded)
	}
}

func TestValidateStandaloneTrace(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		completeness string
		overall      Status
		evidence     evidence.State
		replay       Status
	}{
		{name: "complete", file: "captured.json", completeness: trace.Complete, overall: StatusWarning, evidence: evidence.Observed, replay: StatusPass},
		{name: "partial", file: "proxy-trace.json", completeness: trace.Partial, overall: StatusUnknown, evidence: evidence.Unknown, replay: StatusUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.file)
			document := trace.Document{
				SchemaVersion: 1,
				Redacted:      true,
				Scope:         "outbound",
				Completeness:  test.completeness,
				Events: []trace.Event{{
					Source:      "proxy",
					Channel:     "network",
					Kind:        "request",
					Destination: "analytics",
					Fields:      []string{"region"},
				}},
			}
			data, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			summary, err := trace.Verify(path)
			if err != nil {
				t.Fatal(err)
			}
			report := Validate(path)
			if report.ArtifactKind != KindTrace || report.Overall != test.overall ||
				report.Identity != summary.TraceSHA256 || report.EvidenceState != test.evidence ||
				report.Reason == "" || tierStatus(report, TierReplay) != test.replay ||
				tierStatus(report, TierBoundary) != StatusUnavailable {
				t.Fatalf("report = %#v", report)
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "analytics") || strings.Contains(string(encoded), "region") {
				t.Fatalf("trace report exposed event labels: %s", encoded)
			}
		})
	}
}
func TestValidateProxyReplication(t *testing.T) {
	root := writeValidationProxyReplication(t)
	summary, err := proxy.VerifyReplicated(root)
	if err != nil {
		t.Fatal(err)
	}

	report := Validate(root)
	if report.ArtifactKind != KindProxyReplication ||
		report.Overall != StatusUnknown ||
		report.Identity != summary.ReceiptSHA256 ||
		report.Outcome != string(trace.NoChangeObserved) ||
		report.EvidenceState != evidence.Unknown ||
		report.Reason != ReasonIncompleteCapture ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusUnknown {
		t.Fatalf("report = %#v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) {
		t.Fatalf("proxy report exposed root path: %s", encoded)
	}
}

func TestValidateBrowserReplication(t *testing.T) {
	root := writeValidationBrowserReplication(t)
	summary, err := browser.VerifyFixtureReplicated(root)
	if err != nil {
		t.Fatal(err)
	}

	report := Validate(root)
	if report.ArtifactKind != KindBrowserReplication ||
		report.Overall != StatusPass ||
		report.Identity != summary.ReceiptSHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonVerified ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("report = %#v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) {
		t.Fatalf("browser report exposed root path: %s", encoded)
	}
}

func TestValidateRejectsMalformedHAR(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.har")
	if err := os.WriteFile(path, []byte("{\"log\":{\"version\":\"1.1\",\"entries\":[]}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(path), KindBrowserHAR)
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

	t.Run("malformed proxy replication", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "proxy-replication")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "replication.json"), []byte("{\"adapter\":\"proxy-connect\"}"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(root), KindProxyReplication)
	})

	t.Run("malformed browser replication", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "browser-replication")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "replication.json"), []byte("{\"adapter\":\"browser-local-fixture\"}"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(root), KindBrowserReplication)
	})

	t.Run("malformed browser minimization", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "browser-minimization")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "minimization.json"), []byte("{\"adapter\":\"browser-local-fixture\"}"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(root), KindBrowserMinimization)
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

	t.Run("malformed weather", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "weather")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "weather.json"), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		report := Validate(root)
		assertRejected(t, report, KindBrowserWeather)
	})

	t.Run("malformed standalone trace", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(path), KindTrace)
	})
	t.Run("malformed trace archive", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace-archive.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(path), KindTraceArchive)
	})

	t.Run("malformed trace replication", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace-replication.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(path), KindTraceReplication)
	})

	t.Run("malformed trace case", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace-case.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(path), KindTraceCase)
	})

	t.Run("malformed trace study", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "trace-study.json")
		if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertRejected(t, Validate(path), KindTraceStudy)
	})
	t.Run("nonregular replication marker", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "replication.json"), 0o700); err != nil {
			t.Fatal(err)
		}
		report := Validate(root)
		assertRejected(t, report, KindAndroidReplication)
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

	t.Run("weather and replication are ambiguous", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"weather.json", "replication.json"} {
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

func TestValidateOversizedManifestUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := []byte(strings.Repeat("x", experiment.MaxManifestBytes+1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	report := Validate(path)
	if report.ArtifactKind != KindManifest ||
		report.Overall != StatusUnavailable ||
		report.Reason != ReasonArtifactUnavailable {
		t.Fatalf("report = %#v", report)
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

func TestValidateTraceArchive(t *testing.T) {
	root := t.TempDir()
	tracePath := filepath.Join(root, "trace.json")
	sessionPath := filepath.Join(root, "session.json")
	archivePath := filepath.Join(root, "trace-archive.json")
	document := trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source:      "browser",
			Channel:     "network",
			Kind:        "request",
			Destination: "analytics",
			Fields:      []string{"region"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveSession(tracePath, sessionPath, trace.SessionInput{
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("a", 64),
		Role:            trace.RoleStandalone,
		Order:           trace.OrderStandalone,
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := trace.SaveArchive([]trace.ArchiveInput{{TracePath: tracePath, SessionPath: sessionPath}}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(archivePath)
	if report.ArtifactKind != KindTraceArchive ||
		report.Overall != StatusWarning ||
		report.Identity != saved.ArchiveSHA256 ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("report = %#v", report)
	}
}

func TestReportFromTraceArchive(t *testing.T) {
	summary := trace.ArchiveVerificationSummary{
		SchemaVersion: 2,
		OrderBasis:    "caller",
		Entries:       2,
		Complete:      2,
		ArchiveSHA256: strings.Repeat("b", 64),
	}
	report := reportFromTraceArchive(summary)
	if report.ArtifactKind != KindTraceArchive ||
		report.Overall != StatusPass ||
		report.Identity != summary.ArchiveSHA256 ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.SchemaVersion = 1
	report = reportFromTraceArchive(summary)
	if report.Overall != StatusWarning || tierStatus(report, TierBoundary) != StatusUnavailable {
		t.Fatalf("legacy report = %#v", report)
	}

	summary.Partial = 1
	summary.Complete = 1
	report = reportFromTraceArchive(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("partial report = %#v", report)
	}
}

func TestValidateTraceReplication(t *testing.T) {
	root := t.TempDir()
	first := writeValidationReplicationPair(t, root, "first", trace.OrderBaselineTreatment)
	second := writeValidationReplicationPair(t, root, "second", trace.OrderTreatmentBaseline)
	ledgerPath := filepath.Join(root, "trace-replication.json")
	saved, err := trace.SaveReplicationLedger([]trace.ReplicationPairInput{first, second}, ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(ledgerPath)
	if report.ArtifactKind != KindTraceReplication ||
		report.Overall != StatusWarning ||
		report.Identity != saved.LedgerSHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateTraceCase(t *testing.T) {
	root := t.TempDir()
	tracePath := filepath.Join(root, "trace.json")
	sessionPath := filepath.Join(root, "session.json")
	archivePath := filepath.Join(root, "archive.json")
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	casePath := filepath.Join(root, "trace-case.json")
	document := trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source:      "browser",
			Channel:     "network",
			Kind:        "request",
			Destination: "analytics",
			Fields:      []string{"region"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveSession(tracePath, sessionPath, trace.SessionInput{
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("a", 64),
		Role:            trace.RoleStandalone,
		Order:           trace.OrderStandalone,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveArchive([]trace.ArchiveInput{{TracePath: tracePath, SessionPath: sessionPath}, {TracePath: tracePath, SessionPath: sessionPath}}, archivePath); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}
	saved, err := trace.SaveCase([]trace.CaseInput{{
		Kind:              trace.CaseEntryTraceArchive,
		ArtifactPath:      archivePath,
		QuestionRoundPath: archiveRoundPath,
	}}, casePath)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(casePath)
	if report.ArtifactKind != KindTraceCase ||
		report.Overall != StatusWarning ||
		report.Identity != saved.CaseSHA256 ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("report = %#v", report)
	}
}

func TestReportFromTraceCase(t *testing.T) {
	summary := trace.CaseVerificationSummary{
		SchemaVersion: 1,
		OrderBasis:    "caller",
		Entries:       2,
		Archives:      1,
		Replications:  1,
		CaseSHA256:    strings.Repeat("d", 64),
	}
	report := reportFromTraceCase(summary)
	if report.ArtifactKind != KindTraceCase ||
		report.Overall != StatusWarning ||
		report.Identity != summary.CaseSHA256 ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.UnknownEntries = 1
	report = reportFromTraceCase(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("incomplete report = %#v", report)
	}
}

func TestReportFromTraceReplication(t *testing.T) {
	summary := trace.ReplicationLedgerVerificationSummary{
		SchemaVersion:          1,
		Pairs:                  2,
		BaselineTreatmentPairs: 1,
		TreatmentBaselinePairs: 1,
		ResetConfirmedPairs:    2,
		CompletePairs:          2,
		ChangedPairs:           2,
		OrderBalanced:          true,
		Outcome:                trace.ReplicatedChange,
		EvidenceState:          evidence.Observed,
		LedgerSHA256:           strings.Repeat("c", 64),
	}
	report := reportFromTraceReplication(summary)
	if report.ArtifactKind != KindTraceReplication ||
		report.Overall != StatusWarning ||
		report.Identity != summary.LedgerSHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.UnknownPairs = 1
	summary.CompletePairs = 1
	summary.ResetConfirmedPairs = 1
	summary.Outcome = trace.ReplicationUnknown
	summary.EvidenceState = evidence.Unknown
	report = reportFromTraceReplication(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("incomplete report = %#v", report)
	}
}

func TestValidateTraceStudy(t *testing.T) {
	root := t.TempDir()
	firstBaseline := writeValidationReplicationPair(t, root, "first-a", trace.OrderBaselineTreatment)
	firstTreatment := writeValidationReplicationPair(t, root, "first-b", trace.OrderTreatmentBaseline)
	secondBaseline := writeValidationReplicationPair(t, root, "second-a", trace.OrderBaselineTreatment)
	secondTreatment := writeValidationReplicationPair(t, root, "second-b", trace.OrderTreatmentBaseline)
	firstLedgerPath := filepath.Join(root, "first-ledger.json")
	secondLedgerPath := filepath.Join(root, "second-ledger.json")
	firstRoundPath := filepath.Join(root, "first-round.json")
	secondRoundPath := filepath.Join(root, "second-round.json")
	if _, err := trace.SaveReplicationLedger([]trace.ReplicationPairInput{firstBaseline, firstTreatment}, firstLedgerPath); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveReplicationLedger([]trace.ReplicationPairInput{secondBaseline, secondTreatment}, secondLedgerPath); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveReplicationQuestionRound(firstLedgerPath, firstRoundPath); err != nil {
		t.Fatal(err)
	}
	if _, err := trace.SaveReplicationQuestionRound(secondLedgerPath, secondRoundPath); err != nil {
		t.Fatal(err)
	}
	studyPath := filepath.Join(root, "trace-study.json")
	saved, err := trace.SaveReplicationStudy(strings.Repeat("e", 64), []trace.StudyInput{
		{LedgerPath: firstLedgerPath, RoundPath: firstRoundPath},
		{LedgerPath: secondLedgerPath, RoundPath: secondRoundPath},
	}, studyPath)
	if err != nil {
		t.Fatal(err)
	}
	report := Validate(studyPath)
	if report.ArtifactKind != KindTraceStudy ||
		report.Overall != StatusWarning ||
		report.Identity != saved.StudySHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("report = %#v", report)
	}
}

func TestReportFromTraceStudy(t *testing.T) {
	summary := trace.StudyVerificationSummary{
		Runs:          2,
		Pairs:         4,
		SupportedRuns: 2,
		BalancedRuns:  2,
		CompletePairs: 4,
		Outcome:       trace.ReplicatedChange,
		EvidenceState: evidence.Observed,
		StudySHA256:   strings.Repeat("f", 64),
	}
	report := reportFromTraceStudy(summary)
	if report.ArtifactKind != KindTraceStudy ||
		report.Overall != StatusWarning ||
		report.Identity != summary.StudySHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.UnknownRuns = 1
	summary.SupportedRuns = 1
	summary.EvidenceState = evidence.Unknown
	summary.Outcome = trace.ReplicationUnknown
	report = reportFromTraceStudy(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("incomplete report = %#v", report)
	}
}
func writeValidationBrowserReplication(t testing.TB) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "browser-replicated")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	record := browser.ReplicatedRunRecord{
		SchemaVersion:   browser.BrowserReplicationSchemaVersion,
		Adapter:         browser.BrowserReplicationAdapter,
		AdapterVersion:  browser.BrowserReplicationAdapterVersion,
		ProcedureSHA256: strings.Repeat("c", 64),
		Scope:           "outbound",
		PairsPerOrder:   1,
		ResetPolicy:     browser.BrowserReplicationResetPolicy,
		Status:          browser.BrowserReplicationStatusComplete,
		Pairs:           make([]browser.ReplicatedPairRecord, 0, 2),
	}
	for _, order := range []string{trace.OrderBaselineTreatment, trace.OrderTreatmentBaseline} {
		directory := "pair-001-" + order
		pairRoot := filepath.Join(root, directory)
		if err := os.MkdirAll(filepath.Join(pairRoot, "baseline"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(pairRoot, "treatment"), 0o700); err != nil {
			t.Fatal(err)
		}
		makeDocument := func(fields []string) trace.Document {
			return trace.Document{
				SchemaVersion: 1,
				Redacted:      true,
				Scope:         "outbound",
				Completeness:  trace.Complete,
				Events: []trace.Event{{
					Source: "browser", Channel: "network", Kind: "request",
					Destination: "first-party", Fields: fields,
				}},
			}
		}
		baselineData, err := json.Marshal(makeDocument([]string{"region"}))
		if err != nil {
			t.Fatal(err)
		}
		treatmentData, err := json.Marshal(makeDocument([]string{"region", "location"}))
		if err != nil {
			t.Fatal(err)
		}
		baselineTrace := filepath.Join(pairRoot, "baseline", "trace.json")
		treatmentTrace := filepath.Join(pairRoot, "treatment", "trace.json")
		if err := os.WriteFile(baselineTrace, baselineData, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(treatmentTrace, treatmentData, 0o600); err != nil {
			t.Fatal(err)
		}
		baselineSession := filepath.Join(pairRoot, "baseline", "session.json")
		treatmentSession := filepath.Join(pairRoot, "treatment", "session.json")
		if _, err := trace.SaveSessionPair(
			baselineTrace,
			treatmentTrace,
			baselineSession,
			treatmentSession,
			trace.SessionPairInput{
				Adapter:         browser.BrowserReplicationAdapter,
				AdapterVersion:  browser.BrowserReplicationAdapterVersion,
				ProcedureSHA256: strings.Repeat("c", 64),
				Scope:           "outbound",
				Order:           order,
			},
		); err != nil {
			t.Fatal(err)
		}
		firstSession, secondSession := trace.RoleBaseline, trace.RoleTreatment
		if order == trace.OrderTreatmentBaseline {
			firstSession, secondSession = secondSession, firstSession
		}
		record.Pairs = append(record.Pairs, browser.ReplicatedPairRecord{
			Pair:          1,
			Order:         order,
			Directory:     directory,
			FirstSession:  firstSession,
			SecondSession: secondSession,
			Status:        browser.BrowserReplicationStatusComplete,
		})
	}
	record.CompletedPairs = len(record.Pairs)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, "replication.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeValidationProxyReplication(t testing.TB) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proxy-replicated")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	record := proxy.ReplicatedRunRecord{
		SchemaVersion:              proxy.ProxyReplicationSchemaVersion,
		Adapter:                    proxy.Adapter,
		AdapterVersion:             proxy.AdapterVersion,
		Scope:                      "outbound",
		PairsPerOrder:              1,
		ResetPolicy:                proxy.ProxyReplicationResetPolicy,
		ControlledArgumentPosition: "final",
		ControlledArgumentCount:    1,
		ConditionValuesWithheld:    true,
		ExecutionIdentitySHA256:    strings.Repeat("b", 64),
		Status:                     proxy.ProxyReplicationStatusComplete,
		Pairs:                      make([]proxy.ReplicatedPairRecord, 0, 2),
	}
	for _, order := range []string{trace.OrderBaselineTreatment, trace.OrderTreatmentBaseline} {
		directory := "pair-001-" + order
		pairRoot := filepath.Join(root, directory)
		if err := os.MkdirAll(filepath.Join(pairRoot, "baseline"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(pairRoot, "treatment"), 0o700); err != nil {
			t.Fatal(err)
		}
		document := trace.Document{
			SchemaVersion: 1,
			Redacted:      true,
			Scope:         "outbound",
			Completeness:  trace.Partial,
			Events: []trace.Event{{
				Source: "proxy", Channel: "network", Kind: "request",
				Destination: "first-party", Fields: []string{"region"},
			}},
		}
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		baselineTrace := filepath.Join(pairRoot, "baseline", "trace.json")
		treatmentTrace := filepath.Join(pairRoot, "treatment", "trace.json")
		if err := os.WriteFile(baselineTrace, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(treatmentTrace, data, 0o600); err != nil {
			t.Fatal(err)
		}
		baselineSession := filepath.Join(pairRoot, "baseline", "session.json")
		treatmentSession := filepath.Join(pairRoot, "treatment", "session.json")
		saved, err := trace.SaveSessionPair(
			baselineTrace,
			treatmentTrace,
			baselineSession,
			treatmentSession,
			trace.SessionPairInput{
				Adapter:         proxy.Adapter,
				AdapterVersion:  proxy.AdapterVersion,
				ProcedureSHA256: strings.Repeat("a", 64),
				Scope:           "outbound",
				Order:           order,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		firstSession, secondSession := trace.RoleBaseline, trace.RoleTreatment
		if order == trace.OrderTreatmentBaseline {
			firstSession, secondSession = secondSession, firstSession
		}
		record.Pairs = append(record.Pairs, proxy.ReplicatedPairRecord{
			Pair:                   1,
			Order:                  order,
			Directory:              directory,
			FirstSession:           firstSession,
			SecondSession:          secondSession,
			Status:                 proxy.ProxyReplicationStatusComplete,
			BaselineTraceSHA256:    saved.BaselineTraceSHA256,
			TreatmentTraceSHA256:   saved.TreatmentTraceSHA256,
			BaselineSessionSHA256:  saved.BaselineSessionSHA256,
			TreatmentSessionSHA256: saved.TreatmentSessionSHA256,
			PairSHA256:             saved.PairSHA256,
		})
	}
	record.CompletedPairs = len(record.Pairs)
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(root, "replication.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeValidationReplicationPair(t testing.TB, root, name, order string) trace.ReplicationPairInput {
	t.Helper()
	baselinePath := filepath.Join(root, name+"-baseline-trace.json")
	treatmentPath := filepath.Join(root, name+"-treatment-trace.json")
	baselineSessionPath := filepath.Join(root, name+"-baseline-session.json")
	treatmentSessionPath := filepath.Join(root, name+"-treatment-session.json")
	makeDocument := func(fields []string) trace.Document {
		return trace.Document{
			SchemaVersion: 1,
			Redacted:      true,
			Scope:         "outbound",
			Completeness:  trace.Complete,
			Events: []trace.Event{{
				Source:      "browser",
				Channel:     "network",
				Kind:        "request",
				Destination: "analytics",
				Fields:      fields,
			}},
		}
	}
	treatmentFields := []string{"region", "consent"}
	if strings.HasPrefix(name, "second") {
		treatmentFields = []string{"region", "location"}
	}
	for path, document := range map[string]trace.Document{
		baselinePath:  makeDocument([]string{"region"}),
		treatmentPath: makeDocument(treatmentFields),
	} {
		data, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := trace.SaveSessionPair(
		baselinePath,
		treatmentPath,
		baselineSessionPath,
		treatmentSessionPath,
		trace.SessionPairInput{
			Adapter:         "browser-redacted-audit",
			AdapterVersion:  1,
			ProcedureSHA256: strings.Repeat("a", 64),
			Scope:           "outbound",
			Order:           order,
		},
	); err != nil {
		t.Fatal(err)
	}
	return trace.ReplicationPairInput{
		BaselineTracePath:    baselinePath,
		TreatmentTracePath:   treatmentPath,
		BaselineSessionPath:  baselineSessionPath,
		TreatmentSessionPath: treatmentSessionPath,
		ResetConfirmed:       true,
	}
}

func TestReportFromWeather(t *testing.T) {
	review := browser.WeatherReview{
		ReceiptSHA256: strings.Repeat("a", 64),
		Ladder: minimize.LadderSummary{
			EvidenceState:  evidence.Unknown,
			SelectionState: minimize.SelectionUnknown,
		},
	}
	report := reportFromWeather(review)
	if report.ArtifactKind != KindBrowserWeather || report.Identity != strings.Repeat("a", 64) ||
		report.Overall != StatusUnknown || report.SelectionState != string(minimize.SelectionUnknown) ||
		tierStatus(report, TierBoundary) != StatusPass || tierStatus(report, TierReplay) != StatusUnknown {
		t.Fatalf("unknown weather report = %#v", report)
	}

	review.Ladder.EvidenceState = evidence.Observed
	review.Ladder.SelectionState = minimize.SelectionSelected
	review.Ladder.SelectedCandidate = "coarse"
	report = reportFromWeather(review)
	if report.Overall != StatusPass || report.SelectionState != string(minimize.SelectionSelected) ||
		report.SelectedCandidate != "coarse" || tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("selected weather report = %#v", report)
	}
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

func TestReportFromBrowserReplication(t *testing.T) {
	summary := browser.BrowserReplicationSummary{
		ReceiptSHA256:  strings.Repeat("a", 64),
		Pairs:          2,
		CompletedPairs: 2,
		Outcome:        trace.ReplicatedChange,
		EvidenceState:  evidence.Observed,
	}
	report := reportFromBrowserReplication(summary)
	if report.ArtifactKind != KindBrowserReplication ||
		report.Overall != StatusPass ||
		report.Identity != summary.ReceiptSHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.EvidenceState = evidence.Unknown
	report = reportFromBrowserReplication(summary)
	if report.Overall != StatusUnknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("partial report = %#v", report)
	}
}

func TestReportFromBrowserMinimization(t *testing.T) {
	summary := minimize.LadderSummary{
		SchemaVersion:          minimize.LadderSummarySchemaVersion,
		PlanName:               "browser-account-minimize",
		Variable:               "account-id",
		ReferenceCandidate:     browser.BrowserFixtureReferenceCandidate,
		FunctionalityCriterion: browser.BrowserFunctionalityCriterion,
		Adapter:                browser.BrowserReplicationAdapter,
		AdapterVersion:         browser.BrowserReplicationAdapterVersion,
		ProcedureSHA256:        strings.Repeat("a", 64),
		Scope:                  "outbound",
		ResetPolicy:            browser.BrowserReplicationResetPolicy,
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         minimize.SelectionSelected,
		SelectedCandidate:      browser.BrowserFixtureOmittedCandidate,
		CandidateResults: []minimize.LadderCandidateResult{{
			ID:             browser.BrowserFixtureOmittedCandidate,
			Directory:      "candidate-001-omitted",
			Classification: minimize.CandidateSufficient,
			Outcome:        trace.NoChangeObserved,
			EvidenceState:  evidence.Observed,
			ReceiptSHA256:  strings.Repeat("b", 64),
			Pairs:          2,
			PairsPerOrder:  1,
			CompletedPairs: 2,
			NoChangePairs:  2,
		}},
	}
	report := reportFromBrowserMinimization(summary, strings.Repeat("c", 64))
	if report.ArtifactKind != KindBrowserMinimization ||
		report.Overall != StatusPass ||
		report.SelectedCandidate != browser.BrowserFixtureOmittedCandidate ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.EvidenceState = evidence.Unknown
	summary.SelectionState = minimize.SelectionUnknown
	summary.SelectedCandidate = ""
	summary.CandidateResults[0].Classification = minimize.CandidateUnknown
	summary.CandidateResults[0].Outcome = trace.ReplicationUnknown
	summary.CandidateResults[0].EvidenceState = evidence.Unknown
	summary.CandidateResults[0].UnknownPairs = 1
	report = reportFromBrowserMinimization(summary, "incomplete")
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("incomplete report = %#v", report)
	}
}

func TestReportFromProxyReplication(t *testing.T) {
	summary := proxy.ReplicationSummary{
		ReceiptSHA256:  strings.Repeat("a", 64),
		Pairs:          2,
		CompletedPairs: 2,
		Outcome:        trace.ReplicatedChange,
		EvidenceState:  evidence.Observed,
	}
	report := reportFromProxyReplication(summary)
	if report.ArtifactKind != KindProxyReplication ||
		report.Overall != StatusPass ||
		report.Identity != summary.ReceiptSHA256 ||
		report.Outcome != string(trace.ReplicatedChange) ||
		report.EvidenceState != evidence.Observed ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusPass {
		t.Fatalf("complete report = %#v", report)
	}

	summary.EvidenceState = evidence.Unknown
	report = reportFromProxyReplication(summary)
	if report.Overall != StatusUnknown ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("partial report = %#v", report)
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

func TestValidateSourceAdapterRun(t *testing.T) {
	root := t.TempDir()
	tracePath := filepath.Join(root, "trace.json")
	sessionPath := filepath.Join(root, "session.json")
	document := trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "desktop", Channel: "network", Kind: "request",
			Destination: "analytics", Fields: []string{"region"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	procedure := trace.SourceAdapterProcedure{
		SchemaVersion: trace.SourceAdapterProcedureSchemaVersion,
		Adapter:       "external-desktop-v1", AdapterVersion: 1,
		Source: "desktop", Scope: "outbound", DurationMS: 5000, MaxEvents: 1,
	}
	procedureSHA256, err := trace.SourceAdapterProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	session, err := trace.SaveSession(tracePath, sessionPath, trace.SessionInput{
		Adapter: procedure.Adapter, AdapterVersion: procedure.AdapterVersion,
		Source: procedure.Source, ProcedureSHA256: procedureSHA256,
		Role: trace.RoleStandalone, Order: trace.OrderStandalone,
	})
	if err != nil {
		t.Fatal(err)
	}
	traceSummary, err := trace.Verify(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	receipt := trace.SourceAdapterReceipt{
		SchemaVersion: trace.SourceAdapterReceiptSchemaVersion,
		Adapter:       procedure.Adapter, AdapterVersion: procedure.AdapterVersion,
		Source: procedure.Source, Scope: procedure.Scope,
		Completeness: trace.Complete, Events: traceSummary.Events,
		ProcedureSHA256:  procedureSHA256,
		ExecutableSHA256: strings.Repeat("b", 64),
		ChallengeSHA256:  strings.Repeat("c", 64),
		TraceSHA256:      traceSummary.TraceSHA256,
		SessionSHA256:    session.SessionSHA256,
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), receiptData, 0o600); err != nil {
		t.Fatal(err)
	}
	receiptSHA256, err := trace.SourceAdapterReceiptSHA256(receipt)
	if err != nil {
		t.Fatal(err)
	}

	report := Validate(root)
	if report.ArtifactKind != KindSourceAdapterRun ||
		report.Overall != StatusWarning ||
		report.Identity != receiptSHA256 ||
		report.EvidenceState != evidence.Observed ||
		report.Reason != ReasonProvenanceUnavailable ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusUnavailable {
		t.Fatalf("report = %#v", report)
	}
}

func TestValidateRejectsMalformedSourceAdapterRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(root), KindSourceAdapterRun)

	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "receipt.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, Validate(root), KindSourceAdapterRun)

	root = t.TempDir()
	for _, name := range []string{"receipt.json", "weather.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	assertRejected(t, Validate(root), KindUnknown)
}

func TestReportFromSourceAdapter(t *testing.T) {
	summary := trace.SourceAdapterRunSummary{
		ReceiptSHA256: strings.Repeat("a", 64),
		Receipt: trace.SourceAdapterReceipt{
			ProvenanceSHA256: strings.Repeat("b", 64),
		},
		Trace:   trace.VerificationSummary{Completeness: trace.Complete},
		Session: trace.SessionVerificationSummary{Completeness: trace.Complete},
	}
	report := reportFromSourceAdapter(summary)
	if report.ArtifactKind != KindSourceAdapterRun ||
		report.Overall != StatusWarning ||
		report.EvidenceState != evidence.Observed ||
		report.Identity != summary.ReceiptSHA256 ||
		tierStatus(report, TierBoundary) != StatusPass ||
		tierStatus(report, TierReplay) != StatusUnavailable ||
		report.Reason != ReasonValidationIncomplete {
		t.Fatalf("complete report = %#v", report)
	}

	summary.Receipt.ProvenanceSHA256 = ""
	summary.Trace.Completeness = trace.Partial
	summary.Session.Completeness = trace.Partial
	report = reportFromSourceAdapter(summary)
	if report.Overall != StatusUnknown ||
		report.EvidenceState != evidence.Unknown ||
		tierStatus(report, TierBoundary) != StatusUnavailable ||
		tierStatus(report, TierReplay) != StatusUnknown ||
		report.Reason != ReasonIncompleteCapture {
		t.Fatalf("partial report = %#v", report)
	}
}
