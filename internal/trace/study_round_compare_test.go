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

func TestCompareReplicationStudyQuestionRounds(t *testing.T) {
	firstStudyPath, firstRoundPath := writeComparableStudy(t, strings.Repeat("a", 64), "browser-redacted-audit", true, true, []string{"session-id", "location"})
	changedStudyPath, changedRoundPath := writeComparableStudy(t, strings.Repeat("a", 64), "browser-redacted-audit", false, false, []string{"session-id", "location"})

	comparison, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, changedStudyPath, changedRoundPath)
	if err != nil {
		t.Fatalf("CompareReplicationStudyQuestionRounds() error = %v", err)
	}
	if comparison.SchemaVersion != replicationStudyQuestionRoundComparisonSchemaVersion || comparison.ComparisonID != replicationStudyQuestionRoundComparisonID || comparison.ComparisonQuestion != replicationStudyQuestionRoundComparisonText || comparison.OrderBasis != ReplicationStudyOrderBasis || comparison.Result != studyRoundComparisonChanged || comparison.Compared != len(ReplicationStudyQuestions()) || comparison.Changed != 3 || !ValidSHA256(comparison.FirstRoundSHA256) || !ValidSHA256(comparison.SecondRoundSHA256) || !ValidSHA256(comparison.FirstStudySHA256) || !ValidSHA256(comparison.SecondStudySHA256) {
		t.Fatalf("comparison = %#v", comparison)
	}
	if len(comparison.ChangedQuestions) != 3 {
		t.Fatalf("changed questions = %#v", comparison.ChangedQuestions)
	}
	if got := comparison.ChangedQuestions[0]; got.QuestionID != StudyQuestionOutcome || got.FirstResult != string(ReplicatedChange) || got.SecondResult != string(ReplicationUnknown) || got.FirstEvidenceState != evidence.Observed || got.SecondEvidenceState != evidence.Unknown || got.FirstOutcome != ReplicatedChange || got.SecondOutcome != ReplicationUnknown || !slices.Equal(got.ChangeKinds, []string{"result", "outcome", "evidence-state", "support-counts"}) {
		t.Fatalf("outcome change = %#v", got)
	}

	same, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, firstStudyPath, firstRoundPath)
	if err != nil || same.Result != studyRoundComparisonSame || same.Changed != 0 || same.Compared != len(ReplicationStudyQuestions()) || len(same.ChangedQuestions) != 0 {
		t.Fatalf("same comparison = %#v, %v", same, err)
	}

	reversed, err := CompareReplicationStudyQuestionRounds(changedStudyPath, changedRoundPath, firstStudyPath, firstRoundPath)
	if err != nil || reversed.Result != studyRoundComparisonChanged || reversed.ChangedQuestions[0].FirstResult != string(ReplicationUnknown) || reversed.ChangedQuestions[0].SecondResult != string(ReplicatedChange) {
		t.Fatalf("reversed comparison = %#v, %v", reversed, err)
	}

	encoded, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	encodedString := string(encoded)
	for _, private := range []string{firstStudyPath, firstRoundPath, changedStudyPath, changedRoundPath, strings.Repeat("a", 64), "private-value", "https://"} {
		if strings.Contains(encodedString, private) {
			t.Fatalf("comparison exposed private detail %q: %s", private, encodedString)
		}
	}
}

func TestCompareReplicationStudyQuestionRoundsDistinguishesSupportCounts(t *testing.T) {
	firstStudyPath, firstRoundPath := writeComparableStudy(t, strings.Repeat("b", 64), "browser-redacted-audit", true, true, []string{"session-id", "location"})
	secondStudyPath, secondRoundPath := writeComparableStudy(t, strings.Repeat("b", 64), "browser-redacted-audit", true, true, []string{"session-id", "location", "account-id"})

	comparison, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, secondStudyPath, secondRoundPath)
	if err != nil || comparison.Result != studyRoundComparisonChanged || comparison.Changed != len(ReplicationStudyQuestions()) {
		t.Fatalf("support comparison = %#v, %v", comparison, err)
	}
	for _, change := range comparison.ChangedQuestions {
		if !slices.Equal(change.ChangeKinds, []string{"support-counts"}) {
			t.Fatalf("support change = %#v", change)
		}
	}
}

func TestCompareReplicationStudyQuestionRoundsReturnsIncomparable(t *testing.T) {
	firstStudyPath, firstRoundPath := writeComparableStudy(t, strings.Repeat("c", 64), "browser-redacted-audit", true, true, []string{"session-id", "location"})
	contrastStudyPath, contrastRoundPath := writeComparableStudy(t, strings.Repeat("d", 64), "browser-redacted-audit", true, true, []string{"session-id", "location"})
	contrast, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, contrastStudyPath, contrastRoundPath)
	if err != nil || contrast.Result != studyRoundComparisonIncomparable || contrast.IncomparableReason != "counterfactual commitments differ" || contrast.Compared != 0 || contrast.Changed != 0 || len(contrast.ChangedQuestions) != 0 {
		t.Fatalf("contrast incompatibility = %#v, %v", contrast, err)
	}

	provenanceStudyPath, provenanceRoundPath := writeComparableStudy(t, strings.Repeat("c", 64), "browser-local-fixture", true, true, []string{"session-id", "location"})
	provenance, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, provenanceStudyPath, provenanceRoundPath)
	if err != nil || provenance.Result != studyRoundComparisonIncomparable || provenance.IncomparableReason != "reviewed source provenance differs" {
		t.Fatalf("provenance incompatibility = %#v, %v", provenance, err)
	}
}

func TestCompareReplicationStudyQuestionRoundsRejectsUnboundAndMalformedRounds(t *testing.T) {
	firstStudyPath, firstRoundPath := writeComparableStudy(t, strings.Repeat("e", 64), "browser-redacted-audit", true, true, []string{"session-id", "location"})
	secondStudyPath, secondRoundPath := writeComparableStudy(t, strings.Repeat("e", 64), "browser-redacted-audit", true, true, []string{"device-id", "location"})

	if _, err := CompareReplicationStudyQuestionRounds(" ", firstRoundPath, secondStudyPath, secondRoundPath); err == nil || !strings.Contains(err.Error(), "first study question round") {
		t.Fatalf("empty first study error = %v", err)
	}
	if _, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, filepath.Join(t.TempDir(), "missing.json"), secondRoundPath); err == nil || !strings.Contains(err.Error(), "second study question round") {
		t.Fatalf("missing second study error = %v", err)
	}

	wrongRoundPath := filepath.Join(t.TempDir(), "wrong-round.json")
	wrongRound, _, err := ReadReplicationStudyQuestionRound(secondRoundPath)
	if err != nil {
		t.Fatal(err)
	}
	wrongRound.StudySHA256 = strings.Repeat("e", 64)
	for index := range wrongRound.Answers {
		wrongRound.Answers[index].StudySHA256 = wrongRound.StudySHA256
	}
	data, err := json.Marshal(wrongRound)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrongRoundPath, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, secondStudyPath, wrongRoundPath); err == nil || !strings.Contains(err.Error(), "second study question round") {
		t.Fatalf("unbound round error = %v", err)
	}

	badRoundPath := filepath.Join(t.TempDir(), "bad-round.json")
	if err := os.WriteFile(badRoundPath, []byte(`{"schema_version":1,"answers":[{"reason":"private-value"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, secondStudyPath, badRoundPath); err == nil || !strings.Contains(err.Error(), "second study question round") {
		t.Fatalf("malformed round error = %v", err)
	}

	oversizedRoundPath := filepath.Join(t.TempDir(), "oversized-round.json")
	if err := os.WriteFile(oversizedRoundPath, bytes.Repeat([]byte("x"), maxArchiveBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareReplicationStudyQuestionRounds(firstStudyPath, firstRoundPath, secondStudyPath, oversizedRoundPath); err == nil || !strings.Contains(err.Error(), "second study question round") {
		t.Fatalf("oversized round error = %v", err)
	}
}

func writeComparableStudy(t *testing.T, contrast, adapter string, firstChanged, firstReset bool, variants []string) (string, string) {
	t.Helper()
	if len(variants) < 2 {
		t.Fatal("comparable study requires at least two variants")
	}
	inputs := make([]StudyInput, 0, len(variants))
	for index, variant := range variants {
		ledgerPath, roundPath := writeStudyFixture(t, variant, firstChanged || index > 0, firstReset || index > 0, adapter)
		inputs = append(inputs, StudyInput{LedgerPath: ledgerPath, RoundPath: roundPath})
	}
	studyPath := filepath.Join(t.TempDir(), "study.json")
	if _, err := SaveReplicationStudy(contrast, inputs, studyPath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveReplicationStudyQuestionRound(studyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	return studyPath, roundPath
}
