package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

// This fixture is generated from synthetic labels only, never captured data.
func weatherUIFixture(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	hash := func(v any) string {
		data, _ := json.Marshal(v)
		d := sha256.Sum256(data)
		return hex.EncodeToString(d[:])
	}
	run := browser.WeatherRun{SchemaVersion: 1, ProcedureID: browser.WeatherProcedureID, Sessions: []browser.WeatherSession{}, TraceSHA256: []string{}}
	for i, c := range []string{"precise", "coarse", "coarse", "precise", "precise", "denied", "denied", "precise"} {
		s := browser.WeatherSession{SchemaVersion: 1, ProcedureID: browser.WeatherProcedureID, Candidate: c, ChallengeSHA256: fmt.Sprintf("%064d", i), BrowserSHA256: strings.Repeat("a", 64), Functionality: "available", Geolocation: "granted", Status: "complete", Gaps: []string{"server-side-unobservable"}, Observations: []browser.WeatherObservation{}}
		if c == "denied" {
			s.Functionality = "unavailable"
			s.Geolocation = "denied"
		}
		run.Sessions = append(run.Sessions, s)
		doc := trace.Document{SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: trace.Partial, Events: []trace.Event{}}
		digest, _ := trace.SHA256(doc)
		run.TraceSHA256 = append(run.TraceSHA256, digest)
		data, _ := json.Marshal(doc)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("trace-%02d.json", i+1)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	results := []minimize.LadderCandidateResult{}
	for i, c := range []string{"coarse", "denied"} {
		r := minimize.LadderCandidateResult{ID: c, Directory: minimize.LadderCandidateDirectory(i, c), ReceiptSHA256: hash(run.Sessions[i*4 : i*4+4]), Pairs: 2, PairsPerOrder: 1, CompletedPairs: 2, EvidenceState: evidence.Observed}
		if c == "coarse" {
			r.NoChangePairs = 2
			r.Outcome = trace.NoChangeObserved
		} else {
			r.ChangedPairs = 2
			r.Outcome = trace.ReplicatedChange
		}
		results = append(results, r)
	}
	plan := minimize.LadderPlan{SchemaVersion: 1, Name: "weather-location", Variable: "location", ReferenceCandidate: "precise", FunctionalityCriterion: "local-forecast-available-v1", Candidates: []string{"precise", "coarse", "denied"}}
	summary, err := minimize.SummarizeLadder(plan, minimize.LadderProvenance{Adapter: browser.WeatherProcedureID, AdapterVersion: 1, ProcedureSHA256: hash(browser.WeatherProcedureID), Scope: "outbound", ResetPolicy: browser.BrowserReplicationResetPolicy}, 1, results)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(run)
	if err := os.WriteFile(filepath.Join(root, "weather.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := minimize.SaveLadder(root, summary); err != nil {
		t.Fatal(err)
	}
	return root
}
func TestWeatherReviewReverifiesAndRenders(t *testing.T) {
	root := weatherUIFixture(t)
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), WeatherPath: root, ExpectedHost: "127.0.0.1:8787"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/weather", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, s := range []string{"Where did my information go?", "coarse", "denied", "Observed session outcomes", "Forecast available", "Reverified on every request", "server-side-unobservable", "The browser cannot observe onward handling after a response."} {
		if !strings.Contains(w.Body.String(), s) {
			t.Fatal("missing", s)
		}
	}
	if strings.Contains(w.Body.String(), root) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe projection or cache")
	}
	if err := os.WriteFile(filepath.Join(root, "weather.json"), []byte(`{"private":"<script>secret</script>"}`), 0600); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/weather", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatal("stale or unsafe output")
	}
}
func BenchmarkWeatherReviewRequest(b *testing.B) {
	root := weatherUIFixture(b)
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: b.TempDir(), WeatherPath: root, ExpectedHost: "127.0.0.1:8787"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8787/weather", nil))
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}
func TestWeatherExplanationDoesNotInventDisclosure(t *testing.T) {
	for _, observed := range []bool{false, true} {
		count := 0
		if observed {
			count = 2
		}
		view := browser.WeatherReview{CandidateEvidence: []browser.WeatherCandidateEvidence{
			{Candidate: "precise", SessionCount: 4, AvailableSessions: 4, ResponseBackedSessions: count},
			{Candidate: "coarse", SessionCount: 2, AvailableSessions: 2},
			{Candidate: "denied", SessionCount: 2, UnavailableSessions: 2},
		}}
		var out strings.Builder
		if err := weatherTemplate.Execute(&out, view); err != nil {
			t.Fatal(err)
		}
		text := out.String()
		if strings.Contains(text, "<h2 id=\"observed-title\">Location was sent") != observed {
			t.Fatal("disclosure headline disagrees with observations")
		}
		if !strings.Contains(text, "City-level location produced a forecast every time it was tested.") {
			t.Fatal("missing bounded functionality explanation")
		}
		if strings.Contains(text, "<details open") || !strings.Contains(text, "Explore the technical evidence") {
			t.Fatal("technical evidence must be collapsed by default")
		}
		if !strings.Contains(text, "not everything on your device") || !strings.Contains(text, "not your real location") {
			t.Fatal("missing test scope")
		}
	}
}
