package minimize

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

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	// LadderSchemaVersion is the source-neutral candidate ladder schema.
	LadderSchemaVersion = 1
	// LadderSummarySchemaVersion is the source-neutral minimization receipt schema.
	LadderSummarySchemaVersion = 1
	maxLadderPlanBytes         = 64 << 10
	maxLadderSummaryBytes      = 128 << 10
	maxLadderCandidates        = 8
)

// LadderPlan declares an ordered, adapter-owned candidate ladder. It contains
// identifiers only; candidate values belong exclusively to the source adapter.
type LadderPlan struct {
	SchemaVersion          int      `json:"schema_version"`
	Name                   string   `json:"name"`
	Variable               string   `json:"variable"`
	ReferenceCandidate     string   `json:"reference_candidate"`
	FunctionalityCriterion string   `json:"functionality_criterion"`
	Candidates             []string `json:"candidates"`
}

// LadderProvenance binds a source-neutral receipt to the adapter boundary that
// produced its child replications.
type LadderProvenance struct {
	Adapter         string `json:"adapter"`
	AdapterVersion  int    `json:"adapter_version"`
	ProcedureSHA256 string `json:"procedure_sha256"`
	Scope           string `json:"scope"`
	ResetPolicy     string `json:"reset_policy"`
}

// LadderCandidateResult reuses the shared candidate-result decision shape.
// Browser ladders leave ManifestName empty; its omitempty tag preserves the
// source-specific portable receipt while sharing classification semantics.
type LadderCandidateResult = CandidateResult

// LadderSummary is the canonical, raw-value-free receipt for an adapter-owned
// minimization ladder.
type LadderSummary struct {
	SchemaVersion          int                     `json:"schema_version"`
	PlanName               string                  `json:"plan_name"`
	Variable               string                  `json:"variable"`
	ReferenceCandidate     string                  `json:"reference_candidate"`
	FunctionalityCriterion string                  `json:"functionality_criterion"`
	Adapter                string                  `json:"adapter"`
	AdapterVersion         int                     `json:"adapter_version"`
	ProcedureSHA256        string                  `json:"procedure_sha256"`
	Scope                  string                  `json:"scope"`
	ResetPolicy            string                  `json:"reset_policy"`
	PairsPerOrder          int                     `json:"pairs_per_order"`
	EvidenceState          evidence.State          `json:"evidence_state"`
	SelectionState         SelectionState          `json:"selection_state"`
	SelectedCandidate      string                  `json:"selected_candidate,omitempty"`
	CandidateResults       []LadderCandidateResult `json:"candidate_results"`
}

// LadderChildVerifier verifies the adapter-owned child replication represented
// by one ladder result. It must reject a child with mismatched provenance or
// any result that does not exactly match the portable receipt.
type LadderChildVerifier func(string, LadderSummary, LadderCandidateResult) error

// DecodeLadder reads and validates one bounded source-neutral ladder plan.
func DecodeLadder(reader io.Reader) (LadderPlan, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxLadderPlanBytes+1))
	if err != nil {
		return LadderPlan{}, fmt.Errorf("ladder plan: read input: %w", err)
	}
	if len(data) > maxLadderPlanBytes {
		return LadderPlan{}, fmt.Errorf("ladder plan: exceeds %d-byte limit", maxLadderPlanBytes)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return LadderPlan{}, errors.New("ladder plan: empty input")
	}
	if !utf8.Valid(data) {
		return LadderPlan{}, errors.New("ladder plan: input must be valid UTF-8")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return LadderPlan{}, fmt.Errorf("ladder plan: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan LadderPlan
	if err := decoder.Decode(&plan); err != nil {
		return LadderPlan{}, fmt.Errorf("ladder plan: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return LadderPlan{}, errors.New("ladder plan: trailing data")
	}
	if err := plan.Validate(); err != nil {
		return LadderPlan{}, fmt.Errorf("ladder plan: %w", err)
	}
	return plan, nil
}

// ReadLadder reads one ladder plan from a bounded file.
func ReadLadder(path string) (LadderPlan, error) {
	if strings.TrimSpace(path) == "" {
		return LadderPlan{}, errors.New("ladder plan path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return LadderPlan{}, errors.New("read ladder plan")
	}
	defer file.Close()
	return DecodeLadder(file)
}

// Validate reports whether the ladder can be handed to a source adapter.
func (plan LadderPlan) Validate() error {
	if plan.SchemaVersion != LadderSchemaVersion {
		return fmt.Errorf("schema_version: unsupported value %d", plan.SchemaVersion)
	}
	if !validIdentifier(plan.Name, maxPlanName) {
		return errors.New("name: invalid identifier")
	}
	if !validIdentifier(plan.Variable, maxVariableBytes) {
		return errors.New("variable: invalid identifier")
	}
	if plan.ReferenceCandidate == "" {
		return errors.New("reference_candidate: required")
	}
	if plan.FunctionalityCriterion != FunctionalityCriterionAllNonDisclosureFields {
		return errors.New("functionality_criterion: unsupported value")
	}
	if len(plan.Candidates) < 2 || len(plan.Candidates) > maxLadderCandidates {
		return fmt.Errorf("candidates: expected between 2 and %d entries", maxLadderCandidates)
	}
	if plan.Candidates[0] != plan.ReferenceCandidate {
		return errors.New("reference_candidate: must be the first candidate")
	}
	seen := make(map[string]struct{}, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		if !validIdentifier(candidate, maxCandidateID) {
			return errors.New("candidates: invalid identifier")
		}
		if _, ok := seen[candidate]; ok {
			return errors.New("candidates: duplicate identifier")
		}
		seen[candidate] = struct{}{}
	}
	return nil
}

// Validate reports whether the provenance is safe to retain in a portable
// ladder receipt.
func (provenance LadderProvenance) Validate() error {
	if !validIdentifier(provenance.Adapter, 128) || provenance.AdapterVersion < 1 || provenance.AdapterVersion > 32 ||
		!validDigest(provenance.ProcedureSHA256) || !validIdentifier(provenance.Scope, 64) || !validIdentifier(provenance.ResetPolicy, 128) {
		return errors.New("ladder provenance is invalid")
	}
	return nil
}

// LadderCandidateDirectory returns the stable child directory for one tested
// candidate. The reference candidate is not assigned a child directory.
func LadderCandidateDirectory(index int, id string) string {
	return fmt.Sprintf("candidate-%03d-%s", index+1, id)
}

// SummarizeLadder derives classifications and selection from verified child
// counts. Caller-supplied labels are checked, never trusted as authority.
func SummarizeLadder(plan LadderPlan, provenance LadderProvenance, pairs int, results []LadderCandidateResult) (LadderSummary, error) {
	if err := plan.Validate(); err != nil {
		return LadderSummary{}, err
	}
	if err := provenance.Validate(); err != nil {
		return LadderSummary{}, err
	}
	if pairs < 1 || pairs > 8 {
		return LadderSummary{}, errors.New("pairs must be between 1 and 8")
	}
	if len(results) != len(plan.Candidates)-1 {
		return LadderSummary{}, errors.New("candidate result count does not match ladder")
	}
	for index, candidate := range plan.Candidates[1:] {
		if results[index].ID != candidate {
			return LadderSummary{}, errors.New("candidate result order does not match ladder")
		}
		if results[index].Directory != LadderCandidateDirectory(index, candidate) {
			return LadderSummary{}, errors.New("candidate result directory does not match ladder")
		}
		classification, outcome, err := classifyLadderResult(results[index], pairs)
		if err != nil {
			return LadderSummary{}, fmt.Errorf("candidate %q: %w", candidate, err)
		}
		if results[index].Classification != "" && results[index].Classification != classification {
			return LadderSummary{}, fmt.Errorf("candidate %q: classification disagrees with counts", candidate)
		}
		if results[index].Outcome != outcome {
			return LadderSummary{}, fmt.Errorf("candidate %q: outcome disagrees with counts", candidate)
		}
		results[index].Classification = classification
	}
	selection, selected, state := selectionFor(results)
	summary := LadderSummary{
		SchemaVersion:          LadderSummarySchemaVersion,
		PlanName:               plan.Name,
		Variable:               plan.Variable,
		ReferenceCandidate:     plan.ReferenceCandidate,
		FunctionalityCriterion: plan.FunctionalityCriterion,
		Adapter:                provenance.Adapter,
		AdapterVersion:         provenance.AdapterVersion,
		ProcedureSHA256:        provenance.ProcedureSHA256,
		Scope:                  provenance.Scope,
		ResetPolicy:            provenance.ResetPolicy,
		PairsPerOrder:          pairs,
		EvidenceState:          state,
		SelectionState:         selection,
		SelectedCandidate:      selected,
		CandidateResults:       results,
	}
	if err := summary.Validate(); err != nil {
		return LadderSummary{}, err
	}
	return summary, nil
}

// Validate reports whether a ladder summary is internally consistent.
func (summary LadderSummary) Validate() error {
	if summary.SchemaVersion != LadderSummarySchemaVersion || !validIdentifier(summary.PlanName, maxPlanName) ||
		!validIdentifier(summary.Variable, maxVariableBytes) || !validIdentifier(summary.ReferenceCandidate, maxCandidateID) ||
		summary.FunctionalityCriterion != FunctionalityCriterionAllNonDisclosureFields || summary.PairsPerOrder < 1 || summary.PairsPerOrder > 8 {
		return errors.New("ladder summary is invalid")
	}
	if err := (LadderProvenance{
		Adapter:         summary.Adapter,
		AdapterVersion:  summary.AdapterVersion,
		ProcedureSHA256: summary.ProcedureSHA256,
		Scope:           summary.Scope,
		ResetPolicy:     summary.ResetPolicy,
	}).Validate(); err != nil {
		return err
	}
	if !summary.EvidenceState.Valid() {
		return errors.New("ladder summary evidence_state is invalid")
	}
	switch summary.SelectionState {
	case SelectionSelected, SelectionNoSufficient, SelectionUnknown:
	default:
		return errors.New("ladder summary selection_state is invalid")
	}
	if len(summary.CandidateResults) < 1 || len(summary.CandidateResults) >= maxLadderCandidates {
		return errors.New("ladder summary candidate_results count is invalid")
	}
	seen := make(map[string]struct{}, len(summary.CandidateResults))
	for index, result := range summary.CandidateResults {
		if !validIdentifier(result.ID, maxCandidateID) || result.ID == summary.ReferenceCandidate {
			return errors.New("ladder summary candidate result ID is invalid")
		}
		if _, ok := seen[result.ID]; ok {
			return errors.New("ladder summary candidate result ID is duplicated")
		}
		seen[result.ID] = struct{}{}
		if result.Directory != LadderCandidateDirectory(index, result.ID) {
			return errors.New("ladder summary candidate directory is invalid")
		}
		classification, outcome, err := classifyLadderResult(result, summary.PairsPerOrder)
		if err != nil || result.Classification != classification || result.Outcome != outcome {
			return errors.New("ladder summary candidate classification disagrees with counts")
		}
	}
	selection, selected, state := selectionFor(summary.CandidateResults)
	if selection != summary.SelectionState || selected != summary.SelectedCandidate || state != summary.EvidenceState {
		return errors.New("ladder summary selection does not match candidate results")
	}
	return nil
}

// SaveLadder writes a canonical ladder receipt without overwriting an existing
// receipt in the output directory.
func SaveLadder(rootDir string, summary LadderSummary) error {
	if strings.TrimSpace(rootDir) == "" {
		return errors.New("ladder directory is required")
	}
	if err := requireDirectory(rootDir); err != nil {
		return err
	}
	if err := summary.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return errors.New("ladder receipt encoding failed")
	}
	data = append(data, '\n')
	if len(data) > maxLadderSummaryBytes {
		return fmt.Errorf("ladder receipt exceeds %d-byte limit", maxLadderSummaryBytes)
	}
	path := filepath.Join(rootDir, "minimization.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create ladder receipt: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write ladder receipt: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync ladder receipt: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close ladder receipt: %w", err)
	}
	remove = false
	return nil
}

// VerifyLadder verifies the canonical receipt and delegates each child to its
// source adapter. The returned digest is the identity of the receipt bytes.
func VerifyLadder(rootDir string, verifyChild LadderChildVerifier) (LadderSummary, string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return LadderSummary{}, "", errors.New("ladder directory is required")
	}
	if err := requireDirectory(rootDir); err != nil {
		return LadderSummary{}, "", err
	}
	if verifyChild == nil {
		return LadderSummary{}, "", errors.New("ladder child verifier is required")
	}
	data, err := bundle.ReadBoundedFile(filepath.Join(rootDir, "minimization.json"), maxLadderSummaryBytes)
	if err != nil {
		return LadderSummary{}, "", fmt.Errorf("ladder receipt: %w", err)
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return LadderSummary{}, "", fmt.Errorf("ladder receipt: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var summary LadderSummary
	if err := decoder.Decode(&summary); err != nil {
		return LadderSummary{}, "", fmt.Errorf("ladder receipt: decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return LadderSummary{}, "", errors.New("ladder receipt: trailing data")
	}
	if err := summary.Validate(); err != nil {
		return LadderSummary{}, "", err
	}
	canonical, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return LadderSummary{}, "", errors.New("ladder receipt encoding failed")
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(data, canonical) {
		return LadderSummary{}, "", errors.New("ladder receipt does not match its canonical form")
	}
	for _, result := range summary.CandidateResults {
		if err := verifyChild(rootDir, summary, result); err != nil {
			return LadderSummary{}, "", fmt.Errorf("candidate %q: %w", result.ID, err)
		}
	}
	digest := sha256.Sum256(data)
	return summary, hex.EncodeToString(digest[:]), nil
}

func classifyLadderResult(result LadderCandidateResult, pairs int) (CandidateClassification, portabletrace.ReplicatedOutcome, error) {
	if !validIdentifier(result.ID, maxCandidateID) || !validDigest(result.ReceiptSHA256) || !result.EvidenceState.Valid() {
		return CandidateUnknown, portabletrace.ReplicationUnknown, errors.New("ladder candidate result is invalid")
	}
	if result.Pairs != pairs*2 || result.PairsPerOrder != pairs || result.CompletedPairs < 0 || result.CompletedPairs > result.Pairs ||
		result.ChangedPairs < 0 || result.NoChangePairs < 0 || result.UnknownPairs < 0 ||
		result.ChangedPairs+result.NoChangePairs+result.UnknownPairs != result.Pairs {
		return CandidateUnknown, portabletrace.ReplicationUnknown, errors.New("ladder candidate result counts are invalid")
	}
	outcome := portabletrace.ReplicationUnknown
	if result.EvidenceState == evidence.Observed && result.UnknownPairs == 0 {
		switch {
		case result.ChangedPairs == result.Pairs:
			outcome = portabletrace.ReplicatedChange
		case result.NoChangePairs == result.Pairs:
			outcome = portabletrace.NoChangeObserved
		default:
			outcome = portabletrace.MixedInconsistent
		}
	}
	return classify(outcome, result.EvidenceState), outcome, nil
}

// ClassifyLadderCandidate derives one candidate classification from its
// verified counts and outcome. It is exported so source adapters can bind
// their child verifier to the shared receipt contract.
func ClassifyLadderCandidate(result LadderCandidateResult, pairs int) (CandidateClassification, error) {
	classification, outcome, err := classifyLadderResult(result, pairs)
	if err != nil {
		return CandidateUnknown, err
	}
	if result.Outcome != outcome {
		return CandidateUnknown, errors.New("ladder candidate outcome disagrees with counts")
	}
	return classification, nil
}
