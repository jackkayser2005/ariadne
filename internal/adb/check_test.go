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
	var calls [][]string
	run := func(_ context.Context, binary string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{binary}, args...))
		switch len(calls) {
		case 1:
			return []byte("Android Debug Bridge version 1.0.41\nVersion 35.0.2\n"), nil
		case 2:
			return []byte("device\n"), nil
		default:
			return []byte("package:/data/app/fixture/base.apk\n"), nil
		}
	}

	target, err := checkWith(
		context.Background(),
		"adb",
		"emulator-5554",
		"dev.ariadne.fixture",
		run,
	)
	if err != nil {
		t.Fatalf("checkWith() error = %v", err)
	}
	if target.Version != "1.0.41" ||
		target.Device != "emulator-5554" ||
		target.Package != "dev.ariadne.fixture" {
		t.Fatalf("checkWith() target = %#v", target)
	}

	wantCalls := [][]string{
		{"adb", "version"},
		{"adb", "-s", "emulator-5554", "get-state"},
		{"adb", "-s", "emulator-5554", "shell", "pm", "path", "dev.ariadne.fixture"},
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

func TestCheckFailures(t *testing.T) {
	const secret = "do-not-print-tool-output"

	tests := []struct {
		name    string
		run     commandRunner
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkWith(
				context.Background(),
				"adb",
				"emulator-5554",
				"dev.ariadne.fixture",
				test.run,
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

func TestADBHelperProcess(t *testing.T) {
	marker := os.Args[len(os.Args)-1]
	switch marker {
	case "normal":
		fmt.Println("device")
		os.Exit(0)
	case "oversized":
		fmt.Print(strings.Repeat("x", maxOutputBytes+1))
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
