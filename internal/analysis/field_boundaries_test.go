package analysis

import (
	"strings"
	"testing"
)

func TestObservationFieldValidationRejectsUnsafeBounds(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("x", 129), "contains space"} {
		if validFieldName(value) {
			t.Fatalf("validFieldName(%q) = true", value)
		}
	}
	for _, value := range []string{"", strings.Repeat("x", 1025), "contains space"} {
		if validFieldValue(value) {
			t.Fatalf("validFieldValue(%q) = true", value)
		}
	}
}
