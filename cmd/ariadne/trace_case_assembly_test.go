package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseAssemble(t *testing.T) {
	want := trace.CaseAssemblySummary{
		SchemaVersion:         1,
		Entries:               2,
		CaseSHA256:            strings.Repeat("a", 64),
		DisclosureRoundSHA256: strings.Repeat("b", 64),
		CoverageState:         evidence.Unknown,
		Questions: []trace.CaseAssemblyQuestionSummary{
			{QuestionID: trace.CaseDisclosureQuestionCoverage, Result: "unknown", EvidenceState: evidence.Unknown},
			{QuestionID: trace.CaseDisclosureQuestionOverlap, Result: "overlap-observed", EvidenceState: evidence.Observed},
		},
	}
	assemble := func(planPath, outputDir string) (trace.CaseAssemblySummary, error) {
		if planPath != "plan.json" || outputDir != "workspace" {
			t.Fatalf("assembly inputs = %q, %q", planPath, outputDir)
		}
		return want, nil
	}
	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseAssemble([]string{"--json", "--plan", "plan.json", "--output", "workspace"}, &stdout, &stderr, assemble); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"disclosure_round_sha256"`) || !strings.Contains(stdout.String(), `"overlap-observed"`) {
		t.Fatalf("JSON assembly = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseAssemble([]string{"--plan", "plan.json", "--output", "workspace"}, &stdout, &stderr, assemble); exitCode != 0 || !strings.Contains(stdout.String(), "trace case assembly complete") || !strings.Contains(stdout.String(), "coverage_state: unknown") || !strings.Contains(stdout.String(), "evidence_state: observed") || stderr.Len() != 0 {
		t.Fatalf("human assembly = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceCaseAssembleVerify(t *testing.T) {
	want := trace.CaseAssemblySummary{
		SchemaVersion:         1,
		Entries:               1,
		CaseSHA256:            strings.Repeat("a", 64),
		DisclosureRoundSHA256: strings.Repeat("b", 64),
		CoverageState:         evidence.Observed,
	}
	verify := func(path string) (trace.CaseAssemblySummary, error) {
		if path != "workspace" {
			t.Fatalf("verification path = %q", path)
		}
		return want, nil
	}
	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseAssembleVerify([]string{"--json", "workspace"}, &stdout, &stderr, verify); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "case_sha256") {
		t.Fatalf("JSON verification = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceCaseAssembleVerify([]string{"workspace"}, &stdout, &stderr, verify); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "trace case assembly verified") {
		t.Fatalf("human verification = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runTraceCaseAssembleVerify(nil, &stdout, &stderr, func(string) (trace.CaseAssemblySummary, error) {
		t.Fatal("verifier called for invalid usage")
		return trace.CaseAssemblySummary{}, nil
	}); exitCode != 2 {
		t.Fatalf("verification usage = %d", exitCode)
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceCaseAssembleVerify([]string{"workspace"}, &stdout, &stderr, func(string) (trace.CaseAssemblySummary, error) {
		return trace.CaseAssemblySummary{}, errors.New("verification failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "verification failed safely") {
		t.Fatalf("verification failure = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceCaseAssembleVerify([]string{"workspace"}, failingWriter{}, &strings.Builder{}, verify); exitCode != 1 {
		t.Fatalf("human verification writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAssembleVerify([]string{"--json", "workspace"}, failingWriter{}, &strings.Builder{}, verify); exitCode != 1 {
		t.Fatalf("JSON verification writer error = %d", exitCode)
	}
	if err := writeTraceCaseAssemblyVerificationSummary(failingWriter{}, want); err == nil {
		t.Fatal("writeTraceCaseAssemblyVerificationSummary() accepted a failing writer")
	}
}
func TestRunTraceCaseAssembleFailures(t *testing.T) {
	cases := map[string]struct {
		args     []string
		assemble traceCaseAssembler
		want     int
	}{
		"usage": {
			args: []string{"--plan", "plan.json"},
			assemble: func(string, string) (trace.CaseAssemblySummary, error) {
				t.Fatal("assembler called for invalid usage")
				return trace.CaseAssemblySummary{}, nil
			},
			want: 2,
		},
		"extra argument": {
			args: []string{"--plan", "plan.json", "--output", "workspace", "extra"},
			assemble: func(string, string) (trace.CaseAssemblySummary, error) {
				t.Fatal("assembler called for extra argument")
				return trace.CaseAssemblySummary{}, nil
			},
			want: 2,
		},
		"assembler error": {
			args: []string{"--plan", "plan.json", "--output", "workspace"},
			assemble: func(string, string) (trace.CaseAssemblySummary, error) {
				return trace.CaseAssemblySummary{}, errors.New("assembly failed safely")
			},
			want: 1,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if got := runTraceCaseAssemble(test.args, &stdout, &stderr, test.assemble); got != test.want || (test.want == 1 && !strings.Contains(stderr.String(), "assembly failed safely")) {
				t.Fatalf("runTraceCaseAssemble() = %d, stdout=%q, stderr=%q", got, stdout.String(), stderr.String())
			}
		})
	}
	if exitCode := runTraceCaseAssemble([]string{"--plan", "plan.json", "--output", "workspace"}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseAssemblySummary, error) {
		return trace.CaseAssemblySummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("human writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAssemble([]string{"--json", "--plan", "plan.json", "--output", "workspace"}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseAssemblySummary, error) {
		return trace.CaseAssemblySummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("JSON writer error = %d", exitCode)
	}
	if err := writeTraceCaseAssemblySummary(failingWriter{}, trace.CaseAssemblySummary{}); err == nil {
		t.Fatal("writeTraceCaseAssemblySummary() accepted a failing writer")
	}
}

func TestTraceCaseAssembleVerifyDispatch(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "assemble", "verify", "missing-workspace"}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "trace case assemble verify") {
		t.Fatalf("assembly verification dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
func TestTraceCaseAssembleDispatch(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "assemble", "--plan", "missing-plan.json", "--output", "workspace"}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "trace case assemble") {
		t.Fatalf("assembly dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
