package browser

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestSyntheticMarkerRepresentationsAndNegativeControls(t *testing.T) {
	marker := SyntheticMarker{ID: "m1", Category: "email", Value: "ariadne-test+space name@example.invalid"}
	matcher, err := NewMarkerMatcher([]SyntheticMarker{marker})
	if err != nil {
		t.Fatal(err)
	}
	for encoding, payload := range map[string]string{
		"exact":  marker.Value,
		"url":    url.QueryEscape(marker.Value),
		"base64": base64.StdEncoding.EncodeToString([]byte(marker.Value)),
		"sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(marker.Value))),
	} {
		matches, unavailable := matcher.Match("prefix:" + payload + ":suffix")
		if unavailable || len(matches) != 1 || matches[0].Encoding != encoding || matches[0].Marker != "m1" {
			t.Fatalf("%s: %#v, %v", encoding, matches, unavailable)
		}
	}
	for _, payload := range []string{"", "different@example.invalid", "ariadne-test", strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256([]byte(marker.Value))))} {
		matches, unavailable := matcher.Match(payload)
		if unavailable || len(matches) != 0 {
			t.Fatal("negative control matched")
		}
	}
	if _, unavailable := matcher.Match(strings.Repeat("x", maxMarkerPayloadBytes+1)); !unavailable {
		t.Fatal("oversized input claimed visible")
	}
	if matches, unavailable := (*MarkerMatcher)(nil).Match(marker.Value); !unavailable || len(matches) != 0 {
		t.Fatal("missing matcher claimed visible")
	}
}

func TestSyntheticInputsAreFreshAndBounded(t *testing.T) {
	a, b := GenerateMarkers(), GenerateMarkers()
	if a[0].Value == b[0].Value || !strings.HasSuffix(a[0].Value, "@example.invalid") {
		t.Fatal("unsafe synthetic input")
	}
	if _, err := NewMarkerMatcher(a); err != nil {
		t.Fatal(err)
	}
	valid := SyntheticMarker{ID: "m1", Category: "email", Value: "abcdefgh"}
	for _, markers := range [][]SyntheticMarker{nil, make([]SyntheticMarker, 17), {{ID: "m2", Category: "email", Value: "abcdefgh"}}, {{ID: "m1", Category: "unsupported", Value: "abcdefgh"}}, {{ID: "m1", Category: "email", Value: "short"}}, {{ID: "m1", Category: "email", Value: strings.Repeat("x", 257)}}, {{ID: "m1", Category: "email", Value: "abcdefgh\n"}}, {valid, valid}} {
		if _, err := NewMarkerMatcher(markers); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	matcher, _ := NewMarkerMatcher([]SyntheticMarker{valid})
	matches, unavailable := matcher.Match("abcdefghabcdefgh")
	if unavailable || len(matches) != 1 || matches[0].Encoding != "exact" {
		t.Fatal(matches)
	}
}
