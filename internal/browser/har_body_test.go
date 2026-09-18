package browser

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestInspectHARBodyExtractsJSONStringsAndDecodedFormValues(t *testing.T) {
	jsonText := `{"email-key":"key-value-is-a-string","nested":{"value":"nested-value"},"array":["array-value",{"deep":"deep-value"}],"number":12345}`
	jsonBody := inspectHARBody(&harPostData{
		MimeType: "application/vnd.api+json; charset=UTF-8",
		Text:     &jsonText,
	}, nil)
	if jsonBody.gap || !jsonBody.inspected || jsonBody.channel != "JSON body string" {
		t.Fatalf("JSON body inspection = %#v", jsonBody)
	}
	wantJSON := map[string]bool{
		"key-value-is-a-string": true,
		"nested-value":          true,
		"array-value":           true,
		"deep-value":            true,
	}
	gotJSON := map[string]bool{}
	for _, value := range jsonBody.values {
		gotJSON[value] = true
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("JSON strings = %#v, want %#v", gotJSON, wantJSON)
	}
	if len(jsonBody.values) != len(wantJSON) {
		t.Fatalf("JSON string count = %d, want %d", len(jsonBody.values), len(wantJSON))
	}
	if gotJSON["email-key"] || gotJSON["12345"] {
		t.Fatalf("JSON keys or numbers were treated as strings: %#v", gotJSON)
	}

	formText := "email=body-email%40example.test&literal=body%252Fvalue&email=second%2Bvalue"
	formBody := inspectHARBody(&harPostData{
		MimeType: "application/x-www-form-urlencoded",
		Text:     &formText,
	}, nil)
	if formBody.gap || !formBody.inspected || formBody.channel != "Form body value" {
		t.Fatalf("form body inspection = %#v", formBody)
	}
	wantForm := []string{"body-email@example.test", "second+value", "body%2Fvalue"}
	gotForm := append([]string(nil), formBody.values...)
	sort.Strings(gotForm)
	sort.Strings(wantForm)
	if !reflect.DeepEqual(gotForm, wantForm) {
		t.Fatalf("form values = %#v, want %#v", gotForm, wantForm)
	}
}

func TestInspectHARBodyRejectsUnsupportedOrUnsafeRepresentations(t *testing.T) {
	validJSON := `{"value":"safe-body-value"}`
	oversizedJSON := `"` + strings.Repeat("x", 64<<10) + `"`
	deepJSON := `"deep-value"`
	for i := 0; i < 33; i++ {
		deepJSON = `{"nested":` + deepJSON + `}`
	}

	tests := []struct {
		name string
		post harPostData
	}{
		{
			name: "missing text",
			post: harPostData{MimeType: "application/json"},
		},
		{
			name: "oversized text",
			post: harPostData{MimeType: "application/json", Text: &oversizedJSON},
		},
		{
			name: "encoding marker",
			post: harPostData{MimeType: "application/json", Text: &validJSON, Encoding: "base64"},
		},
		{
			name: "export encoding marker",
			post: harPostData{MimeType: "application/json", Text: &validJSON, ExportEncoding: "base64"},
		},
		{
			name: "non UTF-8 charset",
			post: harPostData{MimeType: "application/json; charset=iso-8859-1", Text: &validJSON},
		},
		{
			name: "unsupported mime",
			post: harPostData{MimeType: "text/plain", Text: &validJSON},
		},
		{
			name: "duplicate JSON keys",
			post: harPostData{MimeType: "application/json", Text: stringPointer(`{"value":"first","value":"second"}`)},
		},
		{
			name: "malformed JSON",
			post: harPostData{MimeType: "application/json", Text: stringPointer(`{"value":`)},
		},
		{
			name: "trailing JSON",
			post: harPostData{MimeType: "application/json", Text: stringPointer(`{"value":"safe-body-value"}{"other":"value"}`)},
		},
		{
			name: "nested beyond limit",
			post: harPostData{MimeType: "application/json", Text: &deepJSON},
		},
		{
			name: "malformed form escape",
			post: harPostData{MimeType: "application/x-www-form-urlencoded", Text: stringPointer("value=%ZZ")},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := inspectHARBody(&test.post, nil)
			if !got.gap || got.inspected || len(got.values) != 0 || got.channel != "" {
				t.Fatalf("inspectHARBody() = %#v, want an uninspected gap", got)
			}
		})
	}
}

func TestInspectHARBodyCountsMetadataGapsAndHonorsConfiguration(t *testing.T) {
	positiveSize := int64(1)
	zeroSize := int64(0)

	if got := inspectHARBody(nil, &positiveSize); !got.gap || got.inspected {
		t.Fatalf("positive bodySize without postData = %#v, want an uninspected gap", got)
	}
	if got := inspectHARBody(nil, nil); got.gap || got.inspected {
		t.Fatalf("absent body metadata = %#v, want no counted gap", got)
	}
	if got := inspectHARBody(nil, &zeroSize); got.gap || got.inspected {
		t.Fatalf("zero bodySize without postData = %#v, want no counted gap", got)
	}
}

func TestInspectHARBodyMatchesIntegratedReviewWithReferencesAndRedaction(t *testing.T) {
	email := "body-email@example.test"
	account := "body-account-12345"
	rules := []harRule{
		{Category: "email", Value: email},
		{Category: "account-id", Value: account},
	}
	entries := []string{
		`{"request":{"url":"https://example.test/one","postData":{"mimeType":"application/json","text":"{\"nested\":{\"email\":\"body-email@example.test\"},\"again\":[\"body-email@example.test\"]}"}}}`,
		`{"request":{"url":"https://example.test/two","postData":{"mimeType":"application/json","text":"{\"email\":\"body-email@example.test\"}"}}}`,
		`{"request":{"url":"https://example.test/three","postData":{"mimeType":"application/x-www-form-urlencoded","text":"email=body-email%40example.test"}}}`,
		`{"request":{"url":"https://example.test/four","bodySize":20}}`,
		`{"request":{"url":"https://example.test/five"}}`,
		`{"request":{"url":"https://other.test/six","postData":{"mimeType":"application/json","text":"{\"account\":\"body-account-12345\"}"}}}`,
	}
	raw := `{"log":{"version":"1.2","entries":[` + strings.Join(entries, ",") + `]}}`

	review, err := inspectHAR([]byte(raw), "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if !review.MatchingConfigured || review.Requests != len(entries) || len(review.Destinations) != 2 {
		t.Fatalf("configured review = %#v", review)
	}
	website := review.Destinations[0]
	if website.Label != "Website origin" || website.Requests != 5 || website.InspectedBodies != 3 || website.UninspectedBodies != 1 {
		t.Fatalf("website body accounting = %#v", website)
	}
	other := review.Destinations[1]
	if other.Label != "Other origin 1" || other.Requests != 1 || other.InspectedBodies != 1 || other.UninspectedBodies != 0 {
		t.Fatalf("other body accounting = %#v", other)
	}

	findMatch := func(destination HARDestination, category, channel string) HARMatch {
		for _, match := range destination.Matches {
			if match.Category == category && match.Channel == channel {
				return match
			}
		}
		t.Fatalf("missing %s/%s match in %#v", category, channel, destination.Matches)
		return HARMatch{}
	}
	emailJSON := findMatch(website, "email", "JSON body string")
	if emailJSON.Requests != 2 || !reflect.DeepEqual(emailJSON.Entries, []int{1, 2}) {
		t.Fatalf("JSON match refs = %#v, want two deduplicated entries", emailJSON)
	}
	emailForm := findMatch(website, "email", "Form body value")
	if emailForm.Requests != 1 || !reflect.DeepEqual(emailForm.Entries, []int{3}) {
		t.Fatalf("form match refs = %#v", emailForm)
	}
	accountJSON := findMatch(other, "account-id", "JSON body string")
	if accountJSON.Requests != 1 || !reflect.DeepEqual(accountJSON.Entries, []int{6}) {
		t.Fatalf("other-origin match refs = %#v", accountJSON)
	}

	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{email, account, "example.test", "other.test"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("review retained %q: %s", secret, encoded)
		}
	}

	unconfigured, err := inspectHAR([]byte(raw), "https://example.test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if unconfigured.MatchingConfigured || len(unconfigured.Destinations) != 2 {
		t.Fatalf("unconfigured review = %#v", unconfigured)
	}
	for _, destination := range unconfigured.Destinations {
		if len(destination.Matches) != 0 || destination.InspectedBodies != 0 || destination.UninspectedBodies != 0 {
			t.Fatalf("body inspection ran without rules: %#v", destination)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}
