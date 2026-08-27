package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/minimize"
	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	// BrowserFunctionalityCriterion is the fixed browser criterion for this
	// slice: non-disclosure tracking fields must remain equivalent.
	BrowserFunctionalityCriterion = minimize.FunctionalityCriterionAllNonDisclosureFields
	maxBrowserMinimizationPairs   = 8
)

// FixtureMinimizationInput selects one fixed local browser minimization run.
// The plan contains candidate IDs only; the adapter owns their input mapping.
type FixtureMinimizationInput struct {
	PlanPath      string
	ProcedurePath string
	DriverPath    string
	DriverArgs    []string
	OutputDir     string
	Pairs         int
}

type fixtureMinimizationRunner func(context.Context, FixtureReplicationInput) error
type fixtureMinimizationVerifier func(string) (BrowserReplicationSummary, error)

// RunFixtureMinimization evaluates the fixed reference/omitted browser ladder
// through replicated fresh-profile pairs.
func RunFixtureMinimization(ctx context.Context, input FixtureMinimizationInput) error {
	return runFixtureMinimizationWith(ctx, input, RunFixtureReplicated, VerifyFixtureReplicatedForFunctionality)
}

func runFixtureMinimizationWith(ctx context.Context, input FixtureMinimizationInput, run fixtureMinimizationRunner, verify fixtureMinimizationVerifier) error {
	plan, procedureSHA256, err := validateFixtureMinimizationInput(input)
	if err != nil {
		return err
	}
	if run == nil || verify == nil {
		return errors.New("browser minimization dependencies are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(input.OutputDir), 0o700); err != nil {
		return errors.New("create browser minimization parent")
	}
	if err := os.Mkdir(input.OutputDir, 0o700); err != nil {
		return errors.New("create browser minimization output")
	}

	results := make([]minimize.LadderCandidateResult, 0, len(plan.Candidates)-1)
	for index, candidateID := range plan.Candidates[1:] {
		directory := minimize.LadderCandidateDirectory(index, candidateID)
		candidateDir := filepath.Join(input.OutputDir, directory)
		runErr := run(ctx, FixtureReplicationInput{
			ProcedurePath: input.ProcedurePath,
			DriverPath:    input.DriverPath,
			DriverArgs:    append([]string(nil), input.DriverArgs...),
			OutputDir:     candidateDir,
			Pairs:         input.Pairs,
			CandidateID:   candidateID,
		})
		verified, verifyErr := verify(candidateDir)
		if verifyErr != nil {
			if runErr != nil {
				return fmt.Errorf("candidate %q did not produce a verifiable browser replication", candidateID)
			}
			return fmt.Errorf("candidate %q: verify browser replication: %w", candidateID, verifyErr)
		}
		if verified.CandidateID != candidateID || verified.Adapter != BrowserReplicationAdapter ||
			verified.AdapterVersion != BrowserReplicationAdapterVersion || verified.ProcedureSHA256 != procedureSHA256 ||
			verified.Scope != "outbound" || verified.ResetPolicy != BrowserReplicationResetPolicy ||
			verified.PairsPerOrder != input.Pairs {
			return fmt.Errorf("candidate %q: browser replication provenance disagrees", candidateID)
		}
		if runErr != nil && verified.Outcome != portabletrace.ReplicationUnknown {
			return fmt.Errorf("candidate %q failed after a non-unknown result", candidateID)
		}
		results = append(results, ladderResultFromBrowser(candidateID, directory, verified))
	}

	provenance := minimize.LadderProvenance{
		Adapter:         BrowserReplicationAdapter,
		AdapterVersion:  BrowserReplicationAdapterVersion,
		ProcedureSHA256: procedureSHA256,
		Scope:           "outbound",
		ResetPolicy:     BrowserReplicationResetPolicy,
	}
	summary, err := minimize.SummarizeLadder(plan, provenance, input.Pairs, results)
	if err != nil {
		return err
	}
	if err := minimize.SaveLadder(input.OutputDir, summary); err != nil {
		return err
	}
	return nil
}

// VerifyFixtureMinimization verifies the portable ladder receipt and each
// child using the browser adapter's criterion-specific comparison.
func VerifyFixtureMinimization(rootDir string) (minimize.LadderSummary, error) {
	summary, _, err := VerifyFixtureMinimizationWithIdentity(rootDir)
	return summary, err
}

// VerifyFixtureMinimizationWithIdentity verifies the portable ladder receipt,
// each child, and returns the canonical receipt identity from that same read.
func VerifyFixtureMinimizationWithIdentity(rootDir string) (minimize.LadderSummary, string, error) {
	summary, digest, err := minimize.VerifyLadder(rootDir, func(root string, summary minimize.LadderSummary, result minimize.LadderCandidateResult) error {
		child, err := VerifyFixtureReplicatedForFunctionality(filepath.Join(root, result.Directory))
		if err != nil {
			return err
		}
		if child.CandidateID != result.ID || child.Adapter != summary.Adapter || child.AdapterVersion != summary.AdapterVersion ||
			child.ProcedureSHA256 != summary.ProcedureSHA256 || child.Scope != summary.Scope ||
			child.ResetPolicy != summary.ResetPolicy || child.PairsPerOrder != summary.PairsPerOrder {
			return errors.New("browser minimization child provenance disagrees")
		}
		expected := ladderResultFromBrowser(result.ID, result.Directory, child)
		classification, err := minimize.ClassifyLadderCandidate(expected, summary.PairsPerOrder)
		if err != nil {
			return err
		}
		expected.Classification = classification
		if expected != result {
			return errors.New("browser minimization child result disagrees")
		}
		return nil
	})
	return summary, digest, err
}

// VerifyFixtureReplicatedForFunctionality uses the browser fixture's fixed
// functionality criterion. The declared account-id field is intentionally
// excluded from the functionality comparison; its disclosure is the input
// being minimized, not evidence of a product behavior change.
func VerifyFixtureReplicatedForFunctionality(rootDir string) (BrowserReplicationSummary, error) {
	return verifyFixtureReplicatedWithFields(rootDir, map[string]struct{}{"account-id": {}})
}

func validateFixtureMinimizationInput(input FixtureMinimizationInput) (minimize.LadderPlan, string, error) {
	if strings.TrimSpace(input.PlanPath) == "" || strings.TrimSpace(input.ProcedurePath) == "" || strings.TrimSpace(input.DriverPath) == "" || strings.TrimSpace(input.OutputDir) == "" {
		return minimize.LadderPlan{}, "", errors.New("browser minimization paths and driver are required")
	}
	if input.Pairs < 1 || input.Pairs > maxBrowserMinimizationPairs {
		return minimize.LadderPlan{}, "", fmt.Errorf("pairs must be between 1 and %d", maxBrowserMinimizationPairs)
	}
	plan, err := minimize.ReadLadder(input.PlanPath)
	if err != nil {
		return minimize.LadderPlan{}, "", err
	}
	if plan.Variable != "account-id" || plan.FunctionalityCriterion != BrowserFunctionalityCriterion ||
		len(plan.Candidates) != 2 || plan.Candidates[0] != BrowserFixtureReferenceCandidate || plan.Candidates[1] != BrowserFixtureOmittedCandidate {
		return minimize.LadderPlan{}, "", errors.New("browser minimization plan is not supported by the fixed fixture adapter")
	}
	_, procedureSHA256, err := validateFixtureReplicationInput(FixtureReplicationInput{
		ProcedurePath: input.ProcedurePath,
		DriverPath:    input.DriverPath,
		DriverArgs:    input.DriverArgs,
		OutputDir:     input.OutputDir,
		Pairs:         input.Pairs,
	})
	if err != nil {
		return minimize.LadderPlan{}, "", err
	}
	return plan, procedureSHA256, nil
}

func ladderResultFromBrowser(id, directory string, summary BrowserReplicationSummary) minimize.LadderCandidateResult {
	return minimize.LadderCandidateResult{
		ID:             id,
		Directory:      directory,
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

func compareFixtureSessionPairIgnoringFields(paths browserReplicationPathsValue, ignoredFields map[string]struct{}, candidateID string) (portabletrace.SessionPairComparison, error) {
	pair, err := portabletrace.VerifySessionPairWithCandidate(paths.baselineSession, paths.baselineTrace, paths.treatmentSession, paths.treatmentTrace, candidateID)
	if err != nil {
		return portabletrace.SessionPairComparison{}, err
	}
	baseline, err := portabletrace.Read(paths.baselineTrace)
	if err != nil {
		return portabletrace.SessionPairComparison{}, err
	}
	baselineSHA256, err := portabletrace.SHA256(baseline)
	if err != nil {
		return portabletrace.SessionPairComparison{}, fmt.Errorf("hash browser baseline trace: %w", err)
	}
	if baselineSHA256 != pair.BaselineTraceSHA256 {
		return portabletrace.SessionPairComparison{}, errors.New("browser baseline trace identity changed during comparison")
	}
	treatment, err := portabletrace.Read(paths.treatmentTrace)
	if err != nil {
		return portabletrace.SessionPairComparison{}, err
	}
	treatmentSHA256, err := portabletrace.SHA256(treatment)
	if err != nil {
		return portabletrace.SessionPairComparison{}, fmt.Errorf("hash browser treatment trace: %w", err)
	}
	if treatmentSHA256 != pair.TreatmentTraceSHA256 {
		return portabletrace.SessionPairComparison{}, errors.New("browser treatment trace identity changed during comparison")
	}
	comparison, err := portabletrace.Compare(removeFields(baseline, ignoredFields), removeFields(treatment, ignoredFields))
	if err != nil {
		return portabletrace.SessionPairComparison{}, err
	}
	verified, err := portabletrace.VerifySessionPairWithCandidate(paths.baselineSession, paths.baselineTrace, paths.treatmentSession, paths.treatmentTrace, candidateID)
	if err != nil {
		return portabletrace.SessionPairComparison{}, err
	}
	if verified != pair {
		return portabletrace.SessionPairComparison{}, errors.New("browser session pair identities changed during comparison")
	}
	return portabletrace.SessionPairComparison{SchemaVersion: pair.SchemaVersion, Pair: pair, Comparison: comparison}, nil
}

func removeFields(document portabletrace.Document, ignoredFields map[string]struct{}) portabletrace.Document {
	events := make([]portabletrace.Event, 0, len(document.Events))
	for _, event := range document.Events {
		fields := make([]string, 0, len(event.Fields))
		for _, field := range event.Fields {
			if _, ignored := ignoredFields[field]; !ignored {
				fields = append(fields, field)
			}
		}
		if len(fields) == 0 {
			continue
		}
		event.Fields = fields
		events = append(events, event)
	}
	document.Events = events
	return document
}

func validFixtureCandidate(value string) bool {
	return value == BrowserFixtureReferenceCandidate || value == BrowserFixtureOmittedCandidate
}
