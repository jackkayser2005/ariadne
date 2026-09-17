package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunHAR(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "input.har")
	output := filepath.Join(root, "report.html")
	if err := os.WriteFile(input, []byte(`{"log":{"version":"1.2","entries":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		code int
	}{
		{nil, 2}, {[]string{"--bad"}, 2},
		{[]string{"--origin", "https://example.test", "--output", output, input}, 0},
		{[]string{"--origin", "https://example.test", "--output", output, input}, 1},
	} {
		var out, stderr bytes.Buffer
		if code := runHAR(tc.args, &out, &stderr); code != tc.code {
			t.Fatalf("%v: %d %s", tc.args, code, stderr.String())
		}
	}
}
