package adb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunReplicatedRecordsBothOrdersAndResets(t *testing.T) {
	manifest := sessionManifest()
	target := sessionTarget()
	captures := [][]byte{
		[]byte(`{"schema_version":1,"region":"us-east","variant":"standard"}`),
		[]byte(`{"schema_version":1,"region":"us-east","variant":"personalized"}`),
		[]byte(`{"schema_version":1,"region":"us-east","variant":"personalized"}`),
		[]byte(`{"schema_version":1,"region":"us-east","variant":"standard"}`),
	}
	var starts []string
	resets := 0
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[3] == "pm" {
			resets++
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			return nil, nil
		}
		if args[3] == "am" {
			for index := range args {
				if args[index] == "--es" && index+2 < len(args) && args[index+1] == "email" {
					starts = append(starts, args[index+2])
				}
			}
			if err := postFixtureObservation(args, captures[0]); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if args[2] == "exec-out" {
			output := captures[0]
			captures = captures[1:]
			return output, nil
		}
		return []byte("Status: ok\n"), nil
	}

	outputDir := filepath.Join(t.TempDir(), "replicated")
	if err := runReplicatedWith(
		context.Background(),
		"adb",
		target,
		manifest,
		outputDir,
		1,
		run,
		sequenceClock(),
	); err != nil {
		t.Fatalf("runReplicatedWith() error = %v", err)
	}
	if resets != 4 {
		t.Fatalf("reset calls = %d, want 4", resets)
	}
	wantStarts := []string{
		"baseline@example.invalid",
		"treatment@example.invalid",
		"treatment@example.invalid",
		"baseline@example.invalid",
	}
	if strings.Join(starts, ",") != strings.Join(wantStarts, ",") {
		t.Fatalf("start order = %v, want %v", starts, wantStarts)
	}

	data, err := os.ReadFile(filepath.Join(outputDir, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record ReplicatedRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Status != ReplicationStatusComplete ||
		record.CompletedPairs != 2 || len(record.Pairs) != 2 ||
		record.Pairs[0].Order != ReplicationOrderBaselineTreatment ||
		record.Pairs[1].Order != ReplicationOrderTreatmentBaseline {
		t.Fatalf("replication record = %#v", record)
	}
	if record.ResetPolicy != ReplicationResetPolicy {
		t.Fatalf("reset policy = %q", record.ResetPolicy)
	}
	for _, pair := range record.Pairs {
		if _, err := os.Stat(filepath.Join(outputDir, pair.Directory, "replication.json")); !os.IsNotExist(err) {
			t.Fatalf("nested replication metadata stat error = %v", err)
		}
	}
}

func TestRunReplicatedRecordsFailureWithoutRawError(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "replicated")
	resets := 0
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[3] == "pm" {
			resets++
			if resets == 3 {
				return []byte("private-captured-value"), errors.New("exit status 1")
			}
			return []byte("Success\n"), nil
		}
		if args[2] == "reverse" {
			return nil, nil
		}
		if args[3] == "am" {
			if err := postFixtureObservation(args, []byte(`{"schema_version":1,"region":"us-east"}`)); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if args[2] == "exec-out" {
			return []byte(`{"schema_version":1,"region":"us-east"}`), nil
		}
		return []byte("Status: ok\n"), nil
	}

	err := runReplicatedWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		1,
		run,
		sequenceClock(),
	)
	if err == nil || strings.Contains(err.Error(), "private-captured-value") {
		t.Fatalf("runReplicatedWith() error = %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(outputDir, "replication.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var record ReplicatedRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Status != ReplicationStatusIncomplete ||
		record.FailurePair != 1 ||
		record.FailureOrder != ReplicationOrderTreatmentBaseline ||
		record.CompletedPairs != 1 || len(record.Pairs) != 2 {
		t.Fatalf("failure record = %#v", record)
	}
	if strings.Contains(string(data), "private-captured-value") {
		t.Fatalf("replication metadata exposed raw error: %s", data)
	}
}

func TestRunReplicatedRejectsExistingOutputAndInvalidPairCount(t *testing.T) {
	for _, pairs := range []int{0, -1, maxReplicatedPairs + 1} {
		t.Run("count-"+strconv.Itoa(pairs), func(t *testing.T) {
			outputDir := filepath.Join(t.TempDir(), "replicated")
			err := runReplicatedWith(
				context.Background(),
				"adb",
				sessionTarget(),
				sessionManifest(),
				outputDir,
				pairs,
				func(context.Context, string, ...string) ([]byte, error) {
					return nil, nil
				},
				sequenceClock(),
			)
			if err == nil {
				t.Fatal("runReplicatedWith() error = nil")
			}
		})
	}

	outputDir := filepath.Join(t.TempDir(), "replicated")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	err := runReplicatedWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		1,
		func(context.Context, string, ...string) ([]byte, error) {
			return nil, nil
		},
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("existing output error = %v", err)
	}
}

func TestRunReplicatedRejectsInvalidOutputBeforeCreatingIt(t *testing.T) {
	if err := runReplicatedWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		"",
		1,
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		sequenceClock(),
	); err == nil || !strings.Contains(err.Error(), "output directory is required") {
		t.Fatalf("empty output error = %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "replicated")
	if err := runReplicatedWith(
		context.Background(),
		"",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		1,
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		sequenceClock(),
	); err == nil || !strings.Contains(err.Error(), "adb binary is invalid") {
		t.Fatalf("invalid config error = %v", err)
	}
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("invalid config created output: %v", err)
	}
}

func TestRunReplicatedReportsOutputParentFailure(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runReplicatedWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		filepath.Join(parent, "replicated"),
		1,
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "create output parent") {
		t.Fatalf("parent failure = %v", err)
	}
}

func TestWriteReplicatedRecordRejectsInvalidMetadata(t *testing.T) {
	outputDir := t.TempDir()
	for _, record := range []ReplicatedRunRecord{
		{},
		{ManifestName: "manifest", DeclaredVariable: "variable", ProvenanceSHA256: "bad"},
	} {
		if err := writeReplicatedRecord(outputDir, record); err == nil {
			t.Fatalf("writeReplicatedRecord(%+v) error = nil", record)
		}
	}
	if _, err := ReplicationProvenance(""); err == nil {
		t.Fatal("ReplicationProvenance() error = nil")
	}
}

func TestRunReplicatedPreservesExclusiveRoot(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "replicated")
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	err := runReplicatedWith(
		context.Background(),
		"adb",
		sessionTarget(),
		sessionManifest(),
		outputDir,
		1,
		func(context.Context, string, ...string) ([]byte, error) { return nil, nil },
		sequenceClock(),
	)
	if err == nil || !strings.Contains(err.Error(), "create output directory") {
		t.Fatalf("existing root error = %v", err)
	}
}

func TestAuthenticatedReplicationRecordsCanonicalProvenance(t *testing.T) {
	manifest := sessionManifest()
	manifest.SchemaVersion = 3
	manifest.TapResourceID = "dev.ariadne.fixture:id/observe_button"
	target := sessionTarget()
	challenges := []string{
		strings.Repeat("0123456789abcdef", 4),
		strings.Repeat("fedcba9876543210", 4),
		strings.Repeat("0011223344556677", 4),
		strings.Repeat("8899aabbccddeeff", 4),
	}
	challengeIndex := 0
	currentInput := fixtureInput{}
	ui := []byte("<hierarchy><node resource-id=\"dev.ariadne.fixture:id/observe_button\" bounds=\"[100,200][300,400]\" /> </hierarchy>")
	writeInput := func(_ context.Context, _ string, data []byte, _ ...string) ([]byte, error) {
		if err := json.Unmarshal(data, &currentInput); err != nil {
			return nil, err
		}
		if err := validateFixtureInput(currentInput); err != nil {
			return nil, err
		}
		return nil, nil
	}
	postObservation := func(body []byte) error {
		response, err := http.Post(
			"http://127.0.0.1:"+strconv.Itoa(currentInput.CollectorPort)+"/observe",
			"application/json",
			strings.NewReader(string(body)),
		)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return errors.New("collector rejected fixture observation")
		}
		return nil
	}
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 3 && args[3] == "pm" {
			return []byte("Success\n"), nil
		}
		if len(args) > 2 && args[2] == "reverse" {
			return nil, nil
		}
		if len(args) > 3 && args[3] == "am" {
			body := []byte("{\"schema_version\":1,\"challenge\":\"" + currentInput.Challenge + "\",\"region\":\"us-east\",\"variant\":\"standard\"}")
			if err := postObservation(body); err != nil {
				return nil, err
			}
			return []byte("Status: ok\n"), nil
		}
		if len(args) > 3 && (args[3] == "uiautomator" || args[3] == "cat") {
			return ui, nil
		}
		if len(args) > 2 && args[2] == "exec-out" {
			return []byte("{\"schema_version\":1,\"challenge\":\"" + currentInput.Challenge + "\",\"region\":\"us-east\",\"variant\":\"standard\"}"), nil
		}
		return []byte("Status: ok\n"), nil
	}
	challenge := func() (string, error) {
		if challengeIndex >= len(challenges) {
			return "", errors.New("test challenge sequence exhausted")
		}
		value := challenges[challengeIndex]
		challengeIndex++
		return value, nil
	}
	outputDir := filepath.Join(t.TempDir(), "replicated")
	if err := runReplicatedWithAuthenticated(
		context.Background(),
		"adb",
		target,
		manifest,
		outputDir,
		1,
		run,
		writeInput,
		challenge,
		sequenceClock(),
	); err != nil {
		t.Fatalf("runReplicatedWithAuthenticated() error = %v", err)
	}
	expected, err := ReplicationProvenanceSHA256(manifest.ContractDigest())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "replication.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record ReplicatedRunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != AuthenticatedReplicatedRunSchemaVersion ||
		record.ManifestContractSHA256 != manifest.ContractDigest() ||
		record.BindingSHA256 == "" {
		t.Fatalf("authenticated replication envelope = %#v", record)
	}
	expectedBinding, err := ReplicatedBindingSHA256(record)
	if err != nil || record.BindingSHA256 != expectedBinding {
		t.Fatalf("root binding = %q, expected %q, error = %v", record.BindingSHA256, expectedBinding, err)
	}
	for _, pair := range record.Pairs {
		expectedPairBinding, err := ReplicatedPairBindingSHA256(record, pair)
		if err != nil ||
			pair.FirstSessionBindingSHA256 == "" ||
			pair.SecondSessionBindingSHA256 == "" ||
			pair.BindingSHA256 != expectedPairBinding {
			t.Fatalf("pair binding = %#v, expected %q, error = %v", pair, expectedPairBinding, err)
		}
	}
	if record.ProvenanceSHA256 != expected {
		t.Fatalf("provenance_sha256 = %q, want %q", record.ProvenanceSHA256, expected)
	}
	for _, secret := range append(challenges, manifest.Baseline["email"], manifest.Treatment["email"]) {
		if strings.Contains(string(data), secret) {
			t.Fatalf("replication metadata exposed %q: %s", secret, data)
		}
	}
}

func TestWriteAuthenticatedReplicatedRecordRejectsBindingGaps(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := func() ReplicatedRunRecord {
		return ReplicatedRunRecord{
			SchemaVersion:          AuthenticatedReplicatedRunSchemaVersion,
			ManifestName:           "manifest",
			DeclaredVariable:       "variable",
			ManifestContractSHA256: digest,
			ResetPolicy:            ReplicationResetPolicy,
			ProvenanceSHA256:       strings.Repeat("b", 64),
			PairsPerOrder:          1,
			Status:                 ReplicationStatusIncomplete,
			FailurePair:            1,
			FailureOrder:           ReplicationOrderBaselineTreatment,
			Pairs: []ReplicatedPairRecord{{
				Pair:          1,
				Order:         ReplicationOrderBaselineTreatment,
				Directory:     "pair-001-baseline-treatment",
				FirstSession:  "baseline",
				SecondSession: "treatment",
				Status:        ReplicationStatusIncomplete,
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*ReplicatedRunRecord)
	}{
		{"manifest contract", func(record *ReplicatedRunRecord) { record.ManifestContractSHA256 = "bad" }},
		{"provenance", func(record *ReplicatedRunRecord) { record.ProvenanceSHA256 = "bad" }},
		{"complete root binding", func(record *ReplicatedRunRecord) { record.Status = ReplicationStatusComplete }},
		{"incomplete root binding", func(record *ReplicatedRunRecord) { record.BindingSHA256 = digest }},
		{"complete pair binding", func(record *ReplicatedRunRecord) { record.Pairs[0].Status = ReplicationStatusComplete }},
		{"incomplete pair binding", func(record *ReplicatedRunRecord) {
			record.Pairs[0].FirstSessionBindingSHA256 = digest
		}},
		{"pair directory", func(record *ReplicatedRunRecord) { record.Pairs[0].Directory = "pair-001-other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := base()
			test.mutate(&record)
			if err := writeReplicatedRecord(t.TempDir(), record); err == nil {
				t.Fatal("writeReplicatedRecord() accepted an invalid authenticated envelope")
			}
		})
	}
}
