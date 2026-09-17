package adb

import (
	"bytes"
	"strings"
	"testing"
)

func TestAndroidProcedureCanonicalIdentityIsIndependent(t *testing.T) {
	procedure := CurrentAndroidProcedure()
	data, err := procedure.CanonicalBytes()
	if err != nil {
		t.Fatalf("CanonicalBytes() error = %v", err)
	}
	want := `{"schema_version":1,"id":"android-experiment-001-authenticated","version":1,"input_boundary":"app-private-run-as-file-v1","capture_scope":"all","reset_policy":"reset-before-each-session"}`
	if string(data) != want {
		t.Fatalf("CanonicalBytes() = %s, want %s", data, want)
	}
	digest, err := procedure.SHA256()
	if err != nil || len(digest) != 64 {
		t.Fatalf("SHA256() = %q, %v", digest, err)
	}
	manifestDigest := strings.Repeat("a", 64)
	if digest == manifestDigest {
		t.Fatal("procedure digest unexpectedly equals a manifest-shaped digest")
	}
	changed := procedure
	changed.Version++
	changedDigest, err := changed.SHA256()
	if err != nil || changedDigest == digest {
		t.Fatalf("changed procedure SHA256() = %q, %v; original = %q", changedDigest, err, digest)
	}
}

func TestProcedureContractRejectsInvalidIdentity(t *testing.T) {
	base := CurrentAndroidProcedure()
	tests := []struct {
		name string
		edit func(*ProcedureContract)
	}{
		{"schema", func(value *ProcedureContract) { value.SchemaVersion = 2 }},
		{"id", func(value *ProcedureContract) { value.ID = "bad value" }},
		{"version", func(value *ProcedureContract) { value.Version = 0 }},
		{"input boundary", func(value *ProcedureContract) { value.InputBoundary = "" }},
		{"capture scope", func(value *ProcedureContract) { value.CaptureScope = "all\n" }},
		{"reset policy", func(value *ProcedureContract) { value.ResetPolicy = strings.Repeat("x", 129) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.edit(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid procedure")
			}
			if _, err := value.SHA256(); err == nil {
				t.Fatal("invalid procedure hashed")
			}
			if _, err := value.CanonicalBytes(); err == nil {
				t.Fatal("CanonicalBytes() accepted invalid procedure")
			}
		})
	}
}

func TestReplicationProvenanceKeepsManifestAndProcedureIndependent(t *testing.T) {
	manifestDigest := strings.Repeat("a", 64)
	procedureDigest, err := AndroidProcedureSHA256()
	if err != nil {
		t.Fatal(err)
	}
	current, err := ReplicationProvenanceWithProcedure(manifestDigest, procedureDigest)
	if err != nil {
		t.Fatal(err)
	}
	if current.ManifestContractSHA256 != manifestDigest || current.ProcedureSHA256 != procedureDigest {
		t.Fatalf("current provenance = %#v", current)
	}
	data, err := current.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{manifestDigest, procedureDigest} {
		if !strings.Contains(string(data), digest) {
			t.Fatalf("canonical provenance omitted independent digest %q: %s", digest, data)
		}
	}
	legacy, err := ReplicationProvenance(manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	legacyDigest, err := legacy.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	currentDigest, err := current.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	if legacy.ManifestContractSHA256 != "" || legacy.ProcedureSHA256 != manifestDigest ||
		legacyDigest == currentDigest {
		t.Fatalf("legacy provenance contract = %#v, digest = %s", legacy, legacyDigest)
	}
}

func TestAndroidProcedureSHA256Stable(t *testing.T) {
	first, err := AndroidProcedureSHA256()
	if err != nil {
		t.Fatal(err)
	}
	second, err := AndroidProcedureSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(first), []byte(second)) {
		t.Fatalf("AndroidProcedureSHA256() changed from %q to %q", first, second)
	}
}
