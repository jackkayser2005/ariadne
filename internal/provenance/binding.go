package provenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

const (
	// BindingSchemaVersion is the version of the canonical execution envelope.
	BindingSchemaVersion = 1

	// Binding kinds identify the level of the execution envelope being hashed.
	SessionBindingKind     = "android-session"
	PairBindingKind        = "android-pair"
	ReplicationBindingKind = "android-replication"
	EvidenceBindingKind    = "android-replication-evidence"
)

// TargetBinding contains safe target identities. DeviceSHA256 binds the exact
// target without putting a device serial into portable output.
type TargetBinding struct {
	ADBVersion         string `json:"adb_version"`
	DeviceSHA256       string `json:"device_sha256"`
	Package            string `json:"package"`
	AndroidAPI         int    `json:"android_api"`
	Architecture       string `json:"architecture"`
	PackageVersionCode uint64 `json:"package_version_code"`
	PackageSHA256      string `json:"package_sha256"`
	AriadneRevision    string `json:"ariadne_revision"`
	AriadneModified    bool   `json:"ariadne_modified"`
}

// ArtifactBinding identifies one already-captured artifact without copying its
// contents into the envelope.
type ArtifactBinding struct {
	Kind      string `json:"kind"`
	Source    string `json:"source"`
	Path      string `json:"path"`
	SizeBytes int    `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// StepBinding identifies one session step without recording arguments or
// command output.
type StepBinding struct {
	Name              string    `json:"name"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	Status            string    `json:"status"`
	ExitCode          int       `json:"exit_code"`
	UIHierarchySHA256 string    `json:"ui_hierarchy_sha256,omitempty"`
}

// SessionBinding is the canonical safe identity of one authenticated Android
// session. It intentionally excludes persona values, payloads, and paths that
// are not fixed artifact names.
type SessionBinding struct {
	SchemaVersion          int               `json:"schema_version"`
	Kind                   string            `json:"kind"`
	Source                 string            `json:"source"`
	Adapter                string            `json:"adapter"`
	AdapterVersion         int               `json:"adapter_version"`
	Scope                  string            `json:"scope"`
	ManifestName           string            `json:"manifest_name"`
	DeclaredVariable       string            `json:"declared_variable"`
	PersonaFields          int               `json:"persona_fields"`
	VolatileFields         []string          `json:"volatile_fields,omitempty"`
	TapResourceID          string            `json:"tap_resource_id"`
	ManifestContractSHA256 string            `json:"manifest_contract_sha256"`
	ProvenanceSHA256       string            `json:"provenance_sha256"`
	ChallengeCommitment    string            `json:"challenge_commitment"`
	Role                   string            `json:"role"`
	Order                  string            `json:"order"`
	ProcedureSHA256        string            `json:"procedure_sha256"`
	ResetPolicy            string            `json:"reset_policy"`
	Target                 TargetBinding     `json:"target"`
	Status                 string            `json:"status"`
	FailureStage           string            `json:"failure_stage,omitempty"`
	StartedAt              time.Time         `json:"started_at"`
	FinishedAt             time.Time         `json:"finished_at"`
	Steps                  []StepBinding     `json:"steps"`
	Artifacts              []ArtifactBinding `json:"artifacts,omitempty"`
}

// PairBinding is the canonical identity of one ordered baseline/treatment
// pair. Session bindings are referenced by digest so the pair remains small.
type PairBinding struct {
	SchemaVersion              int    `json:"schema_version"`
	Kind                       string `json:"kind"`
	Source                     string `json:"source"`
	Adapter                    string `json:"adapter"`
	AdapterVersion             int    `json:"adapter_version"`
	Scope                      string `json:"scope"`
	ManifestName               string `json:"manifest_name"`
	DeclaredVariable           string `json:"declared_variable"`
	ManifestContractSHA256     string `json:"manifest_contract_sha256"`
	ProvenanceSHA256           string `json:"provenance_sha256"`
	ProcedureSHA256            string `json:"procedure_sha256"`
	ResetPolicy                string `json:"reset_policy"`
	Pair                       int    `json:"pair"`
	Order                      string `json:"order"`
	Directory                  string `json:"directory"`
	FirstSession               string `json:"first_session"`
	SecondSession              string `json:"second_session"`
	FirstSessionBindingSHA256  string `json:"first_session_binding_sha256"`
	SecondSessionBindingSHA256 string `json:"second_session_binding_sha256"`
}

// PairReference identifies a pair binding in a replication envelope.
type PairReference struct {
	Pair          int    `json:"pair"`
	Order         string `json:"order"`
	BindingSHA256 string `json:"binding_sha256"`
}

// ReplicationBinding is the canonical identity of a complete ordered
// replication. It binds the root metadata to every pair binding.
type ReplicationBinding struct {
	SchemaVersion          int             `json:"schema_version"`
	Kind                   string          `json:"kind"`
	Source                 string          `json:"source"`
	Adapter                string          `json:"adapter"`
	AdapterVersion         int             `json:"adapter_version"`
	Scope                  string          `json:"scope"`
	ManifestName           string          `json:"manifest_name"`
	DeclaredVariable       string          `json:"declared_variable"`
	ManifestContractSHA256 string          `json:"manifest_contract_sha256"`
	ProvenanceSHA256       string          `json:"provenance_sha256"`
	PairsPerOrder          int             `json:"pairs_per_order"`
	ResetPolicy            string          `json:"reset_policy"`
	Pairs                  []PairReference `json:"pairs"`
}

// EvidencePairReference binds a verified evidence document to its ordered
// pair envelope.
type EvidencePairReference struct {
	Pair              int    `json:"pair"`
	Order             string `json:"order"`
	PairBindingSHA256 string `json:"pair_binding_sha256"`
	EvidenceSHA256    string `json:"evidence_sha256"`
}

// EvidenceBinding is the portable aggregate identity returned after each
// complete pair's evidence document has been verified.
type EvidenceBinding struct {
	SchemaVersion          int                     `json:"schema_version"`
	Kind                   string                  `json:"kind"`
	Source                 string                  `json:"source"`
	Adapter                string                  `json:"adapter"`
	AdapterVersion         int                     `json:"adapter_version"`
	Scope                  string                  `json:"scope"`
	ManifestName           string                  `json:"manifest_name"`
	DeclaredVariable       string                  `json:"declared_variable"`
	ManifestContractSHA256 string                  `json:"manifest_contract_sha256"`
	ProvenanceSHA256       string                  `json:"provenance_sha256"`
	ResetPolicy            string                  `json:"reset_policy"`
	RootBindingSHA256      string                  `json:"root_binding_sha256"`
	ReceiptSHA256          string                  `json:"receipt_sha256"`
	Pairs                  []EvidencePairReference `json:"pairs"`
}

// SHA256 returns the canonical JSON identity of a validated session binding.
func (binding SessionBinding) SHA256() (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return canonicalSHA256(binding)
}

// SHA256 returns the canonical JSON identity of a validated pair binding.
func (binding PairBinding) SHA256() (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return canonicalSHA256(binding)
}

// SHA256 returns the canonical JSON identity of a validated replication
// binding.
func (binding ReplicationBinding) SHA256() (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return canonicalSHA256(binding)
}

// SHA256 returns the canonical JSON identity of a validated evidence binding.
func (binding EvidenceBinding) SHA256() (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	return canonicalSHA256(binding)
}

// Validate reports whether the session binding contains only bounded identity
// data.
func (binding SessionBinding) Validate() error {
	if err := validateCommon(binding.SchemaVersion, binding.Kind, binding.Source, binding.Adapter, binding.AdapterVersion, binding.Scope); err != nil {
		return err
	}
	if binding.Kind != SessionBindingKind {
		return errors.New("session binding kind is invalid")
	}
	if !validText(binding.ManifestName, 1024) || !validText(binding.DeclaredVariable, 128) ||
		binding.PersonaFields < 1 || binding.PersonaFields > 64 ||
		!validDigest(binding.ManifestContractSHA256) || !validDigest(binding.ProvenanceSHA256) ||
		!validDigest(binding.ChallengeCommitment) || binding.Role == "" || binding.Order == "" ||
		!validDigest(binding.ProcedureSHA256) || binding.ResetPolicy == "" {
		return errors.New("session binding identity is invalid")
	}
	if binding.Role != "baseline" && binding.Role != "treatment" {
		return errors.New("session binding role is invalid")
	}
	if binding.Order != "baseline-treatment" && binding.Order != "treatment-baseline" {
		return errors.New("session binding order is invalid")
	}
	if err := binding.Target.Validate(); err != nil {
		return err
	}
	if binding.Status != "complete" && binding.Status != "incomplete" {
		return errors.New("session binding status is invalid")
	}
	if binding.Status == "complete" && binding.FailureStage != "" {
		return errors.New("complete session binding has a failure stage")
	}
	if binding.Status == "incomplete" && !validBindingLabel(binding.FailureStage, 64) {
		return errors.New("incomplete session binding failure stage is invalid")
	}
	if binding.StartedAt.IsZero() || binding.FinishedAt.IsZero() || binding.FinishedAt.Before(binding.StartedAt) {
		return errors.New("session binding timestamps are invalid")
	}
	if len(binding.VolatileFields) > 64 || !sortedLabels(binding.VolatileFields, 128) {
		return errors.New("session binding volatile fields are invalid")
	}
	if binding.TapResourceID != "" && !validBindingLabel(binding.TapResourceID, 512) {
		return errors.New("session binding tap resource is invalid")
	}
	if len(binding.Steps) == 0 || len(binding.Steps) > 16 {
		return errors.New("session binding steps are invalid")
	}
	for _, step := range binding.Steps {
		if !validBindingLabel(step.Name, 64) || (step.Status != "ok" && step.Status != "error") ||
			step.StartedAt.IsZero() || step.FinishedAt.IsZero() || step.FinishedAt.Before(step.StartedAt) ||
			step.ExitCode < -1 || step.ExitCode > 255 ||
			(step.UIHierarchySHA256 != "" && !validDigest(step.UIHierarchySHA256)) {
			return errors.New("session binding step is invalid")
		}
	}
	if len(binding.Artifacts) > 8 {
		return errors.New("session binding artifacts are invalid")
	}
	for _, artifact := range binding.Artifacts {
		if !validBindingLabel(artifact.Kind, 128) || !validSource(artifact.Source) ||
			!validRelativePath(artifact.Path) || artifact.SizeBytes < 0 || artifact.SizeBytes > 256<<20 ||
			!validDigest(artifact.SHA256) {
			return errors.New("session binding artifact is invalid")
		}
	}
	return nil
}

// Validate reports whether the pair binding contains bounded pair identities.
func (binding PairBinding) Validate() error {
	if err := validateCommon(binding.SchemaVersion, binding.Kind, binding.Source, binding.Adapter, binding.AdapterVersion, binding.Scope); err != nil {
		return err
	}
	if binding.Kind != PairBindingKind {
		return errors.New("pair binding kind is invalid")
	}
	if !validText(binding.ManifestName, 1024) || !validText(binding.DeclaredVariable, 128) ||
		!validDigest(binding.ManifestContractSHA256) || !validDigest(binding.ProvenanceSHA256) ||
		!validDigest(binding.ProcedureSHA256) || binding.ResetPolicy == "" || binding.Pair < 1 || binding.Pair > 8 ||
		(binding.Order != "baseline-treatment" && binding.Order != "treatment-baseline") || !validRelativePath(binding.Directory) ||
		(binding.FirstSession != "baseline" && binding.FirstSession != "treatment") ||
		(binding.SecondSession != "baseline" && binding.SecondSession != "treatment") ||
		!validDigest(binding.FirstSessionBindingSHA256) || !validDigest(binding.SecondSessionBindingSHA256) {
		return errors.New("pair binding identity is invalid")
	}
	if binding.Order == "baseline-treatment" && (binding.FirstSession != "baseline" || binding.SecondSession != "treatment") {
		return errors.New("pair binding order is invalid")
	}
	if binding.Order == "treatment-baseline" && (binding.FirstSession != "treatment" || binding.SecondSession != "baseline") {
		return errors.New("pair binding order is invalid")
	}
	return nil
}

// Validate reports whether the replication binding contains every pair in a
// canonical order.
func (binding ReplicationBinding) Validate() error {
	if err := validateCommon(binding.SchemaVersion, binding.Kind, binding.Source, binding.Adapter, binding.AdapterVersion, binding.Scope); err != nil {
		return err
	}
	if binding.Kind != ReplicationBindingKind {
		return errors.New("replication binding kind is invalid")
	}
	if !validText(binding.ManifestName, 1024) || !validText(binding.DeclaredVariable, 128) ||
		!validDigest(binding.ManifestContractSHA256) || !validDigest(binding.ProvenanceSHA256) ||
		binding.PairsPerOrder < 1 || binding.PairsPerOrder > 8 || binding.ResetPolicy == "" {
		return errors.New("replication binding identity is invalid")
	}
	if len(binding.Pairs) != binding.PairsPerOrder*2 {
		return errors.New("replication binding pair count is invalid")
	}
	for index, pair := range binding.Pairs {
		expectedPair := index/2 + 1
		expectedOrder := "baseline-treatment"
		if index%2 == 1 {
			expectedOrder = "treatment-baseline"
		}
		if pair.Pair != expectedPair || pair.Order != expectedOrder || !validDigest(pair.BindingSHA256) {
			return errors.New("replication binding pair is invalid")
		}
	}
	return nil
}

// Validate reports whether the evidence binding contains complete, ordered
// evidence identities.
func (binding EvidenceBinding) Validate() error {
	if err := validateCommon(binding.SchemaVersion, binding.Kind, binding.Source, binding.Adapter, binding.AdapterVersion, binding.Scope); err != nil {
		return err
	}
	if binding.Kind != EvidenceBindingKind {
		return errors.New("evidence binding kind is invalid")
	}
	if !validText(binding.ManifestName, 1024) || !validText(binding.DeclaredVariable, 128) ||
		!validDigest(binding.ManifestContractSHA256) || !validDigest(binding.ProvenanceSHA256) ||
		binding.ResetPolicy == "" || !validDigest(binding.RootBindingSHA256) || !validDigest(binding.ReceiptSHA256) ||
		len(binding.Pairs) < 2 || len(binding.Pairs) > 16 || len(binding.Pairs)%2 != 0 {
		return errors.New("evidence binding identity is invalid")
	}
	for index, pair := range binding.Pairs {
		expectedPair := index/2 + 1
		expectedOrder := "baseline-treatment"
		if index%2 == 1 {
			expectedOrder = "treatment-baseline"
		}
		if pair.Pair != expectedPair || pair.Order != expectedOrder ||
			!validDigest(pair.PairBindingSHA256) || !validDigest(pair.EvidenceSHA256) {
			return errors.New("evidence binding pair is invalid")
		}
	}
	return nil
}

// Validate reports whether the target binding contains only safe identity data.
func (target TargetBinding) Validate() error {
	if !validBindingLabel(target.ADBVersion, 128) || !validDigest(target.DeviceSHA256) ||
		!validBindingLabel(target.Package, 255) || target.AndroidAPI < 1 || target.AndroidAPI > 999 ||
		!validBindingLabel(target.Architecture, 128) || target.PackageVersionCode == 0 ||
		!validDigest(target.PackageSHA256) || !validRevision(target.AriadneRevision) {
		return errors.New("target binding identity is invalid")
	}
	return nil
}

func validateCommon(schema int, kind, source, adapter string, adapterVersion int, scope string) error {
	if schema != BindingSchemaVersion || !validBindingLabel(kind, 64) || !validBindingLabel(source, 64) ||
		!validBindingLabel(adapter, 128) || adapterVersion < 1 || adapterVersion > 32 || !validBindingLabel(scope, 64) {
		return errors.New("binding identity is invalid")
	}
	return nil
}

func canonicalSHA256(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode binding: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// SHA256String returns a lowercase SHA-256 digest for a private identity input. Callers must not use the input itself as portable output.
func SHA256String(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validText(value string, limit int) bool {
	return value != "" && len(value) <= limit && strings.TrimSpace(value) == value &&
		!strings.ContainsFunc(value, unicode.IsControl)
}
func validBindingLabel(value string, limit int) bool {
	if value == "" || len(value) > limit || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && !strings.ContainsRune("._:/-", character) {
			return false
		}
	}
	return true
}

func validSource(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'
		if !letter && !digit && !strings.ContainsRune(" ._:/-", character) {
			return false
		}
	}
	return true
}
func sortedLabels(values []string, limit int) bool {
	previous := ""
	for _, value := range values {
		if !validBindingLabel(value, limit) || (previous != "" && value <= previous) {
			return false
		}
		previous = value
	}
	return true
}

func validRelativePath(value string) bool {
	if !validBindingLabel(value, 512) || strings.Contains(value, ":") || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
		return false
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validRevision(value string) bool {
	if value == "unknown" {
		return true
	}
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	if len(value) == 40 {
		for _, character := range value {
			if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
				return false
			}
		}
		return true
	}
	return validDigest(value)
}
