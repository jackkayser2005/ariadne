package analysis

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

func TestSessionChallengeAccessors(t *testing.T) {
	legacy := Session{}
	if legacy.HasChallenge() || legacy.ChallengeCommitment() != "" {
		t.Fatalf("legacy session accessors = challenge=%v commitment=%q", legacy.HasChallenge(), legacy.ChallengeCommitment())
	}
	challenge := strings.Repeat("a", 64)
	observation := `{"schema_version":1,"challenge":"` + challenge + `","region":"us-east"}`
	session, err := Normalize(strings.NewReader(observation), strings.NewReader(networkArtifact(observation)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(challenge))
	if !session.HasChallenge() || session.ChallengeCommitment() != hex.EncodeToString(digest[:]) {
		t.Fatalf("authenticated session accessors = challenge=%v commitment=%q", session.HasChallenge(), session.ChallengeCommitment())
	}
}

func TestNormalizeNetworkRejectsBoundaryInputs(t *testing.T) {
	body := `{"schema_version":1,"region":"us-east"}`
	valid := networkArtifact(body)
	encodedBody := base64.StdEncoding.EncodeToString([]byte(body))
	tests := []struct {
		name string
		data string
	}{
		{name: "empty", data: ""},
		{name: "invalid utf8", data: string([]byte{0xff})},
		{name: "malformed json", data: "{"},
		{name: "unknown field", data: strings.Replace(valid, `"body_base64":`, `"extra":"value","body_base64":`, 1)},
		{name: "unsupported schema", data: strings.Replace(valid, `"schema_version":1`, `"schema_version":2`, 1)},
		{name: "unexpected metadata", data: strings.Replace(valid, `"method":"POST"`, `"method":"GET"`, 1)},
		{name: "invalid body encoding", data: strings.Replace(valid, encodedBody, "not-base64", 1)},
		{name: "oversized body", data: networkArtifact(strings.Repeat("x", maxStorageBytes+1))},
		{name: "malformed body", data: networkArtifact("{")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeNetwork(strings.NewReader(test.data)); err == nil {
				t.Fatal("NormalizeNetwork() error = nil")
			}
		})
	}
}
