package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func weatherFake(_ context.Context, _ string, _ []string, data []byte) ([]byte, error) {
	var r WeatherRequest
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	s := WeatherSession{SchemaVersion: 1, ProcedureID: WeatherProcedureID, Candidate: r.Candidate, ChallengeSHA256: weatherHash([]byte(r.Challenge)), BrowserSHA256: strings.Repeat("a", 64), Functionality: "available", Geolocation: "granted", Status: "complete", Gaps: []string{"server-side-unobservable"}, Observations: []WeatherObservation{}}
	if r.Candidate == "denied" {
		s.Functionality = "unavailable"
		s.Geolocation = "denied"
	} else {
		s.Observations = []WeatherObservation{{"attempted", "weather-service", "location"}, {"response-backed", "weather-service", "location"}}
	}
	return json.Marshal(s)
}
func weatherInput(t *testing.T) WeatherInput {
	t.Helper()
	return WeatherInput{DriverPath: filepath.Join(t.TempDir(), "driver"), OutputDir: filepath.Join(t.TempDir(), "run")}
}
func weatherFixture(t *testing.T) (WeatherInput, WeatherReview) {
	t.Helper()
	in := weatherInput(t)
	if err := runWeather(context.Background(), in, weatherFake); err != nil {
		t.Fatal(err)
	}
	r, err := VerifyWeather(in.OutputDir)
	if err != nil {
		t.Fatal(err)
	}
	return in, r
}

func TestWeatherTwoOrdersAndIndependentVerification(t *testing.T) {
	in, r := weatherFixture(t)
	if r.Ladder.SelectedCandidate != "coarse" || r.Ladder.EvidenceState != evidence.Observed || len(r.Run.Sessions) != 8 {
		t.Fatalf("unexpected result: %+v", r.Ladder)
	}
	copyDir := filepath.Join(t.TempDir(), "copy")
	if err := os.CopyFS(copyDir, os.DirFS(in.OutputDir)); err != nil {
		t.Fatal(err)
	}
	copied, err := VerifyWeather(copyDir)
	if err != nil || copied.ReceiptSHA256 != r.ReceiptSHA256 {
		t.Fatal("copied identity differs", err)
	}
	for _, name := range []string{"weather.json", "minimization.json", "trace-01.json"} {
		data, _ := os.ReadFile(filepath.Join(copyDir, name))
		for _, secret := range []string{"38.889", "77.035", "https://", "latitude", "longitude"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("leaked %s", secret)
			}
		}
	}
	if err := runWeather(context.Background(), in, weatherFake); err == nil {
		t.Fatal("overwrote existing output")
	}
}

func TestWeatherCandidateEvidenceIsDerived(t *testing.T) {
	_, review := weatherFixture(t)
	if len(review.CandidateEvidence) != 3 {
		t.Fatalf("candidate evidence count = %d", len(review.CandidateEvidence))
	}
	byCandidate := make(map[string]WeatherCandidateEvidence, len(review.CandidateEvidence))
	for _, item := range review.CandidateEvidence {
		byCandidate[item.Candidate] = item
	}
	for _, test := range []struct {
		candidate string
		sessions  int
	}{
		{"precise", 4},
		{"coarse", 2},
	} {
		item := byCandidate[test.candidate]
		if item.SessionCount != test.sessions || item.AvailableSessions != test.sessions || item.UnavailableSessions != 0 || item.UnknownSessions != 0 || item.AttemptedSessions != test.sessions || item.ResponseBackedSessions != test.sessions {
			t.Fatalf("%s evidence = %+v", test.candidate, item)
		}
		if len(item.VisibilityGaps) != 1 || item.VisibilityGaps[0].ID != "server-side-unobservable" {
			t.Fatalf("%s gaps = %+v", test.candidate, item.VisibilityGaps)
		}
	}
	denied := byCandidate["denied"]
	if denied.SessionCount != 2 || denied.AvailableSessions != 0 || denied.UnavailableSessions != 2 || denied.UnknownSessions != 0 || denied.AttemptedSessions != 0 || denied.ResponseBackedSessions != 0 {
		t.Fatalf("denied evidence = %+v", denied)
	}
	run := review.Run
	run.Sessions = append(append([]WeatherSession{}, run.Sessions...), WeatherSession{Candidate: "other", Functionality: "available"})
	run.Sessions[0].Functionality = "unknown"
	derived := weatherCandidateEvidence(run)
	if derived[0].UnknownSessions != 1 || derived[0].AvailableSessions != 3 {
		t.Fatalf("unknown functionality count = %+v", derived[0])
	}
	raw, err := json.Marshal(review.CandidateEvidence)
	if err != nil || strings.Contains(string(raw), "38.889") || strings.Contains(string(raw), "https://") {
		t.Fatalf("unsafe candidate evidence: %s", raw)
	}
}

func TestWeatherGapEvidenceUsesFixedReasons(t *testing.T) {
	for _, test := range []struct {
		id     string
		reason string
	}{
		{"blocked-origin", "A destination outside the reviewed origin allowlist was blocked."},
		{"unsupported-body", "A request body used an encoding outside the bounded parser."},
		{"unsupported-channel", "A worker, service worker, shared worker, or WebSocket channel was outside this capture."},
		{"capture-incomplete", "The capture did not provide complete visibility for supported browser channels."},
		{"server-side-unobservable", "The browser cannot observe onward handling after a response."},
	} {
		got := weatherGapEvidence(test.id)
		if got.ID != test.id || got.Reason != test.reason {
			t.Fatalf("%s = %+v", test.id, got)
		}
	}
	if got := weatherGapEvidence("future-gap").Reason; got != "A reviewed visibility limit was recorded." {
		t.Fatal("unsafe fallback:", got)
	}
}
func TestWeatherFailuresDoNotPublish(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*WeatherSession)
		raw    []byte
		fail   bool
	}{
		{name: "driver", fail: true}, {name: "JSON", raw: []byte(`{"private":"secret"}`)},
		{name: "candidate", mutate: func(s *WeatherSession) { s.Candidate = "denied" }},
		{name: "challenge", mutate: func(s *WeatherSession) { s.ChallengeSHA256 = strings.Repeat("b", 64) }},
		{name: "rate", mutate: func(s *WeatherSession) { s.Status = "rate-limited"; s.Functionality = "unknown" }},
		{name: "browser", mutate: func(s *WeatherSession) {
			if s.Candidate == "coarse" {
				s.BrowserSHA256 = strings.Repeat("b", 64)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := weatherInput(t)
			calls := 0
			err := runWeather(context.Background(), in, func(c context.Context, p string, a []string, d []byte) ([]byte, error) {
				calls++
				if tc.fail {
					return nil, errors.New("secret")
				}
				if tc.raw != nil {
					return tc.raw, nil
				}
				raw, _ := weatherFake(c, p, a, d)
				var s WeatherSession
				json.Unmarshal(raw, &s)
				tc.mutate(&s)
				return json.Marshal(s)
			})
			if err == nil {
				t.Fatal("accepted failure")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("leaked diagnostic")
			}
			if _, err := os.Stat(in.OutputDir); !os.IsNotExist(err) {
				t.Fatal("published failed output")
			}
			if tc.name == "rate" && calls != 1 {
				t.Fatal("continued after rate limit")
			}
		})
	}
	in := weatherInput(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if runWeather(ctx, in, weatherFake) == nil {
		t.Fatal("ignored cancellation")
	}
	if runWeather(nil, in, weatherFake) == nil || runWeather(context.Background(), in, nil) == nil {
		t.Fatal("accepted missing dependencies")
	}
	in.DriverPath = "relative"
	if runWeather(context.Background(), in, weatherFake) == nil {
		t.Fatal("relative driver")
	}
	in = weatherInput(t)
	os.WriteFile(filepath.Dir(in.OutputDir)+"-file", nil, 0600)
	in.OutputDir = filepath.Join(filepath.Dir(in.OutputDir)+"-file", "run")
	if runWeather(context.Background(), in, weatherFake) == nil {
		t.Fatal("file parent")
	}
}

func TestWeatherMalformedArtifacts(t *testing.T) {
	for _, data := range [][]byte{nil, []byte{0xff}, []byte(strings.Repeat("x", weatherLimit+1)), []byte(`{"a":1,"a":2}`), []byte(`{} {}`), []byte(`{"private":"payload"}`)} {
		var s WeatherSession
		if decodeWeather(data, &s) == nil {
			t.Fatalf("accepted malformed %q", string(data[:min(len(data), 32)]))
		}
	}
	_, r := weatherFixture(t)
	valid := r.Run.Sessions[0]
	mutations := []func(*WeatherSession){
		func(s *WeatherSession) { s.SchemaVersion = 2 }, func(s *WeatherSession) { s.Candidate = "other" }, func(s *WeatherSession) { s.Functionality = "other" }, func(s *WeatherSession) { s.Geolocation = "other" }, func(s *WeatherSession) { s.Status = "other" }, func(s *WeatherSession) { s.Status = "workflow-failed" }, func(s *WeatherSession) { s.Gaps = nil }, func(s *WeatherSession) { s.Gaps = []string{"secret"} }, func(s *WeatherSession) { s.Gaps = []string{"blocked-origin", "blocked-origin"} }, func(s *WeatherSession) {
			s.Observations = []WeatherObservation{{"response-backed", "undeclared", "location"}}
		}, func(s *WeatherSession) { s.Observations = append(s.Observations, s.Observations[0]) }, func(s *WeatherSession) { s.Candidate = "denied" },
	}
	for i, mutate := range mutations {
		s := valid
		s.Observations = append([]WeatherObservation{}, valid.Observations...)
		mutate(&s)
		if validateWeatherSession(s) == nil {
			t.Errorf("accepted mutation %d", i)
		}
	}
	for _, mutate := range []func(*WeatherRun){func(r *WeatherRun) { r.Sessions = r.Sessions[:7] }, func(r *WeatherRun) { r.Sessions[1].Candidate = "precise" }, func(r *WeatherRun) { r.Sessions[1].ChallengeSHA256 = r.Sessions[0].ChallengeSHA256 }, func(r *WeatherRun) { r.Sessions[0].SchemaVersion = 9 }} {
		run := r.Run
		run.Sessions = append([]WeatherSession{}, r.Run.Sessions...)
		mutate(&run)
		if _, err := summarizeWeather(run); err == nil {
			t.Error("accepted malformed run")
		}
	}
}

func TestWeatherUnknownAndMixedWithholdSelection(t *testing.T) {
	_, review := weatherFixture(t)
	for _, g := range []string{"blocked-origin", "unsupported-body", "unsupported-channel", "capture-incomplete"} {
		r := review.Run
		r.Sessions = append([]WeatherSession{}, review.Run.Sessions...)
		r.Sessions[1].Gaps = []string{g}
		s, err := summarizeWeather(r)
		if err != nil || s.SelectionState != minimize.SelectionUnknown || s.SelectedCandidate != "" {
			t.Fatal(g, s, err)
		}
	}
	r := review.Run
	r.Sessions = append([]WeatherSession{}, review.Run.Sessions...)
	r.Sessions[1].Functionality = "unavailable"
	s, err := summarizeWeather(r)
	if err != nil || s.SelectionState != minimize.SelectionUnknown {
		t.Fatal("mixed", err, s)
	}
	r.Sessions[2].Functionality = "unavailable"
	s, err = summarizeWeather(r)
	if err != nil || s.SelectionState != minimize.SelectionNoSufficient {
		t.Fatal("insufficient", err, s)
	}
	r.Sessions[0].Status = "workflow-failed"
	r.Sessions[0].Functionality = "unknown"
	s, err = summarizeWeather(r)
	if err != nil || s.SelectionState != minimize.SelectionUnknown {
		t.Fatal("workflow failure", err, s)
	}
}

func TestWeatherTamperingFailsClosed(t *testing.T) {
	for _, name := range []string{"weather.json", "minimization.json", "trace-01.json"} {
		t.Run(name, func(t *testing.T) {
			in, _ := weatherFixture(t)
			p := filepath.Join(in.OutputDir, name)
			data, _ := os.ReadFile(p)
			os.WriteFile(p, []byte(`{}`), 0600)
			if _, err := VerifyWeather(in.OutputDir); err == nil {
				t.Fatal("accepted damaged artifact")
			}
			os.WriteFile(p, data, 0600)
			os.Remove(p)
			if _, err := VerifyWeather(in.OutputDir); err == nil {
				t.Fatal("accepted missing artifact")
			}
		})
	}
	in, r := weatherFixture(t)
	r.Run.TraceSHA256[0] = strings.Repeat("0", 64)
	data, _ := json.Marshal(r.Run)
	os.WriteFile(filepath.Join(in.OutputDir, "weather.json"), data, 0600)
	if _, err := VerifyWeather(in.OutputDir); err == nil {
		t.Fatal("accepted mismatched digest")
	}
	in, r = weatherFixture(t)
	r.Ladder.SelectedCandidate = "denied"
	data, _ = json.Marshal(r.Ladder)
	os.WriteFile(filepath.Join(in.OutputDir, "minimization.json"), data, 0600)
	if _, err := VerifyWeather(in.OutputDir); err == nil {
		t.Fatal("accepted changed selection")
	}
}

func BenchmarkWeatherVerify(b *testing.B) {
	// Fixed eight-session near-boundary artifact verification is measured without live traffic.
	root := b.TempDir()
	run := WeatherRun{SchemaVersion: 1, ProcedureID: WeatherProcedureID, Sessions: []WeatherSession{}, TraceSHA256: []string{}}
	for i, c := range weatherCandidates() {
		raw, _ := weatherFake(context.Background(), "", nil, mustWeatherJSON(WeatherRequest{1, WeatherProcedureID, c, fmt.Sprintf("%064d", i), 30000}))
		var s WeatherSession
		json.Unmarshal(raw, &s)
		doc := weatherTrace(s)
		data, _ := json.Marshal(doc)
		writeExclusive(filepath.Join(root, fmt.Sprintf("trace-%02d.json", i+1)), data)
		digest, _ := trace.SHA256(doc)
		run.Sessions = append(run.Sessions, s)
		run.TraceSHA256 = append(run.TraceSHA256, digest)
	}
	writeExclusive(filepath.Join(root, "weather.json"), mustWeatherJSON(run))
	ladder, _ := summarizeWeather(run)
	minimize.SaveLadder(root, ladder)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := VerifyWeather(root); err != nil {
			b.Fatal(err)
		}
	}
}
func mustWeatherJSON(v any) []byte { data, _ := json.Marshal(v); return data }
