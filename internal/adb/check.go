// Package adb verifies explicitly selected Android test targets.
package adb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// Target identifies one ready device and installed package.
type Target struct {
	Version            string
	Device             string
	Package            string
	AndroidAPI         int
	Architecture       string
	PackageVersionCode uint64
	PackageSHA256      string
	AriadneRevision    string
	AriadneModified    bool
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)
type packageHasher func(context.Context, string, string, string) (string, error)

const maxOutputBytes = 64 << 10
const maxPackageBytes = 256 << 20

// Check verifies that adb can reach device and find package.
func Check(ctx context.Context, binary, device, packageName string) (Target, error) {
	return checkWith(
		ctx,
		binary,
		device,
		packageName,
		runCommand,
		hashInstalledPackage,
	)
}

func checkWith(
	ctx context.Context,
	binary, device, packageName string,
	run commandRunner,
	hash packageHasher,
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
	packagePath, err := parsePackagePath(path)
	if err != nil {
		return Target{}, err
	}

	apiOutput, err := run(
		ctx,
		binary,
		"-s", device,
		"shell", "getprop", "ro.build.version.sdk",
	)
	if err != nil {
		return Target{}, fmt.Errorf("read Android API: %w", err)
	}
	androidAPI, err := parseAndroidAPI(apiOutput)
	if err != nil {
		return Target{}, err
	}

	architectureOutput, err := run(
		ctx,
		binary,
		"-s", device,
		"shell", "getprop", "ro.product.cpu.abi",
	)
	if err != nil {
		return Target{}, fmt.Errorf("read architecture: %w", err)
	}
	architecture, err := parseArchitecture(architectureOutput)
	if err != nil {
		return Target{}, err
	}

	versionOutput, err = run(
		ctx,
		binary,
		"-s", device,
		"shell", "pm", "list", "packages", "--show-versioncode", packageName,
	)
	if err != nil {
		return Target{}, fmt.Errorf("read package version: %w", err)
	}
	versionCode, err := parsePackageVersion(versionOutput, packageName)
	if err != nil {
		return Target{}, err
	}

	packageSHA256, err := hash(ctx, binary, device, packagePath)
	if err != nil {
		return Target{}, fmt.Errorf("hash installed package: %w", err)
	}

	return Target{
		Version:            version,
		Device:             device,
		Package:            packageName,
		AndroidAPI:         androidAPI,
		Architecture:       architecture,
		PackageVersionCode: versionCode,
		PackageSHA256:      packageSHA256,
	}, nil
}

func parsePackagePath(output []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "package:") {
		return "", errors.New("selected package is not installed as one visible APK")
	}
	path := strings.TrimPrefix(lines[0], "package:")
	if !strings.HasPrefix(path, "/") || !validSelection(path) {
		return "", errors.New("selected package path is invalid")
	}
	return path, nil
}

func parseAndroidAPI(output []byte) (int, error) {
	value := strings.TrimSpace(string(output))
	api, err := strconv.Atoi(value)
	if err != nil || api < 1 || api > 999 {
		return 0, errors.New("Android API: unexpected output")
	}
	return api, nil
}

func parseArchitecture(output []byte) (string, error) {
	value := strings.TrimSpace(string(output))
	if value == "" || len(value) > 64 {
		return "", errors.New("architecture: unexpected output")
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '.' &&
			character != '_' &&
			character != '-' {
			return "", errors.New("architecture: unexpected output")
		}
	}
	return value, nil
}

func parsePackageVersion(output []byte, packageName string) (uint64, error) {
	prefix := "package:" + packageName + " versionCode:"
	value := strings.TrimSpace(string(output))
	if !strings.HasPrefix(value, prefix) || strings.Contains(value, "\n") {
		return 0, errors.New("package version: unexpected output")
	}
	versionCode, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || versionCode == 0 {
		return 0, errors.New("package version: unexpected output")
	}
	return versionCode, nil
}

func validSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil &&
		len(decoded) == sha256.Size &&
		value == strings.ToLower(value)
}

func validAriadneRevision(value string) bool {
	if value == "unknown" {
		return true
	}
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
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

func hashInstalledPackage(
	ctx context.Context,
	binary, device, packagePath string,
) (string, error) {
	return hashCommand(
		ctx,
		binary,
		maxPackageBytes,
		"-s", device,
		"exec-out", "cat", packagePath,
	)
}

func hashCommand(
	ctx context.Context,
	binary string,
	limit int64,
	args ...string,
) (string, error) {
	command := exec.CommandContext(ctx, binary, args...)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open package output: %w", err)
	}
	if err := command.Start(); err != nil {
		return "", err
	}

	digest := sha256.New()
	size, readErr := io.Copy(digest, io.LimitReader(stdout, limit+1))
	if size > limit {
		_ = command.Process.Kill()
		_ = command.Wait()
		return "", fmt.Errorf("installed package exceeds %d-byte limit", limit)
	}
	if readErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return "", fmt.Errorf("read package: %w", readErr)
	}
	if err := command.Wait(); err != nil {
		return "", err
	}
	if size == 0 {
		return "", errors.New("installed package is empty")
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
