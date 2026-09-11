package adb

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadSessionBindingAcceptsCanonicalMetadata(t *testing.T) {
	record := sessionBindingRecordForTest("baseline")
	root := t.TempDir()
	writeSessionBindingRecord(t, root, "baseline", record)
	got, err := readSessionBinding(root, "baseline")
	if err != nil {
		t.Fatalf("readSessionBinding() error = %v", err)
	}
	if got != record.BindingSHA256 {
		t.Fatalf("readSessionBinding() = %q, want %q", got, record.BindingSHA256)
	}
}

func TestReadSessionBindingRejectsMalformedOrTamperedMetadata(t *testing.T) {
	tests := []struct {
		name string
		data func(t *testing.T, record SessionRecord) []byte
	}{
		{
			name: "missing",
			data: func(_ *testing.T, record SessionRecord) []byte {
				record.BindingSHA256 = ""
				data, _ := json.Marshal(record)
				return data
			},
		},
		{
			name: "legacy schema",
			data: func(_ *testing.T, record SessionRecord) []byte {
				record.SchemaVersion = sessionSchemaVersion
				data, _ := json.Marshal(record)
				return data
			},
		},
		{
			name: "wrong kind",
			data: func(_ *testing.T, record SessionRecord) []byte {
				record.Kind = "treatment"
				data, _ := json.Marshal(record)
				return data
			},
		},
		{
			name: "tampered metadata",
			data: func(_ *testing.T, record SessionRecord) []byte {
				record.ManifestName = "tampered"
				data, _ := json.Marshal(record)
				return data
			},
		},
		{
			name: "duplicate key",
			data: func(_ *testing.T, _ SessionRecord) []byte {
				return []byte("{\"schema_version\":9,\"schema_version\":9}")
			},
		},
		{
			name: "unknown field",
			data: func(_ *testing.T, record SessionRecord) []byte {
				data, _ := json.Marshal(record)
				trimmed := bytes.TrimSpace(data)
				return append(append(trimmed[:len(trimmed)-1], []byte(",\"unexpected\":true}")...), '\n')
			},
		},
		{
			name: "trailing data",
			data: func(_ *testing.T, record SessionRecord) []byte {
				data, _ := json.Marshal(record)
				return append(data, []byte("{}")...)
			},
		},
		{
			name: "oversized",
			data: func(_ *testing.T, _ SessionRecord) []byte {
				return bytes.Repeat([]byte("x"), maxOutputBytes+1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := sessionBindingRecordForTest("baseline")
			root := t.TempDir()
			writeSessionBindingBytes(t, root, "baseline", test.data(t, record))
			if _, err := readSessionBinding(root, "baseline"); err == nil {
				t.Fatal("readSessionBinding() accepted malformed or tampered metadata")
			}
		})
	}
	if _, err := readSessionBinding(t.TempDir(), "baseline"); err == nil {
		t.Fatal("readSessionBinding() accepted a missing session")
	}
}

func sessionBindingRecordForTest(kind string) SessionRecord {
	started := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	record := SessionRecord{
		SchemaVersion:          AuthenticatedSessionSchemaVersion,
		Kind:                   kind,
		ManifestName:           "experiment-001-email",
		DeclaredVariable:       "email",
		PersonaFields:          2,
		VolatileFields:         []string{"request_id"},
		TapResourceID:          "dev.ariadne.fixture:id/observe_button",
		ManifestContractSHA256: strings.Repeat("a", 64),
		ChallengeCommitment:    strings.Repeat("b", 64),
		Role:                   kind,
		Order:                  ReplicationOrderBaselineTreatment,
		ProcedureSHA256:        strings.Repeat("a", 64),
		ResetPolicy:            ReplicationResetPolicy,
		ADBVersion:             "1.0.41",
		Device:                 "emulator-5554",
		Package:                "dev.ariadne.fixture",
		AndroidAPI:             35,
		Architecture:           "x86_64",
		PackageVersionCode:     1,
		PackageSHA256:          strings.Repeat("c", 64),
		AriadneRevision:        strings.Repeat("d", 40),
		Status:                 sessionStatusComplete,
		StartedAt:              started,
		FinishedAt:             started.Add(5 * time.Second),
		Steps: []StepRecord{{
			Name:       "reset",
			StartedAt:  started.Add(time.Second),
			FinishedAt: started.Add(2 * time.Second),
			Status:     "ok",
			ExitCode:   0,
		}},
	}
	record.BindingSHA256, _ = SessionBindingSHA256(record)
	return record
}

func writeSessionBindingRecord(t *testing.T, root, kind string, record SessionRecord) {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeSessionBindingBytes(t, root, kind, data)
}

func writeSessionBindingBytes(t *testing.T, root, kind string, data []byte) {
	t.Helper()
	dir := filepath.Join(root, kind)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
