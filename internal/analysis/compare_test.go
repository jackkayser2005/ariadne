package analysis

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

const baselineBody = `{"schema_version":1,"region":"us-east","variant":"standard"}`

func TestNormalize(t *testing.T) {
	session, err := Normalize(
		strings.NewReader(baselineBody),
		strings.NewReader(networkArtifact(baselineBody)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(session.Fields, map[string]string{
		"region":  "us-east",
		"variant": "standard",
	}) {
		t.Fatalf("Normalize() = %#v", session)
	}

	network, err := NormalizeNetwork(strings.NewReader(networkArtifact(baselineBody)))
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(network.Fields, session.Fields) {
		t.Fatalf("NormalizeNetwork() = %#v", network)
	}
}

func TestNormalizeRejectsInvalidArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		storage string
		network string
		want    string
	}{
		{name: "empty storage", network: networkArtifact(baselineBody), want: "empty input"},
		{
			name:    "storage duplicate",
			storage: `{"schema_version":1,"region":"first","region":"second","variant":"standard"}`,
			network: networkArtifact(baselineBody),
			want:    `duplicate key "region"`,
		},
		{
			name:    "storage invalid field",
			storage: strings.Replace(baselineBody, `"variant"`, `"bad/field"`, 1),
			network: networkArtifact(baselineBody),
			want:    `field "bad/field" is invalid`,
		},
		{
			name:    "storage non-string field",
			storage: strings.Replace(baselineBody, `"standard"`, `1`, 1),
			network: networkArtifact(baselineBody),
			want:    `field "variant": value must be a string`,
		},
		{
			name:    "storage trailing data",
			storage: baselineBody + `{}`,
			network: networkArtifact(baselineBody),
			want:    "trailing data",
		},
		{
			name:    "storage schema",
			storage: strings.Replace(baselineBody, `"schema_version":1`, `"schema_version":2`, 1),
			network: networkArtifact(baselineBody),
			want:    "unsupported schema_version",
		},
		{
			name:    "storage no fields",
			storage: `{"schema_version":1}`,
			network: networkArtifact(baselineBody),
			want:    "expected 1 to 64 observation fields",
		},
		{
			name:    "storage value",
			storage: strings.Replace(baselineBody, `"us-east"`, `" bad"`, 1),
			network: networkArtifact(baselineBody),
			want:    `field "region": value is invalid`,
		},
		{
			name:    "network duplicate",
			storage: baselineBody,
			network: strings.Replace(
				networkArtifact(baselineBody),
				`"method":"POST"`,
				`"method":"POST","method":"GET"`,
				1,
			),
			want: `duplicate key "method"`,
		},
		{
			name:    "network metadata",
			storage: baselineBody,
			network: strings.Replace(networkArtifact(baselineBody), `"POST"`, `"GET"`, 1),
			want:    "unexpected request metadata",
		},
		{
			name:    "network base64",
			storage: baselineBody,
			network: strings.Replace(
				networkArtifact(baselineBody),
				base64.StdEncoding.EncodeToString([]byte(baselineBody)),
				"***",
				1,
			),
			want: "body_base64 is invalid",
		},
		{
			name:    "network body malformed",
			storage: baselineBody,
			network: networkArtifact(`{"schema_version":`),
			want:    "network observation body",
		},
		{
			name:    "sources disagree",
			storage: baselineBody,
			network: networkArtifact(strings.Replace(baselineBody, "standard", "personalized", 1)),
			want:    "observations disagree",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Normalize(
				strings.NewReader(test.storage),
				strings.NewReader(test.network),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Normalize() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestNormalizeBoundsAndReadFailures(t *testing.T) {
	_, err := Normalize(
		strings.NewReader(strings.Repeat(" ", maxStorageBytes+1)),
		strings.NewReader(networkArtifact(baselineBody)),
	)
	if err == nil || !strings.Contains(err.Error(), "storage observation: exceeds") {
		t.Fatalf("Normalize() error = %v", err)
	}

	_, err = Normalize(
		strings.NewReader(baselineBody),
		strings.NewReader(strings.Repeat(" ", maxNetworkBytes+1)),
	)
	if err == nil || !strings.Contains(err.Error(), "network observation: exceeds") {
		t.Fatalf("Normalize() error = %v", err)
	}

	_, err = Normalize(
		iotest.ErrReader(errors.New("read failed")),
		strings.NewReader(networkArtifact(baselineBody)),
	)
	if err == nil || !strings.Contains(err.Error(), "storage observation: read input") {
		t.Fatalf("Normalize() error = %v", err)
	}

	_, err = Normalize(
		strings.NewReader(baselineBody),
		iotest.ErrReader(errors.New("read failed")),
	)
	if err == nil || !strings.Contains(err.Error(), "network observation: read input") {
		t.Fatalf("Normalize() error = %v", err)
	}
}

func TestNormalizeDoesNotExposeValues(t *testing.T) {
	const secret = "do-not-print-observed-value"
	storage := strings.Replace(baselineBody, "standard", secret, 1)
	network := networkArtifact(baselineBody)

	_, err := Normalize(strings.NewReader(storage), strings.NewReader(network))
	if err == nil {
		t.Fatal("Normalize() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("Normalize() exposed value: %v", err)
	}
}

func TestNormalizeAcceptsGenericFields(t *testing.T) {
	const observation = `{"schema_version":1,"consent.status":"granted","request-id":"fixed"}`
	session, err := Normalize(
		strings.NewReader(observation),
		strings.NewReader(networkArtifact(observation)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(session.Fields, map[string]string{
		"consent.status": "granted",
		"request-id":     "fixed",
	}) {
		t.Fatalf("Normalize() = %#v", session)
	}
}

func TestNormalizeRejectsTooManyFields(t *testing.T) {
	observation := map[string]any{"schema_version": 1}
	for index := 0; index <= maxFields; index++ {
		observation["field-"+strconv.Itoa(index)] = "value"
	}
	data, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Normalize(bytes.NewReader(data), strings.NewReader(networkArtifact(string(data))))
	if err == nil || !strings.Contains(err.Error(), "expected 1 to 64 observation fields") {
		t.Fatalf("Normalize() error = %v", err)
	}
}

func TestCompare(t *testing.T) {
	comparison := Compare(
		Session{Fields: map[string]string{"region": "us-east", "variant": "standard"}},
		Session{Fields: map[string]string{"region": "us-east", "variant": "personalized"}},
	)
	if comparison.SchemaVersion != 3 ||
		len(comparison.UnchangedFields) != 1 ||
		comparison.UnchangedFields[0] != "region" ||
		len(comparison.Differences) != 1 ||
		len(comparison.Unknowns) != 0 {
		t.Fatalf("Compare() = %#v", comparison)
	}
	difference := comparison.Differences[0]
	if difference.Field != "variant" ||
		difference.Kind != "changed" ||
		difference.Baseline != "standard" ||
		difference.Treatment != "personalized" ||
		difference.State != evidence.Observed ||
		len(difference.Evidence) != 4 {
		t.Fatalf("difference = %#v", difference)
	}

	data, err := json.Marshal(comparison)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"schema_version":3,"unchanged_fields":["region"],"differences":[{"field":"variant","kind":"changed","baseline":"standard","treatment":"personalized","state":"observed","evidence":["baseline/observations/storage.json#/variant","baseline/observations/network.json#decoded-body/variant","treatment/observations/storage.json#/variant","treatment/observations/network.json#decoded-body/variant"]}],"unknowns":[]}`
	if string(data) != want {
		t.Fatalf("comparison JSON = %s", data)
	}
}

func TestCompareMultipleAndNoDifferences(t *testing.T) {
	multiple := Compare(
		Session{Fields: map[string]string{"region": "east", "variant": "standard"}},
		Session{Fields: map[string]string{"region": "west", "variant": "personalized"}},
	)
	if len(multiple.Differences) != 2 || multiple.Differences[0].Field != "region" {
		t.Fatalf("Compare() = %#v", multiple)
	}

	none := Compare(
		Session{Fields: map[string]string{"region": "east", "variant": "standard"}},
		Session{Fields: map[string]string{"region": "east", "variant": "standard"}},
	)
	if len(none.Differences) != 0 || len(none.UnchangedFields) != 2 {
		t.Fatalf("Compare() = %#v", none)
	}
}

func TestCompareAddedAndRemovedFields(t *testing.T) {
	comparison := Compare(
		Session{Fields: map[string]string{"removed": "before", "stable": "same"}},
		Session{Fields: map[string]string{"added": "after", "stable": "same"}},
	)
	if len(comparison.Differences) != 2 {
		t.Fatalf("Compare() = %#v", comparison)
	}
	added := comparison.Differences[0]
	if added.Field != "added" ||
		added.Kind != "added" ||
		added.Baseline != "" ||
		added.Treatment != "after" ||
		len(added.Evidence) != 2 {
		t.Fatalf("added difference = %#v", added)
	}
	removed := comparison.Differences[1]
	if removed.Field != "removed" ||
		removed.Kind != "removed" ||
		removed.Baseline != "before" ||
		removed.Treatment != "" ||
		len(removed.Evidence) != 2 {
		t.Fatalf("removed difference = %#v", removed)
	}
}

func networkArtifact(body string) string {
	data, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"method":         "POST",
		"path":           "/observe",
		"content_type":   "application/json",
		"body_base64":    base64.StdEncoding.EncodeToString([]byte(body)),
	})
	if err != nil {
		panic(err)
	}
	return string(bytes.TrimSpace(data))
}
