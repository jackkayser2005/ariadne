package minimize

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestLadderPlanDecodeAndValidate(t *testing.T) {
	plan := testLadderPlan()
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeLadder(bytes.NewReader(data))
	if err != nil || decoded.Name != plan.Name || len(decoded.Candidates) != 2 {
		t.Fatalf("DecodeLadder() = %#v, err=%v", decoded, err)
	}
	if _, err := ReadLadder(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadLadder() accepted a missing path")
	}
	for _, test := range []struct {
		name   string
		mutate func(*LadderPlan)
	}{
		{name: "schema", mutate: func(value *LadderPlan) { value.SchemaVersion = 2 }},
		{name: "name", mutate: func(value *LadderPlan) { value.Name = "bad name" }},
		{name: "variable", mutate: func(value *LadderPlan) { value.Variable = "" }},
		{name: "reference", mutate: func(value *LadderPlan) { value.ReferenceCandidate = "missing" }},
		{name: "criterion", mutate: func(value *LadderPlan) { value.FunctionalityCriterion = "other" }},
		{name: "too few", mutate: func(value *LadderPlan) { value.Candidates = []string{"reference"} }},
		{name: "too many", mutate: func(value *LadderPlan) {
			value.Candidates = []string{"reference", "omitted", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth"}
		}},
		{name: "reference order", mutate: func(value *LadderPlan) { value.Candidates = []string{"omitted", "reference"} }},
		{name: "duplicate", mutate: func(value *LadderPlan) { value.Candidates[1] = "reference" }},
		{name: "unsafe ID", mutate: func(value *LadderPlan) { value.Candidates[1] = "../omitted" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := plan
			value.Candidates = append([]string(nil), plan.Candidates...)
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("LadderPlan.Validate() accepted invalid input")
			}
		})
	}
	for _, data := range [][]byte{nil, []byte(" "), []byte{0xff}, []byte(`{"schema_version":1,"name":"x","variable":"account-id","reference_candidate":"reference","functionality_criterion":"all-non-disclosure-fields-equal-v1","candidates":["reference","omitted"]} {}`), []byte(`{"schema_version":1,"name":"x","variable":"account-id","reference_candidate":"reference","functionality_criterion":"all-non-disclosure-fields-equal-v1","candidates":["reference","omitted"],"extra":true}`)} {
		if _, err := DecodeLadder(bytes.NewReader(data)); err == nil {
			t.Fatalf("DecodeLadder() accepted malformed data %q", data)
		}
	}
}

func TestReadLadderAndClassifyLadderCandidateBoundaries(t *testing.T) {
	plan := testLadderPlan()
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLadder(path)
	if err != nil || got.Name != plan.Name {
		t.Fatalf("ReadLadder() = %#v, err=%v", got, err)
	}
	invalid := ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)
	invalid.EvidenceState = evidence.State("invalid")
	if _, err := ClassifyLadderCandidate(invalid, 1); err == nil {
		t.Fatal("ClassifyLadderCandidate() accepted an invalid evidence state")
	}
	if _, err := ClassifyLadderCandidate(ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2), 0); err == nil {
		t.Fatal("ClassifyLadderCandidate() accepted zero pairs")
	}
	if _, err := SummarizeLadder(plan, testLadderProvenance(), 0, nil); err == nil {
		t.Fatal("SummarizeLadder() accepted zero pairs")
	}
	if _, err := SummarizeLadder(plan, testLadderProvenance(), 1, nil); err == nil {
		t.Fatal("SummarizeLadder() accepted a missing candidate result")
	}
}
func TestSummarizeLadderDerivesSelectionAndRejectsForgedLabels(t *testing.T) {
	plan := testLadderPlan()
	provenance := testLadderProvenance()
	for _, test := range []struct {
		name          string
		result        LadderCandidateResult
		wantClass     CandidateClassification
		wantSelection SelectionState
		wantSelected  string
		wantEvidence  evidence.State
	}{
		{name: "sufficient", result: ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 2, 0, 4, 0, 4), wantClass: CandidateSufficient, wantSelection: SelectionSelected, wantSelected: "omitted", wantEvidence: evidence.Observed},
		{name: "insufficient", result: ladderResult("omitted", portabletrace.ReplicatedChange, evidence.Observed, 2, 4, 0, 0, 4), wantClass: CandidateInsufficient, wantSelection: SelectionNoSufficient, wantEvidence: evidence.Observed},
		{name: "mixed", result: ladderResult("omitted", portabletrace.MixedInconsistent, evidence.Observed, 2, 2, 2, 0, 4), wantClass: CandidateMixedInconsistent, wantSelection: SelectionUnknown, wantEvidence: evidence.Observed},
		{name: "unknown", result: ladderResult("omitted", portabletrace.ReplicationUnknown, evidence.Unknown, 2, 0, 0, 4, 0), wantClass: CandidateUnknown, wantSelection: SelectionUnknown, wantEvidence: evidence.Unknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			summary, err := SummarizeLadder(plan, provenance, 2, []LadderCandidateResult{test.result})
			if err != nil {
				t.Fatal(err)
			}
			if summary.CandidateResults[0].Classification != test.wantClass || summary.SelectionState != test.wantSelection || summary.SelectedCandidate != test.wantSelected || summary.EvidenceState != test.wantEvidence {
				t.Fatalf("summary = %#v", summary)
			}
		})
	}
	for _, test := range []struct {
		name   string
		mutate func(*LadderCandidateResult)
	}{
		{name: "forged classification", mutate: func(result *LadderCandidateResult) { result.Classification = CandidateInsufficient }},
		{name: "forged outcome", mutate: func(result *LadderCandidateResult) { result.Outcome = portabletrace.ReplicatedChange }},
		{name: "counts", mutate: func(result *LadderCandidateResult) { result.ChangedPairs = -1 }},
		{name: "receipt", mutate: func(result *LadderCandidateResult) { result.ReceiptSHA256 = "short" }},
		{name: "directory", mutate: func(result *LadderCandidateResult) { result.Directory = "other" }},
		{name: "order", mutate: func(result *LadderCandidateResult) { result.ID = "reference" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 2, 0, 4, 0, 4)
			test.mutate(&result)
			if _, err := SummarizeLadder(plan, provenance, 2, []LadderCandidateResult{result}); err == nil {
				t.Fatal("SummarizeLadder() accepted invalid result")
			}
		})
	}
	if _, err := SummarizeLadder(plan, LadderProvenance{}, 2, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 2, 0, 4, 0, 4)}); err == nil {
		t.Fatal("SummarizeLadder() accepted invalid provenance")
	}
	if _, err := ClassifyLadderCandidate(ladderResult("omitted", portabletrace.ReplicatedChange, evidence.Observed, 2, 0, 4, 0, 4), 2); err == nil {
		t.Fatal("ClassifyLadderCandidate() accepted forged outcome")
	}
}

func TestLadderDecodeAndVerifyFailureBoundaries(t *testing.T) {
	plan := testLadderPlan()
	invalid := plan
	invalid.Name = ""
	invalidData, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeLadder(bytes.NewReader(invalidData)); err == nil {
		t.Fatal("DecodeLadder() accepted a plan that failed validation")
	}
	if _, err := ReadLadder(" "); err == nil {
		t.Fatal("ReadLadder() accepted an empty path")
	}
	summary, err := SummarizeLadder(plan, testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "ladder")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveLadder("", summary); err == nil {
		t.Fatal("SaveLadder() accepted an empty directory")
	}
	if err := SaveLadder(filepath.Join(t.TempDir(), "missing"), summary); err == nil {
		t.Fatal("SaveLadder() accepted a missing directory")
	}
	if err := SaveLadder(root, summary); err != nil {
		t.Fatal(err)
	}
	noop := func(string, LadderSummary, LadderCandidateResult) error { return nil }
	if _, _, err := VerifyLadder("", noop); err == nil {
		t.Fatal("VerifyLadder() accepted an empty directory")
	}
	if _, _, err := VerifyLadder(filepath.Join(t.TempDir(), "missing"), noop); err == nil {
		t.Fatal("VerifyLadder() accepted a missing directory")
	}
	receiptPath := filepath.Join(root, "minimization.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	unknown := strings.Replace(string(receipt), "{\n", "{\n  \"extra\": true,\n", 1)
	if err := os.WriteFile(receiptPath, []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyLadder(root, noop); err == nil {
		t.Fatal("VerifyLadder() accepted an unknown field")
	}
	canonical := strings.Replace(string(receipt), "\"schema_version\": 1", "\"schema_version\":    1", 1)
	if err := os.WriteFile(receiptPath, []byte(canonical), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyLadder(root, noop); err == nil {
		t.Fatal("VerifyLadder() accepted a non-canonical receipt")
	}
	if err := os.WriteFile(receiptPath, receipt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyLadder(root, func(string, LadderSummary, LadderCandidateResult) error { return errors.New("child failed") }); err == nil || !strings.Contains(err.Error(), "child failed") {
		t.Fatalf("VerifyLadder() child error = %v", err)
	}
}

type ladderErrorReader struct{}

func (ladderErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("reader failed")
}

func TestLadderAdditionalValidationPaths(t *testing.T) {
	plan := testLadderPlan()
	plan.ReferenceCandidate = ""
	if err := plan.Validate(); err == nil {
		t.Fatal("LadderPlan.Validate() accepted an empty reference candidate")
	}
	if _, err := SummarizeLadder(plan, testLadderProvenance(), 1, nil); err == nil {
		t.Fatal("SummarizeLadder() accepted an invalid plan")
	}
	valid := ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)
	if classification, err := ClassifyLadderCandidate(valid, 1); err != nil || classification != CandidateSufficient {
		t.Fatalf("ClassifyLadderCandidate() = %q, err=%v", classification, err)
	}
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{valid})
	if err != nil {
		t.Fatal(err)
	}
	duplicate := summary
	duplicate.CandidateResults = append([]LadderCandidateResult(nil), summary.CandidateResults...)
	copyResult := duplicate.CandidateResults[0]
	copyResult.Directory = LadderCandidateDirectory(1, copyResult.ID)
	duplicate.CandidateResults = append(duplicate.CandidateResults, copyResult)
	if err := duplicate.Validate(); err == nil {
		t.Fatal("LadderSummary.Validate() accepted duplicate candidate results")
	}
	missingRoot := t.TempDir()
	if _, _, err := VerifyLadder(missingRoot, func(string, LadderSummary, LadderCandidateResult) error { return nil }); err == nil {
		t.Fatal("VerifyLadder() accepted a directory without its receipt")
	}
}

func TestLadderSaveAndVerifyBindsChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ladder")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveLadder(root, summary); err != nil {
		t.Fatal(err)
	}
	verified, digest, err := VerifyLadder(root, func(childRoot string, got LadderSummary, result LadderCandidateResult) error {
		if childRoot != root || got.PlanName != summary.PlanName || result.ID != "omitted" {
			return errors.New("child mismatch")
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(verified, summary) || !validDigest(digest) {
		t.Fatalf("VerifyLadder() = %#v, %q, err=%v", verified, digest, err)
	}
	if err := SaveLadder(root, summary); err == nil {
		t.Fatal("SaveLadder() overwrote an existing receipt")
	}
	if _, _, err := VerifyLadder(root, nil); err == nil {
		t.Fatal("VerifyLadder() accepted a nil child verifier")
	}
	receiptPath := filepath.Join(root, "minimization.json")
	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(receipt), "fixture-value") || strings.Contains(string(receipt), "https://") {
		t.Fatal("ladder receipt disclosed source data")
	}
	if err := os.WriteFile(receiptPath, append(receipt, []byte("{}")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := VerifyLadder(root, func(string, LadderSummary, LadderCandidateResult) error { return nil }); err == nil {
		t.Fatal("VerifyLadder() accepted trailing receipt data")
	}
}

func TestLadderSummaryValidationRejectsUnsupportedShape(t *testing.T) {
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	summary.SelectionState = SelectionState("invalid")
	if err := summary.Validate(); err == nil {
		t.Fatal("LadderSummary.Validate() accepted an invalid selection state")
	}
	summary, err = SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	summary.CandidateResults = nil
	if err := summary.Validate(); err == nil {
		t.Fatal("LadderSummary.Validate() accepted an empty candidate result set")
	}
}

func TestLadderSummaryValidationRejectsInvalidFields(t *testing.T) {
	summary, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	summary.EvidenceState = evidence.State("invalid")
	if err := summary.Validate(); err == nil {
		t.Fatal("LadderSummary.Validate() accepted an invalid evidence state")
	}
	summary, err = SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	summary.CandidateResults[0].Directory = "other"
	if err := summary.Validate(); err == nil {
		t.Fatal("LadderSummary.Validate() accepted an invalid candidate directory")
	}
}

func TestLadderSummaryValidationRejectsTampering(t *testing.T) {
	base, err := SummarizeLadder(testLadderPlan(), testLadderProvenance(), 1, []LadderCandidateResult{ladderResult("omitted", portabletrace.NoChangeObserved, evidence.Observed, 1, 0, 2, 0, 2)})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*LadderSummary)
	}{
		{name: "schema", mutate: func(value *LadderSummary) { value.SchemaVersion = 2 }},
		{name: "adapter", mutate: func(value *LadderSummary) { value.Adapter = "bad adapter" }},
		{name: "digest", mutate: func(value *LadderSummary) { value.ProcedureSHA256 = "short" }},
		{name: "state", mutate: func(value *LadderSummary) { value.EvidenceState = evidence.Unknown }},
		{name: "selection", mutate: func(value *LadderSummary) {
			value.SelectedCandidate = "omitted"
			value.SelectionState = SelectionUnknown
		}},
		{name: "result ID", mutate: func(value *LadderSummary) { value.CandidateResults[0].ID = "../omitted" }},
		{name: "result evidence", mutate: func(value *LadderSummary) { value.CandidateResults[0].EvidenceState = evidence.Unknown }},
		{name: "result outcome", mutate: func(value *LadderSummary) { value.CandidateResults[0].Outcome = portabletrace.ReplicatedChange }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			value.CandidateResults = append([]LadderCandidateResult(nil), base.CandidateResults...)
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("LadderSummary.Validate() accepted tampering")
			}
		})
	}
}

func testLadderPlan() LadderPlan {
	return LadderPlan{
		SchemaVersion:          LadderSchemaVersion,
		Name:                   "browser-account-minimize",
		Variable:               "account-id",
		ReferenceCandidate:     "reference",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		Candidates:             []string{"reference", "omitted"},
	}
}

func testLadderProvenance() LadderProvenance {
	return LadderProvenance{
		Adapter:         "browser-local-fixture",
		AdapterVersion:  2,
		ProcedureSHA256: strings.Repeat("a", 64),
		Scope:           "outbound",
		ResetPolicy:     "fresh-ephemeral-profile-before-each-session",
	}
}

func ladderResult(id string, outcome portabletrace.ReplicatedOutcome, state evidence.State, pairs, changed, noChange, unknown, completed int) LadderCandidateResult {
	return LadderCandidateResult{
		ID:             id,
		Directory:      LadderCandidateDirectory(0, id),
		Outcome:        outcome,
		EvidenceState:  state,
		ReceiptSHA256:  strings.Repeat("b", 64),
		Pairs:          pairs * 2,
		PairsPerOrder:  pairs,
		CompletedPairs: completed,
		ChangedPairs:   changed,
		NoChangePairs:  noChange,
		UnknownPairs:   unknown,
	}
}
