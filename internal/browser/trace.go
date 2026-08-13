// Package browser projects an authorized, already-redacted browser audit into
// Ariadne's portable tracking trace contract.
package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	SchemaVersion      = 1
	maxAuditBytes      = 256 << 10
	maxAuditEvents     = 1024
	maxAuditFields     = 64
	browserTraceSource = "browser"
)

// Audit is the bounded input contract emitted by an authorized browser
// driver after it has removed URLs, payloads, cookie values, and identifiers.
// It contains only reviewed category labels.
type Audit struct {
	SchemaVersion int          `json:"schema_version"`
	Redacted      bool         `json:"redacted"`
	Scope         string       `json:"scope"`
	Completeness  string       `json:"completeness"`
	Events        []AuditEvent `json:"events"`
}

// AuditEvent is one redacted browser observation without source-specific data.
type AuditEvent struct {
	Channel     string   `json:"channel"`
	Kind        string   `json:"kind"`
	Destination string   `json:"destination"`
	Fields      []string `json:"fields"`
}

// BuildTrace validates a redacted browser audit and projects it into the
// source-neutral trace contract.
func BuildTrace(data []byte) (portabletrace.Document, error) {
	audit, err := Decode(data)
	if err != nil {
		return portabletrace.Document{}, err
	}
	events := make([]portabletrace.Event, len(audit.Events))
	for index, event := range audit.Events {
		events[index] = portabletrace.Event{
			Source:      browserTraceSource,
			Channel:     event.Channel,
			Kind:        event.Kind,
			Destination: event.Destination,
			Fields:      append([]string(nil), event.Fields...),
		}
	}
	document := portabletrace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         audit.Scope,
		Completeness:  audit.Completeness,
		Events:        events,
	}
	return document, nil
}

// SaveTrace reads a redacted browser audit, writes its portable trace without
// overwriting an existing path, and returns the saved trace identity.
func SaveTrace(inputPath, outputPath string) (portabletrace.VerificationSummary, error) {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" {
		return portabletrace.VerificationSummary{}, errors.New("browser trace paths are required")
	}
	data, err := readBounded(inputPath)
	if err != nil {
		return portabletrace.VerificationSummary{}, fmt.Errorf("browser audit: %w", err)
	}
	document, err := BuildTrace(data)
	if err != nil {
		return portabletrace.VerificationSummary{}, err
	}
	encoded, _ := json.Marshal(document)
	encoded = append(encoded, '\n')
	if err := writeExclusive(outputPath, encoded); err != nil {
		return portabletrace.VerificationSummary{}, fmt.Errorf("browser trace: %w", err)
	}
	summary, err := portabletrace.Verify(outputPath)
	if err != nil {
		return portabletrace.VerificationSummary{}, errors.New("browser trace verification failed")
	}
	return summary, nil
}

// Decode verifies a bounded redacted browser audit without exposing input
// values in errors.
func Decode(data []byte) (Audit, error) {
	if len(data) == 0 || len(data) > maxAuditBytes || !utf8.Valid(data) {
		return Audit{}, errors.New("browser audit is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return Audit{}, errors.New("browser audit is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var audit Audit
	if err := decoder.Decode(&audit); err != nil {
		return Audit{}, errors.New("browser audit is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Audit{}, errors.New("browser audit is invalid")
	}
	if err := validate(&audit); err != nil {
		return Audit{}, err
	}
	return audit, nil
}

func validate(audit *Audit) error {
	if audit.SchemaVersion != SchemaVersion || !audit.Redacted ||
		!validScope(audit.Scope) || !validCompleteness(audit.Completeness) ||
		audit.Events == nil || len(audit.Events) > maxAuditEvents {
		return errors.New("browser audit is invalid")
	}
	for index := range audit.Events {
		event := &audit.Events[index]
		if !validEventShape(event) || event.Fields == nil ||
			len(event.Fields) == 0 || len(event.Fields) > maxAuditFields {
			return errors.New("browser audit is invalid")
		}
		seenFields := make(map[string]struct{}, len(event.Fields))
		for _, field := range event.Fields {
			if !validField(field) {
				return errors.New("browser audit is invalid")
			}
			if _, exists := seenFields[field]; exists {
				return errors.New("browser audit is invalid")
			}
			seenFields[field] = struct{}{}
		}
		event.Fields = slices.Clone(event.Fields)
		slices.Sort(event.Fields)
	}
	seenEvents := make(map[string]struct{}, len(audit.Events))
	for _, event := range audit.Events {
		key := event.Channel + "\x00" + event.Kind + "\x00" + event.Destination
		if _, exists := seenEvents[key]; exists {
			return errors.New("browser audit is invalid")
		}
		seenEvents[key] = struct{}{}
	}
	return nil
}

func validEventShape(event *AuditEvent) bool {
	if !validDestination(event.Destination) {
		return false
	}
	switch event.Channel {
	case "network":
		return event.Kind == "beacon" || event.Kind == "request" || event.Kind == "response"
	case "cookie":
		return event.Kind == "cookie-write"
	case "web-storage":
		return event.Kind == "storage-write"
	default:
		return false
	}
}

func validScope(value string) bool {
	switch value {
	case "all", "inbound", "outbound", "storage":
		return true
	default:
		return false
	}
}

func validCompleteness(value string) bool {
	return value == portabletrace.Complete || value == portabletrace.Partial
}

func validDestination(value string) bool {
	switch value {
	case "advertising", "analytics", "crash-reporting", "first-party", "unknown":
		return true
	default:
		return false
	}
}

func validField(value string) bool {
	switch value {
	case "account-id", "advertising-id", "consent", "cookie-id", "device-id", "email", "ip-address", "location", "phone", "region", "session-id", "unknown", "user-agent":
		return true
	default:
		return false
	}
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("read input")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxAuditBytes+1))
	if err != nil || len(data) > maxAuditBytes {
		return nil, errors.New("read input")
	}
	return data, nil
}

func writeExclusive(path string, data []byte) error {
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
