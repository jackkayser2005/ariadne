package analysis

import (
	"strings"
	"testing"
)

func TestNormalizeRejectsInvalidChallengeCharacter(t *testing.T) {
	challenge := strings.Repeat("g", 64)
	observation := `{"schema_version":1,"challenge":"` + challenge + `","region":"us-east"}`
	if _, err := Normalize(strings.NewReader(observation), strings.NewReader(networkArtifact(observation))); err == nil {
		t.Fatal("Normalize() accepted a non-hex challenge")
	}
}
