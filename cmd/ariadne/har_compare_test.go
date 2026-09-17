package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompareHAR(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "private.har")
	rules := filepath.Join(root, "private-rules.json")
	output := filepath.Join(root, "report.html")
	for path, body := range map[string]string{capture: `{"log":{"version":"1.2","entries":[]}}`, rules: `{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"secret@example.test"}]}`} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"--origin", "https://example.test", "--test-values", rules, "--output", output, capture, capture}
	for _, tc := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"--bad"}, 2}, {args, 0}, {args, 1}} {
		var out, stderr bytes.Buffer
		if code := runCompareHAR(tc.args, &out, &stderr); code != tc.code {
			t.Fatalf("code=%d want=%d: %s", code, tc.code, stderr.String())
		}
		if strings.Contains(out.String()+stderr.String(), "secret@example.test") || strings.Contains(out.String()+stderr.String(), root) {
			t.Fatal("raw CLI data")
		}
	}
}
