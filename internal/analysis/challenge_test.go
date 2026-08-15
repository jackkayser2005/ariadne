package analysis

import (
	"maps"
	"strings"
	"testing"
)

func TestNormalizeTreatsMatchingChallengeAsProtocolMetadata(t *testing.T) {
	challenge := strings.Repeat("0123456789abcdef", 4)
	observation := `{"schema_version":1,"challenge":"` + challenge + `","region":"us-east","variant":"standard"}`
	session, err := Normalize(strings.NewReader(observation), strings.NewReader(networkArtifact(observation)))
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(session.Fields, map[string]string{
		"region":  "us-east",
		"variant": "standard",
	}) {
		t.Fatalf("Normalize() fields = %#v", session.Fields)
	}
}

func TestNormalizeRejectsChallengeDisagreement(t *testing.T) {
	first := strings.Repeat("0123456789abcdef", 4)
	second := strings.Repeat("fedcba9876543210", 4)
	storage := `{"schema_version":1,"challenge":"` + first + `","region":"us-east"}`
	network := `{"schema_version":1,"challenge":"` + second + `","region":"us-east"}`
	_, err := Normalize(strings.NewReader(storage), strings.NewReader(networkArtifact(network)))
	if err == nil || !strings.Contains(err.Error(), "observations disagree") {
		t.Fatalf("Normalize() error = %v", err)
	}

	_, err = Normalize(
		strings.NewReader(`{"schema_version":1,"region":"us-east"}`),
		strings.NewReader(networkArtifact(storage)),
	)
	if err == nil || !strings.Contains(err.Error(), "observations disagree") {
		t.Fatalf("Normalize() missing challenge error = %v", err)
	}
}

func TestNormalizeRejectsInvalidOrDuplicateChallenge(t *testing.T) {
	challenge := strings.Repeat("0123456789abcdef", 4)
	valid := `{"schema_version":1,"challenge":"` + challenge + `","region":"us-east"}`
	for _, body := range []string{
		strings.Replace(valid, challenge, "not-a-challenge", 1),
		`{"schema_version":1,"challenge":"` + challenge + `","challenge":"` + challenge + `","region":"us-east"}`,
	} {
		if _, err := Normalize(strings.NewReader(body), strings.NewReader(networkArtifact(body))); err == nil {
			t.Fatalf("Normalize() accepted invalid challenge body: %s", body)
		}
	}
}
