package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestReplicationStudySaveReadAndVerify(t *testing.T) {
	firstLedger, firstRound := writeStudyFixture(t, "session-id", true, true, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	output := filepath.Join(t.TempDir(), "study.json")
	contrast := strings.Repeat("a", 64)
	summary, err := SaveReplicationStudy(contrast, []StudyInput{
		{LedgerPath: firstLedger, RoundPath: firstRound},
		{LedgerPath: secondLedger, RoundPath: secondRound},
	}, output)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || summary.ContrastSHA256 != contrast || summary.OrderBasis != ReplicationStudyOrderBasis || summary.Runs != 2 || summary.Pairs != 4 || summary.SupportedRuns != 2 || summary.UnknownRuns != 0 || summary.ResetConfirmedPairs != 4 || summary.BalancedRuns != 2 || summary.CompletePairs != 4 || summary.ChangedRuns != 2 || summary.NoChangeRuns != 0 || summary.UnknownPairs != 0 || summary.Outcome != ReplicatedChange || summary.EvidenceState != evidence.Observed || !ValidSHA256(summary.StudySHA256) {
		t.Fatalf("summary = %#v", summary)
	}
	study, verified, err := ReadReplicationStudy(output)
	if err != nil {
		t.Fatal(err)
	}
	if verified != summary || len(study.Runs) != 2 || study.Runs[0].Position != 1 || study.Runs[1].Position != 2 {
		t.Fatalf("study = %#v, summary = %#v", study, verified)
	}
	if got, err := VerifyReplicationStudy(output); err != nil || got != summary {
		t.Fatalf("VerifyReplicationStudy() = %#v, %v", got, err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), firstLedger) || strings.Contains(string(data), firstRound) {
		t.Fatal("study persisted source paths")
	}
	if _, err := SaveReplicationStudy(contrast, []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}, output); err == nil {
		t.Fatal("SaveReplicationStudy() overwrote an existing output")
	}
}

func TestReplicationStudyClassifiesRuns(t *testing.T) {
	tests := []struct {
		name          string
		firstChanged  bool
		secondChanged bool
		firstReset    bool
		wantOutcome   ReplicatedOutcome
		wantEvidence  evidence.State
		wantSupported int
		wantUnknown   int
	}{
		{name: "all changed", firstChanged: true, secondChanged: true, firstReset: true, wantOutcome: ReplicatedChange, wantEvidence: evidence.Observed, wantSupported: 2},
		{name: "all same", firstChanged: false, secondChanged: false, firstReset: true, wantOutcome: NoChangeObserved, wantEvidence: evidence.Observed, wantSupported: 2},
		{name: "mixed", firstChanged: true, secondChanged: false, firstReset: true, wantOutcome: MixedInconsistent, wantEvidence: evidence.Observed, wantSupported: 2},
		{name: "unknown dominates", firstChanged: true, secondChanged: true, firstReset: false, wantOutcome: ReplicationUnknown, wantEvidence: evidence.Unknown, wantSupported: 1, wantUnknown: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			firstLedger, firstRound := writeStudyFixture(t, "session-id", test.firstChanged, test.firstReset, "browser-redacted-audit")
			secondLedger, secondRound := writeStudyFixture(t, "location", test.secondChanged, true, "browser-redacted-audit")
			summary, err := SaveReplicationStudy(strings.Repeat("b", 64), []StudyInput{
				{LedgerPath: firstLedger, RoundPath: firstRound},
				{LedgerPath: secondLedger, RoundPath: secondRound},
			}, filepath.Join(t.TempDir(), "study.json"))
			if err != nil {
				t.Fatal(err)
			}
			if summary.Outcome != test.wantOutcome || summary.EvidenceState != test.wantEvidence || summary.SupportedRuns != test.wantSupported || summary.UnknownRuns != test.wantUnknown {
				t.Fatalf("summary = %#v", summary)
			}
		})
	}
	t.Run("all internally mixed", func(t *testing.T) {
		firstLedger, firstRound := writeMixedStudyFixture(t, "session-id", "browser-redacted-audit")
		secondLedger, secondRound := writeMixedStudyFixture(t, "location", "browser-redacted-audit")
		summary, err := SaveReplicationStudy(strings.Repeat("7", 64), []StudyInput{
			{LedgerPath: firstLedger, RoundPath: firstRound},
			{LedgerPath: secondLedger, RoundPath: secondRound},
		}, filepath.Join(t.TempDir(), "study.json"))
		if err != nil {
			t.Fatal(err)
		}
		if summary.Outcome != MixedInconsistent || summary.MixedRuns != 2 || !strings.Contains(summary.Reason, "internally inconsistent") {
			t.Fatalf("summary = %#v", summary)
		}
	})
}

func TestReplicationStudyPreservesCallerOrder(t *testing.T) {
	firstLedger, firstRound := writeStudyFixture(t, "session-id", true, true, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	firstSummary, err := VerifyReplicationLedger(firstLedger)
	if err != nil {
		t.Fatal(err)
	}
	secondSummary, err := VerifyReplicationLedger(secondLedger)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "study.json")
	if _, err := SaveReplicationStudy(strings.Repeat("c", 64), []StudyInput{
		{LedgerPath: secondLedger, RoundPath: secondRound},
		{LedgerPath: firstLedger, RoundPath: firstRound},
	}, output); err != nil {
		t.Fatal(err)
	}
	study, _, err := ReadReplicationStudy(output)
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity, err := ReplicationLedgerSHA256(study.Runs[0].Ledger)
	if err != nil {
		t.Fatal(err)
	}
	secondIdentity, err := ReplicationLedgerSHA256(study.Runs[1].Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if firstIdentity != secondSummary.LedgerSHA256 || secondIdentity != firstSummary.LedgerSHA256 {
		t.Fatalf("caller order was not retained")
	}
}

func TestReplicationStudyRejectsInvalidInputs(t *testing.T) {
	firstLedger, firstRound := writeStudyFixture(t, "session-id", true, true, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	output := filepath.Join(t.TempDir(), "study.json")
	validInputs := []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}
	for _, test := range []struct {
		name string
		call func() error
		want string
	}{
		{name: "invalid contrast", call: func() error { _, err := SaveReplicationStudy("bad", validInputs, output); return err }, want: "contrast_sha256"},
		{name: "too few runs", call: func() error {
			_, err := SaveReplicationStudy(strings.Repeat("d", 64), validInputs[:1], output)
			return err
		}, want: "run count"},
		{name: "missing path", call: func() error {
			_, err := SaveReplicationStudy(strings.Repeat("d", 64), []StudyInput{{LedgerPath: "", RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}, output)
			return err
		}, want: "paths"},
		{name: "duplicate ledger", call: func() error {
			_, err := SaveReplicationStudy(strings.Repeat("d", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: firstLedger, RoundPath: firstRound}}, output)
			return err
		}, want: "duplicate ledger"},
		{name: "mismatched round", call: func() error {
			_, err := SaveReplicationStudy(strings.Repeat("d", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: secondRound}, {LedgerPath: secondLedger, RoundPath: firstRound}}, output)
			return err
		}, want: "does not match ledger"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	otherLedger, otherRound := writeStudyFixture(t, "location", true, true, "browser-local-fixture")
	if _, err := SaveReplicationStudy(strings.Repeat("e", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: otherLedger, RoundPath: otherRound}}, filepath.Join(t.TempDir(), "provenance.json")); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("provenance mismatch error = %v", err)
	}
	if _, err := SaveReplicationStudy(strings.Repeat("e", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}, ""); err == nil {
		t.Fatal("empty output path was accepted")
	}
}

func TestReplicationStudyDecodeRejectsTamperingAndBounds(t *testing.T) {
	firstLedger, firstRound := writeStudyFixture(t, "session-id", true, true, "browser-redacted-audit")
	secondLedger, secondRound := writeStudyFixture(t, "location", true, true, "browser-redacted-audit")
	output := filepath.Join(t.TempDir(), "study.json")
	if _, err := SaveReplicationStudy(strings.Repeat("f", 64), []StudyInput{{LedgerPath: firstLedger, RoundPath: firstRound}, {LedgerPath: secondLedger, RoundPath: secondRound}}, output); err != nil {
		t.Fatal(err)
	}
	study, _, err := ReadReplicationStudy(output)
	if err != nil {
		t.Fatal(err)
	}
	study.Runs[0].QuestionRound.Answers[0].Result = "wrong"
	tampered, err := json.Marshal(study)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationStudy(tampered); err == nil {
		t.Fatal("DecodeReplicationStudy() accepted tampered answer")
	}
	_, noChangeRound := writeStudyFixture(t, "location", false, true, "browser-redacted-audit")
	fabricatedStudy := study
	fabricatedStudy.Runs = append([]ReplicationStudyRun(nil), study.Runs...)
	fabricatedStudy.Runs[0].QuestionRound = cloneReplicationQuestionRound(study.Runs[0].QuestionRound)
	noChangeRoundDocument, _, err := ReadReplicationQuestionRound(noChangeRound)
	if err != nil {
		t.Fatal(err)
	}
	for index := range noChangeRoundDocument.Answers {
		noChangeRoundDocument.Answers[index].LedgerSHA256 = fabricatedStudy.Runs[0].QuestionRound.LedgerSHA256
	}
	fabricatedStudy.Runs[0].QuestionRound.Answers = noChangeRoundDocument.Answers
	fabricated, err := json.Marshal(fabricatedStudy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeReplicationStudy(fabricated); err == nil {
		t.Fatal("DecodeReplicationStudy() accepted internally consistent fabricated answers")
	}
	if _, err := DecodeReplicationStudy([]byte(`{"schema_version":1,"schema_version":1}`)); err == nil {
		t.Fatal("DecodeReplicationStudy() accepted duplicate keys")
	}
	if _, err := DecodeReplicationStudy([]byte(`{"schema_version":1} trailing`)); err == nil {
		t.Fatal("DecodeReplicationStudy() accepted trailing data")
	}
	if _, err := DecodeReplicationStudy(bytesOfStudyLimit()); err == nil {
		t.Fatal("DecodeReplicationStudy() accepted oversized input")
	}
	if _, err := ReplicationStudySHA256(ReplicationStudy{}); err == nil {
		t.Fatal("ReplicationStudySHA256() accepted invalid study")
	}
	if _, _, err := ReadReplicationStudy(""); err == nil {
		t.Fatal("ReadReplicationStudy() accepted empty path")
	}
}

func writeStudyFixture(t *testing.T, variant string, changed, resetConfirmed bool, adapter string) (string, string) {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("1", 64)
	baseline := replicationTrace("browser", "region", variant)
	treatment := baseline
	if changed {
		treatment = replicationTrace("browser", "region", variant, "consent")
	}
	inputs := []ReplicationPairInput{
		writeReplicationPair(t, root, "forward", baseline, treatment, adapter, procedure, OrderBaselineTreatment, resetConfirmed),
		writeReplicationPair(t, root, "reverse", baseline, treatment, adapter, procedure, OrderTreatmentBaseline, resetConfirmed),
	}
	ledgerPath := filepath.Join(root, "ledger.json")
	if _, err := SaveReplicationLedger(inputs, ledgerPath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "round.json")
	if _, err := SaveReplicationQuestionRound(ledgerPath, roundPath); err != nil {
		t.Fatal(err)
	}
	return ledgerPath, roundPath
}

func writeMixedStudyFixture(t *testing.T, variant, adapter string) (string, string) {
	t.Helper()
	root := t.TempDir()
	procedure := strings.Repeat("2", 64)
	baseline := replicationTrace("browser", "region", variant)
	changedTreatment := replicationTrace("browser", "region", variant, "consent")
	inputs := []ReplicationPairInput{
		writeReplicationPair(t, root, "forward", baseline, changedTreatment, adapter, procedure, OrderBaselineTreatment, true),
		writeReplicationPair(t, root, "reverse", baseline, baseline, adapter, procedure, OrderTreatmentBaseline, true),
	}
	ledgerPath := filepath.Join(root, "ledger.json")
	if _, err := SaveReplicationLedger(inputs, ledgerPath); err != nil {
		t.Fatal(err)
	}
	roundPath := filepath.Join(root, "round.json")
	if _, err := SaveReplicationQuestionRound(ledgerPath, roundPath); err != nil {
		t.Fatal(err)
	}
	return ledgerPath, roundPath
}

func bytesOfStudyLimit() []byte {
	return []byte(strings.Repeat("x", maxReplicationStudyBytes+1))
}
