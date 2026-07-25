// Package experiment defines Ariadne experiment contracts.
package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CurrentSchemaVersion is the manifest version supported by this build.
const CurrentSchemaVersion = 1

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
	SchemaVersion int     `json:"schema_version"`
	Name          string  `json:"name"`
	Variable      string  `json:"variable"`
	Baseline      Persona `json:"baseline"`
	Treatment     Persona `json:"treatment"`
}

// Validate reports whether the manifest describes exactly one declared change.
func (m Manifest) Validate() error {
	if m.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", m.SchemaVersion)
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("name: required")
	}
	if strings.TrimSpace(m.Variable) == "" {
		return errors.New("variable: required")
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
