package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseMap(t *testing.T) {
	result := trace.CaseDisclosureMap{
		SchemaVersion: 1,
		CaseSHA256:    strings.Repeat("a", 64),
		Traces:        4,
		CoverageState: evidence.Unknown,
		Categories: []trace.CaseDisclosureCategory{{
			Category: "location",
			Observations: []trace.CaseDisclosureObservation{{
				Source: "browser", Adapter: "browser-redacted-audit", Channel: "network", Kind: "request", Destination: "analytics", TraceCount: 2, EvidenceState: evidence.Observed,
			}},
		}},
	}
	mapper := func(path string) (trace.CaseDisclosureMap, error) {
		if path != "case.json" {
			t.Fatalf("map path = %q", path)
		}
		return result, nil
	}

	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMap([]string{"case.json"}, &stdout, &stderr, mapper); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "trace case disclosure map") || !strings.Contains(stdout.String(), "coverage_state: unknown") || !strings.Contains(stdout.String(), "category: location") || !strings.Contains(stdout.String(), "trace_count: 2") {
		t.Fatalf("human map = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseMap([]string{"--json", "case.json"}, &stdout, &stderr, mapper); exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"coverage_state":"unknown"`) || !strings.Contains(stdout.String(), `"category":"location"`) {
		t.Fatalf("JSON map = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

func TestRunTraceCaseMapFailures(t *testing.T) {
	for _, args := range [][]string{nil, {"case.json", "extra"}, {"--json=invalid", "case.json"}} {
		var stdout, stderr strings.Builder
		if exitCode := runTraceCaseMap(args, &stdout, &stderr, func(string) (trace.CaseDisclosureMap, error) {
			t.Fatal("mapper called for invalid usage")
			return trace.CaseDisclosureMap{}, nil
		}); exitCode != 2 || stdout.Len() != 0 || stderr.String() != usage {
			t.Fatalf("usage = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	}

	var stdout, stderr strings.Builder
	if exitCode := runTraceCaseMap([]string{"case.json"}, &stdout, &stderr, func(string) (trace.CaseDisclosureMap, error) {
		return trace.CaseDisclosureMap{}, errors.New("private path")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "private path") {
		t.Fatalf("mapper error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceCaseMap([]string{"case.json"}, &failAfterWriter{failAt: 1}, &stderr, func(string) (trace.CaseDisclosureMap, error) {
		return trace.CaseDisclosureMap{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human writer error = %d, stderr=%q", exitCode, stderr.String())
	}
	stderr.Reset()
	if exitCode := runTraceCaseMap([]string{"--json", "case.json"}, &failAfterWriter{failAt: 1}, &stderr, func(string) (trace.CaseDisclosureMap, error) {
		return trace.CaseDisclosureMap{}, nil
	}); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON writer error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestTraceCaseMapDispatch(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "map", "missing.json"}, &stdout, &stderr); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "trace case map") {
		t.Fatalf("trace case map dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
