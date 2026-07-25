// Package adb verifies explicitly selected Android test targets.
package adb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode"
)

// Target identifies one ready device and installed package.
type Target struct {
	Version string
	Device  string
	Package string
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)

const maxOutputBytes = 64 << 10

// Check verifies that adb can reach device and find package.
func Check(ctx context.Context, binary, device, packageName string) (Target, error) {
	return checkWith(ctx, binary, device, packageName, runCommand)
}

func checkWith(
	ctx context.Context,
	binary, device, packageName string,
	run commandRunner,
) (Target, error) {
	if !validSelection(binary) {
		return Target{}, errors.New("adb binary is invalid")
	}
	if !validSelection(device) {
		return Target{}, errors.New("device is invalid")
	}
	if !validSelection(packageName) {
		return Target{}, errors.New("package is invalid")
	}

	versionOutput, err := run(ctx, binary, "version")
	if err != nil {
		return Target{}, fmt.Errorf("adb version: %w", err)
	}
	version, err := parseVersion(versionOutput)
	if err != nil {
		return Target{}, err
	}

	state, err := run(ctx, binary, "-s", device, "get-state")
	if err != nil {
		return Target{}, fmt.Errorf("check device: %w", err)
	}
	if strings.TrimSpace(string(state)) != "device" {
		return Target{}, errors.New("selected device is not ready")
	}

	path, err := run(ctx, binary, "-s", device, "shell", "pm", "path", packageName)
	if err != nil {
		return Target{}, fmt.Errorf("check package: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(path)), "package:") {
		return Target{}, errors.New("selected package is not installed or visible")
	}

	return Target{
		Version: version,
		Device:  device,
		Package: packageName,
	}, nil
}

func parseVersion(output []byte) (string, error) {
	const prefix = "Android Debug Bridge version "

	line, _, _ := strings.Cut(string(output), "\n")
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, prefix) {
		return "", errors.New("adb version: unexpected output")
	}
	version := strings.TrimPrefix(line, prefix)
	if version == "" {
		return "", errors.New("adb version: unexpected output")
	}
	for _, character := range version {
		if (character < '0' || character > '9') && character != '.' {
			return "", errors.New("adb version: unexpected output")
		}
	}
	return version, nil
}

func validSelection(value string) bool {
	return value != "" &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func runCommand(ctx context.Context, binary string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open adb output: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, err
	}

	output, readErr := io.ReadAll(io.LimitReader(stdout, maxOutputBytes+1))
	if len(output) > maxOutputBytes {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("adb output exceeds 65536-byte limit")
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("read adb output: %w", readErr)
	}
	if err := command.Wait(); err != nil {
		return nil, err
	}
	return output, nil
}
