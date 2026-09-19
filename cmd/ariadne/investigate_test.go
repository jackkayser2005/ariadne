package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/trace"
	"github.com/jackkayser2005/ariadne/internal/ui"
)

func TestGuidedCLIValidatesInputsAndRetainsNoOpen(t *testing.T) {
	var output, stderr bytes.Buffer
	called := false
	serve := func(options ui.InvestigationOptions, open bool, w io.Writer) error {
		called = true
		if open || options.InitialURL != "https://example.invalid" || options.OutputRoot == "" {
			t.Fatal("incorrect guided configuration")
		}
		return nil
	}
	if got := runGuided("investigate", []string{"--no-open", "https://example.invalid"}, &output, &stderr, serve); got != 0 || !called {
		t.Fatal(got, stderr.String())
	}
	for _, args := range [][]string{{"--bad"}, {"--addr", "0.0.0.0:1"}, {"--addr", "localhost:8787"}, {"--addr", "127.0.0.1:-1"}, {"--duration", "21m", "https://example.invalid"}, {"--duration", "-1s"}, {"--duration", "1s"}, {"--json"}, {"--location", "deny"}, {"file:///secret"}, {"https://user:password@example.invalid"}, {"a", "b"}} {
		called = false
		stderr.Reset()
		if got := runGuided("investigate", args, &output, &stderr, serve); got != 2 || called {
			t.Fatalf("%v: %d %s", args, got, stderr.String())
		}
	}
	if got := runGuided("investigate", []string{"--help"}, &output, &stderr, serve); got != 0 {
		t.Fatal(got)
	}
	if got := runGuided("investigate", nil, &output, &stderr, func(ui.InvestigationOptions, bool, io.Writer) error { return errors.New("listener unavailable") }); got != 1 {
		t.Fatal(got)
	}
	if got := run([]string{"inspect"}, &output, &stderr); got != 2 {
		t.Fatal("inspect command dispatch failed")
	}
}

func TestInspectCLIReverifiesSavedEvidenceAndOmitsPrivateNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved")
	_, err := browser.SaveInvestigation(path, browser.CaptureResult{Journey: trace.NewJourney(), Destinations: []browser.DestinationName{{Alias: "d1", Origin: "https://private.invalid"}}, Steps: []browser.InvestigationStep{}})
	if err != nil {
		t.Fatal(err)
	}
	var output, stderr bytes.Buffer
	serve := func(options ui.InvestigationOptions, open bool, w io.Writer) error {
		if options.SavedPath != path || !open {
			t.Fatal("inspect configuration")
		}
		return nil
	}
	if got := runGuided("inspect", []string{path}, &output, &stderr, serve); got != 0 {
		t.Fatal(stderr.String())
	}
	if got := runGuided("inspect", []string{"--json", path}, &output, &stderr, serve); got != 0 {
		t.Fatal(stderr.String())
	}
	if strings.Contains(output.String(), "private.invalid") {
		t.Fatal("CLI JSON leaked private context")
	}
	if _, err := browser.DecodeJourneyBundle(output.Bytes()); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--output", "--duration", "--block-origin", "--location"} {
		value := "value"
		if flag == "--duration" {
			value = "1s"
		}
		if got := runGuided("inspect", []string{flag, value, path}, &output, &stderr, serve); got != 2 {
			t.Fatal("invalid inspect flags accepted")
		}
	}
	if err := os.WriteFile(filepath.Join(path, "receipt.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := runGuided("inspect", []string{"--json", path}, &output, &stderr, serve); got != 1 {
		t.Fatal("tampered inspection accepted")
	}
}

func TestGuidedServerHasBoundedShutdownAndFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	if err := serveGuided(ui.InvestigationOptions{Context: ctx, Host: "127.0.0.1:0", OutputRoot: t.TempDir()}, false, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Ariadne local interface: http://127.0.0.1:") {
		t.Fatal("missing local interface link")
	}
	if err := serveGuided(ui.InvestigationOptions{Host: "not an address"}, false, io.Discard); err == nil {
		t.Fatal("invalid listener accepted")
	}
	if err := serveGuided(ui.InvestigationOptions{Host: "127.0.0.1:0", SavedPath: filepath.Join(t.TempDir(), "missing")}, false, io.Discard); err == nil {
		t.Fatal("invalid saved evidence served")
	}
}

func TestTimedInvestigationRecordsAndCancelsInRealBrowser(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("requires installed Chrome/Edge")
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<title>Local timed fixture</title>")
	}))
	defer site.Close()
	var output bytes.Buffer
	path := filepath.Join(t.TempDir(), "saved")
	if err := recordTimed(context.Background(), browser.CaptureOptions{URL: site.URL, Headless: true}, 100*time.Millisecond, path, &output); err != nil {
		t.Fatal(err)
	}
	if _, _, err := browser.ReadInvestigation(path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Saved investigation:") {
		t.Fatal("missing terminal result")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := recordTimed(ctx, browser.CaptureOptions{URL: site.URL, Headless: true}, time.Second, filepath.Join(t.TempDir(), "cancelled"), io.Discard); err == nil {
		t.Fatal("cancelled recording succeeded")
	}
}
