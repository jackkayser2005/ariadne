package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRunServeAcceptsTraceCaseArtifacts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var gotHandler http.Handler
	exitCode := runServe([]string{
		"--trace-case", "case.json",
		"--trace-case-round", "round.json",
		"--trace-case-receipt", "receipt.json",
		"archive-root",
	}, &stdout, &stderr, func(_ string, handler http.Handler) error {
		gotHandler = handler
		return nil
	})
	if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
		t.Fatalf("runServe() = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
	}
}

func TestRunServeRejectsOrphanedTraceCaseArtifacts(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "round", args: []string{"--trace-case-round", "round.json", "archive-root"}},
		{name: "receipt", args: []string{"--trace-case-receipt", "receipt.json", "archive-root"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runServe(test.args, &stdout, &stderr, func(string, http.Handler) error {
				t.Fatal("server called for orphaned trace case artifact")
				return nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != "ariadne: experiment serve: --trace-case-round and --trace-case-receipt require --trace-case\n" {
				t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
}
