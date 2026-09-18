package trace

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReplicationQuestionRoundAndReceiptLifecycle(t *testing.T) {
	ledgerPath, ledger := writeQuestionLedger(t, true, true)
	_, ledgerSummary, err := ReadReplicationLedger(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(t.TempDir(), "round.json")
	roundSummary, err := SaveReplicationQuestionRound(ledgerPath, roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if roundSummary.SchemaVersion != 1 || roundSummary.LedgerSHA256 != ledgerSummary.LedgerSHA256 || roundSummary.Questions != len(ReplicationQuestions()) || !ValidSHA256(roundSummary.RoundSHA256) {
		t.Fatalf("round summary = %#v", roundSummary)
	}
	round, readSummary, err := ReadReplicationQuestionRound(roundPath)
	if err != nil || readSummary != roundSummary || len(round.Answers) != len(ReplicationQuestions()) {
		t.Fatalf("round = %#v, summary = %#v, err = %v", round, readSummary, err)
	}
	if verified, err := VerifyReplicationQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyReplicationQuestionRound() = %#v, %v", verified, err)
	}

	outcome, err := AskReplicationQuestionRound(roundPath, ReplicationQuestionOutcome)
	if err != nil || outcome.Result != string(ReplicatedChange) || outcome.EvidenceState != evidence.Observed || outcome.LedgerSHA256 != ledgerSummary.LedgerSHA256 {
		t.Fatalf("outcome answer = %#v, err = %v", outcome, err)
	}
	all, err := AskAllReplicationQuestions(ledgerPath)
	if err != nil || len(all) != len(ReplicationQuestions()) || all[0].QuestionID != ReplicationQuestionOutcome {
		t.Fatalf("all answers = %#v, err = %v", all, err)
	}
	direct, err := AskReplicationQuestion(ledgerPath, ReplicationQuestionOutcome)
	if err != nil || direct != outcome {
		t.Fatalf("AskReplicationQuestion() = %#v, %v; want %#v", direct, err, outcome)
	}
	fromSummary, err := AnswerAllReplicationQuestionsFromSummary(ledgerSummary)
	if err != nil || !reflect.DeepEqual(fromSummary, all) {
		t.Fatalf("AnswerAllReplicationQuestionsFromSummary() = %#v, %v; want %#v", fromSummary, err, all)
	}
	answer, err := AnswerReplicationQuestion(ledger, ledgerSummary, ReplicationQuestionSupport)
	if err != nil || answer.Result != "supported" || !strings.Contains(answer.Reason, "every retained pair") {
		t.Fatalf("support answer = %#v, err = %v", answer, err)
	}

	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	receiptSummary, err := SaveReplicationQuestionReceipt(roundPath, ReplicationQuestionConsistency, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if receiptSummary.QuestionID != ReplicationQuestionConsistency || receiptSummary.Result != "consistent" || !ValidSHA256(receiptSummary.ReceiptSHA256) {
		t.Fatalf("receipt summary = %#v", receiptSummary)
	}
	receipt, readReceiptSummary, err := ReadReplicationQuestionReceipt(receiptPath)
	if err != nil || readReceiptSummary != receiptSummary || receipt.RoundSHA256 != roundSummary.RoundSHA256 {
		t.Fatalf("receipt = %#v, summary = %#v, err = %v", receipt, readReceiptSummary, err)
	}
	if verified, err := VerifyReplicationQuestionReceipt(receiptPath); err != nil || verified != receiptSummary {
		t.Fatalf("VerifyReplicationQuestionReceipt() = %#v, %v", verified, err)
	}
	selected, err := AskReplicationQuestionReceipt(roundPath, ReplicationQuestionConsistency)
	if err != nil || !reflect.DeepEqual(selected, receipt) {
		t.Fatalf("AskReplicationQuestionReceipt() = %#v, %v; want %#v", selected, err, receipt)
	}
	if _, err := SaveReplicationQuestionRound(ledgerPath, roundPath); err == nil {
		t.Fatal("SaveReplicationQuestionRound() overwrote an existing round")
	}
	if _, err := SaveReplicationQuestionReceipt(roundPath, ReplicationQuestionConsistency, receiptPath); err == nil {
		t.Fatal("SaveReplicationQuestionReceipt() overwrote an existing receipt")
	}
	if _, err := AskReplicationQuestionRound(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskReplicationQuestionRound() accepted an invalid question")
	}
	if _, err := AskReplicationQuestionReceipt(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskReplicationQuestionReceipt() accepted an invalid question")
	}
}

func TestReplicationQuestionAnswersPreserveUnknownAndOutcome(t *testing.T) {
	tests := []struct {
		name             string
		firstChange      bool
		secondChange     bool
		firstReset       bool
		secondReset      bool
		wantOutcome      ReplicatedOutcome
		wantSupport      string
		wantConsistency  string
		wantEvidence     evidence.State
		wantReasonPhrase string
	}{
		{name: "mixed", firstChange: true, secondChange: false, firstReset: true, secondReset: true, wantOutcome: MixedInconsistent, wantSupport: "supported", wantConsistency: "inconsistent", wantEvidence: evidence.Observed, wantReasonPhrase: "disagree"},
		{name: "unknown reset", firstChange: true, secondChange: true, firstReset: false, secondReset: true, wantOutcome: ReplicationUnknown, wantSupport: "unknown", wantConsistency: "unknown", wantEvidence: evidence.Unknown, wantReasonPhrase: "cannot establish"},
		{name: "unknown order", firstChange: true, secondChange: false, firstReset: true, secondReset: true, wantOutcome: ReplicationUnknown, wantSupport: "unknown", wantConsistency: "unknown", wantEvidence: evidence.Unknown, wantReasonPhrase: "equal nonzero"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledgerPath, ledger := writeQuestionLedgerWithChanges(t, test.firstChange, test.secondChange, test.firstReset, test.secondReset, test.name != "unknown order")
			_, summary, err := ReadReplicationLedger(ledgerPath)
			if err != nil {
				t.Fatal(err)
			}
			answers, err := AnswerAllReplicationQuestions(ledger, summary)
			if err != nil {
				t.Fatal(err)
			}
			if answers[0].Result != string(test.wantOutcome) || answers[0].EvidenceState != test.wantEvidence || answers[1].Result != test.wantSupport || answers[2].Result != test.wantConsistency {
				t.Fatalf("answers = %#v, summary = %#v", answers, summary)
			}
			if !strings.Contains(answers[0].Reason, test.wantReasonPhrase) && !strings.Contains(answers[2].Reason, test.wantReasonPhrase) {
				t.Fatalf("answers did not retain reason %q: %#v", test.wantReasonPhrase, answers)
			}
		})
	}
	if _, err := AnswerReplicationQuestion(ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, ReplicationQuestionOutcome); err == nil {
		t.Fatal("AnswerReplicationQuestion() accepted an invalid ledger")
	}
	validPath, ledger := writeQuestionLedger(t, true, true)
	_, summary, err := ReadReplicationLedger(validPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerReplicationQuestion(ledger, summary, "not-a-question"); err == nil {
		t.Fatal("AnswerReplicationQuestion() accepted an invalid question")
	}
	wrong := summary
	wrong.LedgerSHA256 = strings.Repeat("e", 64)
	if _, err := AnswerReplicationQuestion(ledger, wrong, ReplicationQuestionOutcome); err == nil {
		t.Fatal("AnswerReplicationQuestion() accepted a mismatched summary")
	}
}

func TestDecodeReplicationQuestionRoundRejectsMalformedDocuments(t *testing.T) {
	round := validReplicationQuestionRound(t)
	valid, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty"},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown", data: append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"extra":true}`)...)},
		{name: "trailing", data: append(valid, []byte(` {}`)...)},
		{name: "invalid json", data: []byte(`{`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeReplicationQuestionRound(test.data); err == nil {
				t.Fatal("DecodeReplicationQuestionRound() accepted malformed input")
			}
		})
	}
	if _, err := DecodeReplicationQuestionRound(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeReplicationQuestionRound() accepted oversized input")
	}
	for _, test := range []struct {
		name   string
		mutate func(*ReplicationQuestionRound)
	}{
		{name: "schema", mutate: func(round *ReplicationQuestionRound) { round.SchemaVersion = 2 }},
		{name: "ledger identity", mutate: func(round *ReplicationQuestionRound) { round.LedgerSHA256 = strings.Repeat("z", 64) }},
		{name: "answers missing", mutate: func(round *ReplicationQuestionRound) { round.Answers = round.Answers[:1] }},
		{name: "answer identity", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].QuestionID = ReplicationQuestionSupport }},
		{name: "answer ledger", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].LedgerSHA256 = strings.Repeat("e", 64) }},
		{name: "answer counts", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].Pairs++ }},
		{name: "answer evidence", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].EvidenceState = evidence.Inferred }},
		{name: "answer result", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].Result = "unsupported" }},
		{name: "answer result mismatch", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].Result = string(NoChangeObserved) }},
		{name: "answer reason", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].Reason = `C:\\private\\captured-value` }},
		{name: "answer support metrics", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].ResetConfirmedPairs = 0 }},
		{name: "answer outcome metrics", mutate: func(round *ReplicationQuestionRound) {
			round.Answers[0].ChangedPairs = 0
			round.Answers[0].NoChangePairs = 2
		}},
		{name: "answer outcome", mutate: func(round *ReplicationQuestionRound) { round.Answers[0].Outcome = "unsupported" }},
		{name: "answer balance", mutate: func(round *ReplicationQuestionRound) {
			round.Answers[0].OrderBalanced = !round.Answers[0].OrderBalanced
		}},
		{name: "answer metrics disagree", mutate: func(round *ReplicationQuestionRound) { round.Answers[1].Pairs++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneReplicationQuestionRound(round)
			test.mutate(&candidate)
			if _, err := ReplicationQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("ReplicationQuestionRoundSHA256() accepted invalid round")
			}
		})
	}
}

func TestDecodeReplicationQuestionReceiptRejectsMalformedDocuments(t *testing.T) {
	round := validReplicationQuestionRound(t)
	roundSHA256, err := ReplicationQuestionRoundSHA256(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ReplicationQuestionReceipt{ReplicationAnswer: round.Answers[0], RoundSHA256: roundSHA256, Round: round}
	valid, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationQuestionReceipt(valid); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ReplicationQuestionReceipt)
	}{
		{name: "schema", mutate: func(receipt *ReplicationQuestionReceipt) { receipt.SchemaVersion = 2 }},
		{name: "question", mutate: func(receipt *ReplicationQuestionReceipt) { receipt.QuestionID = "not-a-question" }},
		{name: "answer result", mutate: func(receipt *ReplicationQuestionReceipt) { receipt.Result = "unsupported" }},
		{name: "round identity", mutate: func(receipt *ReplicationQuestionReceipt) { receipt.RoundSHA256 = strings.Repeat("z", 64) }},
		{name: "round answer", mutate: func(receipt *ReplicationQuestionReceipt) {
			receipt.Round.Answers[0].Reason = `C:\\private\\captured-value`
		}},
		{name: "selected answer", mutate: func(receipt *ReplicationQuestionReceipt) { receipt.Result = string(NoChangeObserved) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := receipt
			test.mutate(&candidate)
			if _, err := ReplicationQuestionReceiptSHA256(candidate); err == nil {
				t.Fatal("ReplicationQuestionReceiptSHA256() accepted invalid receipt")
			}
		})
	}
	if _, err := DecodeReplicationQuestionReceipt(nil); err == nil {
		t.Fatal("DecodeReplicationQuestionReceipt() accepted empty data")
	}
	if _, err := DecodeReplicationQuestionReceipt(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeReplicationQuestionReceipt() accepted oversized data")
	}
	if _, err := DecodeReplicationQuestionReceipt([]byte(`{"schema_version":1,"question_id":"replication-outcome"}`)); err == nil {
		t.Fatal("DecodeReplicationQuestionReceipt() accepted incomplete data")
	}
}

func TestReplicationQuestionRoundPathsAndIdentities(t *testing.T) {
	ledgerPath, _ := writeQuestionLedger(t, true, true)
	for _, path := range []string{"", " ", filepath.Join(t.TempDir(), "missing.json")} {
		if _, _, err := ReadReplicationQuestionRound(path); err == nil {
			t.Errorf("ReadReplicationQuestionRound(%q) accepted missing path", path)
		}
		if _, err := VerifyReplicationQuestionRound(path); err == nil {
			t.Errorf("VerifyReplicationQuestionRound(%q) accepted missing path", path)
		}
		if _, _, err := ReadReplicationQuestionReceipt(path); err == nil {
			t.Errorf("ReadReplicationQuestionReceipt(%q) accepted missing path", path)
		}
		if _, err := VerifyReplicationQuestionReceipt(path); err == nil {
			t.Errorf("VerifyReplicationQuestionReceipt(%q) accepted missing path", path)
		}
	}
	if _, err := SaveReplicationQuestionRound(ledgerPath, " "); err == nil {
		t.Fatal("SaveReplicationQuestionRound() accepted an empty output path")
	}
	if _, err := SaveReplicationQuestionRound(filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "round.json")); err == nil {
		t.Fatal("SaveReplicationQuestionRound() accepted a missing ledger")
	}
	if _, err := SaveReplicationQuestionReceipt(filepath.Join(t.TempDir(), "missing.json"), ReplicationQuestionOutcome, filepath.Join(t.TempDir(), "receipt.json")); err == nil {
		t.Fatal("SaveReplicationQuestionReceipt() accepted a missing round")
	}
	if _, err := SaveReplicationQuestionReceipt("round.json", ReplicationQuestionOutcome, " "); err == nil {
		t.Fatal("SaveReplicationQuestionReceipt() accepted an empty output path")
	}
	if _, err := AskReplicationQuestion("", ReplicationQuestionOutcome); err == nil {
		t.Fatal("AskReplicationQuestion() accepted an empty ledger path")
	}
	if _, err := AskAllReplicationQuestions(""); err == nil {
		t.Fatal("AskAllReplicationQuestions() accepted an empty ledger path")
	}
	if _, err := ReplicationQuestionRoundSHA256(ReplicationQuestionRound{}); err == nil {
		t.Fatal("ReplicationQuestionRoundSHA256() accepted an invalid round")
	}
	if _, err := ReplicationQuestionReceiptSHA256(ReplicationQuestionReceipt{}); err == nil {
		t.Fatal("ReplicationQuestionReceiptSHA256() accepted an invalid receipt")
	}
	if _, err := AnswerAllReplicationQuestions(ReplicationLedger{}, ReplicationLedgerVerificationSummary{}); err == nil {
		t.Fatal("AnswerAllReplicationQuestions() accepted an invalid ledger")
	}
	if validReplicationQuestionResult("not-a-question", "unknown") {
		t.Fatal("validReplicationQuestionResult() accepted an invalid question")
	}
	if _, ok := replicationQuestionResult("not-a-question", ReplicationUnknown, evidence.Unknown); ok {
		t.Fatal("replicationQuestionResult() accepted an invalid question")
	}
	if _, ok := replicationQuestionResult(ReplicationQuestionConsistency, "not-an-outcome", evidence.Observed); ok {
		t.Fatal("replicationQuestionResult() accepted an invalid outcome")
	}
}

func validReplicationQuestionRound(t *testing.T) ReplicationQuestionRound {
	t.Helper()
	_, ledger := writeQuestionLedger(t, true, true)
	summary, err := replicationLedgerSummary(ledger)
	if err != nil {
		t.Fatal(err)
	}
	answers, err := AnswerAllReplicationQuestions(ledger, summary)
	if err != nil {
		t.Fatal(err)
	}
	return ReplicationQuestionRound{SchemaVersion: replicationQuestionRoundSchemaVersion, LedgerSHA256: summary.LedgerSHA256, Answers: answers}
}

func cloneReplicationQuestionRound(round ReplicationQuestionRound) ReplicationQuestionRound {
	data, err := json.Marshal(round)
	if err != nil {
		panic(err)
	}
	var clone ReplicationQuestionRound
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

func writeQuestionLedger(t *testing.T, firstChange, secondChange bool) (string, ReplicationLedger) {
	return writeQuestionLedgerWithChanges(t, firstChange, secondChange, true, true, true)
}

func writeQuestionLedgerWithChanges(t *testing.T, firstChange, secondChange, firstReset, secondReset, balanced bool) (string, ReplicationLedger) {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("a", 64)
	inputs := []ReplicationPairInput{
		writeReplicationPair(t, root, "first", replicationTrace("browser", "region"), replicationTreatmentTrace("browser", "region", firstChange), "browser-redacted-audit", procedure, OrderBaselineTreatment, firstReset),
	}
	if balanced {
		inputs = append(inputs, writeReplicationPair(t, root, "second", replicationTrace("browser", "region"), replicationTreatmentTrace("browser", "region", secondChange), "browser-redacted-audit", procedure, OrderTreatmentBaseline, secondReset))
	}
	path := filepath.Join(root, "ledger.json")
	if _, err := SaveReplicationLedger(inputs, path); err != nil {
		t.Fatal(err)
	}
	ledger, _, err := ReadReplicationLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, ledger
}

func TestReplicationQuestionHelpersKeepErrorsSafe(t *testing.T) {
	if _, ok := replicationQuestion("not-a-question"); ok {
		t.Fatal("replicationQuestion() accepted an invalid ID")
	}
	if _, ok := replicationQuestionIndex("not-a-question"); ok {
		t.Fatal("replicationQuestionIndex() accepted an invalid ID")
	}
	if _, err := replicationAnswerFromRound(ReplicationQuestionRound{Answers: []ReplicationAnswer{{}}}, "not-a-question"); err == nil {
		t.Fatal("replicationAnswerFromRound() accepted an invalid ID")
	}
	if _, err := AnswerReplicationQuestion(ReplicationLedger{}, ReplicationLedgerVerificationSummary{}, "not-a-question"); err == nil {
		t.Fatal("AnswerReplicationQuestion() accepted an invalid ID before ledger validation")
	}
}

func TestReplicationQuestionReasonAndResultSemantics(t *testing.T) {
	reasons := []struct {
		name   string
		answer ReplicationAnswer
		want   string
	}{
		{name: "outcome unbalanced", answer: ReplicationAnswer{QuestionID: ReplicationQuestionOutcome}, want: "equal nonzero"},
		{name: "outcome changed", answer: ReplicationAnswer{QuestionID: ReplicationQuestionOutcome, OrderBalanced: true, Outcome: ReplicatedChange}, want: "safe category difference"},
		{name: "outcome no change", answer: ReplicationAnswer{QuestionID: ReplicationQuestionOutcome, OrderBalanced: true, Outcome: NoChangeObserved}, want: "no retained pair"},
		{name: "outcome mixed", answer: ReplicationAnswer{QuestionID: ReplicationQuestionOutcome, OrderBalanced: true, Outcome: MixedInconsistent}, want: "disagree"},
		{name: "outcome unknown", answer: ReplicationAnswer{QuestionID: ReplicationQuestionOutcome, OrderBalanced: true, Outcome: ReplicationUnknown}, want: "lacks complete"},
		{name: "support observed", answer: ReplicationAnswer{QuestionID: ReplicationQuestionSupport, EvidenceState: evidence.Observed}, want: "confirmed reset"},
		{name: "support unknown", answer: ReplicationAnswer{QuestionID: ReplicationQuestionSupport, EvidenceState: evidence.Unknown}, want: "lacks reset"},
		{name: "consistency consistent", answer: ReplicationAnswer{QuestionID: ReplicationQuestionConsistency, Outcome: ReplicatedChange}, want: "agree"},
		{name: "consistency mixed", answer: ReplicationAnswer{QuestionID: ReplicationQuestionConsistency, Outcome: MixedInconsistent}, want: "disagree"},
		{name: "consistency unknown", answer: ReplicationAnswer{QuestionID: ReplicationQuestionConsistency, Outcome: ReplicationUnknown}, want: "cannot establish"},
	}
	for _, test := range reasons {
		t.Run(test.name, func(t *testing.T) {
			if got := replicationQuestionReason(test.answer); !strings.Contains(got, test.want) {
				t.Fatalf("replicationQuestionReason() = %q, want %q", got, test.want)
			}
		})
	}
	if replicationQuestionReason(ReplicationAnswer{QuestionID: "not-a-question"}) != "" {
		t.Fatal("replicationQuestionReason() returned a reason for an invalid question")
	}

	results := []struct {
		name      string
		question  string
		outcome   ReplicatedOutcome
		state     evidence.State
		want      string
		wantValid bool
	}{
		{name: "outcome", question: ReplicationQuestionOutcome, outcome: ReplicatedChange, state: evidence.Observed, want: string(ReplicatedChange), wantValid: true},
		{name: "support observed", question: ReplicationQuestionSupport, state: evidence.Observed, want: "supported", wantValid: true},
		{name: "support unknown", question: ReplicationQuestionSupport, state: evidence.Unknown, want: "unknown", wantValid: true},
		{name: "consistency changed", question: ReplicationQuestionConsistency, outcome: ReplicatedChange, want: "consistent", wantValid: true},
		{name: "consistency mixed", question: ReplicationQuestionConsistency, outcome: MixedInconsistent, want: "inconsistent", wantValid: true},
		{name: "consistency unknown", question: ReplicationQuestionConsistency, outcome: ReplicationUnknown, want: "unknown", wantValid: true},
		{name: "consistency invalid outcome", question: ReplicationQuestionConsistency, outcome: "not-an-outcome", wantValid: false},
		{name: "invalid question", question: "not-a-question", outcome: ReplicationUnknown, wantValid: false},
	}
	for _, test := range results {
		t.Run(test.name, func(t *testing.T) {
			got, ok := replicationQuestionResult(test.question, test.outcome, test.state)
			if ok != test.wantValid || (ok && got != test.want) {
				t.Fatalf("replicationQuestionResult() = %q, %v; want %q, %v", got, ok, test.want, test.wantValid)
			}
		})
	}
}
