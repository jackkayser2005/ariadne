package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func comparisonCapture(t *testing.T, urls ...string) string {
	t.Helper()
	entries := make([]any, 0, len(urls))
	for _, u := range urls {
		entries = append(entries, map[string]any{"request": map[string]any{"url": u}})
	}
	data, _ := json.Marshal(map[string]any{"log": map[string]any{"version": "1.2", "entries": entries}})
	path := filepath.Join(t.TempDir(), "private.har")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHARComparisonSharesOriginLabelsAndScopesReferences(t *testing.T) {
	rules := ruleFile(t, validRules)
	left := comparisonCapture(t, "https://alpha.test/?x=test-person%40example.test", "https://beta.test/?x=account-12345")
	right := comparisonCapture(t, "https://BETA.test:443/?x=test-person%40example.test", "https://alpha.test/?email=not-the-value", "https://gamma.test/?x=test-person%40example.test")
	review, err := CompareHARFiles(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	if review.Left.Requests != 2 || review.Right.Requests != 3 || !review.MatchingConfigured {
		t.Fatal(review)
	}
	leftLabels := []string{review.Left.Destinations[0].Label, review.Left.Destinations[1].Label}
	rightLabels := []string{review.Right.Destinations[0].Label, review.Right.Destinations[1].Label, review.Right.Destinations[2].Label}
	if !reflect.DeepEqual(leftLabels, []string{"Other origin 1", "Other origin 2"}) || !reflect.DeepEqual(rightLabels, []string{"Other origin 2", "Other origin 1", "Other origin 3"}) {
		t.Fatal(leftLabels, rightLabels)
	}
	if len(review.Right.Destinations[1].Matches) != 0 || !reflect.DeepEqual(review.Right.Destinations[0].Matches[0].Entries, []int{1}) {
		t.Fatal("cross-file matches conflated", review)
	}
	data, _ := json.Marshal(review)
	for _, secret := range []string{"alpha.test", "beta.test", "gamma.test", "test-person", "account-12345", left, right, rules} {
		if strings.Contains(string(data), secret) {
			t.Fatal("leak", secret)
		}
	}
}

func TestHARComparisonRequiresValidFilesAndRules(t *testing.T) {
	empty := comparisonCapture(t)
	rules := ruleFile(t, validRules)
	review, err := CompareHARFiles(empty, empty, "https://example.test", rules)
	if err != nil || review.Left.Requests != 0 || review.Right.Requests != 0 {
		t.Fatal(review, err)
	}
	bad := ruleFile(t, `{"private":"secret-invalid"}`)
	huge := ruleFile(t, strings.Repeat(" ", maxHARBytes+1))
	for _, tc := range []struct{ left, right, origin, rules string }{
		{empty, empty, "https://example.test", ""}, {empty, empty, "https://example.test", bad},
		{empty, empty, "https://example.test/private", rules}, {bad, empty, "https://example.test", rules},
		{empty, bad, "https://example.test", rules}, {huge, empty, "https://example.test", rules},
		{empty, filepath.Join(t.TempDir(), "secret-missing"), "https://example.test", rules},
	} {
		if _, err := CompareHARFiles(tc.left, tc.right, tc.origin, tc.rules); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), tc.left) {
			t.Fatal("unsafe acceptance/error", err)
		}
	}
}

func TestHARComparisonHTMLFreshnessRedactionAndExclusiveOutput(t *testing.T) {
	left := comparisonCapture(t, "https://example.test/?x=test-person%40example.test")
	right := comparisonCapture(t)
	rules := ruleFile(t, validRules)
	output := filepath.Join(t.TempDir(), "report.html")
	if err := SaveHARComparisonReport(left, right, "https://example.test", output, rules); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveHARComparisonReport(left, right, "https://example.test", output, rules); err == nil {
		t.Fatal("replaced output")
	}
	after, _ := os.ReadFile(output)
	if !reflect.DeepEqual(after, original) {
		t.Fatal("changed report")
	}
	local, err := ReadHARComparisonReport(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{"First capture", "Second capture", "Supporting entries", "entry 1", "no exported requests", "cannot select a minimum", "not observed in the checked parts of this file", "Request contents we could check", `<details class="origin-row"`} {
		if !strings.Contains(string(local), wanted) {
			t.Fatal("missing", wanted)
		}
	}
	for _, secret := range []string{"test-person", "example.test", left, right, rules} {
		if strings.Contains(string(local), secret) || strings.Contains(string(original), secret) {
			t.Fatal("leak", secret)
		}
	}
	if !strings.Contains(string(local), `href="/"`) || strings.Contains(string(original), `href="/"`) {
		t.Fatal("local navigation scope")
	}
	if err := os.WriteFile(right, []byte("secret-invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHARComparisonReport(left, right, "https://example.test", rules); err == nil {
		t.Fatal("stale comparison")
	}
	if err := SaveHARComparisonReport(left, right, "https://example.test", filepath.Join(t.TempDir(), "new.html"), rules); err == nil {
		t.Fatal("published invalid comparison")
	}
}

func TestHARComparisonSummaryDeduplicatesEntriesAcrossChannelsAndDestinations(t *testing.T) {
	comparison := HARComparison{
		Left: HARReview{Destinations: []HARDestination{
			{Label: "Website origin", Matches: []HARMatch{
				{Category: "email", Channel: "URL query", Entries: []int{1, 2}},
				{Category: "email", Channel: "JSON body string", Entries: []int{1}},
			}},
			{Label: "Other origin 1", Matches: []HARMatch{
				{Category: "email", Channel: "Exported form parameter", Entries: []int{2, 3}},
			}},
		}},
		Right: HARReview{},
	}
	summary := comparisonSummaryFor(comparison, map[string]int{"Website origin": 1, "Other origin 1": 2})
	if !summary.AnyMatches || len(summary.Categories) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	category := summary.Categories[0]
	if category.Title != "Email test value" || category.Sides[0].Requests != 3 || category.Sides[0].Destinations != 2 {
		t.Fatalf("category = %#v", category)
	}
	if !reflect.DeepEqual(category.Sides[0].Origins, []harComparisonSummaryOrigin{{Label: "Website origin", Row: 1}, {Label: "Other origin 1", Row: 2}}) {
		t.Fatalf("origins = %#v", category.Sides[0].Origins)
	}
	if category.Sides[1].Observed || category.Sides[1].Requests != 0 || category.Sides[1].Destinations != 0 {
		t.Fatalf("unmatched side = %#v", category.Sides[1])
	}
}

func TestHARComparisonSummaryKeepsReorderedLabelsLinkedToNumericRows(t *testing.T) {
	left := comparisonCapture(t, "https://alpha.test/?x=test-person%40example.test", "https://beta.test/?x=account-12345")
	right := comparisonCapture(t, "https://BETA.test:443/?x=test-person%40example.test", "https://alpha.test/?x=account-12345")
	rules := ruleFile(t, validRules)
	report, err := ReadHARComparisonReport(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, want := range []string{
		"Email test value",
		"Account identifier test value",
		`href="#origin-row-1">Other origin 1</a>`,
		`href="#origin-row-2">Other origin 2</a>`,
		`id="origin-row-1"`,
		`id="origin-row-2"`,
		`<details class="origin-row"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(text, "<details open") || strings.Contains(text, "alpha.test") || strings.Contains(text, "beta.test") {
		t.Fatal("unsafe or expanded detail state")
	}
}

func TestHARComparisonSummaryEmptyStateIsInconclusiveAndHintsStayWeak(t *testing.T) {
	left := comparisonCapture(t, "https://example.test/?email=not-the-value")
	right := comparisonCapture(t, "https://other.test/?account_id=not-the-value")
	rules := ruleFile(t, validRules)
	report, err := ReadHARComparisonReport(left, right, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, want := range []string{
		"No exact supplied test category matched in either file",
		"This is inconclusive",
		"does not show that information stayed private",
		"Field-name clues appear only in the collapsed origin details and do not count as matches",
		"Weaker field-name clues",
	} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want)
		}
	}
	if strings.Contains(text, "privacy success") || strings.Contains(text, "test-person") || strings.Contains(text, "account-12345") {
		t.Fatal("unsafe empty state", text)
	}
}
