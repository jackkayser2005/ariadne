package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func guidedRequest(h *InvestigationHandler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+h.options.Host+path, strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:45000"
	r.Header.Set("Authorization", "Bearer "+h.token)
	r.Header.Set("Origin", "http://"+h.options.Host)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestGuidedRetainsStoppedEvidenceWhenPublicationFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "output")
	if err := os.WriteFile(root, []byte("occupied by a file"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := NewInvestigationHandler(InvestigationOptions{Host: "127.0.0.1:8787", OutputRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	result := browser.CaptureResult{Journey: trace.NewJourney(), Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://private.invalid"}}, Steps: []browser.InvestigationStep{}}
	h.pending = &result
	h.phase = "unsaved"
	bundle, err := browser.BuildJourneyBundle(result.Journey)
	if err != nil {
		t.Fatal(err)
	}
	h.bundle = &bundle
	if response := guidedRequest(h, "POST", "/api/save", `{}`); response.Code != 422 || h.pending == nil {
		t.Fatal("failed save discarded the recording")
	}
	if response := guidedRequest(h, "POST", "/api/start", `{"url":"https://private.invalid"}`); response.Code != 409 {
		t.Fatal("new recording replaced unsaved evidence")
	}
	response := guidedRequest(h, "GET", "/api/export", "")
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if _, err := browser.DecodeJourneyBundle(response.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response.Body.String(), "private.invalid") {
		t.Fatal("fallback export leaked private names")
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if response := guidedRequest(h, "POST", "/api/save", `{}`); response.Code != 200 || h.phase != "saved" || h.pending != nil {
		t.Fatal("could not retry retained recording")
	}
	if _, _, err := browser.ReadInvestigation(h.savedPath); err != nil {
		t.Fatal(err)
	}
	if response := guidedRequest(h, "POST", "/api/save", `{}`); response.Code != 409 {
		t.Fatal("repeated save unexpectedly replaced evidence")
	}
	h.pending = &result
	h.phase = "unsaved"
	if response := guidedRequest(h, "POST", "/api/cancel", `{}`); response.Code != 200 || h.pending != nil || h.bundle != nil || h.savedPath != "" {
		t.Fatal("could not explicitly discard retained recording")
	}
}

func TestGuidedBrowserRecordingSurvivesOutputBecomingUnavailable(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("requires installed Chrome/Edge")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<title>Local save failure fixture</title>")
	}))
	defer site.Close()
	root := filepath.Join(t.TempDir(), "output")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	h, err := NewInvestigationHandler(InvestigationOptions{Host: "127.0.0.1:8787", OutputRoot: root, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	body, _ := json.Marshal(map[string]string{"url": site.URL})
	if response := guidedRequest(h, "POST", "/api/start", string(body)); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("output became unavailable"), 0600); err != nil {
		t.Fatal(err)
	}
	if response := guidedRequest(h, "POST", "/api/stop", `{}`); response.Code != 422 || h.capture != nil || h.pending == nil || h.phase != "unsaved" {
		t.Fatalf("stopped recording was not retained: %d %s", response.Code, response.Body.String())
	}
	if response := guidedRequest(h, "GET", "/api/export", ""); response.Code != 200 {
		t.Fatal("retained recording could not be exported")
	}
	if response := guidedRequest(h, "POST", "/api/cancel", `{}`); response.Code != 200 {
		t.Fatal("retained recording could not be discarded")
	}
}

func TestGuidedCaptureUsesSeparateLoopbackAuthenticationAndOrigins(t *testing.T) {
	h, err := NewInvestigationHandler(InvestigationOptions{Host: "127.0.0.1:8787", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	u, _ := url.Parse(h.URL())
	if len(u.Fragment) < 40 || u.RawQuery != "" {
		t.Fatal("unsafe session bootstrap")
	}
	for _, edit := range []func(*http.Request){
		func(r *http.Request) { r.Host = "attacker.invalid:8787" }, func(r *http.Request) { r.RemoteAddr = "192.0.2.1:3000" },
		func(r *http.Request) { r.RemoteAddr = "invalid" }, func(r *http.Request) { r.Header.Del("Authorization") },
		func(r *http.Request) { r.Header.Set("Origin", "https://attacker.invalid") }, func(r *http.Request) { r.Header.Del("Origin") },
	} {
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/api/start", strings.NewReader(`{"url":"https://example.invalid"}`))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", "http://127.0.0.1:8787")
		r.Header.Set("Authorization", "Bearer "+h.token)
		r.Header.Set("Content-Type", "application/json")
		edit(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code < 400 {
			t.Fatal("unsafe request accepted")
		}
	}
	for _, path := range []string{"/", "/investigate.js", "/api/state"} {
		w := guidedRequest(h, "GET", path, "")
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, test := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/", `{}`, 405}, {"GET", "/missing", "", 404}, {"POST", "/api/state", `{}`, 405},
		{"GET", "/api/start", "", 405}, {"GET", "/api/export", "", 409}, {"POST", "/api/stop", `{}`, 409},
		{"POST", "/api/cancel", `{}`, 409}, {"POST", "/api/fill", `{}`, 409}, {"POST", "/api/unknown", `{}`, 404},
		{"POST", "/api/start", `{"url":"file:///secret"}`, 422}, {"POST", "/api/start", `{"script":"evil"}`, 400},
		{"POST", "/api/start", `{"url":"x","url":"y"}`, 400}, {"POST", "/api/start", `{} {}`, 400},
		{"POST", "/api/start", strings.Repeat("x", 16385), 400},
	} {
		if w := guidedRequest(h, test.method, test.path, test.body); w.Code != test.status {
			t.Errorf("%s %s: %d, want %d", test.method, test.path, w.Code, test.status)
		}
	}
	h.options.OutputRoot = ""
	if w := guidedRequest(h, "POST", "/api/start", `{}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	for _, options := range []InvestigationOptions{{Host: "0.0.0.0:8787"}, {Host: "localhost:8787"}, {Host: "127.0.0.1:8787", InitialURL: "invalid"}, {Host: "127.0.0.1:8787", SavedPath: filepath.Join(t.TempDir(), "missing")}} {
		if handler, err := NewInvestigationHandler(options); err == nil {
			handler.Close()
			t.Fatal("invalid options accepted")
		}
	}
}

func TestGuidedInspectIsReadOnlyAndReverifiesRedactedExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved")
	_, err := browser.SaveInvestigation(path, browser.CaptureResult{Journey: trace.NewJourney(), Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://private.invalid"}}, Steps: []browser.InvestigationStep{}})
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewInvestigationHandler(InvestigationOptions{Context: context.Background(), Host: "127.0.0.1:8787", SavedPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if w := guidedRequest(h, "POST", "/api/start", `{}`); w.Code != 405 {
		t.Fatal("review accepted mutation")
	}
	state := guidedRequest(h, "GET", "/api/state", "")
	if state.Code != 200 || !strings.Contains(state.Body.String(), "private.invalid") {
		t.Fatal("private local names unavailable")
	}
	export := guidedRequest(h, "GET", "/api/export", "")
	if export.Code != 200 || strings.Contains(export.Body.String(), "private.invalid") || !strings.Contains(export.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("export leaked private context")
	}
	if _, err := browser.DecodeJourneyBundle(export.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "journey.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if w := guidedRequest(h, "GET", "/api/export", ""); w.Code != 422 {
		t.Fatal("tampered export accepted")
	}
}

func TestGuidedFreshBrowserCanFillSaveExportCancelAndClose(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("requires installed Chrome/Edge")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><title>Guided fixture</title><label>Email <input autofocus></label>`)
	}))
	defer site.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	root := t.TempDir()
	h, err := NewInvestigationHandler(InvestigationOptions{Context: ctx, Host: "127.0.0.1:8787", OutputRoot: root, InitialURL: site.URL, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	body, _ := json.Marshal(map[string]any{"url": site.URL, "location": "deny"})
	if w := guidedRequest(h, "POST", "/api/start", string(body)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := guidedRequest(h, "POST", "/api/start", string(body)); w.Code != 409 {
		t.Fatal("simultaneous recording accepted")
	}
	if w := guidedRequest(h, "POST", "/api/fill", `{"marker":"m16"}`); w.Code != 422 {
		t.Fatal("undefined marker accepted")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		w := guidedRequest(h, "POST", "/api/fill", `{"marker":"m1"}`)
		if w.Code == 200 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(w.Body.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	state := guidedRequest(h, "GET", "/api/state", "")
	if state.Code != 200 || !strings.Contains(state.Body.String(), `"phase":"recording"`) {
		t.Fatal("recording state missing")
	}
	if w := guidedRequest(h, "POST", "/api/stop", `{}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if h.bundle == nil || h.phase != "saved" || h.savedPath == "" {
		t.Fatal("capture was not saved")
	}
	export := guidedRequest(h, "GET", "/api/export", "")
	if export.Code != 200 {
		t.Fatal(export.Body.String())
	}
	if _, err := browser.DecodeJourneyBundle(export.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(export.Body.String(), site.URL) || strings.Contains(export.Body.String(), h.markers[0].Value) {
		t.Fatal("portable export leaked private input")
	}
	priorMarker := h.markers[0].Value
	if w := guidedRequest(h, "POST", "/api/start", string(body)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := guidedRequest(h, "POST", "/api/cancel", `{}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 1 || h.bundle != nil || h.phase != "cancelled" || h.markers[0].Value == priorMarker {
		t.Fatal("cancel did not discard and reset the investigation", err)
	}
	if w := guidedRequest(h, "POST", "/api/start", string(body)); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if h.capture != nil {
		t.Fatal("capture retained after server close")
	}
}
