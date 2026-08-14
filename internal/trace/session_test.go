package trace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndVerifySession(t *testing.T) {
	tracePath := writeTrace(t, Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{{
			Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"},
		}},
	})
	sessionPath := filepath.Join(t.TempDir(), "nested", "session.json")
	input := SessionInput{
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("a", 64),
		Role:            RoleStandalone,
		Order:           OrderStandalone,
	}
	created, err := SaveSession(tracePath, sessionPath, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.SchemaVersion != 1 || created.Source != "browser" || created.Adapter != input.Adapter ||
		created.AdapterVersion != 1 || created.ProcedureSHA256 != input.ProcedureSHA256 ||
		created.Scope != "outbound" || created.Completeness != Complete || created.Role != RoleStandalone ||
		created.Order != OrderStandalone || created.PairSHA256 != "" ||
		!ValidSHA256(created.TraceSHA256) || !ValidSHA256(created.SessionSHA256) {
		t.Fatalf("created summary = %#v", created)
	}
	verified, err := VerifySession(sessionPath, tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if verified != created {
		t.Fatalf("verified summary = %#v, created = %#v", verified, created)
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	session, err := DecodeSession(data)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := SessionSHA256(session)
	if err != nil || identity != created.SessionSHA256 {
		t.Fatalf("session identity = %q, %v; want %q", identity, err, created.SessionSHA256)
	}
	if strings.Contains(string(data), "analytics") || strings.Contains(string(data), "region") {
		t.Fatalf("session exposed trace event data: %s", data)
	}
	if _, err := SaveSession(tracePath, filepath.Join(t.TempDir(), "paired.json"), SessionInput{Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Role: RoleBaseline, Order: OrderBaselineTreatment, PairSHA256: strings.Repeat("b", 64)}); err == nil || !strings.Contains(err.Error(), "pair create") {
		t.Fatalf("SaveSession() accepted paired creation: %v", err)
	}
}

func TestSaveStandaloneSession(t *testing.T) {
	tracePath := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "all", Completeness: Partial, Events: []Event{},
	})
	summary, err := SaveSession(tracePath, filepath.Join(t.TempDir(), "session.json"), SessionInput{
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("c", 64),
		Role:            RoleStandalone,
		Order:           OrderStandalone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Source != "browser" || summary.Role != RoleStandalone || summary.Order != OrderStandalone || summary.PairSHA256 != "" || summary.Completeness != Partial {
		t.Fatalf("standalone summary = %#v", summary)
	}
}

func TestSaveEmptyTracePreservesDeclaredCompleteness(t *testing.T) {
	for _, completeness := range []string{Complete, Partial} {
		t.Run(completeness, func(t *testing.T) {
			tracePath := writeTrace(t, Document{
				SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: completeness, Events: []Event{},
			})
			summary, err := SaveSession(tracePath, filepath.Join(t.TempDir(), "session.json"), SessionInput{
				Adapter: "android-experiment-001", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Role: RoleStandalone, Order: OrderStandalone,
			})
			if err != nil || summary.Source != "android" || summary.Completeness != completeness {
				t.Fatalf("SaveSession() = %#v, %v", summary, err)
			}
		})
	}
}

func TestVerifySessionPair(t *testing.T) {
	makePair := func(t *testing.T, sameTrace bool) (string, string, string, string) {
		t.Helper()
		root := t.TempDir()
		baselineTrace := writeTrace(t, Document{
			SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
			Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
		})
		treatmentTrace := writeTrace(t, Document{
			SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
			Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id", "region"}}},
		})
		if sameTrace {
			treatmentTrace = baselineTrace
		}
		baselineSessionPath := filepath.Join(root, "baseline-session.json")
		treatmentSessionPath := filepath.Join(root, "treatment-session.json")
		if sameTrace {
			for _, path := range []string{baselineSessionPath, treatmentSessionPath} {
				if _, err := SaveSession(baselineTrace, path, SessionInput{Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("c", 64), Role: RoleStandalone, Order: OrderStandalone}); err != nil {
					t.Fatal(err)
				}
			}
			for _, values := range []struct {
				path string
				role string
			}{
				{baselineSessionPath, RoleBaseline},
				{treatmentSessionPath, RoleTreatment},
			} {
				session, err := DecodeSession(mustRead(values.path))
				if err != nil {
					t.Fatal(err)
				}
				session.Role = values.role
				session.Order = OrderTreatmentBaseline
				session.PairSHA256 = strings.Repeat("b", 64)
				if err := os.WriteFile(values.path, mustJSON(session), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		} else if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSessionPath, treatmentSessionPath, SessionPairInput{
			Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("c", 64), Scope: "outbound", Order: OrderTreatmentBaseline,
		}); err != nil {
			t.Fatal(err)
		}
		return baselineSessionPath, baselineTrace, treatmentSessionPath, treatmentTrace
	}

	baselineSession, baselineTrace, treatmentSession, treatmentTrace := makePair(t, false)
	summary, err := VerifySessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || !ValidSHA256(summary.PairSHA256) || summary.Source != "browser" || summary.Adapter != "browser-redacted-audit" || summary.Order != OrderTreatmentBaseline || summary.BaselineCompleteness != Complete || summary.TreatmentCompleteness != Complete || !ValidSHA256(summary.BaselineTraceSHA256) || !ValidSHA256(summary.TreatmentTraceSHA256) || summary.BaselineTraceSHA256 == summary.TreatmentTraceSHA256 || !ValidSHA256(summary.BaselineSessionSHA256) || !ValidSHA256(summary.TreatmentSessionSHA256) {
		t.Fatalf("pair summary = %#v", summary)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Session)
	}{
		{"adapter", func(session *Session) { session.Adapter = "android-experiment-001" }},
		{"version", func(session *Session) { session.AdapterVersion = 2 }},
		{"source", func(session *Session) { session.Source = "android" }},
		{"procedure", func(session *Session) { session.ProcedureSHA256 = strings.Repeat("d", 64) }},
		{"scope", func(session *Session) { session.Scope = "storage" }},
		{"order", func(session *Session) { session.Order = OrderTreatmentBaseline }},
		{"pair", func(session *Session) { session.PairSHA256 = strings.Repeat("e", 64) }},
	} {
		t.Run("metadata-"+test.name, func(t *testing.T) {
			baseline := validSession()
			treatment := validSession()
			treatment.Role = RoleTreatment
			test.mutate(&treatment)
			if err := validateSessionPairMetadata(baseline, treatment); err == nil {
				t.Fatal("validateSessionPairMetadata() accepted mismatched metadata")
			}
		})
	}

	for _, test := range []struct {
		name   string
		mutate func(string, string) error
		want   string
	}{
		{"same-role", func(_, treatmentSession string) error {
			session, err := DecodeSession(mustRead(treatmentSession))
			if err != nil {
				return err
			}
			session.Role = RoleBaseline
			return os.WriteFile(treatmentSession, mustJSON(session), 0o600)
		}, "roles"},
		{"different-procedure", func(_, treatmentSession string) error {
			session, err := DecodeSession(mustRead(treatmentSession))
			if err != nil {
				return err
			}
			session.ProcedureSHA256 = strings.Repeat("d", 64)
			return os.WriteFile(treatmentSession, mustJSON(session), 0o600)
		}, "provenance"},
		{"different-order", func(_, treatmentSession string) error {
			session, err := DecodeSession(mustRead(treatmentSession))
			if err != nil {
				return err
			}
			session.Order = OrderBaselineTreatment
			return os.WriteFile(treatmentSession, mustJSON(session), 0o600)
		}, "provenance"},
		{"different-pair", func(_, treatmentSession string) error {
			session, err := DecodeSession(mustRead(treatmentSession))
			if err != nil {
				return err
			}
			session.PairSHA256 = strings.Repeat("e", 64)
			return os.WriteFile(treatmentSession, mustJSON(session), 0o600)
		}, "provenance"},
		{"incorrect-pair-identity", func(baselineSession, treatmentSession string) error {
			for _, path := range []string{baselineSession, treatmentSession} {
				session, err := DecodeSession(mustRead(path))
				if err != nil {
					return err
				}
				session.PairSHA256 = strings.Repeat("f", 64)
				if err := os.WriteFile(path, mustJSON(session), 0o600); err != nil {
					return err
				}
			}
			return nil
		}, "identity"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseSession, baseTrace, treatmentSession, treatmentTrace := makePair(t, false)
			if err := test.mutate(baseSession, treatmentSession); err != nil {
				t.Fatal(err)
			}
			_, err := VerifySessionPair(baseSession, baseTrace, treatmentSession, treatmentTrace)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifySessionPair() error = %v, want %q", err, test.want)
			}
		})
	}

	baseSession, baseTrace, treatmentSession, treatmentTrace := makePair(t, true)
	if _, err := VerifySessionPair(baseSession, baseTrace, treatmentSession, treatmentTrace); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("same-trace pair error = %v", err)
	}
	if _, err := VerifySessionPair("", baseTrace, treatmentSession, treatmentTrace); err == nil || !strings.Contains(err.Error(), "paths") {
		t.Fatalf("empty pair path error = %v", err)
	}
}

func TestSessionPairSHA256ValidatesAndCanonicalizes(t *testing.T) {
	input := SessionPairInput{
		Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Scope: "outbound", Order: OrderBaselineTreatment,
	}
	baselineTraceSHA256 := strings.Repeat("b", 64)
	treatmentTraceSHA256 := strings.Repeat("c", 64)
	first, err := SessionPairSHA256(baselineTraceSHA256, treatmentTraceSHA256, input)
	if err != nil || !ValidSHA256(first) {
		t.Fatalf("SessionPairSHA256() = %q, %v", first, err)
	}
	second, err := SessionPairSHA256(baselineTraceSHA256, treatmentTraceSHA256, input)
	if err != nil || first != second {
		t.Fatalf("pair identity is not canonical: %q, %q, %v", first, second, err)
	}
	changedOrder := input
	changedOrder.Order = OrderTreatmentBaseline
	third, err := SessionPairSHA256(baselineTraceSHA256, treatmentTraceSHA256, changedOrder)
	if err != nil || third == first {
		t.Fatalf("pair order did not affect identity: %q, %q, %v", first, third, err)
	}
	for _, test := range []struct {
		name  string
		input SessionPairInput
		base  string
		treat string
		want  string
	}{
		{"adapter", SessionPairInput{Adapter: "custom", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Scope: "outbound", Order: OrderBaselineTreatment}, baselineTraceSHA256, treatmentTraceSHA256, "adapter"},
		{"version", SessionPairInput{Adapter: input.Adapter, AdapterVersion: 0, ProcedureSHA256: input.ProcedureSHA256, Scope: input.Scope, Order: input.Order}, baselineTraceSHA256, treatmentTraceSHA256, "adapter_version"},
		{"procedure", SessionPairInput{Adapter: input.Adapter, AdapterVersion: 1, ProcedureSHA256: "bad", Scope: input.Scope, Order: input.Order}, baselineTraceSHA256, treatmentTraceSHA256, "procedure"},
		{"scope", SessionPairInput{Adapter: input.Adapter, AdapterVersion: 1, ProcedureSHA256: input.ProcedureSHA256, Scope: "https://source.example", Order: input.Order}, baselineTraceSHA256, treatmentTraceSHA256, "scope"},
		{"order", SessionPairInput{Adapter: input.Adapter, AdapterVersion: 1, ProcedureSHA256: input.ProcedureSHA256, Scope: input.Scope, Order: OrderStandalone}, baselineTraceSHA256, treatmentTraceSHA256, "order"},
		{"baseline-sha", input, "bad", treatmentTraceSHA256, "trace identity"},
		{"treatment-sha", input, baselineTraceSHA256, "bad", "trace identity"},
		{"same-trace", input, baselineTraceSHA256, baselineTraceSHA256, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := SessionPairSHA256(test.base, test.treat, test.input)
			if test.want == "" {
				if err != nil {
					t.Fatalf("SessionPairSHA256() rejected identical trace content: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SessionPairSHA256() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSaveSessionPairRejectsInvalidInputs(t *testing.T) {
	baselineTrace := writeTrace(t, validTraceForSource("browser"))
	treatmentTrace := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
		Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id"}}},
	})
	input := SessionPairInput{Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Scope: "outbound", Order: OrderBaselineTreatment}
	validPaths := func(t *testing.T) (string, string) {
		t.Helper()
		root := t.TempDir()
		return filepath.Join(root, "baseline-session.json"), filepath.Join(root, "treatment-session.json")
	}
	tests := []struct {
		name   string
		base   string
		treat  string
		mutate func(*SessionPairInput)
		want   string
	}{
		{"empty-baseline-trace", "", treatmentTrace, nil, "paths"},
		{"empty-treatment-trace", baselineTrace, "", nil, "paths"},
		{"missing-baseline-trace", filepath.Join(t.TempDir(), "missing.json"), treatmentTrace, nil, "baseline trace"},
		{"missing-treatment-trace", baselineTrace, filepath.Join(t.TempDir(), "missing.json"), nil, "treatment trace"},
		{"invalid-adapter", baselineTrace, treatmentTrace, func(input *SessionPairInput) { input.Adapter = "custom" }, "adapter"},
		{"invalid-version", baselineTrace, treatmentTrace, func(input *SessionPairInput) { input.AdapterVersion = 0 }, "adapter_version"},
		{"invalid-procedure", baselineTrace, treatmentTrace, func(input *SessionPairInput) { input.ProcedureSHA256 = "bad" }, "procedure"},
		{"input-scope", baselineTrace, treatmentTrace, func(input *SessionPairInput) { input.Scope = "storage" }, "scope"},
		{"invalid-order", baselineTrace, treatmentTrace, func(input *SessionPairInput) { input.Order = OrderStandalone }, "order"},
		{"scope-disagreement", baselineTrace, writeTrace(t, Document{SchemaVersion: 1, Redacted: true, Scope: "storage", Completeness: Complete, Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"account-id"}}}}), nil, "scopes"},
		{"treatment-source", baselineTrace, writeTrace(t, validTraceForSource("android")), nil, "treatment session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			localInput := input
			if test.mutate != nil {
				test.mutate(&localInput)
			}
			baselineSession, treatmentSession := validPaths(t)
			_, err := SaveSessionPair(test.base, test.treat, baselineSession, treatmentSession, localInput)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SaveSessionPair() error = %v, want %q", err, test.want)
			}
		})
	}

	baselineSession, treatmentSession := validPaths(t)
	if err := os.WriteFile(baselineSession, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, input); err == nil || !strings.Contains(err.Error(), "baseline session") {
		t.Fatalf("existing baseline output error = %v", err)
	}
	baselineSession, treatmentSession = validPaths(t)
	if err := os.WriteFile(treatmentSession, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, input); err == nil || !strings.Contains(err.Error(), "treatment session") {
		t.Fatalf("existing treatment output error = %v", err)
	}
	if _, err := os.Stat(baselineSession); !os.IsNotExist(err) {
		t.Fatalf("partial pair output was not removed: %v", err)
	}
	baselineSession, _ = validPaths(t)
	if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, baselineSession, input); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("same pair output path error = %v", err)
	}
	baselineSession, treatmentSession = validPaths(t)
	_, err := saveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, input, func(string, string, string, string) (SessionPairVerificationSummary, error) {
		return SessionPairVerificationSummary{}, errors.New("verification failed")
	})
	if err == nil || !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("post-write verification error = %v", err)
	}
	if _, err := os.Stat(baselineSession); !os.IsNotExist(err) {
		t.Fatalf("baseline output survived post-write verification failure: %v", err)
	}
	if _, err := os.Stat(treatmentSession); !os.IsNotExist(err) {
		t.Fatalf("treatment output survived post-write verification failure: %v", err)
	}
}

func TestSessionAdapters(t *testing.T) {
	for _, test := range []struct {
		adapter string
		source  string
	}{
		{"android-experiment-001", "android"},
		{"browser-redacted-audit", "browser"},
		{"browser-local-fixture", "browser"},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			tracePath := writeTrace(t, Document{
				SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
				Events: []Event{{Source: test.source, Channel: "network", Kind: "request", Destination: "unknown", Fields: []string{"unknown"}}},
			})
			summary, err := SaveSession(tracePath, filepath.Join(t.TempDir(), "session.json"), SessionInput{
				Adapter: test.adapter, AdapterVersion: 1, ProcedureSHA256: strings.Repeat("d", 64), Role: RoleStandalone, Order: OrderStandalone,
			})
			if err != nil || summary.Source != test.source {
				t.Fatalf("SaveSession() = %#v, %v", summary, err)
			}
		})
	}
}

func TestDecodeSessionRejectsInvalidDocuments(t *testing.T) {
	valid := mustJSON(validSession())
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "empty"},
		{"oversized", []byte(strings.Repeat("x", maxSessionBytes+1)), "65536"},
		{"invalid-utf8", append(append([]byte(nil), valid...), 0xff), "UTF-8"},
		{"non-object", []byte("[]"), "JSON object"},
		{"malformed", []byte("{"), "JSON structure"},
		{"duplicate", []byte(strings.Replace(string(valid), `"source":"browser"`, `"source":"browser","source":"browser"`, 1)), "JSON structure"},
		{"unknown", []byte(strings.Replace(string(valid), `"source":"browser"`, `"payload":"private-value","source":"browser"`, 1)), "JSON fields"},
		{"trailing", append(append([]byte(nil), valid...), []byte("{}")...), "trailing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeSession(test.data)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeSession() error = %v, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "private-value") {
				t.Fatalf("DecodeSession() exposed input value: %v", err)
			}
		})
	}
}

func TestDecodeSessionRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Session)
		want   string
	}{
		{"schema", func(session *Session) { session.SchemaVersion = 2 }, "schema_version"},
		{"trace-sha", func(session *Session) { session.TraceSHA256 = "bad" }, "trace_sha256"},
		{"adapter", func(session *Session) { session.Adapter = "custom-adapter" }, "adapter or source"},
		{"source", func(session *Session) { session.Source = "custom-source" }, "adapter or source"},
		{"version-low", func(session *Session) { session.AdapterVersion = 0 }, "adapter_version"},
		{"version-high", func(session *Session) { session.AdapterVersion = maxAdapterVersion + 1 }, "adapter_version"},
		{"procedure-sha", func(session *Session) { session.ProcedureSHA256 = "bad" }, "procedure_sha256"},
		{"scope", func(session *Session) { session.Scope = "https://source.example" }, "scope"},
		{"completeness", func(session *Session) { session.Completeness = "unknown" }, "completeness"},
		{"role", func(session *Session) { session.Role = "custom" }, "role"},
		{"order", func(session *Session) { session.Order = "custom" }, "order"},
		{"standalone-order", func(session *Session) {
			session.Role = RoleStandalone
			session.Order = OrderBaselineTreatment
			session.PairSHA256 = ""
		}, "standalone session"},
		{"standalone-pair", func(session *Session) {
			session.Role = RoleStandalone
			session.Order = OrderStandalone
			session.PairSHA256 = strings.Repeat("e", 64)
		}, "standalone session"},
		{"paired-order", func(session *Session) {
			session.Role = RoleBaseline
			session.Order = OrderStandalone
			session.PairSHA256 = strings.Repeat("e", 64)
		}, "paired session"},
		{"paired-sha", func(session *Session) { session.PairSHA256 = "bad" }, "paired session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session := validSession()
			test.mutate(&session)
			_, err := DecodeSession(mustJSON(session))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeSession() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestSessionBindingRejectsMismatches(t *testing.T) {
	tracePath := writeTrace(t, Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
		Events: []Event{{Source: "browser", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
	})
	sessionPath := filepath.Join(t.TempDir(), "session.json")
	_, err := SaveSession(tracePath, sessionPath, SessionInput{
		Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("f", 64), Role: RoleStandalone, Order: OrderStandalone,
	})
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Session)
		want   string
	}{
		{"trace", func(session *Session) { session.TraceSHA256 = strings.Repeat("1", 64) }, "trace identity"},
		{"scope", func(session *Session) { session.Scope = "storage" }, "scope or completeness"},
		{"completeness", func(session *Session) { session.Completeness = Partial }, "scope or completeness"},
		{"source", func(session *Session) { session.Source = "desktop" }, "adapter or source"},
	} {
		t.Run(test.name, func(t *testing.T) {
			session, err := DecodeSession(original)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&session)
			data := mustJSON(session)
			if err := os.WriteFile(sessionPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = VerifySession(sessionPath, tracePath)
			if err == nil {
				t.Fatal("VerifySession() accepted mismatched binding")
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("VerifySession() error = %v, want %q", err, test.want)
			}
		})
	}
	otherTrace := writeTrace(t, validTraceForSource("android"))
	freshSession := filepath.Join(t.TempDir(), "session.json")
	if _, err := SaveSession(tracePath, freshSession, SessionInput{Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Role: RoleStandalone, Order: OrderStandalone}); err != nil {
		t.Fatal(err)
	}
	otherSummary, err := Verify(otherTrace)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := DecodeSession(mustRead(freshSession))
	if err != nil {
		t.Fatal(err)
	}
	fresh.TraceSHA256 = otherSummary.TraceSHA256
	if err := os.WriteFile(freshSession, mustJSON(fresh), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySession(freshSession, otherTrace); err == nil || !strings.Contains(err.Error(), "trace events") {
		t.Fatalf("source mismatch error = %v", err)
	}
}

func TestSessionPathAndWriteErrors(t *testing.T) {
	tracePath := writeTrace(t, validTraceForSource("browser"))
	input := SessionInput{Adapter: "browser-redacted-audit", AdapterVersion: 1, ProcedureSHA256: strings.Repeat("a", 64), Role: RoleStandalone, Order: OrderStandalone}
	for _, test := range []struct {
		name        string
		tracePath   string
		sessionPath string
		want        string
	}{
		{"missing-trace", filepath.Join(t.TempDir(), "missing.json"), filepath.Join(t.TempDir(), "session.json"), "trace"},
		{"empty-trace-path", "", filepath.Join(t.TempDir(), "session.json"), "paths"},
		{"empty-session-path", tracePath, "", "paths"},
		{"existing-output", tracePath, func() string {
			path := filepath.Join(t.TempDir(), "existing.json")
			_ = os.WriteFile(path, []byte("existing"), 0o600)
			return path
		}(), "create output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := SaveSession(test.tracePath, test.sessionPath, input)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("SaveSession() error = %v, want %q", err, test.want)
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "missing-session.json")
	if _, err := VerifySession(missing, tracePath); err == nil || !strings.Contains(err.Error(), "read input") {
		t.Fatalf("VerifySession() missing session error = %v", err)
	}
	if _, err := VerifySession("", tracePath); err == nil || !strings.Contains(err.Error(), "paths") {
		t.Fatalf("VerifySession() empty session path error = %v", err)
	}
}

func TestSessionSHA256RejectsInvalidSession(t *testing.T) {
	if _, err := SessionSHA256(Session{}); err == nil {
		t.Fatal("SessionSHA256() accepted invalid session")
	}
}

func validSession() Session {
	return Session{
		SchemaVersion:   sessionSchemaVersion,
		TraceSHA256:     strings.Repeat("1", 64),
		Source:          "browser",
		Adapter:         "browser-redacted-audit",
		AdapterVersion:  1,
		ProcedureSHA256: strings.Repeat("2", 64),
		Scope:           "outbound",
		Completeness:    Complete,
		Role:            RoleBaseline,
		Order:           OrderBaselineTreatment,
		PairSHA256:      strings.Repeat("3", 64),
	}
}

func validTraceForSource(source string) Document {
	return Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{{
			Source: source, Channel: "network", Kind: "request", Destination: "unknown", Fields: []string{"unknown"},
		}},
	}
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}
