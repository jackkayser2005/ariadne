package browser

import (
	"reflect"
	"strings"
	"testing"
)

func TestInformationOverviewDeduplicatesChannelsAndKeepsMissingValuesUnknown(t *testing.T) {
	rules := []harRule{{Category: "email", Value: "test-person@example.test"}, {Category: "phone", Value: "15555550123"}}
	raw := []byte(`{"log":{"version":"1.2","entries":[{"request":{"url":"https://example.test/?x=test-person%40example.test","postData":{"params":[{"name":"contact","value":"test-person@example.test"}]}}},{"request":{"url":"https://other.test/?x=test-person%40example.test&phone=not-the-supplied-value"}}]}}`)
	review, err := inspectHAR(raw, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	overview := informationOverview(review, rules)
	if len(overview) != 2 || overview[0].Requests != 2 || len(overview[0].Destinations) != 2 {
		t.Fatal(overview)
	}
	if !reflect.DeepEqual(overview[0].Destinations[0].Entries, []int{1}) || overview[0].Destinations[1].Index != 2 {
		t.Fatal("incorrect trail reference", overview)
	}
	if overview[1].Requests != 0 || len(overview[1].Destinations) != 0 {
		t.Fatal("field name became disclosure", overview[1])
	}
	if got := informationOverview(review, nil); len(got) != 0 {
		t.Fatal("unconfigured flow", got)
	}
}

func TestInformationTrailsAreRedactedAndDetailsStartCollapsed(t *testing.T) {
	input := comparisonCapture(t, "https://example.test/?x=test-person%40example.test")
	rules := ruleFile(t, validRules)
	report, err := ReadHARReport(input, "https://example.test", rules)
	if err != nil {
		t.Fatal(err)
	}
	text := string(report)
	for _, want := range []string{"Follow the information", "Email test value", "Found in 1 distinct", `href="#destination-1"`, `id="destination-1"`, "Account identifier test value", "No exact match in the supported channels", "Explore request details"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing", want)
		}
	}
	for _, forbidden := range []string{"test-person@example.test", "account-12345", "<details open", input} {
		if strings.Contains(text, forbidden) {
			t.Fatal("unexpected", forbidden)
		}
	}
}
