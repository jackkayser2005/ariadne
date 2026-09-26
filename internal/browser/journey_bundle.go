package browser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	"github.com/jackkayser2005/ariadne/internal/securefs"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

const maxJourneyBundleBytes = 4 << 20

// JourneyReceipt binds the ordered journey to its compatible trace-v1 projection.
// It verifies content integrity, not the truthfulness of the observed website.
type JourneyReceipt struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	JourneySHA256 string `json:"journey_sha256"`
	TraceSHA256   string `json:"trace_sha256"`
}

// JourneyBundle is the complete portable export. It contains no destination
// names, selectors, page addresses, raw payloads, or synthetic marker values.
type JourneyBundle struct {
	SchemaVersion int            `json:"schema_version"`
	Receipt       JourneyReceipt `json:"receipt"`
	Journey       trace.Journey  `json:"journey"`
	Trace         trace.Document `json:"trace"`
}

type privateJourneyContext struct {
	SchemaVersion int                 `json:"schema_version"`
	JourneySHA256 string              `json:"journey_sha256"`
	Destinations  []DestinationName   `json:"destinations"`
	Steps         []InvestigationStep `json:"steps"`
	Trial         *TrialSettings      `json:"trial,omitempty"`
}
type privateJourneyReceipt struct {
	SchemaVersion int    `json:"schema_version"`
	ContextSHA256 string `json:"context_sha256"`
}

// BuildJourneyBundle creates a portable, verified projection from safe observations.
func BuildJourneyBundle(journey trace.Journey) (JourneyBundle, error) {
	journey.LinkMatches()
	document, err := journey.Trace()
	if err != nil {
		return JourneyBundle{}, err
	}
	journeyID, err := journey.SHA256()
	if err != nil {
		return JourneyBundle{}, err
	}
	traceID, err := trace.SHA256(document)
	if err != nil {
		return JourneyBundle{}, err
	}
	return JourneyBundle{SchemaVersion: 1, Receipt: JourneyReceipt{SchemaVersion: 1, Kind: "browser-journey", JourneySHA256: journeyID, TraceSHA256: traceID}, Journey: journey, Trace: document}, nil
}

// Verify checks both identities and independently recomputes the trace projection.
func (bundle JourneyBundle) Verify() error {
	if bundle.SchemaVersion != 1 || bundle.Receipt.SchemaVersion != 1 || bundle.Receipt.Kind != "browser-journey" {
		return errors.New("unsupported investigation bundle")
	}
	journeyID, err := bundle.Journey.SHA256()
	if err != nil || journeyID != bundle.Receipt.JourneySHA256 {
		return errors.New("investigation journey identity does not match")
	}
	traceID, err := trace.SHA256(bundle.Trace)
	if err != nil || traceID != bundle.Receipt.TraceSHA256 {
		return errors.New("investigation trace identity does not match")
	}
	projected, err := bundle.Journey.Trace()
	if err != nil {
		return err
	}
	projectedID, err := trace.SHA256(projected)
	if err != nil || projectedID != traceID {
		return errors.New("investigation trace does not match its journey")
	}
	return nil
}

// PortableJSON validates before encoding only the portable export fields.
func (bundle JourneyBundle) PortableJSON() ([]byte, error) {
	if err := bundle.Verify(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil || len(data) > maxJourneyBundleBytes {
		return nil, errors.New("investigation export exceeds its size limit")
	}
	return data, nil
}

// DecodeJourneyBundle rejects malformed, ambiguous, oversized, or tampered exports.
func DecodeJourneyBundle(data []byte) (JourneyBundle, error) {
	var bundle JourneyBundle
	if err := decodeInvestigationJSON(data, maxJourneyBundleBytes, &bundle); err != nil {
		return bundle, err
	}
	return bundle, bundle.Verify()
}

func decodeInvestigationJSON(data []byte, limit int, target any) error {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) || jsoncheck.RejectDuplicateKeys(data) != nil {
		return errors.New("investigation JSON is invalid or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return errors.New("investigation fields are invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("investigation has trailing data")
	}
	return nil
}

// SaveInvestigation publishes a new directory without replacing any existing
// artifact. Destination context is private and is never embedded in the export.
func SaveInvestigation(path string, result CaptureResult) (JourneyBundle, error) {
	bundle, err := BuildJourneyBundle(result.Journey)
	if err != nil {
		return bundle, err
	}
	private := privateJourneyContext{SchemaVersion: 1, JourneySHA256: bundle.Receipt.JourneySHA256, Destinations: result.Destinations, Steps: result.Steps}
	if result.Trial != nil {
		private.SchemaVersion = 2
		private.Trial = result.Trial
	}
	if err := private.validate(bundle); err != nil {
		return bundle, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return bundle, errors.New("investigation output path is invalid")
	}
	parent := filepath.Dir(absolute)
	if err := securefs.MkdirAll(parent, 0700); err != nil {
		return bundle, errors.New("investigation output directory is unavailable")
	}
	if err := securefs.RequireAbsent(absolute); err != nil {
		return bundle, errors.New("investigation output already exists or is unsafe")
	}
	stage, err := os.MkdirTemp(parent, ".ariadne-investigation-")
	if err != nil {
		return bundle, errors.New("could not create investigation output")
	}
	defer os.RemoveAll(stage)
	privateData, _ := json.Marshal(private)
	digest := sha256.Sum256(privateData)
	files := map[string]any{"journey.json": bundle.Journey, "trace.json": bundle.Trace, "receipt.json": bundle.Receipt, "private-context.json": private, "private-receipt.json": privateJourneyReceipt{SchemaVersion: 1, ContextSHA256: hex.EncodeToString(digest[:])}}
	for name, value := range files {
		data, err := json.Marshal(value)
		if err != nil || len(data) > maxJourneyBundleBytes {
			return bundle, errors.New("investigation output exceeds size limit")
		}
		if err := securefs.WriteExclusiveExistingParent(filepath.Join(stage, name), data, 0600); err != nil {
			return bundle, errors.New("could not write investigation output")
		}
	}
	if err := securefs.PublishDirectoryExclusive(stage, absolute); err != nil {
		return bundle, errors.New("could not publish investigation without replacing existing data")
	}
	return bundle, nil
}

func (private privateJourneyContext) validate(bundle JourneyBundle) error {
	if (private.SchemaVersion != 1 && private.SchemaVersion != 2) || private.JourneySHA256 != bundle.Receipt.JourneySHA256 || len(private.Destinations) == 0 || len(private.Destinations) > 256 || len(private.Steps) > 64 || private.Steps == nil {
		return errors.New("private investigation context is invalid")
	}
	if (private.SchemaVersion == 1 && private.Trial != nil) || (private.SchemaVersion == 2 && (private.Trial == nil || private.Trial.validate() != nil)) {
		return errors.New("private trial settings are invalid")
	}
	seen := map[string]bool{}
	for i, destination := range private.Destinations {
		u, err := InvestigationURL(destination.Origin)
		if err != nil || u.Scheme+"://"+u.Host != destination.Origin || destination.Alias != "d"+strconv.Itoa(i+1) || seen[destination.Origin] {
			return errors.New("private destination context is invalid")
		}
		seen[destination.Origin] = true
	}
	for _, observation := range bundle.Journey.Observations {
		index, _ := strconv.Atoi(observation.Destination[1:])
		if index > len(private.Destinations) {
			return errors.New("private destination context is incomplete")
		}
	}
	for _, step := range private.Steps {
		if len(step.Selector) > 512 || (step.Kind != "input" && step.Kind != "click" && step.Kind != "submit" && step.Kind != "navigation") || (step.Kind != "input" && !step.Checkpoint) {
			return errors.New("private investigation step is invalid")
		}
		if step.Marker != "" {
			number, err := strconv.Atoi(step.Marker[1:])
			if err != nil || number < 1 || number > 16 || step.Marker != "m"+strconv.Itoa(number) {
				return errors.New("private step marker is invalid")
			}
		}
	}
	return nil
}

// ReadInvestigation verifies a saved directory or portable JSON export. Only a
// local directory can return separately verified private destination names.
func ReadInvestigation(path string) (JourneyBundle, []DestinationName, error) {
	bundle, private, err := readInvestigation(path)
	return bundle, private.Destinations, err
}

func readInvestigation(path string) (JourneyBundle, privateJourneyContext, error) {
	var empty privateJourneyContext
	info, err := os.Lstat(path)
	if err != nil {
		return JourneyBundle{}, empty, errors.New("saved investigation is unavailable")
	}
	if !info.IsDir() {
		data, err := readInvestigationFile(path, maxJourneyBundleBytes)
		if err != nil {
			return JourneyBundle{}, empty, err
		}
		bundle, err := DecodeJourneyBundle(data)
		return bundle, empty, err
	}
	if securefs.ValidateDirectory(path) != nil {
		return JourneyBundle{}, empty, errors.New("saved investigation path is unsafe")
	}
	bundle := JourneyBundle{SchemaVersion: 1}
	for name, target := range map[string]any{"journey.json": &bundle.Journey, "trace.json": &bundle.Trace, "receipt.json": &bundle.Receipt} {
		data, err := readInvestigationFile(filepath.Join(path, name), maxJourneyBundleBytes)
		if err != nil {
			return bundle, empty, err
		}
		if err := decodeInvestigationJSON(data, maxJourneyBundleBytes, target); err != nil {
			return bundle, empty, err
		}
	}
	if err := bundle.Verify(); err != nil {
		return bundle, empty, err
	}
	privatePath := filepath.Join(path, "private-context.json")
	privateReceiptPath := filepath.Join(path, "private-receipt.json")
	_, contextErr := os.Lstat(privatePath)
	_, receiptErr := os.Lstat(privateReceiptPath)
	if errors.Is(contextErr, os.ErrNotExist) && errors.Is(receiptErr, os.ErrNotExist) {
		return bundle, empty, nil
	}
	data, err := readInvestigationFile(privatePath, 256<<10)
	if err != nil {
		return bundle, empty, err
	}
	var private privateJourneyContext
	if decodeInvestigationJSON(data, 256<<10, &private) != nil || private.validate(bundle) != nil {
		return bundle, empty, errors.New("private investigation context failed verification")
	}
	receiptData, err := readInvestigationFile(privateReceiptPath, 1024)
	if err != nil {
		return bundle, empty, err
	}
	var receipt privateJourneyReceipt
	canonical, _ := json.Marshal(private)
	digest := sha256.Sum256(canonical)
	if decodeInvestigationJSON(receiptData, 1024, &receipt) != nil || receipt.SchemaVersion != 1 || receipt.ContextSHA256 != hex.EncodeToString(digest[:]) {
		return bundle, empty, errors.New("private investigation context identity does not match")
	}
	return bundle, private, nil
}

func readInvestigationFile(path string, limit int) ([]byte, error) {
	if securefs.ValidateDirectory(filepath.Dir(path)) != nil {
		return nil, errors.New("investigation parent directory is unsafe")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, errors.New("investigation file must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("investigation file unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.New("investigation file changed during verification")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("investigation file exceeds size limit")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, after) || securefs.ValidateDirectory(filepath.Dir(path)) != nil {
		return nil, errors.New("investigation file changed during verification")
	}
	return data, nil
}
