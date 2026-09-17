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

func TestCaseDisclosureQuestionRoundAndReceiptLifecycle(t *testing.T) {
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

	round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
	if err != nil {
		t.Fatal(err)
	}
	if round.SchemaVersion != 1 || round.OrderBasis != "caller" || round.CaseSHA256 != summary.CaseSHA256 || round.Traces != 6 || round.CoverageState != evidence.Observed || len(round.Answers) != 2 {
		t.Fatalf("disclosure question round = %#v", round)
	}
	if len(round.Categories) != 2 || round.Categories[0].Category != "consent" || round.Categories[1].Category != "region" {
		t.Fatalf("round categories = %#v", round.Categories)
	}
	if got := round.Answers[0]; got.Result != "complete" || got.EvidenceState != evidence.Observed || len(got.OverlappingCategories) != 1 || got.OverlappingCategories[0] != "region" {
		t.Fatalf("coverage answer = %#v", got)
	}
	if got := round.Answers[1]; got.Result != "overlap-observed" || got.EvidenceState != evidence.Observed || !reflect.DeepEqual(got.OverlappingCategories, []string{"region"}) {
		t.Fatalf("overlap answer = %#v", got)
	}

	roundPath := filepath.Join(t.TempDir(), "disclosure-round.json")
	roundSummary, err := SaveCaseDisclosureQuestionRound(casePath, roundPath)
	if err != nil {
		t.Fatal(err)
	}
	readRound, verifiedRound, err := ReadCaseDisclosureQuestionRound(roundPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readRound, round) || verifiedRound != roundSummary {
		t.Fatalf("saved round = %#v, summary = %#v, want %#v", readRound, verifiedRound, round)
	}
	if verified, err := VerifyCaseDisclosureQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyCaseDisclosureQuestionRound() = %#v, %v", verified, err)
	}
	answer, err := AskCaseDisclosureQuestionRound(roundPath, CaseDisclosureQuestionOverlap)
	if err != nil || !reflect.DeepEqual(answer, round.Answers[1]) {
		t.Fatalf("AskCaseDisclosureQuestionRound() = %#v, %v", answer, err)
	}

	receiptPath := filepath.Join(t.TempDir(), "disclosure-receipt.json")
	receiptSummary, err := SaveCaseDisclosureQuestionReceipt(roundPath, CaseDisclosureQuestionOverlap, receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt, verifiedReceipt, err := ReadCaseDisclosureQuestionReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RoundSHA256 != roundSummary.RoundSHA256 || !reflect.DeepEqual(receipt.Round, round) || verifiedReceipt != receiptSummary {
		t.Fatalf("saved receipt = %#v, summary = %#v", receipt, verifiedReceipt)
	}
	if verified, err := VerifyCaseDisclosureQuestionReceipt(receiptPath); err != nil || verified != receiptSummary {
		t.Fatalf("VerifyCaseDisclosureQuestionReceipt() = %#v, %v", verified, err)
	}
	selected, err := AskCaseDisclosureQuestionReceipt(roundPath, CaseDisclosureQuestionOverlap)
	if err != nil || !reflect.DeepEqual(selected, receipt) {
		t.Fatalf("AskCaseDisclosureQuestionReceipt() = %#v, %v", selected, err)
	}

	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-value", "target-device", "private-arg", "https://", "coordinates"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("disclosure receipt contains forbidden value %q: %s", forbidden, data)
		}
	}
}

func TestCaseDisclosureQuestionSemanticsForDisjointAndPartialMaps(t *testing.T) {
	t.Run("complete disjoint", func(t *testing.T) {
		archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
		casePath := filepath.Join(t.TempDir(), "case.json")
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
		if got := round.Answers[0]; got.Result != "complete" || got.EvidenceState != evidence.Observed {
			t.Fatalf("coverage answer = %#v", got)
		}
		if got := round.Answers[1]; got.Result != "no-overlap-observed" || got.EvidenceState != evidence.Observed || len(got.OverlappingCategories) != 0 {
			t.Fatalf("overlap answer = %#v", got)
		}
	})

	t.Run("partial disjoint", func(t *testing.T) {
		casePath := writeDisclosureCaseWithPartialArchive(t, false)
		casePackage, summary, err := ReadCase(casePath)
		if err != nil {
			t.Fatal(err)
		}
		round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
		if err != nil {
			t.Fatal(err)
		}
		if got := round.Answers[0]; got.Result != "unknown" || got.EvidenceState != evidence.Unknown {
			t.Fatalf("coverage answer = %#v", got)
		}
		if got := round.Answers[1]; got.Result != "unknown" || got.EvidenceState != evidence.Unknown || len(got.OverlappingCategories) != 0 {
			t.Fatalf("overlap answer = %#v", got)
		}
	})

	t.Run("partial positive overlap", func(t *testing.T) {
		casePath := writeDisclosureCaseWithPartialArchive(t, true)
		casePackage, summary, err := ReadCase(casePath)
		if err != nil {
			t.Fatal(err)
		}
		round, err := AnswerCaseDisclosureQuestionRound(casePackage, summary)
		if err != nil {
			t.Fatal(err)
		}
		if round.CoverageState != evidence.Unknown {
			t.Fatalf("round coverage = %s", round.CoverageState)
		}
		if got := round.Answers[1]; got.Result != "overlap-observed" || got.EvidenceState != evidence.Observed || !reflect.DeepEqual(got.OverlappingCategories, []string{"region"}) {
			t.Fatalf("overlap answer = %#v", got)
		}
	})
}

func TestCaseDisclosureQuestionValidationRejectsTampering(t *testing.T) {
	archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
	casePath := filepath.Join(t.TempDir(), "case.json")
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

	for _, test := range []struct {
		name   string
		mutate func(*CaseDisclosureQuestionRound)
	}{
		{name: "unsafe category", mutate: func(round *CaseDisclosureQuestionRound) { round.Categories[0].Category = "https://private" }},
		{name: "wrong result", mutate: func(round *CaseDisclosureQuestionRound) { round.Answers[0].Result = "no-overlap-observed" }},
		{name: "wrong overlap category", mutate: func(round *CaseDisclosureQuestionRound) { round.Answers[1].OverlappingCategories = []string{"email"} }},
		{name: "wrong round case", mutate: func(round *CaseDisclosureQuestionRound) { round.CaseSHA256 = strings.Repeat("a", 64) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneCaseDisclosureQuestionRound(round)
			test.mutate(&candidate)
			if _, err := CaseDisclosureQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("CaseDisclosureQuestionRoundSHA256() accepted tampered round")
			}
		})
	}

	if _, err := AnswerCaseDisclosureQuestionRound(casePackage, CaseVerificationSummary{}); err == nil {
		t.Fatal("AnswerCaseDisclosureQuestionRound() accepted mismatched summary")
	}
	if _, err := AskCaseDisclosureQuestionRound("missing-round.json", "not-a-question"); err == nil {
		t.Fatal("AskCaseDisclosureQuestionRound() accepted an invalid path")
	}
	if _, err := CaseDisclosureQuestionRoundSHA256(CaseDisclosureQuestionRound{}); err == nil {
		t.Fatal("CaseDisclosureQuestionRoundSHA256() accepted an empty round")
	}
}

func TestCaseDisclosureQuestionDecodersRejectMalformedInput(t *testing.T) {
	archivePath, archiveRoundPath := writeCaseArchive(t, t.TempDir())
	casePath := filepath.Join(t.TempDir(), "case.json")
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
	roundData, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	receipt := CaseDisclosureQuestionReceipt{
		CaseDisclosureQuestionAnswer: round.Answers[0],
		RoundSHA256:                  func() string { digest, _ := CaseDisclosureQuestionRoundSHA256(round); return digest }(),
		Round:                        round,
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{nil, []byte(`{"schema_version":1,"schema_version":1}`), append(roundData, []byte("{}")...), append(receiptData, []byte("{}")...)} {
		if _, err := DecodeCaseDisclosureQuestionRound(data); err == nil {
			t.Fatalf("DecodeCaseDisclosureQuestionRound() accepted %q", data)
		}
		if _, err := DecodeCaseDisclosureQuestionReceipt(data); err == nil {
			t.Fatalf("DecodeCaseDisclosureQuestionReceipt() accepted %q", data)
		}
	}
	if _, err := DecodeCaseDisclosureQuestionRound(bytes.Repeat([]byte("x"), maxCaseBytes+1)); err == nil {
		t.Fatal("DecodeCaseDisclosureQuestionRound() accepted oversized input")
	}
	if _, err := DecodeCaseDisclosureQuestionReceipt(bytes.Repeat([]byte("x"), maxCaseBytes+1)); err == nil {
		t.Fatal("DecodeCaseDisclosureQuestionReceipt() accepted oversized input")
	}
}

func writeDisclosureCaseWithPartialArchive(t *testing.T, includeLedger bool) string {
	t.Helper()
	root := t.TempDir()
	document := validArchiveTrace("region")
	document.Completeness = Partial
	input := writeStandaloneArchiveInput(t, root, "partial", document, strings.Repeat("e", 64))
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	archiveRoundPath := filepath.Join(root, "archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, archiveRoundPath); err != nil {
		t.Fatal(err)
	}
	inputs := []CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}
	if includeLedger {
		ledgerPath, ledgerRoundPath := writeCaseLedger(t)
		inputs = append(inputs, CaseInput{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath})
	}
	casePath := filepath.Join(root, "case.json")
	if _, err := SaveCase(inputs, casePath); err != nil {
		t.Fatal(err)
	}
	return casePath
}

func cloneCaseDisclosureQuestionRound(round CaseDisclosureQuestionRound) CaseDisclosureQuestionRound {
	data, err := json.Marshal(round)
	if err != nil {
		panic(err)
	}
	var clone CaseDisclosureQuestionRound
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}
