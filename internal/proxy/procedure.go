// Package proxy captures bounded, non-MITM CONNECT traffic from one explicitly
// launched, proxy-aware process.
package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// ProcedureSchemaVersion is the version of the proxy procedure contract.
	ProcedureSchemaVersion = 1
	// ConnectProcedureID identifies the loopback-only, non-MITM CONNECT
	// producer.
	ConnectProcedureID = "proxy-connect-v1"
	maxProcedureBytes  = 16 << 10
	minDurationMS      = 100
	maxDurationMS      = 5 * 60 * 1000
	maxEvents          = 1024
	maxAuthorityBytes  = 255
)

// Procedure is the bounded, raw-value-free input shared with one proxy
// capture. The launched executable and its arguments remain outside this
// document so local paths and command details are not retained in its output.
type Procedure struct {
	SchemaVersion   int    `json:"schema_version"`
	ProcedureID     string `json:"procedure_id"`
	Scope           string `json:"scope"`
	DurationMS      int    `json:"duration_ms"`
	MaxEvents       int    `json:"max_events"`
	TargetAuthority string `json:"target_authority"`
}

// DecodeProcedure validates one bounded proxy procedure.
func DecodeProcedure(data []byte) (Procedure, error) {
	if len(data) == 0 || len(data) > maxProcedureBytes || !utf8.Valid(data) {
		return Procedure{}, errors.New("proxy procedure is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Procedure{}, errors.New("proxy procedure is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var procedure Procedure
	if err := decoder.Decode(&procedure); err != nil {
		return Procedure{}, errors.New("proxy procedure is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Procedure{}, errors.New("proxy procedure is invalid")
	}
	if err := validateProcedure(procedure); err != nil {
		return Procedure{}, err
	}
	return procedure, nil
}

// ReadProcedure reads and validates one bounded proxy procedure file.
func ReadProcedure(path string) (Procedure, []byte, error) {
	if strings.TrimSpace(path) == "" {
		return Procedure{}, nil, errors.New("proxy procedure path is required")
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
	data, err := json.Marshal(procedure)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateProcedure(procedure Procedure) error {
	if procedure.SchemaVersion != ProcedureSchemaVersion ||
		procedure.ProcedureID != ConnectProcedureID ||
		procedure.Scope != "outbound" ||
		procedure.DurationMS < minDurationMS ||
		procedure.DurationMS > maxDurationMS ||
		procedure.MaxEvents < 1 ||
		procedure.MaxEvents > maxEvents ||
		!validAuthority(procedure.TargetAuthority) {
		return errors.New("proxy procedure is invalid")
	}
	return nil
}

func validAuthority(value string) bool {
	if value == "" || len(value) > maxAuthorityBytes || strings.TrimSpace(value) != value {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || host == "" || port == "" || host != strings.ToLower(host) || strings.ContainsAny(host, "[]:%") {
		return false
	}
	if net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return false
			}
		}
	}
	parsedPort, err := strconv.Atoi(port)
	return err == nil && port == strconv.Itoa(parsedPort) && parsedPort >= 1 && parsedPort <= 65535 && value == host+":"+port
}
