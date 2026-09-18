package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRunServeMinimizationFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var gotHandler http.Handler
	exitCode := runServe([]string{
		"--minimization", "minimization-run",
		"archive-root",
	}, &stdout, &stderr, func(_ string, handler http.Handler) error {
		gotHandler = handler
		return nil
	})
	if exitCode != 0 || gotHandler == nil || stderr.Len() != 0 || !strings.Contains(stdout.String(), "review UI listening") {
		t.Fatalf("runServe() minimization = %d, handler=%v, stdout=%q, stderr=%q", exitCode, gotHandler, stdout.String(), stderr.String())
	}
}

func TestLoopbackAddressRequiresCanonicalAuthority(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{address: "127.0.0.1:8787", want: true},
		{address: "[::1]:8787", want: true},
		{address: "[0:0:0:0:0:0:0:1]:8787", want: false},
		{address: "127.0.0.1:08787", want: false},
		{address: "127.0.0.1:0", want: false},
		{address: "127.0.0.1:http", want: false},
		{address: "localhost:8787", want: false},
		{address: "127.0.0.1", want: false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := loopbackAddress(test.address); got != test.want {
				t.Fatalf("loopbackAddress(%q) = %t, want %t", test.address, got, test.want)
			}
		})
	}
}
