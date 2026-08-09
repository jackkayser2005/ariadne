package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAskArchiveReturnsOrderedSafeResults(t *testing.T) {
	root := t.TempDir()
	archiveRun(t, root, "z-run", runOptions{})
	archiveRun(t, root, "a-run", runOptions{})

	report, err := AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 2 || report.QuestionID != "counterfactual-change" || report.Question == "" {
		t.Fatalf("AskArchive() question = %#v", report)
	}
	if report.Summary.Checked != 2 || report.Summary.Observed != 2 || report.Summary.Unknown != 0 || report.Summary.Unavailable != 0 {
		t.Fatalf("AskArchive() summary = %#v", report.Summary)
	}
	if len(report.Results) != 2 || report.Results[0].Directory != "a-run" || report.Results[1].Directory != "z-run" {
		t.Fatalf("AskArchive() results = %#v", report.Results)
	}
	for _, result := range report.Results {
		evidence, err := os.ReadFile(filepath.Join(root, result.Directory, "evidence.json"))
		if err != nil {
			t.Fatal(err)
		}
		evidenceDigest := sha256.Sum256(evidence)
		if !result.Available || result.Answer == nil || result.Answer.State != "observed" || result.RecordedAt == "" || result.Provenance == nil || result.Provenance.ManifestContractSHA256 != strings.Repeat("c", 64) || result.Provenance.SourceEvidenceSHA256 != hex.EncodeToString(evidenceDigest[:]) || result.Provenance.AriadneRevision != strings.Repeat("b", 40) || result.Provenance.AriadneModified {
			t.Fatalf("AskArchive() result = %#v", result)
		}
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, rawValue := range []string{
		"standard",
		"personalized",
		"baseline@example.invalid",
		"treatment@example.invalid",
		"emulator-5554",
	} {
		if strings.Contains(string(data), rawValue) {
			t.Fatalf("AskArchive() exposed raw value %q: %s", rawValue, data)
		}
	}
}

func TestAskArchiveCountsUnknownAndUnavailable(t *testing.T) {
	root := t.TempDir()
	archiveRun(t, root, "complete", runOptions{})

	storageRun := makeStorageFailureRun(t, "")
	storageDir := filepath.Join(root, "storage-gap")
	if err := os.Rename(storageRun, storageDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(storageDir); err != nil {
		t.Fatal(err)
	}

	legacyDir := filepath.Join(root, "legacy")
	legacyRun := makeRun(t, runOptions{sessionSchemaVersion: 5})
	if err := os.Rename(legacyRun, legacyDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(legacyDir); err != nil {
		t.Fatal(err)
	}

	report, err := AskArchive(root, "counterfactual-change")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Checked != 3 || report.Summary.Observed != 1 || report.Summary.Unknown != 1 || report.Summary.Unavailable != 1 {
		t.Fatalf("AskArchive() summary = %#v", report.Summary)
	}
	for _, result := range report.Results {
		if result.Directory == "storage-gap" && (result.Answer == nil || result.Answer.State != "unknown") {
			t.Fatalf("storage-gap result = %#v", result)
		}
		if result.Directory == "legacy" && result.Available {
			t.Fatalf("legacy result = %#v", result)
		}
		if result.Directory == "legacy" && result.Provenance != nil {
			t.Fatalf("legacy provenance = %#v", result.Provenance)
		}
	}
}

func TestAskArchiveRejectsInvalidQuestion(t *testing.T) {
	if _, err := AskArchive(t.TempDir(), "not-a-question"); err == nil || !strings.Contains(err.Error(), "question ID is invalid") {
		t.Fatalf("AskArchive() error = %v", err)
	}
}

func TestSortArchiveQuestionResults(t *testing.T) {
	results := []ArchiveQuestionResult{
		{Directory: "run-z", RecordedAt: "2026-07-25T12:00:00Z"},
		{Directory: "legacy"},
		{Directory: "run-old", RecordedAt: "2026-07-24T12:00:00Z"},
		{Directory: "run-fraction", RecordedAt: "2026-07-25T12:00:00.1Z"},
		{Directory: "run-a", RecordedAt: "2026-07-25T12:00:00Z"},
	}
	sortArchiveQuestionResults(results)
	want := []string{"run-old", "run-a", "run-z", "run-fraction", "legacy"}
	for index, result := range results {
		if result.Directory != want[index] {
			t.Fatalf("sortArchiveQuestionResults() = %#v, want %v", results, want)
		}
	}
}
