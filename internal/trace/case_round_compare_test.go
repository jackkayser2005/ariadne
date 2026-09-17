package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCompareCaseQuestionRounds(t *testing.T) {
	firstPath := writeComparisonCaseRound(t, "android-experiment-001", nil)
	changedPath := writeComparisonCaseRound(t, "android-experiment-001", func(round *CaseQuestionRound) {
		for index := range round.Answers {
			round.Answers[index].Sources[0].Entries++
			round.Answers[index].Outcomes[0].Outcome = NoChangeObserved
		}
	})

	comparison, err := CompareCaseQuestionRounds(firstPath, changedPath)
	if err != nil {
		t.Fatalf("CompareCaseQuestionRounds() error = %v", err)
	}
	if comparison.Result != "changed" || comparison.OrderBasis != "caller" || comparison.Compared != len(CaseQuestions()) || comparison.Changed != 2 || !ValidSHA256(comparison.FirstRoundSHA256) || !ValidSHA256(comparison.SecondRoundSHA256) {
		t.Fatalf("comparison = %#v", comparison)
	}
	if len(comparison.ChangedQuestions) != 2 {
		t.Fatalf("changed questions = %#v", comparison.ChangedQuestions)
	}
	if got := comparison.ChangedQuestions[0]; got.QuestionID != CaseQuestionSources || !slices.Equal(got.ChangeKinds, []string{"sources"}) {
		t.Fatalf("source change = %#v", got)
	}
	if got := comparison.ChangedQuestions[1]; got.QuestionID != CaseQuestionOutcomes || got.FirstResult != "available" || got.SecondResult != "available" || got.FirstEvidenceState != evidence.Observed || got.SecondEvidenceState != evidence.Observed || !slices.Equal(got.ChangeKinds, []string{"outcomes"}) {
		t.Fatalf("outcome change = %#v", got)
	}

	metricsPath := writeComparisonCaseRound(t, "android-experiment-001", func(round *CaseQuestionRound) {
		for index := range round.Answers {
			round.Answers[index].UnknownEntries = 1
			round.Answers[index].EvidenceState = evidence.Unknown
		}
		round.Answers[2].Result = "unknown"
		round.Answers[2].Reason = caseAnswerReason(round.Answers[2])
	})
	metrics, err := CompareCaseQuestionRounds(firstPath, metricsPath)
	if err != nil || metrics.Changed != 3 {
		t.Fatalf("metric changes = %#v, %v", metrics, err)
	}
	if !slices.Equal(metrics.ChangedQuestions[0].ChangeKinds, []string{"evidence-state", "counts"}) || !slices.Equal(metrics.ChangedQuestions[1].ChangeKinds, []string{"evidence-state", "counts"}) || !slices.Equal(metrics.ChangedQuestions[2].ChangeKinds, []string{"result", "evidence-state", "counts"}) {
		t.Fatalf("metric change kinds = %#v", metrics.ChangedQuestions)
	}

	nestedEvidencePath := writeComparisonCaseRound(t, "android-experiment-001", func(round *CaseQuestionRound) {
		for index := range round.Answers {
			round.Answers[index].Outcomes[0].EvidenceState = evidence.Unknown
		}
	})
	nestedEvidence, err := CompareCaseQuestionRounds(firstPath, nestedEvidencePath)
	if err != nil || nestedEvidence.Changed != 1 || !slices.Equal(nestedEvidence.ChangedQuestions[0].ChangeKinds, []string{"evidence-state"}) {
		t.Fatalf("nested evidence change = %#v, %v", nestedEvidence, err)
	}

	nestedCountsPath := writeComparisonCaseRound(t, "android-experiment-001", func(round *CaseQuestionRound) {
		for index := range round.Answers {
			round.Answers[index].Outcomes[0].Pairs++
		}
	})
	nestedCounts, err := CompareCaseQuestionRounds(firstPath, nestedCountsPath)
	if err != nil || nestedCounts.Changed != 1 || !slices.Equal(nestedCounts.ChangedQuestions[0].ChangeKinds, []string{"counts"}) {
		t.Fatalf("nested count change = %#v, %v", nestedCounts, err)
	}

	same, err := CompareCaseQuestionRounds(firstPath, firstPath)
	if err != nil || same.Result != "same" || same.Changed != 0 || len(same.ChangedQuestions) != 0 {
		t.Fatalf("same comparison = %#v, %v", same, err)
	}

	reversed, err := CompareCaseQuestionRounds(changedPath, firstPath)
	if err != nil || len(reversed.ChangedQuestions) != 2 || reversed.ChangedQuestions[0].QuestionID != CaseQuestionSources || reversed.ChangedQuestions[1].FirstResult != "available" {
		t.Fatalf("reversed comparison = %#v, %v", reversed, err)
	}

	encoded, err := json.Marshal(same)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "reason") || strings.Contains(string(encoded), firstPath) {
		t.Fatalf("comparison exposed ignored data: %s", encoded)
	}

	otherCasePath := writeComparisonCaseRound(t, "browser-redacted-audit", nil)
	otherCase, err := CompareCaseQuestionRounds(firstPath, otherCasePath)
	if err != nil || otherCase.FirstCaseSHA256 == otherCase.SecondCaseSHA256 || otherCase.Result != "changed" {
		t.Fatalf("different case comparison = %#v, %v", otherCase, err)
	}
}

func TestCompareCaseQuestionRoundsRejectsInvalidInput(t *testing.T) {
	validPath := writeComparisonCaseRound(t, "android-experiment-001", nil)
	if _, err := CompareCaseQuestionRounds(" ", validPath); err == nil || !strings.Contains(err.Error(), "first question round") {
		t.Fatalf("empty first path error = %v", err)
	}

	root := t.TempDir()
	badPath := filepath.Join(root, "bad-round.json")
	if err := os.WriteFile(badPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, badPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("malformed second path error = %v", err)
	}

	oversizedPath := filepath.Join(root, "oversized-round.json")
	if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), maxCaseBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, oversizedPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("oversized second path error = %v", err)
	}

	round, _, err := ReadCaseQuestionRound(validPath)
	if err != nil {
		t.Fatal(err)
	}
	round.Answers[0].QuestionID = CaseQuestionSupport
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	tamperedPath := filepath.Join(root, "tampered-round.json")
	if err := os.WriteFile(tamperedPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, tamperedPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("tampered second path error = %v", err)
	}

	round, _, err = ReadCaseQuestionRound(validPath)
	if err != nil {
		t.Fatal(err)
	}
	round.Answers[1].Outcomes[0].Outcome = "private-value"
	data, err = json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	nestedPath := filepath.Join(root, "nested-invalid-round.json")
	if err := os.WriteFile(nestedPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, nestedPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("invalid nested outcome error = %v", err)
	}

	round, _, err = ReadCaseQuestionRound(validPath)
	if err != nil {
		t.Fatal(err)
	}
	round.Answers[0].Sources[0].Entries++
	data, err = json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	inconsistentPath := filepath.Join(root, "inconsistent-round.json")
	if err := os.WriteFile(inconsistentPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, inconsistentPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("inconsistent answer error = %v", err)
	}

	round, _, err = ReadCaseQuestionRound(validPath)
	if err != nil {
		t.Fatal(err)
	}
	round.Answers[0].Reason = "private-value"
	data, err = json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	reasonPath := filepath.Join(root, "invalid-reason-round.json")
	if err := os.WriteFile(reasonPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareCaseQuestionRounds(validPath, reasonPath); err == nil || !strings.Contains(err.Error(), "second question round") {
		t.Fatalf("invalid reason error = %v", err)
	}
}

func writeComparisonCaseRound(t *testing.T, archiveAdapter string, mutate func(*CaseQuestionRound)) string {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("a", 64)
	archiveTrace := validArchiveTrace("region")
	if archiveAdapter == "browser-redacted-audit" {
		archiveTrace.Events[0].Source = "browser"
	}
	archiveInputs := []ArchiveInput{
		writeStandaloneArchiveInputWithAdapter(t, root, "case-archive-first", archiveTrace, archiveAdapter, procedure),
		writeStandaloneArchiveInputWithAdapter(t, root, "case-archive-second", archiveTrace, archiveAdapter, procedure),
	}
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive(archiveInputs, archivePath); err != nil {
		t.Fatal(err)
	}
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}
	ledgerPath, ledgerRoundPath := writeCaseLedger(t)
	casePath := filepath.Join(root, "case.json")
	if _, err := SaveCase([]CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}, casePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "case-round.json")
	if _, err := SaveCaseQuestionRound(casePath, roundPath); err != nil {
		t.Fatal(err)
	}
	if mutate == nil {
		return roundPath
	}
	round, _, err := ReadCaseQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&round)
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(roundPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return roundPath
}
