package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceReplicationQuestionCommands(t *testing.T) {
	catalog := trace.ReplicationQuestions()
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceReplicationQuestions(nil, &stdout, &stderr, func() []trace.ReplicationQuestion { return catalog }); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication question catalog") || stderr.Len() != 0 {
		t.Fatalf("questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationQuestions([]string{"--json"}, &stdout, &stderr, func() []trace.ReplicationQuestion { return catalog }); exitCode != 0 {
		t.Fatalf("JSON questions = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotCatalog []trace.ReplicationQuestion
	if err := json.Unmarshal(stdout.Bytes(), &gotCatalog); err != nil || !reflect.DeepEqual(gotCatalog, catalog) {
		t.Fatalf("JSON questions = %#v, err=%v", gotCatalog, err)
	}
	if exitCode := runTraceReplicationQuestions([]string{"extra"}, &stdout, &stderr, func() []trace.ReplicationQuestion { return catalog }); exitCode != 2 {
		t.Fatalf("questions usage = %d", exitCode)
	}
	if exitCode := runTraceReplicationQuestions(nil, browserErrorWriter{}, &stderr, func() []trace.ReplicationQuestion { return catalog }); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("questions write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationQuestions([]string{"--json"}, browserErrorWriter{}, &stderr, func() []trace.ReplicationQuestion { return catalog }); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON questions write error = %d, stderr=%q", exitCode, stderr.String())
	}

	answer := trace.ReplicationAnswer{
		SchemaVersion:          1,
		QuestionID:             trace.ReplicationQuestionOutcome,
		Question:               catalog[0].Text,
		Result:                 string(trace.ReplicatedChange),
		EvidenceState:          "observed",
		Reason:                 "safe reason",
		LedgerSHA256:           strings.Repeat("a", 64),
		Pairs:                  2,
		BaselineTreatmentPairs: 1,
		TreatmentBaselinePairs: 1,
		ResetConfirmedPairs:    2,
		CompletePairs:          2,
		ChangedPairs:           2,
		Outcome:                trace.ReplicatedChange,
		OrderBalanced:          true,
	}
	ask := func(path, questionID string) (trace.ReplicationAnswer, error) {
		if path != "ledger.json" || questionID != answer.QuestionID {
			t.Fatalf("ask args = %q, %q", path, questionID)
		}
		return answer, nil
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAsk([]string{"ledger.json", answer.QuestionID}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "result: replicated-change") || !strings.Contains(stdout.String(), "reason: safe reason") {
		t.Fatalf("ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAsk([]string{"--json", "ledger.json", answer.QuestionID}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON ask = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotAnswer trace.ReplicationAnswer
	if err := json.Unmarshal(stdout.Bytes(), &gotAnswer); err != nil || !reflect.DeepEqual(gotAnswer, answer) {
		t.Fatalf("JSON ask = %#v, err=%v", gotAnswer, err)
	}
	all := func(path string) ([]trace.ReplicationAnswer, error) {
		if path != "ledger.json" {
			t.Fatalf("ask all path = %q", path)
		}
		return []trace.ReplicationAnswer{answer}, nil
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskAll([]string{"ledger.json"}, &stdout, &stderr, all); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication questions answered") {
		t.Fatalf("ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskAll([]string{"--json", "ledger.json"}, &stdout, &stderr, all); exitCode != 0 {
		t.Fatalf("JSON ask all = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &[]trace.ReplicationAnswer{}); err != nil {
		t.Fatalf("JSON ask all error = %v", err)
	}

	for _, test := range []struct {
		name string
		call func(ioWriter, ioWriter) int
	}{
		{name: "ask usage", call: func(out, errOut ioWriter) int {
			return runTraceReplicationAsk([]string{"ledger.json"}, out, errOut, ask)
		}},
		{name: "ask error", call: func(out, errOut ioWriter) int {
			return runTraceReplicationAsk([]string{"ledger.json", answer.QuestionID}, out, errOut, func(string, string) (trace.ReplicationAnswer, error) {
				return trace.ReplicationAnswer{}, errors.New("ask failed safely")
			})
		}},
		{name: "ask all usage", call: func(out, errOut ioWriter) int { return runTraceReplicationAskAll(nil, out, errOut, all) }},
		{name: "ask all error", call: func(out, errOut ioWriter) int {
			return runTraceReplicationAskAll([]string{"ledger.json"}, out, errOut, func(string) ([]trace.ReplicationAnswer, error) { return nil, errors.New("all failed safely") })
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(&out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}
	if exitCode := runTraceReplicationAsk([]string{"ledger.json", answer.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAsk([]string{"--json", "ledger.json", answer.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskAll([]string{"ledger.json"}, browserErrorWriter{}, &stderr, all); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("ask all write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskAll([]string{"--json", "ledger.json"}, browserErrorWriter{}, &stderr, all); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON ask all write error = %d, stderr=%q", exitCode, stderr.String())
	}
	answer.Reason = ""
	stdout.Reset()
	if exitCode := runTraceReplicationAsk([]string{"ledger.json", answer.QuestionID}, &stdout, &stderr, func(string, string) (trace.ReplicationAnswer, error) { return answer, nil }); exitCode != 0 || strings.Contains(stdout.String(), "reason:") {
		t.Fatalf("answer without reason = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceReplicationQuestionRoundCommands(t *testing.T) {
	roundSummary := trace.ReplicationQuestionRoundVerificationSummary{SchemaVersion: 1, LedgerSHA256: strings.Repeat("a", 64), Questions: 3, RoundSHA256: strings.Repeat("b", 64)}
	var stdout, stderr bytes.Buffer
	save := func(ledgerPath, roundPath string) (trace.ReplicationQuestionRoundVerificationSummary, error) {
		if ledgerPath != "ledger.json" || roundPath != "round.json" {
			t.Fatalf("round save args = %q, %q", ledgerPath, roundPath)
		}
		return roundSummary, nil
	}
	if exitCode := runTraceReplicationAskAllSave([]string{"ledger.json", "round.json"}, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "question round saved") {
		t.Fatalf("round save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskAllSave([]string{"--json", "ledger.json", "round.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON round save = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.ReplicationQuestionRoundVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got != roundSummary {
		t.Fatalf("JSON round save = %#v, err=%v", got, err)
	}
	verify := func(path string) (trace.ReplicationQuestionRoundVerificationSummary, error) {
		if path != "round.json" {
			t.Fatalf("round verify path = %q", path)
		}
		return roundSummary, nil
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskAllVerify([]string{"--expect-sha256", roundSummary.RoundSHA256, "round.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "question round verified") {
		t.Fatalf("round verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskAllVerify([]string{"--json", "round.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON round verify = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got != roundSummary {
		t.Fatalf("JSON round verify = %#v, err=%v", got, err)
	}
	for _, test := range []struct {
		name string
		args []string
		call func([]string, ioWriter, ioWriter) int
	}{
		{name: "save usage", args: []string{"ledger.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllSave(args, out, errOut, save)
		}},
		{name: "verify usage", args: nil, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllVerify(args, out, errOut, verify)
		}},
		{name: "save error", args: []string{"ledger.json", "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllSave(args, out, errOut, func(string, string) (trace.ReplicationQuestionRoundVerificationSummary, error) {
				return trace.ReplicationQuestionRoundVerificationSummary{}, errors.New("round save failed safely")
			})
		}},
		{name: "verify error", args: []string{"round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllVerify(args, out, errOut, func(string) (trace.ReplicationQuestionRoundVerificationSummary, error) {
				return trace.ReplicationQuestionRoundVerificationSummary{}, errors.New("round verify failed safely")
			})
		}},
		{name: "bad digest", args: []string{"--expect-sha256", "bad", "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllVerify(args, out, errOut, verify)
		}},
		{name: "mismatched digest", args: []string{"--expect-sha256", strings.Repeat("c", 64), "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskAllVerify(args, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(test.args, &out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}
	if exitCode := runTraceReplicationAskAllSave([]string{"ledger.json", "round.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("round save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskAllSave([]string{"--json", "ledger.json", "round.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON round save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskAllVerify([]string{"round.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("round verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceReplicationQuestionReceiptCommands(t *testing.T) {
	question := trace.ReplicationQuestions()[0]
	receipt := trace.ReplicationQuestionReceipt{ReplicationAnswer: trace.ReplicationAnswer{SchemaVersion: 1, QuestionID: question.ID, Question: question.Text, Result: string(trace.ReplicatedChange), EvidenceState: "observed", Reason: "safe reason", LedgerSHA256: strings.Repeat("a", 64), Pairs: 2, BaselineTreatmentPairs: 1, TreatmentBaselinePairs: 1, ResetConfirmedPairs: 2, CompletePairs: 2, ChangedPairs: 2, OrderBalanced: true, Outcome: trace.ReplicatedChange}, RoundSHA256: strings.Repeat("b", 64)}
	receiptSummary := trace.ReplicationQuestionReceiptVerificationSummary{SchemaVersion: 1, QuestionID: receipt.QuestionID, Question: receipt.Question, Result: receipt.Result, EvidenceState: receipt.EvidenceState, LedgerSHA256: receipt.LedgerSHA256, RoundSHA256: receipt.RoundSHA256, ReceiptSHA256: strings.Repeat("c", 64), Outcome: receipt.Outcome}
	ask := func(roundPath, questionID string) (trace.ReplicationQuestionReceipt, error) {
		if roundPath != "round.json" || questionID != receipt.QuestionID {
			t.Fatalf("receipt ask args = %q, %q", roundPath, questionID)
		}
		return receipt, nil
	}
	save := func(roundPath, questionID, receiptPath string) (trace.ReplicationQuestionReceiptVerificationSummary, error) {
		if roundPath != "round.json" || questionID != receipt.QuestionID || receiptPath != "receipt.json" {
			t.Fatalf("receipt save args = %q, %q, %q", roundPath, questionID, receiptPath)
		}
		return receiptSummary, nil
	}
	verify := func(path string) (trace.ReplicationQuestionReceiptVerificationSummary, error) {
		if path != "receipt.json" {
			t.Fatalf("receipt verify path = %q", path)
		}
		return receiptSummary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceReplicationAskReceipt([]string{"round.json", receipt.QuestionID}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "trace replication question receipt") || !strings.Contains(stdout.String(), "reason: safe reason") {
		t.Fatalf("receipt ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceipt([]string{"--json", "round.json", receipt.QuestionID}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON receipt ask = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotReceipt trace.ReplicationQuestionReceipt
	if err := json.Unmarshal(stdout.Bytes(), &gotReceipt); err != nil || !reflect.DeepEqual(gotReceipt, receipt) {
		t.Fatalf("JSON receipt ask = %#v, err=%v", gotReceipt, err)
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceiptSave([]string{"round.json", receipt.QuestionID, "receipt.json"}, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "receipt saved") {
		t.Fatalf("receipt save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceiptSave([]string{"--json", "round.json", receipt.QuestionID, "receipt.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON receipt save = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotSummary trace.ReplicationQuestionReceiptVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &gotSummary); err != nil || gotSummary != receiptSummary {
		t.Fatalf("JSON receipt save = %#v, err=%v", gotSummary, err)
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceiptVerify([]string{"--expect-sha256", receiptSummary.ReceiptSHA256, "receipt.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "receipt verified") {
		t.Fatalf("receipt verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceiptVerify([]string{"--json", "receipt.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON receipt verify = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &gotSummary); err != nil || gotSummary != receiptSummary {
		t.Fatalf("JSON receipt verify = %#v, err=%v", gotSummary, err)
	}
	for _, test := range []struct {
		name string
		args []string
		call func([]string, ioWriter, ioWriter) int
	}{
		{name: "ask usage", args: []string{"round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceipt(args, out, errOut, ask)
		}},
		{name: "save usage", args: []string{"round.json", receipt.QuestionID}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptSave(args, out, errOut, save)
		}},
		{name: "verify usage", args: nil, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptVerify(args, out, errOut, verify)
		}},
		{name: "ask error", args: []string{"round.json", receipt.QuestionID}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceipt(args, out, errOut, func(string, string) (trace.ReplicationQuestionReceipt, error) {
				return trace.ReplicationQuestionReceipt{}, errors.New("receipt ask failed safely")
			})
		}},
		{name: "save error", args: []string{"round.json", receipt.QuestionID, "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptSave(args, out, errOut, func(string, string, string) (trace.ReplicationQuestionReceiptVerificationSummary, error) {
				return trace.ReplicationQuestionReceiptVerificationSummary{}, errors.New("receipt save failed safely")
			})
		}},
		{name: "verify error", args: []string{"receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptVerify(args, out, errOut, func(string) (trace.ReplicationQuestionReceiptVerificationSummary, error) {
				return trace.ReplicationQuestionReceiptVerificationSummary{}, errors.New("receipt verify failed safely")
			})
		}},
		{name: "bad digest", args: []string{"--expect-sha256", "bad", "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptVerify(args, out, errOut, verify)
		}},
		{name: "mismatched digest", args: []string{"--expect-sha256", strings.Repeat("d", 64), "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceReplicationAskReceiptVerify(args, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if exitCode := test.call(test.args, &out, &errOut); exitCode == 0 || errOut.Len() == 0 {
				t.Fatalf("call = %d, stdout=%q, stderr=%q", exitCode, out.String(), errOut.String())
			}
		})
	}
	if exitCode := runTraceReplicationAskReceipt([]string{"round.json", receipt.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskReceipt([]string{"--json", "round.json", receipt.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskReceiptSave([]string{"round.json", receipt.QuestionID, "receipt.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskReceiptSave([]string{"--json", "round.json", receipt.QuestionID, "receipt.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskReceiptVerify([]string{"receipt.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceReplicationAskReceiptVerify([]string{"--json", "receipt.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
	receipt.Reason = ""
	stdout.Reset()
	if exitCode := runTraceReplicationAskReceipt([]string{"round.json", receipt.QuestionID}, &stdout, &stderr, func(string, string) (trace.ReplicationQuestionReceipt, error) { return receipt, nil }); exitCode != 0 || strings.Contains(stdout.String(), "reason:") {
		t.Fatalf("receipt without reason = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceReplicationQuestionWriters(t *testing.T) {
	round := trace.ReplicationQuestionRoundVerificationSummary{SchemaVersion: 1, LedgerSHA256: strings.Repeat("a", 64), Questions: 3, RoundSHA256: strings.Repeat("b", 64)}
	if err := writeTraceReplicationRoundSummary(&failAfterWriter{failAt: 1}, "round", round); err == nil {
		t.Fatal("writeTraceReplicationRoundSummary() accepted a failing writer")
	}
	receiptSummary := trace.ReplicationQuestionReceiptVerificationSummary{SchemaVersion: 1, QuestionID: trace.ReplicationQuestionOutcome, Question: "outcome", Result: string(trace.ReplicatedChange), EvidenceState: "observed", LedgerSHA256: strings.Repeat("a", 64), RoundSHA256: strings.Repeat("b", 64), ReceiptSHA256: strings.Repeat("c", 64), Outcome: trace.ReplicatedChange}
	if err := writeTraceReplicationReceiptSummary(&failAfterWriter{failAt: 1}, "receipt", receiptSummary); err == nil {
		t.Fatal("writeTraceReplicationReceiptSummary() accepted a failing writer")
	}
	answer := trace.ReplicationAnswer{QuestionID: trace.ReplicationQuestionOutcome, Question: "outcome", Result: string(trace.ReplicatedChange), EvidenceState: "observed", Reason: "reason", Outcome: trace.ReplicatedChange}
	if err := writeTraceReplicationAnswer(&failAfterWriter{failAt: 1}, answer); err == nil {
		t.Fatal("writeTraceReplicationAnswer() accepted a failing writer")
	}
	receipt := trace.ReplicationQuestionReceipt{ReplicationAnswer: answer, RoundSHA256: strings.Repeat("b", 64)}
	if err := writeTraceReplicationReceipt(&failAfterWriter{failAt: 1}, "receipt", receipt, strings.Repeat("c", 64)); err == nil {
		t.Fatal("writeTraceReplicationReceipt() accepted a failing writer")
	}
}
