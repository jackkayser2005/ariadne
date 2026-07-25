package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

// MaxManifestBytes is the largest manifest Decode accepts.
const MaxManifestBytes = 64 << 10

// Decode reads and validates one manifest from r.
func Decode(r io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxManifestBytes+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest: read input: %w", err)
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("manifest: exceeds %d-byte limit", MaxManifestBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return Manifest{}, errors.New("manifest: empty input")
	}
	if !utf8.Valid(data) {
		return Manifest{}, errors.New("manifest: input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	if err := rejectUnknownTopLevelFields(data); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("manifest: decode: %w", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("manifest: trailing data")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	return manifest, nil
}

func rejectUnknownTopLevelFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		// The main decoder reports malformed input; this check only enforces names.
		return nil
	}

	allowed := map[string]struct{}{
		"schema_version": {},
		"name":           {},
		"variable":       {},
		"baseline":       {},
		"treatment":      {},
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unknown field %q", field)
		}
	}
	return nil
}
