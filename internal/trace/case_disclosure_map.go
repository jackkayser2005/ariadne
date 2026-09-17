package trace

import (
	"errors"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const caseDisclosureMapSchemaVersion = 1

// CaseDisclosureMap is a deterministic, raw-value-free projection of the
// reviewed trace labels embedded in one verified case. It describes where a
// category was observed; it does not correlate source identities or values.
type CaseDisclosureMap struct {
	SchemaVersion int                      `json:"schema_version"`
	CaseSHA256    string                   `json:"case_sha256"`
	Traces        int                      `json:"traces"`
	CoverageState evidence.State           `json:"coverage_state"`
	Categories    []CaseDisclosureCategory `json:"categories"`
}

// CaseDisclosureCategory groups safe observations by one reviewed data
// category.
type CaseDisclosureCategory struct {
	Category     string                      `json:"category"`
	Observations []CaseDisclosureObservation `json:"observations"`
}

// CaseDisclosureObservation identifies one safe category location across the
// verified trace snapshots. TraceCount counts retained trace documents, not
// source identities or event occurrences.
type CaseDisclosureObservation struct {
	Source        string         `json:"source"`
	Adapter       string         `json:"adapter"`
	Channel       string         `json:"channel"`
	Kind          string         `json:"kind"`
	Destination   string         `json:"destination"`
	TraceCount    int            `json:"trace_count"`
	EvidenceState evidence.State `json:"evidence_state"`
}

// BuildCaseDisclosureMap derives a safe category map from a verified case.
// The supplied summary must identify the same case package. Partial trace
// coverage makes only the aggregate coverage state unknown; labels that were
// directly retained remain observed.
func BuildCaseDisclosureMap(casePackage CasePackage, summary CaseVerificationSummary) (CaseDisclosureMap, error) {
	expectedSummary, err := caseSummary(casePackage)
	if err != nil {
		return CaseDisclosureMap{}, err
	}
	if summary.CaseSHA256 != expectedSummary.CaseSHA256 {
		return CaseDisclosureMap{}, errors.New("case disclosure map case identity does not match summary")
	}

	counts := make(map[caseDisclosureObservationKey]int)
	coverageState := evidence.Observed
	traces := 0
	for _, entry := range casePackage.Entries {
		switch entry.Kind {
		case CaseEntryTraceArchive:
			for _, archiveEntry := range entry.Archive.Entries {
				traces++
				if archiveEntry.Trace.Completeness == Partial {
					coverageState = evidence.Unknown
				}
				addCaseDisclosureTrace(counts, archiveEntry.Session, archiveEntry.Trace)
			}
		case CaseEntryTraceReplication:
			for _, pair := range entry.ReplicationLedger.Pairs {
				traces += 2
				if pair.BaselineTrace.Completeness == Partial || pair.TreatmentTrace.Completeness == Partial {
					coverageState = evidence.Unknown
				}
				addCaseDisclosureTrace(counts, pair.BaselineSession, pair.BaselineTrace)
				addCaseDisclosureTrace(counts, pair.TreatmentSession, pair.TreatmentTrace)
			}
		default:
			return CaseDisclosureMap{}, errors.New("case disclosure map entry kind is invalid")
		}
	}

	mapResult := CaseDisclosureMap{
		SchemaVersion: caseDisclosureMapSchemaVersion,
		CaseSHA256:    expectedSummary.CaseSHA256,
		Traces:        traces,
		CoverageState: coverageState,
		Categories:    make([]CaseDisclosureCategory, 0),
	}
	categoryIndexes := make(map[string]int)
	keys := make([]caseDisclosureObservationKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, compareCaseDisclosureObservationKeys)
	for _, key := range keys {
		categoryIndex, exists := categoryIndexes[key.Category]
		if !exists {
			categoryIndex = len(mapResult.Categories)
			categoryIndexes[key.Category] = categoryIndex
			mapResult.Categories = append(mapResult.Categories, CaseDisclosureCategory{
				Category:     key.Category,
				Observations: make([]CaseDisclosureObservation, 0),
			})
		}
		mapResult.Categories[categoryIndex].Observations = append(mapResult.Categories[categoryIndex].Observations, CaseDisclosureObservation{
			Source:        key.Source,
			Adapter:       key.Adapter,
			Channel:       key.Channel,
			Kind:          key.Kind,
			Destination:   key.Destination,
			TraceCount:    counts[key],
			EvidenceState: evidence.Observed,
		})
	}
	return mapResult, nil
}

type caseDisclosureObservationKey struct {
	Category    string
	Source      string
	Adapter     string
	Channel     string
	Kind        string
	Destination string
}

func addCaseDisclosureTrace(counts map[caseDisclosureObservationKey]int, session Session, document Document) {
	for _, event := range document.Events {
		for _, category := range event.Fields {
			counts[caseDisclosureObservationKey{
				Category:    category,
				Source:      session.Source,
				Adapter:     session.Adapter,
				Channel:     event.Channel,
				Kind:        event.Kind,
				Destination: event.Destination,
			}]++
		}
	}
}

func compareCaseDisclosureObservationKeys(left, right caseDisclosureObservationKey) int {
	for _, pair := range [][2]string{
		{left.Category, right.Category},
		{left.Source, right.Source},
		{left.Adapter, right.Adapter},
		{left.Channel, right.Channel},
		{left.Kind, right.Kind},
		{left.Destination, right.Destination},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
