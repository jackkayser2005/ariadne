// Package analysis normalizes and compares captured experiment observations.
package analysis

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/collector"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	maxStorageBytes = 64 << 10
	maxNetworkBytes = 96 << 10
)

// Session is one normalized pair of agreeing storage and network observations.
type Session struct {
	Region  string
	Variant string
}

// Comparison records stable fields and observed counterfactual differences.
type Comparison struct {
	SchemaVersion   int          `json:"schema_version"`
	UnchangedFields []string     `json:"unchanged_fields"`
	Differences     []Difference `json:"differences"`
}

// Difference records one field whose observed values differ between sessions.
type Difference struct {
	Field     string         `json:"field"`
	Baseline  string         `json:"baseline"`
	Treatment string         `json:"treatment"`
	State     evidence.State `json:"state"`
	Evidence  []string       `json:"evidence"`
}

type fixtureObservation struct {
	SchemaVersion int    `json:"schema_version"`
	Region        string `json:"region"`
	Variant       string `json:"variant"`
}

// Normalize validates and coalesces one session's storage and network artifacts.
func Normalize(storage, network io.Reader) (Session, error) {
	storageData, err := readBounded(storage, maxStorageBytes, "storage")
	if err != nil {
		return Session{}, err
	}
	networkData, err := readBounded(network, maxNetworkBytes, "network")
	if err != nil {
		return Session{}, err
	}

	var stored fixtureObservation
	if err := decodeStrict(
		storageData,
		[]string{"schema_version", "region", "variant"},
		&stored,
	); err != nil {
		return Session{}, fmt.Errorf("storage observation: %w", err)
	}
	if err := stored.validate(); err != nil {
		return Session{}, fmt.Errorf("storage observation: %w", err)
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
	var reported fixtureObservation
	if err := decodeStrict(
		body,
		[]string{"schema_version", "region", "variant"},
		&reported,
	); err != nil {
		return Session{}, fmt.Errorf("network observation body: %w", err)
	}
	if err := reported.validate(); err != nil {
		return Session{}, fmt.Errorf("network observation body: %w", err)
	}
	if stored != reported {
		return Session{}, errors.New("storage and network observations disagree")
	}

	return Session{Region: stored.Region, Variant: stored.Variant}, nil
}

// Compare returns stable fields and observed differences in deterministic order.
func Compare(baseline, treatment Session) Comparison {
	comparison := Comparison{
		SchemaVersion:   1,
		UnchangedFields: make([]string, 0, 2),
		Differences:     make([]Difference, 0, 2),
	}
	fields := []struct {
		name      string
		baseline  string
		treatment string
	}{
		{name: "region", baseline: baseline.Region, treatment: treatment.Region},
		{name: "variant", baseline: baseline.Variant, treatment: treatment.Variant},
	}
	for _, field := range fields {
		if field.baseline == field.treatment {
			comparison.UnchangedFields = append(comparison.UnchangedFields, field.name)
			continue
		}
		comparison.Differences = append(comparison.Differences, Difference{
			Field:     field.name,
			Baseline:  field.baseline,
			Treatment: field.treatment,
			State:     evidence.Observed,
			Evidence: []string{
				"baseline/observations/storage.json#/" + field.name,
				"baseline/observations/network.json#decoded-body/" + field.name,
				"treatment/observations/storage.json#/" + field.name,
				"treatment/observations/network.json#decoded-body/" + field.name,
			},
		})
	}
	return comparison
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

func (observation fixtureObservation) validate() error {
	if observation.SchemaVersion != 1 {
		return errors.New("unsupported schema_version")
	}
	if !validFieldValue(observation.Region) {
		return errors.New("region is invalid")
	}
	if !validFieldValue(observation.Variant) {
		return errors.New("variant is invalid")
	}
	return nil
}

func validFieldValue(value string) bool {
	return value != "" &&
		len(value) <= 1024 &&
		strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
