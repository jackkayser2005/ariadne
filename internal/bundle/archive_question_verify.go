package bundle

import (
	"bytes"
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
	SchemaVersion int    `json:"schema_version"`
	QuestionID    string `json:"question_id"`
	Checked       int    `json:"checked"`
}

// VerifyArchiveQuestionReport checks a saved archive reflection report
// without requiring its source archive. It validates the report contract, not
// the truth of its answers or the evidence referenced by its digests.
func VerifyArchiveQuestionReport(reportPath string) (ArchiveQuestionVerificationSummary, error) {
	if strings.TrimSpace(reportPath) == "" {
		return ArchiveQuestionVerificationSummary{}, errors.New("archive question report path is required")
	}
	data, err := readFileBounded(reportPath, maxOutputBytes)
	if err != nil {
		return ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	report, err := decodeArchiveQuestionReport(data)
	if err != nil {
		return ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	if err := validateArchiveQuestionReport(report); err != nil {
		return ArchiveQuestionVerificationSummary{}, fmt.Errorf("archive question report: %w", err)
	}
	return ArchiveQuestionVerificationSummary{
		SchemaVersion: report.SchemaVersion,
		QuestionID:    report.QuestionID,
		Checked:       report.Summary.Checked,
	}, nil
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
	return report, nil
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
