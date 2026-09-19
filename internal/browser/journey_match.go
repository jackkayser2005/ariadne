package browser

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

const maxMarkerPayloadBytes = 256 << 10

// SyntheticMarker is transient experiment input. Its value must never enter a
// portable artifact, log, provenance digest, or everyday protection profile.
type SyntheticMarker struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Value    string `json:"value"`
}

type markerVariant struct {
	value string
	match trace.JourneyMatch
}

// MarkerMatcher holds a small, precomputed catalog of supported representations.
type MarkerMatcher struct{ variants []markerVariant }

// GenerateMarkers creates fresh reserved-domain inputs without using personal data.
func GenerateMarkers() []SyntheticMarker {
	return []SyntheticMarker{
		{ID: "m1", Category: "email", Value: "ariadne-" + strings.ToLower(rand.Text()) + "@example.invalid"},
		{ID: "m2", Category: "account-id", Value: "ariadne-" + strings.ToLower(rand.Text())},
	}
}

// NewMarkerMatcher validates at most 16 synthetic inputs and compiles bounded
// exact, URL-encoded, Base64, and SHA-256 representations. These are observations
// of matching bytes, not proof of who transformed them or where they came from.
func NewMarkerMatcher(markers []SyntheticMarker) (*MarkerMatcher, error) {
	if len(markers) == 0 || len(markers) > 16 {
		return nil, errors.New("synthetic marker count is invalid")
	}
	matcher := &MarkerMatcher{}
	seen := map[string]bool{}
	for i, marker := range markers {
		if marker.ID != "m"+strconv.Itoa(i+1) || seen[marker.ID] || len(marker.Value) < 8 || len(marker.Value) > 256 || strings.ContainsAny(marker.Value, "\x00\r\n") || !slices.ContainsFunc(trace.CategoryDefinitions(), func(c trace.CategoryDefinition) bool { return c.ID == marker.Category }) {
			return nil, errors.New("synthetic marker is invalid")
		}
		seen[marker.ID] = true
		add := func(encoding string, values ...string) {
			for _, value := range values {
				if encoding != "exact" && value == marker.Value {
					continue
				}
				variant := markerVariant{value: value, match: trace.JourneyMatch{Marker: marker.ID, Category: marker.Category, Encoding: encoding}}
				if !slices.Contains(matcher.variants, variant) {
					matcher.variants = append(matcher.variants, variant)
				}
			}
		}
		add("exact", marker.Value)
		add("url", url.QueryEscape(marker.Value), url.PathEscape(marker.Value), strings.ReplaceAll(url.QueryEscape(marker.Value), "+", "%20"))
		add("base64", base64.StdEncoding.EncodeToString([]byte(marker.Value)), base64.RawStdEncoding.EncodeToString([]byte(marker.Value)), base64.URLEncoding.EncodeToString([]byte(marker.Value)), base64.RawURLEncoding.EncodeToString([]byte(marker.Value)))
		digest := sha256.Sum256([]byte(marker.Value))
		add("sha256", hex.EncodeToString(digest[:]))
	}
	return matcher, nil
}

// Match checks only bounded transient text. Truncated or oversized payloads are
// reported as unavailable, never as evidence that a marker was absent.
func (matcher *MarkerMatcher) Match(payload string) (matches []trace.JourneyMatch, unavailable bool) {
	matches = []trace.JourneyMatch{}
	if matcher == nil || len(payload) > maxMarkerPayloadBytes {
		return matches, true
	}
	for _, variant := range matcher.variants {
		if strings.Contains(payload, variant.value) && !slices.Contains(matches, variant.match) {
			matches = append(matches, variant.match)
		}
	}
	return matches, false
}
