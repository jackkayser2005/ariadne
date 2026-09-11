package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const harSample = `{"log":{"version":"1.2","entries":[{"request":{"url":"https://EXAMPLE.test:443/private?email=secret-email&latitude=secret-location","headers":[{"name":"Cookie","value":"secret-cookie"}],"postData":{"params":[{"name":"phone","value":"secret-phone"},{"name":"account_id","value":"secret-account"}]}}},{"request":{"url":"https://other.test/p?lon=secret-lon","cookies":[{"name":"secret-name","value":"secret-value"}]}},{"request":{"url":"https://example.test/again?unknown=secret-unknown"}}]}}`

func TestInspectHARRedactedGroups(t *testing.T) {
	review, err := InspectHAR([]byte(harSample), "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if review.Requests != 3 || len(review.Destinations) != 2 {
		t.Fatal(review)
	}
	first := review.Destinations[0]
	if first.Label != "Website origin" || first.Requests != 2 || first.CookieRequests != 1 || len(first.Hints) != 4 {
		t.Fatal(first)
	}
	if review.Destinations[1].Label != "Other origin 1" || review.Destinations[1].CookieRequests != 1 {
		t.Fatal(review)
	}
	encoded, _ := json.Marshal(review)
	for _, forbidden := range []string{"secret", "example.test", "other.test", "/private", "latitude", "account_id"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("retained %s", forbidden)
		}
	}
	if _, err := InspectHAR(append([]byte{0xef, 0xbb, 0xbf}, []byte(harSample)...), "https://example.test/"); err != nil {
		t.Fatal(err)
	}
	empty, err := InspectHAR([]byte(`{"log":{"version":"1.2","entries":[]}}`), "http://example.test")
	if err != nil || empty.Requests != 0 {
		t.Fatal(empty, err)
	}
}

func TestInspectHARRejectsHostileInputs(t *testing.T) {
	inputs := []string{
		"", "null", "{}", harSample + "{}", `{"secret":1,"secret":2}`,
		`{"log":{"version":"1.1","entries":[]}}`, `{"log":{"version":"1.2"}}`,
		`{"log":{"version":"1.2","entries":[{}]}}`,
		strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65),
		string([]byte{0xff}), strings.Repeat(" ", maxHARBytes+1),
	}
	for _, rawURL := range []string{"/relative", "file:///secret", "https://user:secret@example.test/", "https://example.test:99999/", "https://example.test:bad/", "https://example.test/?secret=%ZZ", "https://example.test/" + strings.Repeat("a", 8192)} {
		b, _ := json.Marshal(rawURL)
		inputs = append(inputs, `{"log":{"version":"1.2","entries":[{"request":{"url":`+string(b)+`}}]}}`)
	}
	for i, input := range inputs {
		if _, err := InspectHAR([]byte(input), "https://example.test"); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("case %d: unsafe acceptance/error %v", i, err)
		}
	}
	for _, origin := range []string{"secret", "https://example.test/private", "https://example.test/?secret=x", "https://example.test/#secret", "https://example.test/?"} {
		if _, err := InspectHAR([]byte(harSample), origin); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("origin error %v", err)
		}
	}
}

func TestInspectHARLimits(t *testing.T) {
	entry := `{"request":{"url":"https://example.test/"}}`
	entries := strings.TrimSuffix(strings.Repeat(entry+",", 10001), ",")
	if _, err := InspectHAR([]byte(`{"log":{"version":"1.2","entries":[`+entries+`]}}`), "https://example.test"); err == nil {
		t.Fatal("entry limit")
	}
	names := strings.TrimSuffix(strings.Repeat(`{"name":"email"},`, 1025), ",")
	for _, field := range []string{`"headers":[` + names + `]`, `"cookies":[` + names + `]`, `"postData":{"params":[` + names + `]}`} {
		if _, err := InspectHAR([]byte(`{"log":{"version":"1.2","entries":[{"request":{"url":"https://example.test/",`+field+`}}]}}`), "https://example.test"); err == nil {
			t.Fatal("metadata limit")
		}
	}
}

func TestSaveHARReportSafety(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "private.har")
	output := filepath.Join(root, "report.html")
	if err := os.WriteFile(input, []byte(harSample), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveHARReport(input, "https://example.test", output, ""); err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret", "example.test", "other.test", "<script", "https://"} {
		if bytes.Contains(report, []byte(forbidden)) {
			t.Fatal("leak", forbidden)
		}
	}
	for _, wanted := range []string{"not a live monitor", "Values were not inspected", "Other origin 1"} {
		if !bytes.Contains(report, []byte(wanted)) {
			t.Fatal("missing", wanted)
		}
	}
	if err := SaveHARReport(input, "https://example.test", output, ""); err == nil {
		t.Fatal("overwrote report")
	}
	after, _ := os.ReadFile(output)
	if !bytes.Equal(after, report) {
		t.Fatal("changed existing report")
	}
	for _, bad := range []string{root, filepath.Join(root, "missing-secret.har")} {
		if err := SaveHARReport(bad, "https://example.test", output, ""); err == nil || strings.Contains(err.Error(), root) {
			t.Fatal(err)
		}
	}
	os.WriteFile(input, []byte("invalid-secret"), 0600)
	if err := SaveHARReport(input, "https://example.test", filepath.Join(root, "bad.html"), ""); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
