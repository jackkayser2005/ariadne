// Package provenance defines the small canonical identity shared by Ariadne
// source adapters and their reviewed procedures.
package provenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// SchemaVersion is the version of the canonical provenance contract.
	SchemaVersion    = 1
	maxContractBytes = 16 << 10
	maxSourceBytes   = 64
	maxAdapterBytes  = 128
	maxScopeBytes    = 64
)

// Contract is the stable provenance intersection shared by reviewed source
// adapters, procedures, traces, sessions, and replications. It contains no
// captured values, credentials, device serials, or source-specific paths.
type Contract struct {
	SchemaVersion   int    `json:"schema_version"`
	Source          string `json:"source"`
	Adapter         string `json:"adapter"`
	AdapterVersion  int    `json:"adapter_version"`
	ProcedureSHA256 string `json:"procedure_sha256"`
	Scope           string `json:"scope"`
}

// Validate reports whether the contract contains only bounded, canonical
// identity fields.
func (contract Contract) Validate() error {
	if contract.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", contract.SchemaVersion)
	}
	if !validLabel(contract.Source, maxSourceBytes) {
		return errors.New("source: invalid")
	}
	if !validLabel(contract.Adapter, maxAdapterBytes) {
		return errors.New("adapter: invalid")
	}
	if contract.AdapterVersion < 1 || contract.AdapterVersion > 32 {
		return errors.New("adapter_version: invalid")
	}
	if !validSHA256(contract.ProcedureSHA256) {
		return errors.New("procedure_sha256: invalid")
	}
	if !validLabel(contract.Scope, maxScopeBytes) {
		return errors.New("scope: invalid")
	}
	return nil
}

// CanonicalBytes returns deterministic JSON for a valid contract. Struct
// fields are deliberately used instead of maps so equivalent input formatting
// cannot change the identity.
func (contract Contract) CanonicalBytes() ([]byte, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(contract)
	if err != nil {
		return nil, fmt.Errorf("encode provenance contract: %w", err)
	}
	return data, nil
}

// SHA256 returns the canonical identity of a valid contract.
func (contract Contract) SHA256() (string, error) {
	data, err := contract.CanonicalBytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// Decode reads one bounded canonical contract from JSON. Decoding accepts
// ordinary whitespace but rejects duplicate, unknown, and trailing fields.
func Decode(data []byte) (Contract, error) {
	if len(data) == 0 || len(data) > maxContractBytes || !utf8.Valid(data) {
		return Contract{}, errors.New("provenance contract is invalid")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Contract{}, errors.New("provenance contract is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Contract{}, errors.New("provenance contract is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var contract Contract
	if err := decoder.Decode(&contract); err != nil {
		return Contract{}, errors.New("provenance contract is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Contract{}, errors.New("provenance contract is invalid")
	}
	if err := contract.Validate(); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// Equivalent reports whether two validated contracts describe the same
// reviewed adapter boundary.
func Equivalent(left, right Contract) bool {
	return left == right
}

func validLabel(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && !strings.ContainsRune("._:/-", character) {
			return false
		}
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
