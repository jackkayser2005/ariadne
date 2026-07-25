package jsoncheck

import (
	"strings"
	"testing"
)

func TestRejectDuplicateKeys(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "object", input: `{"outer":{"key":"value"},"array":[1,true,null]}`},
		{name: "primitive", input: `"value"`},
		{name: "empty", wantErr: "unexpected end"},
		{name: "malformed", input: `{"key":`, wantErr: "unexpected end"},
		{
			name:    "top-level duplicate",
			input:   `{"key":1,"key":2}`,
			wantErr: `duplicate key "key"`,
		},
		{
			name:    "nested duplicate",
			input:   `{"outer":{"key":1,"key":2}}`,
			wantErr: `duplicate key "key"`,
		},
		{
			name:    "array duplicate",
			input:   `[{"key":1,"key":2}]`,
			wantErr: `duplicate key "key"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := RejectDuplicateKeys([]byte(test.input))
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("RejectDuplicateKeys() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"RejectDuplicateKeys() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}
