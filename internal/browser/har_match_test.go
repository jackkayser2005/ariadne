package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func ruleFile(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private-rules.json")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validRules = `{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"test-person@example.test"},{"category":"account-id","value":"account-12345"}]}`

func TestHARExactMatchesSeparateFromHints(t *testing.T) {
	rules, err := readHARRules(ruleFile(t, validRules))
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"log":{"version":"1.2","entries":[
 {"request":{"url":"https://example.test/?unrelated=test-person%40example.test&repeat=test-person%40example.test&email=not-the-value","postData":{"params":[{"name":"anything","value":"account-12345"}]}}},
 {"request":{"url":"https://example.test/?x=test-person%40example.test"}},
 {"request":{"url":"https://other.test/?email=prefix-test-person%40example.test&x=test-person%2540example.test"}},
 {"request":{"url":"https://other.test/","postData":{"params":[{"name":"unrelated","value":"test-person@example.test"}]}}}
 ]}}`
	review, err := inspectHAR([]byte(raw), "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if !review.MatchingConfigured || review.SchemaVersion != 4 {
		t.Fatal(review)
	}
	first := review.Destinations[0]
	if len(first.Matches) != 2 || first.Matches[0].Category != "account-id" || first.Matches[0].Requests != 1 || first.Matches[1].Category != "email" || first.Matches[1].Requests != 2 {
		t.Fatal(first)
	}
	if !reflect.DeepEqual(first.Matches[0].Entries, []int{1}) || !reflect.DeepEqual(first.Matches[1].Entries, []int{1, 2}) {
		t.Fatal("incorrect match references", first.Matches)
	}
	second := review.Destinations[1]
	if len(second.Matches) != 1 || second.Matches[0].Channel != "Exported form parameter" || second.Matches[0].Requests != 1 || !reflect.DeepEqual(second.Matches[0].Entries, []int{4}) {
		t.Fatal(second)
	}
	encoded, _ := json.Marshal(review)
	for _, secret := range []string{"test-person", "account-12345", "example.test", "unrelated"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("retained", secret)
		}
	}
	noRules, err := InspectHAR([]byte(raw), "https://example.test")
	if err != nil || noRules.MatchingConfigured || len(noRules.Destinations[0].Matches) != 0 {
		t.Fatal("unconfigured matching", noRules, err)
	}
}

func TestHARRulesFailClosed(t *testing.T) {
	inputs := []string{"", "null", "{}", validRules + "{}", strings.Replace(validRules, "true", "false", 1), strings.Replace(validRules, `"email"`, `"location"`, 1), strings.Replace(validRules, "test-person@example.test", "short", 1), strings.Replace(validRules, "test-person@example.test", "has whitespace", 1), strings.Replace(validRules, "test-person@example.test", strings.Repeat("x", 257), 1), strings.Replace(validRules, "account-12345", "test-person@example.test", 1), strings.Replace(validRules, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), strings.Replace(validRules, `"synthetic":true`, `"synthetic":true,"unexpected":"secret"`, 1), strings.Repeat("[", 5) + "0" + strings.Repeat("]", 5), string([]byte{0xff}), strings.Repeat(" ", 16385), `{"schema_version":1,"synthetic":true,"rules":[]}`}
	for i, raw := range inputs {
		if _, err := readHARRules(ruleFile(t, raw)); err == nil || strings.Contains(err.Error(), "test-person") {
			t.Fatalf("case %d: %v", i, err)
		}
	}
	if _, err := readHARRules(filepath.Join(t.TempDir(), "missing-secret")); err == nil || strings.Contains(err.Error(), "missing-secret") {
		t.Fatal(err)
	}
}

func TestHARMatchReportRedactionAndRuleFreshness(t *testing.T) {
	rules := ruleFile(t, validRules)
	input := ruleFile(t, `{"log":{"version":"1.2","entries":[{"request":{"url":"https://example.test/?x=test-person%40example.test"}},{"request":{"url":"https://other.test/?email=not-matching"}}]}}`)
	report, err := ReadHARReport(input, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, want := range []string{"Supplied values found in the capture", "No supplied values matched", "These clues are separate", "presence in this file", "Show the supporting entries", "entry 1", "Reordering the export changes them"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(text, "test-person") || strings.Contains(text, "Values were not inspected") {
		t.Fatal("unsafe or incorrect report")
	}
	if err := os.WriteFile(rules, []byte(`{"secret":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHARReport(input, "https://example.test", rules); err == nil {
		t.Fatal("stale rules")
	}
}

func TestHARMatchSameCategoryCountsOnce(t *testing.T) {
	d := HARDestination{}
	rules := []harRule{{Category: "email", Value: "first@example.test"}, {Category: "email", Value: "second@example.test"}}
	addHARMatches(&d, map[string][]string{"x": {"first@example.test", "second@example.test"}}, nil, rules, 7, harBody{})
	if len(d.Matches) != 1 || d.Matches[0].Requests != 1 || len(d.Matches[0].Entries) != 1 || d.Matches[0].Entries[0] != 7 {
		t.Fatal(d)
	}
}

func BenchmarkHARReviewNearLimit(b *testing.B) {
	entry := `{"request":{"url":"https://example.test/?value=test-person%40example.test"},"comment":"` + strings.Repeat("x", 680) + `"}`
	raw := []byte(`{"log":{"version":"1.2","entries":[` + strings.TrimSuffix(strings.Repeat(entry+",", 10000), ",") + `]}}`)
	if len(raw) > maxHARBytes {
		b.Fatal("fixture exceeds parser limit")
	}
	rules := []harRule{{Category: "email", Value: "test-person@example.test"}}
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		review, err := inspectHAR(raw, "https://example.test", rules)
		if err != nil || len(review.Destinations) != 1 || len(review.Destinations[0].Matches[0].Entries) != 10000 {
			b.Fatal("incomplete near-limit review", err)
		}
	}
}

func TestHARRequestHeaderAndCookieMatchesAreExactAndRedacted(t *testing.T) {
	email := "test-person@example.test"
	account := "account-12345"
	evilHeader := "<script>alert(\"header\")</script>"
	evilCookie := "<img src=x onerror=\"alert(1)\">"
	entries := []any{
		map[string]any{"request": map[string]any{
			"url": "https://example.test/one",
			"headers": []any{
				map[string]any{"name": "X-Profile", "value": email},
				map[string]any{"name": "email", "value": "not-the-value"},
				map[string]any{"name": "Authorization", "value": "Bearer " + email},
				map[string]any{"name": "Cookie", "value": "session=" + account},
			},
			"cookies": []any{
				map[string]any{"name": "account_id", "value": account},
				map[string]any{"name": "repeat", "value": email},
				map[string]any{"name": "encoded", "value": "account-12345%2F"},
			},
		}},
		map[string]any{"request": map[string]any{
			"url": "https://example.test/two",
			"headers": []any{
				map[string]any{"name": "X-Case", "value": "Test-Person@example.test"},
				map[string]any{"name": "X-Prefix", "value": "prefix-" + email},
			},
			"cookies": []any{
				map[string]any{"name": "bearer", "value": "Bearer " + account},
				map[string]any{"name": "encoded", "value": "test-person%40example.test"},
			},
		}},
		map[string]any{"request": map[string]any{
			"url": "https://example.test/three",
			"headers": []any{
				map[string]any{"name": "X-Profile", "value": email},
			},
			"cookies": []any{
				map[string]any{"name": "account", "value": account},
			},
		}},
		map[string]any{"request": map[string]any{
			"url": "https://example.test/four",
			"headers": []any{
				map[string]any{"name": "X-Evil", "value": evilHeader},
			},
			"cookies": []any{
				map[string]any{"name": "evil", "value": evilCookie},
			},
		}},
	}
	raw, err := json.Marshal(map[string]any{"log": map[string]any{"version": "1.2", "entries": entries}})
	if err != nil {
		t.Fatal(err)
	}
	rules := []harRule{{Category: "email", Value: email}, {Category: "account-id", Value: account}}
	review, err := inspectHAR(raw, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if review.SchemaVersion != 4 || review.Requests != 4 || len(review.Destinations) != 1 {
		t.Fatalf("review = %#v", review)
	}
	first := review.Destinations[0]
	if len(first.Matches) != 3 {
		t.Fatalf("matches = %#v", first.Matches)
	}
	findMatch := func(category, channel string) HARMatch {
		for _, match := range first.Matches {
			if match.Category == category && match.Channel == channel {
				return match
			}
		}
		t.Fatalf("missing %s/%s match in %#v", category, channel, first.Matches)
		return HARMatch{}
	}
	emailHeader := findMatch("email", "Request header value")
	if emailHeader.Requests != 2 || !reflect.DeepEqual(emailHeader.Entries, []int{1, 3}) {
		t.Fatalf("header match = %#v", emailHeader)
	}
	emailCookie := findMatch("email", "Request cookie value")
	if emailCookie.Requests != 1 || !reflect.DeepEqual(emailCookie.Entries, []int{1}) {
		t.Fatalf("cookie email match = %#v", emailCookie)
	}
	accountCookie := findMatch("account-id", "Request cookie value")
	if accountCookie.Requests != 2 || !reflect.DeepEqual(accountCookie.Entries, []int{1, 3}) {
		t.Fatalf("cookie account match = %#v", accountCookie)
	}
	for _, match := range first.Matches {
		if match.Channel != "Request header value" && match.Channel != "Request cookie value" {
			t.Fatalf("unexpected channel match = %#v", match)
		}
	}
	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{email, account, evilHeader, evilCookie, "Bearer"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("review retained %q: %s", forbidden, encoded)
		}
	}
	unconfigured, err := InspectHAR(raw, "https://example.test")
	if err != nil || unconfigured.SchemaVersion != 4 || len(unconfigured.Destinations[0].Matches) != 0 {
		t.Fatalf("unconfigured review = %#v, err=%v", unconfigured, err)
	}

	capture := filepath.Join(t.TempDir(), "private.har")
	if err := os.WriteFile(capture, raw, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := ReadHARReport(capture, "https://example.test", ruleFile(t, validRules))
	if err != nil {
		t.Fatal(err)
	}
	reportText := string(report)
	for _, wanted := range []string{"Request header value", "Request cookie value", "not proof of sending or receipt", "do not establish identity", "cookie purpose"} {
		if !strings.Contains(reportText, wanted) {
			t.Fatalf("report missing %q", wanted)
		}
	}
	for _, forbidden := range []string{email, account, evilHeader, evilCookie, "Bearer"} {
		if strings.Contains(reportText, forbidden) {
			t.Fatalf("report retained %q", forbidden)
		}
	}
}

func TestHARRequestMetadataOverLimitFailsClosed(t *testing.T) {
	names := strings.TrimSuffix(strings.Repeat("{\"name\":\"field\",\"value\":\"safe-value\"},", 1025), ",")
	for _, field := range []string{"\"headers\":[" + names + "]", "\"cookies\":[" + names + "]"} {
		raw := []byte("{\"log\":{\"version\":\"1.2\",\"entries\":[{\"request\":{\"url\":\"https://example.test/\", " + field + "}}]}}")
		if _, err := InspectHAR(raw, "https://example.test"); err == nil || strings.Contains(err.Error(), "safe-value") {
			t.Fatalf("metadata limit accepted or leaked: %v", err)
		}
	}
}

func TestHARURLPathSegmentMatchesAreExactOnceAndRedacted(t *testing.T) {
	email := "test+person@example.test"
	account := "account-12345"
	entries := []any{
		map[string]any{"request": map[string]any{"url": "https://example.test/one/" + email + "/" + account}},
		map[string]any{"request": map[string]any{"url": "https://example.test/two/test%2Bperson@example.test"}},
		map[string]any{"request": map[string]any{"url": "https://example.test/three/" + email + "/" + email}},
		map[string]any{"request": map[string]any{"url": "https://example.test/four/test%252Bperson@example.test"}},
		map[string]any{"request": map[string]any{"url": "https://example.test/five/prefix-" + email}},
		map[string]any{"request": map[string]any{"url": "https://example.test/six/" + email + "-suffix"}},
		map[string]any{"request": map[string]any{"url": "https://example.test/seven/" + email + "%2F" + account}},
		map[string]any{"request": map[string]any{"url": "https://example.test/eight/no-match#" + email}},
		map[string]any{"request": map[string]any{"url": "https://account-12345.example.test/nine/no-match"}},
	}
	raw, err := json.Marshal(map[string]any{"log": map[string]any{"version": "1.2", "entries": entries}})
	if err != nil {
		t.Fatal(err)
	}
	rules := []harRule{{Category: "email", Value: email}, {Category: "account-id", Value: account}}
	review, err := inspectHAR(raw, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if review.SchemaVersion != 4 || review.Requests != len(entries) || len(review.Destinations) != 2 {
		t.Fatalf("review = %#v", review)
	}
	findDestination := func(label string) HARDestination {
		for _, destination := range review.Destinations {
			if destination.Label == label {
				return destination
			}
		}
		t.Fatalf("missing destination %q", label)
		return HARDestination{}
	}
	website := findDestination("Website origin")
	if len(website.Matches) != 2 {
		t.Fatalf("website matches = %#v", website.Matches)
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
	emailMatch := findMatch(website, "email", "URL path segment")
	if emailMatch.Requests != 3 || !reflect.DeepEqual(emailMatch.Entries, []int{1, 2, 3}) {
		t.Fatalf("email path match = %#v", emailMatch)
	}
	accountMatch := findMatch(website, "account-id", "URL path segment")
	if accountMatch.Requests != 1 || !reflect.DeepEqual(accountMatch.Entries, []int{1}) {
		t.Fatalf("account path match = %#v", accountMatch)
	}
	if other := findDestination("Other origin 1"); len(other.Matches) != 0 {
		t.Fatalf("host text became a match: %#v", other.Matches)
	}
	encoded, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{email, account, "prefix-", "suffix", "example.test"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("review retained %q: %s", forbidden, encoded)
		}
	}
	unconfigured, err := InspectHAR(raw, "https://example.test")
	if err != nil || unconfigured.SchemaVersion != 4 {
		t.Fatalf("unconfigured review = %#v, err=%v", unconfigured, err)
	}
	for _, destination := range unconfigured.Destinations {
		if len(destination.Matches) != 0 {
			t.Fatalf("unconfigured path match = %#v", destination.Matches)
		}
	}

	capture := filepath.Join(t.TempDir(), "private.har")
	if err := os.WriteFile(capture, raw, 0600); err != nil {
		t.Fatal(err)
	}
	report, err := ReadHARReport(capture, "https://example.test", ruleFile(t, validRules))
	if err != nil {
		t.Fatal(err)
	}
	reportText := string(report)
	for _, wanted := range []string{"URL path segment", "not delivery to a server", "path substrings", "sending or receipt"} {
		if !strings.Contains(reportText, wanted) {
			t.Fatalf("report missing %q", wanted)
		}
	}
	for _, forbidden := range []string{email, account, "prefix-", "suffix", "example.test"} {
		if strings.Contains(reportText, forbidden) {
			t.Fatalf("report retained %q", forbidden)
		}
	}
}

func TestHARURLPathSegmentBoundsFailClosedWithAndWithoutRules(t *testing.T) {
	withinLimit := strings.Repeat("/", maxHARPathSegments-1) + "end"
	overLimit := strings.Repeat("/", maxHARPathSegments)
	malformed := "https://example.test/%ZZ"
	for _, rawURL := range []string{"https://example.test/" + withinLimit, "https://example.test/" + overLimit, malformed} {
		raw, err := json.Marshal(map[string]any{"log": map[string]any{"version": "1.2", "entries": []any{map[string]any{"request": map[string]any{"url": rawURL}}}}})
		if err != nil {
			t.Fatal(err)
		}
		for _, rules := range [][]harRule{nil, {{Category: "email", Value: "test-person@example.test"}}} {
			review, err := inspectHAR(raw, "https://example.test", rules)
			if rawURL == "https://example.test/"+withinLimit {
				if err != nil || review.Requests != 1 {
					t.Fatalf("within path bound rejected: %v %#v", err, review)
				}
				continue
			}
			if err == nil || strings.Contains(err.Error(), rawURL) || strings.Contains(err.Error(), "end") {
				t.Fatalf("unsafe path acceptance/error for %q: %v", rawURL, err)
			}
		}
	}
}
