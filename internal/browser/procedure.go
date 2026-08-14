package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strconv"
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
	// BrowserLocalFixtureProcedureID identifies the deterministic local browser
	// fixture producer. Its target is fixed inside that producer and is not
	// supplied by the procedure.
	BrowserLocalFixtureProcedureID = "browser-local-fixture-v1"
	// BrowserTargetProcedureID identifies the isolated, explicitly authorized
	// HTTPS browser target producer. Its origin is part of the procedure
	// identity and is never copied into the resulting trace.
	BrowserTargetProcedureID = "browser-target-v1"
	maxProcedureBytes        = 16 << 10
)

// Procedure is the bounded, raw-value-free input shared with one capture
// driver. TargetOrigin is present only for the isolated browser-target
// producer; it is an origin allowlist, not a path, selector, script, header,
// payload, profile path, or authorization claim.
type Procedure struct {
	SchemaVersion int    `json:"schema_version"`
	ProcedureID   string `json:"procedure_id"`
	Scope         string `json:"scope"`
	DurationMS    int    `json:"duration_ms"`
	MaxEvents     int    `json:"max_events"`
	TargetOrigin  string `json:"target_origin,omitempty"`
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
		!validProcedureID(procedure.ProcedureID) ||
		!validScope(procedure.Scope) ||
		procedure.DurationMS < minCaptureDurationMS ||
		procedure.DurationMS > maxCaptureDurationMS ||
		procedure.MaxEvents < 1 ||
		procedure.MaxEvents > maxAuditEvents {
		return errors.New("browser procedure is invalid")
	}
	switch procedure.ProcedureID {
	case BrowserTargetProcedureID:
		if !validTargetOrigin(procedure.TargetOrigin) {
			return errors.New("browser procedure is invalid")
		}
	default:
		if procedure.TargetOrigin != "" {
			return errors.New("browser procedure is invalid")
		}
	}
	return nil
}

func validProcedureID(value string) bool {
	return value == BrowserAuditProcedureID || value == BrowserLocalFixtureProcedureID || value == BrowserTargetProcedureID
}

func validTargetOrigin(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return false
	}
	if value != parsed.Scheme+"://"+parsed.Host || parsed.Host != strings.ToLower(parsed.Host) {
		return false
	}
	hostname := parsed.Hostname()
	if hostname == "" || hostname != strings.ToLower(hostname) {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return false
			}
		}
	}
	if port := parsed.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || port != strconv.Itoa(parsedPort) || parsedPort < 1 || parsedPort > 65535 || parsedPort == 443 {
			return false
		}
	}
	return true
}
