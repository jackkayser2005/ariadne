package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// CaptureProcedureSchemaVersion is the version of the fixed driver input
	// contract.
	CaptureProcedureSchemaVersion = 1
	// BrowserAuditProcedureID is the first catalogued browser procedure. Its
	// target and interaction are external to Ariadne and are not serialized.
	BrowserAuditProcedureID = "browser-audit-v1"
	maxProcedureBytes       = 16 << 10
)

// Procedure is the bounded, raw-value-free input shared with one capture
// driver. It intentionally has no target URL, profile path, selector,
// JavaScript, header, payload, or authorization claim.
type Procedure struct {
	SchemaVersion int    `json:"schema_version"`
	ProcedureID   string `json:"procedure_id"`
	Scope         string `json:"scope"`
	DurationMS    int    `json:"duration_ms"`
	MaxEvents     int    `json:"max_events"`
}

// DecodeProcedure validates one bounded browser capture procedure.
func DecodeProcedure(data []byte) (Procedure, error) {
	if len(data) == 0 || len(data) > maxProcedureBytes || !utf8.Valid(data) {
		return Procedure{}, errors.New("browser procedure is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Procedure{}, errors.New("browser procedure is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var procedure Procedure
	if err := decoder.Decode(&procedure); err != nil {
		return Procedure{}, errors.New("browser procedure is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Procedure{}, errors.New("browser procedure is invalid")
	}
	if err := validateProcedure(procedure); err != nil {
		return Procedure{}, err
	}
	return procedure, nil
}

// ReadProcedure reads and validates one bounded procedure file.
func ReadProcedure(path string) (Procedure, []byte, error) {
	if strings.TrimSpace(path) == "" {
		return Procedure{}, nil, errors.New("browser procedure path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return Procedure{}, nil, errors.New("read procedure")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxProcedureBytes+1))
	if err != nil || len(data) > maxProcedureBytes {
		return Procedure{}, nil, errors.New("read procedure")
	}
	procedure, err := DecodeProcedure(data)
	if err != nil {
		return Procedure{}, nil, err
	}
	return procedure, data, nil
}

// ProcedureSHA256 returns the canonical identity of a validated procedure.
func ProcedureSHA256(procedure Procedure) (string, error) {
	if err := validateProcedure(procedure); err != nil {
		return "", err
	}
	return procedureDigest(procedure)
}

func validateProcedure(procedure Procedure) error {
	if procedure.SchemaVersion != CaptureProcedureSchemaVersion ||
		procedure.ProcedureID != BrowserAuditProcedureID ||
		!validScope(procedure.Scope) ||
		procedure.DurationMS < minCaptureDurationMS ||
		procedure.DurationMS > maxCaptureDurationMS ||
		procedure.MaxEvents < 1 ||
		procedure.MaxEvents > maxAuditEvents {
		return errors.New("browser procedure is invalid")
	}
	return nil
}
