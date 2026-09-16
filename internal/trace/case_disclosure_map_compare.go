package trace

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const (
	caseDisclosureMapComparisonSchemaVersion = 1
	caseDisclosureMapComparisonID            = "case-disclosure-map-change"
	caseDisclosureMapComparisonText          = "Did the reviewed disclosure map change between these cases?"
)

const (
	caseDisclosureMapComparisonSame         = "same"
	caseDisclosureMapComparisonChanged      = "changed"
	caseDisclosureMapComparisonIncomparable = "incomparable"
)

// CaseDisclosureMapComparison is a raw-value-free comparison of two verified
// assembled case workspaces. It reports only safe category and boundary
// presence changes; it does not infer chronology, causality, or enforcement.
type CaseDisclosureMapComparison struct {
	SchemaVersion                 int                               `json:"schema_version"`
	ComparisonID                  string                            `json:"comparison_id"`
	ComparisonQuestion            string                            `json:"comparison_question"`
	OrderBasis                    string                            `json:"order_basis"`
	Result                        string                            `json:"result"`
	IncomparableReason            string                            `json:"incomparable_reason,omitempty"`
	InvestigationCommitmentSHA256 string                            `json:"investigation_commitment_sha256"`
	FirstCaseSHA256               string                            `json:"first_case_sha256"`
	SecondCaseSHA256              string                            `json:"second_case_sha256"`
	FirstRoundSHA256              string                            `json:"first_round_sha256"`
	SecondRoundSHA256             string                            `json:"second_round_sha256"`
	FirstCoverageState            evidence.State                    `json:"first_coverage_state"`
	SecondCoverageState           evidence.State                    `json:"second_coverage_state"`
	EvidenceState                 evidence.State                    `json:"evidence_state"`
	ComparedCategories            int                               `json:"compared_categories"`
	AddedCategories               []string                          `json:"added_categories"`
	RemovedCategories             []string                          `json:"removed_categories"`
	UnknownCategories             []string                          `json:"unknown_categories"`
	ComparedBoundaries            int                               `json:"compared_boundaries"`
	AddedBoundaries               []CaseDisclosureMapBoundaryChange `json:"added_boundaries"`
	RemovedBoundaries             []CaseDisclosureMapBoundaryChange `json:"removed_boundaries"`
	UnknownBoundaries             []CaseDisclosureMapBoundaryChange `json:"unknown_boundaries"`
}

// CaseDisclosureMapBoundaryChange identifies one safe category location that
// was added, removed, or left unresolved by coverage.
type CaseDisclosureMapBoundaryChange struct {
	Category    string `json:"category"`
	Source      string `json:"source"`
	Adapter     string `json:"adapter"`
	Channel     string `json:"channel"`
	Kind        string `json:"kind"`
	Destination string `json:"destination"`
}

// CompareCaseDisclosureMapWorkspaces verifies two assembled case workspaces
// and compares their disclosure maps in caller-supplied order. The supplied
// commitment must be present and identical in both case packages.
func CompareCaseDisclosureMapWorkspaces(firstWorkspace, secondWorkspace, commitmentSHA256 string) (CaseDisclosureMapComparison, error) {
	if !ValidSHA256(commitmentSHA256) {
		return CaseDisclosureMapComparison{}, errors.New("trace case disclosure map comparison commitment is invalid")
	}
	firstMap, firstAssembly, firstProvenance, firstObservationProvenance, err := readCaseDisclosureMapWorkspace(firstWorkspace)
	if err != nil {
		return CaseDisclosureMapComparison{}, fmt.Errorf("first case disclosure map: %w", err)
	}
	secondMap, secondAssembly, secondProvenance, secondObservationProvenance, err := readCaseDisclosureMapWorkspace(secondWorkspace)
	if err != nil {
		return CaseDisclosureMapComparison{}, fmt.Errorf("second case disclosure map: %w", err)
	}

	comparison := CaseDisclosureMapComparison{
		SchemaVersion:       caseDisclosureMapComparisonSchemaVersion,
		ComparisonID:        caseDisclosureMapComparisonID,
		ComparisonQuestion:  caseDisclosureMapComparisonText,
		OrderBasis:          "caller",
		Result:              caseDisclosureMapComparisonSame,
		FirstCaseSHA256:     firstAssembly.CaseSHA256,
		SecondCaseSHA256:    secondAssembly.CaseSHA256,
		FirstRoundSHA256:    firstAssembly.DisclosureRoundSHA256,
		SecondRoundSHA256:   secondAssembly.DisclosureRoundSHA256,
		FirstCoverageState:  firstMap.CoverageState,
		SecondCoverageState: secondMap.CoverageState,
		EvidenceState:       evidence.Observed,
		AddedCategories:     make([]string, 0),
		RemovedCategories:   make([]string, 0),
		UnknownCategories:   make([]string, 0),
		AddedBoundaries:     make([]CaseDisclosureMapBoundaryChange, 0),
		RemovedBoundaries:   make([]CaseDisclosureMapBoundaryChange, 0),
		UnknownBoundaries:   make([]CaseDisclosureMapBoundaryChange, 0),
	}
	if firstAssembly.InvestigationCommitmentSHA256 == "" || secondAssembly.InvestigationCommitmentSHA256 == "" {
		return incomparableCaseDisclosureMapComparison(comparison, "investigation commitment is unavailable"), nil
	}
	if firstAssembly.InvestigationCommitmentSHA256 != secondAssembly.InvestigationCommitmentSHA256 {
		return incomparableCaseDisclosureMapComparison(comparison, "investigation commitments differ"), nil
	}
	if firstAssembly.InvestigationCommitmentSHA256 != commitmentSHA256 {
		return incomparableCaseDisclosureMapComparison(comparison, "supplied investigation commitment does not match case"), nil
	}
	comparison.InvestigationCommitmentSHA256 = commitmentSHA256
	if !slices.Equal(firstProvenance, secondProvenance) {
		return incomparableCaseDisclosureMapComparison(comparison, "reviewed source provenance differs"), nil
	}
	if !caseDisclosureObservationProvenanceEqual(firstObservationProvenance, secondObservationProvenance) {
		return incomparableCaseDisclosureMapComparison(comparison, "reviewed source provenance is associated with different boundaries"), nil
	}

	compareCaseDisclosureMapContents(&comparison, firstMap, secondMap)
	return comparison, nil
}

func readCaseDisclosureMapWorkspace(workspace string) (CaseDisclosureMap, CaseAssemblySummary, []caseDisclosureProvenanceKey, map[caseDisclosureObservationKey][]caseDisclosureProvenanceKey, error) {
	if workspace == "" {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, errors.New("workspace path is required")
	}
	assembly, err := VerifyCaseAssembly(workspace)
	if err != nil {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, err
	}
	casePackage, summary, err := ReadCase(filepath.Join(filepath.Clean(workspace), "case.json"))
	if err != nil {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, errors.New("case cannot be reopened")
	}
	mapResult, err := BuildCaseDisclosureMap(casePackage, summary)
	if err != nil {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, errors.New("disclosure map cannot be derived")
	}
	if mapResult.CaseSHA256 != assembly.CaseSHA256 || mapResult.CoverageState != assembly.CoverageState {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, errors.New("disclosure map identity does not match assembly")
	}
	provenance, observationProvenance, err := caseDisclosureProvenance(casePackage)
	if err != nil {
		return CaseDisclosureMap{}, CaseAssemblySummary{}, nil, nil, err
	}
	return mapResult, assembly, provenance, observationProvenance, nil
}

type caseDisclosureProvenanceKey struct {
	Source          string
	Adapter         string
	AdapterVersion  int
	ProcedureSHA256 string
	Scope           string
}

func caseDisclosureProvenance(casePackage CasePackage) ([]caseDisclosureProvenanceKey, map[caseDisclosureObservationKey][]caseDisclosureProvenanceKey, error) {
	provenance := make(map[caseDisclosureProvenanceKey]struct{})
	observationProvenance := make(map[caseDisclosureObservationKey]map[caseDisclosureProvenanceKey]struct{})
	addDocument := func(session Session, document Document) {
		key := caseDisclosureProvenanceFromSession(session)
		provenance[key] = struct{}{}
		for _, event := range document.Events {
			for _, category := range event.Fields {
				observationKey := caseDisclosureObservationKey{
					Category:    category,
					Source:      session.Source,
					Adapter:     session.Adapter,
					Channel:     event.Channel,
					Kind:        event.Kind,
					Destination: event.Destination,
				}
				if observationProvenance[observationKey] == nil {
					observationProvenance[observationKey] = make(map[caseDisclosureProvenanceKey]struct{})
				}
				observationProvenance[observationKey][key] = struct{}{}
			}
		}
	}
	for _, entry := range casePackage.Entries {
		switch entry.Kind {
		case CaseEntryTraceArchive:
			if entry.Archive == nil {
				return nil, nil, errors.New("case disclosure map archive is unavailable")
			}
			for _, archiveEntry := range entry.Archive.Entries {
				addDocument(archiveEntry.Session, archiveEntry.Trace)
			}
		case CaseEntryTraceReplication:
			if entry.ReplicationLedger == nil {
				return nil, nil, errors.New("case disclosure map replication ledger is unavailable")
			}
			for _, pair := range entry.ReplicationLedger.Pairs {
				addDocument(pair.BaselineSession, pair.BaselineTrace)
				addDocument(pair.TreatmentSession, pair.TreatmentTrace)
			}
		default:
			return nil, nil, errors.New("case disclosure map entry kind is invalid")
		}
	}
	result := make([]caseDisclosureProvenanceKey, 0, len(provenance))
	for key := range provenance {
		result = append(result, key)
	}
	slices.SortFunc(result, compareCaseDisclosureProvenanceKeys)

	perObservation := make(map[caseDisclosureObservationKey][]caseDisclosureProvenanceKey, len(observationProvenance))
	for observationKey, keys := range observationProvenance {
		values := make([]caseDisclosureProvenanceKey, 0, len(keys))
		for key := range keys {
			values = append(values, key)
		}
		slices.SortFunc(values, compareCaseDisclosureProvenanceKeys)
		perObservation[observationKey] = values
	}
	return result, perObservation, nil
}

func caseDisclosureObservationProvenanceEqual(first, second map[caseDisclosureObservationKey][]caseDisclosureProvenanceKey) bool {
	for key, firstValues := range first {
		secondValues, ok := second[key]
		if ok && !slices.Equal(firstValues, secondValues) {
			return false
		}
	}
	return true
}

func caseDisclosureProvenanceFromSession(session Session) caseDisclosureProvenanceKey {
	return caseDisclosureProvenanceKey{
		Source:          session.Source,
		Adapter:         session.Adapter,
		AdapterVersion:  session.AdapterVersion,
		ProcedureSHA256: session.ProcedureSHA256,
		Scope:           session.Scope,
	}
}

func compareCaseDisclosureProvenanceKeys(left, right caseDisclosureProvenanceKey) int {
	for _, pair := range [][2]string{
		{left.Source, right.Source},
		{left.Adapter, right.Adapter},
		{left.ProcedureSHA256, right.ProcedureSHA256},
		{left.Scope, right.Scope},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if left.AdapterVersion < right.AdapterVersion {
		return -1
	}
	if left.AdapterVersion > right.AdapterVersion {
		return 1
	}
	return 0
}

func incomparableCaseDisclosureMapComparison(comparison CaseDisclosureMapComparison, reason string) CaseDisclosureMapComparison {
	comparison.Result = caseDisclosureMapComparisonIncomparable
	comparison.EvidenceState = evidence.Unknown
	comparison.IncomparableReason = reason
	return comparison
}

func compareCaseDisclosureMapContents(comparison *CaseDisclosureMapComparison, first, second CaseDisclosureMap) {
	firstCategories, firstBoundaries := caseDisclosureMapKeys(first)
	secondCategories, secondBoundaries := caseDisclosureMapKeys(second)
	categoryUnion := make(map[string]struct{}, len(firstCategories)+len(secondCategories))
	for category := range firstCategories {
		categoryUnion[category] = struct{}{}
	}
	for category := range secondCategories {
		categoryUnion[category] = struct{}{}
	}
	boundaryUnion := make(map[caseDisclosureObservationKey]struct{}, len(firstBoundaries)+len(secondBoundaries))
	for boundary := range firstBoundaries {
		boundaryUnion[boundary] = struct{}{}
	}
	for boundary := range secondBoundaries {
		boundaryUnion[boundary] = struct{}{}
	}
	comparison.ComparedCategories = len(categoryUnion)
	comparison.ComparedBoundaries = len(boundaryUnion)

	firstComplete := first.CoverageState == evidence.Observed
	secondComplete := second.CoverageState == evidence.Observed
	categories := make([]string, 0, len(categoryUnion))
	for category := range categoryUnion {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	for _, category := range categories {
		_, inFirst := firstCategories[category]
		_, inSecond := secondCategories[category]
		switch {
		case !inFirst && inSecond && firstComplete:
			comparison.AddedCategories = append(comparison.AddedCategories, category)
		case inFirst && !inSecond && secondComplete:
			comparison.RemovedCategories = append(comparison.RemovedCategories, category)
		case inFirst != inSecond:
			comparison.UnknownCategories = append(comparison.UnknownCategories, category)
		}
	}

	boundaries := make([]caseDisclosureObservationKey, 0, len(boundaryUnion))
	for boundary := range boundaryUnion {
		boundaries = append(boundaries, boundary)
	}
	slices.SortFunc(boundaries, compareCaseDisclosureObservationKeys)
	for _, boundary := range boundaries {
		_, inFirst := firstBoundaries[boundary]
		_, inSecond := secondBoundaries[boundary]
		change := caseDisclosureMapBoundaryChange(boundary)
		switch {
		case !inFirst && inSecond && firstComplete:
			comparison.AddedBoundaries = append(comparison.AddedBoundaries, change)
		case inFirst && !inSecond && secondComplete:
			comparison.RemovedBoundaries = append(comparison.RemovedBoundaries, change)
		case inFirst != inSecond:
			comparison.UnknownBoundaries = append(comparison.UnknownBoundaries, change)
		}
	}

	if len(comparison.AddedCategories) > 0 || len(comparison.RemovedCategories) > 0 || len(comparison.AddedBoundaries) > 0 || len(comparison.RemovedBoundaries) > 0 {
		comparison.Result = caseDisclosureMapComparisonChanged
	}
	if !firstComplete || !secondComplete {
		comparison.EvidenceState = evidence.Unknown
		if comparison.Result == caseDisclosureMapComparisonSame {
			comparison.Result = caseDisclosureMapComparisonIncomparable
		}
		if comparison.Result == caseDisclosureMapComparisonIncomparable {
			comparison.IncomparableReason = "partial coverage prevents a complete disclosure-map comparison"
		} else {
			comparison.IncomparableReason = "partial coverage leaves category or boundary absences unresolved"
		}
	}
}

func caseDisclosureMapKeys(mapResult CaseDisclosureMap) (map[string]struct{}, map[caseDisclosureObservationKey]struct{}) {
	categories := make(map[string]struct{}, len(mapResult.Categories))
	boundaries := make(map[caseDisclosureObservationKey]struct{})
	for _, category := range mapResult.Categories {
		categories[category.Category] = struct{}{}
		for _, observation := range category.Observations {
			boundaries[caseDisclosureObservationKey{
				Category:    category.Category,
				Source:      observation.Source,
				Adapter:     observation.Adapter,
				Channel:     observation.Channel,
				Kind:        observation.Kind,
				Destination: observation.Destination,
			}] = struct{}{}
		}
	}
	return categories, boundaries
}

func caseDisclosureMapBoundaryChange(boundary caseDisclosureObservationKey) CaseDisclosureMapBoundaryChange {
	return CaseDisclosureMapBoundaryChange{
		Category:    boundary.Category,
		Source:      boundary.Source,
		Adapter:     boundary.Adapter,
		Channel:     boundary.Channel,
		Kind:        boundary.Kind,
		Destination: boundary.Destination,
	}
}
