package browser

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHARComparisonHeaderCookieFlowPreservesCountsAndReferences(t *testing.T) {
	root := t.TempDir()
	left, right := filepath.Join(root, "first.har"), filepath.Join(root, "second.har")
	for path, raw := range map[string]string{left: `{"log":{"version":"1.2","entries":[{"request":{"url":"https://example.test/?email=not-the-value","headers":[{"name":"X-Contact","value":"test-person@example.test"},{"name":"X-Contact-Repeat","value":"test-person@example.test"},{"name":"Cookie","value":"id=account-12345"},{"name":"X-Extra","value":"<script>private-header</script>"}],"cookies":[{"name":"session","value":"account-12345"}],"postData":{"mimeType":"application/json","text":"{\"contact\":\"test-person@example.test\"}"}}},{"request":{"url":"https://service.test/","headers":[{"name":"X-Account","value":"account-12345"}],"cookies":[{"name":"contact","value":"test-person@example.test"}]}}]}}`, right: `{"log":{"version":"1.2","entries":[{"request":{"url":"https://service.test/","headers":[{"name":"Authorization","value":"Bearer account-12345"}],"cookies":[{"name":"contact","value":"test-person%40example.test"}]}},{"request":{"url":"https://example.test/","headers":[{"name":"X-Account","value":"account-12345"}],"cookies":[{"name":"contact","value":"Test-person@example.test"}],"bodySize":48}}]}}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rules := ruleFile(t, validRules)
	comparison, err := CompareHARFiles(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Left.SchemaVersion != 4 || comparison.Right.SchemaVersion != 4 {
		t.Fatal("expanded channel vocabulary is not versioned")
	}
	actual := map[string][]int{}
	for _, destination := range comparison.Left.Destinations {
		for _, match := range destination.Matches {
			if match.Requests != len(match.Entries) {
				t.Fatal("count and references disagree")
			}
			actual[destination.Label+"|"+match.Category+"|"+match.Channel] = match.Entries
		}
	}
	expected := map[string][]int{
		"Website origin|email|Request header value":      {1},
		"Website origin|email|JSON body string":          {1},
		"Website origin|account-id|Request cookie value": {1},
		"Other origin 1|account-id|Request header value": {2},
		"Other origin 1|email|Request cookie value":      {2},
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("wrong channel references: %#v", actual)
	}
	summary := comparisonSummaryFor(comparison, map[string]int{"Website origin": 1, "Other origin 1": 2})
	if len(summary.Categories) != 2 {
		t.Fatal("lost a matched category", summary)
	}
	email, account := summary.Categories[0], summary.Categories[1]
	if email.Title != "Email test value" || email.Sides[0].Requests != 2 || email.Sides[0].Destinations != 2 || email.Sides[1].Observed {
		t.Fatal("email was double counted or transformed values matched", email)
	}
	if account.Title != "Account identifier test value" || account.Sides[0].Requests != 2 || account.Sides[1].Requests != 1 || account.Sides[1].Destinations != 1 {
		t.Fatal("account comparison conflated entries", account)
	}
	if len(comparison.Right.Destinations[0].Matches) != 0 || comparison.Right.UninspectedBodies != 1 {
		t.Fatal("unsupported values or missing bodies became positive evidence")
	}
	match := comparison.Right.Destinations[1].Matches
	if len(match) != 1 || match[0].Channel != "Request header value" || !reflect.DeepEqual(match[0].Entries, []int{2}) {
		t.Fatal("second-file reference drift", match)
	}
	report, err := ExportHARComparisonReport(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"Request header value", "Request cookie value", "not observed in the checked parts of this file", "entry 1", "entry 2"} {
		if !strings.Contains(string(report), wanted) {
			t.Fatal("missing explanation", wanted)
		}
	}
	for _, secret := range []string{"test-person@example.test", "account-12345", "example.test", "service.test", "X-Contact", "X-Account", "private-header", left, right, rules} {
		if strings.Contains(string(report), secret) {
			t.Fatal("report retained private source content")
		}
	}
}
