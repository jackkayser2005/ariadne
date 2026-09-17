package main

import (
	"strings"
	"testing"
)

func TestTraceCaseMapRetainsLegacyMapCommandAndReservesSubcommands(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "map", "--json", "missing-case.json"}, &stdout, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "ariadne: trace case map:") {
		t.Fatalf("legacy map dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "case", "map", "questions"}, &stdout, &stderr); exitCode != 0 || !strings.Contains(stdout.String(), "trace case disclosure question catalog") || stderr.Len() != 0 {
		t.Fatalf("questions reservation = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := run([]string{"trace", "case", "map", "ask"}, &stdout, &stderr); exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("ask reservation = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
