package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWeatherReviewBoundary(t *testing.T) {
	for _, tc := range []struct {
		path, method, host, root string
		code                     int
	}{{"/weather", "GET", "127.0.0.1:8787", "", 404}, {"/weather", "GET", "127.0.0.1:8787", t.TempDir(), 500}, {"/weather", "POST", "127.0.0.1:8787", t.TempDir(), 405}, {"/weather", "GET", "evil.test", t.TempDir(), 421}} {
		h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), WeatherPath: tc.root, ExpectedHost: "127.0.0.1:8787"})
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = tc.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Errorf("%s %s: %d", tc.method, tc.host, w.Code)
		}
		if tc.root != "" && strings.Contains(w.Body.String(), tc.root) {
			t.Fatal("path leaked")
		}
	}
}
func BenchmarkReviewRequest(b *testing.B) {
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: b.TempDir(), ExpectedHost: "127.0.0.1:8787"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8787/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}
