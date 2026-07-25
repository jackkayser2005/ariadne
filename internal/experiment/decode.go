package experiment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
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
	if err := rejectDuplicateKeys(data); err != nil {
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

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	err := scanJSONValue(decoder)
	if errors.Is(err, io.EOF) {
		return errors.New("invalid JSON: unexpected end of input")
	}
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate key %q", key)
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected closing delimiter")
	}

	_, err = decoder.Token()
	return err
}
