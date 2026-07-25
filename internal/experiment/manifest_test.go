package experiment

import (
	"strings"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*Manifest)
		wantErr string
	}{
		{name: "valid"},
		{name: "unsupported version", change: func(m *Manifest) {
			m.SchemaVersion = 2
		}, wantErr: "schema_version"},
		{name: "blank name", change: func(m *Manifest) {
			m.Name = " "
		}, wantErr: "name"},
		{name: "blank variable", change: func(m *Manifest) {
			m.Variable = " "
		}, wantErr: "variable"},
		{name: "missing baseline", change: func(m *Manifest) {
			m.Baseline = nil
		}, wantErr: "baseline"},
		{name: "missing treatment", change: func(m *Manifest) {
			m.Treatment = nil
		}, wantErr: "treatment"},
		{name: "variable missing from baseline", change: func(m *Manifest) {
			delete(m.Baseline, "email")
		}, wantErr: "baseline"},
		{name: "variable missing from treatment", change: func(m *Manifest) {
			delete(m.Treatment, "email")
		}, wantErr: "treatment"},
		{name: "different key counts", change: func(m *Manifest) {
			m.Treatment["locale"] = "en-US"
		}, wantErr: "key sets differ"},
		{name: "different keys", change: func(m *Manifest) {
			delete(m.Treatment, "region")
			m.Treatment["locale"] = "en-US"
		}, wantErr: "key sets differ"},
		{name: "blank persona key", change: func(m *Manifest) {
			m.Baseline[""] = "same"
			m.Treatment[""] = "same"
		}, wantErr: "keys must not be blank"},
		{name: "no differences", change: func(m *Manifest) {
			m.Treatment["email"] = m.Baseline["email"]
		}, wantErr: "found 0"},
		{name: "multiple differences", change: func(m *Manifest) {
			m.Treatment["region"] = "eu-west"
		}, wantErr: "found 2"},
		{name: "wrong declared variable", change: func(m *Manifest) {
			m.Variable = "region"
		}, wantErr: "does not match"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest()
			if test.change != nil {
				test.change(&manifest)
			}

			err := manifest.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func validManifest() Manifest {
	return Manifest{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "experiment-001-email",
		Variable:      "email",
		Baseline: Persona{
			"email":  "baseline@example.invalid",
			"region": "us-east",
		},
		Treatment: Persona{
			"email":  "treatment@example.invalid",
			"region": "us-east",
		},
	}
}
