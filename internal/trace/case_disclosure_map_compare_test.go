package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCompareCaseDisclosureMapWorkspacesReportsCompleteChanges(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("1", 64)
	first := assembleComparisonCase(t, root, "first", validArchiveTrace("location"), "android-experiment-001", procedure)
	second := assembleComparisonCase(t, root, "second", validArchiveTrace("consent"), "android-experiment-001", procedure)

	comparison, err := CompareCaseDisclosureMapWorkspaces(first, second, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonChanged || comparison.EvidenceState != evidence.Observed || comparison.FirstCoverageState != evidence.Observed || comparison.SecondCoverageState != evidence.Observed {
		t.Fatalf("complete comparison = %#v", comparison)
	}
	if comparison.InvestigationCommitmentSHA256 != comparisonCommitment() || comparison.ComparedCategories != 2 || comparison.ComparedBoundaries != 2 {
		t.Fatalf("complete comparison identity = %#v", comparison)
	}
	if !slices.Equal(comparison.AddedCategories, []string{"consent"}) || !slices.Equal(comparison.RemovedCategories, []string{"location"}) || len(comparison.UnknownCategories) != 0 {
		t.Fatalf("category changes = %#v", comparison)
	}
	if len(comparison.AddedBoundaries) != 1 || len(comparison.RemovedBoundaries) != 1 || len(comparison.UnknownBoundaries) != 0 {
		t.Fatalf("boundary changes = %#v", comparison)
	}
	if comparison.AddedBoundaries[0].Category != "consent" || comparison.RemovedBoundaries[0].Category != "location" || comparison.AddedBoundaries[0].Source != "android" || comparison.AddedBoundaries[0].Adapter != "android-experiment-001" {
		t.Fatalf("boundary identities = %#v", comparison)
	}
	data, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{root, "https://", "secret-value", "private-arg", "target-device"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("comparison contains forbidden value %q: %s", forbidden, data)
		}
	}
}

func TestCompareCaseDisclosureMapWorkspacesReturnsSameForIdenticalCompleteMaps(t *testing.T) {
	root := t.TempDir()
	workspace := assembleComparisonCase(t, root, "same", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("2", 64))

	comparison, err := CompareCaseDisclosureMapWorkspaces(workspace, workspace, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonSame || comparison.EvidenceState != evidence.Observed || comparison.IncomparableReason != "" || len(comparison.AddedBoundaries) != 0 || len(comparison.RemovedBoundaries) != 0 {
		t.Fatalf("identical comparison = %#v", comparison)
	}
	if comparison.ComparedCategories != 1 || comparison.ComparedBoundaries != 1 {
		t.Fatalf("identical comparison counts = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesPreservesPartialUnknownSemantics(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("3", 64)
	first := assembleComparisonCase(t, root, "complete", validArchiveTrace("location"), "android-experiment-001", procedure)
	partial := validArchiveTrace("region")
	partial.Completeness = Partial
	second := assembleComparisonCase(t, root, "partial", partial, "android-experiment-001", procedure)

	comparison, err := CompareCaseDisclosureMapWorkspaces(first, second, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonChanged || comparison.EvidenceState != evidence.Unknown || comparison.IncomparableReason != "partial coverage leaves category or boundary absences unresolved" {
		t.Fatalf("partial comparison = %#v", comparison)
	}
	if !slices.Equal(comparison.AddedCategories, []string{"region"}) || len(comparison.RemovedCategories) != 0 || !slices.Equal(comparison.UnknownCategories, []string{"location"}) {
		t.Fatalf("partial category changes = %#v", comparison)
	}
	if len(comparison.AddedBoundaries) != 1 || len(comparison.RemovedBoundaries) != 0 || len(comparison.UnknownBoundaries) != 1 || comparison.AddedBoundaries[0].Category != "region" || comparison.UnknownBoundaries[0].Category != "location" {
		t.Fatalf("partial boundary changes = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesDoesNotCallPartialMapsSame(t *testing.T) {
	root := t.TempDir()
	partial := validArchiveTrace("location")
	partial.Completeness = Partial
	workspace := assembleComparisonCase(t, root, "partial", partial, "android-experiment-001", strings.Repeat("4", 64))

	comparison, err := CompareCaseDisclosureMapWorkspaces(workspace, workspace, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.EvidenceState != evidence.Unknown || comparison.IncomparableReason != "partial coverage prevents a complete disclosure-map comparison" {
		t.Fatalf("partial identical comparison = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesReturnsIncomparableForCommitmentOrProvenance(t *testing.T) {
	root := t.TempDir()
	first := assembleComparisonCaseWithCommitment(t, root, "first", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("5", 64), strings.Repeat("e", 64))
	second := assembleComparisonCaseWithCommitment(t, root, "second", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("6", 64), strings.Repeat("f", 64))

	comparison, err := CompareCaseDisclosureMapWorkspaces(first, second, strings.Repeat("e", 64))
	if err != nil {
		t.Fatalf("commitment comparison error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.IncomparableReason != "investigation commitments differ" || comparison.InvestigationCommitmentSHA256 != "" || comparison.EvidenceState != evidence.Unknown {
		t.Fatalf("commitment mismatch = %#v", comparison)
	}

	first = assembleComparisonCaseWithCommitment(t, root, "first-same-commitment", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("5", 64), strings.Repeat("e", 64))
	second = assembleComparisonCaseWithCommitment(t, root, "second-same-commitment", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("6", 64), strings.Repeat("e", 64))
	comparison, err = CompareCaseDisclosureMapWorkspaces(first, second, strings.Repeat("e", 64))
	if err != nil {
		t.Fatalf("provenance comparison error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.IncomparableReason != "reviewed source provenance differs" || comparison.InvestigationCommitmentSHA256 != strings.Repeat("e", 64) || comparison.EvidenceState != evidence.Unknown {
		t.Fatalf("provenance mismatch = %#v", comparison)
	}

	comparison, err = CompareCaseDisclosureMapWorkspaces(first, second, strings.Repeat("f", 64))
	if err != nil {
		t.Fatalf("supplied commitment comparison error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.IncomparableReason != "supplied investigation commitment does not match case" || comparison.InvestigationCommitmentSHA256 != "" || comparison.EvidenceState != evidence.Unknown {
		t.Fatalf("supplied commitment mismatch = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesRejectsProvenanceSwappedBetweenBoundaries(t *testing.T) {
	root := t.TempDir()
	procedureA := strings.Repeat("8", 64)
	procedureB := strings.Repeat("9", 64)
	first := assembleComparisonMultiArchiveCase(t, root, "swapped-first", []Document{validArchiveTrace("location"), validArchiveTrace("consent")}, []string{procedureA, procedureB}, comparisonCommitment())
	second := assembleComparisonMultiArchiveCase(t, root, "swapped-second", []Document{validArchiveTrace("location"), validArchiveTrace("consent")}, []string{procedureB, procedureA}, comparisonCommitment())

	comparison, err := CompareCaseDisclosureMapWorkspaces(first, second, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.IncomparableReason != "reviewed source provenance is associated with different boundaries" || comparison.EvidenceState != evidence.Unknown {
		t.Fatalf("swapped provenance comparison = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesReturnsIncomparableForLegacyUnboundCases(t *testing.T) {
	root := t.TempDir()
	legacy := assembleComparisonCaseWithCommitment(t, root, "legacy", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("a", 64), "")

	comparison, err := CompareCaseDisclosureMapWorkspaces(legacy, legacy, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonIncomparable || comparison.IncomparableReason != "investigation commitment is unavailable" || comparison.InvestigationCommitmentSHA256 != "" || comparison.EvidenceState != evidence.Unknown {
		t.Fatalf("legacy comparison = %#v", comparison)
	}
}

func TestCompareCaseDisclosureMapWorkspacesRejectsInvalidInputsAndTampering(t *testing.T) {
	root := t.TempDir()
	workspace := assembleComparisonCase(t, root, "valid", validArchiveTrace("location"), "android-experiment-001", strings.Repeat("7", 64))
	for _, commitment := range []string{"", strings.Repeat("A", 64), "not-a-digest"} {
		if _, err := CompareCaseDisclosureMapWorkspaces(workspace, workspace, commitment); err == nil {
			t.Fatalf("invalid commitment %q was accepted", commitment)
		}
	}
	for _, firstWorkspace := range []string{"", filepath.Join(root, "missing")} {
		if _, err := CompareCaseDisclosureMapWorkspaces(firstWorkspace, workspace, strings.Repeat("8", 64)); err == nil {
			t.Fatalf("unavailable first workspace %q was accepted", firstWorkspace)
		}
	}

	roundPath := filepath.Join(workspace, "disclosure-round.json")
	data, err := os.ReadFile(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(roundPath, append(data, data...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseDisclosureMapWorkspaces(workspace, workspace, strings.Repeat("9", 64)); err == nil {
		t.Fatal("tampered disclosure round was accepted")
	}
}

func assembleComparisonCase(t *testing.T, root, name string, document Document, adapter, procedure string) string {
	return assembleComparisonCaseWithCommitment(t, root, name, document, adapter, procedure, comparisonCommitment())
}

func assembleComparisonCaseWithCommitment(t *testing.T, root, name string, document Document, adapter, procedure, commitment string) string {
	t.Helper()
	input := writeStandaloneArchiveInputWithAdapter(t, root, name, document, adapter, procedure)
	archivePath, roundPath := saveAssemblyArchive(t, root, name, input)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion:                 caseAssemblyPlanSchemaVersion,
		OrderBasis:                    "caller",
		InvestigationCommitmentSHA256: commitment,
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceArchive,
			ArtifactPath:      archivePath,
			QuestionRoundPath: roundPath,
		}},
	})
	workspace := filepath.Join(root, name+"-workspace")
	if _, err := AssembleCase(planPath, workspace); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func assembleComparisonMultiArchiveCase(t *testing.T, root, name string, documents []Document, procedures []string, commitment string) string {
	t.Helper()
	if len(documents) != len(procedures) {
		t.Fatal("comparison archive fixture lengths differ")
	}
	inputs := make([]ArchiveInput, 0, len(documents))
	for index, document := range documents {
		inputs = append(inputs, writeStandaloneArchiveInputWithAdapter(t, root, name+"-"+strconv.Itoa(index+1), document, "android-experiment-001", procedures[index]))
	}
	archivePath := filepath.Join(root, name+"-archive.json")
	if _, err := SaveArchive(inputs, archivePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, name+"-archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion:                 caseAssemblyPlanSchemaVersion,
		OrderBasis:                    "caller",
		InvestigationCommitmentSHA256: commitment,
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceArchive,
			ArtifactPath:      archivePath,
			QuestionRoundPath: roundPath,
		}},
	})
	workspace := filepath.Join(root, name+"-workspace")
	if _, err := AssembleCase(planPath, workspace); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func TestCompareCaseDisclosureMapWorkspacesSupportsReplicationEntries(t *testing.T) {
	root := t.TempDir()
	workspace := assembleComparisonReplicationCase(t, root, "replication")

	comparison, err := CompareCaseDisclosureMapWorkspaces(workspace, workspace, comparisonCommitment())
	if err != nil {
		t.Fatalf("CompareCaseDisclosureMapWorkspaces() error = %v", err)
	}
	if comparison.Result != caseDisclosureMapComparisonSame || comparison.EvidenceState != evidence.Observed || comparison.ComparedCategories == 0 || comparison.ComparedBoundaries == 0 {
		t.Fatalf("replication comparison = %#v", comparison)
	}
}

func assembleComparisonReplicationCase(t *testing.T, root, name string) string {
	t.Helper()
	ledgerPath, roundPath := writeCaseLedger(t)
	planPath := writeCaseAssemblyPlan(t, root, CaseAssemblyPlan{
		SchemaVersion:                 caseAssemblyPlanSchemaVersion,
		OrderBasis:                    "caller",
		InvestigationCommitmentSHA256: comparisonCommitment(),
		Entries: []CaseAssemblyPlanEntry{{
			Kind:              CaseEntryTraceReplication,
			ArtifactPath:      ledgerPath,
			QuestionRoundPath: roundPath,
		}},
	})
	workspace := filepath.Join(root, name+"-workspace")
	if _, err := AssembleCase(planPath, workspace); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func comparisonCommitment() string {
	return strings.Repeat("a", 64)
}
