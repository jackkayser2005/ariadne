package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestBuildTraceProjectsSafeBrowserAudit(t *testing.T) {
	audit := []byte(`{
  "schema_version": 1,
  "redacted": true,
  "scope": "outbound",
  "completeness": "complete",
  "events": [
    {"channel":"cookie","kind":"cookie-write","destination":"advertising","fields":["consent"]},
    {"channel":"network","kind":"request","destination":"analytics","fields":["region","session-id"]}
  ]
}`)
	document, err := BuildTrace(audit)
	if err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || !document.Redacted || document.Scope != "outbound" || document.Completeness != portabletrace.Complete || len(document.Events) != 2 {
		t.Fatalf("document = %#v", document)
	}
	for _, event := range document.Events {
		if event.Source != "browser" {
			t.Fatalf("event source = %q", event.Source)
		}
	}
	if document.Events[1].Fields[0] != "region" || document.Events[1].Fields[1] != "session-id" {
		t.Fatalf("fields were not normalized: %#v", document.Events[1].Fields)
	}
	if _, err := portabletrace.SHA256(document); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTraceRejectsInvalidAudit(t *testing.T) {
	if _, err := BuildTrace([]byte("not-json")); err == nil {
		t.Fatal("BuildTrace() accepted invalid audit")
	}
}

func TestBuildTraceAcceptsReviewedBrowserEventKinds(t *testing.T) {
	data := []byte(`{
  "schema_version": 1,
  "redacted": true,
  "scope": "all",
  "completeness": "partial",
  "events": [
    {"channel":"network","kind":"beacon","destination":"analytics","fields":["region"]},
    {"channel":"network","kind":"response","destination":"first-party","fields":["consent"]},
    {"channel":"web-storage","kind":"storage-write","destination":"advertising","fields":["cookie-id"]}
  ]
}`)
	document, err := BuildTrace(data)
	if err != nil || len(document.Events) != 3 || document.Completeness != portabletrace.Partial {
		t.Fatalf("BuildTrace() = %#v, err = %v", document, err)
	}
}

func TestSaveTraceReturnsIdentityAndDoesNotOverwrite(t *testing.T) {
	input := filepath.Join(t.TempDir(), "audit.json")
	output := filepath.Join(t.TempDir(), "nested", "trace.json")
	if err := os.WriteFile(input, validAudit(t), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := SaveTrace(input, output)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TraceSHA256 == "" || summary.Scope != "outbound" || summary.Events != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if _, err := SaveTrace(input, output); err == nil {
		t.Fatal("SaveTrace() overwrote an existing trace")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "https://") || strings.Contains(string(data), "payload") {
		t.Fatalf("trace contained source details: %s", data)
	}
}

func TestSaveTraceRejectsInvalidPathsAndInputs(t *testing.T) {
	if _, err := SaveTrace("", ""); err == nil {
		t.Fatal("SaveTrace() accepted empty paths")
	}
	if _, err := SaveTrace(filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "trace.json")); err == nil {
		t.Fatal("SaveTrace() accepted a missing audit")
	}
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTrace(malformed, filepath.Join(t.TempDir(), "trace.json")); err == nil {
		t.Fatal("SaveTrace() accepted a malformed audit")
	}
	oversized := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversized, []byte(strings.Repeat("x", maxAuditBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTrace(oversized, filepath.Join(t.TempDir(), "trace.json")); err == nil {
		t.Fatal("SaveTrace() accepted an oversized audit")
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveTrace(writeAudit(t), filepath.Join(parentFile, "trace.json")); err == nil {
		t.Fatal("SaveTrace() accepted a file as the output parent")
	}
	if err := writeExclusive("", []byte("x")); err == nil {
		t.Fatal("writeExclusive() accepted an empty path")
	}
	directory := t.TempDir()
	if err := writeExclusive(directory, []byte("x")); err == nil {
		t.Fatal("writeExclusive() accepted a directory path")
	}
}

func TestDecodeRejectsUnsafeBrowserAudits(t *testing.T) {
	valid := string(validAudit(t))
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "unknown field", data: strings.Replace(valid, `"events"`, `"url":"https://tracker.invalid","events"`, 1)},
		{name: "not redacted", data: strings.Replace(valid, `"redacted":true`, `"redacted":false`, 1)},
		{name: "invalid scope", data: strings.Replace(valid, `"scope":"outbound"`, `"scope":"custom"`, 1)},
		{name: "invalid channel", data: strings.Replace(valid, `"channel":"network"`, `"channel":"custom"`, 1)},
		{name: "invalid kind combination", data: strings.Replace(valid, `"kind":"request"`, `"kind":"cookie-write"`, 1)},
		{name: "invalid destination", data: strings.Replace(valid, `"destination":"analytics"`, `"destination":"tracker-123"`, 1)},
		{name: "invalid field", data: strings.Replace(valid, `"fields":["region"]`, `"fields":["secret-value"]`, 1)},
		{name: "duplicate field", data: strings.Replace(valid, `"fields":["region"]`, `"fields":["region","region"]`, 1)},
		{name: "duplicate event", data: strings.Replace(valid, `}]}`, `},{"channel":"network","kind":"request","destination":"analytics","fields":["region"]}]}`, 1)},
		{name: "trailing", data: valid + "{}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.data)); err == nil {
				t.Fatal("Decode() accepted invalid audit")
			} else if strings.Contains(err.Error(), "tracker.invalid") || strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("Decode() exposed input: %v", err)
			}
		})
	}
	if _, err := Decode([]byte(strings.Repeat("x", maxAuditBytes+1))); err == nil {
		t.Fatal("Decode() accepted oversized input")
	}
	if _, err := Decode([]byte{'{', 0xff}); err == nil {
		t.Fatal("Decode() accepted invalid UTF-8")
	}
}

func TestDecodeAcceptsPartialBrowserAudit(t *testing.T) {
	data := validAudit(t)
	var audit Audit
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatal(err)
	}
	audit.Completeness = portabletrace.Partial
	data, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || decoded.Completeness != portabletrace.Partial {
		t.Fatalf("Decode() = %#v, err = %v", decoded, err)
	}
}

func validAudit(t *testing.T) []byte {
	t.Helper()
	return []byte(`{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[{"channel":"network","kind":"request","destination":"analytics","fields":["region"]}]}`)
}

func writeAudit(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := os.WriteFile(path, validAudit(t), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
