// Package experiment defines Ariadne experiment contracts.
package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// CurrentSchemaVersion is the manifest version supported by this build.
const CurrentSchemaVersion = 2

const maxVolatileFields = 64

// Persona is a flat set of named string values used in one experiment run.
type Persona map[string]string

// UnmarshalJSON rejects persona values that are not JSON strings.
func (p *Persona) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	persona := make(Persona, len(fields))
	for key, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("persona field %q: value must be a string", key)
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("persona field %q: value must be a string: %w", key, err)
		}
		persona[key] = value
	}
	*p = persona
	return nil
}

// Manifest declares the single controlled difference between two personas.
type Manifest struct {
	SchemaVersion  int      `json:"schema_version"`
	Name           string   `json:"name"`
	Variable       string   `json:"variable"`
	Baseline       Persona  `json:"baseline"`
	Treatment      Persona  `json:"treatment"`
	VolatileFields []string `json:"volatile_fields,omitempty"`
}

// Validate reports whether the manifest describes exactly one declared change.
func (m Manifest) Validate() error {
	if m.SchemaVersion < 1 || m.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", m.SchemaVersion)
	}
	if m.SchemaVersion == 1 && len(m.VolatileFields) > 0 {
		return errors.New("volatile_fields: require schema_version 2")
	}
	if err := ValidateVolatileFields(m.VolatileFields); err != nil {
		return err
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name: required")
	}
	if strings.ContainsFunc(m.Name, unicode.IsControl) {
		return errors.New("name: control characters are not allowed")
	}
	if strings.TrimSpace(m.Variable) == "" {
		return errors.New("variable: required")
	}
	if strings.ContainsFunc(m.Variable, unicode.IsControl) {
		return errors.New("variable: control characters are not allowed")
	}
	if len(m.Baseline) == 0 {
		return errors.New("baseline: required")
	}
	if len(m.Treatment) == 0 {
		return errors.New("treatment: required")
	}
	if _, ok := m.Baseline[m.Variable]; !ok {
		return errors.New("baseline: declared variable is missing")
	}
	if _, ok := m.Treatment[m.Variable]; !ok {
		return errors.New("treatment: declared variable is missing")
	}
	if len(m.Baseline) != len(m.Treatment) {
		return errors.New("personas: key sets differ")
	}

	differences := 0
	differingKey := ""
	for key, baselineValue := range m.Baseline {
		if strings.TrimSpace(key) == "" {
			return errors.New("personas: keys must not be blank")
		}
		if strings.ContainsFunc(key, unicode.IsControl) {
			return errors.New("personas: key control characters are not allowed")
		}
		treatmentValue, ok := m.Treatment[key]
		if !ok {
			return errors.New("personas: key sets differ")
		}
		if baselineValue != treatmentValue {
			differences++
			differingKey = key
		}
	}

	if differences != 1 {
		return fmt.Errorf("personas: expected exactly one differing value, found %d", differences)
	}
	if differingKey != m.Variable {
		return errors.New("variable: declared key does not match differing persona key")
	}
	return nil
}

// ValidateVolatileFields validates explicit observation fields that may be normalized.
func ValidateVolatileFields(fields []string) error {
	if len(fields) > maxVolatileFields {
		return fmt.Errorf("volatile_fields: exceeds %d-field limit", maxVolatileFields)
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !validObservationFieldName(field) {
			return errors.New("volatile_fields: field name is invalid")
		}
		if _, exists := seen[field]; exists {
			return errors.New("volatile_fields: duplicate field")
		}
		seen[field] = struct{}{}
	}
	return nil
}

// CanonicalVolatileFields returns a sorted copy of validated volatile fields.
func CanonicalVolatileFields(fields []string) []string {
	return slices.Sorted(slices.Values(fields))
}

func validObservationFieldName(value string) bool {
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
