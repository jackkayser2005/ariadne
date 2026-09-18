package ui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestStandaloneTraceViewRendersReaderLabelsAndHidesPath(t *testing.T) {
	tracePath := `C:\private\trace-with-secret.json`
	digest := strings.Repeat("a", 64)
	summary := trace.VerificationSummary{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Partial,
		Events:        1,
		TraceSHA256:   digest,
	}
	document := trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Partial,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request",
			Destination: "analytics", Fields: []string{"location"},
		}},
	}
	h := newHandler(handler{
		tracePath: tracePath,
		traceRead: func(path string) (trace.Document, trace.VerificationSummary, error) {
			if path != tracePath {
				t.Fatalf("trace path = %q", path)
			}
			return document, summary, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace", nil))
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", recorder.Code, body)
	}
	for _, want := range []string{
		"What did this trace record?",
		"Browser",
		"Network",
		"Request",
		"Location",
		"Analytics",
		"Some declared channels were missing",
		"Missing channels remain unknown.",
		digest,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, secret := range []string{tracePath, "secret", "https://"} {
		if strings.Contains(body, secret) {
			t.Fatalf("body disclosed %q", secret)
		}
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/trace", nil))
	if post.Code != http.StatusMethodNotAllowed || post.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST status = %d, allow=%q, body=%q", post.Code, post.Header().Get("Allow"), post.Body.String())
	}
}

func TestStandaloneTraceViewRejectsUnavailableTrace(t *testing.T) {
	h := newHandler(handler{
		tracePath: "trace.json",
		traceRead: func(string) (trace.Document, trace.VerificationSummary, error) {
			return trace.Document{}, trace.VerificationSummary{}, errors.New("private verifier detail")
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/trace", nil))
	if recorder.Code != http.StatusUnprocessableEntity || recorder.Body.String() != "trace unavailable\n" {
		t.Fatalf("response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestStandaloneTraceViewAddsIndexCard(t *testing.T) {
	h := newHandler(handler{
		root:      "archive",
		index:     func(string) ([]bundle.ArchiveEntry, error) { return nil, nil },
		tracePath: "trace.json",
		traceRead: func(string) (trace.Document, trace.VerificationSummary, error) {
			return trace.Document{}, trace.VerificationSummary{}, nil
		},
	})
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Open standalone trace explanation") {
		t.Fatalf("index response = %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestStandaloneTraceLabelsHaveSafeFallbacks(t *testing.T) {
	for _, test := range []struct {
		name string
		got  string
		want string
	}{
		{name: "source", got: traceSourceLabel("other"), want: "Reviewed source"},
		{name: "channel", got: traceChannelLabel("other"), want: "Reviewed channel"},
		{name: "kind", got: traceKindLabel("other"), want: "Reviewed event"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("label = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestStandaloneTraceReadsVerifiedFileAndRejectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-trace.json")
	document := trace.Document{SchemaVersion: 1, Redacted: true, Scope: "all", Completeness: trace.Partial, Events: []trace.Event{
		{Source: "android", Channel: "app-storage", Kind: "storage-write", Destination: "first-party", Fields: []string{"account-id"}},
		{Source: "browser", Channel: "cookie", Kind: "cookie-write", Destination: "first-party", Fields: []string{"cookie-id"}},
		{Source: "browser", Channel: "web-storage", Kind: "storage-write", Destination: "first-party", Fields: []string{"email"}},
		{Source: "desktop", Channel: "network", Kind: "request", Destination: "unknown", Fields: []string{"device-id"}},
		{Source: "proxy", Channel: "network", Kind: "response", Destination: "unknown", Fields: []string{"session-id"}},
		{Source: "browser", Channel: "network", Kind: "beacon", Destination: "analytics", Fields: []string{"region"}},
	}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	h := HandlerWithReviewOptions(ReviewOptions{ArchiveRoot: t.TempDir(), TracePath: path, ExpectedHost: "127.0.0.1:8787"})
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:8787/trace", nil))
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, label := range []string{"Android app", "App storage", "Browser cookie", "Cookie write", "Web storage", "Storage write", "Desktop app", "Network proxy", "Response", "Background beacon"} {
		if !strings.Contains(response.Body.String(), label) {
			t.Fatalf("missing label %q", label)
		}
	}
	if strings.Contains(response.Body.String(), path) {
		t.Fatal("private path leaked")
	}
	if err := os.WriteFile(path, []byte(`{"private":"secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:8787/trace", nil))
	if response.Code != 422 || strings.Contains(response.Body.String(), "secret") {
		t.Fatal("invalid trace was not withheld")
	}
}
