// Package trace defines the portable, raw-value-free tracking trace contract.
package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	schemaVersion    = 1
	maxDocumentBytes = 256 << 10
	maxEvents        = 1024
	maxFields        = 64
)

const (
	// Complete means the source adapter covered the declared trace scope.
	Complete = "complete"
	// Partial means absence from the trace cannot establish absence from the source.
	Partial = "partial"
)

// Event is the safe identity of one tracking observation. It contains labels,
// not payloads, URLs, identifiers, or captured values.
type Event struct {
	Source      string   `json:"source"`
	Channel     string   `json:"channel"`
	Kind        string   `json:"kind"`
	Destination string   `json:"destination"`
	Fields      []string `json:"fields"`
}

// Document is a source-neutral, raw-value-free trace produced by an authorized
// source adapter after it has removed payloads and source-specific identifiers.
type Document struct {
	SchemaVersion int     `json:"schema_version"`
	Redacted      bool    `json:"redacted"`
	Scope         string  `json:"scope"`
	Completeness  string  `json:"completeness"`
	Events        []Event `json:"events"`
}

// VerificationSummary identifies a structurally valid trace without exposing
// any source payload.
type VerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	Redacted      bool   `json:"redacted"`
	Scope         string `json:"scope"`
	Completeness  string `json:"completeness"`
	Events        int    `json:"events"`
	TraceSHA256   string `json:"trace_sha256"`
}

// EventChange is a safe structural difference between two trace events.
type EventChange struct {
	Source          string         `json:"source"`
	Channel         string         `json:"channel"`
	Kind            string         `json:"kind"`
	Destination     string         `json:"destination"`
	KindOfChange    string         `json:"change"`
	BaselineFields  []string       `json:"baseline_fields,omitempty"`
	TreatmentFields []string       `json:"treatment_fields,omitempty"`
	State           evidence.State `json:"state"`
}

// Unknown records where incomplete source coverage prevents a safe absence
// conclusion.
type Unknown struct {
	Source      string         `json:"source"`
	Channel     string         `json:"channel"`
	Kind        string         `json:"kind"`
	Destination string         `json:"destination"`
	State       evidence.State `json:"state"`
	Reason      string         `json:"reason"`
}

// Comparison is the raw-value-free structural comparison of two traces.
type Comparison struct {
	SchemaVersion         int           `json:"schema_version"`
	Scope                 string        `json:"scope"`
	BaselineCompleteness  string        `json:"baseline_completeness"`
	TreatmentCompleteness string        `json:"treatment_completeness"`
	BaselineTraceSHA256   string        `json:"baseline_trace_sha256"`
	TreatmentTraceSHA256  string        `json:"treatment_trace_sha256"`
	Unchanged             []Event       `json:"unchanged"`
	Differences           []EventChange `json:"differences"`
	Unknowns              []Unknown     `json:"unknowns"`
}

// ReplicatedOutcome classifies repeated counterfactual pairs independently
// from the evidence state reported for their traces.
type ReplicatedOutcome string

const (
	// ReplicatedChange means every complete pair observed a difference.
	ReplicatedChange ReplicatedOutcome = "replicated-change"
	// NoChangeObserved means every complete pair observed no difference.
	NoChangeObserved ReplicatedOutcome = "no-change-observed"
	// MixedInconsistent means complete pairs disagree about whether a difference occurred.
	MixedInconsistent ReplicatedOutcome = "mixed-inconsistent"
	// ReplicationUnknown means a reset, capture, or pair verification is incomplete.
	ReplicationUnknown ReplicatedOutcome = "unknown"
)

// ReplicatedPairObservation is the safe result of one verified pair.
type ReplicatedPairObservation struct {
	Differences   int
	Unknowns      int
	EvidenceState evidence.State
}

// ReplicatedClassification is the source-neutral aggregate of repeated pairs.
type ReplicatedClassification struct {
	Outcome       ReplicatedOutcome
	EvidenceState evidence.State
	ChangedPairs  int
	NoChangePairs int
	UnknownPairs  int
}

// ClassifyReplicatedPairs preserves the distinction between an observed
// outcome and an evidence state that cannot support one.
func ClassifyReplicatedPairs(pairs []ReplicatedPairObservation) ReplicatedClassification {
	result := ReplicatedClassification{EvidenceState: evidence.Observed}
	if len(pairs) == 0 {
		result.Outcome = ReplicationUnknown
		result.EvidenceState = evidence.Unknown
		return result
	}
	for _, pair := range pairs {
		switch {
		case pair.Unknowns > 0 || pair.EvidenceState == evidence.Unknown:
			result.UnknownPairs++
		case pair.Differences > 0:
			result.ChangedPairs++
		default:
			result.NoChangePairs++
		}
		if pair.EvidenceState != evidence.Observed {
			result.EvidenceState = evidence.Unknown
		}
	}
	switch {
	case result.UnknownPairs > 0:
		result.Outcome = ReplicationUnknown
	case result.ChangedPairs == len(pairs):
		result.Outcome = ReplicatedChange
	case result.NoChangePairs == len(pairs):
		result.Outcome = NoChangeObserved
	default:
		result.Outcome = MixedInconsistent
	}
	return result
}

// Read verifies one trace file and returns its normalized document.
func Read(path string) (Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, fmt.Errorf("read trace: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("read trace: %w", err)
	}
	document, err := Decode(data)
	if err != nil {
		return Document{}, err
	}
	return document, nil
}

// Decode verifies one trace document and normalizes event and field order.
func Decode(data []byte) (Document, error) {
	if len(data) == 0 {
		return Document{}, errors.New("trace is empty")
	}
	if len(data) > maxDocumentBytes {
		return Document{}, errors.New("trace exceeds 262144-byte limit")
	}
	if !utf8.Valid(data) {
		return Document{}, errors.New("trace must be valid UTF-8")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Document{}, errors.New("trace must be a JSON object")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Document{}, errors.New("trace has invalid JSON structure")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, errors.New("trace has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("trace has trailing data")
	}
	if err := validate(&document); err != nil {
		return Document{}, err
	}
	return document, nil
}

// Verify checks a trace file and returns its canonical content identity.
func Verify(path string) (VerificationSummary, error) {
	document, err := Read(path)
	if err != nil {
		return VerificationSummary{}, err
	}
	digest, err := SHA256(document)
	if err != nil {
		return VerificationSummary{}, err
	}
	return VerificationSummary{
		SchemaVersion: document.SchemaVersion,
		Redacted:      document.Redacted,
		Scope:         document.Scope,
		Completeness:  document.Completeness,
		Events:        len(document.Events),
		TraceSHA256:   digest,
	}, nil
}

// SHA256 returns the identity of a normalized trace document.
func SHA256(document Document) (string, error) {
	if err := validate(&document); err != nil {
		return "", err
	}
	data, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode trace: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// ValidSHA256 reports whether value is a lowercase hexadecimal SHA-256 digest.
func ValidSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

// CompareFiles verifies and compares two traces with the same declared scope.
func CompareFiles(baselinePath, treatmentPath string) (Comparison, error) {
	baseline, err := Read(baselinePath)
	if err != nil {
		return Comparison{}, fmt.Errorf("baseline trace: %w", err)
	}
	treatment, err := Read(treatmentPath)
	if err != nil {
		return Comparison{}, fmt.Errorf("treatment trace: %w", err)
	}
	return Compare(baseline, treatment)
}

// Compare returns only structural tracking changes. It never compares or
// emits payload values because the trace contract does not carry them.
func Compare(baseline, treatment Document) (Comparison, error) {
	if err := validate(&baseline); err != nil {
		return Comparison{}, fmt.Errorf("baseline trace: %w", err)
	}
	if err := validate(&treatment); err != nil {
		return Comparison{}, fmt.Errorf("treatment trace: %w", err)
	}
	if baseline.Scope != treatment.Scope {
		return Comparison{}, errors.New("trace scopes disagree")
	}
	baselineSHA256, err := SHA256(baseline)
	if err != nil {
		return Comparison{}, err
	}
	treatmentSHA256, err := SHA256(treatment)
	if err != nil {
		return Comparison{}, err
	}

	baselineEvents := indexEvents(baseline.Events)
	treatmentEvents := indexEvents(treatment.Events)
	keys := make(map[eventKey]struct{}, len(baselineEvents)+len(treatmentEvents))
	for key := range baselineEvents {
		keys[key] = struct{}{}
	}
	for key := range treatmentEvents {
		keys[key] = struct{}{}
	}
	sortedKeys := slices.SortedFunc(maps.Keys(keys), compareEventKeys)
	comparison := Comparison{
		SchemaVersion:         schemaVersion,
		Scope:                 baseline.Scope,
		BaselineCompleteness:  baseline.Completeness,
		TreatmentCompleteness: treatment.Completeness,
		BaselineTraceSHA256:   baselineSHA256,
		TreatmentTraceSHA256:  treatmentSHA256,
		Unchanged:             make([]Event, 0),
		Differences:           make([]EventChange, 0),
		Unknowns:              make([]Unknown, 0),
	}
	for _, key := range sortedKeys {
		baselineEvent, baselineOK := baselineEvents[key]
		treatmentEvent, treatmentOK := treatmentEvents[key]
		switch {
		case baselineOK && treatmentOK:
			if slices.Equal(baselineEvent.Fields, treatmentEvent.Fields) {
				comparison.Unchanged = append(comparison.Unchanged, baselineEvent)
				continue
			}
			if baseline.Completeness != Complete || treatment.Completeness != Complete {
				comparison.Unknowns = append(comparison.Unknowns, Unknown{
					Source:      key.Source,
					Channel:     key.Channel,
					Kind:        key.Kind,
					Destination: key.Destination,
					State:       evidence.Unknown,
					Reason:      "partial trace coverage does not establish field-set change",
				})
				continue
			}
			comparison.Differences = append(comparison.Differences, EventChange{
				Source:          key.Source,
				Channel:         key.Channel,
				Kind:            key.Kind,
				Destination:     key.Destination,
				KindOfChange:    "changed",
				BaselineFields:  slices.Clone(baselineEvent.Fields),
				TreatmentFields: slices.Clone(treatmentEvent.Fields),
				State:           evidence.Observed,
			})
		case baseline.Completeness == Complete && treatment.Completeness == Complete:
			change := EventChange{
				Source:      key.Source,
				Channel:     key.Channel,
				Kind:        key.Kind,
				Destination: key.Destination,
				State:       evidence.Observed,
			}
			if baselineOK {
				change.KindOfChange = "removed"
				change.BaselineFields = slices.Clone(baselineEvent.Fields)
			} else {
				change.KindOfChange = "added"
				change.TreatmentFields = slices.Clone(treatmentEvent.Fields)
			}
			comparison.Differences = append(comparison.Differences, change)
		default:
			comparison.Unknowns = append(comparison.Unknowns, Unknown{
				Source:      key.Source,
				Channel:     key.Channel,
				Kind:        key.Kind,
				Destination: key.Destination,
				State:       evidence.Unknown,
				Reason:      "partial trace coverage does not establish event absence",
			})
		}
	}
	return comparison, nil
}

type eventKey struct {
	Source      string
	Channel     string
	Kind        string
	Destination string
}

func indexEvents(events []Event) map[eventKey]Event {
	indexed := make(map[eventKey]Event, len(events))
	for _, event := range events {
		indexed[eventKey{
			Source:      event.Source,
			Channel:     event.Channel,
			Kind:        event.Kind,
			Destination: event.Destination,
		}] = event
	}
	return indexed
}

func compareEventKeys(left, right eventKey) int {
	for _, pair := range [][2]string{
		{left.Source, right.Source},
		{left.Channel, right.Channel},
		{left.Kind, right.Kind},
		{left.Destination, right.Destination},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func validate(document *Document) error {
	if document.SchemaVersion != schemaVersion {
		return errors.New("trace has unsupported schema_version")
	}
	if !document.Redacted {
		return errors.New("trace must be marked redacted")
	}
	if !validScope(document.Scope) {
		return errors.New("trace scope is invalid")
	}
	if document.Completeness != Complete && document.Completeness != Partial {
		return errors.New("trace completeness is invalid")
	}
	if document.Events == nil || len(document.Events) > maxEvents {
		return errors.New("trace events are invalid")
	}
	seen := make(map[eventKey]struct{}, len(document.Events))
	for index := range document.Events {
		event := &document.Events[index]
		if !validSource(event.Source) || !validChannel(event.Channel) ||
			!validKind(event.Kind) || !validDestination(event.Destination) {
			return errors.New("trace event identity is invalid")
		}
		if event.Fields == nil || len(event.Fields) == 0 || len(event.Fields) > maxFields {
			return errors.New("trace event fields are invalid")
		}
		fieldSet := make(map[string]struct{}, len(event.Fields))
		for _, field := range event.Fields {
			if !validField(field) {
				return errors.New("trace event field is invalid")
			}
			if _, exists := fieldSet[field]; exists {
				return errors.New("trace event fields contain duplicates")
			}
			fieldSet[field] = struct{}{}
		}
		event.Fields = slices.Sorted(maps.Keys(fieldSet))
		key := eventKey{
			Source:      event.Source,
			Channel:     event.Channel,
			Kind:        event.Kind,
			Destination: event.Destination,
		}
		if _, exists := seen[key]; exists {
			return errors.New("trace events contain duplicate identities")
		}
		seen[key] = struct{}{}
	}
	slices.SortFunc(document.Events, func(left, right Event) int {
		return compareEventKeys(eventKey{
			Source:      left.Source,
			Channel:     left.Channel,
			Kind:        left.Kind,
			Destination: left.Destination,
		}, eventKey{
			Source:      right.Source,
			Channel:     right.Channel,
			Kind:        right.Kind,
			Destination: right.Destination,
		})
	})
	return nil
}

// ponytail: fixed safe catalog until a source can prove a new label is not a
// raw identifier; add reviewed labels here instead of accepting arbitrary text.
func validScope(value string) bool {
	switch value {
	case "all", "inbound", "outbound", "storage":
		return true
	default:
		return false
	}
}

func validSource(value string) bool {
	switch value {
	case "android", "browser", "desktop", "proxy":
		return true
	default:
		return false
	}
}

func validChannel(value string) bool {
	switch value {
	case "app-storage", "cookie", "network", "web-storage":
		return true
	default:
		return false
	}
}

func validKind(value string) bool {
	switch value {
	case "beacon", "cookie-write", "request", "response", "storage-write":
		return true
	default:
		return false
	}
}

func validDestination(value string) bool {
	switch value {
	case "advertising", "analytics", "crash-reporting", "first-party", "unknown":
		return true
	default:
		return false
	}
}

func validField(value string) bool {
	switch value {
	case "account-id", "advertising-id", "consent", "cookie-id", "device-id", "email", "ip-address", "location", "phone", "region", "session-id", "unknown", "user-agent":
		return true
	default:
		return false
	}
}
