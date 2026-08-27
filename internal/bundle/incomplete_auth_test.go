package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestIncompleteAuthenticatedEvidenceBecomesUnknown(t *testing.T) {
	baselineChallenge := strings.Repeat("a", 64)
	treatmentChallenge := strings.Repeat("b", 64)
	baselineBody := `{"schema_version":1,"challenge":"` + baselineChallenge + `","region":"us-east","variant":"standard"}`
	treatmentBody := `{"schema_version":1,"challenge":"` + treatmentChallenge + `","region":"us-east","variant":"personalized"}`
	commitment := func(challenge string) string {
		return challengeCommitmentForTest(challenge)
	}
	runDir := makeRun(t, runOptions{
		sessionSchemaVersion: 8,
		baselineStorage:      baselineBody,
		baselineNetworkBody:  baselineBody,
		treatmentNetworkBody: treatmentBody,
		mutateBaseline: func(record *adb.SessionRecord) {
			record.ChallengeCommitment = commitment(baselineChallenge)
			record.Role = "baseline"
			record.Order = adb.ReplicationOrderBaselineTreatment
			record.ProcedureSHA256 = record.ManifestContractSHA256
		},
		mutateTreatment: func(record *adb.SessionRecord) {
			record.Status = "incomplete"
			record.FailureStage = "capture_network"
			record.ChallengeCommitment = strings.Repeat("f", 64)
			record.Role = "treatment"
			record.Order = adb.ReplicationOrderBaselineTreatment
			record.ProcedureSHA256 = record.ManifestContractSHA256
			record.Artifacts = record.Artifacts[:1]
			for index := range record.Steps {
				if record.Steps[index].Name == "capture_network" || record.Steps[index].Name == "capture_storage" {
					record.Steps[index].Status = "error"
					record.Steps[index].ExitCode = -1
				}
			}
		},
	})

	summary, err := Write(runDir)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if summary.AnswerState != evidence.Unknown || summary.Unknowns == 0 {
		t.Fatalf("summary = %#v, want unknown comparison", summary)
	}
}

func TestIncompleteAuthenticatedNetworkCaptureWithoutArtifactBecomesUnknown(t *testing.T) {
	baselineChallenge := strings.Repeat("a", 64)
	treatmentChallenge := strings.Repeat("b", 64)
	baselineBody := `{"schema_version":1,"challenge":"` + baselineChallenge + `","region":"us-east","variant":"standard"}`
	treatmentBody := `{"schema_version":1,"challenge":"` + treatmentChallenge + `","region":"us-east","variant":"personalized"}`
	runDir := makeRun(t, runOptions{
		sessionSchemaVersion: 8,
		baselineStorage:      baselineBody,
		baselineNetworkBody:  baselineBody,
		treatmentStorage:     treatmentBody,
		treatmentNetworkBody: treatmentBody,
		mutateBaseline: func(record *adb.SessionRecord) {
			record.ChallengeCommitment = challengeCommitmentForTest(baselineChallenge)
			record.Role = "baseline"
			record.Order = adb.ReplicationOrderBaselineTreatment
			record.ProcedureSHA256 = record.ManifestContractSHA256
		},
		mutateTreatment: func(record *adb.SessionRecord) {
			record.Status = "incomplete"
			record.FailureStage = "capture_network"
			record.ChallengeCommitment = challengeCommitmentForTest(treatmentChallenge)
			record.Role = "treatment"
			record.Order = adb.ReplicationOrderBaselineTreatment
			record.ProcedureSHA256 = record.ManifestContractSHA256
			record.Artifacts = nil
			for index := range record.Steps {
				if record.Steps[index].Name == "capture_network" || record.Steps[index].Name == "capture_storage" {
					record.Steps[index].Status = "error"
					record.Steps[index].ExitCode = -1
				}
			}
		},
	})
	for _, path := range []string{"network.json", "storage.json"} {
		if err := os.Remove(filepath.Join(runDir, "treatment", "observations", path)); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := Write(runDir)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if summary.AnswerState != evidence.Unknown || summary.Unknowns == 0 {
		t.Fatalf("summary = %#v, want unknown comparison", summary)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "evidence.json"))
	if err != nil || !strings.Contains(string(data), treatmentNetworkObservationUnknownReason) {
		t.Fatalf("network-unavailable evidence = %s, %v", data, err)
	}
}

func challengeCommitmentForTest(challenge string) string {
	digest := sha256.Sum256([]byte(challenge))
	return hex.EncodeToString(digest[:])
}
