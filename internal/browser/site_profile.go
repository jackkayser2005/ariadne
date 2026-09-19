package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/securefs"
)

// SiteControls are the persistent controls supported by the companion.
// Approximate location is deliberately excluded until it can be enforced there.
type SiteControls struct {
	Location     string   `json:"location"`
	BlockOrigins []string `json:"block_origins"`
}

// ProtectionTest binds controls to a verified comparison's identities and
// bounded conclusions. It is a receipt, not a signature or source attestation.
type ProtectionTest struct {
	BaselineJourneySHA256  string                           `json:"baseline_journey_sha256"`
	TreatmentJourneySHA256 string                           `json:"treatment_journey_sha256"`
	ComparisonSHA256       string                           `json:"comparison_sha256"`
	Order                  string                           `json:"order"`
	Baseline               DisclosureCount                  `json:"baseline"`
	Treatment              DisclosureCount                  `json:"treatment"`
	Reduction              string                           `json:"reduction"`
	EvidenceState          evidence.State                   `json:"evidence_state"`
	Functionality          TaskConfirmation                 `json:"functionality"`
	FunctionalityState     evidence.State                   `json:"functionality_state"`
	AutomaticFunctionality minimize.CandidateClassification `json:"automatic_functionality"`
}

// SiteProtectionProfile is a private file intended for explicit companion import.
// It contains destination names and must never be substituted for portable evidence.
type SiteProtectionProfile struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	SiteOrigin    string         `json:"site_origin"`
	Controls      SiteControls   `json:"controls"`
	Test          ProtectionTest `json:"test"`
	ProfileSHA256 string         `json:"profile_sha256,omitempty"`
}

// BuildSiteProtectionProfile rereads the tested sources and copies only controls
// recorded by their capture engine. Callers cannot append untested controls.
func BuildSiteProtectionProfile(baselinePath, treatmentPath string, confirmation TaskConfirmation) (SiteProtectionProfile, error) {
	base, bc, err := readInvestigation(baselinePath)
	if err != nil {
		return SiteProtectionProfile{}, err
	}
	trial, tc, err := readInvestigation(treatmentPath)
	if err != nil {
		return SiteProtectionProfile{}, err
	}
	comparison, err := compareJourneys(base, bc, trial, tc, confirmation)
	if err != nil {
		return SiteProtectionProfile{}, err
	}
	if !comparison.PairBound {
		return SiteProtectionProfile{}, errors.New("a protection profile requires a bound baseline and treatment")
	}
	if tc.Trial.Location == "approximate" {
		return SiteProtectionProfile{}, errors.New("approximate location is lab-only; retest with unchanged or denied location before exporting protection")
	}
	if tc.Trial.Location == "unchanged" && len(tc.Trial.BlockOrigins) == 0 {
		return SiteProtectionProfile{}, errors.New("the trial did not install a persistent protection control")
	}
	profile := SiteProtectionProfile{SchemaVersion: 1, Kind: "private-site-protection", SiteOrigin: tc.Destinations[0].Origin, Controls: SiteControls{Location: tc.Trial.Location, BlockOrigins: slices.Clone(tc.Trial.BlockOrigins)}, Test: ProtectionTest{BaselineJourneySHA256: comparison.BaselineJourneySHA256, TreatmentJourneySHA256: comparison.TreatmentJourneySHA256, ComparisonSHA256: comparison.SHA256(), Order: comparison.Order, Baseline: comparison.Baseline, Treatment: comparison.Treatment, Reduction: comparison.Reduction, EvidenceState: comparison.EvidenceState, Functionality: comparison.Functionality, FunctionalityState: comparison.FunctionalityState, AutomaticFunctionality: comparison.AutomaticFunctionality}}
	profile.ProfileSHA256 = profile.identity()
	return profile, profile.Validate()
}

func (profile SiteProtectionProfile) identity() string {
	profile.ProfileSHA256 = ""
	data, _ := json.Marshal(profile)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

// Validate checks private profile integrity and rejects unsupported enforcement
// or invented automatic functionality. Source identities remain receipt references.
func (profile SiteProtectionProfile) Validate() error {
	u, err := InvestigationURL(profile.SiteOrigin)
	if err != nil || u.Scheme+"://"+u.Host != profile.SiteOrigin || profile.SchemaVersion != 1 || profile.Kind != "private-site-protection" || profile.ProfileSHA256 != profile.identity() {
		return errors.New("private protection profile failed verification")
	}
	if !slices.Contains([]string{"unchanged", "deny"}, profile.Controls.Location) || profile.Controls.BlockOrigins == nil || len(profile.Controls.BlockOrigins) > 64 || (profile.Controls.Location == "unchanged" && len(profile.Controls.BlockOrigins) == 0) {
		return errors.New("profile controls are invalid or lab-only")
	}
	for i, origin := range profile.Controls.BlockOrigins {
		u, err := InvestigationURL(origin)
		if err != nil || u.Scheme+"://"+u.Host != origin || (i > 0 && profile.Controls.BlockOrigins[i-1] >= origin) {
			return errors.New("profile destination controls are invalid")
		}
	}
	proof := profile.Test
	if !journeyDigest(proof.BaselineJourneySHA256) || !journeyDigest(proof.TreatmentJourneySHA256) || !journeyDigest(proof.ComparisonSHA256) || !slices.Contains([]string{"baseline-treatment", "treatment-baseline"}, proof.Order) || proof.Functionality.validate() != nil || proof.AutomaticFunctionality != minimize.CandidateUnknown {
		return errors.New("profile test receipt is invalid")
	}
	for _, count := range []DisclosureCount{proof.Baseline, proof.Treatment} {
		if count.Requests < 0 || count.Requests > 2048 || count.Responses < 0 || count.Blocked < 0 || count.Responses+count.Blocked > count.Requests || count.WebSocketFrames < 0 || count.WebSocketFrames > 2048 {
			return errors.New("profile observation counts are invalid")
		}
	}
	if (proof.EvidenceState == evidence.Unknown && proof.Reduction != "unknown") || (proof.EvidenceState == evidence.Observed && !slices.Contains([]string{"reduced", "not-reduced"}, proof.Reduction)) || !slices.Contains([]evidence.State{evidence.Unknown, evidence.Observed}, proof.EvidenceState) {
		return errors.New("profile reduction overstates evidence")
	}
	if proof.EvidenceState == evidence.Observed {
		reduced := proof.Treatment.Requests-proof.Treatment.Blocked+proof.Treatment.WebSocketFrames < proof.Baseline.Requests-proof.Baseline.Blocked+proof.Baseline.WebSocketFrames
		if reduced != (proof.Reduction == "reduced") {
			return errors.New("profile reduction disagrees with its counts")
		}
	}
	wantState := evidence.Claimed
	if proof.Functionality.Baseline == "unknown" || proof.Functionality.Treatment == "unknown" {
		wantState = evidence.Unknown
	}
	if proof.FunctionalityState != wantState {
		return errors.New("profile functionality overstates a user report")
	}
	return nil
}

// PrivateJSON validates and serializes a profile containing local destination names.
func (profile SiteProtectionProfile) PrivateJSON() ([]byte, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("private profile exceeds its size limit")
	}
	return data, nil
}

// DecodeSiteProtectionProfile reads the strict, bounded private profile format.
func DecodeSiteProtectionProfile(data []byte) (SiteProtectionProfile, error) {
	var profile SiteProtectionProfile
	if err := decodeInvestigationJSON(data, 64<<10, &profile); err != nil {
		return profile, err
	}
	return profile, profile.Validate()
}

// SaveSiteProtectionProfile publishes one private file without replacing existing data.
func SaveSiteProtectionProfile(path string, profile SiteProtectionProfile) error {
	data, err := profile.PrivateJSON()
	if err != nil {
		return err
	}
	if err := securefs.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("profile output directory is unavailable")
	}
	return securefs.WriteExclusiveExistingParent(path, data, 0600)
}
