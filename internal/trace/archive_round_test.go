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

func TestArchiveQuestionRoundAndReceiptLifecycle(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "archive.json")
	inputOne := writeStandaloneArchiveInput(t, root, "one", validArchiveTrace("region"), strings.Repeat("a", 64))
	inputTwo := writeStandaloneArchiveInput(t, root, "two", validArchiveTrace("region", "consent"), strings.Repeat("a", 64))
	if _, err := SaveArchive([]ArchiveInput{inputOne, inputTwo}, archivePath); err != nil {
		t.Fatal(err)
	}

	roundPath := filepath.Join(root, "round.json")
	roundSummary, err := SaveArchiveQuestionRound(archivePath, roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if roundSummary.Questions != len(ArchiveQuestions()) || !ValidSHA256(roundSummary.ArchiveSHA256) || !ValidSHA256(roundSummary.RoundSHA256) {
		t.Fatalf("round summary = %#v", roundSummary)
	}
	round, verifiedRound, err := ReadArchiveQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if roundSummary != verifiedRound || round.ArchiveSHA256 != roundSummary.ArchiveSHA256 || len(round.Answers) != 3 {
		t.Fatalf("round = %#v, summary = %#v, verified = %#v", round, roundSummary, verifiedRound)
	}
	if verified, err := VerifyArchiveQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyArchiveQuestionRound() = %#v, %v", verified, err)
	}

	answer, err := AskArchiveQuestionRound(roundPath, ArchiveQuestionChange)
	if err != nil {
		t.Fatal(err)
	}
	if answer.QuestionID != ArchiveQuestionChange || answer.ArchiveSHA256 != roundSummary.ArchiveSHA256 || answer.Result != archiveResultChanged || answer.EvidenceState != evidence.Observed {
		t.Fatalf("answer = %#v", answer)
	}

	receipt, err := AskArchiveQuestionReceipt(roundPath, ArchiveQuestionChange)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.QuestionID != answer.QuestionID || receipt.RoundSHA256 != roundSummary.RoundSHA256 || receipt.Result != answer.Result || receipt.EvidenceState != answer.EvidenceState {
		t.Fatalf("receipt = %#v", receipt)
	}
	receiptPath := filepath.Join(root, "receipt.json")
	receiptSummary, err := SaveArchiveQuestionReceipt(roundPath, ArchiveQuestionChange, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if receiptSummary.QuestionID != ArchiveQuestionChange || !ValidSHA256(receiptSummary.ReceiptSHA256) {
		t.Fatalf("receipt summary = %#v", receiptSummary)
	}
	readReceipt, verifiedReceipt, err := ReadArchiveQuestionReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readReceipt, receipt) || verifiedReceipt != receiptSummary {
		t.Fatalf("read receipt = %#v, summary = %#v", readReceipt, verifiedReceipt)
	}
	if verified, err := VerifyArchiveQuestionReceipt(receiptPath); err != nil || verified != receiptSummary {
		t.Fatalf("VerifyArchiveQuestionReceipt() = %#v, %v", verified, err)
	}

	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err == nil {
		t.Fatalf("second round save error = %v", err)
	}
	if _, err := SaveArchiveQuestionReceipt(roundPath, ArchiveQuestionChange, receiptPath); err == nil {
		t.Fatalf("second receipt save error = %v", err)
	}
	if _, err := AskArchiveQuestionRound(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskArchiveQuestionRound() accepted an invalid question")
	}
	if _, err := AskArchiveQuestionReceipt(roundPath, "not-a-question"); err == nil {
		t.Fatal("AskArchiveQuestionReceipt() accepted an invalid question")
	}
}

func TestArchiveQuestionRoundAndReceiptRequirePathsAndFiles(t *testing.T) {
	for _, path := range []string{"", " ", filepath.Join(t.TempDir(), "missing.json")} {
		if _, _, err := ReadArchiveQuestionRound(path); err == nil {
			t.Errorf("ReadArchiveQuestionRound(%q) accepted missing path", path)
		}
		if _, _, err := ReadArchiveQuestionReceipt(path); err == nil {
			t.Errorf("ReadArchiveQuestionReceipt(%q) accepted missing path", path)
		}
		if _, err := VerifyArchiveQuestionRound(path); err == nil {
			t.Errorf("VerifyArchiveQuestionRound(%q) accepted missing path", path)
		}
		if _, err := VerifyArchiveQuestionReceipt(path); err == nil {
			t.Errorf("VerifyArchiveQuestionReceipt(%q) accepted missing path", path)
		}
	}
	root := t.TempDir()
	if _, err := SaveArchiveQuestionRound("archive.json", " "); err == nil {
		t.Fatal("SaveArchiveQuestionRound() accepted an empty output path")
	}
	if _, err := SaveArchiveQuestionReceipt("round.json", ArchiveQuestionCoverage, " "); err == nil {
		t.Fatal("SaveArchiveQuestionReceipt() accepted an empty output path")
	}
	if _, err := SaveArchiveQuestionRound(filepath.Join(root, "missing.json"), filepath.Join(root, "round.json")); err == nil {
		t.Fatal("SaveArchiveQuestionRound() accepted a missing archive")
	}
	if _, err := SaveArchiveQuestionReceipt(filepath.Join(root, "missing.json"), ArchiveQuestionCoverage, filepath.Join(root, "receipt.json")); err == nil {
		t.Fatal("SaveArchiveQuestionReceipt() accepted a missing round")
	}
}

func TestDecodeArchiveQuestionRoundRejectsMalformedDocuments(t *testing.T) {
	valid := validArchiveQuestionRound(t)
	validData, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown", data: append(append([]byte(nil), validData[:len(validData)-1]...), []byte(`,"extra":true}`)...)},
		{name: "trailing", data: append(validData, []byte(` {}`)...)},
		{name: "invalid json", data: []byte(`{`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeArchiveQuestionRound(test.data); err == nil {
				t.Fatal("DecodeArchiveQuestionRound() accepted malformed input")
			}
		})
	}
	if _, err := DecodeArchiveQuestionRound(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeArchiveQuestionRound() accepted oversized input")
	}
}

func TestDecodeArchiveQuestionReceiptRejectsMalformedDocuments(t *testing.T) {
	round := validArchiveQuestionRound(t)
	receipt := archiveQuestionReceiptFromAnswer(round.Answers[1], strings.Repeat("b", 64))
	validData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "duplicate", data: []byte(`{"question_id":"trace-change","question_id":"trace-change"}`)},
		{name: "unknown", data: append(append([]byte(nil), validData[:len(validData)-1]...), []byte(`,"extra":true}`)...)},
		{name: "trailing", data: append(validData, []byte(` {}`)...)},
		{name: "invalid json", data: []byte(`{`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeArchiveQuestionReceipt(test.data); err == nil {
				t.Fatal("DecodeArchiveQuestionReceipt() accepted malformed input")
			}
		})
	}
	if _, err := DecodeArchiveQuestionReceipt(bytes.Repeat([]byte("x"), maxArchiveBytes+1)); err == nil {
		t.Fatal("DecodeArchiveQuestionReceipt() accepted oversized input")
	}
}

func TestArchiveQuestionRoundValidationRejectsInvalidState(t *testing.T) {
	valid := validArchiveQuestionRound(t)
	tests := []struct {
		name   string
		mutate func(*ArchiveQuestionRound)
	}{
		{name: "schema", mutate: func(round *ArchiveQuestionRound) { round.SchemaVersion = 2 }},
		{name: "archive identity", mutate: func(round *ArchiveQuestionRound) { round.ArchiveSHA256 = strings.Repeat("z", 64) }},
		{name: "counts", mutate: func(round *ArchiveQuestionRound) { round.Complete++ }},
		{name: "sources empty", mutate: func(round *ArchiveQuestionRound) { round.Sources = nil }},
		{name: "sources unsorted", mutate: func(round *ArchiveQuestionRound) {
			round.Sources = append(round.Sources, ArchiveSourceSummary{Source: "zzz", Adapter: "adapter", Entries: 1})
		}},
		{name: "answers missing", mutate: func(round *ArchiveQuestionRound) { round.Answers = round.Answers[:1] }},
		{name: "answer identity", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].QuestionID = ArchiveQuestionSources }},
		{name: "answer archive identity", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].ArchiveSHA256 = strings.Repeat("c", 64) }},
		{name: "answer counts", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].Entries++ }},
		{name: "answer evidence", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].EvidenceState = evidence.Inferred }},
		{name: "answer result", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].Result = "same" }},
		{name: "answer sources", mutate: func(round *ArchiveQuestionRound) { round.Answers[0].Sources = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneArchiveQuestionRound(valid)
			test.mutate(&candidate)
			if _, err := ArchiveQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("ArchiveQuestionRoundSHA256() accepted invalid round")
			}
		})
	}
}

func TestArchiveQuestionReceiptValidationRejectsInvalidState(t *testing.T) {
	valid := archiveQuestionReceiptFromAnswer(validArchiveQuestionRound(t).Answers[1], strings.Repeat("b", 64))
	tests := []struct {
		name   string
		mutate func(*ArchiveQuestionReceipt)
	}{
		{name: "schema", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.SchemaVersion = 2 }},
		{name: "question", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.QuestionID = "not-a-question" }},
		{name: "question text", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.Question = "other" }},
		{name: "result", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.Result = "complete" }},
		{name: "evidence", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.EvidenceState = evidence.Inferred }},
		{name: "archive identity", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.ArchiveSHA256 = strings.Repeat("z", 64) }},
		{name: "round identity", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.RoundSHA256 = strings.Repeat("z", 64) }},
		{name: "source", mutate: func(receipt *ArchiveQuestionReceipt) { receipt.Sources = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.mutate(&candidate)
			if _, err := ArchiveQuestionReceiptSHA256(candidate); err == nil {
				t.Fatal("ArchiveQuestionReceiptSHA256() accepted invalid receipt")
			}
		})
	}
}

func validArchiveQuestionRound(t *testing.T) ArchiveQuestionRound {
	t.Helper()
	root := t.TempDir()
	archivePath := filepath.Join(root, "archive.json")
	input := writeStandaloneArchiveInput(t, root, "one", validArchiveTrace("region"), strings.Repeat("a", 64))
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	round, _, err := ReadArchiveQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	return round
}

func cloneArchiveQuestionRound(round ArchiveQuestionRound) ArchiveQuestionRound {
	data, err := json.Marshal(round)
	if err != nil {
		panic(err)
	}
	var clone ArchiveQuestionRound
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

func TestArchiveQuestionRoundAndReceiptErrorsDoNotExposePrivateDetails(t *testing.T) {
	root := t.TempDir()
	if _, _, err := ReadArchiveQuestionRound(filepath.Join(root, "missing.json")); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("round error = %v", err)
	}
	if _, _, err := ReadArchiveQuestionReceipt(filepath.Join(root, "missing.json")); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("receipt error = %v", err)
	}
	if _, err := ArchiveQuestionRoundSHA256(ArchiveQuestionRound{}); err == nil {
		t.Fatal("ArchiveQuestionRoundSHA256() accepted an empty round")
	}
	if _, err := ArchiveQuestionReceiptSHA256(ArchiveQuestionReceipt{}); err == nil {
		t.Fatal("ArchiveQuestionReceiptSHA256() accepted an empty receipt")
	}
}
