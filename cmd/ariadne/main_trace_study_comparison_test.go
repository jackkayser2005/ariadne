package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRunServeTraceStudyComparisonFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var gotHandler http.Handler
	exitCode := runServe([]string{
		"--trace-study", "first-study.json",
		"--trace-study-round", "first-round.json",
		"--trace-study-second", "second-study.json",
		"--trace-study-round-second", "second-round.json",
		"archive-root",
	}, &stdout, &stderr, func(_ string, handler http.Handler) error {
		gotHandler = handler
		return nil
	})
	if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
		t.Fatalf("runServe() comparison = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
	}
}

func TestRunServeRejectsPartialTraceStudyComparisonFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "second pair",
			args: []string{"--trace-study", "first-study.json", "--trace-study-round", "first-round.json", "--trace-study-second", "second-study.json", "archive-root"},
			want: "ariadne: experiment serve: --trace-study-second and --trace-study-round-second must be supplied together\n",
		},
		{
			name: "first pair",
			args: []string{"--trace-study", "first-study.json", "--trace-study-second", "second-study.json", "--trace-study-round-second", "second-round.json", "archive-root"},
			want: "ariadne: experiment serve: --trace-study-second and --trace-study-round-second require --trace-study and --trace-study-round\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runServe(test.args, &stdout, &stderr, func(string, http.Handler) error {
				t.Fatal("server called for invalid comparison flags")
				return nil
			})
			if exitCode != 2 || stdout.Len() != 0 || stderr.String() != test.want {
				t.Fatalf("runServe() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
			}
		})
	}
}
