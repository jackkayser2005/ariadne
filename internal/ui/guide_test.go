package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuidePageIsReaderFirstAndGETOnly(t *testing.T) {
	h := newHandler(handler{})

	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/guide", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET /guide status = %d, want %d", get.Code, http.StatusOK)
	}
	for _, want := range []string{
		"See the information trail without reading the raw data.",
		"1 · Investigate",
		"2 · Compare",
		"3 · Trace",
		"4 · Verify",
		"Unknown means the evidence cannot answer the question yet.",
		"A category is a label, not the value.",
		"Account or device",
		"Reviewed vocabulary",
		"Account identifier",
		"user-agent",
		"A precise or approximate place.",
		"Raw captured values stay outside this guide.",
	} {
		if !strings.Contains(get.Body.String(), want) {
			t.Fatalf("GET /guide missing %q", want)
		}
	}
	for _, forbidden := range []string{"<script", "captured-secret", "private-value"} {
		if strings.Contains(get.Body.String(), forbidden) {
			t.Fatalf("GET /guide contains forbidden %q", forbidden)
		}
	}
	if got := get.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("GET /guide missing content security policy")
	}

	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/guide", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /guide status/allow = %d/%q, want %d/%q", post.Code, post.Header().Get("Allow"), http.StatusMethodNotAllowed, http.MethodGet)
	}
}

func TestGuideOnlyOffersConfiguredWeatherInvestigation(t *testing.T) {
	for _, configured := range []bool{false, true} {
		settings := handler{}
		if configured {
			settings.weatherPath = "weather-run"
		}
		response := httptest.NewRecorder()
		newHandler(settings).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/guide", nil))
		if got := strings.Contains(response.Body.String(), `href="/weather"`); got != configured {
			t.Fatalf("weather link available=%v, configured=%v", got, configured)
		}
	}
}
