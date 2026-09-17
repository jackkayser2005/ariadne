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

func TestRunTraceArchiveQuestionRoundCommands(t *testing.T) {
	roundSummary := trace.ArchiveQuestionRoundVerificationSummary{
		SchemaVersion: 1,
		ArchiveSHA256: strings.Repeat("a", 64),
		Questions:     3,
		RoundSHA256:   strings.Repeat("b", 64),
	}
	var stdout, stderr bytes.Buffer
	save := func(archivePath, roundPath string) (trace.ArchiveQuestionRoundVerificationSummary, error) {
		if archivePath != "archive.json" || roundPath != "round.json" {
			t.Fatalf("SaveArchiveQuestionRound() args = %q, %q", archivePath, roundPath)
		}
		return roundSummary, nil
	}
	if exitCode := runTraceArchiveAskAllSave([]string{"archive.json", "round.json"}, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "question round saved") {
		t.Fatalf("round save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskAllSave([]string{"--json", "archive.json", "round.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON round save = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotRoundSummary trace.ArchiveQuestionRoundVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &gotRoundSummary); err != nil || !reflect.DeepEqual(gotRoundSummary, roundSummary) {
		t.Fatalf("JSON round save = %#v, err=%v", gotRoundSummary, err)
	}

	verify := func(path string) (trace.ArchiveQuestionRoundVerificationSummary, error) {
		if path != "round.json" {
			t.Fatalf("VerifyArchiveQuestionRound() path = %q", path)
		}
		return roundSummary, nil
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskAllVerify([]string{"--expect-sha256", roundSummary.RoundSHA256, "round.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "question round verified") {
		t.Fatalf("round verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskAllVerify([]string{"--json", "round.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON round verify = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &gotRoundSummary); err != nil || !reflect.DeepEqual(gotRoundSummary, roundSummary) {
		t.Fatalf("JSON round verify = %#v, err=%v", gotRoundSummary, err)
	}

	for _, test := range []struct {
		name string
		args []string
		call func([]string, ioWriter, ioWriter) int
	}{
		{name: "save usage", args: []string{"archive.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllSave(args, out, errOut, save)
		}},
		{name: "verify usage", args: nil, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllVerify(args, out, errOut, verify)
		}},
		{name: "bad save", args: []string{"archive.json", "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllSave(args, out, errOut, func(string, string) (trace.ArchiveQuestionRoundVerificationSummary, error) {
				return trace.ArchiveQuestionRoundVerificationSummary{}, errors.New("round save failed safely")
			})
		}},
		{name: "bad verify", args: []string{"round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllVerify(args, out, errOut, func(string) (trace.ArchiveQuestionRoundVerificationSummary, error) {
				return trace.ArchiveQuestionRoundVerificationSummary{}, errors.New("round verify failed safely")
			})
		}},
		{name: "bad expected round digest", args: []string{"--expect-sha256", "bad", "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllVerify(args, out, errOut, verify)
		}},
		{name: "mismatched round digest", args: []string{"--expect-sha256", strings.Repeat("c", 64), "round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskAllVerify(args, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := test.call(test.args, &out, &errOut); got != 1 && got != 2 {
				t.Fatalf("exit code = %d, stdout=%q, stderr=%q", got, out.String(), errOut.String())
			}
			if errOut.Len() == 0 {
				t.Fatalf("stderr is empty for %s", test.name)
			}
		})
	}
	if exitCode := runTraceArchiveAskAllSave([]string{"archive.json", "round.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("round save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskAllSave([]string{"--json", "archive.json", "round.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON round save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskAllVerify([]string{"round.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("round verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceArchiveQuestionReceiptCommands(t *testing.T) {
	receipt := trace.ArchiveQuestionReceipt{
		SchemaVersion: 1,
		QuestionID:    trace.ArchiveQuestionChange,
		Question:      "Did the safe tracking categories change across the retained traces?",
		Result:        "changed",
		EvidenceState: "observed",
		Reason:        "safe reason",
		ArchiveSHA256: strings.Repeat("a", 64),
		RoundSHA256:   strings.Repeat("b", 64),
		Entries:       2,
		Compared:      1,
		Changed:       1,
	}
	receiptSummary := trace.ArchiveQuestionReceiptVerificationSummary{
		SchemaVersion: 1,
		QuestionID:    receipt.QuestionID,
		Question:      receipt.Question,
		Result:        receipt.Result,
		EvidenceState: receipt.EvidenceState,
		ArchiveSHA256: receipt.ArchiveSHA256,
		RoundSHA256:   receipt.RoundSHA256,
		ReceiptSHA256: strings.Repeat("c", 64),
	}
	ask := func(roundPath, questionID string) (trace.ArchiveQuestionReceipt, error) {
		if roundPath != "round.json" || questionID != receipt.QuestionID {
			t.Fatalf("AskArchiveQuestionReceipt() args = %q, %q", roundPath, questionID)
		}
		return receipt, nil
	}
	save := func(roundPath, questionID, receiptPath string) (trace.ArchiveQuestionReceiptVerificationSummary, error) {
		if roundPath != "round.json" || questionID != receipt.QuestionID || receiptPath != "receipt.json" {
			t.Fatalf("SaveArchiveQuestionReceipt() args = %q, %q, %q", roundPath, questionID, receiptPath)
		}
		return receiptSummary, nil
	}
	verify := func(path string) (trace.ArchiveQuestionReceiptVerificationSummary, error) {
		if path != "receipt.json" {
			t.Fatalf("VerifyArchiveQuestionReceipt() path = %q", path)
		}
		return receiptSummary, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceArchiveAskReceipt([]string{"round.json", receipt.QuestionID}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "trace archive question receipt") || !strings.Contains(stdout.String(), "reason: safe reason") {
		t.Fatalf("receipt ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceipt([]string{"--json", "round.json", receipt.QuestionID}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON receipt ask = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotReceipt trace.ArchiveQuestionReceipt
	if err := json.Unmarshal(stdout.Bytes(), &gotReceipt); err != nil || !reflect.DeepEqual(gotReceipt, receipt) {
		t.Fatalf("JSON receipt ask = %#v, err=%v", gotReceipt, err)
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceiptSave([]string{"round.json", receipt.QuestionID, "receipt.json"}, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), "receipt saved") {
		t.Fatalf("receipt save = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceiptSave([]string{"--json", "round.json", receipt.QuestionID, "receipt.json"}, &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON receipt save = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotReceiptSummary trace.ArchiveQuestionReceiptVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &gotReceiptSummary); err != nil || !reflect.DeepEqual(gotReceiptSummary, receiptSummary) {
		t.Fatalf("JSON receipt save = %#v, err=%v", gotReceiptSummary, err)
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceiptVerify([]string{"--expect-sha256", receiptSummary.ReceiptSHA256, "receipt.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "receipt verified") {
		t.Fatalf("receipt verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceiptVerify([]string{"--json", "receipt.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON receipt verify = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &gotReceiptSummary); err != nil || !reflect.DeepEqual(gotReceiptSummary, receiptSummary) {
		t.Fatalf("JSON receipt verify = %#v, err=%v", gotReceiptSummary, err)
	}

	for _, test := range []struct {
		name string
		args []string
		call func([]string, ioWriter, ioWriter) int
	}{
		{name: "ask usage", args: []string{"round.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceipt(args, out, errOut, ask)
		}},
		{name: "save usage", args: []string{"round.json", receipt.QuestionID}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptSave(args, out, errOut, save)
		}},
		{name: "verify usage", args: nil, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptVerify(args, out, errOut, verify)
		}},
		{name: "ask error", args: []string{"round.json", receipt.QuestionID}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceipt(args, out, errOut, func(string, string) (trace.ArchiveQuestionReceipt, error) {
				return trace.ArchiveQuestionReceipt{}, errors.New("receipt ask failed safely")
			})
		}},
		{name: "save error", args: []string{"round.json", receipt.QuestionID, "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptSave(args, out, errOut, func(string, string, string) (trace.ArchiveQuestionReceiptVerificationSummary, error) {
				return trace.ArchiveQuestionReceiptVerificationSummary{}, errors.New("receipt save failed safely")
			})
		}},
		{name: "verify error", args: []string{"receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptVerify(args, out, errOut, func(string) (trace.ArchiveQuestionReceiptVerificationSummary, error) {
				return trace.ArchiveQuestionReceiptVerificationSummary{}, errors.New("receipt verify failed safely")
			})
		}},
		{name: "bad expected receipt digest", args: []string{"--expect-sha256", "bad", "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptVerify(args, out, errOut, verify)
		}},
		{name: "mismatched receipt digest", args: []string{"--expect-sha256", strings.Repeat("d", 64), "receipt.json"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveAskReceiptVerify(args, out, errOut, verify)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := test.call(test.args, &out, &errOut); got != 1 && got != 2 {
				t.Fatalf("exit code = %d, stdout=%q, stderr=%q", got, out.String(), errOut.String())
			}
			if errOut.Len() == 0 {
				t.Fatalf("stderr is empty for %s", test.name)
			}
		})
	}
	if exitCode := runTraceArchiveAskReceipt([]string{"round.json", receipt.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskReceipt([]string{"--json", "round.json", receipt.QuestionID}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskReceiptSave([]string{"round.json", receipt.QuestionID, "receipt.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskReceiptSave([]string{"--json", "round.json", receipt.QuestionID, "receipt.json"}, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt save write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskReceiptVerify([]string{"receipt.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("receipt verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskReceiptVerify([]string{"--json", "receipt.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON receipt verify write error = %d, stderr=%q", exitCode, stderr.String())
	}

	noReason := receipt
	noReason.Reason = ""
	stdout.Reset()
	if exitCode := runTraceArchiveAskReceipt([]string{"round.json", receipt.QuestionID}, &stdout, &stderr, func(string, string) (trace.ArchiveQuestionReceipt, error) { return noReason, nil }); exitCode != 0 || strings.Contains(stdout.String(), "reason:") {
		t.Fatalf("receipt without reason = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceArchiveRoundWriters(t *testing.T) {
	round := trace.ArchiveQuestionRoundVerificationSummary{SchemaVersion: 1, ArchiveSHA256: strings.Repeat("a", 64), Questions: 3, RoundSHA256: strings.Repeat("b", 64)}
	if err := writeTraceArchiveRoundSummary(&failAfterWriter{failAt: 1}, "round", round); err == nil {
		t.Fatal("writeTraceArchiveRoundSummary() accepted a failing writer")
	}
	receiptSummary := trace.ArchiveQuestionReceiptVerificationSummary{SchemaVersion: 1, QuestionID: trace.ArchiveQuestionCoverage, Question: "coverage", Result: "complete", EvidenceState: "observed", ArchiveSHA256: strings.Repeat("a", 64), RoundSHA256: strings.Repeat("b", 64), ReceiptSHA256: strings.Repeat("c", 64)}
	if err := writeTraceArchiveReceiptSummary(&failAfterWriter{failAt: 1}, "receipt", receiptSummary); err == nil {
		t.Fatal("writeTraceArchiveReceiptSummary() accepted a failing writer")
	}
	receipt := trace.ArchiveQuestionReceipt{SchemaVersion: 1, QuestionID: trace.ArchiveQuestionCoverage, Question: "coverage", Result: "complete", EvidenceState: "observed", Reason: "reason", ArchiveSHA256: strings.Repeat("a", 64), RoundSHA256: strings.Repeat("b", 64), Entries: 1}
	if err := writeTraceArchiveReceipt(&failAfterWriter{failAt: 1}, "receipt", receipt, strings.Repeat("c", 64)); err == nil {
		t.Fatal("writeTraceArchiveReceipt() accepted a failing writer")
	}
}
