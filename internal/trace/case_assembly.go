package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	caseAssemblyPlanSchemaVersion    = 1
	caseAssemblySummarySchemaVersion = 1
	maxCaseAssemblyPlanBytes         = 64 << 10
)

// CaseAssemblyPlanEntry identifies one already-produced trace artifact and its
// matching fixed question round. Paths are local-only inputs and are never
// copied into the assembled workspace or returned summary.
type CaseAssemblyPlanEntry struct {
	Kind              string `json:"kind"`
	ArtifactPath      string `json:"artifact_path"`
	QuestionRoundPath string `json:"question_round_path"`
}

// CaseAssemblyPlan is the local-only input for assembling a portable case and
// its disclosure-map question round in one atomic workspace.
type CaseAssemblyPlan struct {
	SchemaVersion                 int                     `json:"schema_version"`
	OrderBasis                    string                  `json:"order_basis"`
	InvestigationCommitmentSHA256 string                  `json:"investigation_commitment_sha256,omitempty"`
	Entries                       []CaseAssemblyPlanEntry `json:"entries"`
}

// CaseAssemblyQuestionSummary is the safe result of one fixed disclosure-map
// question produced during case assembly.
type CaseAssemblyQuestionSummary struct {
	QuestionID    string         `json:"question_id"`
	Result        string         `json:"result"`
	EvidenceState evidence.State `json:"evidence_state"`
}

// CaseAssemblySummary identifies the two generated portable artifacts and
// repeats only their bounded disclosure-question results. It contains no
// local paths or source-specific values.
type CaseAssemblySummary struct {
	SchemaVersion                 int                           `json:"schema_version"`
	Entries                       int                           `json:"entries"`
	InvestigationCommitmentSHA256 string                        `json:"investigation_commitment_sha256,omitempty"`
	CaseSHA256                    string                        `json:"case_sha256"`
	DisclosureRoundSHA256         string                        `json:"disclosure_round_sha256"`
	CoverageState                 evidence.State                `json:"coverage_state"`
	Questions                     []CaseAssemblyQuestionSummary `json:"questions"`
}

// ReadCaseAssemblyPlan reads and validates a local assembly plan.
func ReadCaseAssemblyPlan(path string) (CaseAssemblyPlan, error) {
	if strings.TrimSpace(path) == "" {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan path is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return CaseAssemblyPlan{}, errors.New("read trace case assembly plan")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCaseAssemblyPlanBytes+1))
	if err != nil || len(data) > maxCaseAssemblyPlanBytes {
		return CaseAssemblyPlan{}, errors.New("read trace case assembly plan")
	}
	return DecodeCaseAssemblyPlan(data)
}

// DecodeCaseAssemblyPlan validates one bounded local assembly plan document.
func DecodeCaseAssemblyPlan(data []byte) (CaseAssemblyPlan, error) {
	if len(data) == 0 {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan is empty")
	}
	if len(data) > maxCaseAssemblyPlanBytes {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan exceeds 65536-byte limit")
	}
	if !utf8.Valid(data) {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan must be valid UTF-8")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan must be a JSON object")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan has invalid JSON structure")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var plan CaseAssemblyPlan
	if err := decoder.Decode(&plan); err != nil {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan has invalid JSON fields")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CaseAssemblyPlan{}, errors.New("trace case assembly plan has trailing data")
	}
	if err := validateCaseAssemblyPlan(plan); err != nil {
		return CaseAssemblyPlan{}, err
	}
	return plan, nil
}

// AssembleCase verifies the local plan, creates a portable case and derives
// its disclosure question round from that newly written case. The destination
// must not exist; the final workspace becomes visible only after both files
// have been generated and re-verified.
func AssembleCase(planPath, outputDir string) (CaseAssemblySummary, error) {
	if strings.TrimSpace(planPath) == "" {
		return CaseAssemblySummary{}, errors.New("trace case assembly plan path is required")
	}
	if strings.TrimSpace(outputDir) == "" {
		return CaseAssemblySummary{}, errors.New("trace case assembly output directory is required")
	}
	plan, err := ReadCaseAssemblyPlan(planPath)
	if err != nil {
		return CaseAssemblySummary{}, err
	}

	outputDir = filepath.Clean(outputDir)
	if _, err := os.Lstat(outputDir); err == nil {
		return CaseAssemblySummary{}, errors.New("trace case assembly output directory already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return CaseAssemblySummary{}, errors.New("trace case assembly output directory is unavailable")
	}
	parent := filepath.Dir(outputDir)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return CaseAssemblySummary{}, errors.New("trace case assembly output parent is unavailable")
	}
	stagingDir, err := os.MkdirTemp(parent, ".ariadne-case-assembly-")
	if err != nil {
		return CaseAssemblySummary{}, errors.New("create trace case assembly workspace")
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	inputs := make([]CaseInput, 0, len(plan.Entries))
	for _, entry := range plan.Entries {
		inputs = append(inputs, CaseInput{
			Kind:              entry.Kind,
			ArtifactPath:      entry.ArtifactPath,
			QuestionRoundPath: entry.QuestionRoundPath,
		})
	}
	casePath := filepath.Join(stagingDir, "case.json")
	var caseErr error
	if plan.InvestigationCommitmentSHA256 == "" {
		_, caseErr = SaveCase(inputs, casePath)
	} else {
		_, caseErr = SaveCaseWithCommitment(plan.InvestigationCommitmentSHA256, inputs, casePath)
	}
	if caseErr != nil {
		return CaseAssemblySummary{}, fmt.Errorf("trace case assembly case: %w", caseErr)
	}
	roundPath := filepath.Join(stagingDir, "disclosure-round.json")
	if _, err := SaveCaseDisclosureQuestionRound(casePath, roundPath); err != nil {
		return CaseAssemblySummary{}, fmt.Errorf("trace case assembly disclosure round: %w", err)
	}

	summary, err := VerifyCaseAssembly(stagingDir)
	if err != nil {
		return CaseAssemblySummary{}, fmt.Errorf("trace case assembly generated files: %w", err)
	}

	if err := os.Rename(stagingDir, outputDir); err != nil {
		return CaseAssemblySummary{}, errors.New("publish trace case assembly workspace")
	}
	committed = true
	return summary, nil
}

// VerifyCaseAssembly verifies the fixed files in one assembled workspace and
// confirms that its durable disclosure round is derived from its case.
func VerifyCaseAssembly(outputDir string) (CaseAssemblySummary, error) {
	if strings.TrimSpace(outputDir) == "" {
		return CaseAssemblySummary{}, errors.New("trace case assembly workspace path is required")
	}
	outputDir = filepath.Clean(outputDir)
	info, err := os.Stat(outputDir)
	if err != nil || !info.IsDir() {
		return CaseAssemblySummary{}, errors.New("trace case assembly workspace is unavailable")
	}
	casePackage, caseSummary, err := ReadCase(filepath.Join(outputDir, "case.json"))
	if err != nil {
		return CaseAssemblySummary{}, errors.New("trace case assembly case is unavailable")
	}
	round, roundSummary, err := ReadCaseDisclosureQuestionRound(filepath.Join(outputDir, "disclosure-round.json"))
	if err != nil {
		return CaseAssemblySummary{}, errors.New("trace case assembly disclosure round is unavailable")
	}
	if roundSummary.CaseSHA256 != caseSummary.CaseSHA256 {
		return CaseAssemblySummary{}, errors.New("trace case assembly identities disagree")
	}
	expectedRound, err := AnswerCaseDisclosureQuestionRound(casePackage, caseSummary)
	if err != nil {
		return CaseAssemblySummary{}, errors.New("trace case assembly disclosure round cannot be derived")
	}
	expectedRoundSHA256, err := CaseDisclosureQuestionRoundSHA256(expectedRound)
	if err != nil || expectedRoundSHA256 != roundSummary.RoundSHA256 {
		return CaseAssemblySummary{}, errors.New("trace case assembly disclosure round does not match case")
	}
	summary := caseAssemblySummary(caseSummary, round, roundSummary)

	return summary, nil
}

func validateCaseAssemblyPlan(plan CaseAssemblyPlan) error {
	if plan.SchemaVersion != caseAssemblyPlanSchemaVersion {
		return errors.New("trace case assembly plan has unsupported schema_version")
	}
	if plan.OrderBasis != "caller" {
		return errors.New("trace case assembly plan order_basis is invalid")
	}
	if plan.InvestigationCommitmentSHA256 != "" && !ValidSHA256(plan.InvestigationCommitmentSHA256) {
		return errors.New("trace case assembly plan investigation commitment is invalid")
	}
	if plan.Entries == nil || len(plan.Entries) == 0 || len(plan.Entries) > maxCaseEntries {
		return errors.New("trace case assembly plan entries are invalid")
	}
	seen := make(map[string]struct{}, len(plan.Entries))
	for _, entry := range plan.Entries {
		if entry.Kind != CaseEntryTraceArchive && entry.Kind != CaseEntryTraceReplication {
			return errors.New("trace case assembly plan entry kind is invalid")
		}
		if !validCaseAssemblyPath(entry.ArtifactPath) || !validCaseAssemblyPath(entry.QuestionRoundPath) {
			return errors.New("trace case assembly plan entry paths are invalid")
		}
		key := entry.Kind + "\x00" + entry.ArtifactPath + "\x00" + entry.QuestionRoundPath
		if _, exists := seen[key]; exists {
			return errors.New("trace case assembly plan entries contain duplicates")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validCaseAssemblyPath(value string) bool {
	return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func caseAssemblySummary(caseSummary CaseVerificationSummary, round CaseDisclosureQuestionRound, roundSummary CaseDisclosureQuestionRoundVerificationSummary) CaseAssemblySummary {
	questions := make([]CaseAssemblyQuestionSummary, 0, len(round.Answers))
	for _, answer := range round.Answers {
		questions = append(questions, CaseAssemblyQuestionSummary{
			QuestionID:    answer.QuestionID,
			Result:        answer.Result,
			EvidenceState: answer.EvidenceState,
		})
	}
	return CaseAssemblySummary{
		SchemaVersion:                 caseAssemblySummarySchemaVersion,
		InvestigationCommitmentSHA256: caseSummary.InvestigationCommitmentSHA256,
		Entries:                       caseSummary.Entries,
		CaseSHA256:                    caseSummary.CaseSHA256,
		DisclosureRoundSHA256:         roundSummary.RoundSHA256,
		CoverageState:                 round.CoverageState,
		Questions:                     questions,
	}
}

func validateCaseAssemblySummary(summary CaseAssemblySummary) error {
	if summary.SchemaVersion != caseAssemblySummarySchemaVersion || summary.Entries <= 0 || summary.Entries > maxCaseEntries {
		return errors.New("trace case assembly summary is invalid")
	}
	if !ValidSHA256(summary.CaseSHA256) || !ValidSHA256(summary.DisclosureRoundSHA256) {
		return errors.New("trace case assembly summary identities are invalid")
	}
	if summary.InvestigationCommitmentSHA256 != "" && !ValidSHA256(summary.InvestigationCommitmentSHA256) {
		return errors.New("trace case assembly summary investigation commitment is invalid")
	}
	if summary.CoverageState != evidence.Observed && summary.CoverageState != evidence.Unknown {
		return errors.New("trace case assembly summary coverage_state is invalid")
	}
	questions := CaseDisclosureQuestions()
	if len(summary.Questions) != len(questions) {
		return errors.New("trace case assembly summary question count is invalid")
	}
	for index, answer := range summary.Questions {
		if answer.QuestionID != questions[index].ID || !validCaseAssemblyQuestionResult(answer.QuestionID, answer.Result) {
			return errors.New("trace case assembly summary question is invalid")
		}
		if answer.EvidenceState != evidence.Observed && answer.EvidenceState != evidence.Unknown {
			return errors.New("trace case assembly summary question evidence_state is invalid")
		}
	}
	return nil
}

func validCaseAssemblyQuestionResult(questionID, result string) bool {
	switch questionID {
	case CaseDisclosureQuestionCoverage:
		return result == caseDisclosureResultComplete || result == caseDisclosureResultUnknown
	case CaseDisclosureQuestionOverlap:
		return result == caseDisclosureResultOverlap || result == caseDisclosureResultNoOverlap || result == caseDisclosureResultUnknown
	default:
		return false
	}
}
