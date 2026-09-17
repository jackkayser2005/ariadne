package trace

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCaseDisclosureQuestionCountsSameSourceAdapterBoundaries(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("f", 64)
	firstDocument := validArchiveTrace("region")
	firstDocument.Events[0].Source = "browser"
	secondDocument := validArchiveTrace("region")
	secondDocument.Events[0].Source = "browser"
	first := writeStandaloneArchiveInputWithAdapter(t, root, "first", firstDocument, "browser-redacted-audit", procedure)
	second := writeStandaloneArchiveInputWithAdapter(t, root, "second", secondDocument, "browser-local-fixture", procedure)
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{first, second}, archivePath); err != nil {
		t.Fatal(err)
	}
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(root, "case.json")
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, casePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePath)
	if err != nil {
		t.Fatal(err)
	}
	round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	answer := round.Answers[1]
	if answer.QuestionID != CaseDisclosureQuestionOverlap || answer.Result != "overlap-observed" || answer.EvidenceState != evidence.Observed || !strings.EqualFold(answer.OverlappingCategories[0], "region") {
		t.Fatalf("same-source adapter overlap answer = %#v", answer)
	}
	if len(answer.Categories) != 1 || len(answer.Categories[0].Boundaries) != 2 || answer.Categories[0].Boundaries[0].Source != "browser" || answer.Categories[0].Boundaries[1].Source != "browser" {
		t.Fatalf("same-source adapter boundaries = %#v", answer.Categories)
	}
}
