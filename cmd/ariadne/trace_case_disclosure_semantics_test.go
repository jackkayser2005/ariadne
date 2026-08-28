package main

import (
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestTraceCaseDisclosureCLIProjectsPartialSemantics(t *testing.T) {
	positive := trace.CaseDisclosureQuestionAnswer{
		QuestionID: trace.CaseDisclosureQuestionOverlap,
		Question:   "Which reviewed categories appeared across multiple source or adapter boundaries?",
		Result:     "overlap-observed", EvidenceState: evidence.Observed,
		Reason:     "one or more reviewed categories appeared across multiple source or adapter boundaries",
		CaseSHA256: strings.Repeat("a", 64), Traces: 2, CoverageState: evidence.Unknown,
		Categories:            []trace.CaseDisclosureCategorySummary{{Category: "region", Boundaries: []trace.CaseDisclosureBoundary{{Source: "browser", Adapter: "browser-redacted-audit"}, {Source: "browser", Adapter: "browser-local-fixture"}}}},
		OverlappingCategories: []string{"region"},
	}
	negative := positive
	negative.Result = "unknown"
	negative.EvidenceState = evidence.Unknown
	negative.Reason = "no cross-boundary overlap was observed, but partial coverage prevents a no-overlap conclusion"
	negative.Categories = []trace.CaseDisclosureCategorySummary{{Category: "region", Boundaries: []trace.CaseDisclosureBoundary{{Source: "browser", Adapter: "browser-redacted-audit"}}}}
	negative.OverlappingCategories = nil
	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMapAskAll([]string{"case.json"}, &stdout, &stderr, func(string) ([]trace.CaseDisclosureQuestionAnswer, error) {
		return []trace.CaseDisclosureQuestionAnswer{positive, negative}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "overlap-observed") || !strings.Contains(stdout.String(), "no cross-boundary overlap") {
		t.Fatalf("partial human semantics = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceCaseMapAskAll([]string{"--json", "case.json"}, &stdout, &stderr, func(string) ([]trace.CaseDisclosureQuestionAnswer, error) {
		return []trace.CaseDisclosureQuestionAnswer{positive, negative}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), `"result":"overlap-observed"`) || !strings.Contains(stdout.String(), `"result":"unknown"`) || stderr.Len() != 0 {
		t.Fatalf("partial semantics = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
