package ui

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHARReviewBoundariesAndFreshness(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "secret.har")
	raw := `{"log":{"version":"1.2","entries":[{"request":{"url":"https://secret.test/path?email=private-value","postData":{"mimeType":"application/json","text":"{\"x\":\"private-value\"}"}}},{"request":{"url":"https://secret.test/missing","bodySize":20}}]}}`
	if err := os.WriteFile(capture, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(root, "rules.json")
	if err := os.WriteFile(rules, []byte(`{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"private-value"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: root, HARPath: capture, HAROrigin: "https://secret.test", HARTestValuesPath: rules, ExpectedHost: "127.0.0.1:8787"})
	for _, tc := range []struct {
		path, method, host string
		code               int
	}{
		{"/capture", "GET", "127.0.0.1:8787", 200},
		{"/capture?path=/private&origin=https://evil.test", "GET", "127.0.0.1:8787", 200},
		{"/capture", "POST", "127.0.0.1:8787", 405},
		{"/capture", "GET", "evil.test", 421},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = tc.host
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
		for _, value := range []string{"secret.test", "private-value", capture, "evil.test"} {
			if strings.Contains(w.Body.String(), value) {
				t.Fatal("leak", value)
			}
		}
		if tc.code == 200 && (!strings.Contains(w.Body.String(), "Website origin") || !strings.Contains(w.Body.String(), "Supplied values found in the capture") || !strings.Contains(w.Body.String(), "JSON body string") || !strings.Contains(w.Body.String(), "1 textual body/bodies inspected; 1 body entry/entries") || !strings.Contains(w.Body.String(), `href="/"`) || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("missing safe review")
		}
	}
	if err := os.WriteFile(capture, []byte(`{"secret":"<script>private-value</script>"}`), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8787/capture", nil)
	h.ServeHTTP(w, r)
	if w.Code != 422 || strings.Contains(w.Body.String(), "private-value") || strings.Contains(w.Body.String(), "Website origin") {
		t.Fatal("stale or raw report", w.Body.String())
	}
}

func TestHARReviewConfiguration(t *testing.T) {
	for _, configured := range []bool{false, true} {
		opts := ReviewOptions{ArchiveRoot: t.TempDir(), ExpectedHost: "127.0.0.1:8787"}
		if configured {
			opts.HARPath = "missing.har"
			opts.HAROrigin = "https://example.test"
		}
		h := HandlerWithReviewOptions(opts)
		for _, path := range []string{"/", "/capture"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787"+path, nil))
			if path == "/" {
				if w.Code != 200 || strings.Contains(w.Body.String(), "Explain this capture") != configured {
					t.Fatal("landing configuration")
				}
			} else {
				expected := 404
				if configured {
					expected = 422
				}
				if w.Code != expected {
					t.Fatal(w.Code)
				}
			}
		}
	}
}
