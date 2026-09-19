package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func savedGuidedPair(t *testing.T, location string) *InvestigationHandler {
	t.Helper()
	h, err := NewInvestigationHandler(InvestigationOptions{Host: "127.0.0.1:8787", InitialURL: "https://site.invalid", OutputRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	identity := browser.NewTrialIdentity("baseline-treatment")
	h.trialIdentity = &identity
	h.confirmation = browser.TaskConfirmation{Baseline: "unknown", Treatment: "unknown"}
	for i, role := range []string{"baseline", "treatment"} {
		id := identity
		id.Role = role
		settings := &browser.TrialSettings{SchemaVersion: 1, Identity: id, StartedAt: int64(i + 1), Browser: "Chrome/153.0.0.0", Platform: "windows", Location: "unchanged", BlockOrigins: []string{}}
		if i == 1 {
			settings.Location = location
		}
		journey := trace.NewJourney()
		journey.AddGap("instrumentation-unavailable")
		h.pending = &browser.CaptureResult{Journey: journey, Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://site.invalid"}}, Steps: []browser.InvestigationStep{}, Trial: settings}
		h.currentTrial = i == 1
		if err := h.publish(); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func TestGuidedComparisonReportsRemainClaimedAndProfileStaysPrivate(t *testing.T) {
	h := savedGuidedPair(t, "deny")
	if h.comparison == nil || h.comparison.Reduction != "unknown" {
		t.Fatal("missing incomplete comparison")
	}
	for _, task := range []string{"works", "broken", "unknown"} {
		response := guidedRequest(h, "POST", "/api/functionality", `{"functionality":{"baseline":"works","treatment":"`+task+`"}}`)
		if response.Code != 200 {
			t.Fatal(response.Body.String())
		}
		if h.comparison.Functionality.Treatment != task || h.comparison.AutomaticFunctionality != minimize.CandidateUnknown {
			t.Fatal("user report became automatic proof")
		}
		profileResponse := guidedRequest(h, "GET", "/api/profile", "")
		if profileResponse.Code != 200 {
			t.Fatal(profileResponse.Body.String())
		}
		profile, err := browser.DecodeSiteProtectionProfile(profileResponse.Body.Bytes())
		if err != nil || profile.SiteOrigin != "https://site.invalid" || profile.Test.Reduction != "unknown" {
			t.Fatalf("private profile: %v", err)
		}
		if task != "unknown" && profile.Test.FunctionalityState != evidence.Claimed {
			t.Fatal("user reports were not marked claimed")
		}
		portable := guidedRequest(h, "GET", "/api/export", "")
		if portable.Code != 200 || strings.Contains(portable.Body.String(), "site.invalid") || strings.Contains(portable.Body.String(), "pair_id") {
			t.Fatal("portable export contains private context")
		}
	}
	if r := guidedRequest(h, "POST", "/api/functionality", `{"functionality":{"baseline":"observed","treatment":"works"}}`); r.Code != 422 {
		t.Fatal("forged confirmation accepted")
	}
	if r := guidedRequest(h, "POST", "/api/profile", `{}`); r.Code != 405 {
		t.Fatal("profile mutation accepted")
	}
	if err := os.WriteFile(filepath.Join(h.baselinePath, "receipt.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if r := guidedRequest(h, "GET", "/api/profile", ""); r.Code != 422 {
		t.Fatal("profile did not reverify sources")
	}
	state := guidedRequest(h, "GET", "/api/state", "")
	if h.comparison != nil || !strings.Contains(state.Body.String(), "could not be verified") {
		t.Fatal("stale comparison survived source tampering")
	}
	if r := guidedRequest(h, "POST", "/api/functionality", `{}`); r.Code != 409 {
		t.Fatal("confirmed invalid comparison")
	}
	if r := guidedRequest(h, "GET", "/api/profile", ""); r.Code != 409 {
		t.Fatal("exported invalid comparison")
	}
}

func TestGuidedComparisonCancellationRestoresBaselineAndRejectsUnsafeRetry(t *testing.T) {
	h := savedGuidedPair(t, "approximate")
	baseline := h.baselinePath
	if r := guidedRequest(h, "GET", "/api/profile", ""); r.Code != 422 || !strings.Contains(r.Body.String(), "lab-only") {
		t.Fatal("exported approximate location")
	}
	h.pending = &browser.CaptureResult{}
	if r := guidedRequest(h, "POST", "/api/cancel", `{}`); r.Code != 200 || h.savedPath != baseline || h.currentTrial || h.phase != "saved" || h.notice == "" {
		t.Fatal("cancel lost the saved baseline")
	}
	if r := guidedRequest(h, "POST", "/api/start", `{"url":"https://other.invalid","trial":true}`); r.Code != 409 {
		t.Fatal("different site accepted")
	}
	if err := os.WriteFile(filepath.Join(baseline, "receipt.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if r := guidedRequest(h, "POST", "/api/start", `{"url":"https://site.invalid","trial":true}`); r.Code != 422 {
		t.Fatal("tampered baseline accepted")
	}
	h.currentTrial = true
	h.pending = &browser.CaptureResult{}
	if r := guidedRequest(h, "POST", "/api/cancel", `{}`); r.Code != 200 || h.phase != "cancelled" || h.trialIdentity != nil {
		t.Fatal("invalid baseline restored")
	}
	if r := guidedRequest(h, "POST", "/api/start", `{"url":"https://site.invalid","trial":true}`); r.Code != 409 {
		t.Fatal("trial without baseline accepted")
	}
}

func TestGuidedBrowserPairedTrialReusesInputsAndObservesEnforcement(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("requires installed Chrome/Edge")
	}
	var received atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(204)
	}))
	defer collector.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!doctype html><title>Paired fixture</title><input autofocus><script>document.querySelector('input').oninput=e=>fetch(%q+'/collect?value='+encodeURIComponent(e.target.value)).catch(()=>{});</script>`, collector.URL)
	}))
	defer site.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	h, err := NewInvestigationHandler(InvestigationOptions{Context: ctx, Host: "127.0.0.1:8787", OutputRoot: t.TempDir(), Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	marker := ""
	for _, trial := range []bool{false, true} {
		body, _ := json.Marshal(map[string]any{"url": site.URL, "trial": trial, "location": map[bool]string{false: "unchanged", true: "deny"}[trial], "block_origins": map[bool][]string{false: {}, true: {collector.URL}}[trial]})
		if r := guidedRequest(h, "POST", "/api/start", string(body)); r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		if trial && h.markers[0].Value != marker {
			t.Fatal("trial changed synthetic inputs")
		}
		marker = h.markers[0].Value
		deadline := time.Now().Add(8 * time.Second)
		for {
			err = h.capture.FillSynthetic(ctx, "input", "m1")
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		wantKind := "response"
		if trial {
			wantKind = "blocked"
		}
		for {
			found := false
			for _, o := range h.capture.Snapshot().Journey.Observations {
				if o.Kind == wantKind && o.Destination != "d1" {
					found = true
				}
			}
			if found {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("missing %s", wantKind)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if r := guidedRequest(h, "POST", "/api/stop", `{}`); r.Code != 200 {
			t.Fatal(r.Body.String())
		}
	}
	if received.Load() != 1 || h.comparison == nil || !h.comparison.PairBound || h.comparison.Baseline.Requests != 1 || h.comparison.Treatment.Blocked != 1 || h.comparison.Reduction != "unknown" {
		t.Fatalf("paired enforcement: received %d comparison %+v", received.Load(), h.comparison)
	}
	if r := guidedRequest(h, "POST", "/api/functionality", `{"functionality":{"baseline":"works","treatment":"broken"}}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := guidedRequest(h, "GET", "/api/profile", ""); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
}
