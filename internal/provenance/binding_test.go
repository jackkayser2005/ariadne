package provenance

import (
	"strings"
	"testing"
	"time"
)

func bindingDigest() string {
	return strings.Repeat("a", 64)
}

func validTargetBindingForTest() TargetBinding {
	return TargetBinding{
		ADBVersion:         "1.0.41",
		DeviceSHA256:       bindingDigest(),
		Package:            "dev.ariadne.fixture",
		AndroidAPI:         35,
		Architecture:       "x86_64",
		PackageVersionCode: 1,
		PackageSHA256:      strings.Repeat("b", 64),
		AriadneRevision:    strings.Repeat("c", 40),
	}
}

func validSessionBindingForTest() SessionBinding {
	started := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	finished := started.Add(15 * time.Second)
	return SessionBinding{
		SchemaVersion:          BindingSchemaVersion,
		Kind:                   SessionBindingKind,
		Source:                 "android",
		Adapter:                "android-experiment-001",
		AdapterVersion:         1,
		Scope:                  "all",
		ManifestName:           "experiment-001-email",
		DeclaredVariable:       "email",
		PersonaFields:          2,
		VolatileFields:         []string{"request_id"},
		TapResourceID:          "dev.ariadne.fixture:id/observe_button",
		ManifestContractSHA256: bindingDigest(),
		ProvenanceSHA256:       strings.Repeat("d", 64),
		ChallengeCommitment:    strings.Repeat("e", 64),
		Role:                   "baseline",
		Order:                  "baseline-treatment",
		ProcedureSHA256:        bindingDigest(),
		ResetPolicy:            "reset-before-each-session",
		Target:                 validTargetBindingForTest(),
		Status:                 "complete",
		StartedAt:              started,
		FinishedAt:             finished,
		Steps: []StepBinding{{
			Name:       "reset",
			StartedAt:  started.Add(time.Second),
			FinishedAt: started.Add(2 * time.Second),
			Status:     "ok",
			ExitCode:   0,
		}},
		Artifacts: []ArtifactBinding{{
			Kind:      "http_request",
			Source:    "POST /observe",
			Path:      "observations/network.json",
			SizeBytes: 1,
			SHA256:    strings.Repeat("f", 64),
		}},
	}
}

func validPairBindingForTest() PairBinding {
	return PairBinding{
		SchemaVersion:              BindingSchemaVersion,
		Kind:                       PairBindingKind,
		Source:                     "android",
		Adapter:                    "android-experiment-001",
		AdapterVersion:             1,
		Scope:                      "all",
		ManifestName:               "experiment-001-email",
		DeclaredVariable:           "email",
		ManifestContractSHA256:     bindingDigest(),
		ProvenanceSHA256:           strings.Repeat("d", 64),
		ProcedureSHA256:            bindingDigest(),
		ResetPolicy:                "reset-before-each-session",
		Pair:                       1,
		Order:                      "baseline-treatment",
		Directory:                  "pair-001-baseline-treatment",
		FirstSession:               "baseline",
		SecondSession:              "treatment",
		FirstSessionBindingSHA256:  strings.Repeat("e", 64),
		SecondSessionBindingSHA256: strings.Repeat("f", 64),
	}
}

func validReplicationBindingForTest() ReplicationBinding {
	return ReplicationBinding{
		SchemaVersion:          BindingSchemaVersion,
		Kind:                   ReplicationBindingKind,
		Source:                 "android",
		Adapter:                "android-experiment-001",
		AdapterVersion:         1,
		Scope:                  "all",
		ManifestName:           "experiment-001-email",
		DeclaredVariable:       "email",
		ManifestContractSHA256: bindingDigest(),
		ProvenanceSHA256:       strings.Repeat("d", 64),
		PairsPerOrder:          1,
		ResetPolicy:            "reset-before-each-session",
		Pairs: []PairReference{
			{Pair: 1, Order: "baseline-treatment", BindingSHA256: strings.Repeat("e", 64)},
			{Pair: 1, Order: "treatment-baseline", BindingSHA256: strings.Repeat("f", 64)},
		},
	}
}

func validEvidenceBindingForTest() EvidenceBinding {
	return EvidenceBinding{
		SchemaVersion:          BindingSchemaVersion,
		Kind:                   EvidenceBindingKind,
		Source:                 "android",
		Adapter:                "android-experiment-001",
		AdapterVersion:         1,
		Scope:                  "all",
		ManifestName:           "experiment-001-email",
		DeclaredVariable:       "email",
		ManifestContractSHA256: bindingDigest(),
		ProvenanceSHA256:       strings.Repeat("d", 64),
		ResetPolicy:            "reset-before-each-session",
		RootBindingSHA256:      strings.Repeat("e", 64),
		ReceiptSHA256:          strings.Repeat("f", 64),
		Pairs: []EvidencePairReference{
			{
				Pair:              1,
				Order:             "baseline-treatment",
				PairBindingSHA256: strings.Repeat("1", 64),
				EvidenceSHA256:    strings.Repeat("2", 64),
			},
			{
				Pair:              1,
				Order:             "treatment-baseline",
				PairBindingSHA256: strings.Repeat("3", 64),
				EvidenceSHA256:    strings.Repeat("4", 64),
			},
		},
	}
}

func TestBindingDigestsAreCanonicalAndMutationSensitive(t *testing.T) {
	session := validSessionBindingForTest()
	first, err := session.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second || len(first) != 64 {
		t.Fatalf("session digest = %q, %q", first, second)
	}
	session.Status = "incomplete"
	session.FailureStage = "capture_storage"
	if changed, err := session.SHA256(); err != nil || changed == first {
		t.Fatalf("mutated session digest = %q, error = %v", changed, err)
	}

	pair := validPairBindingForTest()
	pairDigest, err := pair.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	replication := validReplicationBindingForTest()
	replicationDigest, err := replication.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	evidence := validEvidenceBindingForTest()
	evidenceDigest, err := evidence.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	for name, digest := range map[string]string{
		"pair":        pairDigest,
		"replication": replicationDigest,
		"evidence":    evidenceDigest,
	} {
		if len(digest) != 64 {
			t.Fatalf("%s digest length = %d", name, len(digest))
		}
	}
	if digest := SHA256String("private-value"); len(digest) != 64 || digest == "private-value" {
		t.Fatal("SHA256String() returned an unexpected digest")
	}
}

func TestBindingDigestsRejectInvalidValues(t *testing.T) {
	session := validSessionBindingForTest()
	session.Status = "invalid"
	if _, err := session.SHA256(); err == nil {
		t.Fatal("SessionBinding.SHA256() accepted an invalid binding")
	}

	pair := validPairBindingForTest()
	pair.Order = "invalid"
	if _, err := pair.SHA256(); err == nil {
		t.Fatal("PairBinding.SHA256() accepted an invalid binding")
	}

	replication := validReplicationBindingForTest()
	replication.Pairs = replication.Pairs[:1]
	if _, err := replication.SHA256(); err == nil {
		t.Fatal("ReplicationBinding.SHA256() accepted an incomplete binding")
	}

	evidence := validEvidenceBindingForTest()
	evidence.Pairs = evidence.Pairs[:1]
	if _, err := evidence.SHA256(); err == nil {
		t.Fatal("EvidenceBinding.SHA256() accepted a one-order binding")
	}
}
func TestSessionBindingValidationRejectsUnsafeStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SessionBinding)
	}{
		{"schema", func(binding *SessionBinding) { binding.SchemaVersion = 2 }},
		{"kind", func(binding *SessionBinding) { binding.Kind = "other" }},
		{"role", func(binding *SessionBinding) { binding.Role = "other" }},
		{"order", func(binding *SessionBinding) { binding.Order = "other" }},
		{"status", func(binding *SessionBinding) { binding.Status = "other" }},
		{"failure", func(binding *SessionBinding) {
			binding.Status = "complete"
			binding.FailureStage = "capture_network"
		}},
		{"timestamps", func(binding *SessionBinding) { binding.FinishedAt = binding.StartedAt.Add(-time.Second) }},
		{"volatile order", func(binding *SessionBinding) { binding.VolatileFields = []string{"z", "a"} }},
		{"tap resource", func(binding *SessionBinding) { binding.TapResourceID = "bad value" }},
		{"step status", func(binding *SessionBinding) { binding.Steps[0].Status = "other" }},
		{"step digest", func(binding *SessionBinding) { binding.Steps[0].UIHierarchySHA256 = "bad" }},
		{"artifact source", func(binding *SessionBinding) { binding.Artifacts[0].Source = "bad\\nsource" }},
		{"artifact path", func(binding *SessionBinding) { binding.Artifacts[0].Path = "../secret" }},
		{"artifact size", func(binding *SessionBinding) { binding.Artifacts[0].SizeBytes = -1 }},
		{"artifact digest", func(binding *SessionBinding) { binding.Artifacts[0].SHA256 = "bad" }},
		{"target", func(binding *SessionBinding) { binding.Target.DeviceSHA256 = "bad" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := validSessionBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe session binding")
			}
		})
	}
}

func TestPairReplicationAndEvidenceValidationRejectsUnsafeStates(t *testing.T) {
	pairTests := []struct {
		name   string
		mutate func(*PairBinding)
	}{
		{"order", func(binding *PairBinding) { binding.Order = "treatment-baseline" }},
		{"session", func(binding *PairBinding) { binding.FirstSession = "other" }},
		{"directory", func(binding *PairBinding) { binding.Directory = "../pair" }},
		{"digest", func(binding *PairBinding) { binding.FirstSessionBindingSHA256 = "bad" }},
	}
	for _, test := range pairTests {
		t.Run("pair-"+test.name, func(t *testing.T) {
			binding := validPairBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("PairBinding.Validate() accepted unsafe state")
			}
		})
	}

	replicationTests := []struct {
		name   string
		mutate func(*ReplicationBinding)
	}{
		{"count", func(binding *ReplicationBinding) { binding.Pairs = binding.Pairs[:1] }},
		{"order", func(binding *ReplicationBinding) { binding.Pairs[0].Order = "treatment-baseline" }},
		{"digest", func(binding *ReplicationBinding) { binding.Pairs[0].BindingSHA256 = "bad" }},
	}
	for _, test := range replicationTests {
		t.Run("replication-"+test.name, func(t *testing.T) {
			binding := validReplicationBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("ReplicationBinding.Validate() accepted unsafe state")
			}
		})
	}

	evidenceTests := []struct {
		name   string
		mutate func(*EvidenceBinding)
	}{
		{"receipt", func(binding *EvidenceBinding) { binding.ReceiptSHA256 = "bad" }},
		{"order", func(binding *EvidenceBinding) { binding.Pairs[0].Order = "treatment-baseline" }},
		{"pair digest", func(binding *EvidenceBinding) { binding.Pairs[0].PairBindingSHA256 = "bad" }},
		{"evidence digest", func(binding *EvidenceBinding) { binding.Pairs[0].EvidenceSHA256 = "bad" }},
	}
	for _, test := range evidenceTests {
		t.Run("evidence-"+test.name, func(t *testing.T) {
			binding := validEvidenceBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("EvidenceBinding.Validate() accepted unsafe state")
			}
		})
	}
}

func TestBindingValidationRejectsMissingIdentityAndBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SessionBinding)
	}{
		{"manifest", func(binding *SessionBinding) { binding.ManifestName = "" }},
		{"variable", func(binding *SessionBinding) { binding.DeclaredVariable = "" }},
		{"persona fields low", func(binding *SessionBinding) { binding.PersonaFields = 0 }},
		{"persona fields high", func(binding *SessionBinding) { binding.PersonaFields = 65 }},
		{"volatile count", func(binding *SessionBinding) { binding.VolatileFields = make([]string, 65) }},
		{"volatile label", func(binding *SessionBinding) { binding.VolatileFields = []string{"bad value"} }},
		{"failure stage", func(binding *SessionBinding) {
			binding.Status = "incomplete"
			binding.FailureStage = ""
		}},
		{"step count", func(binding *SessionBinding) { binding.Steps = make([]StepBinding, 17) }},
		{"step name", func(binding *SessionBinding) { binding.Steps[0].Name = "bad value" }},
		{"step timestamps", func(binding *SessionBinding) { binding.Steps[0].FinishedAt = time.Time{} }},
		{"step exit code", func(binding *SessionBinding) { binding.Steps[0].ExitCode = 256 }},
		{"artifact count", func(binding *SessionBinding) { binding.Artifacts = make([]ArtifactBinding, 9) }},
		{"artifact kind", func(binding *SessionBinding) { binding.Artifacts[0].Kind = "bad value" }},
		{"source", func(binding *SessionBinding) { binding.Source = "bad value" }},
		{"adapter", func(binding *SessionBinding) { binding.Adapter = "" }},
		{"adapter version", func(binding *SessionBinding) { binding.AdapterVersion = 33 }},
		{"scope", func(binding *SessionBinding) { binding.Scope = "" }},
		{"manifest digest", func(binding *SessionBinding) { binding.ManifestContractSHA256 = "bad" }},
		{"provenance digest", func(binding *SessionBinding) { binding.ProvenanceSHA256 = "bad" }},
		{"challenge digest", func(binding *SessionBinding) { binding.ChallengeCommitment = "bad" }},
		{"procedure digest", func(binding *SessionBinding) { binding.ProcedureSHA256 = "bad" }},
		{"reset policy", func(binding *SessionBinding) { binding.ResetPolicy = "" }},
		{"target adb version", func(binding *SessionBinding) { binding.Target.ADBVersion = "" }},
		{"target package", func(binding *SessionBinding) { binding.Target.Package = "bad value" }},
		{"target api", func(binding *SessionBinding) { binding.Target.AndroidAPI = 0 }},
		{"target architecture", func(binding *SessionBinding) { binding.Target.Architecture = "" }},
		{"target version code", func(binding *SessionBinding) { binding.Target.PackageVersionCode = 0 }},
		{"target package digest", func(binding *SessionBinding) { binding.Target.PackageSHA256 = "bad" }},
		{"target revision", func(binding *SessionBinding) { binding.Target.AriadneRevision = "bad" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding := validSessionBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("Validate() accepted missing or unsafe session identity")
			}
		})
	}
}

func TestPairReplicationAndEvidenceValidationRejectsMissingIdentity(t *testing.T) {
	pairTests := []struct {
		name   string
		mutate func(*PairBinding)
	}{
		{"manifest", func(binding *PairBinding) { binding.ManifestName = "" }},
		{"variable", func(binding *PairBinding) { binding.DeclaredVariable = "" }},
		{"source", func(binding *PairBinding) { binding.Source = "" }},
		{"adapter version", func(binding *PairBinding) { binding.AdapterVersion = 0 }},
		{"manifest digest", func(binding *PairBinding) { binding.ManifestContractSHA256 = "bad" }},
		{"provenance digest", func(binding *PairBinding) { binding.ProvenanceSHA256 = "bad" }},
		{"procedure digest", func(binding *PairBinding) { binding.ProcedureSHA256 = "bad" }},
		{"reset policy", func(binding *PairBinding) { binding.ResetPolicy = "" }},
		{"pair number", func(binding *PairBinding) { binding.Pair = 0 }},
		{"order label", func(binding *PairBinding) { binding.Order = "bad value" }},
		{"second session", func(binding *PairBinding) { binding.SecondSession = "other" }},
		{"second digest", func(binding *PairBinding) { binding.SecondSessionBindingSHA256 = "bad" }},
	}
	for _, test := range pairTests {
		t.Run("pair-"+test.name, func(t *testing.T) {
			binding := validPairBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("PairBinding.Validate() accepted missing or unsafe identity")
			}
		})
	}

	replicationTests := []struct {
		name   string
		mutate func(*ReplicationBinding)
	}{
		{"manifest", func(binding *ReplicationBinding) { binding.ManifestName = "" }},
		{"variable", func(binding *ReplicationBinding) { binding.DeclaredVariable = "" }},
		{"source", func(binding *ReplicationBinding) { binding.Source = "" }},
		{"adapter version", func(binding *ReplicationBinding) { binding.AdapterVersion = 0 }},
		{"manifest digest", func(binding *ReplicationBinding) { binding.ManifestContractSHA256 = "bad" }},
		{"provenance digest", func(binding *ReplicationBinding) { binding.ProvenanceSHA256 = "bad" }},
		{"pairs per order", func(binding *ReplicationBinding) { binding.PairsPerOrder = 0 }},
		{"reset policy", func(binding *ReplicationBinding) { binding.ResetPolicy = "" }},
		{"pair number", func(binding *ReplicationBinding) { binding.Pairs[0].Pair = 0 }},
	}
	for _, test := range replicationTests {
		t.Run("replication-"+test.name, func(t *testing.T) {
			binding := validReplicationBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("ReplicationBinding.Validate() accepted missing or unsafe identity")
			}
		})
	}

	evidenceTests := []struct {
		name   string
		mutate func(*EvidenceBinding)
	}{
		{"manifest", func(binding *EvidenceBinding) { binding.ManifestName = "" }},
		{"variable", func(binding *EvidenceBinding) { binding.DeclaredVariable = "" }},
		{"source", func(binding *EvidenceBinding) { binding.Source = "" }},
		{"adapter version", func(binding *EvidenceBinding) { binding.AdapterVersion = 0 }},
		{"manifest digest", func(binding *EvidenceBinding) { binding.ManifestContractSHA256 = "bad" }},
		{"provenance digest", func(binding *EvidenceBinding) { binding.ProvenanceSHA256 = "bad" }},
		{"reset policy", func(binding *EvidenceBinding) { binding.ResetPolicy = "" }},
		{"root digest", func(binding *EvidenceBinding) { binding.RootBindingSHA256 = "bad" }},
		{"empty pairs", func(binding *EvidenceBinding) { binding.Pairs = nil }},
		{"odd pair count", func(binding *EvidenceBinding) { binding.Pairs = binding.Pairs[:1] }},
		{"pair number", func(binding *EvidenceBinding) { binding.Pairs[0].Pair = 0 }},
	}
	for _, test := range evidenceTests {
		t.Run("evidence-"+test.name, func(t *testing.T) {
			binding := validEvidenceBindingForTest()
			test.mutate(&binding)
			if err := binding.Validate(); err == nil {
				t.Fatal("EvidenceBinding.Validate() accepted missing or unsafe identity")
			}
		})
	}
}

func TestBindingPrimitiveValidation(t *testing.T) {
	if !validText("text", 4) || validText(" text", 5) || validText("text\n", 5) || validText("", 4) {
		t.Fatal("validText() accepted an invalid value")
	}
	if !validBindingLabel("label:/._-", 32) || validBindingLabel("bad value", 32) ||
		validBindingLabel("", 32) || validBindingLabel(strings.Repeat("x", 33), 32) {
		t.Fatal("validBindingLabel() accepted an invalid value")
	}
	if !validSource("POST /observe") || validSource("bad\\source") ||
		validSource(" bad") || validSource(strings.Repeat("x", 257)) {
		t.Fatal("validSource() accepted an invalid value")
	}
	if !validDigest(bindingDigest()) || validDigest("bad") || validDigest(strings.Repeat("A", 64)) {
		t.Fatal("validDigest() accepted an invalid value")
	}
	if !validRevision(strings.Repeat("a", 40)) || !validRevision(strings.Repeat("a", 64)) ||
		validRevision("bad") || validRevision(strings.Repeat("A", 64)) {
		t.Fatal("validRevision() accepted an invalid value")
	}
}
