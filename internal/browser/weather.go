package browser

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

// WeatherProcedureID identifies the fixed synthetic Washington weather workflow.
const WeatherProcedureID = "browser-weather-location-v1"
const weatherCriterion = "local-forecast-available-v1"
const weatherLimit = 256 << 10

// WeatherRequest binds one isolated session to a one-shot runner challenge.
// Origins, selectors, and synthetic coordinates are fixed in the reviewed driver.
type WeatherRequest struct {
	SchemaVersion int    `json:"schema_version"`
	ProcedureID   string `json:"procedure_id"`
	Candidate     string `json:"candidate"`
	Challenge     string `json:"challenge"`
	DurationMS    int    `json:"duration_ms"`
}

// WeatherObservation distinguishes attempted from response-backed disclosure.
// Matching occurs inside the driver, before values are discarded.
type WeatherObservation struct {
	Stage       string `json:"stage"`
	Destination string `json:"destination"`
	Category    string `json:"category"`
}

// WeatherSession is the bounded and redacted driver response.
type WeatherSession struct {
	SchemaVersion   int                  `json:"schema_version"`
	ProcedureID     string               `json:"procedure_id"`
	Candidate       string               `json:"candidate"`
	ChallengeSHA256 string               `json:"challenge_sha256"`
	BrowserSHA256   string               `json:"browser_sha256"`
	Functionality   string               `json:"functionality"`
	Geolocation     string               `json:"geolocation"`
	Status          string               `json:"status"`
	Gaps            []string             `json:"gaps"`
	Observations    []WeatherObservation `json:"observations"`
}

// WeatherGapEvidence explains one validated visibility gap without retaining
// captured request data.
type WeatherGapEvidence struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// WeatherCandidateEvidence summarizes verified session observations for one
// candidate. Counts are sessions; gaps are unique validated gaps.
type WeatherCandidateEvidence struct {
	Candidate              string               `json:"candidate"`
	SessionCount           int                  `json:"session_count"`
	AvailableSessions      int                  `json:"available_sessions"`
	UnavailableSessions    int                  `json:"unavailable_sessions"`
	UnknownSessions        int                  `json:"unknown_sessions"`
	AttemptedSessions      int                  `json:"attempted_sessions"`
	ResponseBackedSessions int                  `json:"response_backed_sessions"`
	VisibilityGaps         []WeatherGapEvidence `json:"visibility_gaps"`
}

func weatherGapEvidence(id string) WeatherGapEvidence {
	var reason string
	switch id {
	case "blocked-origin":
		reason = "A destination outside the reviewed origin allowlist was blocked."
	case "unsupported-body":
		reason = "A request body used an encoding outside the bounded parser."
	case "unsupported-channel":
		reason = "A worker, service worker, shared worker, or WebSocket channel was outside this capture."
	case "capture-incomplete":
		reason = "The capture did not provide complete visibility for supported browser channels."
	case "server-side-unobservable":
		reason = "The browser cannot observe onward handling after a response."
	default:
		reason = "A reviewed visibility limit was recorded."
	}
	return WeatherGapEvidence{ID: id, Reason: reason}
}
func weatherCandidateEvidence(run WeatherRun) []WeatherCandidateEvidence {
	candidates := weatherPlan().Candidates
	evidenceByCandidate := make([]WeatherCandidateEvidence, len(candidates))
	indexByCandidate := make(map[string]int, len(candidates))
	seenGaps := make([]map[string]bool, len(candidates))
	for index, candidate := range candidates {
		evidenceByCandidate[index] = WeatherCandidateEvidence{
			Candidate:      candidate,
			VisibilityGaps: []WeatherGapEvidence{},
		}
		indexByCandidate[candidate] = index
		seenGaps[index] = map[string]bool{}
	}
	for _, session := range run.Sessions {
		index, ok := indexByCandidate[session.Candidate]
		if !ok {
			continue
		}
		item := &evidenceByCandidate[index]
		item.SessionCount++
		switch session.Functionality {
		case "available":
			item.AvailableSessions++
		case "unavailable":
			item.UnavailableSessions++
		case "unknown":
			item.UnknownSessions++
		}
		attempted, responseBacked := false, false
		for _, observation := range session.Observations {
			switch observation.Stage {
			case "attempted":
				attempted = true
			case "response-backed":
				responseBacked = true
			}
		}
		if attempted {
			item.AttemptedSessions++
		}
		if responseBacked {
			item.ResponseBackedSessions++
		}
		for _, gap := range session.Gaps {
			if !seenGaps[index][gap] {
				seenGaps[index][gap] = true
				item.VisibilityGaps = append(item.VisibilityGaps, weatherGapEvidence(gap))
			}
		}
	}
	return evidenceByCandidate
}

// WeatherRun retains both orders for each candidate. Its identity is structural
// consistency, not an independent signature or proof of server-side handling.
type WeatherRun struct {
	SchemaVersion int              `json:"schema_version"`
	ProcedureID   string           `json:"procedure_id"`
	Sessions      []WeatherSession `json:"sessions"`
	TraceSHA256   []string         `json:"trace_sha256"`
}

// WeatherReview contains one reverified run and its shared minimization result.
type WeatherReview struct {
	Run               WeatherRun                 `json:"run"`
	Ladder            minimize.LadderSummary     `json:"ladder"`
	ReceiptSHA256     string                     `json:"receipt_sha256"`
	CandidateEvidence []WeatherCandidateEvidence `json:"candidate_evidence"`
}

// WeatherInput selects an explicit executable and a new output directory.
type WeatherInput struct {
	DriverPath string
	DriverArgs []string
	OutputDir  string
}

func weatherHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func weatherDigest(value any) string { data, _ := json.Marshal(value); return weatherHash(data) }
func weatherCandidates() []string {
	return []string{"precise", "coarse", "coarse", "precise", "precise", "denied", "denied", "precise"}
}
func weatherPlan() minimize.LadderPlan {
	return minimize.LadderPlan{SchemaVersion: 1, Name: "weather-location", Variable: "location", ReferenceCandidate: "precise", FunctionalityCriterion: weatherCriterion, Candidates: []string{"precise", "coarse", "denied"}}
}

func decodeWeather(data []byte, value any) error {
	if len(data) == 0 || len(data) > weatherLimit || !utf8.Valid(data) {
		return errors.New("weather artifact size or encoding invalid")
	}
	if jsoncheck.RejectDuplicateKeys(data) != nil {
		return errors.New("weather artifact JSON invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil {
		return errors.New("weather artifact fields invalid")
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return errors.New("weather artifact trailing content")
	}
	return nil
}

func validateWeatherSession(s WeatherSession) error {
	if s.SchemaVersion != 1 || s.ProcedureID != WeatherProcedureID || !trace.ValidSHA256(s.ChallengeSHA256) || !trace.ValidSHA256(s.BrowserSHA256) {
		return errors.New("weather session identity invalid")
	}
	if s.Candidate != "precise" && s.Candidate != "coarse" && s.Candidate != "denied" {
		return errors.New("weather candidate invalid")
	}
	if s.Functionality != "available" && s.Functionality != "unavailable" && s.Functionality != "unknown" {
		return errors.New("weather functionality invalid")
	}
	if s.Geolocation != "granted" && s.Geolocation != "denied" && s.Geolocation != "unknown" {
		return errors.New("weather geolocation invalid")
	}
	if s.Status != "complete" && s.Status != "workflow-failed" && s.Status != "rate-limited" {
		return errors.New("weather status invalid")
	}
	if s.Status != "complete" && s.Functionality != "unknown" {
		return errors.New("weather failure cannot establish functionality")
	}
	if s.Gaps == nil || s.Observations == nil || len(s.Gaps) > 16 || len(s.Observations) > 64 {
		return errors.New("weather observation limits invalid")
	}
	seen := map[string]bool{}
	for _, g := range s.Gaps {
		switch g {
		case "blocked-origin", "unsupported-body", "unsupported-channel", "capture-incomplete", "server-side-unobservable":
		default:
			return errors.New("weather gap invalid")
		}
		if seen[g] {
			return errors.New("weather gap duplicated")
		}
		seen[g] = true
	}
	seen = map[string]bool{}
	for _, o := range s.Observations {
		if (o.Stage != "attempted" && o.Stage != "response-backed") || (o.Destination != "weather-service" && o.Destination != "undeclared") || o.Category != "location" || (o.Destination == "undeclared" && o.Stage != "attempted") {
			return errors.New("weather observation invalid")
		}
		key := o.Stage + o.Destination
		if seen[key] {
			return errors.New("weather observation duplicated")
		}
		seen[key] = true
	}
	if s.Candidate == "denied" && s.Geolocation == "granted" {
		return errors.New("weather permission disagrees")
	}
	return nil
}

func weatherTrace(s WeatherSession) trace.Document {
	events := []trace.Event{}
	for _, o := range s.Observations {
		if o.Stage == "response-backed" {
			events = append(events, trace.Event{Source: "browser", Channel: "network", Kind: "request", Destination: "first-party", Fields: []string{"location"}})
		}
	}
	return trace.Document{SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: trace.Partial, Events: events}
}

// RunWeather performs eight fresh-profile sessions. Rate limits stop the run
// without publishing a conclusion; bounded workflow failures remain unknown.
func RunWeather(ctx context.Context, input WeatherInput) error {
	return runWeather(ctx, input, runDriver)
}
func runWeather(ctx context.Context, input WeatherInput, driver captureRunner) error {
	if ctx == nil || driver == nil || !filepath.IsAbs(input.DriverPath) || input.OutputDir == "" {
		return errors.New("weather requires context, absolute driver, and new output")
	}
	if _, err := os.Lstat(input.OutputDir); !os.IsNotExist(err) {
		return errors.New("weather output already exists or is inaccessible")
	}
	parent := filepath.Dir(input.OutputDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return errors.New("create weather parent")
	}
	staging, err := os.MkdirTemp(parent, ".weather-")
	if err != nil {
		return errors.New("create weather staging")
	}
	defer os.RemoveAll(staging)
	run := WeatherRun{SchemaVersion: 1, ProcedureID: WeatherProcedureID, Sessions: []WeatherSession{}, TraceSHA256: []string{}}
	for index, candidate := range weatherCandidates() {
		if ctx.Err() != nil {
			return errors.New("weather investigation canceled")
		}
		challenge := make([]byte, 32)
		if _, err := rand.Read(challenge); err != nil {
			return errors.New("weather challenge unavailable")
		}
		request := WeatherRequest{1, WeatherProcedureID, candidate, hex.EncodeToString(challenge), 30000}
		payload, _ := json.Marshal(request)
		sessionCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		data, err := driver(sessionCtx, input.DriverPath, input.DriverArgs, payload)
		cancel()
		if err != nil {
			return fmt.Errorf("weather session %d: driver failed", index+1)
		}
		var session WeatherSession
		if err := decodeWeather(data, &session); err != nil {
			return err
		}
		if err := validateWeatherSession(session); err != nil {
			return err
		}
		if session.Candidate != candidate || session.ChallengeSHA256 != weatherHash([]byte(request.Challenge)) {
			return errors.New("weather response binding disagrees")
		}
		if len(run.Sessions) > 0 && session.BrowserSHA256 != run.Sessions[0].BrowserSHA256 {
			return errors.New("weather browser changed between sessions")
		}
		if session.Status == "rate-limited" {
			return errors.New("weather site rate limited; stopped without publishing a result")
		}
		document := weatherTrace(session)
		traceData, _ := json.Marshal(document)
		digest, err := trace.SHA256(document)
		if err != nil {
			return err
		}
		if err := writeExclusive(filepath.Join(staging, fmt.Sprintf("trace-%02d.json", index+1)), traceData); err != nil {
			return err
		}
		run.Sessions = append(run.Sessions, session)
		run.TraceSHA256 = append(run.TraceSHA256, digest)
	}
	data, _ := json.Marshal(run)
	if err := writeExclusive(filepath.Join(staging, "weather.json"), data); err != nil {
		return err
	}
	ladder, err := summarizeWeather(run)
	if err != nil {
		return err
	}
	if err := minimize.SaveLadder(staging, ladder); err != nil {
		return err
	}
	if _, err := VerifyWeather(staging); err != nil {
		return err
	}
	if err := os.Rename(staging, input.OutputDir); err != nil {
		return errors.New("publish weather investigation")
	}
	return nil
}

func summarizeWeather(run WeatherRun) (minimize.LadderSummary, error) {
	if run.SchemaVersion != 1 || run.ProcedureID != WeatherProcedureID || len(run.Sessions) != 8 || len(run.TraceSHA256) != 8 {
		return minimize.LadderSummary{}, errors.New("weather run shape invalid")
	}
	seen := map[string]bool{}
	for i, s := range run.Sessions {
		if err := validateWeatherSession(s); err != nil {
			return minimize.LadderSummary{}, err
		}
		if s.Candidate != weatherCandidates()[i] || seen[s.ChallengeSHA256] || s.BrowserSHA256 != run.Sessions[0].BrowserSHA256 || s.Status == "rate-limited" {
			return minimize.LadderSummary{}, errors.New("weather run binding invalid")
		}
		seen[s.ChallengeSHA256] = true
	}
	results := []minimize.LadderCandidateResult{}
	for n, id := range []string{"coarse", "denied"} {
		sessions := run.Sessions[n*4 : n*4+4]
		result := minimize.LadderCandidateResult{ID: id, Directory: minimize.LadderCandidateDirectory(n, id), ReceiptSHA256: weatherDigest(sessions), Pairs: 2, PairsPerOrder: 1, CompletedPairs: 2, EvidenceState: evidence.Observed}
		for pair := 0; pair < 2; pair++ {
			a, b := sessions[pair*2], sessions[pair*2+1]
			if pair == 1 {
				a, b = b, a
			}
			unknown := a.Status != "complete" || b.Status != "complete" || a.Functionality != "available" || b.Functionality == "unknown"
			for _, s := range []WeatherSession{a, b} {
				for _, g := range s.Gaps {
					if g != "server-side-unobservable" {
						unknown = true
					}
				}
			}
			if unknown {
				result.UnknownPairs++
			} else if b.Functionality == "available" {
				result.NoChangePairs++
			} else {
				result.ChangedPairs++
			}
		}
		switch {
		case result.UnknownPairs > 0:
			result.Outcome = trace.ReplicationUnknown
			result.EvidenceState = evidence.Unknown
		case result.NoChangePairs == 2:
			result.Outcome = trace.NoChangeObserved
		case result.ChangedPairs == 2:
			result.Outcome = trace.ReplicatedChange
		default:
			result.Outcome = trace.MixedInconsistent
		}
		results = append(results, result)
	}
	return minimize.SummarizeLadder(weatherPlan(), minimize.LadderProvenance{Adapter: WeatherProcedureID, AdapterVersion: 1, ProcedureSHA256: weatherDigest(WeatherProcedureID), Scope: "outbound", ResetPolicy: BrowserReplicationResetPolicy}, 1, results)
}

func readWeatherFile(root, name string) ([]byte, error) {
	data, err := bundle.ReadBoundedFile(filepath.Join(root, name), weatherLimit)
	if err != nil {
		return nil, errors.New("weather artifact unavailable or invalid")
	}
	return data, nil
}

// VerifyWeather re-derives results and binds each portable trace to its session.
// ReceiptSHA256 may be retained independently as a trust anchor.
func VerifyWeather(root string) (WeatherReview, error) {
	data, err := readWeatherFile(root, "weather.json")
	if err != nil {
		return WeatherReview{}, err
	}
	var run WeatherRun
	if err := decodeWeather(data, &run); err != nil {
		return WeatherReview{}, err
	}
	expected, err := summarizeWeather(run)
	if err != nil {
		return WeatherReview{}, err
	}
	for i, s := range run.Sessions {
		data, err := readWeatherFile(root, fmt.Sprintf("trace-%02d.json", i+1))
		if err != nil {
			return WeatherReview{}, err
		}
		doc, err := trace.Decode(data)
		if err != nil {
			return WeatherReview{}, err
		}
		digest, err := trace.SHA256(doc)
		if err != nil {
			return WeatherReview{}, err
		}
		want, _ := trace.SHA256(weatherTrace(s))
		if digest != want || digest != run.TraceSHA256[i] {
			return WeatherReview{}, errors.New("weather trace binding disagrees")
		}
	}
	data, err = readWeatherFile(root, "minimization.json")
	if err != nil {
		return WeatherReview{}, err
	}
	var saved minimize.LadderSummary
	if err := decodeWeather(data, &saved); err != nil {
		return WeatherReview{}, err
	}
	if !reflect.DeepEqual(saved, expected) {
		return WeatherReview{}, errors.New("weather minimization disagrees")
	}
	return WeatherReview{Run: run, Ladder: expected, ReceiptSHA256: weatherDigest(run), CandidateEvidence: weatherCandidateEvidence(run)}, nil
}
