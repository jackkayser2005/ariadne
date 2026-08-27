package main

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestRunServeAcceptsMinimizationQuestionArtifacts(t *testing.T) {
	var stdout, stderr bytes.Buffer
	called := false
	args := []string{
		"--minimization", "run",
		"--minimization-round", "round.json",
		"--minimization-receipt", "receipt.json",
		"archive-root",
	}
	if exitCode := runServe(args, &stdout, &stderr, func(address string, handler http.Handler) error {
		called = address == "127.0.0.1:8787" && handler != nil
		return nil
	}); exitCode != 0 || !called || stderr.Len() != 0 {
		t.Fatalf("runServe() = %d, called=%v, stdout=%q, stderr=%q", exitCode, called, stdout.String(), stderr.String())
	}
}

func TestRunServeRejectsMinimizationArtifactsWithoutRun(t *testing.T) {
	for _, args := range [][]string{
		{"--minimization-round", "round.json", "archive-root"},
		{"--minimization-receipt", "receipt.json", "archive-root"},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := runServe(args, &stdout, &stderr, func(string, http.Handler) error {
			return errors.New("server should not start")
		}); exitCode != 2 || !strings.Contains(stderr.String(), "require --minimization") {
			t.Fatalf("runServe(%v) = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
}
