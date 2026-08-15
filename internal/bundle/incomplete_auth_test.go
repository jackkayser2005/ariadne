package bundle

import (
	"crypto/sha256"
	"encoding/hex"
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

func challengeCommitmentForTest(challenge string) string {
	digest := sha256.Sum256([]byte(challenge))
	return hex.EncodeToString(digest[:])
}
