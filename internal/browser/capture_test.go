package browser

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func init() {
	switch os.Getenv("ARIADNE_BROWSER_TEST_DRIVER") {
	case "success":
		_, _ = io.WriteString(os.Stdout, string(validDriverAudit))
		os.Exit(0)
	case "stdout-overflow":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", maxAuditBytes*2))
		os.Exit(0)
	case "stderr-overflow":
		_, _ = io.WriteString(os.Stderr, strings.Repeat("x", maxDriverStderrBytes*2))
		os.Exit(0)
	case "sleep":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
}

var validDriverAudit = []byte(`{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[{"channel":"network","kind":"request","destination":"analytics","fields":["region"]}]}`)

func TestCaptureWithRunnerBindsProcedureAndWritesTrace(t *testing.T) {
	procedurePath := writeProcedure(t, validProcedureData)
	outputPath := filepath.Join(t.TempDir(), "nested", "trace.json")
	var received []byte
	summary, err := captureWithRunner(procedurePath, "driver", outputPath, func(ctx context.Context, driver string, procedure []byte) ([]byte, error) {
		if driver != "driver" || ctx.Err() != nil {
			t.Fatalf("runner args = driver %q, context error %v", driver, ctx.Err())
		}
		received = append([]byte(nil), procedure...)
		return validDriverAudit, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(received) != string(validProcedureData) {
		t.Fatalf("driver procedure = %s", received)
	}
	procedure, err := DecodeProcedure(validProcedureData)
	if err != nil {
		t.Fatal(err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProcedureSHA256 != procedureSHA256 || summary.Trace.Scope != "outbound" || summary.Trace.Events != 1 || summary.Trace.TraceSHA256 == "" {
		t.Fatalf("summary = %#v", summary)
	}
	document, err := portabletrace.Read(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Events) != 1 || document.Events[0].Source != "browser" || document.Events[0].Fields[0] != "region" {
		t.Fatalf("document = %#v", document)
	}
	if _, err := captureWithRunner(procedurePath, "driver", outputPath, func(context.Context, string, []byte) ([]byte, error) {
		return validDriverAudit, nil
	}); err == nil {
		t.Fatal("capture overwrote an existing trace")
	}
}

func TestCaptureInvokesSelectedDriver(t *testing.T) {
	t.Setenv("ARIADNE_BROWSER_TEST_DRIVER", "success")
	outputPath := filepath.Join(t.TempDir(), "trace.json")
	summary, err := Capture(writeProcedure(t, longProcedureData), os.Args[0], outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProcedureSHA256 == "" || summary.Trace.TraceSHA256 == "" || summary.Trace.Events != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestCaptureWithRunnerRejectsUnsafeDriverResults(t *testing.T) {
	procedurePath := writeProcedure(t, validProcedureData)
	tests := []struct {
		name string
		data []byte
		want string
		run  captureRunner
	}{
		{name: "invalid audit", data: []byte(`{"schema_version":1}`), want: "browser capture output"},
		{name: "scope mismatch", data: []byte(`{"schema_version":1,"redacted":true,"scope":"storage","completeness":"complete","events":[]}`), want: "does not match procedure"},
		{name: "event limit", data: []byte(`{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[{"channel":"network","kind":"request","destination":"analytics","fields":["region"]},{"channel":"network","kind":"response","destination":"first-party","fields":["consent"]},{"channel":"cookie","kind":"cookie-write","destination":"advertising","fields":["consent"]}]}`), want: "does not match procedure"},
		{name: "driver error", want: "driver unavailable", run: func(context.Context, string, []byte) ([]byte, error) { return nil, errors.New("driver unavailable") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := test.run
			if run == nil {
				run = func(context.Context, string, []byte) ([]byte, error) { return test.data, nil }
			}
			_, err := captureWithRunner(procedurePath, "driver", filepath.Join(t.TempDir(), "trace.json"), run)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "unavailable") && test.name != "driver error" {
				t.Fatalf("error exposed unexpected input: %v", err)
			}
		})
	}
	if _, err := captureWithRunner("", "driver", "trace.json", func(context.Context, string, []byte) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("capture accepted empty procedure path")
	}
}

func TestRunDriverBoundsAndTimesOut(t *testing.T) {
	t.Setenv("ARIADNE_BROWSER_TEST_DRIVER", "success")
	output, err := runDriver(context.Background(), os.Args[0], []byte("procedure"))
	if err != nil || string(output) != string(validDriverAudit) {
		t.Fatalf("successful driver output = %q, err = %v", output, err)
	}

	t.Setenv("ARIADNE_BROWSER_TEST_DRIVER", "stdout-overflow")
	if output, err := runDriver(context.Background(), os.Args[0], nil); err == nil || !strings.Contains(err.Error(), "output exceeds limit") {
		t.Fatalf("stdout overflow output length = %d, error = %v", len(output), err)
	}
	t.Setenv("ARIADNE_BROWSER_TEST_DRIVER", "stderr-overflow")
	if _, err := runDriver(context.Background(), os.Args[0], nil); err == nil || !strings.Contains(err.Error(), "diagnostics exceed limit") {
		t.Fatalf("stderr overflow error = %v", err)
	}
	t.Setenv("ARIADNE_BROWSER_TEST_DRIVER", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := runDriver(ctx, os.Args[0], nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}
	if _, err := runDriver(context.Background(), filepath.Join(t.TempDir(), "missing-driver.exe"), nil); err == nil || !strings.Contains(err.Error(), "driver failed") {
		t.Fatalf("missing driver error = %v", err)
	}
	if _, err := runDriver(context.Background(), "", nil); err == nil {
		t.Fatal("runDriver accepted empty driver")
	}
}

func TestBoundedBuffer(t *testing.T) {
	var buffer boundedBuffer
	buffer.limit = 2
	if _, err := buffer.Write([]byte("ab")); err != nil || buffer.String() != "ab" {
		t.Fatalf("exact write = %q, err = %v", buffer.String(), err)
	}
	if _, err := buffer.Write([]byte("c")); err == nil || !buffer.overflow || buffer.String() != "ab" {
		t.Fatalf("overflow write = %q, err = %v, overflow=%v", buffer.String(), err, buffer.overflow)
	}
}

func TestDecodeProcedureRejectsMalformedAndOutOfBoundsInput(t *testing.T) {
	valid := string(validProcedureData)
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "unknown field", data: strings.Replace(valid, `"scope"`, `"url":"https://private.invalid","scope"`, 1)},
		{name: "duplicate", data: strings.Replace(valid, `{"schema_version":1,`, `{"schema_version":1,"schema_version":1,`, 1)},
		{name: "trailing", data: valid + "{}"},
		{name: "schema", data: strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1)},
		{name: "procedure id", data: strings.Replace(valid, `"procedure_id":"browser-audit-v1"`, `"procedure_id":"custom"`, 1)},
		{name: "scope", data: strings.Replace(valid, `"scope":"outbound"`, `"scope":"custom"`, 1)},
		{name: "short duration", data: strings.Replace(valid, `"duration_ms":500`, `"duration_ms":99`, 1)},
		{name: "long duration", data: strings.Replace(valid, `"duration_ms":500`, `"duration_ms":300001`, 1)},
		{name: "empty event limit", data: strings.Replace(valid, `"max_events":2`, `"max_events":0`, 1)},
		{name: "large event limit", data: strings.Replace(valid, `"max_events":2`, `"max_events":1025`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeProcedure([]byte(test.data)); err == nil {
				t.Fatal("DecodeProcedure() accepted invalid input")
			} else if strings.Contains(err.Error(), "private.invalid") {
				t.Fatalf("DecodeProcedure() exposed input: %v", err)
			}
		})
	}
	if _, err := DecodeProcedure([]byte(strings.Repeat("x", maxProcedureBytes+1))); err == nil {
		t.Fatal("DecodeProcedure() accepted oversized input")
	}
	if _, err := DecodeProcedure([]byte{'{', 0xff}); err == nil {
		t.Fatal("DecodeProcedure() accepted invalid UTF-8")
	}
}

func TestReadProcedureAndIdentity(t *testing.T) {
	path := writeProcedure(t, validProcedureData)
	procedure, data, err := ReadProcedure(path)
	if err != nil || string(data) != string(validProcedureData) || procedure.ProcedureID != BrowserAuditProcedureID {
		t.Fatalf("ReadProcedure() = %#v, %q, err = %v", procedure, data, err)
	}
	first, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProcedureSHA256(procedure)
	if err != nil || first != second || !portabletrace.ValidSHA256(first) {
		t.Fatalf("procedure identity = %q, %q, err = %v", first, second, err)
	}
	if _, _, err := ReadProcedure(""); err == nil {
		t.Fatal("ReadProcedure() accepted empty path")
	}
	if _, _, err := ReadProcedure(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadProcedure() accepted missing path")
	}
	oversized := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", maxProcedureBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadProcedure(oversized); err == nil {
		t.Fatal("ReadProcedure() accepted oversized file")
	}
	if _, err := ProcedureSHA256(Procedure{}); err == nil {
		t.Fatal("ProcedureSHA256() accepted invalid procedure")
	}
}

func writeProcedure(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "procedure.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

var validProcedureData = []byte(`{"schema_version":1,"procedure_id":"browser-audit-v1","scope":"outbound","duration_ms":500,"max_events":2}`)

var longProcedureData = []byte(`{"schema_version":1,"procedure_id":"browser-audit-v1","scope":"outbound","duration_ms":5000,"max_events":2}`)
