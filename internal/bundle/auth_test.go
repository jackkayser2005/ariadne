package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/adb"
)

func TestAuthenticatedSessionReceiptValidation(t *testing.T) {
	challengeBaseline := strings.Repeat("d", 64)
	challengeTreatment := strings.Repeat("e", 64)
	commitment := func(challenge string) string {
		digest := sha256.Sum256([]byte(challenge))
		return hex.EncodeToString(digest[:])
	}
	baselineBody := `{"schema_version":1,"challenge":"` + challengeBaseline + `","region":"us-east","variant":"standard"}`
	treatmentBody := `{"schema_version":1,"challenge":"` + challengeTreatment + `","region":"us-east","variant":"personalized"}`
	mutate := func(kind, challenge string) func(*adb.SessionRecord) {
		return func(record *adb.SessionRecord) {
			record.ChallengeCommitment = challenge
			record.Role = kind
			record.Order = adb.ReplicationOrderBaselineTreatment
			record.ProcedureSHA256 = record.ManifestContractSHA256
		}
	}
	runDir := makeRun(t, runOptions{
		sessionSchemaVersion: 8,
		baselineStorage:      baselineBody,
		baselineNetworkBody:  baselineBody,
		treatmentStorage:     treatmentBody,
		treatmentNetworkBody: treatmentBody,
		mutateBaseline:       mutate("baseline", commitment(challengeBaseline)),
		mutateTreatment:      mutate("treatment", commitment(challengeTreatment)),
	})

	baseline, err := loadSession(runDir, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := loadSession(runDir, "treatment")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePair(baseline.record, treatment.record); err != nil {
		t.Fatalf("validatePair() error = %v", err)
	}
	if _, err := Write(runDir); err != nil {
		t.Fatalf("Write() authenticated run error = %v", err)
	}
	for _, path := range []string{"evidence.json", "report.md"} {
		data, err := os.ReadFile(filepath.Join(runDir, path))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), challengeBaseline) || strings.Contains(string(data), challengeTreatment) {
			t.Fatalf("%s exposed raw authentication challenge", path)
		}
	}

	treatment.record.ChallengeCommitment = baseline.record.ChallengeCommitment
	if err := validatePair(baseline.record, treatment.record); err == nil {
		t.Fatal("validatePair() accepted reused challenge commitment")
	}
}

func TestAuthenticatedEvidenceBindingRejectsMissingOrChangedChallenge(t *testing.T) {
	challengeBaseline := strings.Repeat("d", 64)
	challengeTreatment := strings.Repeat("e", 64)
	commitment := func(challenge string) string {
		digest := sha256.Sum256([]byte(challenge))
		return hex.EncodeToString(digest[:])
	}
	baselineBody := `{"schema_version":1,"challenge":"` + challengeBaseline + `","region":"us-east","variant":"standard"}`
	treatmentBody := `{"schema_version":1,"challenge":"` + challengeTreatment + `","region":"us-east","variant":"personalized"}`
	tests := []struct {
		name            string
		baselineBody    string
		treatmentBody   string
		baselineCommit  string
		treatmentCommit string
		want            string
	}{
		{
			name:            "missing",
			baselineBody:    `{"schema_version":1,"region":"us-east","variant":"standard"}`,
			treatmentBody:   treatmentBody,
			baselineCommit:  commitment(challengeBaseline),
			treatmentCommit: commitment(challengeTreatment),
			want:            "unavailable",
		},
		{
			name:            "baseline mismatch",
			baselineBody:    baselineBody,
			treatmentBody:   treatmentBody,
			baselineCommit:  strings.Repeat("f", 64),
			treatmentCommit: commitment(challengeTreatment),
			want:            "not bound",
		},
		{
			name:            "treatment mismatch",
			baselineBody:    baselineBody,
			treatmentBody:   treatmentBody,
			baselineCommit:  commitment(challengeBaseline),
			treatmentCommit: strings.Repeat("f", 64),
			want:            "not bound",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := makeRun(t, runOptions{
				sessionSchemaVersion: 8,
				baselineStorage:      test.baselineBody,
				baselineNetworkBody:  test.baselineBody,
				treatmentStorage:     test.treatmentBody,
				treatmentNetworkBody: test.treatmentBody,
				mutateBaseline: func(record *adb.SessionRecord) {
					record.ChallengeCommitment = test.baselineCommit
					record.Role = "baseline"
					record.Order = adb.ReplicationOrderBaselineTreatment
					record.ProcedureSHA256 = record.ManifestContractSHA256
				},
				mutateTreatment: func(record *adb.SessionRecord) {
					record.ChallengeCommitment = test.treatmentCommit
					record.Role = "treatment"
					record.Order = adb.ReplicationOrderBaselineTreatment
					record.ProcedureSHA256 = record.ManifestContractSHA256
				},
			})
			_, err := Write(runDir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Write() error = %v, want containing %q", err, test.want)
			}
		})
	}
}
func TestAuthenticatedSessionReceiptRejectsMissingProtocolFields(t *testing.T) {
	runDir := makeRun(t, runOptions{sessionSchemaVersion: 8})
	baseline, err := loadSession(runDir, "baseline")
	if err == nil || !strings.Contains(err.Error(), "challenge_commitment") {
		t.Fatalf("loadSession() error = %v", err)
	}
	_ = baseline
}

func TestExperimentTraceOmitsChallenge(t *testing.T) {
	challenge := strings.Repeat("a", 64)
	fields, err := experimentTraceObservationFields([]byte(
		`{"schema_version":1,"challenge":"` + challenge + `","region":"us-east","request_id":"request-1","variant":"standard"}`,
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields[0] != "region" || fields[1] != "session-id" {
		t.Fatalf("trace fields = %v", fields)
	}
}
