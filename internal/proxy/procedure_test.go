package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestDecodeProcedureAcceptsCanonicalAuthority(t *testing.T) {
	data := []byte(`{"schema_version":1,"procedure_id":"proxy-connect-v1","scope":"outbound","duration_ms":500,"max_events":8,"target_authority":"example.com:443"}`)
	procedure, err := DecodeProcedure(data)
	if err != nil || procedure.TargetAuthority != "example.com:443" {
		t.Fatalf("DecodeProcedure() = %#v, err = %v", procedure, err)
	}
	digest, err := ProcedureSHA256(procedure)
	if err != nil || !portabletrace.ValidSHA256(digest) {
		t.Fatalf("ProcedureSHA256() = %q, err = %v", digest, err)
	}
}

func TestDecodeProcedureRejectsUnsafeAuthorities(t *testing.T) {
	valid := `{"schema_version":1,"procedure_id":"proxy-connect-v1","scope":"outbound","duration_ms":500,"max_events":8,"target_authority":"example.com:443"}`
	for _, test := range []struct {
		name string
		data string
	}{
		{name: "scheme", data: strings.Replace(valid, "example.com:443", "https://example.com:443", 1)},
		{name: "uppercase", data: strings.Replace(valid, "example.com:443", "EXAMPLE.com:443", 1)},
		{name: "path", data: strings.Replace(valid, "example.com:443", "example.com/path:443", 1)},
		{name: "ip", data: strings.Replace(valid, "example.com:443", "127.0.0.1:443", 1)},
		{name: "ipv6", data: strings.Replace(valid, "example.com:443", "[::1]:443", 1)},
		{name: "noncanonical port", data: strings.Replace(valid, "example.com:443", "example.com:0443", 1)},
		{name: "missing port", data: strings.Replace(valid, "example.com:443", "example.com", 1)},
		{name: "unknown field", data: strings.Replace(valid, "target_authority", "extra", 1)},
		{name: "duplicate field", data: strings.Replace(valid, "\"target_authority\":\"example.com:443\"", "\"target_authority\":\"example.com:443\",\"target_authority\":\"other.com:443\"", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeProcedure([]byte(test.data)); err == nil {
				t.Fatal("DecodeProcedure() accepted invalid procedure")
			}
		})
	}
}

func TestDecodeProcedureRejectsMalformedInput(t *testing.T) {
	valid := []byte(`{"schema_version":1,"procedure_id":"proxy-connect-v1","scope":"outbound","duration_ms":500,"max_events":8,"target_authority":"example.com:443"}`)
	cases := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "invalid utf8", data: append(append([]byte(nil), valid...), 0xff)},
		{name: "oversized", data: bytes.Repeat([]byte("x"), maxProcedureBytes+1)},
		{name: "trailing value", data: append(append([]byte(nil), valid...), []byte(" {}")...)},
		{name: "empty label", data: bytes.Replace(valid, []byte("example.com:443"), []byte("example..com:443"), 1)},
		{name: "leading hyphen", data: bytes.Replace(valid, []byte("example.com:443"), []byte("-example.com:443"), 1)},
		{name: "trailing hyphen", data: bytes.Replace(valid, []byte("example.com:443"), []byte("example-.com:443"), 1)},
		{name: "invalid character", data: bytes.Replace(valid, []byte("example.com:443"), []byte("example_com:443"), 1)},
		{name: "zero port", data: bytes.Replace(valid, []byte("example.com:443"), []byte("example.com:0"), 1)},
		{name: "large port", data: bytes.Replace(valid, []byte("example.com:443"), []byte("example.com:65536"), 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeProcedure(test.data); err == nil {
				t.Fatal("DecodeProcedure() accepted malformed input")
			}
		})
	}
}

func TestReadProcedureBoundsAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "procedure.json")
	data := []byte(`{"schema_version":1,"procedure_id":"proxy-connect-v1","scope":"outbound","duration_ms":500,"max_events":8,"target_authority":"example.com:443"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	procedure, received, err := ReadProcedure(path)
	if err != nil || string(received) != string(data) || procedure.ProcedureID != ConnectProcedureID {
		t.Fatalf("ReadProcedure() = %#v, %q, err = %v", procedure, received, err)
	}
	first, err := ProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProcedureSHA256(procedure)
	if err != nil || first != second {
		t.Fatalf("procedure identity = %q, %q, err = %v", first, second, err)
	}
	if _, _, err := ReadProcedure(""); err == nil {
		t.Fatal("ReadProcedure() accepted empty path")
	}
	if _, _, err := ReadProcedure(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadProcedure() accepted missing path")
	}
	oversized := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte("x"), maxProcedureBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadProcedure(oversized); err == nil {
		t.Fatal("ReadProcedure() accepted oversized input")
	}
	if _, err := ProcedureSHA256(Procedure{}); err == nil {
		t.Fatal("ProcedureSHA256() accepted invalid procedure")
	}
}
