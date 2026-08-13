package adb

import (
	"context"
	"encoding/json"
	"errors"
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
	if err := writeReplicatedRecord(outputDir, ReplicatedRunRecord{}); err == nil {
		t.Fatal("writeReplicatedRecord() error = nil")
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
