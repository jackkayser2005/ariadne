package adb

import (
	"strings"
	"testing"
)

func TestAuthParserRejectsMalformedChallengeAndInput(t *testing.T) {
	expected := strings.Repeat("a", 64)
	if err := validateObservationChallenge([]byte("{"), expected); err == nil {
		t.Fatal("validateObservationChallenge() accepted malformed JSON")
	}
	if _, err := encodeFixtureInput(fixtureInput{}); err == nil {
		t.Fatal("encodeFixtureInput() accepted invalid input")
	}
}
