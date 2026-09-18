package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func testJourney(t *testing.T) Journey {
	t.Helper()
	j := NewJourney()
	for _, kind := range []string{"input", "storage-write", "worker-send", "request"} {
		if err := j.Append(JourneyObservation{Kind: kind, Context: "page", Destination: "d1", Reference: "r1", Matches: []JourneyMatch{{Marker: "m1", Category: "email", Encoding: "exact"}}}); err != nil {
			t.Fatal(err)
		}
	}
	j.LinkMatches()
	return j
}

func TestJourneyPreservesOrderAndMarksEqualValuesAsInference(t *testing.T) {
	j := testJourney(t)
	data, err := j.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeJourney(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Relationships) != 3 || decoded.Observations[0].Kind != "input" || decoded.Observations[3].Kind != "request" {
		t.Fatal(decoded)
	}
	for _, relation := range decoded.Relationships {
		if relation.State != evidence.Inferred {
			t.Fatal("causality invented")
		}
	}
	before, _ := j.SHA256()
	j.Observations[1].Kind = "storage-read"
	after, _ := j.SHA256()
	if before == after {
		t.Fatal("ordered observation not bound")
	}
	j.Observations[0], j.Observations[1] = j.Observations[1], j.Observations[0]
	if _, err := j.SHA256(); err == nil {
		t.Fatal("out-of-order sequence accepted")
	}
}

func TestJourneyRejectsHostileArtifactsAndUnsupportedClaims(t *testing.T) {
	for _, data := range [][]byte{nil, []byte{255}, bytes.Repeat([]byte("x"), maxJourneyBytes+1), []byte(`{"schema_version":1,"schema_version":1}`), []byte(`{"payload":"secret"}`), []byte(`[]`)} {
		if _, err := DecodeJourney(data); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	edits := []func(*Journey){
		func(j *Journey) { j.SchemaVersion = 99 }, func(j *Journey) { j.Redacted = false }, func(j *Journey) { j.Scope = "raw" },
		func(j *Journey) { j.Completeness = "maybe" }, func(j *Journey) { j.Observations = nil }, func(j *Journey) { j.Gaps = nil }, func(j *Journey) { j.Relationships = nil },
		func(j *Journey) { j.Gaps = []string{"secret"} }, func(j *Journey) { j.Gaps = []string{"size-limit", "size-limit"} }, func(j *Journey) { j.Gaps = []string{"size-limit"} },
		func(j *Journey) { j.Observations[0].Sequence = 0 }, func(j *Journey) { j.Observations[0].Destination = "https://private.test" },
		func(j *Journey) { j.Observations[0].Destination = "d257" }, func(j *Journey) { j.Observations[0].Reference = "r01" },
		func(j *Journey) { j.Observations[0].Context = "secret" }, func(j *Journey) { j.Observations[0].Kind = "secret" },
		func(j *Journey) { j.Observations[0].Matches = nil }, func(j *Journey) { j.Observations[0].Matches[0].Category = "secret" },
		func(j *Journey) { j.Observations[0].Matches[0].Encoding = "encrypted" }, func(j *Journey) { j.Observations[0].Matches[0].Marker = "m17" },
		func(j *Journey) {
			j.Observations[0].Matches = append(j.Observations[0].Matches, j.Observations[0].Matches[0])
		},
		func(j *Journey) { j.Relationships[0].State = evidence.Observed }, func(j *Journey) { j.Relationships[0].Kind = "caused" },
		func(j *Journey) { j.Relationships[0].From = 0 }, func(j *Journey) { j.Relationships[0].To = 99 },
		func(j *Journey) { j.Relationships[0].Marker = "m2" }, func(j *Journey) { j.Relationships = append(j.Relationships, j.Relationships[0]) },
	}
	for i, edit := range edits {
		j := testJourney(t)
		edit(&j)
		if _, err := j.CanonicalBytes(); err == nil {
			t.Fatalf("mutation %d accepted", i)
		}
		if _, err := j.Trace(); err == nil {
			t.Fatalf("mutation %d projected", i)
		}
	}
	data, _ := testJourney(t).CanonicalBytes()
	if _, err := DecodeJourney(append(data, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing input")
	}
}

func TestJourneyVisibilityLimitsAndLegacyProjection(t *testing.T) {
	j := testJourney(t)
	for _, kind := range []string{"storage-read", "cookie-read", "cookie-write", "beacon", "response", "websocket-sent", "websocket-received", "navigation"} {
		if err := j.Append(JourneyObservation{Kind: kind, Context: "network", Destination: "d2", Reference: "r2", Matches: []JourneyMatch{{Marker: "m1", Category: "email", Encoding: "base64"}}}); err != nil {
			t.Fatal(err)
		}
	}
	j.AddGap("server-side-unobservable")
	j.AddGap("unmatched-information")
	if j.Completeness != Complete {
		t.Fatal("out-of-scope limit changed bounded completeness")
	}
	projected, err := j.Trace()
	if err != nil {
		t.Fatal(err)
	}
	if projected.Completeness != Partial || projected.SchemaVersion != 1 {
		t.Fatal("legacy coverage upgraded")
	}
	if _, err := SHA256(projected); err != nil {
		t.Fatal(err)
	}
	j.AddGap("worker-unavailable")
	j.AddGap("worker-unavailable")
	j.AddGap("unreviewed-detail")
	if j.Completeness != Partial || len(j.Gaps) != 4 {
		t.Fatal(j.Gaps)
	}
	j.LinkMatches()
	if err := j.Validate(); err != nil {
		t.Fatal(err)
	}
	j.Observations = make([]JourneyObservation, MaxJourneyObservations)
	if err := j.Append(JourneyObservation{}); err == nil {
		t.Fatal("unbounded observations")
	}
	if err := NewJourney().Validate(); err != nil {
		t.Fatal(err)
	}
	empty := NewJourney()
	if err := empty.Append(JourneyObservation{}); err == nil {
		t.Fatal("invalid observation appended")
	}
}

func TestJourneyReadIsBoundedAndCanonical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journey.json")
	if _, err := ReadJourney(path); err == nil {
		t.Fatal("missing path")
	}
	j := testJourney(t)
	data, _ := j.CanonicalBytes()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	read, err := ReadJourney(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := j.SHA256()
	got, _ := read.SHA256()
	if want != got {
		t.Fatal("identity drift")
	}
	for _, secret := range []string{"http", "example.invalid", "value", "selector"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("unexpected raw field %q", secret)
		}
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxJourneyBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadJourney(path); err == nil {
		t.Fatal("oversized file")
	}
	data, _ = json.Marshal(j)
	if _, err := DecodeJourney(data); err != nil {
		t.Fatal(err)
	}
}
