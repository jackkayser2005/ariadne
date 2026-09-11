package ui

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHARDownloadsAreStandaloneRedactedAndFresh(t *testing.T) {
	root := t.TempDir()
	capture := filepath.Join(root, "private.har")
	rules := filepath.Join(root, "private-rules.json")
	for path, data := range map[string]string{
		capture: `{"log":{"version":"1.2","entries":[{"request":{"url":"https://private.test/?x=synthetic-secret-value"}}]}}`,
		rules:   `{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"synthetic-secret-value"}]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	options := ReviewOptions{ArchiveRoot: t.TempDir(), HARPath: capture, HARSecondPath: capture, HAROrigin: "https://private.test", HARTestValuesPath: rules, ExpectedHost: "127.0.0.1:8787"}
	h := HandlerWithReviewOptions(options)
	for _, tc := range []struct{ path, filename string }{{"/capture-report", "ariadne-capture.html"}, {"/capture-comparison-report", "ariadne-comparison.html"}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787"+tc.path+"?path=ignored&filename=private.har", nil))
		if w.Code != 200 || w.Header().Get("Content-Disposition") != `attachment; filename="`+tc.filename+`"` || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe download headers", w.Code, w.Header())
		}
		for _, secret := range []string{"private.test", "synthetic-secret-value", capture, rules, `href="/"`, "Save explanation", "Save comparison", "<script"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("not standalone/redacted", secret)
			}
		}
		if !strings.Contains(w.Body.String(), "email") || !strings.Contains(w.Body.String(), "entry 1") {
			t.Fatal("lost evidence references")
		}
		for _, method := range []string{"POST", "PUT", "DELETE"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(method, "http://127.0.0.1:8787"+tc.path, nil))
			if w.Code != 405 {
				t.Fatal("download mutation method", w.Code)
			}
		}
		badHost := httptest.NewRecorder()
		h.ServeHTTP(badHost, httptest.NewRequest("GET", "http://evil.test"+tc.path, nil))
		if badHost.Code != 421 {
			t.Fatal("host validation", badHost.Code)
		}
	}
	if err := os.WriteFile(capture, []byte("secret-invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/capture-report", "/capture-comparison-report"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787"+path, nil))
		if w.Code != 422 || w.Header().Get("Content-Disposition") != "" || strings.Contains(w.Body.String(), "secret-invalid") {
			t.Fatal("stale or unsafe failed download", w.Code, w.Header())
		}
		empty := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), ExpectedHost: "127.0.0.1:8787"})
		w = httptest.NewRecorder()
		empty.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787"+path, nil))
		if w.Code != 404 {
			t.Fatal("unconfigured download", w.Code)
		}
	}
}
