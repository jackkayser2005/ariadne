package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func validAndroidAcceptanceRecordForTest(t *testing.T) AndroidAcceptanceRecord {
	t.Helper()
	contract := strings.Repeat("c", 64)
	provenance, err := adb.ReplicationProvenanceSHA256(contract)
	if err != nil {
		t.Fatal(err)
	}
	evidenceSHA256 := strings.Repeat("d", 64)
	return AndroidAcceptanceRecord{
		SchemaVersion:                  1,
		Workflow:                       "experiment-001-emulator",
		ManifestName:                   "experiment-001-email",
		DeclaredVariable:               "email",
		ManifestContractSHA256:         contract,
		Package:                        "dev.ariadne.fixture",
		AndroidAPI:                     35,
		Architecture:                   "x86_64",
		PackageVersionCode:             1,
		PackageSHA256:                  strings.Repeat("a", 64),
		AriadneRevision:                strings.Repeat("b", 40),
		RunEvidenceSHA256:              evidenceSHA256,
		RunReportSHA256:                strings.Repeat("e", 64),
		RunDifferences:                 1,
		RunUnknowns:                    0,
		ReplicationReceiptSHA256:       strings.Repeat("f", 64),
		ReplicationProvenanceSHA256:    provenance,
		Outcome:                        ReplicatedChange,
		EvidenceState:                  evidence.Observed,
		PairsPerOrder:                  1,
		CompletedPairs:                 2,
		ChangedPairs:                   2,
		NoChangePairs:                  0,
		UnknownPairs:                   0,
		ExportSourceEvidenceSHA256:     evidenceSHA256,
		ExportSHA256:                   strings.Repeat("9", 64),
		ReflectionSHA256:               strings.Repeat("7", 64),
		ReflectionSourceEvidenceSHA256: evidenceSHA256,
		QuestionID:                     "counterfactual-change",
		QuestionState:                  evidence.Observed,
		ReviewMethod:                   "GET",
		ReviewPath:                     "/",
		ReviewStatus:                   "self-attested",
	}
}

func writeAndroidAcceptanceRecordForTest(t *testing.T, record AndroidAcceptanceRecord) string {
	t.Helper()
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "acceptance.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyAndroidAcceptanceRecord(t *testing.T) {
	record := validAndroidAcceptanceRecordForTest(t)
	path := writeAndroidAcceptanceRecordForTest(t, record)

	summary, err := VerifyAndroidAcceptanceRecord(path)
	if err != nil {
		t.Fatalf("VerifyAndroidAcceptanceRecord() error = %v", err)
	}
	wantSHA256, err := AndroidAcceptanceRecordSHA256(record)
	if err != nil {
		t.Fatal(err)
	}
	if summary.AcceptanceSHA256 != wantSHA256 ||
		summary.Workflow != record.Workflow ||
		summary.Outcome != ReplicatedChange ||
		summary.EvidenceState != evidence.Observed ||
		summary.ReviewMethod != "GET" ||
		summary.ReviewPath != "/" {
		t.Fatalf("summary = %#v", summary)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"standard", "personalized", "emulator-5554", "challenge"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("acceptance record exposed %q: %s", secret, data)
		}
	}
}

func TestVerifyAndroidAcceptanceRecordRejectsUnsafeOrInvalidInput(t *testing.T) {
	valid := validAndroidAcceptanceRecordForTest(t)
	tests := []struct {
		name   string
		mutate func(*AndroidAcceptanceRecord)
		want   string
	}{
		{name: "wrong workflow", mutate: func(record *AndroidAcceptanceRecord) { record.Workflow = "other" }, want: "contract"},
		{name: "raw revision", mutate: func(record *AndroidAcceptanceRecord) { record.AriadneRevision = "unknown" }, want: "ariadne_revision"},
		{name: "invalid digest", mutate: func(record *AndroidAcceptanceRecord) { record.ExportSHA256 = "not-a-digest" }, want: "export_sha256"},
		{name: "no change outcome", mutate: func(record *AndroidAcceptanceRecord) { record.Outcome = NoChangeObserved }, want: "result"},
		{name: "unknown evidence", mutate: func(record *AndroidAcceptanceRecord) { record.EvidenceState = evidence.Unknown }, want: "result"},
		{name: "review path", mutate: func(record *AndroidAcceptanceRecord) { record.ReviewPath = "/run" }, want: "contract"},
		{name: "mismatched source", mutate: func(record *AndroidAcceptanceRecord) {
			record.ReflectionSourceEvidenceSHA256 = strings.Repeat("0", 64)
		}, want: "result"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			test.mutate(&record)
			path := writeAndroidAcceptanceRecordForTest(t, record)
			if _, err := VerifyAndroidAcceptanceRecord(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyAndroidAcceptanceRecord() error = %v, want containing %q", err, test.want)
			}
			if _, err := AndroidAcceptanceRecordSHA256(record); err == nil {
				t.Fatal("AndroidAcceptanceRecordSHA256() accepted invalid record")
			}
		})
	}

	if _, err := VerifyAndroidAcceptanceRecord(""); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty path error = %v", err)
	}
	if _, err := VerifyAndroidAcceptanceRecord(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing path accepted")
	}
}

func TestVerifyAndroidAcceptanceRecordRejectsMalformedJSON(t *testing.T) {
	record := validAndroidAcceptanceRecordForTest(t)
	validData, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "invalid JSON"},
		{name: "unknown field", data: append(append([]byte{}, validData[:len(validData)-1]...), []byte(",\"extra\":true}")...), want: "unknown field"},
		{name: "duplicate field", data: []byte("{\"schema_version\":1,\"schema_version\":1}"), want: "duplicate"},
		{name: "trailing data", data: append(append([]byte{}, validData...), []byte("{}")...), want: "trailing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "acceptance.json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyAndroidAcceptanceRecord(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifyAndroidAcceptanceRecord() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func acceptanceObservationBody(challenge, variant string) string {
	data, err := json.Marshal(map[string]interface{}{
		"schema_version": 1,
		"challenge":      challenge,
		"region":         "us-east",
		"variant":        variant,
	})
	if err != nil {
		panic(err)
	}
	return string(data)
}

func acceptanceRunOptions(order, baselineChallenge, treatmentChallenge string) runOptions {
	return runOptions{
		sessionSchemaVersion: 8,
		baselineStorage:      acceptanceObservationBody(baselineChallenge, "standard"),
		baselineNetworkBody:  acceptanceObservationBody(baselineChallenge, "standard"),
		treatmentStorage:     acceptanceObservationBody(treatmentChallenge, "personalized"),
		treatmentNetworkBody: acceptanceObservationBody(treatmentChallenge, "personalized"),
		mutateBaseline: func(record *adb.SessionRecord) {
			record.ChallengeCommitment = challengeCommitmentForTest(baselineChallenge)
			record.Role = "baseline"
			record.Order = order
			record.ProcedureSHA256 = record.ManifestContractSHA256
		},
		mutateTreatment: func(record *adb.SessionRecord) {
			record.ChallengeCommitment = challengeCommitmentForTest(treatmentChallenge)
			record.Role = "treatment"
			record.Order = order
			record.ProcedureSHA256 = record.ManifestContractSHA256
		},
	}
}

func makeAuthenticatedAcceptanceRun(t *testing.T, destination, order, baselineChallenge, treatmentChallenge string) string {
	t.Helper()
	options := acceptanceRunOptions(order, baselineChallenge, treatmentChallenge)
	source := makeRun(t, options)
	if order == adb.ReplicationOrderTreatmentBaseline {
		shiftSession(t, filepath.Join(source, "treatment", "session.json"), -30*time.Second)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	renameTestRun(t, source, destination)
	if _, err := Write(destination); err != nil {
		t.Fatalf("Write(%s): %v", destination, err)
	}
	return destination
}

func makeAuthenticatedAcceptanceReplication(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "replicated")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	contract := strings.Repeat("c", 64)
	provenance, err := adb.ReplicationProvenanceSHA256(contract)
	if err != nil {
		t.Fatal(err)
	}
	pairs := []adb.ReplicatedPairRecord{
		{
			Pair:          1,
			Order:         adb.ReplicationOrderBaselineTreatment,
			Directory:     "pair-001-baseline-treatment",
			FirstSession:  "baseline",
			SecondSession: "treatment",
			Status:        adb.ReplicationStatusComplete,
		},
		{
			Pair:          1,
			Order:         adb.ReplicationOrderTreatmentBaseline,
			Directory:     "pair-001-treatment-baseline",
			FirstSession:  "treatment",
			SecondSession: "baseline",
			Status:        adb.ReplicationStatusComplete,
		},
	}
	makeAuthenticatedAcceptanceRun(
		t,
		filepath.Join(root, pairs[0].Directory),
		pairs[0].Order,
		strings.Repeat("d", 64),
		strings.Repeat("e", 64),
	)
	makeAuthenticatedAcceptanceRun(
		t,
		filepath.Join(root, pairs[1].Directory),
		pairs[1].Order,
		strings.Repeat("f", 64),
		strings.Repeat("9", 64),
	)

	record := adb.ReplicatedRunRecord{
		SchemaVersion:    adb.ReplicatedRunSchemaVersion,
		ManifestName:     "experiment-001-email",
		DeclaredVariable: "email",
		PairsPerOrder:    1,
		ResetPolicy:      adb.ReplicationResetPolicy,
		ProvenanceSHA256: provenance,
		Status:           adb.ReplicationStatusComplete,
		CompletedPairs:   2,
		Pairs:            pairs,
	}
	writeReplicatedRecordForTest(t, root, record)
	return root
}

func TestSaveAndroidAcceptanceRecord(t *testing.T) {
	archiveRoot := t.TempDir()
	runDir := makeAuthenticatedAcceptanceRun(
		t,
		filepath.Join(archiveRoot, "experiment-001"),
		adb.ReplicationOrderBaselineTreatment,
		strings.Repeat("d", 64),
		strings.Repeat("e", 64),
	)
	replicationRoot := makeAuthenticatedAcceptanceReplication(t)
	exportPath := filepath.Join(t.TempDir(), "redacted-export.json")
	if _, err := Export(runDir, exportPath); err != nil {
		t.Fatalf("Export() error = %v", err)
	}
	reflectionPath := filepath.Join(archiveRoot, "reflection.json")
	if _, err := SaveArchiveQuestionReport(archiveRoot, "counterfactual-change", reflectionPath); err != nil {
		t.Fatalf("SaveArchiveQuestionReport() error = %v", err)
	}
	recordPath := filepath.Join(t.TempDir(), "acceptance.json")
	summary, err := SaveAndroidAcceptanceRecord(runDir, replicationRoot, exportPath, reflectionPath, recordPath, true)
	if err != nil {
		t.Fatalf("SaveAndroidAcceptanceRecord() error = %v", err)
	}
	if summary.Outcome != ReplicatedChange ||
		summary.EvidenceState != evidence.Observed ||
		summary.QuestionState != evidence.Observed ||
		summary.ReviewMethod != "GET" ||
		summary.ReviewStatus != "self-attested" {
		t.Fatalf("summary = %#v", summary)
	}
	if _, err := VerifyAndroidAcceptanceRecord(recordPath); err != nil {
		t.Fatalf("saved acceptance record does not verify: %v", err)
	}
	data, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		"standard",
		"personalized",
		"emulator-5554",
		"challenge_commitment",
	} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("saved acceptance exposed %q: %s", secret, data)
		}
	}
	if _, err := SaveAndroidAcceptanceRecord(runDir, replicationRoot, exportPath, reflectionPath, recordPath, true); err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("second save error = %v", err)
	}
	if _, err := SaveAndroidAcceptanceRecord(runDir, replicationRoot, exportPath, reflectionPath, filepath.Join(t.TempDir(), "other.json"), false); err == nil || !strings.Contains(err.Error(), "review") {
		t.Fatalf("review requirement error = %v", err)
	}
}

func TestAndroidAcceptanceRequiresAuthenticatedBoundaries(t *testing.T) {
	runDir := makeRun(t, runOptions{})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveAndroidAcceptanceRecord(
		runDir,
		filepath.Join(t.TempDir(), "replicated"),
		filepath.Join(t.TempDir(), "export.json"),
		filepath.Join(t.TempDir(), "reflection.json"),
		filepath.Join(t.TempDir(), "acceptance.json"),
		true,
	); err == nil || !strings.Contains(err.Error(), "authenticated") {
		t.Fatalf("SaveAndroidAcceptanceRecord() error = %v", err)
	}
}

func TestAndroidAcceptanceReplicationMustMatchRunTarget(t *testing.T) {
	runDir := makeAuthenticatedAcceptanceRun(
		t,
		filepath.Join(t.TempDir(), "experiment-001"),
		adb.ReplicationOrderBaselineTreatment,
		strings.Repeat("d", 64),
		strings.Repeat("e", 64),
	)
	runSummary, err := Verify(runDir)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	replicationRoot := makeAuthenticatedAcceptanceReplication(t)
	tests := []struct {
		name   string
		mutate func(*Summary)
	}{
		{name: "adb version", mutate: func(summary *Summary) { summary.TargetADBVersion = "other" }},
		{name: "package", mutate: func(summary *Summary) { summary.TargetPackage = "other.package" }},
		{name: "android api", mutate: func(summary *Summary) { summary.TargetAndroidAPI = 34 }},
		{name: "architecture", mutate: func(summary *Summary) { summary.TargetArchitecture = "arm64-v8a" }},
		{name: "package version", mutate: func(summary *Summary) { summary.TargetPackageVersionCode = 2 }},
		{name: "package digest", mutate: func(summary *Summary) { summary.TargetPackageSHA256 = strings.Repeat("9", 64) }},
		{name: "revision", mutate: func(summary *Summary) { summary.AriadneRevision = strings.Repeat("e", 40) }},
		{name: "modified", mutate: func(summary *Summary) { summary.AriadneModified = true }},
		{name: "manifest contract", mutate: func(summary *Summary) { summary.ManifestContractSHA256 = strings.Repeat("1", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := runSummary
			test.mutate(&mutated)
			if err := requireAndroidAcceptanceReplicationBinding(replicationRoot, mutated); err == nil ||
				!strings.Contains(err.Error(), "does not match run") {
				t.Fatalf("requireAndroidAcceptanceReplicationBinding() error = %v", err)
			}
		})
	}
}

func TestRequireAuthenticatedAndroidReplicationRejectsMissingOrReusedBoundary(t *testing.T) {
	root := makeAuthenticatedAcceptanceReplication(t)
	if err := requireAuthenticatedAndroidReplication(root); err != nil {
		t.Fatalf("valid replication boundary error = %v", err)
	}

	recordData, err := os.ReadFile(filepath.Join(root, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record adb.ReplicatedRunRecord
	if err := json.Unmarshal(recordData, &record); err != nil {
		t.Fatal(err)
	}
	record.ProvenanceSHA256 = ""
	writeReplicatedRecordForTest(t, root, record)
	if err := requireAuthenticatedAndroidReplication(root); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("missing provenance error = %v", err)
	}

	record.ProvenanceSHA256, err = adb.ReplicationProvenanceSHA256(strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	writeReplicatedRecordForTest(t, root, record)
	sessionPath := filepath.Join(root, "pair-001-treatment-baseline", "treatment", "session.json")
	sessionData, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	var session adb.SessionRecord
	if err := json.Unmarshal(sessionData, &session); err != nil {
		t.Fatal(err)
	}
	session.ChallengeCommitment = challengeCommitmentForTest(strings.Repeat("d", 64))
	sessionData, err = json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, append(sessionData, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireAuthenticatedAndroidReplication(root); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("reused challenge error = %v", err)
	}
}
