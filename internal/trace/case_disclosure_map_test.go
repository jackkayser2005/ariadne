package trace

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestBuildCaseDisclosureMapGroupsCategoriesAndCountsTraces(t *testing.T) {
	archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
	ledgerPath, ledgerRoundPath := writeCaseLedger(t)
	casePath := filepath.Join(t.TempDir(), "case.json")
	if _, err := SaveCase([]CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}

	first, err := BuildCaseDisclosureMap(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildCaseDisclosureMap(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("disclosure map is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.SchemaVersion != 1 || first.CaseSHA256 != summary.CaseSHA256 || first.Traces != 6 || first.CoverageState != evidence.Observed {
		t.Fatalf("disclosure map identity = %#v", first)
	}
	if len(first.Categories) != 2 || first.Categories[0].Category != "consent" || first.Categories[1].Category != "region" {
		t.Fatalf("disclosure map categories = %#v", first.Categories)
	}
	if got := first.Categories[0].Observations; len(got) != 1 || got[0].Source != "browser" || got[0].Adapter != "browser-redacted-audit" || got[0].Channel != "network" || got[0].Kind != "request" || got[0].Destination != "analytics" || got[0].TraceCount != 2 || got[0].EvidenceState != evidence.Observed {
		t.Fatalf("consent observations = %#v", got)
	}
	if got := first.Categories[1].Observations; len(got) != 2 || got[0].Source != "android" || got[0].TraceCount != 2 || got[1].Source != "browser" || got[1].TraceCount != 4 {
		t.Fatalf("region observations = %#v", got)
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-value", "target-device", "private-arg", "https://"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("disclosure map contains forbidden value %q: %s", forbidden, data)
		}
	}
}

func TestBuildCaseDisclosureMapMarksPartialCoverageUnknown(t *testing.T) {
	root := t.TempDir()
	document := validArchiveTrace("location")
	document.Completeness = Partial
	input := writeStandaloneArchiveInput(t, root, "partial", document, strings.Repeat("e", 64))
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(root, "case.json")
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: roundPath}}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := BuildCaseDisclosureMap(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	if result.CoverageState != evidence.Unknown || result.Traces != 1 || len(result.Categories) != 1 || result.Categories[0].Category != "location" || result.Categories[0].Observations[0].EvidenceState != evidence.Observed {
		t.Fatalf("partial disclosure map = %#v", result)
	}
}

func TestBuildCaseDisclosureMapRejectsMismatchedOrInvalidInputs(t *testing.T) {
	archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
	casePath := filepath.Join(t.TempDir(), "case.json")
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	wrongSummary := summary
	wrongSummary.CaseSHA256 = strings.Repeat("a", 64)
	if _, err := BuildCaseDisclosureMap(casePackage, wrongSummary); err == nil || !strings.Contains(err.Error(), "case identity") {
		t.Fatalf("mismatched summary error = %v", err)
	}
	if _, err := BuildCaseDisclosureMap(CasePackage{}, summary); err == nil {
		t.Fatal("invalid case package was accepted")
	}
}
