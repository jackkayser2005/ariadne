package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunServeWiresStandaloneTraceFlag(t *testing.T) {
	tracePath := writeTraceFile(t, trace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  trace.Complete,
		Events: []trace.Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"location"},
		}},
	})
	var stdout, stderr bytes.Buffer
	called := false
	if exitCode := runServe([]string{"--trace", tracePath, "archive-root"}, &stdout, &stderr, func(address string, handler http.Handler) error {
		called = address == "127.0.0.1:8787" && handler != nil
		if !called {
			return nil
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8787/trace", nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "What did this trace record?") {
			t.Fatalf("trace route = %d %q", recorder.Code, recorder.Body.String())
		}
		return nil
	}); exitCode != 0 || !called || stderr.Len() != 0 {
		t.Fatalf("runServe() = %d, called=%v, stdout=%q, stderr=%q", exitCode, called, stdout.String(), stderr.String())
	}
}
