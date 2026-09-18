package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

//go:embed investigate.html
var investigateHTML string

//go:embed investigate.js
var investigateJS string

// InvestigationOptions configures a separate authenticated capture surface.
// SavedPath makes the entire handler read-only, including all capture routes.
type InvestigationOptions struct {
	Context                                 context.Context
	Host, InitialURL, OutputRoot, SavedPath string
	Headless                                bool
}

// InvestigationHandler owns one guided session. Existing evidence review
// handlers remain read-only and do not share its capture controls or token.
type InvestigationHandler struct {
	mu               sync.Mutex
	options          InvestigationOptions
	ctx              context.Context
	cancel           context.CancelFunc
	token            string
	capture          *browser.JourneyCapture
	markers          []browser.SyntheticMarker
	bundle           *browser.JourneyBundle
	destinations     []browser.DestinationName
	savedPath, phase string
}

// NewInvestigationHandler validates loopback binding and any saved bundle before
// making a capture or review surface available.
func NewInvestigationHandler(options InvestigationOptions) (*InvestigationHandler, error) {
	ip, _, ok := reviewAuthority(options.Host)
	if !ok || !ip.IsLoopback() {
		return nil, errors.New("investigation interface must bind to a loopback IP and port")
	}
	if options.InitialURL != "" {
		if _, err := browser.InvestigationURL(options.InitialURL); err != nil {
			return nil, err
		}
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(options.Context)
	h := &InvestigationHandler{options: options, ctx: ctx, cancel: cancel, token: rand.Text() + rand.Text(), markers: browser.GenerateMarkers(), phase: "ready"}
	if options.SavedPath != "" {
		bundle, names, err := browser.ReadInvestigation(options.SavedPath)
		if err != nil {
			cancel()
			return nil, err
		}
		h.bundle = &bundle
		h.destinations = names
		h.savedPath = options.SavedPath
		h.phase = "saved"
	}
	return h, nil
}

// URL returns the single-session bootstrap URL. The token stays in the fragment,
// outside HTTP requests, access logs, and referrer headers.
func (h *InvestigationHandler) URL() string { return "http://" + h.options.Host + "/#" + h.token }

// Close cancels the session and removes its owned browser profile.
func (h *InvestigationHandler) Close() error {
	h.cancel()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.capture != nil {
		_, err := h.capture.Stop(true)
		h.capture = nil
		return err
	}
	return nil
}

func (h *InvestigationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	if !reviewHostMatches(h.options.Host, r.Host) {
		http.Error(w, "host not allowed", http.StatusMisdirectedRequest)
		return
	}
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || net.ParseIP(remote) == nil || !net.ParseIP(remote).IsLoopback() {
		http.Error(w, "loopback connection required", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/" || r.URL.Path == "/investigate.js" {
		if !getOnly(w, r) {
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, investigateHTML)
		} else {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = io.WriteString(w, investigateJS)
		}
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	origin := "http://" + h.options.Host
	if (r.Method != http.MethodGet && r.Header.Get("Origin") != origin) || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(provided), []byte(h.token)) != 1 {
		http.Error(w, "open the session link printed by Ariadne", http.StatusUnauthorized)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.URL.Path == "/api/state" {
		if !getOnly(w, r) {
			return
		}
		h.state(w)
		return
	}
	if r.URL.Path == "/api/export" {
		if !getOnly(w, r) {
			return
		}
		if h.savedPath == "" {
			http.Error(w, "stop and save the investigation before exporting", http.StatusConflict)
			return
		}
		bundle, _, err := browser.ReadInvestigation(h.savedPath)
		if err != nil {
			http.Error(w, "saved evidence failed verification", http.StatusUnprocessableEntity)
			return
		}
		data, err := bundle.PortableJSON()
		if err != nil {
			http.Error(w, "saved evidence failed verification", http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="ariadne-evidence.json"`)
		_, _ = w.Write(data)
		return
	}
	if h.options.SavedPath != "" {
		http.Error(w, "saved investigation review is read only", http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "JSON request required", http.StatusUnsupportedMediaType)
		return
	}
	var input struct {
		URL          string   `json:"url"`
		BlockOrigins []string `json:"block_origins"`
		Location     string   `json:"location"`
		Marker       string   `json:"marker"`
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil || jsoncheck.RejectDuplicateKeys(data) != nil {
		http.Error(w, "request is invalid or oversized", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "request fields are invalid", http.StatusBadRequest)
		return
	}
	switch r.URL.Path {
	case "/api/start":
		if h.capture != nil {
			http.Error(w, "an investigation is already recording", http.StatusConflict)
			return
		}
		if h.options.OutputRoot == "" {
			http.Error(w, "an output directory is required", http.StatusConflict)
			return
		}
		capture, err := browser.StartCapture(h.ctx, browser.CaptureOptions{URL: input.URL, Markers: h.markers, BlockOrigins: input.BlockOrigins, Location: input.Location, Headless: h.options.Headless})
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		h.capture = capture
		h.bundle = nil
		h.savedPath = ""
		h.destinations = nil
		h.phase = "recording"
	case "/api/fill":
		if h.capture == nil {
			http.Error(w, "start recording before filling a test input", http.StatusConflict)
			return
		}
		if err := h.capture.FillSynthetic(r.Context(), ":focus", input.Marker); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	case "/api/stop", "/api/cancel":
		if h.capture == nil {
			http.Error(w, "no investigation is recording", http.StatusConflict)
			return
		}
		cancelled := r.URL.Path == "/api/cancel"
		result, cleanupErr := h.capture.Stop(cancelled)
		h.capture = nil
		if cancelled {
			h.phase = "cancelled"
			h.bundle = nil
			h.destinations = nil
			h.savedPath = ""
			h.markers = browser.GenerateMarkers()
		} else {
			path := filepath.Join(h.options.OutputRoot, "investigation-"+time.Now().UTC().Format("20060102-150405")+"-"+rand.Text()[:8])
			bundle, err := browser.SaveInvestigation(path, result)
			if err != nil {
				h.phase = "error"
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
				return
			}
			h.bundle = &bundle
			h.destinations = result.Destinations
			h.savedPath = path
			h.phase = "saved"
		}
		if cleanupErr != nil {
			http.Error(w, cleanupErr.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	h.state(w)
}

func (h *InvestigationHandler) state(w http.ResponseWriter) {
	var journey *trace.Journey
	if h.capture != nil {
		snapshot := h.capture.Snapshot()
		journey = &snapshot.Journey
		h.destinations = snapshot.Destinations
		if !h.capture.Recording() {
			h.phase = "interrupted"
		}
	} else if h.bundle != nil {
		journey = &h.bundle.Journey
	}
	state := struct {
		Phase        string                     `json:"phase"`
		InitialURL   string                     `json:"initial_url"`
		ReadOnly     bool                       `json:"read_only"`
		Markers      []browser.SyntheticMarker  `json:"markers"`
		Journey      *trace.Journey             `json:"journey"`
		Destinations []browser.DestinationName  `json:"destinations"`
		SavedPath    string                     `json:"saved_path"`
		Categories   []trace.CategoryDefinition `json:"categories"`
	}{h.phase, h.options.InitialURL, h.options.SavedPath != "", h.markers, journey, h.destinations, h.savedPath, trace.CategoryDefinitions()}
	if state.ReadOnly {
		state.Markers = nil
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(state)
}
