package trace

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReplicationStudyQuestionsClassifyVerifiedStudies(t *testing.T) {
	tests := []struct {
		name            string
		firstChanged    bool
		secondChanged   bool
		firstReset      bool
		wantOutcome     ReplicatedOutcome
		wantSupport     string
		wantConsistency string
		wantEvidence    evidence.State
	}{
		{name: "changed", firstChanged: true, secondChanged: true, firstReset: true, wantOutcome: ReplicatedChange, wantSupport: "supported", wantConsistency: "consistent", wantEvidence: evidence.Observed},
		{name: "no change", firstChanged: false, secondChanged: false, firstReset: true, wantOutcome: NoChangeObserved, wantSupport: "supported", wantConsistency: "consistent", wantEvidence: evidence.Observed},
		{name: "mixed", firstChanged: true, secondChanged: false, firstReset: true, wantOutcome: MixedInconsistent, wantSupport: "supported", wantConsistency: "inconsistent", wantEvidence: evidence.Observed},
		{name: "unknown", firstChanged: true, secondChanged: true, firstReset: false, wantOutcome: ReplicationUnknown, wantSupport: "unknown", wantConsistency: "unknown", wantEvidence: evidence.Unknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstLedger, firstRound := writeStudyFixture(t, "session-id", test.firstChanged, test.firstReset, "browser-redacted-audit")
			secondLedger, secondRound := writeStudyFixture(t, "location", test.secondChanged, true, "browser-redacted-audit")
			studyPath := filepath.Join(t.TempDir(), "study.json")
			if _, err := SaveReplicationStudy(strings.Repeat("1", 64), []StudyInput{
				{LedgerPath: firstLedger, RoundPath: firstRound},
				{LedgerPath: secondLedger, RoundPath: secondRound},
			}, studyPath); err != nil {
				t.Fatal(err)
			}
			study, summary, err := ReadReplicationStudy(studyPath)
			if err != nil {
				t.Fatal(err)
			}
			answers, err := AnswerAllReplicationStudyQuestions(study, summary)
			if err != nil {
				t.Fatal(err)
			}
			if len(answers) != 3 || answers[0].Result != string(test.wantOutcome) || answers[0].EvidenceState != test.wantEvidence || answers[1].Result != test.wantSupport || answers[2].Result != test.wantConsistency {
				t.Fatalf("answers = %#v", answers)
			}
			for _, answer := range answers {
				if answer.StudySHA256 != summary.StudySHA256 || answer.Outcome != summary.Outcome || answer.Runs != summary.Runs || answer.Pairs != summary.Pairs {
					t.Fatalf("answer identity/metrics = %#v, summary = %#v", answer, summary)
				}
			}
			if answer, err := AskReplicationStudyQuestion(studyPath, StudyQuestionOutcome); err != nil || answer.Result != string(test.wantOutcome) {
				t.Fatalf("AskReplicationStudyQuestion() = %#v, %v", answer, err)
			}
			if asked, err := AskAllReplicationStudyQuestions(studyPath); err != nil || len(asked) != 3 {
				t.Fatalf("AskAllReplicationStudyQuestions() = %#v, %v", asked, err)
			}
		})
	}
}

func TestReplicationStudyQuestionsRejectInvalidIdentityAndIDs(t *testing.T) {
	firstLedger, firstRound := writeStudyFixture(t, "session-id", true, true, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	studyPath := filepath.Join(t.TempDir(), "study.json")
	if _, err := SaveReplicationStudy(strings.Repeat("2", 64), []StudyInput{
		{LedgerPath: firstLedger, RoundPath: firstRound},
		{LedgerPath: secondLedger, RoundPath: secondRound},
	}, studyPath); err != nil {
		t.Fatal(err)
	}
	study, summary, err := ReadReplicationStudy(studyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerReplicationStudyQuestion(study, summary, "not-a-question"); err == nil {
		t.Fatal("AnswerReplicationStudyQuestion() accepted an invalid ID")
	}
	if _, err := AnswerReplicationStudyQuestion(study, StudyVerificationSummary{}, StudyQuestionOutcome); err == nil {
		t.Fatal("AnswerReplicationStudyQuestion() accepted an invalid summary")
	}
	if _, err := AnswerAllReplicationStudyQuestions(study, StudyVerificationSummary{}); err == nil {
		t.Fatal("AnswerAllReplicationStudyQuestions() accepted an invalid summary")
	}
	if _, err := AskReplicationStudyQuestion(filepath.Join(t.TempDir(), "missing.json"), StudyQuestionOutcome); err == nil {
		t.Fatal("AskReplicationStudyQuestion() accepted a missing study")
	}
	if _, err := AskAllReplicationStudyQuestions(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("AskAllReplicationStudyQuestions() accepted a missing study")
	}
	for _, question := range ReplicationStudyQuestions() {
		if question.ID == "" || question.Text == "" {
			t.Fatalf("invalid question = %#v", question)
		}
	}
	if _, ok := replicationStudyQuestion("not-a-question"); ok {
		t.Fatal("replicationStudyQuestion() accepted an invalid ID")
	}
}
