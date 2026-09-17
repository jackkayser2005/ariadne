package main

import (
	"io"
	"os"
	"strings"
)

const ariadneColorEnv = "ARIADNE_COLOR"

const (
	ansiGreen = "\x1b[32m"
	ansiReset = "\x1b[0m"
)

// writeCLIStatus writes a human-readable success status. Color is opt-in and
// only applies when the destination is an interactive terminal.
func writeCLIStatus(stdout io.Writer, message string) error {
	if cliColorEnabled(stdout) {
		_, err := io.WriteString(stdout, ansiGreen+message+ansiReset)
		return err
	}
	_, err := io.WriteString(stdout, message)
	return err
}

func cliColorEnabled(stdout io.Writer) bool {
	if !colorRequested() {
		return false
	}
	file, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func colorRequested() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(ariadneColorEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
