package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	androidAcceptanceSchemaVersion = 1
	androidAcceptanceWorkflow      = "experiment-001-emulator"
	androidAcceptanceManifest      = "experiment-001-email"
	androidAcceptanceVariable      = "email"
	androidAcceptancePackage       = "dev.ariadne.fixture"
	androidAcceptanceReviewMethod  = "GET"
	androidAcceptanceReviewPath    = "/"
	androidAcceptanceReviewStatus  = "self-attested"
)

// AndroidAcceptanceRecord is the raw-value-free receipt for the hosted
// Experiment 001 Android acceptance path.
type AndroidAcceptanceRecord struct {
	SchemaVersion                  int               `json:"schema_version"`
	Workflow                       string            `json:"workflow"`
	ManifestName                   string            `json:"manifest_name"`
	DeclaredVariable               string            `json:"declared_variable"`
	ManifestContractSHA256         string            `json:"manifest_contract_sha256"`
	Package                        string            `json:"package"`
	AndroidAPI                     int               `json:"android_api"`
	Architecture                   string            `json:"architecture"`
	PackageVersionCode             uint64            `json:"package_version_code"`
	PackageSHA256                  string            `json:"package_sha256"`
	AriadneRevision                string            `json:"ariadne_revision"`
	AriadneModified                bool              `json:"ariadne_modified"`
	RunEvidenceSHA256              string            `json:"run_evidence_sha256"`
	RunReportSHA256                string            `json:"run_report_sha256"`
	RunDifferences                 int               `json:"run_differences"`
	RunUnknowns                    int               `json:"run_unknowns"`
	ReplicationReceiptSHA256       string            `json:"replication_receipt_sha256"`
	ReplicationProvenanceSHA256    string            `json:"replication_provenance_sha256"`
	Outcome                        ReplicatedOutcome `json:"outcome"`
	EvidenceState                  evidence.State    `json:"evidence_state"`
	PairsPerOrder                  int               `json:"pairs_per_order"`
	CompletedPairs                 int               `json:"completed_pairs"`
	ChangedPairs                   int               `json:"changed_pairs"`
	NoChangePairs                  int               `json:"no_change_pairs"`
	UnknownPairs                   int               `json:"unknown_pairs"`
	ExportSourceEvidenceSHA256     string            `json:"export_source_evidence_sha256"`
	ExportSHA256                   string            `json:"export_sha256"`
	ReflectionSHA256               string            `json:"reflection_sha256"`
	ReflectionSourceEvidenceSHA256 string            `json:"reflection_source_evidence_sha256"`
	QuestionID                     string            `json:"question_id"`
	QuestionState                  evidence.State    `json:"question_state"`
	ReviewMethod                   string            `json:"review_method"`
	ReviewPath                     string            `json:"review_path"`
	ReviewStatus                   string            `json:"review_status"`
}

// AndroidAcceptanceVerificationSummary describes a verified acceptance receipt
// and its canonical content identity.
type AndroidAcceptanceVerificationSummary struct {
	SchemaVersion               int               `json:"schema_version"`
	Workflow                    string            `json:"workflow"`
	ManifestName                string            `json:"manifest_name"`
	DeclaredVariable            string            `json:"declared_variable"`
	ManifestContractSHA256      string            `json:"manifest_contract_sha256"`
	RunEvidenceSHA256           string            `json:"run_evidence_sha256"`
	ReplicationReceiptSHA256    string            `json:"replication_receipt_sha256"`
	ReplicationProvenanceSHA256 string            `json:"replication_provenance_sha256"`
	Outcome                     ReplicatedOutcome `json:"outcome"`
	EvidenceState               evidence.State    `json:"evidence_state"`
	QuestionID                  string            `json:"question_id"`
	QuestionState               evidence.State    `json:"question_state"`
	ReviewMethod                string            `json:"review_method"`
	ReviewPath                  string            `json:"review_path"`
	ReviewStatus                string            `json:"review_status"`
	AcceptanceSHA256            string            `json:"acceptance_sha256"`
}

// AndroidAcceptanceRecordSHA256 returns the canonical identity of one valid
// raw-value-free acceptance receipt.
func AndroidAcceptanceRecordSHA256(record AndroidAcceptanceRecord) (string, error) {
	if err := validateAndroidAcceptanceRecord(record); err != nil {
		return "", fmt.Errorf("android acceptance record: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("canonicalize android acceptance record: %w", err)
	}
	return digestSHA256(data), nil
}

// VerifyAndroidAcceptanceRecord checks an acceptance receipt without reopening
// its source artifacts. It verifies the receipt contract, not target behavior.
func VerifyAndroidAcceptanceRecord(recordPath string) (AndroidAcceptanceVerificationSummary, error) {
	if strings.TrimSpace(recordPath) == "" {
		return AndroidAcceptanceVerificationSummary{}, errors.New("android acceptance record path is required")
	}
	data, err := readFileBounded(recordPath, maxOutputBytes)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("android acceptance record: %w", err)
	}
	record, err := decodeAndroidAcceptanceRecord(data)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("android acceptance record: %w", err)
	}
	acceptanceSHA256, err := AndroidAcceptanceRecordSHA256(record)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}
	return androidAcceptanceVerificationSummary(record, acceptanceSHA256), nil
}

// SaveAndroidAcceptanceRecord verifies the current Experiment 001 bundle,
// replicated root, redacted export, and selected question reflection, then
// writes one raw-value-free acceptance receipt without overwriting it.
// reviewChecked is a caller self-attestation after the hosted GET-only review;
// it is not an authenticated server verdict.
func SaveAndroidAcceptanceRecord(runDir, replicationDir, exportPath, reflectionPath, recordPath string, reviewChecked bool) (AndroidAcceptanceVerificationSummary, error) {
	if strings.TrimSpace(recordPath) == "" {
		return AndroidAcceptanceVerificationSummary{}, errors.New("android acceptance record path is required")
	}
	if !reviewChecked {
		return AndroidAcceptanceVerificationSummary{}, errors.New("GET-only review check is required")
	}

	runSummary, err := Verify(runDir)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("run verification: %w", err)
	}
	if err := requireAuthenticatedAndroidRun(runDir); err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}

	replicationSummary, err := VerifyReplicated(replicationDir)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("replication verification: %w", err)
	}
	if err := requireAuthenticatedAndroidReplication(replicationDir); err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}

	if err := requireAndroidAcceptanceReplicationBinding(replicationDir, runSummary); err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}

	exportSummary, err := VerifyExport(exportPath)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("redacted export verification: %w", err)
	}
	reflection, reflectionSummary, err := readVerifiedArchiveQuestionReport(reflectionPath)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("question reflection verification: %w", err)
	}
	reportData, err := ReadBoundedFile(filepath.Join(runDir, "report.md"), maxOutputBytes)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("human-readable report verification: %w", err)
	}

	if err := validateAndroidAcceptanceInputs(
		runSummary, replicationSummary, exportSummary, reflection, reflectionSummary,
		digestSHA256(reportData),
	); err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}

	record := AndroidAcceptanceRecord{
		SchemaVersion:                  androidAcceptanceSchemaVersion,
		Workflow:                       androidAcceptanceWorkflow,
		ManifestName:                   runSummary.ManifestName,
		DeclaredVariable:               runSummary.DeclaredVariable,
		ManifestContractSHA256:         runSummary.ManifestContractSHA256,
		Package:                        runSummary.TargetPackage,
		AndroidAPI:                     runSummary.TargetAndroidAPI,
		Architecture:                   runSummary.TargetArchitecture,
		PackageVersionCode:             runSummary.TargetPackageVersionCode,
		PackageSHA256:                  runSummary.TargetPackageSHA256,
		AriadneRevision:                runSummary.AriadneRevision,
		AriadneModified:                runSummary.AriadneModified,
		RunEvidenceSHA256:              runSummary.EvidenceSHA256,
		RunReportSHA256:                digestSHA256(reportData),
		RunDifferences:                 runSummary.Differences,
		RunUnknowns:                    runSummary.Unknowns,
		ReplicationReceiptSHA256:       replicationSummary.ReceiptSHA256,
		ReplicationProvenanceSHA256:    replicationSummary.ProvenanceSHA256,
		Outcome:                        replicationSummary.Outcome,
		EvidenceState:                  replicationSummary.EvidenceState,
		PairsPerOrder:                  replicationSummary.PairsPerOrder,
		CompletedPairs:                 replicationSummary.CompletedPairs,
		ChangedPairs:                   replicationSummary.ChangedPairs,
		NoChangePairs:                  replicationSummary.NoChangePairs,
		UnknownPairs:                   replicationSummary.UnknownPairs,
		ExportSourceEvidenceSHA256:     exportSummary.SourceEvidenceSHA256,
		ExportSHA256:                   exportSummary.ExportSHA256,
		ReflectionSHA256:               reflectionSummary.ReflectionSHA256,
		ReflectionSourceEvidenceSHA256: reflection.Results[0].Provenance.SourceEvidenceSHA256,
		QuestionID:                     reflection.QuestionID,
		QuestionState:                  reflection.Results[0].Answer.State,
		ReviewMethod:                   androidAcceptanceReviewMethod,
		ReviewPath:                     androidAcceptanceReviewPath,
		ReviewStatus:                   androidAcceptanceReviewStatus,
	}
	acceptanceSHA256, err := AndroidAcceptanceRecordSHA256(record)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return AndroidAcceptanceVerificationSummary{}, fmt.Errorf("encode android acceptance record: %w", err)
	}
	if err := writeExclusive(recordPath, append(data, '\n')); err != nil {
		return AndroidAcceptanceVerificationSummary{}, err
	}
	return androidAcceptanceVerificationSummary(record, acceptanceSHA256), nil
}

func decodeAndroidAcceptanceRecord(data []byte) (AndroidAcceptanceRecord, error) {
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return AndroidAcceptanceRecord{}, err
	}
	var record AndroidAcceptanceRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return AndroidAcceptanceRecord{}, fmt.Errorf("decode: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return AndroidAcceptanceRecord{}, errors.New("trailing data")
	}
	return record, validateAndroidAcceptanceRecord(record)
}

func validateAndroidAcceptanceRecord(record AndroidAcceptanceRecord) error {
	if record.SchemaVersion != androidAcceptanceSchemaVersion ||
		record.Workflow != androidAcceptanceWorkflow ||
		record.ManifestName != androidAcceptanceManifest ||
		record.DeclaredVariable != androidAcceptanceVariable ||
		record.Package != androidAcceptancePackage ||
		record.Architecture != "x86_64" ||
		record.AndroidAPI != 35 ||
		record.PackageVersionCode != 1 ||
		record.AriadneModified ||
		record.QuestionID != "counterfactual-change" ||
		record.ReviewMethod != androidAcceptanceReviewMethod ||
		record.ReviewPath != androidAcceptanceReviewPath ||
		record.ReviewStatus != androidAcceptanceReviewStatus {
		return errors.New("android acceptance record contract is invalid")
	}
	for name, value := range map[string]string{
		"manifest_contract_sha256":          record.ManifestContractSHA256,
		"package_sha256":                    record.PackageSHA256,
		"ariadne_revision":                  record.AriadneRevision,
		"run_evidence_sha256":               record.RunEvidenceSHA256,
		"run_report_sha256":                 record.RunReportSHA256,
		"replication_receipt_sha256":        record.ReplicationReceiptSHA256,
		"replication_provenance_sha256":     record.ReplicationProvenanceSHA256,
		"export_source_evidence_sha256":     record.ExportSourceEvidenceSHA256,
		"export_sha256":                     record.ExportSHA256,
		"reflection_sha256":                 record.ReflectionSHA256,
		"reflection_source_evidence_sha256": record.ReflectionSourceEvidenceSHA256,
	} {
		if name == "ariadne_revision" {
			if !validRevision(value) || value == "unknown" {
				return errors.New("ariadne_revision is invalid")
			}
		} else if !validDigest(value) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	if record.RunDifferences != 1 || record.RunUnknowns != 0 ||
		record.Outcome != ReplicatedChange ||
		record.EvidenceState != evidence.Observed ||
		record.PairsPerOrder != 1 ||
		record.CompletedPairs != 2 ||
		record.ChangedPairs != 2 ||
		record.NoChangePairs != 0 ||
		record.UnknownPairs != 0 ||
		record.QuestionState != evidence.Observed ||
		record.ExportSourceEvidenceSHA256 != record.RunEvidenceSHA256 ||
		record.ReflectionSourceEvidenceSHA256 != record.RunEvidenceSHA256 {
		return errors.New("android acceptance result is invalid")
	}
	expectedProvenance, err := adb.ReplicationProvenanceSHA256(record.ManifestContractSHA256)
	if err != nil || expectedProvenance != record.ReplicationProvenanceSHA256 {
		return errors.New("android acceptance provenance is invalid")
	}
	return nil
}

func validateAndroidAcceptanceInputs(
	run Summary,
	replication ReplicatedExperimentSummary,
	export ExportVerificationSummary,
	reflection ArchiveQuestionReport,
	reflectionSummary ArchiveQuestionVerificationSummary,
	reportSHA256 string,
) error {
	if run.ManifestName != androidAcceptanceManifest ||
		run.DeclaredVariable != androidAcceptanceVariable ||
		run.ManifestContractSHA256 == "" ||
		run.TargetPackage != androidAcceptancePackage ||
		run.TargetAndroidAPI != 35 ||
		run.TargetArchitecture != "x86_64" ||
		run.TargetPackageVersionCode != 1 ||
		!validDigest(run.TargetPackageSHA256) ||
		!validRevision(run.AriadneRevision) ||
		run.AriadneRevision == "unknown" ||
		run.AriadneModified ||
		run.AnswerState != evidence.Observed ||
		run.Differences != 1 ||
		run.Unknowns != 0 ||
		!validDigest(run.EvidenceSHA256) ||
		!validDigest(reportSHA256) {
		return errors.New("android acceptance run does not match the golden contract")
	}
	if replication.ManifestName != run.ManifestName ||
		replication.DeclaredVariable != run.DeclaredVariable ||
		replication.ProvenanceSHA256 == "" ||
		replication.Outcome != ReplicatedChange ||
		replication.EvidenceState != evidence.Observed ||
		replication.PairsPerOrder != 1 ||
		replication.Pairs != 2 ||
		replication.CompletedPairs != 2 ||
		replication.ChangedPairs != 2 ||
		replication.NoChangePairs != 0 ||
		replication.UnknownPairs != 0 {
		return errors.New("android acceptance replication does not match the golden contract")
	}
	expectedProvenance, err := adb.ReplicationProvenanceSHA256(run.ManifestContractSHA256)
	if err != nil || replication.ProvenanceSHA256 != expectedProvenance {
		return errors.New("android acceptance replication provenance does not match the run")
	}
	if export.SourceEvidenceSHA256 != run.EvidenceSHA256 ||
		!validDigest(export.ExportSHA256) {
		return errors.New("android acceptance export does not match the run")
	}
	if reflection.QuestionID != "counterfactual-change" ||
		reflection.Summary != (ArchiveQuestionSummary{Observed: 1, Checked: 1}) ||
		len(reflection.Results) != 1 {
		return errors.New("android acceptance reflection does not match the golden contract")
	}
	result := reflection.Results[0]
	if result.Directory != "experiment-001" ||
		result.ManifestName != run.ManifestName ||
		!result.Available ||
		result.Provenance == nil ||
		result.Answer == nil ||
		result.Provenance.ManifestContractSHA256 != run.ManifestContractSHA256 ||
		result.Provenance.SourceEvidenceSHA256 != run.EvidenceSHA256 ||
		result.Answer.State != evidence.Observed ||
		result.Answer.QuestionID != reflection.QuestionID ||
		reflectionSummary.ReflectionSHA256 == "" {
		return errors.New("android acceptance reflection binding is invalid")
	}
	return nil
}

func requireAuthenticatedAndroidRun(runDir string) error {
	seen := make(map[string]struct{}, 2)
	for _, kind := range []string{"baseline", "treatment"} {
		session, err := loadSession(runDir, kind)
		if err != nil {
			return fmt.Errorf("authenticated %s session: %w", kind, err)
		}
		if session.record.SchemaVersion < 8 ||
			session.record.Order != adb.ReplicationOrderBaselineTreatment {
			return errors.New("authenticated session boundary is unavailable")
		}
		if _, exists := seen[session.record.ChallengeCommitment]; exists {
			return errors.New("authenticated session challenges are reused")
		}
		seen[session.record.ChallengeCommitment] = struct{}{}
	}
	return nil
}

func requireAuthenticatedAndroidReplication(rootDir string) error {
	record, _, err := readReplicatedRecord(rootDir)
	if err != nil {
		return fmt.Errorf("authenticated replication metadata: %w", err)
	}
	if record.ProvenanceSHA256 == "" {
		return errors.New("authenticated replication provenance is unavailable")
	}
	seen := make(map[string]struct{}, len(record.Pairs)*2)
	for _, pair := range record.Pairs {
		if pair.Status != adb.ReplicationStatusComplete {
			continue
		}
		for _, kind := range []string{pair.FirstSession, pair.SecondSession} {
			session, err := loadSession(filepath.Join(rootDir, pair.Directory), kind)
			if err != nil {
				return fmt.Errorf("authenticated replication session: %w", err)
			}
			if session.record.SchemaVersion < 8 {
				return errors.New("authenticated replication session boundary is unavailable")
			}
			if _, exists := seen[session.record.ChallengeCommitment]; exists {
				return errors.New("authenticated replication challenges are reused")
			}
			seen[session.record.ChallengeCommitment] = struct{}{}
		}
	}
	return nil
}

func requireAndroidAcceptanceReplicationBinding(rootDir string, run Summary) error {
	record, _, err := readReplicatedRecord(rootDir)
	if err != nil {
		return fmt.Errorf("android acceptance replication metadata: %w", err)
	}
	for _, pair := range record.Pairs {
		if pair.Status != adb.ReplicationStatusComplete {
			continue
		}
		for _, kind := range []string{pair.FirstSession, pair.SecondSession} {
			session, err := loadSession(filepath.Join(rootDir, pair.Directory), kind)
			if err != nil {
				return fmt.Errorf("android acceptance replication session: %w", err)
			}
			target := session.record
			if target.ManifestName != run.ManifestName ||
				target.DeclaredVariable != run.DeclaredVariable ||
				target.ManifestContractSHA256 != run.ManifestContractSHA256 ||
				target.ADBVersion != run.TargetADBVersion ||
				target.Package != run.TargetPackage ||
				target.AndroidAPI != run.TargetAndroidAPI ||
				target.Architecture != run.TargetArchitecture ||
				target.PackageVersionCode != run.TargetPackageVersionCode ||
				target.PackageSHA256 != run.TargetPackageSHA256 ||
				target.AriadneRevision != run.AriadneRevision ||
				target.AriadneModified != run.AriadneModified {
				return errors.New("android acceptance replication target does not match run")
			}
		}
	}
	return nil
}

func androidAcceptanceVerificationSummary(record AndroidAcceptanceRecord, acceptanceSHA256 string) AndroidAcceptanceVerificationSummary {
	return AndroidAcceptanceVerificationSummary{
		SchemaVersion:               record.SchemaVersion,
		Workflow:                    record.Workflow,
		ManifestName:                record.ManifestName,
		DeclaredVariable:            record.DeclaredVariable,
		ManifestContractSHA256:      record.ManifestContractSHA256,
		RunEvidenceSHA256:           record.RunEvidenceSHA256,
		ReplicationReceiptSHA256:    record.ReplicationReceiptSHA256,
		ReplicationProvenanceSHA256: record.ReplicationProvenanceSHA256,
		Outcome:                     record.Outcome,
		EvidenceState:               record.EvidenceState,
		QuestionID:                  record.QuestionID,
		QuestionState:               record.QuestionState,
		ReviewMethod:                record.ReviewMethod,
		ReviewPath:                  record.ReviewPath,
		ReviewStatus:                record.ReviewStatus,
		AcceptanceSHA256:            acceptanceSHA256,
	}
}
