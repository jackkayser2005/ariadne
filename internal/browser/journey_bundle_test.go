package browser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func bundleFixture(t *testing.T) CaptureResult {
	t.Helper()
	journey := trace.NewJourney()
	if err := journey.Append(trace.JourneyObservation{Kind: "request", Context: "network", Destination: "d1", Reference: "r1", Matches: []trace.JourneyMatch{{Marker: "m1", Category: "email", Encoding: "exact"}}}); err != nil {
		t.Fatal(err)
	}
	return CaptureResult{Journey: journey, Destinations: []DestinationName{{Alias: "d1", Origin: "https://private-fixture.invalid"}}, Steps: []InvestigationStep{{Kind: "input", Selector: "input:nth-of-type(1)", Marker: "m1"}}}
}

func TestInvestigationRoundTripSeparatesPortableAndPrivateEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved")
	bundle, err := SaveInvestigation(path, bundleFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	read, names, err := ReadInvestigation(path)
	if err != nil || len(names) != 1 || read.Receipt != bundle.Receipt {
		t.Fatalf("read: %#v %v", names, err)
	}
	portable, err := read.PortableJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-fixture.invalid", "input:nth-of-type", "private-context", "destinations", "selector"} {
		if bytes.Contains(portable, []byte(secret)) {
			t.Fatalf("private context leaked: %s", secret)
		}
	}
	file := filepath.Join(t.TempDir(), "evidence.json")
	if err := os.WriteFile(file, portable, 0600); err != nil {
		t.Fatal(err)
	}
	read, names, err = ReadInvestigation(file)
	if err != nil || names != nil || read.Receipt != bundle.Receipt {
		t.Fatal(err)
	}
	if _, err := SaveInvestigation(path, bundleFixture(t)); err == nil {
		t.Fatal("overwrote an investigation")
	}
	if err := os.Remove(filepath.Join(path, "private-context.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadInvestigation(path); err == nil {
		t.Fatal("incomplete private context accepted")
	}
	if err := os.Remove(filepath.Join(path, "private-receipt.json")); err != nil {
		t.Fatal(err)
	}
	if _, names, err := ReadInvestigation(path); err != nil || names != nil {
		t.Fatal("portable directory rejected", err)
	}
}

func TestInvestigationRejectsTamperingMalformedAndOversizedArtifacts(t *testing.T) {
	bundle, err := BuildJourneyBundle(bundleFixture(t).Journey)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := bundle.PortableJSON()
	for _, input := range [][]byte{nil, []byte{255}, []byte(`{"schema_version":1,"schema_version":1}`), []byte(`{"unknown":true}`), append(bytes.Clone(data), []byte(` {}`)...), bytes.Repeat([]byte("x"), maxJourneyBundleBytes+1)} {
		if _, err := DecodeJourneyBundle(input); err == nil {
			t.Fatal("malformed export accepted")
		}
	}
	for _, mutate := range []func(*JourneyBundle){
		func(b *JourneyBundle) { b.SchemaVersion = 2 }, func(b *JourneyBundle) { b.Receipt.Kind = "other" },
		func(b *JourneyBundle) { b.Journey.Observations[0].Kind = "beacon" }, func(b *JourneyBundle) { b.Receipt.TraceSHA256 = strings.Repeat("0", 64) },
		func(b *JourneyBundle) { b.Trace.Events = nil },
		func(b *JourneyBundle) {
			b.Trace.Events[0].Fields = []string{"location"}
			b.Receipt.TraceSHA256, _ = trace.SHA256(b.Trace)
		},
	} {
		copy, err := DecodeJourneyBundle(data)
		if err != nil {
			t.Fatal(err)
		}
		mutate(&copy)
		if _, err := copy.PortableJSON(); err == nil {
			t.Fatal("tampered evidence accepted")
		}
	}
	for _, file := range []string{"journey.json", "trace.json", "receipt.json", "private-context.json", "private-receipt.json"} {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "saved")
			_, err := SaveInvestigation(path, bundleFixture(t))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, file), []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := ReadInvestigation(path); err == nil {
				t.Fatal("malformed file accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "large.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxJourneyBundleBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadInvestigation(path); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, _, err := ReadInvestigation(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing input accepted")
	}
}

func TestInvestigationPrivateContextAndUnsafePathsFailClosed(t *testing.T) {
	for _, mutate := range []func(*CaptureResult){
		func(r *CaptureResult) { r.Journey.Redacted = false }, func(r *CaptureResult) { r.Destinations = nil }, func(r *CaptureResult) { r.Steps = nil },
		func(r *CaptureResult) { r.Destinations[0].Alias = "d2" }, func(r *CaptureResult) { r.Destinations[0].Origin = "https://private.invalid/path" },
		func(r *CaptureResult) { r.Journey.Observations[0].Destination = "d2" }, func(r *CaptureResult) { r.Steps[0].Kind = "script" },
		func(r *CaptureResult) { r.Steps[0].Kind = "click" }, func(r *CaptureResult) { r.Steps[0].Marker = "invalid" },
		func(r *CaptureResult) { r.Steps[0].Selector = strings.Repeat("x", 513) },
	} {
		result := bundleFixture(t)
		mutate(&result)
		if _, err := SaveInvestigation(filepath.Join(t.TempDir(), "saved"), result); err == nil {
			t.Fatal("invalid context accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveInvestigation(filepath.Join(path, "saved"), bundleFixture(t)); err == nil {
		t.Fatal("file parent accepted")
	}
	if _, err := readInvestigationFile(t.TempDir(), 10); err == nil {
		t.Fatal("directory accepted as file")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(path, link); err == nil {
		if _, _, err := ReadInvestigation(link); err == nil {
			t.Fatal("symlink accepted")
		}
	}
	root := filepath.Join(t.TempDir(), "saved")
	_, err := SaveInvestigation(root, bundleFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(root, "private-context.json")
	data, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	var private privateJourneyContext
	if err := json.Unmarshal(data, &private); err != nil {
		t.Fatal(err)
	}
	private.Destinations[0].Origin = "https://different.invalid"
	data, _ = json.Marshal(private)
	if err := os.WriteFile(privatePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadInvestigation(root); err == nil {
		t.Fatal("altered destination context accepted")
	}
}
