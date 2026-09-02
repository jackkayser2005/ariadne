// Package validation exposes one safe, tiered entry point for the artifact
// verifiers that are stable enough to compose.
package validation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/minimize"
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
	if filepath.Base(path) != "manifest.json" && !strings.EqualFold(filepath.Ext(path), ".json") {
		return unavailableReport(KindUnknown, ReasonUnsupportedArtifact)
	}
	return validateManifest(path)
}

func validateDirectory(path string) Report {
	replication, replicationErr := inspectMarker(path, "replication.json")
	minimization, minimizationErr := inspectMarker(path, "minimization.json")
	if replicationErr != nil || minimizationErr != nil {
		return unavailableReport(KindUnknown, ReasonArtifactUnavailable)
	}
	if replication.present && minimization.present {
		return rejectedReport(KindUnknown)
	}
	if replication.present {
		if !replication.regular {
			return rejectedReport(KindAndroidReplication)
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
