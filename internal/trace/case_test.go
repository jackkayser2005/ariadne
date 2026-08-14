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

func TestCaseLifecycleAndQuestionRound(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	ledgerPath, ledgerRoundPath := writeCaseLedger(t)
	casePath := filepath.Join(root, "case.json")
	inputs := []CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}

	summary, err := SaveCase(inputs, casePath)
	if err != nil {
		t.Fatalf("SaveCase() error = %v", err)
	}
	if summary.Entries != 2 || summary.Archives != 1 || summary.Replications != 1 || summary.UnknownEntries != 0 || len(summary.Sources) != 2 || len(summary.Outcomes) != 1 || len(summary.EntrySummaries) != 2 || !ValidSHA256(summary.CaseSHA256) {
		t.Fatalf("case summary = %#v", summary)
	}

	casePackage, readSummary, err := ReadCase(casePath)
	if err != nil {
		t.Fatalf("ReadCase() error = %v", err)
	}
	if !reflectCaseSummaryEqual(readSummary, summary) || len(casePackage.Entries) != 2 {
		t.Fatalf("ReadCase() = %#v, %#v", casePackage, readSummary)
	}
	if verified, err := VerifyCase(casePath); err != nil || !reflectCaseSummaryEqual(verified, summary) {
		t.Fatalf("VerifyCase() = %#v, %v", verified, err)
	}

	answers, err := AskAllCaseQuestions(casePath)
	if err != nil || len(answers) != len(CaseQuestions()) {
		t.Fatalf("AskAllCaseQuestions() = %#v, %v", answers, err)
	}
	if answers[0].Result != "available" || answers[1].Result != "available" || answers[2].Result != "supported" || answers[2].EvidenceState != evidence.Observed {
		t.Fatalf("case answers = %#v", answers)
	}
	answer, err := AskCaseQuestion(casePath, CaseQuestionOutcomes)
	if err != nil || answer.Replications != 1 || answer.Outcomes[0].Outcome != ReplicatedChange {
		t.Fatalf("AskCaseQuestion() = %#v, %v", answer, err)
	}
	if _, err := AskCaseQuestion(casePath, "not-a-question"); err == nil {
		t.Fatal("AskCaseQuestion() accepted an invalid question")
	}

	roundPath := filepath.Join(root, "case-round.json")
	roundSummary, err := SaveCaseQuestionRound(casePath, roundPath)
	if err != nil {
		t.Fatalf("SaveCaseQuestionRound() error = %v", err)
	}
	if roundSummary.Questions != len(CaseQuestions()) || !ValidSHA256(roundSummary.RoundSHA256) || roundSummary.CaseSHA256 != summary.CaseSHA256 {
		t.Fatalf("case round summary = %#v", roundSummary)
	}
	round, readRoundSummary, err := ReadCaseQuestionRound(roundPath)
	if err != nil || len(round.Answers) != len(CaseQuestions()) || readRoundSummary != roundSummary {
		t.Fatalf("ReadCaseQuestionRound() = %#v, %#v, %v", round, readRoundSummary, err)
	}
	if verified, err := VerifyCaseQuestionRound(roundPath); err != nil || verified != roundSummary {
		t.Fatalf("VerifyCaseQuestionRound() = %#v, %v", verified, err)
	}
	if _, err := SaveCaseQuestionRound(casePath, roundPath); err == nil {
		t.Fatal("SaveCaseQuestionRound() overwrote an existing round")
	}
	if _, err := SaveCase(inputs, casePath); err == nil {
		t.Fatal("SaveCase() overwrote an existing case")
	}

	if _, err := AnswerCase(casePackage, summary, CaseQuestionSupport); err != nil {
		t.Fatalf("AnswerCase() error = %v", err)
	}
	if _, err := AnswerCase(casePackage, CaseVerificationSummary{}, CaseQuestionSupport); err == nil {
		t.Fatal("AnswerCase() accepted a mismatched summary")
	}
	if _, err := AnswerAllCaseQuestions(casePackage, CaseVerificationSummary{}); err == nil {
		t.Fatal("AnswerAllCaseQuestions() accepted a mismatched summary")
	}
	if _, err := CaseSHA256(casePackage); err != nil {
		t.Fatalf("CaseSHA256() error = %v", err)
	}

	archiveOnlyPath := filepath.Join(root, "archive-only-case.json")
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, archiveOnlyPath); err != nil {
		t.Fatal(err)
	}
	archiveOnlyAnswers, err := AskAllCaseQuestions(archiveOnlyPath)
	if err != nil || archiveOnlyAnswers[1].Result != "unknown" || archiveOnlyAnswers[1].EvidenceState != evidence.Unknown {
		t.Fatalf("archive-only answers = %#v, %v", archiveOnlyAnswers, err)
	}
}

func TestCaseUnknownSupportRemainsAnOutcome(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	ledgerPath, ledgerRoundPath := writeCaseLedgerWithUnknown(t)
	casePath := filepath.Join(root, "case.json")
	summary, err := SaveCase([]CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceReplication, ArtifactPath: ledgerPath, QuestionRoundPath: ledgerRoundPath},
	}, casePath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UnknownEntries != 1 || summary.Outcomes[0].Outcome != ReplicationUnknown || summary.Outcomes[0].EvidenceState != evidence.Unknown {
		t.Fatalf("unknown case summary = %#v", summary)
	}
	answers, err := AskAllCaseQuestions(casePath)
	if err != nil || answers[1].Result != "available" || answers[1].EvidenceState != evidence.Unknown || answers[2].Result != "unknown" {
		t.Fatalf("unknown case answers = %#v, %v", answers, err)
	}
}

func TestCaseRejectsInvalidInputsAndDocuments(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	validPath := filepath.Join(root, "valid.json")
	validSummary, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, validPath)
	if err != nil {
		t.Fatal(err)
	}
	valid, _, err := ReadCase(validPath)
	if err != nil {
		t.Fatal(err)
	}

	for name, inputs := range map[string][]CaseInput{
		"none":                  nil,
		"kind":                  {{Kind: "other", ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}},
		"missing artifact":      {{Kind: CaseEntryTraceArchive, QuestionRoundPath: archiveRoundPath}},
		"missing round":         {{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath}},
		"missing archive file":  {{Kind: CaseEntryTraceArchive, ArtifactPath: filepath.Join(root, "missing-archive.json"), QuestionRoundPath: archiveRoundPath}},
		"missing archive round": {{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: filepath.Join(root, "missing-archive-round.json")}},
		"missing replication":   {{Kind: CaseEntryTraceReplication, ArtifactPath: filepath.Join(root, "missing-ledger.json"), QuestionRoundPath: filepath.Join(root, "missing-ledger-round.json")}},
	} {
		t.Run("save "+name, func(t *testing.T) {
			if _, err := SaveCase(inputs, filepath.Join(t.TempDir(), "case.json")); err == nil {
				t.Fatal("SaveCase() accepted invalid input")
			}
		})
	}
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, " "); err == nil {
		t.Fatal("SaveCase() accepted an empty output path")
	}
	tooMany := make([]CaseInput, maxCaseEntries+1)
	if _, err := SaveCase(tooMany, filepath.Join(root, "too-many.json")); err == nil {
		t.Fatal("SaveCase() accepted too many entries")
	}
	if _, err := SaveCase([]CaseInput{
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
		{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath},
	}, filepath.Join(root, "duplicate-save.json")); err == nil {
		t.Fatal("SaveCase() accepted duplicate artifact inputs")
	}
	otherRoot := t.TempDir()
	otherDocument := validArchiveTrace("region")
	otherDocument.Events[0].Source = "browser"
	otherInput := writeStandaloneArchiveInputWithAdapter(t, otherRoot, "other-archive", otherDocument, "browser-redacted-audit", strings.Repeat("1", 64))
	otherArchivePath := filepath.Join(otherRoot, "other-archive.json")
	if _, err := SaveArchive([]ArchiveInput{otherInput}, otherArchivePath); err != nil {
		t.Fatal(err)
	}
	otherArchiveRoundPath := filepath.Join(otherRoot, "other-archive-round.json")
	if _, err := SaveArchiveQuestionRound(otherArchivePath, otherArchiveRoundPath); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: otherArchiveRoundPath}}, filepath.Join(root, "mismatched-archive-round.json")); err == nil {
		t.Fatal("SaveCase() accepted a mismatched archive round")
	}
	otherLedgerPath, otherLedgerRoundPath := writeCaseLedger(t)
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceReplication, ArtifactPath: otherLedgerPath, QuestionRoundPath: otherLedgerRoundPath}}, filepath.Join(root, "valid-other-ledger.json")); err != nil {
		t.Fatal(err)
	}
	firstLedgerPath, _ := writeCaseLedger(t)
	unknownLedgerPath, unknownLedgerRoundPath := writeCaseLedgerWithUnknown(t)
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceReplication, ArtifactPath: firstLedgerPath, QuestionRoundPath: unknownLedgerRoundPath}}, filepath.Join(root, "mismatched-ledger-round.json")); err == nil {
		t.Fatal("SaveCase() accepted a mismatched replication round")
	}
	_ = unknownLedgerPath
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceReplication, ArtifactPath: otherLedgerPath, QuestionRoundPath: filepath.Join(root, "missing-ledger-round.json")}}, filepath.Join(root, "missing-ledger-round-case.json")); err == nil {
		t.Fatal("SaveCase() accepted a missing replication round")
	}
	if _, _, err := ReadCase(""); err == nil {
		t.Fatal("ReadCase() accepted an empty path")
	}
	if _, err := VerifyCase(filepath.Join(root, "missing.json")); err == nil {
		t.Fatal("VerifyCase() accepted a missing path")
	}
	if _, _, err := ReadCase(root); err == nil {
		t.Fatal("ReadCase() accepted a directory")
	}
	badCasePath := filepath.Join(root, "bad-case.json")
	if err := os.WriteFile(badCasePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCase(badCasePath); err == nil {
		t.Fatal("ReadCase() accepted malformed input")
	}

	clone := cloneCase(valid)
	mutations := []struct {
		name   string
		mutate func(*CasePackage)
	}{
		{name: "schema", mutate: func(candidate *CasePackage) { candidate.SchemaVersion = 2 }},
		{name: "order", mutate: func(candidate *CasePackage) { candidate.OrderBasis = "chronology" }},
		{name: "position", mutate: func(candidate *CasePackage) { candidate.Entries[0].Position = 2 }},
		{name: "kind", mutate: func(candidate *CasePackage) { candidate.Entries[0].Kind = "other" }},
		{name: "missing archive", mutate: func(candidate *CasePackage) { candidate.Entries[0].Archive = nil }},
		{name: "missing round", mutate: func(candidate *CasePackage) { candidate.Entries[0].ArchiveQuestionRound = nil }},
		{name: "replication field", mutate: func(candidate *CasePackage) { candidate.Entries[0].ReplicationLedger = &ReplicationLedger{} }},
		{name: "duplicate artifact", mutate: func(candidate *CasePackage) {
			candidate.Entries = append(candidate.Entries, candidate.Entries[0])
			candidate.Entries[1].Position = 2
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneCase(clone)
			test.mutate(&candidate)
			if _, err := CaseSHA256(candidate); err == nil {
				t.Fatal("CaseSHA256() accepted invalid package")
			}
		})
	}

	data, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	malformed := [][]byte{
		{},
		append(append([]byte(nil), data...), []byte("{}")...),
		[]byte(`{"schema_version":1,"order_basis":"caller","entries":[],"extra":true}`),
		[]byte(`{"schema_version":1,"order_basis":"caller","entries":[{"position":1,"kind":"trace-archive","archive":null,"archive_question_round":null},{"position":1,"kind":"trace-archive","archive":null,"archive_question_round":null}]}`),
	}
	for index, candidate := range malformed {
		if _, err := DecodeCase(candidate); err == nil {
			t.Fatalf("DecodeCase() accepted malformed document %d", index)
		}
	}
	if _, err := DecodeCase([]byte(`{"schema_version":1,"order_basis":"caller","entries":[{"position":1,"kind":"trace-archive","archive":null,"archive_question_round":null}]}`)); err == nil {
		t.Fatal("DecodeCase() accepted null variants")
	}
	if _, err := DecodeCase([]byte(`{"schema_version":1,"schema_version":1,"order_basis":"caller","entries":[]}`)); err == nil {
		t.Fatal("DecodeCase() accepted duplicate keys")
	}
	if _, err := DecodeCase(bytes.Repeat([]byte("x"), maxCaseBytes+1)); err == nil {
		t.Fatal("DecodeCase() accepted oversized input")
	}

	badRound := CaseQuestionRound{}
	if _, err := CaseQuestionRoundSHA256(badRound); err == nil {
		t.Fatal("CaseQuestionRoundSHA256() accepted an invalid round")
	}
	if _, _, err := ReadCaseQuestionRound(""); err == nil {
		t.Fatal("ReadCaseQuestionRound() accepted an empty path")
	}
	if _, err := VerifyCaseQuestionRound(filepath.Join(root, "missing-round.json")); err == nil {
		t.Fatal("VerifyCaseQuestionRound() accepted a missing path")
	}
	if _, err := SaveCaseQuestionRound(validPath, " "); err == nil {
		t.Fatal("SaveCaseQuestionRound() accepted an empty output path")
	}
	if _, err := SaveCaseQuestionRound(filepath.Join(root, "missing-case.json"), filepath.Join(root, "missing-case-round.json")); err == nil {
		t.Fatal("SaveCaseQuestionRound() accepted a missing case")
	}
	if _, err := AskCaseQuestion(filepath.Join(root, "missing-case.json"), CaseQuestionSources); err == nil {
		t.Fatal("AskCaseQuestion() accepted a missing case")
	}
	if _, err := AskAllCaseQuestions(filepath.Join(root, "missing-case.json")); err == nil {
		t.Fatal("AskAllCaseQuestions() accepted a missing case")
	}

	validRoundPath := filepath.Join(root, "valid-round.json")
	if _, err := SaveCaseQuestionRound(validPath, validRoundPath); err != nil {
		t.Fatal(err)
	}
	round, _, err := ReadCaseQuestionRound(validRoundPath)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CaseQuestionRound){
		"schema":          func(candidate *CaseQuestionRound) { candidate.SchemaVersion = 2 },
		"order":           func(candidate *CaseQuestionRound) { candidate.OrderBasis = "chronology" },
		"identity":        func(candidate *CaseQuestionRound) { candidate.CaseSHA256 = strings.Repeat("z", 64) },
		"entries":         func(candidate *CaseQuestionRound) { candidate.Entries++ },
		"answers":         func(candidate *CaseQuestionRound) { candidate.Answers = candidate.Answers[:1] },
		"answer identity": func(candidate *CaseQuestionRound) { candidate.Answers[0].QuestionID = CaseQuestionSupport },
		"answer result":   func(candidate *CaseQuestionRound) { candidate.Answers[0].Result = "unsupported" },
		"answer evidence": func(candidate *CaseQuestionRound) { candidate.Answers[0].EvidenceState = evidence.Inferred },
		"answer source":   func(candidate *CaseQuestionRound) { candidate.Answers[0].Sources = nil },
	} {
		t.Run("round "+name, func(t *testing.T) {
			candidate := cloneCaseQuestionRound(round)
			mutate(&candidate)
			if _, err := CaseQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("CaseQuestionRoundSHA256() accepted invalid round")
			}
		})
	}
	if _, err := DecodeCaseQuestionRound(bytes.Repeat([]byte("x"), maxCaseBytes+1)); err == nil {
		t.Fatal("DecodeCaseQuestionRound() accepted oversized input")
	}
	badRoundPath := filepath.Join(root, "bad-round.json")
	if err := os.WriteFile(badRoundPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCaseQuestionRound(badRoundPath); err == nil {
		t.Fatal("ReadCaseQuestionRound() accepted malformed input")
	}
	roundData, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string][]byte{
		"trailing":  append(append([]byte(nil), roundData...), []byte("{}")...),
		"unknown":   append(append([]byte(nil), roundData[:len(roundData)-1]...), []byte(`,"extra":true}`)...),
		"duplicate": []byte(`{"schema_version":1,"schema_version":1,"order_basis":"caller","case_sha256":"` + round.CaseSHA256 + `","entries":1,"answers":[]}`),
	} {
		if _, err := DecodeCaseQuestionRound(candidate); err == nil {
			t.Fatalf("DecodeCaseQuestionRound() accepted %s input", name)
		}
	}
	for name, mutate := range map[string]func(*CaseQuestionRound){
		"answer counts": func(candidate *CaseQuestionRound) { candidate.Answers[0].Archives++ },
		"outcome no replication": func(candidate *CaseQuestionRound) {
			candidate.Answers[1].Archives = candidate.Entries
			candidate.Answers[1].Replications = 0
			candidate.Answers[1].Result = "available"
		},
	} {
		t.Run("round "+name, func(t *testing.T) {
			candidate := cloneCaseQuestionRound(round)
			mutate(&candidate)
			if _, err := CaseQuestionRoundSHA256(candidate); err == nil {
				t.Fatal("CaseQuestionRoundSHA256() accepted invalid answer")
			}
		})
	}
	if validSummary.CaseSHA256 == "" {
		t.Fatal("valid case has no identity")
	}
}

func TestCaseRoundAndEntryBindingRejectTampering(t *testing.T) {
	root := t.TempDir()
	archivePath, archiveRoundPath := writeCaseArchive(t, root)
	casePackagePath := filepath.Join(root, "case.json")
	if _, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: archivePath, QuestionRoundPath: archiveRoundPath}}, casePackagePath); err != nil {
		t.Fatal(err)
	}
	casePackage, summary, err := ReadCase(casePackagePath)
	if err != nil {
		t.Fatal(err)
	}
	archiveRound := cloneCaseArchiveQuestionRound(*casePackage.Entries[0].ArchiveQuestionRound)
	archiveRound.Answers[0].Reason = "tampered"
	casePackage.Entries[0].ArchiveQuestionRound = &archiveRound
	if _, err := CaseSHA256(casePackage); err == nil {
		t.Fatal("CaseSHA256() accepted a tampered archive round")
	}
	casePackage, _, err = ReadCase(casePackagePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerCase(casePackage, summary, "not-a-question"); err == nil {
		t.Fatal("AnswerCase() accepted an invalid question")
	}

	data, err := os.ReadFile(casePackagePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCase(data); err != nil {
		t.Fatal(err)
	}
	archiveRound.ArchiveSHA256 = strings.Repeat("2", 64)
	archiveRoundSummary, err := archiveQuestionRoundSummaryChecked(archiveRound)
	if err == nil {
		archiveMeta, summaryErr := archiveSummary(*casePackage.Entries[0].Archive)
		if summaryErr != nil {
			t.Fatal(summaryErr)
		}
		if err := validateCaseArchiveRound(*casePackage.Entries[0].Archive, archiveMeta, archiveRound, archiveRoundSummary); err == nil {
			t.Fatal("validateCaseArchiveRound() accepted mismatched archive identity")
		}
	}
	archiveMeta, err := archiveSummary(*casePackage.Entries[0].Archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCaseArchiveRound(*casePackage.Entries[0].Archive, archiveMeta, *casePackage.Entries[0].ArchiveQuestionRound, ArchiveQuestionRoundVerificationSummary{ArchiveSHA256: archiveMeta.ArchiveSHA256, Questions: len(ArchiveQuestions()), RoundSHA256: strings.Repeat("3", 64)}); err == nil {
		t.Fatal("validateCaseArchiveRound() accepted mismatched round identity")
	}
}

func TestCaseAdditionalBoundaries(t *testing.T) {
	root := t.TempDir()
	partialOne := writeStandaloneArchiveInput(t, root, "partial-one", func() Document {
		document := validArchiveTrace("region")
		document.Completeness = Partial
		return document
	}(), strings.Repeat("4", 64))
	partialTwo := writeStandaloneArchiveInput(t, root, "partial-two", func() Document {
		document := validArchiveTrace("region")
		document.Completeness = Partial
		return document
	}(), strings.Repeat("4", 64))
	partialArchivePath := filepath.Join(root, "partial-archive.json")
	if _, err := SaveArchive([]ArchiveInput{partialOne, partialTwo}, partialArchivePath); err != nil {
		t.Fatal(err)
	}
	partialRoundPath := filepath.Join(root, "partial-round.json")
	if _, err := SaveArchiveQuestionRound(partialArchivePath, partialRoundPath); err != nil {
		t.Fatal(err)
	}
	partialCasePath := filepath.Join(root, "partial-case.json")
	partialSummary, err := SaveCase([]CaseInput{{Kind: CaseEntryTraceArchive, ArtifactPath: partialArchivePath, QuestionRoundPath: partialRoundPath}}, partialCasePath)
	if err != nil || partialSummary.UnknownEntries != 1 {
		t.Fatalf("partial case = %#v, %v", partialSummary, err)
	}

	if _, err := validateCaseEntry(CaseEntry{Kind: CaseEntryTraceArchive}); err == nil {
		t.Fatal("validateCaseEntry() accepted no variants")
	}
	if err := validateCase(CasePackage{SchemaVersion: caseSchemaVersion, OrderBasis: "caller"}); err == nil {
		t.Fatal("validateCase() accepted no entries")
	}
	if _, err := validateCaseEntry(CaseEntry{Kind: CaseEntryTraceReplication, Archive: &Archive{}}); err == nil {
		t.Fatal("validateCaseEntry() accepted a replication entry with archive fields")
	}

	ledgerPath, _ := writeCaseLedger(t)
	ledger, ledgerSummary, err := ReadReplicationLedger(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	ledgerRoundPath := filepath.Join(filepath.Dir(ledgerPath), "ledger-round-extra.json")
	if _, err := SaveReplicationQuestionRound(ledgerPath, ledgerRoundPath); err != nil {
		t.Fatal(err)
	}
	ledgerRound, _, err := ReadReplicationQuestionRound(ledgerRoundPath)
	if err != nil {
		t.Fatal(err)
	}
	wrongLedgerRound := cloneReplicationQuestionRound(ledgerRound)
	wrongLedgerRound.LedgerSHA256 = strings.Repeat("2", 64)
	if err := validateCaseReplicationRound(ledger, ledgerSummary, wrongLedgerRound, ReplicationQuestionRoundVerificationSummary{LedgerSHA256: wrongLedgerRound.LedgerSHA256, Questions: len(ReplicationQuestions()), RoundSHA256: strings.Repeat("3", 64)}); err == nil {
		t.Fatal("validateCaseReplicationRound() accepted mismatched ledger identity")
	}
	if err := validateCaseReplicationRound(ledger, ledgerSummary, ledgerRound, ReplicationQuestionRoundVerificationSummary{LedgerSHA256: ledgerSummary.LedgerSHA256, Questions: len(ReplicationQuestions()), RoundSHA256: strings.Repeat("3", 64)}); err == nil {
		t.Fatal("validateCaseReplicationRound() accepted mismatched round identity")
	}
	if _, err := replicationQuestionRoundSummaryChecked(ReplicationQuestionRound{}); err == nil {
		t.Fatal("replicationQuestionRoundSummaryChecked() accepted an invalid round")
	}
	if _, err := archiveQuestionRoundSummaryChecked(ArchiveQuestionRound{}); err == nil {
		t.Fatal("archiveQuestionRoundSummaryChecked() accepted an invalid round")
	}

	if compareCaseSources(CaseSourceSummary{Source: "android", Adapter: "z"}, CaseSourceSummary{Source: "android", Adapter: "a"}) <= 0 {
		t.Fatal("compareCaseSources() did not order adapters")
	}
	if compareCaseSources(CaseSourceSummary{Source: "z", Adapter: "a"}, CaseSourceSummary{Source: "android", Adapter: "a"}) <= 0 {
		t.Fatal("compareCaseSources() did not order sources")
	}
	if compareCaseSources(CaseSourceSummary{Source: "android", Adapter: "a"}, CaseSourceSummary{Source: "android", Adapter: "a"}) != 0 {
		t.Fatal("compareCaseSources() did not identify equal sources")
	}
	partialRound, _, err := ReadArchiveQuestionRound(partialRoundPath)
	if err != nil {
		t.Fatal(err)
	}
	if caseRoundEvidenceStateArchive(partialRound) != evidence.Unknown {
		t.Fatal("caseRoundEvidenceStateArchive() accepted partial support")
	}
	if err := writeCaseExclusive(filepath.Join(root, "too-large-case.json"), bytes.Repeat([]byte("x"), maxCaseBytes+1)); err == nil {
		t.Fatal("writeCaseExclusive() accepted oversized output")
	}
	if err := writeCaseExclusive("", nil); err == nil {
		t.Fatal("writeCaseExclusive() accepted an empty path")
	}
	parentFile := filepath.Join(root, "parent-file")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeCaseExclusive(filepath.Join(parentFile, "child.json"), nil); err == nil {
		t.Fatal("writeCaseExclusive() accepted a file as a directory")
	}
	existing := filepath.Join(root, "existing-case.json")
	if err := os.WriteFile(existing, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeCaseExclusive(existing, nil); err == nil {
		t.Fatal("writeCaseExclusive() overwrote an existing output")
	}
}

func writeCaseArchive(t *testing.T, root string) (string, string) {
	t.Helper()
	first := writeStandaloneArchiveInput(t, root, "case-archive-first", validArchiveTrace("region"), strings.Repeat("1", 64))
	second := writeStandaloneArchiveInput(t, root, "case-archive-second", validArchiveTrace("region"), strings.Repeat("1", 64))
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{first, second}, archivePath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "archive-round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	return archivePath, roundPath
}

func writeCaseLedger(t *testing.T) (string, string) {
	t.Helper()
	ledgerPath, _ := writeQuestionLedger(t, true, true)
	roundPath := filepath.Join(filepath.Dir(ledgerPath), "ledger-round.json")
	if _, err := SaveReplicationQuestionRound(ledgerPath, roundPath); err != nil {
		t.Fatal(err)
	}
	return ledgerPath, roundPath
}

func writeCaseLedgerWithUnknown(t *testing.T) (string, string) {
	t.Helper()
	ledgerPath, _ := writeQuestionLedgerWithChanges(t, true, true, false, true, true)
	roundPath := filepath.Join(filepath.Dir(ledgerPath), "ledger-round.json")
	if _, err := SaveReplicationQuestionRound(ledgerPath, roundPath); err != nil {
		t.Fatal(err)
	}
	return ledgerPath, roundPath
}

func cloneCase(casePackage CasePackage) CasePackage {
	data, err := json.Marshal(casePackage)
	if err != nil {
		panic(err)
	}
	var clone CasePackage
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

func cloneCaseQuestionRound(round CaseQuestionRound) CaseQuestionRound {
	data, err := json.Marshal(round)
	if err != nil {
		panic(err)
	}
	var clone CaseQuestionRound
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(err)
	}
	return clone
}

func cloneCaseArchiveQuestionRound(round ArchiveQuestionRound) ArchiveQuestionRound {
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

func reflectCaseSummaryEqual(left, right CaseVerificationSummary) bool {
	return reflect.DeepEqual(left, right)
}
