package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareSessionPairBindsStructuralResult(t *testing.T) {
	root := t.TempDir()
	baselineTrace := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
		Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
	})
	treatmentTrace := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
		Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id", "region"}}},
	})
	baselineSession := filepath.Join(root, "baseline-session.json")
	treatmentSession := filepath.Join(root, "treatment-session.json")
	created, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, SessionPairInput{
		Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Scope: "outbound", Order: OrderBaselineTreatment,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := CompareSessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != 1 || result.Pair != created || result.Comparison.Scope != "outbound" ||
		result.Comparison.BaselineTraceSHA256 != created.BaselineTraceSHA256 ||
		result.Comparison.TreatmentTraceSHA256 != created.TreatmentTraceSHA256 ||
		len(result.Comparison.Unchanged) != 0 || len(result.Comparison.Differences) != 1 || len(result.Comparison.Unknowns) != 0 ||
		result.Comparison.Differences[0].State != "observed" {
		t.Fatalf("CompareSessionPair() = %#v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-value") || strings.Contains(string(data), "https://") {
		t.Fatalf("comparison exposed unsafe source details: %s", data)
	}
}

func TestCompareSessionPairPreservesUnknownCoverage(t *testing.T) {
	root := t.TempDir()
	baselineTrace := writeTrace(t, validTraceForSource("browser"))
	treatmentTrace := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Partial,
		Events: []Event{},
	})
	baselineSession := filepath.Join(root, "baseline-session.json")
	treatmentSession := filepath.Join(root, "treatment-session.json")
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, SessionPairInput{
		Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("b", 64), Scope: "outbound", Order: OrderTreatmentBaseline,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := CompareSessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Comparison.Differences) != 0 || len(result.Comparison.Unknowns) != 1 || result.Comparison.Unknowns[0].State != "unknown" || result.Pair.TreatmentCompleteness != Partial {
		t.Fatalf("partial comparison = %#v", result)
	}
}

func TestCompareSessionPairRejectsInvalidInputs(t *testing.T) {
	root := t.TempDir()
	baselineTrace := writeTrace(t, validTraceForSource("browser"))
	treatmentTrace := writeTrace(t, validTraceForSource("browser"))
	baselineSession := filepath.Join(root, "baseline-session.json")
	treatmentSession := filepath.Join(root, "treatment-session.json")
	if _, err := CompareSessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace); err == nil || !strings.Contains(err.Error(), "baseline session") {
		t.Fatalf("missing pair error = %v", err)
	}
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, SessionPairInput{
		Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("c", 64), Scope: "outbound", Order: OrderBaselineTreatment,
	}); err == nil {
		t.Fatal("SaveSessionPair() accepted identical traces")
	}
	if err := os.WriteFile(baselineSession, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareSessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace); err == nil || !strings.Contains(err.Error(), "baseline session") {
		t.Fatalf("invalid pair error = %v", err)
	}
}

func TestCompareSessionPairRejectsSessionMutation(t *testing.T) {
	pair := SessionPairVerificationSummary{
		SchemaVersion: 1, PairSHA256: strings.Repeat("a", 64), Source: "browser", Adapter: "browser-redacted-audit", AdapterVersion: 1,
		ProcedureSHA256: strings.Repeat("b", 64), Scope: "outbound", Order: OrderBaselineTreatment,
		BaselineTraceSHA256: strings.Repeat("c", 64), TreatmentTraceSHA256: strings.Repeat("d", 64),
		BaselineCompleteness: Complete, TreatmentCompleteness: Complete,
		BaselineSessionSHA256: strings.Repeat("e", 64), TreatmentSessionSHA256: strings.Repeat("f", 64),
	}
	changed := pair
	changed.TreatmentSessionSHA256 = strings.Repeat("1", 64)
	verificationCalls := 0
	verify := func(string, string, string, string) (SessionPairVerificationSummary, error) {
		verificationCalls++
		if verificationCalls == 1 {
			return pair, nil
		}
		return changed, nil
	}
	compare := func(string, string) (Comparison, error) {
		return Comparison{
			SchemaVersion: 1, Scope: pair.Scope,
			BaselineTraceSHA256:  pair.BaselineTraceSHA256,
			TreatmentTraceSHA256: pair.TreatmentTraceSHA256,
			Unchanged:            []Event{}, Differences: []EventChange{}, Unknowns: []Unknown{},
		}, nil
	}
	_, err := compareSessionPair("baseline-session", "baseline-trace", "treatment-session", "treatment-trace", verify, compare)
	if err == nil || !strings.Contains(err.Error(), "session pair identities changed") || verificationCalls != 2 {
		t.Fatalf("mutation guard error = %v, verification calls = %d", err, verificationCalls)
	}
}
