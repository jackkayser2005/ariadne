package adb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	const packageDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var calls [][]string
	run := func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		switch len(calls) {
		case 1:
			return []byte("Android Debug Bridge version 1.0.41\nVersion 35.0.2\n"), nil
		case 2:
			return []byte("device\n"), nil
		case 3:
			return []byte("package:/data/app/fixture/base.apk\n"), nil
		case 4:
			return []byte("35\n"), nil
		case 5:
			return []byte("x86_64\n"), nil
		default:
			return []byte("package:dev.ariadne.fixture versionCode:1\n"), nil
		}
	}
	hash := func(
		_ context.Context,
		binary, device, packagePath string,
	) (string, error) {
		if binary != "adb" ||
			device != "emulator-5554" ||
			packagePath != "/data/app/fixture/base.apk" {
			t.Fatalf(
				"hash() arguments = %q, %q, %q",
				binary,
				device,
				packagePath,
			)
		}
		return packageDigest, nil
	}

	target, err := checkWith(
		context.Background(),
		"adb",
		"emulator-5554",
		"dev.ariadne.fixture",
		run,
		hash,
	)
	if err != nil {
		t.Fatalf("checkWith() error = %v", err)
	}
	if target.Version != "1.0.41" ||
		target.Device != "emulator-5554" ||
		target.Package != "dev.ariadne.fixture" ||
		target.AndroidAPI != 35 ||
		target.Architecture != "x86_64" ||
		target.PackageVersionCode != 1 ||
		target.PackageSHA256 != packageDigest {
		t.Fatalf("checkWith() target = %#v", target)
	}

	wantCalls := [][]string{
		{"adb", "version"},
		{"adb", "-s", "emulator-5554", "get-state"},
		{"adb", "-s", "emulator-5554", "shell", "pm", "path", "dev.ariadne.fixture"},
		{"adb", "-s", "emulator-5554", "shell", "getprop", "ro.build.version.sdk"},
		{"adb", "-s", "emulator-5554", "shell", "getprop", "ro.product.cpu.abi"},
		{
			"adb", "-s", "emulator-5554",
			"shell", "pm", "list", "packages", "--show-versioncode",
			"dev.ariadne.fixture",
		},
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("checkWith() calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestCheckRejectsMissingSelection(t *testing.T) {
	tests := []struct {
		name        string
		binary      string
		device      string
		packageName string
		want        string
	}{
		{name: "binary", device: "device", packageName: "package", want: "binary"},
		{name: "device", binary: "adb", packageName: "package", want: "device"},
		{name: "package", binary: "adb", device: "device", want: "package"},
		{
			name:        "control character",
			binary:      "adb",
			device:      "device\nforged",
			packageName: "package",
			want:        "device",
		},
		{
			name:        "package shell punctuation",
			binary:      "adb",
			device:      "device",
			packageName: "dev.ariadne.fixture;id",
			want:        "package",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			run := func(context.Context, string, ...string) ([]byte, error) {
				called = true
				return nil, nil
			}

			_, err := checkWith(
				context.Background(),
				test.binary,
				test.device,
				test.packageName,
				run,
				func(context.Context, string, string, string) (string, error) {
					t.Fatal("hash called for invalid input")
					return "", nil
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("checkWith() error = %v, want containing %q", err, test.want)
			}
			if called {
				t.Fatal("checkWith() invoked adb for invalid input")
			}
		})
	}
}

func TestValidPackageName(t *testing.T) {
	for _, value := range []string{"dev.ariadne.fixture", "dev.ariadne_fixture.v2"} {
		if !validPackageName(value) {
			t.Fatalf("validPackageName(%q) = false", value)
		}
	}
	for _, value := range []string{
		"dev..fixture",
		"dev.1fixture",
		"dev.foo-bar",
		strings.Repeat("a", 256),
	} {
		if validPackageName(value) {
			t.Fatalf("validPackageName(%q) = true", value)
		}
	}
}

func TestCheckFailures(t *testing.T) {
	const secret = "do-not-print-tool-output"

	tests := []struct {
		name    string
		run     commandRunner
		hash    packageHasher
		wantErr string
	}{
		{
			name: "version command",
			run: func(context.Context, string, ...string) ([]byte, error) {
				return []byte(secret), errors.New("exit status 1")
			},
			wantErr: "adb version",
		},
		{
			name: "version output",
			run: func(context.Context, string, ...string) ([]byte, error) {
				return []byte(secret), nil
			},
			wantErr: "unexpected output",
		},
		{
			name: "device command",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				resultError(secret, errors.New("exit status 1")),
			),
			wantErr: "check device",
		},
		{
			name: "device not ready",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result(secret),
			),
			wantErr: "not ready",
		},
		{
			name: "package command",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				resultError(secret, errors.New("exit status 1")),
			),
			wantErr: "check package",
		},
		{
			name: "package missing",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result(secret),
			),
			wantErr: "not installed",
		},
		{
			name: "split package",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\npackage:/split.apk\n"),
			),
			wantErr: "one visible APK",
		},
		{
			name: "Android API command",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				resultError(secret, errors.New("exit status 1")),
			),
			wantErr: "read Android API",
		},
		{
			name: "Android API output",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result(secret),
			),
			wantErr: "unexpected output",
		},
		{
			name: "architecture command",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result("35\n"),
				resultError(secret, errors.New("exit status 1")),
			),
			wantErr: "read architecture",
		},
		{
			name: "architecture output",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result("35\n"),
				result(secret+"/"),
			),
			wantErr: "architecture",
		},
		{
			name: "package version command",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result("35\n"),
				result("x86_64\n"),
				resultError(secret, errors.New("exit status 1")),
			),
			wantErr: "read package version",
		},
		{
			name: "package version output",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result("35\n"),
				result("x86_64\n"),
				result(secret),
			),
			wantErr: "package version",
		},
		{
			name: "package hash",
			run: sequence(
				result("Android Debug Bridge version 1.0.41\n"),
				result("device\n"),
				result("package:/base.apk\n"),
				result("35\n"),
				result("x86_64\n"),
				result("package:dev.ariadne.fixture versionCode:1\n"),
			),
			hash: func(context.Context, string, string, string) (string, error) {
				return "", errors.New("exit status 1")
			},
			wantErr: "hash installed package",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hash := test.hash
			if hash == nil {
				hash = func(context.Context, string, string, string) (string, error) {
					return strings.Repeat("a", 64), nil
				}
			}
			_, err := checkWith(
				context.Background(),
				"adb",
				"emulator-5554",
				"dev.ariadne.fixture",
				test.run,
				hash,
			)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("checkWith() error = %v, want containing %q", err, test.wantErr)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("checkWith() exposed tool output: %v", err)
			}
		})
	}
}

func TestRunCommand(t *testing.T) {
	output, err := runCommand(
		context.Background(),
		os.Args[0],
		"-test.run=TestADBHelperProcess",
		"--",
		"normal",
	)
	if err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}
	if string(output) != "device\n" {
		t.Fatalf("runCommand() output = %q", output)
	}

	_, err = runCommand(
		context.Background(),
		os.Args[0],
		"-test.run=TestADBHelperProcess",
		"--",
		"oversized",
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("runCommand() error = %v, want output limit error", err)
	}
}

func TestHashCommand(t *testing.T) {
	digest, err := hashCommand(
		context.Background(),
		os.Args[0],
		64,
		"-test.run=TestADBHelperProcess",
		"--",
		"digest",
	)
	if err != nil {
		t.Fatalf("hashCommand() error = %v", err)
	}
	const want = "f16d05ec6b29248d2c61adb1e9263f78e4f7bace1b955014a2d17872cfe4064d"
	if digest != want {
		t.Fatalf("hashCommand() = %q, want %q", digest, want)
	}

	_, err = hashCommand(
		context.Background(),
		os.Args[0],
		8,
		"-test.run=TestADBHelperProcess",
		"--",
		"oversized-digest",
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("hashCommand() error = %v, want size error", err)
	}

	_, err = hashCommand(
		context.Background(),
		os.Args[0],
		8,
		"-test.run=TestADBHelperProcess",
		"--",
		"empty-digest",
	)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("hashCommand() error = %v, want empty error", err)
	}
}

func TestADBHelperProcess(t *testing.T) {
	marker := os.Args[len(os.Args)-1]
	switch marker {
	case "normal":
		fmt.Println("device")
		os.Exit(0)
	case "oversized":
		fmt.Print(strings.Repeat("x", maxOutputBytes+1))
		os.Exit(0)
	case "exit-seven":
		os.Exit(7)
	case "digest":
		fmt.Print("fixture")
		os.Exit(0)
	case "oversized-digest":
		fmt.Print("123456789")
		os.Exit(0)
	case "empty-digest":
		os.Exit(0)
	}
}

type commandResult struct {
	output []byte
	err    error
}

func result(output string) commandResult {
	return commandResult{output: []byte(output)}
}

func resultError(output string, err error) commandResult {
	return commandResult{output: []byte(output), err: err}
}

func sequence(results ...commandResult) commandRunner {
	next := 0
	return func(context.Context, string, ...string) ([]byte, error) {
		result := results[next]
		next++
		return result.output, result.err
	}
}
