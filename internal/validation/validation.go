// Package validation exposes one safe, tiered entry point for the artifact
// verifiers that are stable enough to compose.
package validation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/proxy"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

// SchemaVersion is the validation report schema supported by this build.
const SchemaVersion = 1

// ArtifactKind identifies the artifact family selected by Validate.
type ArtifactKind string

const (
	// KindUnknown means the input could not be identified as a supported artifact.
	KindUnknown ArtifactKind = "unknown"
	// KindManifest identifies an experiment manifest.
	KindManifest ArtifactKind = "manifest"
	// KindAndroidReplication identifies an Android replication directory.
	KindAndroidReplication ArtifactKind = "android-replication"
	// KindAndroidMinimization identifies an Android minimization directory.
	KindAndroidMinimization ArtifactKind = "android-minimization"
	// KindBrowserMinimization identifies a verified browser fixture minimization directory.
	KindBrowserMinimization ArtifactKind = "browser-minimization"
	// KindBrowserWeather identifies a verified browser weather investigation.
	KindBrowserWeather ArtifactKind = "browser-weather"
	// KindBrowserReplication identifies a verified browser fixture replication directory.
	KindBrowserReplication ArtifactKind = "browser-replication"
	// KindBrowserHAR identifies a bounded HAR export inventory.
	KindBrowserHAR ArtifactKind = "browser-har"
	// KindTrace identifies a standalone redacted trace document.
	KindTrace ArtifactKind = "trace"
	// KindTraceArchive identifies a verified source-neutral trace archive.
	KindTraceArchive ArtifactKind = "trace-archive"
	// KindTraceReplication identifies a verified source-neutral replication ledger.
	KindTraceReplication ArtifactKind = "trace-replication"
	// KindTraceCase identifies a verified cross-source trace case.
	KindTraceCase ArtifactKind = "trace-case"
	// KindTraceStudy identifies a verified cross-run replication study.
	KindTraceStudy ArtifactKind = "trace-study"
	// KindSourceAdapterRun identifies a verified generic source-adapter run.
	KindSourceAdapterRun ArtifactKind = "source-adapter-run"
	// KindProxyReplication identifies a verified loopback proxy replication directory.
	KindProxyReplication ArtifactKind = "proxy-replication"
)

// Tier identifies one independent validation guarantee.
type Tier string

const (
	// TierStructural checks that the artifact has a valid schema and shape.
	TierStructural Tier = "structural"
	// TierIntegrity checks canonical content and child artifact identities.
	TierIntegrity Tier = "integrity"
	// TierBoundary checks provenance and source-boundary consistency.
	TierBoundary Tier = "boundary"
	// TierReplay checks whether recorded evidence is complete enough to replay
	// or reproduce the controlled comparison. It does not launch anything.
	TierReplay Tier = "replay"
)

// Status is the result of one tier or the aggregate report.
type Status string

const (
	// StatusPass means the corresponding guarantee was verified.
	StatusPass Status = "pass"
	// StatusFail means validation rejected the artifact or guarantee.
	StatusFail Status = "fail"
	// StatusWarning means the artifact is valid but one or more tiers are
	// unavailable.
	StatusWarning Status = "warning"
	// StatusUnknown means available evidence cannot establish the guarantee.
	StatusUnknown Status = "unknown"
	// StatusUnavailable means the guarantee or artifact was not available.
	StatusUnavailable Status = "unavailable"
)

// Stable reason codes avoid copying verifier errors, paths, or captured data
// into portable output.
const (
	ReasonVerified               = "verified"
	ReasonArtifactRejected       = "artifact-rejected"
	ReasonArtifactUnavailable    = "artifact-unavailable"
	ReasonUnsupportedArtifact    = "unsupported-artifact"
	ReasonProvenanceUnavailable  = "provenance-unavailable"
	ReasonProvenanceInconsistent = "provenance-inconsistent"
	ReasonIncompleteCapture      = "incomplete-capture"
	ReasonNotApplicable          = "not-applicable"
	ReasonNotChecked             = "not-checked"
	ReasonValidationIncomplete   = "validation-incomplete"
)

// TierResult records one tier without exposing source paths or values.
type TierResult struct {
	Tier   Tier   `json:"tier"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Report is the raw-value-free result of one validation operation.
type Report struct {
	SchemaVersion     int            `json:"schema_version"`
	ArtifactKind      ArtifactKind   `json:"artifact_kind"`
	Overall           Status         `json:"overall"`
	Identity          string         `json:"identity,omitempty"`
	Outcome           string         `json:"outcome,omitempty"`
	EvidenceState     evidence.State `json:"evidence_state"`
	SelectionState    string         `json:"selection_state,omitempty"`
	SelectedCandidate string         `json:"selected_candidate,omitempty"`
	Tiers             []TierResult   `json:"tiers"`
	Reason            string         `json:"reason,omitempty"`
}

// Validate identifies and validates one supported artifact. It never includes
// the supplied path, verifier error, persona, payload, or process argument in
// the returned report.
func Validate(path string) Report {
	if strings.TrimSpace(path) == "" {
		return unavailableReport(KindUnknown, ReasonArtifactUnavailable)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return unavailableReport(KindUnknown, ReasonArtifactUnavailable)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return rejectedReport(KindUnknown)
	}
	if info.IsDir() {
		return validateDirectory(path)
	}
	if !info.Mode().IsRegular() {
		return rejectedReport(KindUnknown)
	}
	if strings.EqualFold(filepath.Ext(path), ".har") {
		summary, err := browser.VerifyHAR(path)
		if err != nil {
			return rejectedReport(KindBrowserHAR)
		}
		return reportFromHAR(summary)
	}
	if filepath.Base(path) != "manifest.json" && !strings.EqualFold(filepath.Ext(path), ".json") {
		return unavailableReport(KindUnknown, ReasonUnsupportedArtifact)
	}
	if summary, err := trace.VerifyArchive(path); err == nil {
		return reportFromTraceArchive(summary)
	}
	if looksLikeTraceArchivePath(path) {
		return rejectedReport(KindTraceArchive)
	}
	if summary, err := trace.VerifyReplicationLedger(path); err == nil {
		return reportFromTraceReplication(summary)
	}
	if looksLikeTraceReplicationPath(path) {
		return rejectedReport(KindTraceReplication)
	}
	if summary, err := trace.VerifyCase(path); err == nil {
		return reportFromTraceCase(summary)
	}
	if looksLikeTraceCasePath(path) {
		return rejectedReport(KindTraceCase)
	}
	if summary, err := trace.VerifyReplicationStudy(path); err == nil {
		return reportFromTraceStudy(summary)
	}
	if looksLikeTraceStudyPath(path) {
		return rejectedReport(KindTraceStudy)
	}
	if summary, err := trace.Verify(path); err == nil {
		return reportFromTrace(summary)
	}
	if looksLikeTracePath(path) {
		return rejectedReport(KindTrace)
	}
	return validateManifest(path)
}

func looksLikeTraceArchivePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "archive.json" || strings.HasSuffix(base, "-archive.json")
}

func looksLikeTraceCasePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "case.json" || base == "trace-case.json" || base == "case-package.json" || strings.HasSuffix(base, "-case.json")
}

func looksLikeTraceStudyPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "study.json" || base == "trace-study.json" || base == "replication-study.json" || strings.HasSuffix(base, "-study.json")
}

func looksLikeTracePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "trace.json" || strings.HasSuffix(base, "-trace.json")
}

func looksLikeTraceReplicationPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "ledger.json" || base == "trace-replication.json" ||
		base == "replication-ledger.json" || strings.HasSuffix(base, "-replication.json") ||
		strings.HasSuffix(base, "-ledger.json")
}

func validateDirectory(path string) Report {
	weather, weatherErr := inspectMarker(path, "weather.json")
	replication, replicationErr := inspectMarker(path, "replication.json")
	minimization, minimizationErr := inspectMarker(path, "minimization.json")
	sourceAdapter, sourceAdapterErr := inspectMarker(path, "receipt.json")
	if weatherErr != nil || replicationErr != nil || minimizationErr != nil || sourceAdapterErr != nil {
		return unavailableReport(KindUnknown, ReasonArtifactUnavailable)
	}
	if sourceAdapter.present && (weather.present || replication.present || minimization.present) {
		return rejectedReport(KindUnknown)
	}
	if weather.present && replication.present {
		return rejectedReport(KindUnknown)
	}
	if sourceAdapter.present {
		if !sourceAdapter.regular {
			return rejectedReport(KindSourceAdapterRun)
		}
		summary, err := trace.VerifySourceAdapterRun(path)
		if err != nil {
			return rejectedReport(KindSourceAdapterRun)
		}
		return reportFromSourceAdapter(summary)
	}
	if weather.present {
		if !weather.regular {
			return rejectedReport(KindBrowserWeather)
		}
		review, err := browser.VerifyWeather(path)
		if err != nil {
			return rejectedReport(KindBrowserWeather)
		}
		return reportFromWeather(review)
	}
	if replication.present && minimization.present {
		return rejectedReport(KindUnknown)
	}
	if replication.present {
		if !replication.regular {
			return rejectedReport(KindAndroidReplication)
		}
		if proxy.LooksLikeReplication(path) {
			summary, err := proxy.VerifyReplicated(path)
			if err != nil {
				return rejectedReport(KindProxyReplication)
			}
			return reportFromProxyReplication(summary)
		}
		if browser.LooksLikeReplication(path) {
			summary, err := browser.VerifyFixtureReplicated(path)
			if err != nil {
				return rejectedReport(KindBrowserReplication)
			}
			return reportFromBrowserReplication(summary)
		}
		summary, err := bundle.VerifyReplicated(path)
		if err != nil {
			return rejectedReport(KindAndroidReplication)
		}
		return reportFromReplication(summary)
	}
	if minimization.present {
		if !minimization.regular {
			return rejectedReport(KindAndroidMinimization)
		}
		if minimize.LooksLikeLadder(path, browser.BrowserReplicationAdapter) {
			summary, identity, err := browser.VerifyFixtureMinimizationWithIdentity(path)
			if err != nil {
				return rejectedReport(KindBrowserMinimization)
			}
			return reportFromBrowserMinimization(summary, identity)
		}
		summary, identity, err := minimize.VerifyWithIdentity(path)
		if err != nil {
			return rejectedReport(KindAndroidMinimization)
		}
		return reportFromMinimization(summary, identity)
	}
	return unavailableReport(KindUnknown, ReasonUnsupportedArtifact)
}

type marker struct {
	present bool
	regular bool
}

func inspectMarker(root, name string) (marker, error) {
	info, err := os.Lstat(filepath.Join(root, name))
	if os.IsNotExist(err) {
		return marker{}, nil
	}
	if err != nil {
		return marker{}, err
	}
	return marker{
		present: true,
		regular: info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0,
	}, nil
}

func validateManifest(path string) Report {
	data, err := bundle.ReadBoundedFile(path, experiment.MaxManifestBytes)
	if err != nil {
		if bundle.IsPathSafetyError(err) {
			return rejectedReport(KindManifest)
		}
		return unavailableReport(KindManifest, ReasonArtifactUnavailable)
	}

	manifest, err := experiment.Decode(bytes.NewReader(data))
	if err != nil {
		return rejectedReport(KindManifest)
	}
	report := verifiedReport(KindManifest)
	report.Identity = manifest.ContractDigest()
	setTier(&report, TierBoundary, StatusUnavailable, ReasonNotApplicable)
	setTier(&report, TierReplay, StatusUnavailable, ReasonNotApplicable)
	return finalize(report)
}

func reportFromReplication(summary bundle.ReplicatedExperimentSummary) Report {
	report := verifiedReport(KindAndroidReplication)
	report.Identity = summary.ReceiptSHA256
	report.Outcome = string(summary.Outcome)
	report.EvidenceState = summary.EvidenceState

	if summary.ProvenanceSHA256 == "" {
		setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	} else {
		setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	}
	if summary.CompletedPairs < summary.Pairs ||
		summary.UnknownPairs > 0 ||
		summary.Outcome == bundle.ReplicationUnknown {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	} else {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	}
	return finalize(report)
}

func reportFromBrowserReplication(summary browser.BrowserReplicationSummary) Report {
	report := verifiedReport(KindBrowserReplication)
	report.Identity = summary.ReceiptSHA256
	report.Outcome = string(summary.Outcome)
	report.EvidenceState = summary.EvidenceState
	setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	if summary.CompletedPairs == summary.Pairs &&
		summary.UnknownPairs == 0 &&
		summary.EvidenceState == evidence.Observed &&
		summary.Outcome != trace.ReplicationUnknown {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromProxyReplication(summary proxy.ReplicationSummary) Report {
	report := verifiedReport(KindProxyReplication)
	report.Identity = summary.ReceiptSHA256
	report.Outcome = string(summary.Outcome)
	report.EvidenceState = summary.EvidenceState
	setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	if summary.CompletedPairs == summary.Pairs &&
		summary.UnknownPairs == 0 &&
		summary.EvidenceState == evidence.Observed &&
		summary.Outcome != trace.ReplicationUnknown {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromTraceArchive(summary trace.ArchiveVerificationSummary) Report {
	report := verifiedReport(KindTraceArchive)
	report.Identity = summary.ArchiveSHA256
	if summary.Partial == 0 {
		report.EvidenceState = evidence.Observed
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		report.EvidenceState = evidence.Unknown
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	if summary.SchemaVersion >= 2 {
		setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	}
	return finalize(report)
}

func reportFromTraceReplication(summary trace.ReplicationLedgerVerificationSummary) Report {
	report := verifiedReport(KindTraceReplication)
	report.Identity = summary.LedgerSHA256
	report.Outcome = string(summary.Outcome)
	report.EvidenceState = summary.EvidenceState
	setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	if summary.OrderBalanced &&
		summary.ResetConfirmedPairs == summary.Pairs &&
		summary.CompletePairs == summary.Pairs &&
		summary.UnknownPairs == 0 &&
		summary.EvidenceState == evidence.Observed &&
		summary.Outcome != trace.ReplicationUnknown {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromTraceCase(summary trace.CaseVerificationSummary) Report {
	report := verifiedReport(KindTraceCase)
	report.Identity = summary.CaseSHA256
	setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	if summary.Entries > 0 && summary.UnknownEntries == 0 {
		report.EvidenceState = evidence.Observed
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		report.EvidenceState = evidence.Unknown
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromTraceStudy(summary trace.StudyVerificationSummary) Report {
	report := verifiedReport(KindTraceStudy)
	report.Identity = summary.StudySHA256
	report.Outcome = string(summary.Outcome)
	report.EvidenceState = summary.EvidenceState
	setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	if summary.Runs > 0 &&
		summary.SupportedRuns == summary.Runs &&
		summary.UnknownRuns == 0 &&
		summary.BalancedRuns == summary.Runs &&
		summary.CompletePairs == summary.Pairs &&
		summary.UnknownPairs == 0 &&
		summary.EvidenceState == evidence.Observed &&
		summary.Outcome != trace.ReplicationUnknown {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromSourceAdapter(summary trace.SourceAdapterRunSummary) Report {
	report := verifiedReport(KindSourceAdapterRun)
	report.Identity = summary.ReceiptSHA256
	if summary.Trace.Completeness == trace.Complete && summary.Session.Completeness == trace.Complete {
		report.EvidenceState = evidence.Observed
	} else {
		report.EvidenceState = evidence.Unknown
	}
	if summary.Receipt.ProvenanceSHA256 == "" {
		setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	} else {
		setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	}
	// A portable run deliberately omits the procedure and executable bytes. It
	// can be verified and compared, but cannot be replayed from this directory.
	if report.EvidenceState == evidence.Observed {
		setTier(&report, TierReplay, StatusUnavailable, ReasonNotApplicable)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromHAR(summary browser.HARVerificationSummary) Report {
	report := verifiedReport(KindBrowserHAR)
	report.Identity = summary.HARFileSHA256
	setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	setTier(&report, TierReplay, StatusUnavailable, ReasonNotApplicable)
	return finalize(report)
}
func reportFromTrace(summary trace.VerificationSummary) Report {
	report := verifiedReport(KindTrace)
	report.Identity = summary.TraceSHA256
	setTier(&report, TierBoundary, StatusUnavailable, ReasonProvenanceUnavailable)
	if summary.Completeness == trace.Complete {
		report.EvidenceState = evidence.Observed
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		report.EvidenceState = evidence.Unknown
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromWeather(review browser.WeatherReview) Report {
	report := verifiedReport(KindBrowserWeather)
	report.Identity = review.ReceiptSHA256
	report.EvidenceState = review.Ladder.EvidenceState
	report.SelectionState = string(review.Ladder.SelectionState)
	if review.Ladder.SelectionState == minimize.SelectionSelected {
		report.SelectedCandidate = review.Ladder.SelectedCandidate
	}
	setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	if review.Ladder.SelectionState == minimize.SelectionSelected && review.Ladder.EvidenceState == evidence.Observed {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func reportFromBrowserMinimization(summary minimize.LadderSummary, identity string) Report {
	report := reportFromMinimization(summaryAsMinimization(summary), identity)
	report.ArtifactKind = KindBrowserMinimization
	report.Reason = ""
	setTier(&report, TierBoundary, StatusPass, ReasonVerified)
	return finalize(report)
}

func summaryAsMinimization(summary minimize.LadderSummary) minimize.MinimizationSummary {
	return minimize.MinimizationSummary{
		SchemaVersion:          summary.SchemaVersion,
		PlanName:               summary.PlanName,
		Variable:               summary.Variable,
		ReferenceCandidate:     summary.ReferenceCandidate,
		FunctionalityCriterion: summary.FunctionalityCriterion,
		PairsPerOrder:          summary.PairsPerOrder,
		EvidenceState:          summary.EvidenceState,
		SelectionState:         summary.SelectionState,
		SelectedCandidate:      summary.SelectedCandidate,
		CandidateResults:       summary.CandidateResults,
	}
}

func reportFromMinimization(summary minimize.MinimizationSummary, identity string) Report {
	report := verifiedReport(KindAndroidMinimization)
	report.Identity = identity
	report.EvidenceState = summary.EvidenceState
	report.SelectionState = string(summary.SelectionState)
	if summary.SelectionState == minimize.SelectionSelected {
		report.SelectedCandidate = summary.SelectedCandidate
	}

	boundaryStatus, boundaryReason := minimizationBoundary(summary.CandidateResults)
	setTier(&report, TierBoundary, boundaryStatus, boundaryReason)
	if minimizationReady(summary) {
		setTier(&report, TierReplay, StatusPass, ReasonVerified)
	} else {
		setTier(&report, TierReplay, StatusUnknown, ReasonIncompleteCapture)
	}
	return finalize(report)
}

func minimizationBoundary(results []minimize.CandidateResult) (Status, string) {
	if len(results) == 0 {
		return StatusFail, ReasonArtifactRejected
	}
	hasProvenance := false
	missingProvenance := false
	for _, result := range results {
		if result.ProvenanceSHA256 == "" {
			missingProvenance = true
		} else {
			hasProvenance = true
		}
	}
	if !hasProvenance {
		return StatusUnavailable, ReasonProvenanceUnavailable
	}
	if missingProvenance {
		return StatusFail, ReasonProvenanceInconsistent
	}
	// Candidate names are part of each manifest contract, so authenticated
	// candidates intentionally have candidate-specific procedure identities.
	return StatusPass, ReasonVerified
}

func minimizationReady(summary minimize.MinimizationSummary) bool {
	for _, result := range summary.CandidateResults {
		if result.CompletedPairs != result.Pairs ||
			result.UnknownPairs > 0 ||
			result.Outcome == bundle.ReplicationUnknown {
			return false
		}
	}
	return len(summary.CandidateResults) > 0
}

func verifiedReport(kind ArtifactKind) Report {
	return Report{
		SchemaVersion: SchemaVersion,
		ArtifactKind:  kind,
		EvidenceState: evidence.Unknown,
		Tiers: []TierResult{
			{Tier: TierStructural, Status: StatusPass, Reason: ReasonVerified},
			{Tier: TierIntegrity, Status: StatusPass, Reason: ReasonVerified},
			{Tier: TierBoundary, Status: StatusUnavailable, Reason: ReasonNotChecked},
			{Tier: TierReplay, Status: StatusUnavailable, Reason: ReasonNotChecked},
		},
	}
}

func unavailableReport(kind ArtifactKind, reason string) Report {
	return Report{
		SchemaVersion: SchemaVersion,
		ArtifactKind:  kind,
		Overall:       StatusUnavailable,
		EvidenceState: evidence.Unknown,
		Tiers: []TierResult{
			{Tier: TierStructural, Status: StatusUnavailable, Reason: reason},
			{Tier: TierIntegrity, Status: StatusUnavailable, Reason: reason},
			{Tier: TierBoundary, Status: StatusUnavailable, Reason: reason},
			{Tier: TierReplay, Status: StatusUnavailable, Reason: reason},
		},
		Reason: reason,
	}
}

func rejectedReport(kind ArtifactKind) Report {
	return Report{
		SchemaVersion: SchemaVersion,
		ArtifactKind:  kind,
		Overall:       StatusFail,
		EvidenceState: evidence.Unknown,
		Tiers: []TierResult{
			{Tier: TierStructural, Status: StatusFail, Reason: ReasonArtifactRejected},
			{Tier: TierIntegrity, Status: StatusFail, Reason: ReasonArtifactRejected},
			{Tier: TierBoundary, Status: StatusUnavailable, Reason: ReasonArtifactRejected},
			{Tier: TierReplay, Status: StatusUnavailable, Reason: ReasonArtifactRejected},
		},
		Reason: ReasonArtifactRejected,
	}
}

func setTier(report *Report, tier Tier, status Status, reason string) {
	for index := range report.Tiers {
		if report.Tiers[index].Tier == tier {
			report.Tiers[index].Status = status
			report.Tiers[index].Reason = reason
			return
		}
	}
}

func finalize(report Report) Report {
	hasPass := false
	hasUnavailable := false
	for _, tier := range report.Tiers {
		switch tier.Status {
		case StatusFail:
			report.Overall = StatusFail
			if report.Reason == "" {
				report.Reason = tierReason(tier, ReasonArtifactRejected)
			}
			return report
		case StatusUnknown:
			report.Overall = StatusUnknown
			if report.Reason == "" {
				report.Reason = tierReason(tier, ReasonIncompleteCapture)
			}
			return report
		case StatusWarning:
			report.Overall = StatusWarning
			if report.Reason == "" {
				report.Reason = tierReason(tier, ReasonValidationIncomplete)
			}
			return report
		case StatusPass:
			hasPass = true
		case StatusUnavailable:
			hasUnavailable = true
		}
	}
	if hasPass && hasUnavailable {
		report.Overall = StatusWarning
		if report.Reason == "" {
			report.Reason = unavailableReason(report.Tiers)
		}
		return report
	}
	if hasPass {
		report.Overall = StatusPass
		if report.Reason == "" {
			report.Reason = ReasonVerified
		}
		return report
	}
	report.Overall = StatusUnavailable
	if report.Reason == "" {
		report.Reason = ReasonNotChecked
	}
	return report
}

func tierReason(tier TierResult, fallback string) string {
	if tier.Reason == "" {
		return fallback
	}
	return tier.Reason
}

func unavailableReason(tiers []TierResult) string {
	for _, tier := range tiers {
		if tier.Status == StatusUnavailable && tier.Reason != "" && tier.Reason != ReasonNotApplicable && tier.Reason != ReasonNotChecked {
			return tier.Reason
		}
	}
	return ReasonValidationIncomplete
}
