package ui

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHARComparisonHTTPBoundaries(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "private-left.har")
	right := filepath.Join(root, "private-right.har")
	rules := filepath.Join(root, "private-rules.json")
	for path, body := range map[string]string{
		left:  `{"log":{"version":"1.2","entries":[{"request":{"url":"https://private.test/?x=synthetic-secret-value"}}]}}`,
		right: `{"log":{"version":"1.2","entries":[]}}`,
		rules: `{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"synthetic-secret-value"}]}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), HARPath: left, HARSecondPath: right, HAROrigin: "https://private.test", HARTestValuesPath: rules, ExpectedHost: "127.0.0.1:8787"})
	for _, tc := range []struct {
		path, method, host string
		code               int
	}{
		{"/capture-compare", "GET", "127.0.0.1:8787", 200},
		{"/capture-compare?path=/secret&second=/secret&origin=https://evil.test", "GET", "127.0.0.1:8787", 200},
		{"/capture-compare", "POST", "127.0.0.1:8787", 405},
		{"/capture-compare", "GET", "evil.test", 421},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = tc.host
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		for _, secret := range []string{"synthetic-secret-value", "private.test", root, "evil.test"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("comparison leaked", secret)
			}
		}
		if tc.code == 200 && (w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "email")) {
			t.Fatal("missing safe comparison")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Compare these captures") {
		t.Fatal("comparison not discoverable")
	}
	if err := os.WriteFile(right, []byte(`{"private":"invalid-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/capture-compare", nil))
	if w.Code != 422 || strings.Contains(w.Body.String(), "invalid-secret") || strings.Contains(w.Body.String(), "email") {
		t.Fatal("stale or unsafe comparison", w.Body.String())
	}
}

func TestHARComparisonRequiresConfiguration(t *testing.T) {
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), ExpectedHost: "127.0.0.1:8787"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/capture-compare", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
