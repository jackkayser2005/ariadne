package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReplicationStudyQuestionRoundLifecycle(t *testing.T) {
	studyPath, study, summary := validReplicationStudyForRound(t, true, true)
	roundPath := filepath.Join(t.TempDir(), "study-round.json")
	roundSummary, err := SaveReplicationStudyQuestionRound(studyPath, roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if roundSummary.SchemaVersion != replicationStudyQuestionRoundSchemaVersion || roundSummary.StudySHA256 != summary.StudySHA256 || roundSummary.Questions != len(ReplicationStudyQuestions()) || !ValidSHA256(roundSummary.RoundSHA256) {
		t.Fatalf("round summary = %#v", roundSummary)
	}
	round, verified, err := ReadReplicationStudyQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if verified != roundSummary || round.StudySHA256 != summary.StudySHA256 || len(round.Answers) != 3 || round.Answers[0].Result != string(ReplicatedChange) || round.Answers[0].EvidenceState != evidence.Observed {
		t.Fatalf("round = %#v, summary = %#v", round, verified)
	}
	if got, err := VerifyReplicationStudyQuestionRound(roundPath); err != nil || got != roundSummary {
		t.Fatalf("VerifyReplicationStudyQuestionRound() = %#v, %v", got, err)
	}
	if answer, err := AskReplicationStudyQuestionRound(roundPath, StudyQuestionSupport); err != nil || answer.Result != "supported" || answer.StudySHA256 != summary.StudySHA256 {
		t.Fatalf("AskReplicationStudyQuestionRound() = %#v, %v", answer, err)
	}
	receiptPath := filepath.Join(t.TempDir(), "study-receipt.json")
	receiptSummary, err := SaveReplicationStudyQuestionReceipt(roundPath, StudyQuestionOutcome, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if receiptSummary.QuestionID != StudyQuestionOutcome || receiptSummary.Result != string(ReplicatedChange) || receiptSummary.RoundSHA256 != roundSummary.RoundSHA256 || !ValidSHA256(receiptSummary.ReceiptSHA256) {
		t.Fatalf("receipt summary = %#v", receiptSummary)
	}
	receipt, verifiedReceipt, err := ReadReplicationStudyQuestionReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if verifiedReceipt != receiptSummary || receipt.RoundSHA256 != roundSummary.RoundSHA256 || receipt.Round.StudySHA256 != summary.StudySHA256 {
		t.Fatalf("receipt = %#v, summary = %#v", receipt, verifiedReceipt)
	}
	if got, err := VerifyReplicationStudyQuestionReceipt(receiptPath); err != nil || got != receiptSummary {
		t.Fatalf("VerifyReplicationStudyQuestionReceipt() = %#v, %v", got, err)
	}
	if selected, err := AskReplicationStudyQuestionReceipt(roundPath, StudyQuestionOutcome); err != nil || !reflect.DeepEqual(selected, receipt) {
		t.Fatalf("AskReplicationStudyQuestionReceipt() = %#v, %v; want %#v", selected, err, receipt)
	}
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), studyPath) || strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "target-device") {
		t.Fatal("study receipt persisted a path or unsafe value")
	}
	if _, err := SaveReplicationStudyQuestionRound(studyPath, roundPath); err == nil {
		t.Fatal("SaveReplicationStudyQuestionRound() overwrote an existing round")
	}
	if _, err := SaveReplicationStudyQuestionReceipt(roundPath, StudyQuestionOutcome, receiptPath); err == nil {
		t.Fatal("SaveReplicationStudyQuestionReceipt() overwrote an existing receipt")
	}
	if _, err := AnswerReplicationStudyQuestionRound(study, summary); err != nil {
		t.Fatalf("AnswerReplicationStudyQuestionRound() = %v", err)
	}
}

func TestReplicationStudyQuestionRoundPreservesUnknownSemantics(t *testing.T) {
	studyPath, _, _ := validReplicationStudyForRound(t, true, false)
	roundPath := filepath.Join(t.TempDir(), "unknown-round.json")
	if _, err := SaveReplicationStudyQuestionRound(studyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	if _, err := AskAllReplicationStudyQuestions(roundPath); err == nil {
		t.Fatal("AskAllReplicationStudyQuestions() accepted a round path")
	}
	round, _, err := ReadReplicationStudyQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if round.Answers[0].Result != string(ReplicationUnknown) || round.Answers[0].EvidenceState != evidence.Unknown || round.Answers[1].Result != "unknown" || round.Answers[2].Result != "unknown" {
		t.Fatalf("unknown answers = %#v", round.Answers)
	}
	if _, err := SaveReplicationStudyQuestionReceipt(roundPath, StudyQuestionConsistency, filepath.Join(t.TempDir(), "unknown-receipt.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeReplicationStudyQuestionRoundRejectsTamperingAndBounds(t *testing.T) {
	round := validReplicationStudyQuestionRound(t, true, true)
	valid, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationStudyQuestionRound(valid); err != nil {
		t.Fatalf("DecodeReplicationStudyQuestionRound(valid) = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ReplicationStudyQuestionRound)
	}{
		{name: "schema", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.SchemaVersion = 2 }},
		{name: "study identity", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.StudySHA256 = strings.Repeat("z", 64) }},
		{name: "answers missing", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers = candidate.Answers[:1] }},
		{name: "answer identity", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[0].QuestionID = StudyQuestionSupport }},
		{name: "answer study identity", mutate: func(candidate *ReplicationStudyQuestionRound) {
			candidate.Answers[0].StudySHA256 = strings.Repeat("e", 64)
		}},
		{name: "answer counts", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[0].Pairs++ }},
		{name: "balanced runs", mutate: func(candidate *ReplicationStudyQuestionRound) {
			for index := range candidate.Answers {
				candidate.Answers[index].BalancedRuns = 0
			}
		}},
		{name: "answer evidence", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[0].EvidenceState = evidence.Inferred }},
		{name: "answer result", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[0].Result = "unsupported" }},
		{name: "answer reason", mutate: func(candidate *ReplicationStudyQuestionRound) {
			candidate.Answers[0].Reason = `C:\\private\\captured-value`
		}},
		{name: "answer outcome", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[0].Outcome = ReplicationUnknown }},
		{name: "metrics disagree", mutate: func(candidate *ReplicationStudyQuestionRound) { candidate.Answers[1].Pairs++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneStudyQuestionRound(round)
			test.mutate(&candidate)
			if _, err := ReplicationStudyQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("ReplicationStudyQuestionRoundSHA256() accepted invalid round")
			}
			data, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, err := DecodeReplicationStudyQuestionRound(data); err == nil {
				t.Fatal("DecodeReplicationStudyQuestionRound() accepted tampering")
			}
		})
	}
	if _, err := DecodeReplicationStudyQuestionRound(nil); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionRound() accepted empty data")
	}
	if _, err := DecodeReplicationStudyQuestionRound(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionRound() accepted oversized data")
	}
	if _, err := DecodeReplicationStudyQuestionRound([]byte(`{"schema_version":1,"schema_version":1}`)); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionRound() accepted duplicate keys")
	}
	if _, err := DecodeReplicationStudyQuestionRound([]byte(`{"schema_version":1} trailing`)); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionRound() accepted trailing data")
	}
	if _, err := ReplicationStudyQuestionRoundSHA256(ReplicationStudyQuestionRound{}); err == nil {
		t.Fatal("ReplicationStudyQuestionRoundSHA256() accepted invalid round")
	}
}

func TestDecodeReplicationStudyQuestionReceiptRejectsTamperingAndBounds(t *testing.T) {
	round := validReplicationStudyQuestionRound(t, true, true)
	roundSHA256, err := ReplicationStudyQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ReplicationStudyQuestionReceipt{
		StudyQuestionAnswer: round.Answers[0],
		RoundSHA256:         roundSHA256,
		Round:               round,
	}
	valid, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationStudyQuestionReceipt(valid); err != nil {
		t.Fatalf("DecodeReplicationStudyQuestionReceipt(valid) = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ReplicationStudyQuestionReceipt)
	}{
		{name: "schema", mutate: func(candidate *ReplicationStudyQuestionReceipt) { candidate.SchemaVersion = 2 }},
		{name: "question", mutate: func(candidate *ReplicationStudyQuestionReceipt) { candidate.QuestionID = "not-a-question" }},
		{name: "answer result", mutate: func(candidate *ReplicationStudyQuestionReceipt) { candidate.Result = "unsupported" }},
		{name: "round identity", mutate: func(candidate *ReplicationStudyQuestionReceipt) { candidate.RoundSHA256 = strings.Repeat("z", 64) }},
		{name: "round answer", mutate: func(candidate *ReplicationStudyQuestionReceipt) {
			candidate.Round.Answers[0].Reason = `C:\\private\\captured-value`
		}},
		{name: "selected answer", mutate: func(candidate *ReplicationStudyQuestionReceipt) { candidate.Result = string(NoChangeObserved) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneStudyQuestionReceipt(receipt)
			test.mutate(&candidate)
			data, marshalErr := json.Marshal(candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, err := DecodeReplicationStudyQuestionReceipt(data); err == nil {
				t.Fatal("DecodeReplicationStudyQuestionReceipt() accepted tampering")
			}
		})
	}
	if _, err := DecodeReplicationStudyQuestionReceipt(nil); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionReceipt() accepted empty data")
	}
	if _, err := DecodeReplicationStudyQuestionReceipt(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionReceipt() accepted oversized data")
	}
	if _, err := DecodeReplicationStudyQuestionReceipt([]byte(`{"schema_version":1,"question_id":"study-outcome"}`)); err == nil {
		t.Fatal("DecodeReplicationStudyQuestionReceipt() accepted incomplete data")
	}
	if _, err := ReplicationStudyQuestionReceiptSHA256(ReplicationStudyQuestionReceipt{}); err == nil {
		t.Fatal("ReplicationStudyQuestionReceiptSHA256() accepted invalid receipt")
	}
}

func TestReplicationStudyQuestionRoundPathsAndHelpers(t *testing.T) {
	studyPath, _, _ := validReplicationStudyForRound(t, true, true)
	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveReplicationStudyQuestionRound(studyPath, roundPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", filepath.Join(t.TempDir(), "missing.json")} {
		if _, _, err := ReadReplicationStudyQuestionRound(path); err == nil {
			t.Errorf("ReadReplicationStudyQuestionRound(%q) accepted path", path)
		}
		if _, err := VerifyReplicationStudyQuestionRound(path); err == nil {
			t.Errorf("VerifyReplicationStudyQuestionRound(%q) accepted path", path)
		}
	}
	if _, err := SaveReplicationStudyQuestionRound(studyPath, " "); err == nil {
		t.Fatal("SaveReplicationStudyQuestionRound() accepted an empty output path")
	}
	if _, err := SaveReplicationStudyQuestionRound(filepath.Join(t.TempDir(), "missing-study.json"), filepath.Join(t.TempDir(), "round.json")); err == nil {
		t.Fatal("SaveReplicationStudyQuestionRound() accepted a missing study")
	}
	if _, err := AskReplicationStudyQuestionRound(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskReplicationStudyQuestionRound() accepted an invalid question")
	}
	if _, err := AskReplicationStudyQuestionReceipt(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskReplicationStudyQuestionReceipt() accepted an invalid question")
	}
	if _, err := SaveReplicationStudyQuestionReceipt(filepath.Join(t.TempDir(), "missing-round.json"), StudyQuestionOutcome, filepath.Join(t.TempDir(), "receipt.json")); err == nil {
		t.Fatal("SaveReplicationStudyQuestionReceipt() accepted a missing round")
	}
	if _, err := SaveReplicationStudyQuestionReceipt(roundPath, StudyQuestionOutcome, " "); err == nil {
		t.Fatal("SaveReplicationStudyQuestionReceipt() accepted an empty output path")
	}
	if _, _, err := ReadReplicationStudyQuestionReceipt(""); err == nil {
		t.Fatal("ReadReplicationStudyQuestionReceipt() accepted an empty path")
	}
	if _, err := VerifyReplicationStudyQuestionReceipt(filepath.Join(t.TempDir(), "missing-receipt.json")); err == nil {
		t.Fatal("VerifyReplicationStudyQuestionReceipt() accepted a missing receipt")
	}
	if _, err := AnswerReplicationStudyQuestionRound(ReplicationStudy{}, StudyVerificationSummary{}); err == nil {
		t.Fatal("AnswerReplicationStudyQuestionRound() accepted an invalid study")
	}
	if _, ok := replicationStudyQuestionIndex("not-a-question"); ok {
		t.Fatal("replicationStudyQuestionIndex() accepted an invalid ID")
	}
}

func validReplicationStudyQuestionRound(t *testing.T, firstChanged, firstReset bool) ReplicationStudyQuestionRound {
	t.Helper()
	studyPath, study, summary := validReplicationStudyForRound(t, firstChanged, firstReset)
	_ = studyPath
	round, err := AnswerReplicationStudyQuestionRound(study, summary)
	if err != nil {
		t.Fatal(err)
	}
	return round
}

func validReplicationStudyForRound(t *testing.T, firstChanged, firstReset bool) (string, ReplicationStudy, StudyVerificationSummary) {
	t.Helper()
	firstLedger, firstRound := writeStudyFixture(t, "session-id", firstChanged, firstReset, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	studyPath := filepath.Join(t.TempDir(), "study.json")
	if _, err := SaveReplicationStudy(strings.Repeat("a", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}, studyPath); err != nil {
		t.Fatal(err)
	}
	study, summary, err := ReadReplicationStudy(studyPath)
	if err != nil {
		t.Fatal(err)
	}
	return studyPath, study, summary
}

func cloneStudyQuestionRound(round ReplicationStudyQuestionRound) ReplicationStudyQuestionRound {
	data, _ := json.Marshal(round)
	var clone ReplicationStudyQuestionRound
	_ = json.Unmarshal(data, &clone)
	return clone
}

func cloneStudyQuestionReceipt(receipt ReplicationStudyQuestionReceipt) ReplicationStudyQuestionReceipt {
	data, _ := json.Marshal(receipt)
	var clone ReplicationStudyQuestionReceipt
	_ = json.Unmarshal(data, &clone)
	return clone
}
