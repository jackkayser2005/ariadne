package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWriteCLIStatusStaysPlainForNonTerminal(t *testing.T) {
	t.Setenv(ariadneColorEnv, "1")
	var stdout bytes.Buffer

	if err := writeCLIStatus(&stdout, "ready\n"); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "ready\n" {
		t.Fatalf("status = %q, want plain text", got)
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("status unexpectedly contains ANSI escape code: %q", stdout.String())
	}
}

func TestColorRequestedAcceptsExplicitValues(t *testing.T) {
	for _, value := range []string{"1", "true", "yes", "on"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(ariadneColorEnv, value)
			if !colorRequested() {
				t.Fatalf("colorRequested() = false for %q", value)
			}
		})
	}

	for _, value := range []string{"", "0", "false", "always"} {
		t.Run("disabled_"+value, func(t *testing.T) {
			t.Setenv(ariadneColorEnv, value)
			if colorRequested() {
				t.Fatalf("colorRequested() = true for %q", value)
			}
		})
	}
}

func TestWriteCLIStatusReturnsWriteError(t *testing.T) {
	want := errors.New("status write failed")
	if err := writeCLIStatus(cliStyleFailWriter{err: want}, "ready\n"); !errors.Is(err, want) {
		t.Fatalf("writeCLIStatus() error = %v, want %v", err, want)
	}
}

type cliStyleFailWriter struct {
	err error
}

func (w cliStyleFailWriter) Write([]byte) (int, error) {
	return 0, w.err
}
