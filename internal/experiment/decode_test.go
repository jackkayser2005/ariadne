package experiment

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

const validJSON = `{
	"schema_version": 1,
	"name": "experiment-001-email",
	"variable": "email",
	"baseline": {
		"email": "baseline@example.invalid",
		"region": "us-east"
	},
	"treatment": {
		"email": "treatment@example.invalid",
		"region": "us-east"
	}
}`

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "valid", input: validJSON},
		{name: "valid trailing whitespace", input: validJSON + "\n\t "},
		{name: "empty", input: " \n", wantErr: "empty input"},
		{name: "malformed", input: `{"schema_version":`, wantErr: "unexpected end of input"},
		{name: "duplicate top-level key", input: `{
			"schema_version": 1,
			"schema_version": 1
		}`, wantErr: `duplicate key "schema_version"`},
		{name: "duplicate persona key", input: `{
			"schema_version": 1,
			"name": "duplicate",
			"variable": "email",
			"baseline": {"email": "first", "email": "second"},
			"treatment": {"email": "third"}
		}`, wantErr: `duplicate key "email"`},
		{name: "unknown field", input: `{
			"schema_version": 1,
			"name": "unknown",
			"variable": "email",
			"baseline": {"email": "first"},
			"treatment": {"email": "second"},
			"extra": [{"nested": true}]
		}`, wantErr: `unknown field "extra"`},
		{name: "trailing data", input: validJSON + `{}`, wantErr: "trailing data"},
		{name: "non-string persona value", input: `{
			"schema_version": 1,
			"name": "typed",
			"variable": "consent",
			"baseline": {"consent": false},
			"treatment": {"consent": true}
		}`, wantErr: "cannot unmarshal bool"},
		{name: "semantic failure", input: strings.Replace(
			validJSON,
			"treatment@example.invalid",
			"baseline@example.invalid",
			1,
		), wantErr: "found 0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := Decode(strings.NewReader(test.input))
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Decode() error = %v", err)
				}
				if manifest.Name != "experiment-001-email" {
					t.Fatalf("Decode() name = %q", manifest.Name)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Decode() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeRejectsOversizedInput(t *testing.T) {
	input := strings.Repeat(" ", MaxManifestBytes+1)

	_, err := Decode(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Decode() error = %v, want size limit error", err)
	}
}

func TestDecodeReportsReadFailure(t *testing.T) {
	_, err := Decode(iotest.ErrReader(errors.New("read failed")))
	if err == nil || !strings.Contains(err.Error(), "read input") {
		t.Fatalf("Decode() error = %v, want read error", err)
	}
}

func TestDecodeDoesNotExposePersonaValues(t *testing.T) {
	const secret = "do-not-log-this-value"
	input := strings.Replace(validJSON, "treatment@example.invalid", secret, 1)
	input = strings.Replace(input, "baseline@example.invalid", secret, 1)

	_, err := Decode(strings.NewReader(input))
	if err == nil {
		t.Fatal("Decode() error = nil, want semantic error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Decode() error exposed persona value: %v", err)
	}
}
