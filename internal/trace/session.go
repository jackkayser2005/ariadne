package trace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	sessionSchemaVersion = 1
	maxSessionBytes      = 64 << 10
	maxAdapterVersion    = 32
)

const (
	// RoleStandalone identifies a trace that is not one side of a matched pair.
	RoleStandalone = "standalone"
	// RoleBaseline identifies the baseline side of a matched pair.
	RoleBaseline = "baseline"
	// RoleTreatment identifies the treatment side of a matched pair.
	RoleTreatment = "treatment"
)

const (
	// OrderStandalone identifies a trace without a counterfactual pair order.
	OrderStandalone = "standalone"
	// OrderBaselineTreatment identifies a pair run in baseline-then-treatment order.
	OrderBaselineTreatment = "baseline-treatment"
	// OrderTreatmentBaseline identifies a pair run in treatment-then-baseline order.
	OrderTreatmentBaseline = "treatment-baseline"
)

// Session is a raw-value-free provenance envelope for one verified trace.
// It binds the trace to a reviewed adapter and procedure, and optionally to a
// counterfactual pair, without claiming that the adapter's capture is true.
type Session struct {
	SchemaVersion   int    `json:"schema_version"`
	TraceSHA256     string `json:"trace_sha256"`
	Source          string `json:"source"`
	Adapter         string `json:"adapter"`
	AdapterVersion  int    `json:"adapter_version"`
	ProcedureSHA256 string `json:"procedure_sha256"`
	Scope           string `json:"scope"`
	Completeness    string `json:"completeness"`
	Role            string `json:"role"`
	Order           string `json:"order"`
	PairSHA256      string `json:"pair_sha256"`
}

// SessionInput contains the reviewed metadata needed to create a session
// envelope. The adapter catalog supplies the source label.
type SessionInput struct {
	Adapter         string
	AdapterVersion  int
	ProcedureSHA256 string
	Role            string
	Order           string
	PairSHA256      string
}

// SessionPairInput contains the reviewed metadata shared by a matched pair.
// The pair identity is derived from this metadata and both verified trace
// identities.
type SessionPairInput struct {
	Adapter         string
	AdapterVersion  int
	ProcedureSHA256 string
	Scope           string
	Order           string
}

type sessionPairVerifier func(string, string, string, string) (SessionPairVerificationSummary, error)

// SessionVerificationSummary identifies a valid session and the trace it
// binds without exposing captured values.
type SessionVerificationSummary struct {
	SchemaVersion   int    `json:"schema_version"`
	TraceSHA256     string `json:"trace_sha256"`
	Source          string `json:"source"`
	Adapter         string `json:"adapter"`
	AdapterVersion  int    `json:"adapter_version"`
	ProcedureSHA256 string `json:"procedure_sha256"`
	Scope           string `json:"scope"`
	Completeness    string `json:"completeness"`
	Role            string `json:"role"`
	Order           string `json:"order"`
	PairSHA256      string `json:"pair_sha256"`
	SessionSHA256   string `json:"session_sha256"`
}

// SessionPairVerificationSummary identifies two verified, complementary
// counterfactual sessions without claiming that either capture is truthful.
type SessionPairVerificationSummary struct {
	SchemaVersion          int    `json:"schema_version"`
	PairSHA256             string `json:"pair_sha256"`
	Source                 string `json:"source"`
	Adapter                string `json:"adapter"`
	AdapterVersion         int    `json:"adapter_version"`
	ProcedureSHA256        string `json:"procedure_sha256"`
	Scope                  string `json:"scope"`
	Order                  string `json:"order"`
	BaselineTraceSHA256    string `json:"baseline_trace_sha256"`
	TreatmentTraceSHA256   string `json:"treatment_trace_sha256"`
	BaselineCompleteness   string `json:"baseline_completeness"`
	TreatmentCompleteness  string `json:"treatment_completeness"`
	BaselineSessionSHA256  string `json:"baseline_session_sha256"`
	TreatmentSessionSHA256 string `json:"treatment_session_sha256"`
}

// SaveSession verifies a trace, writes its provenance envelope without
// overwriting an existing path, and returns the two safe identities.
func SaveSession(tracePath, sessionPath string, input SessionInput) (SessionVerificationSummary, error) {
	if strings.TrimSpace(tracePath) == "" || strings.TrimSpace(sessionPath) == "" {
		return SessionVerificationSummary{}, errors.New("trace session paths are required")
	}
	if input.Role != RoleStandalone {
		return SessionVerificationSummary{}, errors.New("paired sessions require trace session pair create")
	}
	document, err := Read(tracePath)
	if err != nil {
		return SessionVerificationSummary{}, fmt.Errorf("trace session trace: %w", err)
	}
	traceSHA256, err := SHA256(document)
	if err != nil {
		return SessionVerificationSummary{}, fmt.Errorf("trace session trace: %w", err)
	}
	session, err := newSession(document, traceSHA256, input)
	if err != nil {
		return SessionVerificationSummary{}, err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return SessionVerificationSummary{}, errors.New("trace session encoding failed")
	}
	data = append(data, '\n')
	if err := writeSessionExclusive(sessionPath, data); err != nil {
		return SessionVerificationSummary{}, fmt.Errorf("trace session: %w", err)
	}
	sessionSHA256, err := SessionSHA256(session)
	if err != nil {
		return SessionVerificationSummary{}, errors.New("trace session verification failed")
	}
	return sessionVerificationSummary(session, sessionSHA256), nil
}

// SaveSessionPair verifies two traces, derives their pair identity, writes
// complementary session envelopes, and returns their safe identities.
func SaveSessionPair(baselineTracePath, treatmentTracePath, baselineSessionPath, treatmentSessionPath string, input SessionPairInput) (SessionPairVerificationSummary, error) {
	return saveSessionPair(baselineTracePath, treatmentTracePath, baselineSessionPath, treatmentSessionPath, input, VerifySessionPair)
}

func saveSessionPair(baselineTracePath, treatmentTracePath, baselineSessionPath, treatmentSessionPath string, input SessionPairInput, verify sessionPairVerifier) (SessionPairVerificationSummary, error) {
	if strings.TrimSpace(baselineTracePath) == "" || strings.TrimSpace(treatmentTracePath) == "" ||
		strings.TrimSpace(baselineSessionPath) == "" || strings.TrimSpace(treatmentSessionPath) == "" {
		return SessionPairVerificationSummary{}, errors.New("trace session pair paths are required")
	}
	if filepath.Clean(baselineTracePath) == filepath.Clean(treatmentTracePath) {
		return SessionPairVerificationSummary{}, errors.New("trace session pair trace paths must be distinct")
	}
	if filepath.Clean(baselineSessionPath) == filepath.Clean(treatmentSessionPath) {
		return SessionPairVerificationSummary{}, errors.New("trace session pair output paths must be distinct")
	}
	baselineDocument, baselineTraceSHA256, err := readTraceIdentity(baselineTracePath)
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("baseline trace: %w", err)
	}
	treatmentDocument, treatmentTraceSHA256, err := readTraceIdentity(treatmentTracePath)
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("treatment trace: %w", err)
	}
	if baselineDocument.Scope != treatmentDocument.Scope {
		return SessionPairVerificationSummary{}, errors.New("trace session pair scopes disagree")
	}
	if input.Scope != "" && input.Scope != baselineDocument.Scope {
		return SessionPairVerificationSummary{}, errors.New("trace session pair scope does not match traces")
	}
	pairSHA256, err := SessionPairSHA256(baselineTraceSHA256, treatmentTraceSHA256, SessionPairInput{
		Adapter:         input.Adapter,
		AdapterVersion:  input.AdapterVersion,
		ProcedureSHA256: input.ProcedureSHA256,
		Scope:           baselineDocument.Scope,
		Order:           input.Order,
	})
	if err != nil {
		return SessionPairVerificationSummary{}, err
	}
	baselineSession, err := newSession(baselineDocument, baselineTraceSHA256, SessionInput{
		Adapter:         input.Adapter,
		AdapterVersion:  input.AdapterVersion,
		ProcedureSHA256: input.ProcedureSHA256,
		Role:            RoleBaseline,
		Order:           input.Order,
		PairSHA256:      pairSHA256,
	})
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("baseline session: %w", err)
	}
	treatmentSession, err := newSession(treatmentDocument, treatmentTraceSHA256, SessionInput{
		Adapter:         input.Adapter,
		AdapterVersion:  input.AdapterVersion,
		ProcedureSHA256: input.ProcedureSHA256,
		Role:            RoleTreatment,
		Order:           input.Order,
		PairSHA256:      pairSHA256,
	})
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("treatment session: %w", err)
	}
	baselineData, err := json.Marshal(baselineSession)
	if err != nil {
		return SessionPairVerificationSummary{}, errors.New("baseline session encoding failed")
	}
	treatmentData, err := json.Marshal(treatmentSession)
	if err != nil {
		return SessionPairVerificationSummary{}, errors.New("treatment session encoding failed")
	}
	baselineData = append(baselineData, '\n')
	treatmentData = append(treatmentData, '\n')
	createdBaseline := false
	createdTreatment := false
	keepOutputs := false
	defer func() {
		if keepOutputs {
			return
		}
		if createdTreatment {
			_ = os.Remove(treatmentSessionPath)
		}
		if createdBaseline {
			_ = os.Remove(baselineSessionPath)
		}
	}()
	if err := writeSessionExclusive(baselineSessionPath, baselineData); err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("baseline session: %w", err)
	}
	createdBaseline = true
	if err := writeSessionExclusive(treatmentSessionPath, treatmentData); err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("treatment session: %w", err)
	}
	createdTreatment = true
	summary, err := verify(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath)
	if err != nil {
		return SessionPairVerificationSummary{}, err
	}
	keepOutputs = true
	return summary, nil
}

// VerifySession checks a saved session and confirms that its trace identity,
// source, scope, and completeness still match the supplied trace.
func VerifySession(sessionPath, tracePath string) (SessionVerificationSummary, error) {
	if strings.TrimSpace(sessionPath) == "" || strings.TrimSpace(tracePath) == "" {
		return SessionVerificationSummary{}, errors.New("trace session paths are required")
	}
	_, summary, err := verifySession(sessionPath, tracePath)
	return summary, err
}

// VerifySessionPair verifies a baseline and treatment session, requiring
// complementary roles and matching safe provenance metadata.
func VerifySessionPair(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath string) (SessionPairVerificationSummary, error) {
	if strings.TrimSpace(baselineSessionPath) == "" || strings.TrimSpace(baselineTracePath) == "" ||
		strings.TrimSpace(treatmentSessionPath) == "" || strings.TrimSpace(treatmentTracePath) == "" {
		return SessionPairVerificationSummary{}, errors.New("trace session pair paths are required")
	}
	if filepath.Clean(baselineTracePath) == filepath.Clean(treatmentTracePath) {
		return SessionPairVerificationSummary{}, errors.New("trace session pair trace paths must be distinct")
	}
	baseline, baselineSummary, err := verifySession(baselineSessionPath, baselineTracePath)
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("baseline session: %w", err)
	}
	treatment, treatmentSummary, err := verifySession(treatmentSessionPath, treatmentTracePath)
	if err != nil {
		return SessionPairVerificationSummary{}, fmt.Errorf("treatment session: %w", err)
	}
	if baseline.Role != RoleBaseline || treatment.Role != RoleTreatment {
		return SessionPairVerificationSummary{}, errors.New("session pair roles are not complementary")
	}
	if err := validateSessionPairMetadata(baseline, treatment); err != nil {
		return SessionPairVerificationSummary{}, err
	}
	expectedPairSHA256, err := SessionPairSHA256(baseline.TraceSHA256, treatment.TraceSHA256, SessionPairInput{
		Adapter:         baseline.Adapter,
		AdapterVersion:  baseline.AdapterVersion,
		ProcedureSHA256: baseline.ProcedureSHA256,
		Scope:           baseline.Scope,
		Order:           baseline.Order,
	})
	if err != nil {
		return SessionPairVerificationSummary{}, err
	}
	if baseline.PairSHA256 != expectedPairSHA256 || treatment.PairSHA256 != expectedPairSHA256 {
		return SessionPairVerificationSummary{}, errors.New("session pair identity does not match verified traces")
	}
	return SessionPairVerificationSummary{
		SchemaVersion:          sessionSchemaVersion,
		PairSHA256:             expectedPairSHA256,
		Source:                 baseline.Source,
		Adapter:                baseline.Adapter,
		AdapterVersion:         baseline.AdapterVersion,
		ProcedureSHA256:        baseline.ProcedureSHA256,
		Scope:                  baseline.Scope,
		Order:                  baseline.Order,
		BaselineTraceSHA256:    baseline.TraceSHA256,
		TreatmentTraceSHA256:   treatment.TraceSHA256,
		BaselineCompleteness:   baseline.Completeness,
		TreatmentCompleteness:  treatment.Completeness,
		BaselineSessionSHA256:  baselineSummary.SessionSHA256,
		TreatmentSessionSHA256: treatmentSummary.SessionSHA256,
	}, nil
}

// SessionPairSHA256 returns the canonical identity of a matched pair's
// reviewed metadata and two verified trace identities.
func SessionPairSHA256(baselineTraceSHA256, treatmentTraceSHA256 string, input SessionPairInput) (string, error) {
	source, ok := adapterSource(input.Adapter)
	if !ok {
		return "", errors.New("trace session adapter is invalid")
	}
	if !validAdapterVersion(input.Adapter, input.AdapterVersion) {
		return "", errors.New("trace session adapter_version is invalid")
	}
	if !ValidSHA256(input.ProcedureSHA256) {
		return "", errors.New("trace session procedure_sha256 is invalid")
	}
	if !validScope(input.Scope) {
		return "", errors.New("trace session scope is invalid")
	}
	if input.Order != OrderBaselineTreatment && input.Order != OrderTreatmentBaseline {
		return "", errors.New("trace session pair order is invalid")
	}
	if !ValidSHA256(baselineTraceSHA256) || !ValidSHA256(treatmentTraceSHA256) {
		return "", errors.New("trace session trace identity is invalid")
	}
	identity := struct {
		Domain               string `json:"domain"`
		SchemaVersion        int    `json:"schema_version"`
		Source               string `json:"source"`
		Adapter              string `json:"adapter"`
		AdapterVersion       int    `json:"adapter_version"`
		ProcedureSHA256      string `json:"procedure_sha256"`
		Scope                string `json:"scope"`
		Order                string `json:"order"`
		BaselineTraceSHA256  string `json:"baseline_trace_sha256"`
		TreatmentTraceSHA256 string `json:"treatment_trace_sha256"`
	}{
		Domain:               "ariadne-trace-session-pair-v1",
		SchemaVersion:        sessionSchemaVersion,
		Source:               source,
		Adapter:              input.Adapter,
		AdapterVersion:       input.AdapterVersion,
		ProcedureSHA256:      input.ProcedureSHA256,
		Scope:                input.Scope,
		Order:                input.Order,
		BaselineTraceSHA256:  baselineTraceSHA256,
		TreatmentTraceSHA256: treatmentTraceSHA256,
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode session pair: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateSessionPairMetadata(baseline, treatment Session) error {
	if baseline.Role != RoleBaseline || treatment.Role != RoleTreatment {
		return errors.New("session pair roles are not complementary")
	}
	if baseline.Source != treatment.Source || baseline.Adapter != treatment.Adapter ||
		baseline.AdapterVersion != treatment.AdapterVersion || baseline.ProcedureSHA256 != treatment.ProcedureSHA256 ||
		baseline.Scope != treatment.Scope || baseline.Order != treatment.Order || baseline.PairSHA256 != treatment.PairSHA256 {
		return errors.New("session pair provenance does not match")
	}
	return nil
}

func readTraceIdentity(path string) (Document, string, error) {
	document, err := Read(path)
	if err != nil {
		return Document{}, "", err
	}
	digest, err := SHA256(document)
	if err != nil {
		return Document{}, "", err
	}
	return document, digest, nil
}

func verifySession(sessionPath, tracePath string) (Session, SessionVerificationSummary, error) {
	data, err := readSession(sessionPath)
	if err != nil {
		return Session{}, SessionVerificationSummary{}, fmt.Errorf("trace session: %w", err)
	}
	session, err := DecodeSession(data)
	if err != nil {
		return Session{}, SessionVerificationSummary{}, fmt.Errorf("trace session: %w", err)
	}
	document, err := Read(tracePath)
	if err != nil {
		return Session{}, SessionVerificationSummary{}, fmt.Errorf("trace session trace: %w", err)
	}
	traceSHA256, err := SHA256(document)
	if err != nil {
		return Session{}, SessionVerificationSummary{}, fmt.Errorf("trace session trace: %w", err)
	}
	if err := validateSessionBinding(session, document, traceSHA256); err != nil {
		return Session{}, SessionVerificationSummary{}, err
	}
	sessionSHA256, err := SessionSHA256(session)
	if err != nil {
		return Session{}, SessionVerificationSummary{}, errors.New("trace session verification failed")
	}
	return session, sessionVerificationSummary(session, sessionSHA256), nil
}

// DecodeSession validates a bounded, raw-value-free session envelope.
func DecodeSession(data []byte) (Session, error) {
	if len(data) == 0 {
		return Session{}, errors.New("session is empty")
	}
	if len(data) > maxSessionBytes {
		return Session{}, errors.New("session exceeds 65536-byte limit")
	}
	if !utf8.Valid(data) {
		return Session{}, errors.New("session must be valid UTF-8")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Session{}, errors.New("session must be a JSON object")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Session{}, errors.New("session has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var session Session
	if err := decoder.Decode(&session); err != nil {
		return Session{}, errors.New("session has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Session{}, errors.New("session has trailing data")
	}
	if err := validateSession(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// SessionSHA256 returns the canonical identity of a valid session envelope.
func SessionSHA256(session Session) (string, error) {
	if err := validateSession(session); err != nil {
		return "", err
	}
	data, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("encode session: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func newSession(document Document, traceSHA256 string, input SessionInput) (Session, error) {
	source, ok := adapterSource(input.Adapter)
	if !ok {
		return Session{}, errors.New("trace session adapter is invalid")
	}
	session := Session{
		SchemaVersion:   sessionSchemaVersion,
		TraceSHA256:     traceSHA256,
		Source:          source,
		Adapter:         input.Adapter,
		AdapterVersion:  input.AdapterVersion,
		ProcedureSHA256: input.ProcedureSHA256,
		Scope:           document.Scope,
		Completeness:    document.Completeness,
		Role:            input.Role,
		Order:           input.Order,
		PairSHA256:      input.PairSHA256,
	}
	if err := validateSessionBinding(session, document, traceSHA256); err != nil {
		return Session{}, err
	}
	return session, nil
}

func validateSession(session Session) error {
	if session.SchemaVersion != sessionSchemaVersion {
		return errors.New("session has unsupported schema_version")
	}
	if !ValidSHA256(session.TraceSHA256) {
		return errors.New("session trace_sha256 is invalid")
	}
	source, ok := adapterSource(session.Adapter)
	if !ok || session.Source != source {
		return errors.New("session adapter or source is invalid")
	}
	if !validAdapterVersion(session.Adapter, session.AdapterVersion) {
		return errors.New("session adapter_version is invalid")
	}
	if !ValidSHA256(session.ProcedureSHA256) {
		return errors.New("session procedure_sha256 is invalid")
	}
	if !validScope(session.Scope) {
		return errors.New("session scope is invalid")
	}
	if session.Completeness != Complete && session.Completeness != Partial {
		return errors.New("session completeness is invalid")
	}
	if session.Role != RoleStandalone && session.Role != RoleBaseline && session.Role != RoleTreatment {
		return errors.New("session role is invalid")
	}
	if session.Order != OrderStandalone && session.Order != OrderBaselineTreatment && session.Order != OrderTreatmentBaseline {
		return errors.New("session order is invalid")
	}
	if session.Role == RoleStandalone {
		if session.Order != OrderStandalone || session.PairSHA256 != "" {
			return errors.New("standalone session pair metadata is invalid")
		}
		return nil
	}
	if session.Order == OrderStandalone || !ValidSHA256(session.PairSHA256) {
		return errors.New("paired session metadata is invalid")
	}
	return nil
}

func validateSessionBinding(session Session, document Document, traceSHA256 string) error {
	if err := validateSession(session); err != nil {
		return err
	}
	if session.TraceSHA256 != traceSHA256 {
		return errors.New("session trace identity does not match trace")
	}
	if session.Scope != document.Scope || session.Completeness != document.Completeness {
		return errors.New("session scope or completeness does not match trace")
	}
	for _, event := range document.Events {
		if event.Source != session.Source {
			return errors.New("session source does not match trace events")
		}
	}
	return nil
}

func adapterSource(adapter string) (string, bool) {
	switch adapter {
	case "android-experiment-001":
		return "android", true
	case "browser-redacted-audit":
		return "browser", true
	case "browser-local-fixture":
		return "browser", true
	case "proxy-connect":
		return "proxy", true
	default:
		return "", false
	}
}

func validAdapterVersion(adapter string, version int) bool {
	if version < 1 || version > maxAdapterVersion {
		return false
	}
	return adapter != "proxy-connect" || version == 1
}

func sessionVerificationSummary(session Session, sessionSHA256 string) SessionVerificationSummary {
	return SessionVerificationSummary{
		SchemaVersion:   session.SchemaVersion,
		TraceSHA256:     session.TraceSHA256,
		Source:          session.Source,
		Adapter:         session.Adapter,
		AdapterVersion:  session.AdapterVersion,
		ProcedureSHA256: session.ProcedureSHA256,
		Scope:           session.Scope,
		Completeness:    session.Completeness,
		Role:            session.Role,
		Order:           session.Order,
		PairSHA256:      session.PairSHA256,
		SessionSHA256:   sessionSHA256,
	}
}

func readSession(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("read input")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionBytes+1))
	if err != nil || len(data) > maxSessionBytes {
		return nil, errors.New("read input")
	}
	return data, nil
}

func writeSessionExclusive(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("create output directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create output")
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.New("write output")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync output")
	}
	if err := file.Close(); err != nil {
		return errors.New("close output")
	}
	remove = false
	return nil
}
