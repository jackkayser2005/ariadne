package analysis

import (
	"strings"
	"testing"
)

func TestNormalizeNetworkRejectsTrailingData(t *testing.T) {
	valid := networkArtifact(`{"schema_version":1,"region":"us-east"}`)
	if _, err := NormalizeNetwork(strings.NewReader(valid + " {}")); err == nil {
		t.Fatal("NormalizeNetwork() accepted trailing data")
	}
}
