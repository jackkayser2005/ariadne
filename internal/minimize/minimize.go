// Package minimize runs bounded minimum-disclosure experiments over replicated
// counterfactual runs.
package minimize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// CurrentSchemaVersion is the minimization plan schema supported by this build.
	CurrentSchemaVersion = 1
	// SummarySchemaVersion is the raw-value-free minimization receipt schema.
	SummarySchemaVersion = 1

	// FunctionalityCriterionAllNonDisclosureFields names the first fixed
	// criterion: every observed field other than the declared input and request
	// identifier must remain equivalent.
	FunctionalityCriterionAllNonDisclosureFields = "all-non-disclosure-fields-equal-v1"

	maxPlanBytes     = 64 << 10
	maxSummaryBytes  = 128 << 10
	maxCandidates    = 8
	maxCandidateID   = 64
	maxPlanName      = 128
	maxVariableBytes = 128
)

// Candidate identifies one disclosure level. Value is kept in the local plan
// only; it is never copied into a minimization summary. Omitted uses the
// fixture-safe sentinel understood by the first Android adapter.
type Candidate struct {
	ID      string `json:"id"`
	Value   string `json:"value,omitempty"`
	Omitted bool   `json:"omitted,omitempty"`
}

// MinimizationPlan declares one controlled input and an ordered disclosure
// ladder. Candidates are evaluated in the supplied order, from the reference
// level toward less-disclosing levels.
type MinimizationPlan struct {
	SchemaVersion          int                `json:"schema_version"`
	Name                   string             `json:"name"`
	Variable               string             `json:"variable"`
	ReferenceCandidate     string             `json:"reference_candidate"`
	FunctionalityCriterion string             `json:"functionality_criterion"`
	TapResourceID          string             `json:"tap_resource_id"`
	BasePersona            experiment.Persona `json:"base_persona"`
	Candidates             []Candidate        `json:"candidates"`
}

// CandidateClassification is the bounded functionality conclusion for one
// candidate, independent of evidence.State.
type CandidateClassification string

const (
	CandidateSufficient        CandidateClassification = "sufficient"
	CandidateInsufficient      CandidateClassification = "insufficient"
	CandidateMixedInconsistent CandidateClassification = "mixed-inconsistent"
	CandidateUnknown           CandidateClassification = "unknown"
)

// SelectionState describes whether the ladder produced a usable selected
// candidate. It is a decision state, not a counterfactual outcome.
type SelectionState string

const (
	SelectionSelected     SelectionState = "selected"
	SelectionNoSufficient SelectionState = "no-sufficient-candidate"
	SelectionUnknown      SelectionState = "unknown"
)

// CandidateResult is a raw-value-free result for one replicated candidate.
type CandidateResult struct {
	ID             string                   `json:"id"`
	ManifestName   string                   `json:"manifest_name"`
	Directory      string                   `json:"directory"`
	Classification CandidateClassification  `json:"classification"`
	Outcome        bundle.ReplicatedOutcome `json:"outcome"`
	EvidenceState  evidence.State           `json:"evidence_state"`
	ReceiptSHA256  string                   `json:"receipt_sha256"`
	Pairs          int                      `json:"pairs"`
	PairsPerOrder  int                      `json:"pairs_per_order"`
	CompletedPairs int                      `json:"completed_pairs"`
	ChangedPairs   int                      `json:"changed_pairs"`
	NoChangePairs  int                      `json:"no_change_pairs"`
	UnknownPairs   int                      `json:"unknown_pairs"`
}

// MinimizationSummary is the raw-value-free receipt for one complete ladder
// evaluation. A selected candidate is only reported when every candidate was
// observed consistently.
type MinimizationSummary struct {
	SchemaVersion          int               `json:"schema_version"`
	PlanName               string            `json:"plan_name"`
	Variable               string            `json:"variable"`
	ReferenceCandidate     string            `json:"reference_candidate"`
	FunctionalityCriterion string            `json:"functionality_criterion"`
	PairsPerOrder          int               `json:"pairs_per_order"`
	EvidenceState          evidence.State    `json:"evidence_state"`
	SelectionState         SelectionState    `json:"selection_state"`
	SelectedCandidate      string            `json:"selected_candidate,omitempty"`
	CandidateResults       []CandidateResult `json:"candidate_results"`
}

// ReplicatedRunner is the source-specific execution boundary used by the
// minimization engine.
type ReplicatedRunner func(
	context.Context,
	string,
	adb.Target,
	experiment.Manifest,
	string,
	int,
) error

// SummaryReporter creates the authoritative evidence outputs for one pair.
type SummaryReporter func(string) (bundle.Summary, error)

// SummaryVerifier verifies one replicated candidate directory.
type SummaryVerifier func(string) (bundle.ReplicatedExperimentSummary, error)

// Decode reads and validates one bounded minimization plan.
func Decode(reader io.Reader) (MinimizationPlan, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxPlanBytes+1))
	if err != nil {
		return MinimizationPlan{}, fmt.Errorf("minimization plan: read input: %w", err)
	}
	if len(data) > maxPlanBytes {
		return MinimizationPlan{}, fmt.Errorf("minimization plan: exceeds %d-byte limit", maxPlanBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return MinimizationPlan{}, errors.New("minimization plan: empty input")
	}
	if !utf8.Valid(data) {
		return MinimizationPlan{}, errors.New("minimization plan: input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return MinimizationPlan{}, fmt.Errorf("minimization plan: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan MinimizationPlan
	if err := decoder.Decode(&plan); err != nil {
		return MinimizationPlan{}, fmt.Errorf("minimization plan: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MinimizationPlan{}, errors.New("minimization plan: trailing data")
	}
	if err := plan.Validate(); err != nil {
		return MinimizationPlan{}, fmt.Errorf("minimization plan: %w", err)
	}
	return plan, nil
}

// Validate reports whether the plan is safe to turn into authenticated
// Android manifests.
func (plan MinimizationPlan) Validate() error {
	if plan.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", plan.SchemaVersion)
	}
	if !validIdentifier(plan.Name, maxPlanName) {
		return errors.New("name: invalid identifier")
	}
	if len(plan.Variable) == 0 || len(plan.Variable) > maxVariableBytes {
		return errors.New("variable: invalid length")
	}
	if err := experiment.ValidateVolatileFields([]string{plan.Variable}); err != nil {
		return fmt.Errorf("variable: %w", err)
	}
	if plan.ReferenceCandidate == "" {
		return errors.New("reference_candidate: required")
	}
	if plan.FunctionalityCriterion != FunctionalityCriterionAllNonDisclosureFields {
		return errors.New("functionality_criterion: unsupported value")
	}
	if !experiment.ValidResourceID(plan.TapResourceID) {
		return errors.New("tap_resource_id: invalid resource identifier")
	}
	if len(plan.BasePersona) == 0 || len(plan.BasePersona) >= 64 {
		return errors.New("base_persona: expected between 1 and 63 fields")
	}
	for key, value := range plan.BasePersona {
		if !safeShellToken(key) || !safeShellToken(value) {
			return errors.New("base_persona: contains an unsafe field")
		}
		if key == plan.Variable {
			return errors.New("base_persona: must not contain the declared variable")
		}
	}
	if len(plan.Candidates) < 2 || len(plan.Candidates) > maxCandidates {
		return fmt.Errorf("candidates: expected between 2 and %d entries", maxCandidates)
	}
	if plan.Candidates[0].ID != plan.ReferenceCandidate {
		return errors.New("reference_candidate: must be the first candidate")
	}
	seen := make(map[string]struct{}, len(plan.Candidates))
	referenceFound := false
	for _, candidate := range plan.Candidates {
		if !validIdentifier(candidate.ID, maxCandidateID) {
			return errors.New("candidates: invalid identifier")
		}
		if _, ok := seen[candidate.ID]; ok {
			return errors.New("candidates: duplicate identifier")
		}
		seen[candidate.ID] = struct{}{}
		if candidate.ID == plan.ReferenceCandidate {
			referenceFound = true
		}
		if candidate.Omitted {
			if candidate.Value != "" {
				return errors.New("candidates: omitted entry must not include a value")
			}
			continue
		}
		if !safeShellToken(candidate.Value) {
			return errors.New("candidates: value is unsafe or empty")
		}
	}
	if !referenceFound {
		return errors.New("reference_candidate: candidate is missing")
	}
	return nil
}

// ManifestFor constructs the authenticated experiment manifest for one
// candidate without exposing the candidate value in any portable result.
func (plan MinimizationPlan) ManifestFor(candidateID string) (experiment.Manifest, error) {
	if err := plan.Validate(); err != nil {
		return experiment.Manifest{}, err
	}
	var reference, candidate Candidate
	foundReference := false
	foundCandidate := false
	for _, item := range plan.Candidates {
		if item.ID == plan.ReferenceCandidate {
			reference = item
			foundReference = true
		}
		if item.ID == candidateID {
			candidate = item
			foundCandidate = true
		}
	}
	if !foundReference || !foundCandidate {
		return experiment.Manifest{}, errors.New("candidate is not part of the plan")
	}
	if candidateID == plan.ReferenceCandidate {
		return experiment.Manifest{}, errors.New("reference candidate is not a treatment candidate")
	}
	baseline := maps.Clone(plan.BasePersona)
	treatment := maps.Clone(plan.BasePersona)
	baseline[plan.Variable] = inputValue(reference)
	treatment[plan.Variable] = inputValue(candidate)
	manifest := experiment.Manifest{
		SchemaVersion:  experiment.CurrentSchemaVersion,
		Name:           plan.Name + "-" + candidate.ID,
		Variable:       plan.Variable,
		Baseline:       baseline,
		Treatment:      treatment,
		VolatileFields: experiment.CanonicalVolatileFields([]string{plan.Variable, "request_id"}),
		TapResourceID:  plan.TapResourceID,
	}
	if err := manifest.Validate(); err != nil {
		return experiment.Manifest{}, fmt.Errorf("candidate manifest: %w", err)
	}
	return manifest, nil
}

func inputValue(candidate Candidate) string {
	if candidate.Omitted {
		return "omitted"
	}
	return candidate.Value
}

// Execute evaluates every candidate through the existing replicated runner,
// writes each pair's authoritative report, and saves a raw-value-free receipt.
func Execute(
	ctx context.Context,
	binary string,
	target adb.Target,
	plan MinimizationPlan,
	outputDir string,
	pairs int,
	runner ReplicatedRunner,
) (MinimizationSummary, error) {
	return execute(ctx, binary, target, plan, outputDir, pairs, runner, bundle.Write, bundle.VerifyReplicated)
}

func execute(
	ctx context.Context,
	binary string,
	target adb.Target,
	plan MinimizationPlan,
	outputDir string,
	pairs int,
	runner ReplicatedRunner,
	reporter SummaryReporter,
	verifier SummaryVerifier,
) (MinimizationSummary, error) {
	if err := plan.Validate(); err != nil {
		return MinimizationSummary{}, err
	}
	if strings.TrimSpace(outputDir) == "" {
		return MinimizationSummary{}, errors.New("output directory is required")
	}
	if pairs < 1 || pairs > 8 {
		return MinimizationSummary{}, errors.New("pairs must be between 1 and 8")
	}
	if runner == nil || reporter == nil || verifier == nil {
		return MinimizationSummary{}, errors.New("minimization dependencies are required")
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o700); err != nil {
		return MinimizationSummary{}, fmt.Errorf("create output parent: %w", err)
	}
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		return MinimizationSummary{}, fmt.Errorf("create output directory: %w", err)
	}

	results := make([]CandidateResult, 0, len(plan.Candidates)-1)
	testedIndex := 0
	for _, candidate := range plan.Candidates {
		if candidate.ID == plan.ReferenceCandidate {
			continue
		}
		manifest, err := plan.ManifestFor(candidate.ID)
		if err != nil {
			return MinimizationSummary{}, err
		}
		directory := candidateDirectory(testedIndex, candidate.ID)
		candidateDir := filepath.Join(outputDir, directory)
		runErr := runner(ctx, binary, target, manifest, candidateDir, pairs)
		if reportErr := reportExistingPairs(candidateDir, pairs, reporter); reportErr != nil {
			return MinimizationSummary{}, fmt.Errorf("candidate %q: %w", candidate.ID, reportErr)
		}
		verified, verifyErr := verifier(candidateDir)
		if verifyErr != nil {
			if runErr != nil {
				return MinimizationSummary{}, fmt.Errorf("candidate %q did not produce a verifiable replication", candidate.ID)
			}
			return MinimizationSummary{}, fmt.Errorf("candidate %q: verify replication: %w", candidate.ID, verifyErr)
		}
		if verified.ManifestName != manifest.Name {
			return MinimizationSummary{}, fmt.Errorf("candidate %q: replication manifest metadata disagrees", candidate.ID)
		}
		if runErr != nil && verified.Outcome != bundle.ReplicationUnknown {
			return MinimizationSummary{}, fmt.Errorf("candidate %q failed after a non-unknown result", candidate.ID)
		}
		results = append(results, candidateResult(candidate.ID, directory, verified))
		testedIndex++
	}

	summary, err := summarize(plan, pairs, results)
	if err != nil {
		return MinimizationSummary{}, err
	}
	if err := Save(outputDir, summary); err != nil {
		return MinimizationSummary{}, err
	}
	return summary, nil
}

func reportExistingPairs(candidateDir string, pairs int, reporter SummaryReporter) error {
	info, err := os.Lstat(candidateDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect candidate directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("candidate directory is not a directory")
	}
	completed, err := completedPairDirectories(candidateDir, pairs)
	if err != nil {
		return err
	}
	for pair := 1; pair <= pairs; pair++ {
		for _, order := range []string{
			adb.ReplicationOrderBaselineTreatment,
			adb.ReplicationOrderTreatmentBaseline,
		} {
			pairDir := filepath.Join(candidateDir, fmt.Sprintf("pair-%03d-%s", pair, order))
			directory := filepath.Base(pairDir)
			if _, ok := completed[directory]; !ok {
				continue
			}
			info, err := os.Lstat(pairDir)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return fmt.Errorf("inspect pair directory: %w", err)
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return errors.New("pair directory is not a directory")
			}
			evidenceInfo, err := os.Lstat(filepath.Join(pairDir, "evidence.json"))
			if errors.Is(err, os.ErrNotExist) {
				if _, err := reporter(pairDir); err != nil {
					return fmt.Errorf("write pair report: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("inspect pair evidence: %w", err)
			} else if evidenceInfo.Mode()&os.ModeSymlink != 0 || !evidenceInfo.Mode().IsRegular() {
				return errors.New("pair evidence is not a regular file")
			}
		}
	}
	return nil
}
func completedPairDirectories(candidateDir string, pairs int) (map[string]struct{}, error) {
	data, err := bundle.ReadBoundedFile(filepath.Join(candidateDir, "replication.json"), maxSummaryBytes)
	if err != nil {
		return nil, fmt.Errorf("replication metadata: %w", err)
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return nil, fmt.Errorf("replication metadata: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record adb.ReplicatedRunRecord
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("replication metadata: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("replication metadata: trailing data")
	}
	if record.SchemaVersion != adb.ReplicatedRunSchemaVersion || record.PairsPerOrder != pairs {
		return nil, errors.New("replication metadata configuration disagrees")
	}
	completed := make(map[string]struct{}, len(record.Pairs))
	for _, pair := range record.Pairs {
		if pair.Status != adb.ReplicationStatusComplete {
			continue
		}
		directory := fmt.Sprintf("pair-%03d-%s", pair.Pair, pair.Order)
		if pair.Pair < 1 || pair.Pair > pairs ||
			(pair.Order != adb.ReplicationOrderBaselineTreatment && pair.Order != adb.ReplicationOrderTreatmentBaseline) ||
			pair.Directory != directory {
			return nil, errors.New("replication pair metadata is invalid")
		}
		completed[directory] = struct{}{}
	}
	return completed, nil
}
func candidateDirectory(index int, id string) string {
	return fmt.Sprintf("candidate-%03d-%s", index+1, id)
}

func candidateResult(id, directory string, summary bundle.ReplicatedExperimentSummary) CandidateResult {
	return CandidateResult{
		ID:             id,
		ManifestName:   summary.ManifestName,
		Directory:      directory,
		Classification: classify(summary.Outcome, summary.EvidenceState),
		Outcome:        summary.Outcome,
		EvidenceState:  summary.EvidenceState,
		ReceiptSHA256:  summary.ReceiptSHA256,
		Pairs:          summary.Pairs,
		PairsPerOrder:  summary.PairsPerOrder,
		CompletedPairs: summary.CompletedPairs,
		ChangedPairs:   summary.ChangedPairs,
		NoChangePairs:  summary.NoChangePairs,
		UnknownPairs:   summary.UnknownPairs,
	}
}

func classify(outcome bundle.ReplicatedOutcome, state evidence.State) CandidateClassification {
	if state != evidence.Observed {
		return CandidateUnknown
	}
	switch outcome {
	case bundle.NoChangeObserved:
		return CandidateSufficient
	case bundle.ReplicatedChange:
		return CandidateInsufficient
	case bundle.MixedInconsistent:
		return CandidateMixedInconsistent
	default:
		return CandidateUnknown
	}
}

func summarize(plan MinimizationPlan, pairs int, results []CandidateResult) (MinimizationSummary, error) {
	if err := plan.Validate(); err != nil {
		return MinimizationSummary{}, err
	}
	if len(results) != len(plan.Candidates)-1 {
		return MinimizationSummary{}, errors.New("candidate result count does not match plan")
	}
	resultIndex := 0
	for _, candidate := range plan.Candidates {
		if candidate.ID == plan.ReferenceCandidate {
			continue
		}
		if results[resultIndex].ID != candidate.ID {
			return MinimizationSummary{}, errors.New("candidate result order does not match plan")
		}
		resultIndex++
	}
	selection, selected, state := selectionFor(results)
	summary := MinimizationSummary{
		SchemaVersion:          SummarySchemaVersion,
		PlanName:               plan.Name,
		Variable:               plan.Variable,
		ReferenceCandidate:     plan.ReferenceCandidate,
		FunctionalityCriterion: plan.FunctionalityCriterion,
		PairsPerOrder:          pairs,
		EvidenceState:          state,
		SelectionState:         selection,
		SelectedCandidate:      selected,
		CandidateResults:       results,
	}
	if err := validateSummary(summary); err != nil {
		return MinimizationSummary{}, err
	}
	return summary, nil
}

func selectionFor(results []CandidateResult) (SelectionState, string, evidence.State) {
	state := evidence.Observed
	selection := SelectionNoSufficient
	selected := ""
	for _, result := range results {
		if result.EvidenceState != evidence.Observed {
			state = evidence.Unknown
		}
		if result.Classification == CandidateUnknown || result.Classification == CandidateMixedInconsistent {
			selection = SelectionUnknown
		}
		if result.Classification == CandidateSufficient {
			selected = result.ID
		}
	}
	if selection != SelectionUnknown && selected != "" {
		selection = SelectionSelected
	}
	return selection, selectedFor(selection, selected), state
}

func selectedFor(selection SelectionState, selected string) string {
	if selection != SelectionSelected {
		return ""
	}
	return selected
}

// Save writes a raw-value-free minimization receipt without overwriting an
// existing receipt.
func Save(rootDir string, summary MinimizationSummary) error {
	if strings.TrimSpace(rootDir) == "" {
		return errors.New("minimization directory is required")
	}
	if err := requireDirectory(rootDir); err != nil {
		return err
	}
	if err := validateSummary(summary); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return errors.New("minimization receipt encoding failed")
	}
	data = append(data, '\n')
	if len(data) > maxSummaryBytes {
		return fmt.Errorf("minimization receipt exceeds %d-byte limit", maxSummaryBytes)
	}
	path := filepath.Join(rootDir, "minimization.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create minimization receipt: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write minimization receipt: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync minimization receipt: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close minimization receipt: %w", err)
	}
	remove = false
	return nil
}

// Verify checks the receipt and every child replication without reading a
// plan or reconstructing any raw candidate values.
func Verify(rootDir string) (MinimizationSummary, error) {
	if strings.TrimSpace(rootDir) == "" {
		return MinimizationSummary{}, errors.New("minimization directory is required")
	}
	if err := requireDirectory(rootDir); err != nil {
		return MinimizationSummary{}, err
	}
	data, err := bundle.ReadBoundedFile(filepath.Join(rootDir, "minimization.json"), maxSummaryBytes)
	if err != nil {
		return MinimizationSummary{}, fmt.Errorf("minimization receipt: %w", err)
	}
	summary, err := decodeSummary(data)
	if err != nil {
		return MinimizationSummary{}, err
	}
	canonical, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return MinimizationSummary{}, errors.New("minimization receipt encoding failed")
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(data, canonical) {
		return MinimizationSummary{}, errors.New("minimization receipt does not match its canonical form")
	}
	if err := verifyChildren(rootDir, summary); err != nil {
		return MinimizationSummary{}, err
	}
	return summary, nil
}

func decodeSummary(data []byte) (MinimizationSummary, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return MinimizationSummary{}, errors.New("minimization receipt: empty input")
	}
	if !utf8.Valid(data) {
		return MinimizationSummary{}, errors.New("minimization receipt: input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return MinimizationSummary{}, fmt.Errorf("minimization receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var summary MinimizationSummary
	if err := decoder.Decode(&summary); err != nil {
		return MinimizationSummary{}, fmt.Errorf("minimization receipt: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return MinimizationSummary{}, errors.New("minimization receipt: trailing data")
	}
	if err := validateSummary(summary); err != nil {
		return MinimizationSummary{}, fmt.Errorf("minimization receipt: %w", err)
	}
	return summary, nil
}

func verifyChildren(rootDir string, summary MinimizationSummary) error {
	for _, result := range summary.CandidateResults {
		childDir := filepath.Join(rootDir, result.Directory)
		info, err := os.Lstat(childDir)
		if err != nil {
			return fmt.Errorf("candidate %q: directory: %w", result.ID, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("candidate %q: directory is invalid", result.ID)
		}
		child, err := bundle.VerifyReplicated(childDir)
		if err != nil {
			return fmt.Errorf("candidate %q: verify replication: %w", result.ID, err)
		}
		if child.DeclaredVariable != summary.Variable || child.PairsPerOrder != summary.PairsPerOrder {
			return fmt.Errorf("candidate %q: replication metadata disagrees", result.ID)
		}
		if child.ManifestName != summary.PlanName+"-"+result.ID {
			return fmt.Errorf("candidate %q: replication manifest metadata disagrees", result.ID)
		}
		expected := candidateResult(result.ID, result.Directory, child)
		if expected != result {
			return fmt.Errorf("candidate %q: result does not match replication", result.ID)
		}
	}
	selection, selected, state := selectionFor(summary.CandidateResults)
	if selection != summary.SelectionState || selected != summary.SelectedCandidate || state != summary.EvidenceState {
		return errors.New("minimization receipt selection does not match candidate results")
	}
	return nil
}

func validateSummary(summary MinimizationSummary) error {
	if summary.SchemaVersion != SummarySchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", summary.SchemaVersion)
	}
	if !validIdentifier(summary.PlanName, maxPlanName) {
		return errors.New("plan_name is invalid")
	}
	if len(summary.Variable) == 0 || len(summary.Variable) > maxVariableBytes {
		return errors.New("variable is invalid")
	}
	if err := experiment.ValidateVolatileFields([]string{summary.Variable}); err != nil {
		return errors.New("variable is invalid")
	}
	if !validIdentifier(summary.ReferenceCandidate, maxCandidateID) {
		return errors.New("reference_candidate is invalid")
	}
	if summary.FunctionalityCriterion != FunctionalityCriterionAllNonDisclosureFields {
		return errors.New("functionality_criterion is invalid")
	}
	if summary.PairsPerOrder < 1 || summary.PairsPerOrder > 8 {
		return errors.New("pairs_per_order is invalid")
	}
	if !summary.EvidenceState.Valid() {
		return errors.New("evidence_state is invalid")
	}
	switch summary.SelectionState {
	case SelectionSelected, SelectionNoSufficient, SelectionUnknown:
	default:
		return errors.New("selection_state is invalid")
	}
	if len(summary.CandidateResults) < 1 || len(summary.CandidateResults) >= maxCandidates {
		return errors.New("candidate_results count is invalid")
	}
	seen := make(map[string]struct{}, len(summary.CandidateResults))
	for index, result := range summary.CandidateResults {
		if !validIdentifier(result.ID, maxCandidateID) {
			return errors.New("candidate result id is invalid")
		}
		if _, ok := seen[result.ID]; ok {
			return errors.New("candidate result id is duplicated")
		}
		seen[result.ID] = struct{}{}
		if result.ID == summary.ReferenceCandidate {
			return errors.New("reference candidate must not have a treatment result")
		}
		if result.Directory != candidateDirectory(index, result.ID) {
			return errors.New("candidate result directory is invalid")
		}
		if !validIdentifier(result.ManifestName, maxPlanName+maxCandidateID+1) {
			return errors.New("candidate result manifest_name is invalid")
		}
		if result.ManifestName != summary.PlanName+"-"+result.ID {
			return errors.New("candidate result manifest_name disagrees")
		}
		if !validDigest(result.ReceiptSHA256) {
			return errors.New("candidate result receipt_sha256 is invalid")
		}
		if result.Pairs != result.PairsPerOrder*2 ||
			result.PairsPerOrder != summary.PairsPerOrder ||
			result.CompletedPairs < 0 || result.CompletedPairs > result.Pairs ||
			result.ChangedPairs < 0 || result.NoChangePairs < 0 || result.UnknownPairs < 0 ||
			result.ChangedPairs+result.NoChangePairs+result.UnknownPairs != result.Pairs {
			return errors.New("candidate result counts are invalid")
		}
		if !result.EvidenceState.Valid() {
			return errors.New("candidate result evidence_state is invalid")
		}
		switch result.Outcome {
		case bundle.ReplicatedChange, bundle.NoChangeObserved, bundle.MixedInconsistent, bundle.ReplicationUnknown:
		default:
			return errors.New("candidate result outcome is invalid")
		}
		switch result.Classification {
		case CandidateSufficient, CandidateInsufficient, CandidateMixedInconsistent, CandidateUnknown:
		default:
			return errors.New("candidate result classification is invalid")
		}
		if classify(result.Outcome, result.EvidenceState) != result.Classification {
			return errors.New("candidate result classification disagrees with outcome")
		}
	}
	selection, selected, state := selectionFor(summary.CandidateResults)
	if selection != summary.SelectionState || selected != summary.SelectedCandidate || state != summary.EvidenceState {
		return errors.New("selection does not match candidate results")
	}
	if summary.SelectionState != SelectionSelected && summary.SelectedCandidate != "" {
		return errors.New("selected_candidate is invalid for selection state")
	}
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("minimization directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("minimization directory is not a directory")
	}
	return nil
}

func validIdentifier(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	for index, character := range value {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if index == 0 && !letter && !digit {
			return false
		}
		if !letter && !digit && !strings.ContainsRune("._-", character) {
			return false
		}
	}
	return true
}

func safeShellToken(value string) bool {
	if value == "" || len(value) > 1024 || strings.ContainsFunc(value, unicode.IsControl) {
		return false
	}
	for _, character := range value {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && !strings.ContainsRune("._@:+-", character) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
