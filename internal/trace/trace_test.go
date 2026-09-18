package trace

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestDecodeNormalizesTrace(t *testing.T) {
	document, err := Decode([]byte(`{
		"schema_version":1,
		"redacted":true,
		"scope":"outbound",
		"completeness":"complete",
		"events":[
			{"source":"browser","channel":"network","kind":"request","destination":"analytics","fields":["region","device-id"]},
			{"source":"android","channel":"cookie","kind":"cookie-write","destination":"advertising","fields":["consent"]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if document.Events[0].Source != "android" || !slices.Equal(document.Events[1].Fields, []string{"device-id", "region"}) {
		t.Fatalf("Decode() did not normalize events: %#v", document)
	}
	if document.Redacted != true || document.Completeness != Complete {
		t.Fatalf("Decode() = %#v", document)
	}
}

func TestVerifyAndSHA256(t *testing.T) {
	path := writeTrace(t, completeTrace())
	summary, err := Verify(path)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || !summary.Redacted || summary.Scope != "outbound" || summary.Completeness != Complete || summary.Events != 3 || len(summary.TraceSHA256) != 64 {
		t.Fatalf("Verify() = %#v", summary)
	}

	first, err := SHA256(completeTrace())
	if err != nil {
		t.Fatal(err)
	}
	second, err := SHA256(Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{
			{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}},
			{Source: "android", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"device-id", "region"}},
			{Source: "desktop", Channel: "cookie", Kind: "cookie-write", Destination: "advertising", Fields: []string{"consent"}},
		},
	})
	if err != nil || first != second {
		t.Fatalf("canonical SHA-256 mismatch: %q %q %v", first, second, err)
	}
}

func TestCompareCompleteTraces(t *testing.T) {
	baseline := completeTrace()
	treatment := Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{
			{Source: "android", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"device-id", "region"}},
			{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region", "account-id"}},
			{Source: "proxy", Channel: "cookie", Kind: "cookie-write", Destination: "advertising", Fields: []string{"consent"}},
		},
	}
	comparison, err := Compare(baseline, treatment)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.SchemaVersion != 1 || comparison.Scope != "outbound" || len(comparison.Unchanged) != 1 || len(comparison.Differences) != 3 || len(comparison.Unknowns) != 0 {
		t.Fatalf("Compare() = %#v", comparison)
	}
	if comparison.Differences[0].KindOfChange != "changed" || comparison.Differences[1].KindOfChange != "removed" || comparison.Differences[2].KindOfChange != "added" {
		t.Fatalf("changes = %#v", comparison.Differences)
	}
	for _, difference := range comparison.Differences {
		if difference.State != evidence.Observed {
			t.Fatalf("difference state = %#v", difference)
		}
	}
}

func TestComparePartialCoverageReturnsUnknownForAbsence(t *testing.T) {
	baseline := completeTrace()
	treatment := Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Partial,
		Events: []Event{
			{Source: "android", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"device-id", "region"}},
			{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}},
			{Source: "proxy", Channel: "network", Kind: "request", Destination: "advertising", Fields: []string{"consent"}},
		},
	}
	comparison, err := Compare(baseline, treatment)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Differences) != 0 || len(comparison.Unknowns) != 2 || comparison.Unknowns[0].State != evidence.Unknown || comparison.Unknowns[1].State != evidence.Unknown {
		t.Fatalf("Compare() = %#v", comparison)
	}
	if !strings.Contains(comparison.Unknowns[0].Reason, "partial trace coverage") {
		t.Fatalf("unknown reason = %q", comparison.Unknowns[0].Reason)
	}
}

func TestComparePartialCoverageReturnsUnknownForFieldChange(t *testing.T) {
	baseline := Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Partial,
		Events: []Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	}
	treatment := baseline
	treatment.Events = []Event{{
		Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id", "region"},
	}}
	comparison, err := Compare(baseline, treatment)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Differences) != 0 || len(comparison.Unknowns) != 1 || comparison.Unknowns[0].Reason != "partial trace coverage does not establish field-set change" {
		t.Fatalf("Compare() = %#v", comparison)
	}
}

func TestReadAndCompareErrorPaths(t *testing.T) {
	missing := t.TempDir() + "\\missing.json"
	if _, err := Read(missing); err == nil || !strings.Contains(err.Error(), "read trace") {
		t.Fatalf("Read() error = %v", err)
	}
	if _, err := Verify(missing); err == nil || !strings.Contains(err.Error(), "read trace") {
		t.Fatalf("Verify() error = %v", err)
	}
	validPath := writeTrace(t, completeTrace())
	if _, err := CompareFiles(validPath, missing); err == nil || !strings.Contains(err.Error(), "treatment trace") {
		t.Fatalf("CompareFiles() treatment error = %v", err)
	}
	if _, err := Compare(Document{}, completeTrace()); err == nil || !strings.Contains(err.Error(), "baseline trace") {
		t.Fatalf("Compare() baseline error = %v", err)
	}
	if _, err := Compare(completeTrace(), Document{}); err == nil || !strings.Contains(err.Error(), "treatment trace") {
		t.Fatalf("Compare() treatment error = %v", err)
	}
}

func TestCompareRejectsDifferentScopesAndFiles(t *testing.T) {
	otherScope := completeTrace()
	otherScope.Scope = "storage"
	if _, err := Compare(completeTrace(), otherScope); err == nil || !strings.Contains(err.Error(), "scopes disagree") {
		t.Fatalf("Compare() error = %v", err)
	}
	missing := t.TempDir() + "\\missing.json"
	if _, err := CompareFiles(missing, missing); err == nil || !strings.Contains(err.Error(), "baseline trace") {
		t.Fatalf("CompareFiles() error = %v", err)
	}
	baselinePath := writeTrace(t, completeTrace())
	treatmentPath := writeTrace(t, otherScope)
	if _, err := CompareFiles(baselinePath, treatmentPath); err == nil || !strings.Contains(err.Error(), "scopes disagree") {
		t.Fatalf("CompareFiles() scope error = %v", err)
	}
}

func TestDecodeRejectsInvalidDocuments(t *testing.T) {
	valid := string(mustJSON(completeTrace()))
	tests := []struct {
		name string
		data string
		want string
	}{
		{"empty", "", "empty"},
		{"non-object", "[]", "JSON object"},
		{"schema", strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1), "schema_version"},
		{"not-redacted", strings.Replace(valid, `"redacted":true`, `"redacted":false`, 1), "marked redacted"},
		{"scope", strings.Replace(valid, `"scope":"outbound"`, `"scope":"https://tracker.example"`, 1), "scope is invalid"},
		{"completeness", strings.Replace(valid, `"completeness":"complete"`, `"completeness":"unknown"`, 1), "completeness is invalid"},
		{"unknown-field", strings.Replace(valid, `"redacted":true`, `"payload":"secret-value","redacted":true`, 1), "invalid JSON fields"},
		{"duplicate-key", strings.Replace(valid, `"scope":"outbound"`, `"scope":"outbound","scope":"other"`, 1), "invalid JSON structure"},
		{"trailing", valid + "{}", "trailing data"},
		{"event-token", strings.Replace(valid, `"source":"android"`, `"source":"android/app"`, 1), "event identity"},
		{"empty-token", strings.Replace(valid, `"source":"android"`, `"source":""`, 1), "event identity"},
		{"long-token", strings.Replace(valid, `"source":"android"`, `"source":"`+strings.Repeat("a", 129)+`"`, 1), "event identity"},
		{"empty-fields", strings.Replace(valid, `"fields":["device-id","region"]`, `"fields":[]`, 1), "event fields"},
		{"duplicate-fields", strings.Replace(valid, `"fields":["device-id","region"]`, `"fields":["region","region"]`, 1), "duplicates"},
		{"duplicate-events", duplicateEventTrace(), "duplicate identities"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Decode() error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "tracker.example") {
				t.Fatalf("Decode() exposed input value: %v", err)
			}
		})
	}
}

func TestDecodeRejectsEachUnsafeCatalogValue(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Document)
	}{
		{"source", func(document *Document) { document.Events[0].Source = "device-123" }},
		{"channel", func(document *Document) { document.Events[0].Channel = "unknown-channel" }},
		{"kind", func(document *Document) { document.Events[0].Kind = "custom-event" }},
		{"destination", func(document *Document) { document.Events[0].Destination = "tracker-123" }},
		{"field", func(document *Document) { document.Events[0].Fields = []string{"custom-field"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := completeTrace()
			test.mutate(&document)
			_, err := Decode(mustJSON(document))
			if err == nil {
				t.Fatal("Decode() accepted unsafe catalog value")
			}
		})
	}
}

func TestDecodeRejectsBoundsAndInvalidSHA(t *testing.T) {
	if _, err := Decode([]byte(strings.Repeat("x", maxDocumentBytes+1))); err == nil {
		t.Fatal("Decode() accepted oversized input")
	}
	invalidUTF8 := append([]byte(`{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[]}`), 0xff)
	if _, err := Decode(invalidUTF8); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("Decode() invalid UTF-8 error = %v", err)
	}
	if _, err := SHA256(Document{}); err == nil {
		t.Fatal("SHA256() accepted invalid document")
	}
	if ValidSHA256(strings.Repeat("A", 64)) {
		t.Fatal("ValidSHA256() accepted uppercase digest")
	}
}

func completeTrace() Document {
	return Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{
			{Source: "android", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"device-id", "region"}},
			{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}},
			{Source: "desktop", Channel: "cookie", Kind: "cookie-write", Destination: "advertising", Fields: []string{"consent"}},
		},
	}
}

func duplicateEventTrace() string {
	return `{"schema_version":1,"redacted":true,"scope":"outbound","completeness":"complete","events":[{"source":"android","channel":"network","kind":"request","destination":"analytics","fields":["region"]},{"source":"android","channel":"network","kind":"request","destination":"analytics","fields":["device-id"]}]}`
}

func writeTrace(t *testing.T, document Document) string {
	t.Helper()
	path := t.TempDir() + "\\trace.json"
	if err := os.WriteFile(path, mustJSON(document), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
