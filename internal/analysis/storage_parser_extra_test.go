package analysis

import (
	"strings"
	"testing"
)

func TestNormalizeRejectsMalformedStorageBytes(t *testing.T) {
	network := networkArtifact(`{"schema_version":1,"region":"us-east"}`)
	if _, err := Normalize(strings.NewReader(string([]byte{0xff})), strings.NewReader(network)); err == nil {
		t.Fatal("Normalize() accepted invalid UTF-8 storage")
	}
	if _, err := Normalize(strings.NewReader("{"), strings.NewReader(network)); err == nil {
		t.Fatal("Normalize() accepted malformed storage")
	}
}
