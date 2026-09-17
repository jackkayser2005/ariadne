package bundle

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	experimentTraceScope         = "all"
	experimentTraceSchemaVersion = 1
)

// BuildExperimentTrace verifies one Experiment 001 run and projects one
// selected Android session into the portable raw-value-free trace contract.
// The fixed mapping recognizes only the fixture's known observation shape.
func BuildExperimentTrace(runDir, session string) (portabletrace.Document, error) {
	if session != "baseline" && session != "treatment" {
		return portabletrace.Document{}, errors.New("experiment trace session must be baseline or treatment")
	}
	if _, _, err := verifyDocument(runDir); err != nil {
		return portabletrace.Document{}, fmt.Errorf("experiment trace: verify run: %w", err)
	}
	loaded, err := loadSession(runDir, session)
	if err != nil {
		return portabletrace.Document{}, fmt.Errorf("experiment trace: load %s session: %w", session, err)
	}

	networkFields, err := experimentTraceNetworkFields(loaded.network)
	if err != nil {
		return portabletrace.Document{}, fmt.Errorf("experiment trace: %s network: %w", session, err)
	}
	events := []portabletrace.Event{{
		Source:      "android",
		Channel:     "network",
		Kind:        "request",
		Destination: "first-party",
		Fields:      slices.Clone(networkFields),
	}}
	completeness := portabletrace.Complete
	if !sessionComplete(loaded.record) {
		completeness = portabletrace.Partial
	} else {
		storageFields, err := experimentTraceObservationFields(loaded.storage)
		if err != nil {
			return portabletrace.Document{}, fmt.Errorf("experiment trace: %s storage: %w", session, err)
		}
		if !slices.Equal(networkFields, storageFields) {
			return portabletrace.Document{}, errors.New("experiment trace: network and storage fields disagree")
		}
		events = append(events, portabletrace.Event{
			Source:      "android",
			Channel:     "app-storage",
			Kind:        "storage-write",
			Destination: "first-party",
			Fields:      slices.Clone(storageFields),
		})
	}

	document := portabletrace.Document{
		SchemaVersion: experimentTraceSchemaVersion,
		Redacted:      true,
		Scope:         experimentTraceScope,
		Completeness:  completeness,
		Events:        events,
	}
	if _, err := portabletrace.SHA256(document); err != nil {
		return portabletrace.Document{}, fmt.Errorf("experiment trace: %w", err)
	}
	return document, nil
}

// SaveExperimentTrace verifies one Experiment 001 run, writes its selected
// session trace without overwriting an existing path, and returns its trace
// identity.
func SaveExperimentTrace(runDir, session, tracePath string) (portabletrace.VerificationSummary, error) {
	if strings.TrimSpace(tracePath) == "" {
		return portabletrace.VerificationSummary{}, errors.New("experiment trace path is required")
	}
	document, err := BuildExperimentTrace(runDir, session)
	if err != nil {
		return portabletrace.VerificationSummary{}, err
	}
	data, err := json.Marshal(document)
	if err != nil {
		return portabletrace.VerificationSummary{}, fmt.Errorf("experiment trace: encode: %w", err)
	}
	if err := writeExclusive(tracePath, append(data, '\n')); err != nil {
		return portabletrace.VerificationSummary{}, err
	}
	summary, err := portabletrace.Verify(tracePath)
	if err != nil {
		return portabletrace.VerificationSummary{}, fmt.Errorf("experiment trace: verify saved trace: %w", err)
	}
	return summary, nil
}

type experimentTraceNetworkObservation struct {
	SchemaVersion int    `json:"schema_version"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	ContentType   string `json:"content_type"`
	BodyBase64    string `json:"body_base64"`
}

func experimentTraceNetworkFields(data []byte) ([]string, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return nil, errors.New("network observation is unsupported")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var captured experimentTraceNetworkObservation
	if err := decoder.Decode(&captured); err != nil {
		return nil, errors.New("network observation is unsupported")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("network observation is unsupported")
	}
	if captured.SchemaVersion != 1 || captured.Method != "POST" || captured.Path != "/observe" || captured.ContentType != "application/json" {
		return nil, errors.New("network observation is unsupported")
	}
	body, err := base64.StdEncoding.Strict().DecodeString(captured.BodyBase64)
	if err != nil {
		return nil, errors.New("network observation is unsupported")
	}
	return experimentTraceObservationFields(body)
}

func experimentTraceObservationFields(data []byte) ([]string, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return nil, errors.New("observation fields are unsupported")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("observation fields are unsupported")
	}
	var schemaVersion int
	if err := json.Unmarshal(raw["schema_version"], &schemaVersion); err != nil || schemaVersion != 1 {
		return nil, errors.New("observation fields are unsupported")
	}
	fields := make([]string, 0, 2)
	for field := range raw {
		switch field {
		case "schema_version", "variant", "challenge":
			// Variant and challenge are experiment protocol metadata, not
			// tracking categories, so they are intentionally absent from traces.
		case "region":
			fields = append(fields, "region")
		case "request_id":
			fields = append(fields, "session-id")
		default:
			return nil, errors.New("observation fields are unsupported")
		}
	}
	slices.Sort(fields)
	if !slices.Equal(fields, []string{"region", "session-id"}) {
		return nil, errors.New("observation fields are unsupported")
	}
	return fields, nil
}
