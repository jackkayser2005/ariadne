package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseMapCompare(t *testing.T) {
	digest := strings.Repeat("a", 64)
	want := trace.CaseDisclosureMapComparison{
		SchemaVersion:                 1,
		ComparisonID:                  "case-disclosure-map-change",
		ComparisonQuestion:            "Did the reviewed disclosure map change between these cases?",
		OrderBasis:                    "caller",
		Result:                        "changed",
		InvestigationCommitmentSHA256: digest,
		FirstCaseSHA256:               strings.Repeat("b", 64),
		SecondCaseSHA256:              strings.Repeat("c", 64),
		FirstRoundSHA256:              strings.Repeat("d", 64),
		SecondRoundSHA256:             strings.Repeat("e", 64),
		FirstCoverageState:            evidence.Observed,
		SecondCoverageState:           evidence.Observed,
		EvidenceState:                 evidence.Observed,
		ComparedCategories:            2,
		AddedCategories:               []string{"consent"},
		RemovedCategories:             []string{"location"},
		ComparedBoundaries:            2,
		AddedBoundaries:               []trace.CaseDisclosureMapBoundaryChange{{Category: "consent", Source: "android", Adapter: "android-experiment-001", Channel: "network", Kind: "request", Destination: "analytics"}},
		RemovedBoundaries:             []trace.CaseDisclosureMapBoundaryChange{{Category: "location", Source: "android", Adapter: "android-experiment-001", Channel: "network", Kind: "request", Destination: "analytics"}},
		UnknownCategories:             []string{},
		UnknownBoundaries:             []trace.CaseDisclosureMapBoundaryChange{},
	}
	compare := func(first, second, commitment string) (trace.CaseDisclosureMapComparison, error) {
		if first != "first" || second != "second" || commitment != digest {
			t.Fatalf("comparison inputs = %q, %q, %q", first, second, commitment)
		}
		return want, nil
	}
	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMapCompare([]string{"--commitment-sha256", digest, "first", "second"}, &stdout, &stderr, compare); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "trace case disclosure maps compared") || !strings.Contains(stdout.String(), "result: changed") || !strings.Contains(stdout.String(), "added_category: consent") || !strings.Contains(stdout.String(), "added_boundary: consent/android") {
		t.Fatalf("human comparison = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseMapCompare([]string{"--json", "--commitment-sha256", digest, "first", "second"}, &stdout, &stderr, compare); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"result":"changed"`) || !strings.Contains(stdout.String(), `"investigation_commitment_sha256":"`+digest+`"`) {
		t.Fatalf("JSON comparison = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceCaseMapCompareFailures(t *testing.T) {
	digest := strings.Repeat("a", 64)
	compare := func(string, string, string) (trace.CaseDisclosureMapComparison, error) {
		return trace.CaseDisclosureMapComparison{}, errors.New("comparison failed safely")
	}
	for _, args := range [][]string{
		nil,
		{"--commitment-sha256", digest, "first"},
		{"--commitment-sha256", "bad", "first", "second"},
	} {
		var stdout, stderr strings.Builder
		if exitCode := runTraceCaseMapCompare(args, &stdout, &stderr, func(string, string, string) (trace.CaseDisclosureMapComparison, error) {
			t.Fatal("comparer called for invalid usage")
			return trace.CaseDisclosureMapComparison{}, nil
		}); exitCode != 2 || stdout.Len() != 0 {
			t.Fatalf("usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMapCompare([]string{"--commitment-sha256", digest, "first", "second"}, &stdout, &stderr, compare); exitCode != 1 || !strings.Contains(stderr.String(), "comparison failed safely") {
		t.Fatalf("comparer failure = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceCaseMapCompare([]string{"--commitment-sha256", digest, "first", "second"}, failingWriter{}, &stderr, func(string, string, string) (trace.CaseDisclosureMapComparison, error) {
		return trace.CaseDisclosureMapComparison{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human writer failure = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceCaseMapCompare([]string{"--json", "--commitment-sha256", digest, "first", "second"}, failingWriter{}, &stderr, func(string, string, string) (trace.CaseDisclosureMapComparison, error) {
		return trace.CaseDisclosureMapComparison{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON writer failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if err := writeTraceCaseMapComparison(failingWriter{}, trace.CaseDisclosureMapComparison{}); err == nil {
		t.Fatal("writeTraceCaseMapComparison() accepted a failing writer")
	}
}

func TestTraceCaseMapCompareDispatch(t *testing.T) {
	digest := strings.Repeat("a", 64)
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "map", "compare", "--commitment-sha256", digest, "missing", "missing"}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "trace case map compare") {
		t.Fatalf("comparison dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
