package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func writeArchiveQuestionRoundArtifact(t *testing.T, path string) (string, trace.ArchiveQuestionRound) {
	t.Helper()
	archiveSHA256 := strings.Repeat("a", 64)
	sources := []trace.ArchiveSourceSummary{{Source: "browser", Adapter: "fixture", Entries: 1}}
	results := []string{"complete", "same", "available"}
	questions := trace.ArchiveQuestions()
	answers := make([]trace.ArchiveAnswer, len(questions))
	for index, question := range questions {
		answers[index] = trace.ArchiveAnswer{
			SchemaVersion: 1,
			QuestionID:    question.ID,
			Question:      question.Text,
			Result:        results[index],
			EvidenceState: evidence.Observed,
			ArchiveSHA256: archiveSHA256,
			Entries:       1,
			Compared:      1,
			Same:          1,
			Sources:       sources,
		}
	}
	round := trace.ArchiveQuestionRound{
		SchemaVersion: 1,
		OrderBasis:    "caller",
		ArchiveSHA256: archiveSHA256,
		Entries:       1,
		Complete:      1,
		Sources:       sources,
		Answers:       answers,
	}
	data, err := json.Marshal(round)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := trace.VerifyArchiveQuestionRound(path)
	if err != nil {
		t.Fatal(err)
	}
	return summary.RoundSHA256, round
}

func writeArchiveQuestionReceiptArtifact(t *testing.T, path, roundSHA256 string) string {
	t.Helper()
	question := trace.ArchiveQuestions()[0]
	receipt := trace.ArchiveQuestionReceipt{
		SchemaVersion: 1,
		QuestionID:    question.ID,
		Question:      question.Text,
		Result:        "complete",
		EvidenceState: evidence.Observed,
		ArchiveSHA256: strings.Repeat("a", 64),
		RoundSHA256:   roundSHA256,
		Entries:       1,
		Compared:      1,
		Same:          1,
		Sources:       []trace.ArchiveSourceSummary{{Source: "browser", Adapter: "fixture", Entries: 1}},
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := trace.VerifyArchiveQuestionReceipt(path)
	if err != nil {
		t.Fatal(err)
	}
	return summary.ReceiptSHA256
}

func TestValidatePortableQuestionArtifacts(t *testing.T) {
	root := t.TempDir()
	roundPath := filepath.Join(root, "archive-question-round.json")
	roundSHA256, _ := writeArchiveQuestionRoundArtifact(t, roundPath)
	receiptPath := filepath.Join(root, "archive-question-receipt.json")
	receiptSHA256 := writeArchiveQuestionReceiptArtifact(t, receiptPath, roundSHA256)

	roundReport := Validate(roundPath)
	if roundReport.ArtifactKind != KindQuestionRound ||
		roundReport.Overall != StatusWarning ||
		roundReport.Identity != roundSHA256 ||
		roundReport.EvidenceState != evidence.Unknown ||
		roundReport.Reason != ReasonProvenanceUnavailable ||
		tierStatus(roundReport, TierBoundary) != StatusUnavailable ||
		tierStatus(roundReport, TierReplay) != StatusUnavailable {
		t.Fatalf("round report = %#v", roundReport)
	}
	receiptReport := Validate(receiptPath)
	if receiptReport.ArtifactKind != KindQuestionReceipt ||
		receiptReport.Overall != StatusWarning ||
		receiptReport.Identity != receiptSHA256 ||
		receiptReport.EvidenceState != evidence.Unknown ||
		receiptReport.Reason != ReasonProvenanceUnavailable ||
		tierStatus(receiptReport, TierBoundary) != StatusUnavailable ||
		tierStatus(receiptReport, TierReplay) != StatusUnavailable {
		t.Fatalf("receipt report = %#v", receiptReport)
	}
	roundEncoded, err := json.Marshal(roundReport)
	if err != nil {
		t.Fatal(err)
	}
	receiptEncoded, err := json.Marshal(receiptReport)
	if err != nil {
		t.Fatal(err)
	}
	for _, encoded := range [][]byte{roundEncoded, receiptEncoded} {
		for _, secret := range []string{"browser", "fixture", "complete", "same"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("question-artifact report exposed %q: %s", secret, encoded)
			}
		}
	}
}

func TestValidateRejectsMalformedPortableQuestionArtifacts(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name string
		kind ArtifactKind
	}{
		{name: "round.json", kind: KindQuestionRound},
		{name: "question-receipt.json", kind: KindQuestionReceipt},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name)
			if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			assertRejected(t, Validate(path), test.kind)
		})
	}
}
