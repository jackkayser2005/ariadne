// Package analysis normalizes and compares captured experiment observations.
package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	maxStorageBytes = 64 << 10
	maxNetworkBytes = 96 << 10
	maxFields       = 64
)

// Session is one normalized pair of agreeing storage and network observations.
type Session struct {
	Fields    map[string]string
	challenge string
}

// HasChallenge reports whether the session carried authenticated protocol metadata.
func (session Session) HasChallenge() bool {
	return session.challenge != ""
}

// ChallengeCommitment returns the raw-value-free commitment for the session challenge.
func (session Session) ChallengeCommitment() string {
	if session.challenge == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(session.challenge))
	return hex.EncodeToString(digest[:])
}

// Comparison records stable, normalized, observed, and unsupported conclusions.
type Comparison struct {
	SchemaVersion    int          `json:"schema_version"`
	UnchangedFields  []string     `json:"unchanged_fields"`
	NormalizedFields []string     `json:"normalized_fields"`
	Differences      []Difference `json:"differences"`
	Unknowns         []Unknown    `json:"unknowns"`
}

// Difference records one added, removed, or changed field between sessions.
type Difference struct {
	// ID is a deterministic identity when the difference is attached to a bundle.
	ID        string         `json:"id,omitempty"`
	Field     string         `json:"field"`
	Kind      string         `json:"kind"`
	Baseline  string         `json:"baseline,omitempty"`
	Treatment string         `json:"treatment,omitempty"`
	State     evidence.State `json:"state"`
	Evidence  []string       `json:"evidence"`
}

// Unknown records a field that the available capture cannot establish.
type Unknown struct {
	// ID is a deterministic identity when the unknown is attached to a bundle.
	ID       string         `json:"id,omitempty"`
	Field    string         `json:"field"`
	State    evidence.State `json:"state"`
	Reason   string         `json:"reason"`
	Evidence []string       `json:"evidence"`
}

// Normalize validates and coalesces one session's storage and network artifacts.
func Normalize(storage, network io.Reader) (Session, error) {
	stored, err := normalizeStorage(storage)
	if err != nil {
		return Session{}, err
	}
	reported, err := NormalizeNetwork(network)
	if err != nil {
		return Session{}, err
	}
	if stored.challenge != reported.challenge || !maps.Equal(stored.Fields, reported.Fields) {
		return Session{}, errors.New("storage and network observations disagree")
	}
	return stored, nil
}

func normalizeStorage(storage io.Reader) (Session, error) {
	storageData, err := readBounded(storage, maxStorageBytes, "storage")
	if err != nil {
		return Session{}, err
	}
	stored, err := decodeObservation(storageData)
	if err != nil {
		return Session{}, fmt.Errorf("storage observation: %w", err)
	}
	return stored, nil
}

// NormalizeNetwork validates and decodes one captured network observation.
func NormalizeNetwork(network io.Reader) (Session, error) {
	networkData, err := readBounded(network, maxNetworkBytes, "network")
	if err != nil {
		return Session{}, err
	}
	var captured collector.Observation
	if err := decodeStrict(
		networkData,
		[]string{"schema_version", "method", "path", "content_type", "body_base64"},
		&captured,
	); err != nil {
		return Session{}, fmt.Errorf("network observation: %w", err)
	}
	if captured.SchemaVersion != 1 {
		return Session{}, errors.New("network observation: unsupported schema_version")
	}
	if captured.Method != "POST" ||
		captured.Path != "/observe" ||
		captured.ContentType != "application/json" {
		return Session{}, errors.New("network observation: unexpected request metadata")
	}

	body, err := base64.StdEncoding.Strict().DecodeString(captured.BodyBase64)
	if err != nil {
		return Session{}, errors.New("network observation: body_base64 is invalid")
	}
	if len(body) > maxStorageBytes {
		return Session{}, errors.New("network observation: decoded body exceeds 65536-byte limit")
	}
	reported, err := decodeObservation(body)
	if err != nil {
		return Session{}, fmt.Errorf("network observation body: %w", err)
	}
	return reported, nil
}

// Compare applies declared volatile fields and returns deterministic conclusions.
func Compare(baseline, treatment Session, volatileFields []string) Comparison {
	comparison := Comparison{
		SchemaVersion:    4,
		UnchangedFields:  make([]string, 0, len(baseline.Fields)),
		NormalizedFields: make([]string, 0, len(volatileFields)),
		Differences:      make([]Difference, 0),
		Unknowns:         make([]Unknown, 0),
	}
	volatile := make(map[string]struct{}, len(volatileFields))
	for _, field := range volatileFields {
		volatile[field] = struct{}{}
	}
	fields := make(map[string]struct{}, len(baseline.Fields)+len(treatment.Fields))
	for field := range baseline.Fields {
		fields[field] = struct{}{}
	}
	for field := range treatment.Fields {
		fields[field] = struct{}{}
	}
	names := slices.Sorted(maps.Keys(fields))
	for _, field := range names {
		baselineValue, baselineOK := baseline.Fields[field]
		treatmentValue, treatmentOK := treatment.Fields[field]
		if _, ignored := volatile[field]; ignored && baselineOK && treatmentOK {
			comparison.NormalizedFields = append(comparison.NormalizedFields, field)
			continue
		}
		if baselineOK && treatmentOK && baselineValue == treatmentValue {
			comparison.UnchangedFields = append(comparison.UnchangedFields, field)
			continue
		}
		comparison.Differences = append(
			comparison.Differences,
			observedDifference(field, baselineValue, treatmentValue, baselineOK, treatmentOK),
		)
	}
	return comparison
}

func observedDifference(
	field, baseline, treatment string,
	baselineOK, treatmentOK bool,
) Difference {
	difference := Difference{
		Field:    field,
		State:    evidence.Observed,
		Evidence: make([]string, 0, 4),
	}
	if baselineOK {
		difference.Baseline = baseline
		difference.Evidence = append(
			difference.Evidence,
			"baseline/observations/storage.json#/"+field,
			"baseline/observations/network.json#decoded-body/"+field,
		)
	}
	if treatmentOK {
		difference.Treatment = treatment
		difference.Evidence = append(
			difference.Evidence,
			"treatment/observations/storage.json#/"+field,
			"treatment/observations/network.json#decoded-body/"+field,
		)
	}
	switch {
	case !baselineOK:
		difference.Kind = "added"
	case !treatmentOK:
		difference.Kind = "removed"
	default:
		difference.Kind = "changed"
	}
	return difference
}

func decodeObservation(data []byte) (Session, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Session{}, errors.New("empty input")
	}
	if !utf8.Valid(data) {
		return Session{}, errors.New("input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Session{}, errors.New("duplicate object key")
	}

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return Session{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Session{}, errors.New("trailing data")
	}

	var schemaVersion int
	if err := json.Unmarshal(raw["schema_version"], &schemaVersion); err != nil ||
		schemaVersion != 1 {
		return Session{}, errors.New("unsupported schema_version")
	}
	delete(raw, "schema_version")
	challenge := ""
	if rawChallenge, ok := raw["challenge"]; ok {
		if err := json.Unmarshal(rawChallenge, &challenge); err != nil || !validChallenge(challenge) {
			return Session{}, errors.New("observation challenge is invalid")
		}
		delete(raw, "challenge")
	}
	if len(raw) == 0 || len(raw) > maxFields {
		return Session{}, fmt.Errorf("expected 1 to %d observation fields", maxFields)
	}

	fields := make(map[string]string, len(raw))
	for _, field := range slices.Sorted(maps.Keys(raw)) {
		encoded := raw[field]
		if !validFieldName(field) {
			return Session{}, errors.New("observation field name is invalid")
		}
		var value string
		if err := json.Unmarshal(encoded, &value); err != nil {
			return Session{}, errors.New("observation field value must be a string")
		}
		if !validFieldValue(value) {
			return Session{}, errors.New("observation field value is invalid")
		}
		fields[field] = value
	}
	return Session{Fields: fields, challenge: challenge}, nil
}

func validChallenge(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func readBounded(reader io.Reader, limit int64, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s observation: read input: %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s observation: exceeds %d-byte limit", name, limit)
	}
	return data, nil
}

func decodeStrict(data []byte, allowed []string, destination any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty input")
	}
	if !utf8.Valid(data) {
		return errors.New("input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err == nil {
		allowedFields := make(map[string]struct{}, len(allowed))
		for _, field := range allowed {
			allowedFields[field] = struct{}{}
		}
		for field := range fields {
			if _, ok := allowedFields[field]; !ok {
				return fmt.Errorf("unknown field %q", field)
			}
		}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func validFieldName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && !strings.ContainsRune("._:-", character) {
			return false
		}
	}
	return true
}

func validFieldValue(value string) bool {
	if value == "" || len(value) > 1024 {
		return false
	}
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && !bytes.ContainsRune([]byte("._@:+-"), character) {
			return false
		}
	}
	return true
}
