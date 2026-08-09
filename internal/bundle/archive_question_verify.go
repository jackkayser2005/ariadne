package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

// ArchiveQuestionVerificationSummary describes a structurally valid derived
// reflection report. It does not prove the underlying evidence.
type ArchiveQuestionVerificationSummary struct {
	SchemaVersion    int    `json:"schema_version"`
	QuestionID       string `json:"question_id"`
	Checked          int    `json:"checked"`
	ReflectionSHA256 string `json:"reflection_sha256"`
}

// VerifyArchiveQuestionReport checks a saved archive reflection report
// without requiring its source archive. It validates the report contract, not
// the truth of its answers or the evidence referenced by its digests.
func VerifyArchiveQuestionReport(reportPath string) (ArchiveQuestionVerificationSummary, error) {
	_, summary, err := readVerifiedArchiveQuestionReport(reportPath)
	return summary, err
}

// ArchiveQuestionReportReflectionSHA256 returns the canonical identity of a
// structurally valid, raw-value-free archive question report.
func ArchiveQuestionReportReflectionSHA256(report ArchiveQuestionReport) (string, error) {
	if err := validateArchiveQuestionReport(report); err != nil {
		return "", fmt.Errorf("archive question report: %w", err)
	}
	return archiveQuestionReflectionSHA256(report)
}

func readVerifiedArchiveQuestionReport(reportPath string) (ArchiveQuestionReport, ArchiveQuestionVerificationSummary, error) {
	if strings.TrimSpace(reportPath) == "" {
		return ArchiveQuestionReport{}, ArchiveQuestionVerificationSummary{}, errors.New("archive question report path is required")
	}
	data, err := readFileBounded(reportPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionReport{}, ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	report, err := decodeArchiveQuestionReport(data)
	if err != nil {
		return ArchiveQuestionReport{}, ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	if err := validateArchiveQuestionReport(report); err != nil {
		return ArchiveQuestionReport{}, ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	reflectionSHA256, err := archiveQuestionReflectionSHA256(report)
	if err != nil {
		return ArchiveQuestionReport{}, ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	summary := ArchiveQuestionVerificationSummary{
		SchemaVersion:    report.SchemaVersion,
		QuestionID:       report.QuestionID,
		Checked:          report.Summary.Checked,
		ReflectionSHA256: reflectionSHA256,
	}
	return report, summary, nil
}

func archiveQuestionReflectionSHA256(report ArchiveQuestionReport) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("canonicalize: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func decodeArchiveQuestionReport(data []byte) (ArchiveQuestionReport, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return ArchiveQuestionReport{}, err
	}
	var report ArchiveQuestionReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return ArchiveQuestionReport{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ArchiveQuestionReport{}, errors.New("trailing data")
	}
	if err := validateArchiveQuestionJSON(data); err != nil {
		return ArchiveQuestionReport{}, err
	}
	return report, nil
}

func validateArchiveQuestionJSON(data []byte) error {
	root, err := archiveQuestionObject(data,
		[]string{"schema_version", "question_id", "question", "summary", "results"}, nil)
	if err != nil {
		return err
	}
	if _, err := archiveQuestionObject(root["summary"],
		[]string{"observed", "unknown", "unavailable", "checked"}, nil); err != nil {
		return fmt.Errorf("summary: %w", err)
	}
	results, err := archiveQuestionArray(root["results"], "results")
	if err != nil {
		return err
	}
	for _, rawResult := range results {
		result, err := archiveQuestionObject(rawResult,
			[]string{"directory", "manifest_name", "available"},
			[]string{"recorded_at", "provenance", "answer"})
		if err != nil {
			return fmt.Errorf("result: %w", err)
		}
		if provenance, ok := result["provenance"]; ok {
			if _, err := archiveQuestionObject(provenance,
				[]string{"manifest_contract_sha256", "source_evidence_sha256", "ariadne_revision", "ariadne_modified"}, nil); err != nil {
				return fmt.Errorf("provenance: %w", err)
			}
		}
		if answer, ok := result["answer"]; ok {
			answerObject, err := archiveQuestionObject(answer,
				[]string{"question_id", "question", "answer_state", "finding_ids"},
				[]string{"reason"})
			if err != nil {
				return fmt.Errorf("answer: %w", err)
			}
			if _, err := archiveQuestionArray(answerObject["finding_ids"], "finding_ids"); err != nil {
				return err
			}
		}
	}
	return nil
}

func archiveQuestionObject(data []byte, required, optional []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("object: %w", err)
	}
	if object == nil {
		return nil, errors.New("object is required")
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = struct{}{}
	}
	for _, key := range optional {
		allowed[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("unknown field %q", key)
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return nil, fmt.Errorf("missing required field %q", key)
		}
	}
	return object, nil
}

func archiveQuestionArray(data []byte, field string) ([]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("%s must be an array", field)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("%s must be an array: %w", field, err)
	}
	return values, nil
}

func validateArchiveQuestionReport(report ArchiveQuestionReport) error {
	if report.SchemaVersion != archiveQuestionReportSchemaVersion {
		return errors.New("unsupported schema_version")
	}
	catalogQuestion, ok := questionForID(report.QuestionID)
	if !ok {
		return errors.New("question ID is invalid")
	}
	if report.Question != catalogQuestion.Text {
		return errors.New("question text does not match catalog")
	}

	expectedSummary := ArchiveQuestionSummary{Checked: len(report.Results)}
	seenDirectories := make(map[string]struct{}, len(report.Results))
	for _, result := range report.Results {
		if err := validateArchiveQuestionResult(result, report.QuestionID, report.Question); err != nil {
			return err
		}
		if _, exists := seenDirectories[result.Directory]; exists {
			return errors.New("archive result directory is duplicated")
		}
		seenDirectories[result.Directory] = struct{}{}
		if !result.Available {
			expectedSummary.Unavailable++
			continue
		}
		switch result.Answer.State {
		case evidence.Observed:
			expectedSummary.Observed++
		case evidence.Unknown:
			expectedSummary.Unknown++
		}
	}
	if report.Summary != expectedSummary {
		return errors.New("summary does not match results")
	}

	ordered := slices.Clone(report.Results)
	sortArchiveQuestionResults(ordered)
	for index := range report.Results {
		if report.Results[index].Directory != ordered[index].Directory {
			return errors.New("results are not ordered chronologically")
		}
	}
	return nil
}

func validateArchiveQuestionResult(result ArchiveQuestionResult, questionID, question string) error {
	if !validArchiveEntryName(result.Directory) || result.ManifestName == "" || !validMetadataValue(result.ManifestName) {
		return errors.New("archive result metadata is invalid")
	}
	if result.RecordedAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, result.RecordedAt); err != nil {
			return errors.New("archive result recorded_at is invalid")
		}
	}
	if result.Provenance != nil {
		if !validDigest(result.Provenance.ManifestContractSHA256) ||
			!validDigest(result.Provenance.SourceEvidenceSHA256) ||
			!validRevision(result.Provenance.AriadneRevision) {
			return errors.New("archive result provenance is invalid")
		}
	}
	if !result.Available {
		if result.Answer != nil {
			return errors.New("unavailable archive result has an answer")
		}
		return nil
	}
	if result.Answer == nil || result.Provenance == nil {
		return errors.New("available archive result is incomplete")
	}
	if result.Answer.QuestionID != questionID || result.Answer.Question != question {
		return errors.New("archive answer does not match question")
	}
	if result.Answer.State != evidence.Observed && result.Answer.State != evidence.Unknown {
		return errors.New("archive answer state is invalid")
	}
	if result.Answer.Reason != "" {
		if result.Answer.State != evidence.Unknown || safeUnknownReason(result.Answer.Reason) == "" {
			return errors.New("archive answer reason is invalid")
		}
	}
	if result.Answer.FindingIDs == nil {
		return errors.New("archive answer finding_ids are required")
	}
	for _, findingID := range result.Answer.FindingIDs {
		if !validFindingID(findingID) {
			return errors.New("archive answer finding ID is invalid")
		}
	}
	return nil
}
