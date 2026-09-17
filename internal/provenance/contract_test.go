package provenance

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func validContract() Contract {
	return Contract{
		SchemaVersion:   SchemaVersion,
		Source:          "android",
		Adapter:         "android-experiment-001",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("a", 64),
		Scope:           "all",
	}
}

func TestContractCanonicalIdentity(t *testing.T) {
	contract := validContract()
	wantJSON := `{"schema_version":1,"source":"android","adapter":"android-experiment-001","adapter_version":1,"procedure_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","scope":"all"}`
	data, err := contract.CanonicalBytes()
	if err != nil {
		t.Fatalf("CanonicalBytes() error = %v", err)
	}
	if string(data) != wantJSON {
		t.Fatalf("CanonicalBytes() = %s, want %s", data, wantJSON)
	}
	digest, err := contract.SHA256()
	if err != nil || len(digest) != 64 {
		t.Fatalf("SHA256() = %q, %v", digest, err)
	}
	decoded, err := Decode(append([]byte(" \n\t"), append(data, []byte("\n ")...)...))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded != contract {
		t.Fatalf("Decode() = %#v, want %#v", decoded, contract)
	}
	mutated := contract
	mutated.Scope = "outbound"
	mutatedDigest, err := mutated.SHA256()
	if err != nil || mutatedDigest == digest {
		t.Fatalf("mutated SHA256() = %q, %v; original = %q", mutatedDigest, err, digest)
	}
	if !Equivalent(contract, decoded) || Equivalent(contract, mutated) {
		t.Fatal("Equivalent() returned the wrong result")
	}
}

func TestContractValidationRejectsInvalidFields(t *testing.T) {
	base := validContract()
	tests := []struct {
		name string
		edit func(*Contract)
	}{
		{name: "schema", edit: func(value *Contract) { value.SchemaVersion = 2 }},
		{name: "empty source", edit: func(value *Contract) { value.Source = "" }},
		{name: "source whitespace", edit: func(value *Contract) { value.Source = "android fixture" }},
		{name: "source too long", edit: func(value *Contract) { value.Source = strings.Repeat("a", maxSourceBytes+1) }},
		{name: "adapter whitespace", edit: func(value *Contract) { value.Adapter = "android\nfixture" }},
		{name: "adapter too long", edit: func(value *Contract) { value.Adapter = strings.Repeat("a", maxAdapterBytes+1) }},
		{name: "adapter version low", edit: func(value *Contract) { value.AdapterVersion = 0 }},
		{name: "adapter version high", edit: func(value *Contract) { value.AdapterVersion = 33 }},
		{name: "procedure short", edit: func(value *Contract) { value.ProcedureSHA256 = "bad" }},
		{name: "procedure uppercase", edit: func(value *Contract) { value.ProcedureSHA256 = strings.Repeat("A", 64) }},
		{name: "scope empty", edit: func(value *Contract) { value.Scope = "" }},
		{name: "scope too long", edit: func(value *Contract) { value.Scope = strings.Repeat("a", maxScopeBytes+1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.edit(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid contract")
			}
			if _, err := value.CanonicalBytes(); err == nil {
				t.Fatal("CanonicalBytes() accepted invalid contract")
			}
			if _, err := value.SHA256(); err == nil {
				t.Fatal("SHA256() accepted invalid contract")
			}
		})
	}
}

func TestDecodeRejectsHostileInput(t *testing.T) {
	contractJSON, err := json.Marshal(validContract())
	if err != nil {
		t.Fatal(err)
	}
	valid := string(contractJSON)
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "whitespace", data: []byte(" \n\t")},
		{name: "array", data: []byte("[]")},
		{name: "duplicate", data: []byte(strings.TrimSuffix(valid, "}") + `,"scope":"all"}`)},
		{name: "unknown", data: []byte(strings.TrimSuffix(valid, "}") + `,"extra":true}`)},
		{name: "trailing", data: []byte(valid + " {}")},
		{name: "invalid utf8", data: []byte{'{', '"', 0xff, '"', ':', '1', '}'}},
		{name: "oversized", data: bytes.Repeat([]byte("x"), maxContractBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(test.data); err == nil {
				t.Fatal("Decode() accepted hostile input")
			}
		})
	}
}
