package minimize

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
)

func TestDecodeAndValidatePlan(t *testing.T) {
	plan := testPlan()
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(strings.NewReader(string(data) + "\n"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.Name != plan.Name || len(decoded.Candidates) != len(plan.Candidates) {
		t.Fatalf("Decode() = %#v", decoded)
	}

	tests := []struct {
		name string
		data string
	}{
		{name: "duplicate", data: `{"schema_version":1,"schema_version":1}`},
		{name: "unknown", data: `{"schema_version":1,"unexpected":true}`},
		{name: "trailing", data: `{"schema_version":1} {}`},
		{name: "empty", data: "  "},
		{name: "invalid utf8", data: string([]byte{0xff})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(test.data)); err == nil {
				t.Fatal("Decode() error = nil")
			}
		})
	}

	invalid := plan
	invalid.Candidates = append([]Candidate(nil), plan.Candidates...)
	invalid.Candidates[1].ID = invalid.Candidates[0].ID
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() error = nil for duplicate candidate")
	}
	invalid = plan
	invalid.BasePersona = experiment.Persona{"email": "unsafe value"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() error = nil for unsafe persona")
	}
	invalid = plan
	invalid.Candidates = []Candidate{{ID: "exact", Value: "x"}, {ID: "city", Omitted: true, Value: "x"}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() error = nil for omitted value")
	}
}

func TestDecodeAndValidatePropagateBoundaryErrors(t *testing.T) {
	plan := testPlan()
	plan.SchemaVersion = 2
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(data))); err == nil {
		t.Fatal("Decode() accepted a plan rejected by Validate()")
	}
	plan = testPlan()
	plan.Variable = "bad variable"
	if err := plan.Validate(); err == nil {
		t.Fatal("Validate() accepted an unsafe variable")
	}
	plan = testPlan()
	plan.ReferenceCandidate = ""
	if err := plan.Validate(); err == nil {
		t.Fatal("Validate() accepted an empty reference candidate")
	}
}
func TestManifestForUsesReferenceAndHidesInputFromContract(t *testing.T) {
	plan := testPlan()
	manifest, err := plan.ManifestFor("city")
	if err != nil {
		t.Fatalf("ManifestFor() error = %v", err)
	}
	if manifest.Baseline["location"] != "37.7749-122.4194" ||
		manifest.Treatment["location"] != "san-francisco" {
		t.Fatalf("manifest locations = %#v / %#v", manifest.Baseline, manifest.Treatment)
	}
	if strings.Join(manifest.VolatileFields, ",") != "location,request_id" {
		t.Fatalf("volatile fields = %v", manifest.VolatileFields)
	}
	if _, err := plan.ManifestFor("exact"); err == nil {
		t.Fatal("ManifestFor() accepted the reference as treatment")
	}
	if strings.Contains(manifest.ContractDigest(), "san-francisco") {
		t.Fatal("contract digest exposed candidate value")
	}
}

func TestManifestForRejectsUnknownCandidate(t *testing.T) {
	if _, err := testPlan().ManifestFor("missing"); err == nil {
		t.Fatal("ManifestFor() accepted a candidate outside the plan")
	}
}
func TestManifestForRejectsInvalidPlan(t *testing.T) {
	plan := testPlan()
	plan.SchemaVersion = 0
	if _, err := plan.ManifestFor("city"); err == nil {
		t.Fatal("ManifestFor() accepted an invalid plan")
	}
}
func TestReportExistingPairsSkipsExistingEvidence(t *testing.T) {
	root := t.TempDir()
	for _, order := range []string{adb.ReplicationOrderBaselineTreatment, adb.ReplicationOrderTreatmentBaseline} {
		pairDir := filepath.Join(root, "pair-001-"+order)
		if err := os.Mkdir(pairDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pairDir, "evidence.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reports := 0
	if err := reportExistingPairs(root, 1, func(string) (bundle.Summary, error) {
		reports++
		return bundle.Summary{}, nil
	}); err != nil {
		t.Fatalf("reportExistingPairs() error = %v", err)
	}
	if reports != 0 {
		t.Fatalf("reportExistingPairs() reported %d existing pairs", reports)
	}
}

func TestReportExistingPairsRejectsSymlinks(t *testing.T) {
	t.Run("candidate", func(t *testing.T) {
		root := t.TempDir()
		link := filepath.Join(root, "candidate")
		target := filepath.Join(root, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if err := reportExistingPairs(link, 1, nil); err == nil {
			t.Fatal("reportExistingPairs() accepted a candidate symlink")
		}
	})
	t.Run("pair", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, "pair-001-"+adb.ReplicationOrderBaselineTreatment)); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if err := reportExistingPairs(root, 1, nil); err == nil {
			t.Fatal("reportExistingPairs() accepted a pair symlink")
		}
	})
	t.Run("evidence", func(t *testing.T) {
		root := t.TempDir()
		pairDir := filepath.Join(root, "pair-001-"+adb.ReplicationOrderBaselineTreatment)
		if err := os.Mkdir(pairDir, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "evidence-target")
		if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(pairDir, "evidence.json")); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		if err := reportExistingPairs(root, 1, nil); err == nil {
			t.Fatal("reportExistingPairs() accepted evidence symlink")
		}
	})
}
func TestSummarizeSeparatesSelectionOutcomeAndEvidence(t *testing.T) {
	tests := []struct {
		name      string
		city      CandidateClassification
		cityState evidence.State
		omitted   CandidateClassification
		omState   evidence.State
		selection SelectionState
		selected  string
		state     evidence.State
	}{
		{
			name:      "least disclosure selected",
			city:      CandidateSufficient,
			cityState: evidence.Observed,
			omitted:   CandidateSufficient,
			omState:   evidence.Observed,
			selection: SelectionSelected,
			selected:  "omitted",
			state:     evidence.Observed,
		},
		{
			name:      "insufficient then sufficient",
			city:      CandidateInsufficient,
			cityState: evidence.Observed,
			omitted:   CandidateSufficient,
			omState:   evidence.Observed,
			selection: SelectionSelected,
			selected:  "omitted",
			state:     evidence.Observed,
		},
		{
			name:      "no sufficient candidate",
			city:      CandidateInsufficient,
			cityState: evidence.Observed,
			omitted:   CandidateInsufficient,
			omState:   evidence.Observed,
			selection: SelectionNoSufficient,
			state:     evidence.Observed,
		},
		{
			name:      "mixed is not assurance",
			city:      CandidateMixedInconsistent,
			cityState: evidence.Observed,
			omitted:   CandidateSufficient,
			omState:   evidence.Observed,
			selection: SelectionUnknown,
			state:     evidence.Observed,
		},
		{
			name:      "missing evidence",
			city:      CandidateUnknown,
			cityState: evidence.Unknown,
			omitted:   CandidateSufficient,
			omState:   evidence.Observed,
			selection: SelectionUnknown,
			state:     evidence.Unknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results := []CandidateResult{
				testResult("city", 0, test.city, test.cityState),
				testResult("omitted", 1, test.omitted, test.omState),
			}
			summary, err := summarize(testPlan(), 1, results)
			if err != nil {
				t.Fatalf("summarize() error = %v", err)
			}
			if summary.SelectionState != test.selection ||
				summary.SelectedCandidate != test.selected ||
				summary.EvidenceState != test.state {
				t.Fatalf("summary = %#v", summary)
			}
		})
	}
}

func TestSummarizeRejectsResultCountAndOrder(t *testing.T) {
	plan := testPlan()
	if _, err := summarize(plan, 1, []CandidateResult{testResult("city", 0, CandidateSufficient, evidence.Observed)}); err == nil {
		t.Fatal("summarize() accepted an incomplete result list")
	}
	if _, err := summarize(plan, 1, []CandidateResult{
		testResult("omitted", 0, CandidateSufficient, evidence.Observed),
		testResult("city", 1, CandidateSufficient, evidence.Observed),
	}); err == nil {
		t.Fatal("summarize() accepted results in the wrong order")
	}
}
func TestExecuteRejectsParentFile(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, string, adb.Target, experiment.Manifest, string, int) error { return nil }
	reporter := func(string) (bundle.Summary, error) { return bundle.Summary{}, nil }
	verifier := func(string) (bundle.ReplicatedExperimentSummary, error) {
		return childSummary("android-location-minimize-city"), nil
	}
	if _, err := execute(context.Background(), "adb", adb.Target{}, testPlan(), filepath.Join(parent, "run"), 1, runner, reporter, verifier); err == nil {
		t.Fatal("execute() accepted a file as its output parent")
	}
}
func TestExecuteRunsOnlyTreatmentCandidatesAndWritesSafeReceipt(t *testing.T) {
	plan := testPlan()
	root := filepath.Join(t.TempDir(), "minimize")
	var manifests []experiment.Manifest
	reports := 0
	runner := func(_ context.Context, _ string, _ adb.Target, manifest experiment.Manifest, output string, pairs int) error {
		manifests = append(manifests, manifest)
		for pair := 1; pair <= pairs; pair++ {
			for _, order := range []string{adb.ReplicationOrderBaselineTreatment, adb.ReplicationOrderTreatmentBaseline} {
				if err := os.MkdirAll(filepath.Join(output, "pair-"+formatPair(pair)+"-"+order), 0o700); err != nil {
					return err
				}
			}
		}
		return nil
	}
	reporter := func(string) (bundle.Summary, error) {
		reports++
		return bundle.Summary{}, nil
	}
	verifier := func(directory string) (bundle.ReplicatedExperimentSummary, error) {
		id := ""
		if strings.Contains(filepath.Base(directory), "city") {
			id = "city"
		} else {
			id = "omitted"
		}
		return childSummary(plan.Name + "-" + id), nil
	}
	summary, err := execute(
		context.Background(),
		"adb",
		adb.Target{},
		plan,
		root,
		1,
		runner,
		reporter,
		verifier,
	)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if summary.SelectedCandidate != "omitted" || len(manifests) != 2 || reports != 4 {
		t.Fatalf("summary/manifests/reports = %#v / %d / %d", summary, len(manifests), reports)
	}
	if manifests[0].Treatment["location"] != "san-francisco" || manifests[1].Treatment["location"] != "omitted" {
		t.Fatalf("treatment values = %#v / %#v", manifests[0].Treatment, manifests[1].Treatment)
	}
	data, err := os.ReadFile(filepath.Join(root, "minimization.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "37.7749-122.4194") || strings.Contains(string(data), "san-francisco") {
		t.Fatalf("receipt exposed raw candidate value: %s", data)
	}
}

func TestExecuteRecordsRunnerFailureAsUnknownWhenReceiptVerifies(t *testing.T) {
	plan := testPlan()
	root := filepath.Join(t.TempDir(), "minimize")
	count := 0
	runner := func(context.Context, string, adb.Target, experiment.Manifest, string, int) error {
		count++
		if count == 1 {
			return errors.New("private value must not escape")
		}
		return nil
	}
	verifier := func(directory string) (bundle.ReplicatedExperimentSummary, error) {
		if strings.Contains(filepath.Base(directory), "001") {
			return childUnknown(plan.Name + "-city"), nil
		}
		return childSummary(plan.Name + "-omitted"), nil
	}
	reporter := func(string) (bundle.Summary, error) { return bundle.Summary{}, nil }
	summary, err := execute(context.Background(), "adb", adb.Target{}, plan, root, 1, runner, reporter, verifier)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if summary.SelectionState != SelectionUnknown || summary.EvidenceState != evidence.Unknown || summary.SelectedCandidate != "" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestSaveRejectsOverwriteAndVerifyRejectsMissingChild(t *testing.T) {
	root := t.TempDir()
	summary, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(root, summary); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := Save(root, summary); err == nil {
		t.Fatal("Save() overwrote an existing receipt")
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "candidate") {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestValidatePlanRejectsBoundaryInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*MinimizationPlan)
	}{
		{"schema", func(plan *MinimizationPlan) { plan.SchemaVersion = 2 }},
		{"name", func(plan *MinimizationPlan) { plan.Name = "bad name" }},
		{"variable", func(plan *MinimizationPlan) { plan.Variable = "" }},
		{"reference missing", func(plan *MinimizationPlan) { plan.ReferenceCandidate = "missing" }},
		{"reference not first", func(plan *MinimizationPlan) {
			plan.Candidates[0], plan.Candidates[1] = plan.Candidates[1], plan.Candidates[0]
		}},
		{"criterion", func(plan *MinimizationPlan) { plan.FunctionalityCriterion = "other" }},
		{"tap resource", func(plan *MinimizationPlan) { plan.TapResourceID = "unsafe id" }},
		{"empty base", func(plan *MinimizationPlan) { plan.BasePersona = nil }},
		{"unsafe base", func(plan *MinimizationPlan) { plan.BasePersona["email"] = "unsafe value" }},
		{"variable in base", func(plan *MinimizationPlan) { plan.BasePersona["location"] = "exact" }},
		{"too many base fields", func(plan *MinimizationPlan) {
			for index := 0; index < 64; index++ {
				plan.BasePersona[fmt.Sprintf("field-%d", index)] = "value"
			}
		}},
		{"too few candidates", func(plan *MinimizationPlan) { plan.Candidates = plan.Candidates[:1] }},
		{"too many candidates", func(plan *MinimizationPlan) {
			for index := 0; index < 6; index++ {
				plan.Candidates = append(plan.Candidates, Candidate{ID: fmt.Sprintf("extra-%d", index), Value: "value"})
			}
		}},
		{"bad candidate id", func(plan *MinimizationPlan) { plan.Candidates[1].ID = "../city" }},
		{"empty candidate value", func(plan *MinimizationPlan) { plan.Candidates[1].Value = "" }},
		{"unsafe candidate value", func(plan *MinimizationPlan) { plan.Candidates[1].Value = "unsafe value" }},
		{"omitted value", func(plan *MinimizationPlan) { plan.Candidates[1].Omitted = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := testPlan()
			test.mutate(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestExecuteRejectsConfigurationAndDependencyFailures(t *testing.T) {
	plan := testPlan()
	runner := func(context.Context, string, adb.Target, experiment.Manifest, string, int) error { return nil }
	reporter := func(string) (bundle.Summary, error) { return bundle.Summary{}, nil }
	verifier := func(string) (bundle.ReplicatedExperimentSummary, error) {
		return childSummary("android-location-minimize-city"), nil
	}
	for _, test := range []struct {
		name   string
		output string
		pairs  int
		run    ReplicatedRunner
		report SummaryReporter
		verify SummaryVerifier
	}{
		{"empty output", "", 1, runner, reporter, verifier},
		{"bad pairs", filepath.Join(t.TempDir(), "run"), 0, runner, reporter, verifier},
		{"missing runner", filepath.Join(t.TempDir(), "run"), 1, nil, reporter, verifier},
		{"missing reporter", filepath.Join(t.TempDir(), "run"), 1, runner, nil, verifier},
		{"missing verifier", filepath.Join(t.TempDir(), "run"), 1, runner, reporter, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := execute(context.Background(), "adb", adb.Target{}, plan, test.output, test.pairs, test.run, test.report, test.verify); err == nil {
				t.Fatal("execute() error = nil")
			}
		})
	}
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(context.Background(), "adb", adb.Target{}, plan, existing, 1, runner, reporter, verifier); err == nil {
		t.Fatal("execute() accepted existing output")
	}
}

func TestDecodeSummaryRejectsMalformedAndNonCanonicalReceipts(t *testing.T) {
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.MarshalIndent(valid, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte{},
		[]byte(`{"schema_version":1,"schema_version":1}`),
		[]byte(`{"schema_version":1,"unexpected":true}`),
		append(append([]byte(nil), canonical...), []byte(" {}")...),
	} {
		if _, err := decodeSummary(data); err == nil {
			t.Fatalf("decodeSummary(%q) error = nil", data)
		}
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "minimization.json"), append(canonical, '\n', '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("Verify() noncanonical error = %v", err)
	}
}

func TestExecuteReportsChildAndVerifierFailures(t *testing.T) {
	plan := testPlan()
	makePairs := func(output string, asFile bool) error {
		for _, order := range []string{adb.ReplicationOrderBaselineTreatment, adb.ReplicationOrderTreatmentBaseline} {
			path := filepath.Join(output, "pair-001-"+order)
			if asFile {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					return err
				}
			} else if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
		}
		return nil
	}
	runner := func(_ context.Context, _ string, _ adb.Target, _ experiment.Manifest, output string, _ int) error {
		return makePairs(output, false)
	}
	if _, err := execute(context.Background(), "adb", adb.Target{}, plan, filepath.Join(t.TempDir(), "report-error"), 1, runner,
		func(string) (bundle.Summary, error) { return bundle.Summary{}, errors.New("report failed") },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return childSummary("android-location-minimize-city"), nil
		}); err == nil {
		t.Fatal("execute() error = nil for reporter failure")
	}
	if _, err := execute(context.Background(), "adb", adb.Target{}, plan, filepath.Join(t.TempDir(), "verify-error"), 1, runner,
		func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return bundle.ReplicatedExperimentSummary{}, errors.New("verify failed")
		}); err == nil {
		t.Fatal("execute() error = nil for verifier failure")
	}
	fileRunner := func(_ context.Context, _ string, _ adb.Target, _ experiment.Manifest, output string, _ int) error {
		return makePairs(output, true)
	}
	if _, err := execute(context.Background(), "adb", adb.Target{}, plan, filepath.Join(t.TempDir(), "file-pair"), 1, fileRunner,
		func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return childSummary("android-location-minimize-city"), nil
		}); err == nil {
		t.Fatal("execute() error = nil for non-directory pair")
	}
}

func TestExecuteRejectsFailedRunnerAfterNonUnknownResult(t *testing.T) {
	plan := testPlan()
	runner := func(_ context.Context, _ string, _ adb.Target, _ experiment.Manifest, output string, _ int) error {
		if err := os.MkdirAll(output, 0o700); err != nil {
			return err
		}
		return errors.New("runner failed")
	}
	_, err := execute(
		context.Background(),
		"adb",
		adb.Target{},
		plan,
		filepath.Join(t.TempDir(), "run"),
		1,
		runner,
		func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return childSummary("android-location-minimize-city"), nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "non-unknown") {
		t.Fatalf("execute() error = %v", err)
	}
}
func TestExecuteRejectsVerifierManifestMismatch(t *testing.T) {
	plan := testPlan()
	_, err := execute(
		context.Background(),
		"adb",
		adb.Target{},
		plan,
		filepath.Join(t.TempDir(), "run"),
		1,
		func(context.Context, string, adb.Target, experiment.Manifest, string, int) error { return nil },
		func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return childSummary("android-location-minimize-secret"), nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "manifest metadata disagrees") {
		t.Fatalf("execute() error = %v", err)
	}
}
func TestExecuteRejectsFailedRunnerWithoutVerifiableReceipt(t *testing.T) {
	plan := testPlan()
	_, err := execute(
		context.Background(),
		"adb",
		adb.Target{},
		plan,
		filepath.Join(t.TempDir(), "run"),
		1,
		func(context.Context, string, adb.Target, experiment.Manifest, string, int) error {
			return errors.New("runner failed")
		},
		func(string) (bundle.Summary, error) { return bundle.Summary{}, nil },
		func(string) (bundle.ReplicatedExperimentSummary, error) {
			return bundle.ReplicatedExperimentSummary{}, errors.New("receipt unavailable")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "verifiable replication") {
		t.Fatalf("execute() error = %v", err)
	}
}
func TestExecuteRejectsInvalidSummaryResult(t *testing.T) {
	results := []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	}
	results[0].Classification = CandidateInsufficient
	if _, err := summarize(testPlan(), 1, results); err == nil {
		t.Fatal("summarize() accepted a result with inconsistent classification")
	}
}
func TestExecuteWrapperRejectsInvalidPlan(t *testing.T) {
	plan := testPlan()
	plan.SchemaVersion = 0
	if _, err := Execute(context.Background(), "adb", adb.Target{}, plan, filepath.Join(t.TempDir(), "run"), 1, nil); err == nil {
		t.Fatal("Execute() error = nil")
	}
}
func testPlan() MinimizationPlan {
	return MinimizationPlan{
		SchemaVersion:          CurrentSchemaVersion,
		Name:                   "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		TapResourceID:          "dev.ariadne.fixture:id/observe_button",
		BasePersona: experiment.Persona{
			"email":  "baseline@example.invalid",
			"region": "us-east",
		},
		Candidates: []Candidate{
			{ID: "exact", Value: "37.7749-122.4194"},
			{ID: "city", Value: "san-francisco"},
			{ID: "omitted", Omitted: true},
		},
	}
}

func testResult(id string, index int, classification CandidateClassification, state evidence.State) CandidateResult {
	outcome := bundle.NoChangeObserved
	changed, same, unknown := 0, 2, 0
	switch classification {
	case CandidateInsufficient:
		outcome, changed, same = bundle.ReplicatedChange, 2, 0
	case CandidateMixedInconsistent:
		outcome, changed, same = bundle.MixedInconsistent, 1, 1
	case CandidateUnknown:
		outcome, changed, same, unknown = bundle.ReplicationUnknown, 0, 0, 2
	}
	return CandidateResult{
		ID:             id,
		ManifestName:   "android-location-minimize-" + id,
		Directory:      candidateDirectory(index, id),
		Classification: classification,
		Outcome:        outcome,
		EvidenceState:  state,
		ReceiptSHA256:  strings.Repeat("a", 64),
		Pairs:          2,
		PairsPerOrder:  1,
		CompletedPairs: 2,
		ChangedPairs:   changed,
		NoChangePairs:  same,
		UnknownPairs:   unknown,
	}
}

func childSummary(name string) bundle.ReplicatedExperimentSummary {
	return bundle.ReplicatedExperimentSummary{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     name,
		DeclaredVariable: "location",
		ReceiptSHA256:    strings.Repeat("a", 64),
		Pairs:            2,
		PairsPerOrder:    1,
		CompletedPairs:   2,
		NoChangePairs:    2,
		Outcome:          bundle.NoChangeObserved,
		EvidenceState:    evidence.Observed,
	}
}

func childUnknown(name string) bundle.ReplicatedExperimentSummary {
	return bundle.ReplicatedExperimentSummary{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     name,
		DeclaredVariable: "location",
		ReceiptSHA256:    strings.Repeat("b", 64),
		Pairs:            2,
		PairsPerOrder:    1,
		CompletedPairs:   0,
		UnknownPairs:     2,
		Outcome:          bundle.ReplicationUnknown,
		EvidenceState:    evidence.Unknown,
	}
}

func formatPair(pair int) string {
	return fmt.Sprintf("%03d", pair)
}

func TestVerifyChecksValidChildReplication(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "candidate-001-city")
	makeValidChildReplication(t, childDir)
	child, err := bundle.VerifyReplicated(childDir)
	if err != nil {
		t.Fatalf("bundle.VerifyReplicated() error = %v", err)
	}
	result := candidateResult("city", "candidate-001-city", child)
	summary := MinimizationSummary{
		SchemaVersion:          SummarySchemaVersion,
		PlanName:               "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         SelectionSelected,
		SelectedCandidate:      "city",
		CandidateResults:       []CandidateResult{result},
	}
	if err := Save(root, summary); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	verified, err := Verify(root)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.SelectedCandidate != "city" || verified.CandidateResults[0].ReceiptSHA256 != child.ReceiptSHA256 {
		t.Fatalf("verified summary = %#v", verified)
	}
}

func makeValidChildReplication(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	orders := []struct {
		name          string
		firstSession  string
		secondSession string
	}{
		{adb.ReplicationOrderBaselineTreatment, "baseline", "treatment"},
		{adb.ReplicationOrderTreatmentBaseline, "treatment", "baseline"},
	}
	pairs := make([]adb.ReplicatedPairRecord, 0, 2)
	for _, order := range orders {
		pairDir := filepath.Join(root, "pair-001-"+order.name)
		if err := os.MkdirAll(pairDir, 0o700); err != nil {
			t.Fatal(err)
		}
		base := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
		if order.name == adb.ReplicationOrderTreatmentBaseline {
			writeValidSession(t, pairDir, "treatment", "city", base)
			writeValidSession(t, pairDir, "baseline", "exact", base.Add(20*time.Second))
		} else {
			writeValidSession(t, pairDir, "baseline", "exact", base)
			writeValidSession(t, pairDir, "treatment", "city", base.Add(20*time.Second))
		}
		if _, err := bundle.Write(pairDir); err != nil {
			t.Fatalf("bundle.Write(%s) error = %v", order.name, err)
		}
		pairs = append(pairs, adb.ReplicatedPairRecord{
			Pair:          1,
			Order:         order.name,
			Directory:     "pair-001-" + order.name,
			FirstSession:  order.firstSession,
			SecondSession: order.secondSession,
			Status:        adb.ReplicationStatusComplete,
		})
	}
	record := adb.ReplicatedRunRecord{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     "android-location-minimize-city",
		DeclaredVariable: "location",
		PairsPerOrder:    1,
		ResetPolicy:      adb.ReplicationResetPolicy,
		Status:           adb.ReplicationStatusComplete,
		CompletedPairs:   2,
		Pairs:            pairs,
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "replication.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeValidSession(t *testing.T, pairDir, kind, location string, started time.Time) {
	t.Helper()
	sessionDir := filepath.Join(pairDir, kind)
	observations := filepath.Join(sessionDir, "observations")
	if err := os.MkdirAll(observations, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"schema_version":1,"location":"%s","request_id":"%s","variant":"stable"}`, location, kind+"-request"))
	network, err := json.MarshalIndent(collector.Observation{
		SchemaVersion: 1,
		Method:        "POST",
		Path:          "/observe",
		ContentType:   "application/json",
		BodyBase64:    base64.StdEncoding.EncodeToString(body),
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	network = append(network, '\n')
	if err := os.WriteFile(filepath.Join(observations, "network.json"), network, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(observations, "storage.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	stepNames := []string{"reset", "connect_network", "start", "interact", "capture_network", "capture_storage", "disconnect_network"}
	steps := make([]adb.StepRecord, 0, len(stepNames))
	for index, name := range stepNames {
		stepStart := started.Add(time.Duration(index+1) * time.Second)
		step := adb.StepRecord{Name: name, StartedAt: stepStart, FinishedAt: stepStart.Add(time.Second), Status: "ok", ExitCode: 0}
		if name == "interact" {
			step.UIHierarchySHA256 = strings.Repeat("d", 64)
		}
		steps = append(steps, step)
	}
	record := adb.SessionRecord{
		SchemaVersion:          7,
		Kind:                   kind,
		ManifestName:           "android-location-minimize-city",
		DeclaredVariable:       "location",
		PersonaFields:          3,
		VolatileFields:         []string{"location", "request_id"},
		TapResourceID:          "dev.ariadne.fixture:id/observe_button",
		ManifestContractSHA256: strings.Repeat("c", 64),
		ADBVersion:             "1.0.41",
		Device:                 "emulator-5554",
		Package:                "dev.ariadne.fixture",
		AndroidAPI:             35,
		Architecture:           "x86_64",
		PackageVersionCode:     1,
		PackageSHA256:          strings.Repeat("a", 64),
		AriadneRevision:        strings.Repeat("b", 40),
		Status:                 "complete",
		StartedAt:              started,
		FinishedAt:             started.Add(10 * time.Second),
		Steps:                  steps,
		Artifacts: []adb.Artifact{
			minimizeArtifact("http_request", "POST /observe", "observations/network.json", network),
			minimizeArtifact("android_private_storage", "files/observation.json", "observations/storage.json", body),
		},
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "session.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func minimizeArtifact(kind, source, path string, data []byte) adb.Artifact {
	digest := sha256.Sum256(data)
	return adb.Artifact{Kind: kind, Source: source, Path: path, SizeBytes: len(data), SHA256: hex.EncodeToString(digest[:])}
}

func TestVerifyRejectsTamperedChildResult(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "candidate-001-city")
	makeValidChildReplication(t, childDir)
	child, err := bundle.VerifyReplicated(childDir)
	if err != nil {
		t.Fatalf("bundle.VerifyReplicated() error = %v", err)
	}
	summary := MinimizationSummary{
		SchemaVersion:          SummarySchemaVersion,
		PlanName:               "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         SelectionSelected,
		SelectedCandidate:      "city",
		CandidateResults:       []CandidateResult{candidateResult("city", "candidate-001-city", child)},
	}
	if err := Save(root, summary); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	summary.CandidateResults[0].ReceiptSHA256 = strings.Repeat("e", 64)
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "minimization.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "does not match replication") {
		t.Fatalf("Verify() error = %v", err)
	}
	summary.Variable = "other"
	data, err = json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "minimization.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "metadata disagrees") {
		t.Fatalf("Verify() metadata error = %v", err)
	}
}

func TestVerifyRejectsInvalidChildReceipt(t *testing.T) {
	root := t.TempDir()
	childDir := filepath.Join(root, "candidate-001-city")
	makeValidChildReplication(t, childDir)
	child, err := bundle.VerifyReplicated(childDir)
	if err != nil {
		t.Fatalf("bundle.VerifyReplicated() error = %v", err)
	}
	summary := MinimizationSummary{
		SchemaVersion:          SummarySchemaVersion,
		PlanName:               "android-location-minimize",
		Variable:               "location",
		ReferenceCandidate:     "exact",
		FunctionalityCriterion: FunctionalityCriterionAllNonDisclosureFields,
		PairsPerOrder:          1,
		EvidenceState:          evidence.Observed,
		SelectionState:         SelectionSelected,
		SelectedCandidate:      "city",
		CandidateResults:       []CandidateResult{candidateResult("city", "candidate-001-city", child)},
	}
	if err := Save(root, summary); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := os.Remove(filepath.Join(childDir, "replication.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(root); err == nil || !strings.Contains(err.Error(), "verify replication") {
		t.Fatalf("Verify() error = %v", err)
	}
}
func TestReceiptBoundariesRejectInvalidDirectoriesAndOversize(t *testing.T) {
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(file, valid); err == nil {
		t.Fatal("Save() accepted a file as its root")
	}
	if _, err := Verify(file); err == nil {
		t.Fatal("Verify() accepted a file as its root")
	}
	if _, err := Verify(t.TempDir()); err == nil {
		t.Fatal("Verify() accepted a directory without a receipt")
	}
	oversized := t.TempDir()
	if err := os.WriteFile(filepath.Join(oversized, "minimization.json"), []byte(strings.Repeat("x", maxSummaryBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(oversized); err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("Verify() oversized error = %v", err)
	}
}
func TestClassifyUnknownOutcome(t *testing.T) {
	if got := classify("unrecognized", evidence.Observed); got != CandidateUnknown {
		t.Fatalf("classify() = %q, want %q", got, CandidateUnknown)
	}
}

func TestVerifyRejectsRootSymlink(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "minimization-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err := Verify(link); err == nil {
		t.Fatal("Verify() accepted a root symlink")
	}
}
func TestSaveAndDecodeRejectInvalidSummary(t *testing.T) {
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.SelectionState = SelectionNoSufficient
	if err := Save(t.TempDir(), invalid); err == nil {
		t.Fatal("Save() accepted an inconsistent summary")
	}
	invalid = valid
	invalid.PairsPerOrder = 0
	data, err := json.Marshal(invalid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSummary(data); err == nil {
		t.Fatal("decodeSummary() accepted an invalid summary")
	}
}
func TestValidateSummaryRejectsBoundaryInputs(t *testing.T) {
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*MinimizationSummary)
	}{
		{"schema", func(summary *MinimizationSummary) { summary.SchemaVersion = 2 }},
		{"plan name", func(summary *MinimizationSummary) { summary.PlanName = "bad name" }},
		{"variable", func(summary *MinimizationSummary) { summary.Variable = "" }},
		{"reference", func(summary *MinimizationSummary) { summary.ReferenceCandidate = "../exact" }},
		{"criterion", func(summary *MinimizationSummary) { summary.FunctionalityCriterion = "other" }},
		{"pairs", func(summary *MinimizationSummary) { summary.PairsPerOrder = 0 }},
		{"evidence state", func(summary *MinimizationSummary) { summary.EvidenceState = "other" }},
		{"selection state", func(summary *MinimizationSummary) { summary.SelectionState = "other" }},
		{"empty results", func(summary *MinimizationSummary) { summary.CandidateResults = nil }},
		{"too many results", func(summary *MinimizationSummary) {
			for index := 0; index < 6; index++ {
				summary.CandidateResults = append(summary.CandidateResults, testResult(fmt.Sprintf("extra-%d", index), index+2, CandidateSufficient, evidence.Observed))
			}
		}},
		{"bad result id", func(summary *MinimizationSummary) { summary.CandidateResults[0].ID = "../city" }},
		{"duplicate result id", func(summary *MinimizationSummary) { summary.CandidateResults[1].ID = "city" }},
		{"reference result id", func(summary *MinimizationSummary) { summary.CandidateResults[0].ID = "exact" }},
		{"directory", func(summary *MinimizationSummary) { summary.CandidateResults[0].Directory = "other" }},
		{"manifest name", func(summary *MinimizationSummary) { summary.CandidateResults[0].ManifestName = "bad name" }},
		{"receipt length", func(summary *MinimizationSummary) { summary.CandidateResults[0].ReceiptSHA256 = "short" }},
		{"receipt hex", func(summary *MinimizationSummary) {
			summary.CandidateResults[0].ReceiptSHA256 = strings.Repeat("g", 64)
		}},
		{"pairs total", func(summary *MinimizationSummary) { summary.CandidateResults[0].Pairs = 1 }},
		{"pairs per order", func(summary *MinimizationSummary) { summary.CandidateResults[0].PairsPerOrder = 2 }},
		{"completed count", func(summary *MinimizationSummary) { summary.CandidateResults[0].CompletedPairs = 3 }},
		{"changed count", func(summary *MinimizationSummary) { summary.CandidateResults[0].ChangedPairs = -1 }},
		{"same count", func(summary *MinimizationSummary) { summary.CandidateResults[0].NoChangePairs = -1 }},
		{"unknown count", func(summary *MinimizationSummary) { summary.CandidateResults[0].UnknownPairs = -1 }},
		{"count sum", func(summary *MinimizationSummary) { summary.CandidateResults[0].NoChangePairs = 1 }},
		{"result evidence", func(summary *MinimizationSummary) { summary.CandidateResults[0].EvidenceState = "other" }},
		{"result outcome", func(summary *MinimizationSummary) { summary.CandidateResults[0].Outcome = "other" }},
		{"result classification", func(summary *MinimizationSummary) { summary.CandidateResults[0].Classification = "other" }},
		{"classification mismatch", func(summary *MinimizationSummary) { summary.CandidateResults[0].Classification = CandidateInsufficient }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := valid
			summary.CandidateResults = append([]CandidateResult(nil), valid.CandidateResults...)
			test.mutate(&summary)
			if err := validateSummary(summary); err == nil {
				t.Fatal("validateSummary() error = nil")
			}
		})
	}
}

func TestSummaryBindsManifestNameToSafeCandidateID(t *testing.T) {
	summary, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	summary.CandidateResults[0].ManifestName = "android-location-minimize-secret"
	if err := validateSummary(summary); err == nil {
		t.Fatal("validateSummary() accepted a manifest name not bound to the candidate ID")
	}
}
func TestDecodeRejectsOversizedPlanAndSaveRejectsInvalidPath(t *testing.T) {
	if _, err := Decode(strings.NewReader(strings.Repeat("x", maxPlanBytes+1))); err == nil {
		t.Fatal("Decode() accepted oversized plan")
	}
	valid, err := summarize(testPlan(), 1, []CandidateResult{
		testResult("city", 0, CandidateSufficient, evidence.Observed),
		testResult("omitted", 1, CandidateSufficient, evidence.Observed),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(t.TempDir(), "missing", "root"), valid); err == nil {
		t.Fatal("Save() accepted a missing directory")
	}
	if _, err := Verify(""); err == nil {
		t.Fatal(`Verify("") error = nil`)
	}
}
