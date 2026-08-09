package bundle

import (
	"errors"
	"path/filepath"
	"sort"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

// ArchiveQuestionSummary counts the safe outcomes of one archive question.
type ArchiveQuestionSummary struct {
	Observed    int `json:"observed"`
	Unknown     int `json:"unknown"`
	Unavailable int `json:"unavailable"`
	Checked     int `json:"checked"`
}

// ArchiveQuestionResult is the safe result for one archive entry.
type ArchiveQuestionResult struct {
	Directory    string  `json:"directory"`
	ManifestName string  `json:"manifest_name"`
	RecordedAt   string  `json:"recorded_at,omitempty"`
	Answer       *Answer `json:"answer,omitempty"`
	Available    bool    `json:"available"`
}

// ArchiveQuestionReport is the deterministic, raw-value-free answer to one
// bounded question across an archive.
type ArchiveQuestionReport struct {
	QuestionID string                  `json:"question_id"`
	Question   string                  `json:"question"`
	Summary    ArchiveQuestionSummary  `json:"summary"`
	Results    []ArchiveQuestionResult `json:"results"`
}

// AskArchive re-verifies one bounded question across immediate archive
// entries. Entries that cannot answer the current question remain visible as
// unavailable without exposing verifier errors.
func AskArchive(archiveRoot, questionID string) (ArchiveQuestionReport, error) {
	catalogQuestion, ok := questionForID(questionID)
	if !ok {
		return ArchiveQuestionReport{}, errors.New("question ID is invalid")
	}
	entries, err := Index(archiveRoot)
	if err != nil {
		return ArchiveQuestionReport{}, err
	}

	report := ArchiveQuestionReport{
		QuestionID: questionID,
		Question:   catalogQuestion.Text,
		Results:    make([]ArchiveQuestionResult, 0, len(entries)),
	}
	for _, entry := range entries {
		result := ArchiveQuestionResult{
			Directory:    entry.Directory,
			ManifestName: entry.ManifestName,
		}
		runDir := filepath.Join(archiveRoot, entry.Directory)
		verified, verifyErr := Verify(runDir)
		if verifyErr == nil {
			result.RecordedAt = verified.RecordedAt
			answer, askErr := Ask(runDir, questionID)
			if askErr == nil {
				result.Answer = &answer
				result.Available = true
			}
		}

		report.Summary.Checked++
		if !result.Available {
			report.Summary.Unavailable++
		} else {
			switch result.Answer.State {
			case evidence.Observed:
				report.Summary.Observed++
			case evidence.Unknown:
				report.Summary.Unknown++
			}
		}
		report.Results = append(report.Results, result)
	}
	sortArchiveQuestionResults(report.Results)
	return report, nil
}

func sortArchiveQuestionResults(results []ArchiveQuestionResult) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if left.RecordedAt == right.RecordedAt {
			return left.Directory < right.Directory
		}
		if left.RecordedAt == "" {
			return false
		}
		if right.RecordedAt == "" {
			return true
		}
		return left.RecordedAt < right.RecordedAt
	})
}
