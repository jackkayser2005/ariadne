package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

// JourneySchemaVersion identifies the ordered browser-observation companion to trace-v1.
const JourneySchemaVersion = 1

// MaxJourneyObservations bounds both recording and decoding.
const MaxJourneyObservations = 2048
const maxJourneyMatches = 4096
const maxJourneyBytes = 2 << 20

// JourneyMatch records a synthetic marker's representation, never its value.
// A transformed match identifies an observed representation, not its producer.
type JourneyMatch struct {
	Marker   string `json:"marker"`
	Category string `json:"category"`
	Encoding string `json:"encoding"`
}

// JourneyObservation is one ordered observation with a bounded evidence reference.
// Sequence is collector order; it does not establish causality across contexts.
type JourneyObservation struct {
	Sequence    int            `json:"sequence"`
	Kind        string         `json:"kind"`
	Context     string         `json:"context"`
	Destination string         `json:"destination"`
	Reference   string         `json:"reference"`
	Matches     []JourneyMatch `json:"matches"`
}

// JourneyRelationship connects observations without upgrading value equality to causality.
type JourneyRelationship struct {
	From   int            `json:"from"`
	To     int            `json:"to"`
	Kind   string         `json:"kind"`
	State  evidence.State `json:"state"`
	Marker string         `json:"marker,omitempty"`
}

// Journey retains ordered, raw-value-free browser observations. Destination
// names, page URLs, selector paths, and synthetic values belong outside this artifact.
type Journey struct {
	SchemaVersion int                   `json:"schema_version"`
	Redacted      bool                  `json:"redacted"`
	Scope         string                `json:"scope"`
	Completeness  string                `json:"completeness"`
	Observations  []JourneyObservation  `json:"observations"`
	Relationships []JourneyRelationship `json:"relationships"`
	Gaps          []string              `json:"gaps"`
}

// NewJourney returns an empty bounded observation record. Coverage refers only
// to supported synthetic-marker channels, never all personal information.
func NewJourney() Journey {
	return Journey{SchemaVersion: JourneySchemaVersion, Redacted: true, Scope: "browser-synthetic-markers-v1", Completeness: Complete,
		Observations: []JourneyObservation{}, Relationships: []JourneyRelationship{}, Gaps: []string{}}
}

// JourneyGapKnown reports whether a visibility gap has a reviewed explanation.
func JourneyGapKnown(gap string) bool {
	return slices.Contains([]string{"instrumentation-unavailable", "unsupported-payload", "size-limit", "event-limit", "request-failed", "browser-crashed", "cancelled", "out-of-scope-navigation", "manual-checkpoint", "worker-unavailable", "frame-unavailable", "unmatched-information", "server-side-unobservable"}, gap)
}

// AddGap keeps missing visibility explicit and deduplicated.
func (j *Journey) AddGap(gap string) {
	if !JourneyGapKnown(gap) {
		gap = "instrumentation-unavailable"
	}
	if !slices.Contains(j.Gaps, gap) {
		j.Gaps = append(j.Gaps, gap)
		slices.Sort(j.Gaps)
	}
	// These are limits outside the declared synthetic-marker instrumentation.
	// They still appear prominently in the reader, even when that scope completed.
	if gap != "unmatched-information" && gap != "server-side-unobservable" {
		j.Completeness = Partial
	}
}

// Append records one observation, rejecting malformed metadata and bounding memory.
func (j *Journey) Append(observation JourneyObservation) error {
	totalMatches := len(observation.Matches)
	for _, previous := range j.Observations {
		totalMatches += len(previous.Matches)
	}
	if len(j.Observations) >= MaxJourneyObservations || totalMatches > maxJourneyMatches {
		j.AddGap("event-limit")
		return errors.New("journey observation limit reached")
	}
	observation.Sequence = len(j.Observations) + 1
	if err := validateJourneyObservation(observation); err != nil {
		return err
	}
	observation.Matches = slices.Clone(observation.Matches)
	j.Observations = append(j.Observations, observation)
	return nil
}

// LinkMatches records adjacency within each synthetic marker's observations.
// Every such relationship is explicitly inferred; a matching value alone cannot
// establish a transfer, a transformation operation, or a causal path.
func (j *Journey) LinkMatches() {
	j.Relationships = []JourneyRelationship{}
	last := map[string]int{}
	for _, observation := range j.Observations {
		seen := map[string]bool{}
		for _, match := range observation.Matches {
			if seen[match.Marker] {
				continue
			}
			seen[match.Marker] = true
			if previous := last[match.Marker]; previous > 0 {
				j.Relationships = append(j.Relationships, JourneyRelationship{From: previous, To: observation.Sequence, Kind: "same-marker", State: evidence.Inferred, Marker: match.Marker})
			}
			last[match.Marker] = observation.Sequence
		}
	}
}

// Validate checks the complete portable journey contract without sorting observations.
func (j Journey) Validate() error {
	if j.SchemaVersion != JourneySchemaVersion || !j.Redacted || j.Scope != "browser-synthetic-markers-v1" ||
		(j.Completeness != Complete && j.Completeness != Partial) || len(j.Observations) > MaxJourneyObservations ||
		len(j.Relationships) > MaxJourneyObservations*16 || len(j.Gaps) > 16 || j.Observations == nil || j.Relationships == nil || j.Gaps == nil {
		return errors.New("journey contract is invalid")
	}
	categories := map[string]string{}
	totalMatches := 0
	for i, observation := range j.Observations {
		if observation.Sequence != i+1 {
			return errors.New("journey observation order is invalid")
		}
		if err := validateJourneyObservation(observation); err != nil {
			return err
		}
		for _, match := range observation.Matches {
			if category, exists := categories[match.Marker]; exists && category != match.Category {
				return errors.New("journey marker category changed")
			}
			categories[match.Marker] = match.Category
			totalMatches++
		}
	}
	if totalMatches > maxJourneyMatches {
		return errors.New("journey match limit exceeded")
	}
	for i, gap := range j.Gaps {
		if !JourneyGapKnown(gap) || (i > 0 && j.Gaps[i-1] >= gap) {
			return errors.New("journey visibility gaps are invalid")
		}
		if gap != "unmatched-information" && gap != "server-side-unobservable" && j.Completeness != Partial {
			return errors.New("journey has incomplete visibility")
		}
	}
	seen := map[JourneyRelationship]bool{}
	for _, r := range j.Relationships {
		if r.From < 1 || r.To <= r.From || r.To > len(j.Observations) || seen[r] ||
			r.Kind != "same-marker" || r.State != evidence.Inferred || !numberedLabel(r.Marker, "m", 16) {
			return errors.New("journey relationship is invalid")
		}
		if !journeyHasMarker(j.Observations[r.From-1], r.Marker) || !journeyHasMarker(j.Observations[r.To-1], r.Marker) {
			return errors.New("journey relationship has no supporting match")
		}
		seen[r] = true
	}
	return nil
}

func journeyHasMarker(observation JourneyObservation, marker string) bool {
	return slices.ContainsFunc(observation.Matches, func(match JourneyMatch) bool { return match.Marker == marker })
}

func validateJourneyObservation(o JourneyObservation) error {
	if o.Sequence < 1 || o.Sequence > MaxJourneyObservations || !numberedLabel(o.Destination, "d", 256) ||
		!numberedLabel(o.Reference, "r", MaxJourneyObservations*16) ||
		!slices.Contains([]string{"page", "frame", "worker", "network"}, o.Context) ||
		!slices.Contains([]string{"navigation", "click", "input", "storage-read", "storage-write", "cookie-read", "cookie-write", "worker-send", "worker-receive", "fetch", "xhr", "beacon", "request", "response", "redirect", "websocket-sent", "websocket-received", "location", "blocked"}, o.Kind) ||
		len(o.Matches) > 64 || o.Matches == nil {
		return errors.New("journey observation is invalid")
	}
	seen := map[JourneyMatch]bool{}
	for _, match := range o.Matches {
		if !numberedLabel(match.Marker, "m", 16) || !validField(match.Category) || !slices.Contains([]string{"exact", "url", "base64", "sha256"}, match.Encoding) || seen[match] {
			return errors.New("journey marker match is invalid")
		}
		seen[match] = true
	}
	return nil
}

func numberedLabel(value, prefix string, limit int) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	number, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	return err == nil && number > 0 && number <= limit && value == prefix+strconv.Itoa(number)
}

// CanonicalBytes validates and encodes a journey without changing its ordered evidence.
func (j Journey) CanonicalBytes() ([]byte, error) {
	if err := j.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(j)
	if err != nil || len(data) > maxJourneyBytes {
		return nil, errors.New("journey exceeds size limit")
	}
	return data, nil
}

// SHA256 returns the ordered journey's canonical content identity.
func (j Journey) SHA256() (string, error) {
	data, err := j.CanonicalBytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// DecodeJourney rejects unknown fields, duplicate keys, malformed, oversized,
// and reordered observations. It leaves trace-v1 normalization unchanged.
func DecodeJourney(data []byte) (Journey, error) {
	if len(data) == 0 || len(data) > maxJourneyBytes || !utf8.Valid(data) {
		return Journey{}, errors.New("journey size or encoding is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Journey{}, errors.New("journey JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var journey Journey
	if err := decoder.Decode(&journey); err != nil {
		return Journey{}, errors.New("journey fields are invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Journey{}, errors.New("journey has trailing data")
	}
	return journey, journey.Validate()
}

// ReadJourney reads and verifies a bounded, non-symlink portable artifact.
func ReadJourney(path string) (Journey, error) {
	data, err := readSourceAdapterFile(path, maxJourneyBytes)
	if err != nil {
		return Journey{}, errors.New("journey file unavailable")
	}
	return DecodeJourney(data)
}

// Trace projects matching observations into trace-v1's existing fixed vocabulary.
// Trace-v1 is a normalized set: ordered evidence remains exclusively in journey.json.
func (j Journey) Trace() (Document, error) {
	if err := j.Validate(); err != nil {
		return Document{}, err
	}
	events := map[string]Event{}
	for _, o := range j.Observations {
		channel, kind := "network", "request"
		switch o.Kind {
		case "storage-write":
			channel, kind = "web-storage", "storage-write"
		case "cookie-write":
			channel, kind = "cookie", "cookie-write"
		case "response", "websocket-received":
			kind = "response"
		case "beacon":
			kind = "beacon"
		case "request", "fetch", "xhr", "websocket-sent":
		default:
			continue
		}
		if len(o.Matches) == 0 {
			continue
		}
		destination := "unknown"
		if o.Destination == "d1" {
			destination = "first-party"
		}
		key := channel + "/" + kind + "/" + destination
		event := events[key]
		event.Source = "browser"
		event.Channel = channel
		event.Kind = kind
		event.Destination = destination
		for _, match := range o.Matches {
			if !slices.Contains(event.Fields, match.Category) {
				event.Fields = append(event.Fields, match.Category)
			}
		}
		events[key] = event
	}
	document := Document{SchemaVersion: 1, Redacted: true, Scope: "all", Completeness: Partial, Events: []Event{}}
	// The legacy trace scope includes unclassified information. A completed
	// synthetic-marker journey never upgrades legacy all-information coverage.
	for _, event := range events {
		document.Events = append(document.Events, event)
	}
	if err := validate(&document); err != nil {
		return Document{}, fmt.Errorf("project journey: %w", err)
	}
	return document, nil
}
