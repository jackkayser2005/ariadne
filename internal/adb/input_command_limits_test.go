package adb

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRunInputCommandRejectsOversizedOutput(t *testing.T) {
	t.Setenv("ARIADNE_LARGE_INPUT_HELPER", "1")
	if _, err := runInputCommand(
		context.Background(),
		os.Args[0],
		nil,
		"-test.run=TestRunInputCommandLargeOutputHelper",
	); err == nil || !strings.Contains(err.Error(), "output exceeds") {
		t.Fatalf("runInputCommand() error = %v, want output limit", err)
	}
}

func TestRunInputCommandLargeOutputHelper(t *testing.T) {
	if os.Getenv("ARIADNE_LARGE_INPUT_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.Write([]byte(strings.Repeat("x", maxOutputBytes+1)))
	os.Exit(0)
}
