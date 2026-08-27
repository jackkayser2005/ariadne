package analysis

import (
	"strings"
	"testing"
)

func TestNormalizeRejectsNonObjectStorage(t *testing.T) {
	network := networkArtifact(`{"schema_version":1,"region":"us-east"}`)
	if _, err := Normalize(strings.NewReader("[]"), strings.NewReader(network)); err == nil {
		t.Fatal("Normalize() accepted an array as storage observation")
	}
}
