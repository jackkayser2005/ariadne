package adb

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// AndroidProcedureSchemaVersion identifies the canonical Android procedure contract.
	AndroidProcedureSchemaVersion = 1
	// AndroidProcedureID identifies the reviewed Experiment 001 execution procedure.
	AndroidProcedureID = "android-experiment-001-authenticated"
	// AndroidProcedureVersion identifies the reviewed implementation of the procedure.
	AndroidProcedureVersion = 1
	// AndroidProcedureInputBoundary identifies the app-private fixture input boundary.
	AndroidProcedureInputBoundary = "app-private-run-as-file-v1"
)

// ProcedureContract is the raw-value-free identity of the reviewed Android
// execution procedure. It is independent of the experiment manifest.
type ProcedureContract struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Version       int    `json:"version"`
	InputBoundary string `json:"input_boundary"`
	CaptureScope  string `json:"capture_scope"`
	ResetPolicy   string `json:"reset_policy"`
}

// CurrentAndroidProcedure returns the reviewed procedure used by authenticated
// Experiment 001 sessions.
func CurrentAndroidProcedure() ProcedureContract {
	return ProcedureContract{
		SchemaVersion: AndroidProcedureSchemaVersion,
		ID:            AndroidProcedureID,
		Version:       AndroidProcedureVersion,
		InputBoundary: AndroidProcedureInputBoundary,
		CaptureScope:  ReplicationScope,
		ResetPolicy:   ReplicationResetPolicy,
	}
}

// Validate reports whether the procedure contract is bounded and canonical.
func (procedure ProcedureContract) Validate() error {
	if procedure.SchemaVersion != AndroidProcedureSchemaVersion ||
		!validProcedureLabel(procedure.ID, 128) ||
		procedure.Version < 1 || procedure.Version > 32 ||
		!validProcedureLabel(procedure.InputBoundary, 128) ||
		!validProcedureLabel(procedure.CaptureScope, 64) ||
		!validProcedureLabel(procedure.ResetPolicy, 128) {
		return errors.New("android procedure contract is invalid")
	}
	return nil
}

// CanonicalBytes returns deterministic JSON for a valid procedure contract.
func (procedure ProcedureContract) CanonicalBytes() ([]byte, error) {
	if err := procedure.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(procedure)
	if err != nil {
		return nil, fmt.Errorf("encode android procedure: %w", err)
	}
	return data, nil
}

// SHA256 returns the canonical identity of a valid procedure contract.
func (procedure ProcedureContract) SHA256() (string, error) {
	data, err := procedure.CanonicalBytes()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// AndroidProcedureSHA256 returns the canonical identity of the current
// reviewed Android execution procedure.
func AndroidProcedureSHA256() (string, error) {
	return CurrentAndroidProcedure().SHA256()
}

func validProcedureLabel(value string, limit int) bool {
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
