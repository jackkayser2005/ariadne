package browser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

// TaskConfirmation contains only explicit user reports. It cannot promote a
// manual confirmation into an automatically verified functionality result.
type TaskConfirmation struct {
	Baseline  string `json:"baseline"`
	Treatment string `json:"treatment"`
}

func (confirmation TaskConfirmation) validate() error {
	for _, value := range []string{confirmation.Baseline, confirmation.Treatment} {
		if !slices.Contains([]string{"unknown", "works", "broken"}, value) {
			return errors.New("task confirmation is invalid")
		}
	}
	return nil
}

// DisclosureCount counts unique supported marker-bearing network observations.
// Responses and blocked attempts are linked to their recorded request reference.
type DisclosureCount struct {
	Requests        int `json:"requests"`
	Responses       int `json:"responses"`
	Blocked         int `json:"blocked"`
	WebSocketFrames int `json:"websocket_frames"`
}

// JourneyComparison keeps observed counts, bounded reduction, user reports,
// and automatic functionality classification separate. It contains no names.
type JourneyComparison struct {
	SchemaVersion          int                              `json:"schema_version"`
	BaselineJourneySHA256  string                           `json:"baseline_journey_sha256"`
	TreatmentJourneySHA256 string                           `json:"treatment_journey_sha256"`
	Order                  string                           `json:"order"`
	PairBound              bool                             `json:"pair_bound"`
	Baseline               DisclosureCount                  `json:"baseline"`
	Treatment              DisclosureCount                  `json:"treatment"`
	Reduction              string                           `json:"reduction"`
	EvidenceState          evidence.State                   `json:"evidence_state"`
	Functionality          TaskConfirmation                 `json:"functionality"`
	FunctionalityState     evidence.State                   `json:"functionality_state"`
	AutomaticFunctionality minimize.CandidateClassification `json:"automatic_functionality"`
	Trace                  trace.Comparison                 `json:"trace"`
	Unknowns               []string                         `json:"unknowns"`
}

// CompareInvestigations freshly verifies both saved sources before comparing
// their supported observations. Missing private trial bindings remain unknown.
func CompareInvestigations(baselinePath, treatmentPath string, confirmation TaskConfirmation) (JourneyComparison, error) {
	baseline, baseContext, err := readInvestigation(baselinePath)
	if err != nil {
		return JourneyComparison{}, err
	}
	treatment, trialContext, err := readInvestigation(treatmentPath)
	if err != nil {
		return JourneyComparison{}, err
	}
	return compareJourneys(baseline, baseContext, treatment, trialContext, confirmation)
}

func compareJourneys(baseline JourneyBundle, baseContext privateJourneyContext, treatment JourneyBundle, trialContext privateJourneyContext, confirmation TaskConfirmation) (JourneyComparison, error) {
	if confirmation.validate() != nil || baseline.Verify() != nil || treatment.Verify() != nil {
		return JourneyComparison{}, errors.New("comparison inputs failed verification")
	}
	result := JourneyComparison{SchemaVersion: 1, BaselineJourneySHA256: baseline.Receipt.JourneySHA256, TreatmentJourneySHA256: treatment.Receipt.JourneySHA256, Order: "unknown", Reduction: "unknown", EvidenceState: evidence.Unknown, Functionality: confirmation, FunctionalityState: evidence.Unknown, Unknowns: []string{}}
	var err error
	result.Trace, err = trace.Compare(baseline.Trace, treatment.Trace)
	if err != nil {
		return result, err
	}
	result.Baseline, err = journeyDisclosure(baseline.Journey)
	if err != nil {
		return result, err
	}
	result.Treatment, err = journeyDisclosure(treatment.Journey)
	if err != nil {
		return result, err
	}
	if baseContext.Trial == nil || trialContext.Trial == nil {
		result.Unknowns = append(result.Unknowns, "trial-provenance-unavailable")
	} else {
		if baseContext.validate(baseline) != nil || trialContext.validate(treatment) != nil {
			return result, errors.New("comparison private context failed verification")
		}
		base, trial := baseContext.Trial, trialContext.Trial
		if base.Identity.Role != "baseline" || trial.Identity.Role != "treatment" || base.Identity.PairID != trial.Identity.PairID || base.Identity.InputSetID != trial.Identity.InputSetID || base.Identity.Order != trial.Identity.Order || base.Browser != trial.Browser || base.Platform != trial.Platform || baseContext.Destinations[0].Origin != trialContext.Destinations[0].Origin {
			return result, errors.New("investigations do not belong to the same controlled pair")
		}
		if (base.Identity.Order == "baseline-treatment" && base.StartedAt >= trial.StartedAt) || (base.Identity.Order == "treatment-baseline" && trial.StartedAt >= base.StartedAt) {
			return result, errors.New("recorded execution order disagrees with trial start times")
		}
		result.PairBound = true
		result.Order = base.Identity.Order
		if !slices.Equal(baseContext.Steps, trialContext.Steps) {
			result.Unknowns = append(result.Unknowns, "interaction-steps-differ")
		}
		if trial.Location == "unchanged" && len(trial.BlockOrigins) == 0 {
			result.Unknowns = append(result.Unknowns, "no-control-change")
		}
	}
	if baseline.Journey.Completeness != trace.Complete || treatment.Journey.Completeness != trace.Complete {
		result.Unknowns = append(result.Unknowns, "incomplete-visibility")
	}
	if result.Baseline.Requests+result.Baseline.WebSocketFrames == 0 {
		result.Unknowns = append(result.Unknowns, "baseline-has-no-supported-disclosure")
	}
	if len(result.Unknowns) == 0 {
		result.EvidenceState = evidence.Observed
		result.Reduction = "not-reduced"
		if result.Treatment.Requests-result.Treatment.Blocked+result.Treatment.WebSocketFrames < result.Baseline.Requests-result.Baseline.Blocked+result.Baseline.WebSocketFrames {
			result.Reduction = "reduced"
		}
	}
	// A user's task report is useful, but remains claimed even when both
	// recordings have complete synthetic-marker coverage.
	if confirmation.Baseline != "unknown" && confirmation.Treatment != "unknown" {
		result.FunctionalityState = evidence.Claimed
	}
	outcome := trace.ReplicationUnknown
	if confirmation.Baseline == "works" && confirmation.Treatment == "works" {
		outcome = trace.NoChangeObserved
	} else if confirmation.Baseline == "works" && confirmation.Treatment == "broken" {
		outcome = trace.ReplicatedChange
	}
	result.AutomaticFunctionality = minimize.ClassifyFunctionality(outcome, result.FunctionalityState)
	return result, nil
}

func journeyDisclosure(journey trace.Journey) (DisclosureCount, error) {
	var result DisclosureCount
	requests := map[string]trace.JourneyObservation{}
	responses, blocked := map[string]bool{}, map[string]bool{}
	for _, observation := range journey.Observations {
		switch observation.Kind {
		case "request":
			if _, exists := requests[observation.Reference]; exists {
				return result, errors.New("duplicate journey request reference")
			}
			requests[observation.Reference] = observation
			if len(observation.Matches) > 0 {
				result.Requests++
			}
		case "response", "blocked":
			request, exists := requests[observation.Reference]
			if !exists {
				if len(observation.Matches) > 0 {
					return result, errors.New("unbound journey network result")
				}
				continue
			}
			if request.Destination != observation.Destination {
				return result, errors.New("journey network destination changed")
			}
			if len(request.Matches) == 0 {
				continue
			}
			if observation.Kind == "blocked" {
				blocked[observation.Reference] = true
			} else {
				responses[observation.Reference] = true
			}
		case "websocket-sent":
			if len(observation.Matches) > 0 {
				result.WebSocketFrames++
			}
		}
	}
	for reference := range blocked {
		if responses[reference] {
			return result, errors.New("journey request both responded and was blocked")
		}
	}
	result.Responses = len(responses)
	result.Blocked = len(blocked)
	return result, nil
}

// SHA256 identifies the safe comparison result, including the exact user report.
func (comparison JourneyComparison) SHA256() string {
	data, _ := json.Marshal(comparison)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
