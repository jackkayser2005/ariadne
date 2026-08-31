package trace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestTraceArchiveQuestionsAndVerification(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("a", 64)
	first := writeStandaloneArchiveInput(t, root, "first", validArchiveTrace("region"), procedure)
	second := writeStandaloneArchiveInput(t, root, "second", validArchiveTrace("region", "session-id"), procedure)
	partial := validArchiveTrace("region", "session-id")
	partial.Completeness = Partial
	third := writeStandaloneArchiveInput(t, root, "third", partial, procedure)
	archivePath := filepath.Join(root, "archive.json")

	saved, err := SaveArchive([]ArchiveInput{first, second, third}, archivePath)
	if err != nil {
		t.Fatalf("SaveArchive() error = %v", err)
	}
	if saved.Entries != 3 || saved.Complete != 2 || saved.Partial != 1 || saved.OrderBasis != "caller" || len(saved.Sources) != 1 {
		t.Fatalf("saved summary = %#v", saved)
	}

	archive, verified, err := ReadArchive(archivePath)
	if err != nil {
		t.Fatalf("ReadArchive() error = %v", err)
	}
	if verified.ArchiveSHA256 != saved.ArchiveSHA256 || len(archive.Entries) != 3 || archive.Entries[0].Position != 1 {
		t.Fatalf("verified archive = %#v, summary = %#v", archive, verified)
	}

	coverage, err := AskArchive(archivePath, ArchiveQuestionCoverage)
	if err != nil {
		t.Fatalf("coverage question error = %v", err)
	}
	if coverage.Result != archiveResultUnknown || coverage.EvidenceState != evidence.Unknown || coverage.Unknown != 1 {
		t.Fatalf("coverage answer = %#v", coverage)
	}
	change, err := AskArchive(archivePath, ArchiveQuestionChange)
	if err != nil {
		t.Fatalf("change question error = %v", err)
	}
	if change.Result != archiveResultUnknown || change.EvidenceState != evidence.Unknown || change.Compared != 2 || change.Unknown != 1 {
		t.Fatalf("change answer = %#v", change)
	}
	sources, err := AskArchive(archivePath, ArchiveQuestionSources)
	if err != nil {
		t.Fatalf("sources question error = %v", err)
	}
	if sources.Result != archiveResultAvailable || sources.EvidenceState != evidence.Observed || len(sources.Sources) != 1 || sources.Sources[0].Entries != 3 {
		t.Fatalf("sources answer = %#v", sources)
	}
	answers, err := AskAllArchive(archivePath)
	if err != nil || len(answers) != len(ArchiveQuestions()) {
		t.Fatalf("AskAllArchive() = %#v, error = %v", answers, err)
	}
	if got := answers[0].ArchiveSHA256; got != saved.ArchiveSHA256 {
		t.Fatalf("answer archive identity = %q, want %q", got, saved.ArchiveSHA256)
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-value") || strings.Contains(string(data), "https://") || strings.Contains(string(data), "payload") {
		t.Fatalf("archive exposed unsafe source data: %s", data)
	}
}

func TestSaveSourceAdapterArchivePreservesRunReceiptsAndFlowsThroughCase(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 2)
	driverPath := sourceAdapterTestDriver(t)
	firstDir := filepath.Join(t.TempDir(), "first-run")
	secondDir := filepath.Join(t.TempDir(), "second-run")
	first, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), firstDir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), secondDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReceiptSHA256 == second.ReceiptSHA256 {
		t.Fatal("source adapter runs unexpectedly share a receipt identity")
	}

	archivePath := filepath.Join(t.TempDir(), "archive.json")
	saved, err := SaveSourceAdapterArchive([]string{firstDir, secondDir}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SchemaVersion != archiveSchemaVersion || saved.Entries != 2 || saved.Complete != 2 {
		t.Fatalf("saved summary = %#v", saved)
	}
	archive, verified, err := ReadArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if verified.ArchiveSHA256 != saved.ArchiveSHA256 || len(archive.Entries) != 2 {
		t.Fatalf("verified archive = %#v, summary = %#v", archive, verified)
	}
	for index, wantReceipt := range []string{first.ReceiptSHA256, second.ReceiptSHA256} {
		binding := archive.Entries[index].AdapterRun
		if binding == nil || binding.ReceiptSHA256 != wantReceipt || binding.Receipt.TraceSHA256 != archive.Entries[index].Session.TraceSHA256 {
			t.Fatalf("archive entry %d binding = %#v", index+1, binding)
		}
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{driverPath, "\"challenge\":\"", "\"procedure\":\""} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("receipt-bound archive exposed transient input %q: %s", forbidden, data)
		}
	}
	if !strings.Contains(serialized, "\"adapter_run\"") {
		t.Fatalf("receipt-bound archive omitted adapter provenance: %s", data)
	}

	roundPath := filepath.Join(t.TempDir(), "round.json")
	if _, err := SaveArchiveQuestionRound(archivePath, roundPath); err != nil {
		t.Fatal(err)
	}
	casePath := filepath.Join(t.TempDir(), "case.json")
	caseSummary, err := SaveCase([]CaseInput{{
		Kind:              CaseEntryTraceArchive,
		ArtifactPath:      archivePath,
		QuestionRoundPath: roundPath,
	}}, casePath)
	if err != nil {
		t.Fatal(err)
	}
	if caseSummary.Entries != 1 || caseSummary.Archives != 1 {
		t.Fatalf("case summary = %#v", caseSummary)
	}
	if _, err := VerifyCase(casePath); err != nil {
		t.Fatal(err)
	}
}

func TestSaveSourceAdapterArchiveRejectsBindingTamperingAndDuplicateRuns(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 1)
	driverPath := sourceAdapterTestDriver(t)
	runDir := filepath.Join(t.TempDir(), "run")
	if _, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), runDir); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "archive.json")
	if _, err := SaveSourceAdapterArchive([]string{runDir}, archivePath); err != nil {
		t.Fatal(err)
	}
	archive, _, err := ReadArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name  string
		apply func(*Archive)
	}{
		{name: "legacy schema", apply: func(value *Archive) { value.SchemaVersion = archiveLegacySchemaVersion }},
		{name: "receipt identity", apply: func(value *Archive) {
			binding := *value.Entries[0].AdapterRun
			binding.ReceiptSHA256 = strings.Repeat("f", 64)
			value.Entries[0].AdapterRun = &binding
		}},
		{name: "receipt trace binding", apply: func(value *Archive) {
			binding := *value.Entries[0].AdapterRun
			binding.Receipt.TraceSHA256 = strings.Repeat("e", 64)
			value.Entries[0].AdapterRun = &binding
		}},
		{name: "duplicate receipt", apply: func(value *Archive) {
			second := value.Entries[0]
			second.Position = 2
			value.Entries = []ArchiveEntry{value.Entries[0], second}
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := archive
			mutated.Entries = append([]ArchiveEntry(nil), archive.Entries...)
			mutation.apply(&mutated)
			data, err := json.Marshal(mutated)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeArchive(data); err == nil {
				t.Fatal("DecodeArchive() accepted tampered adapter archive")
			}
		})
	}
	if _, err := SaveSourceAdapterArchive([]string{runDir, runDir}, filepath.Join(t.TempDir(), "duplicate.json")); err == nil {
		t.Fatal("SaveSourceAdapterArchive() accepted duplicate run receipts")
	}
	if err := os.WriteFile(filepath.Join(runDir, "extra.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSourceAdapterArchive([]string{runDir}, filepath.Join(t.TempDir(), "extra-archive.json")); err == nil {
		t.Fatal("SaveSourceAdapterArchive() accepted an extra run artifact")
	}
}
func TestTraceArchiveChangeResults(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("b", 64)
	base := validArchiveTrace("region")
	changed := validArchiveTrace("region", "session-id")
	third := validArchiveTrace("region", "session-id", "device-id")

	tests := []struct {
		name        string
		traces      []Document
		wantResult  string
		wantState   evidence.State
		wantChange  int
		wantSame    int
		wantUnknown int
	}{
		{name: "changed", traces: []Document{base, changed}, wantResult: archiveResultChanged, wantState: evidence.Observed, wantChange: 1},
		{name: "same", traces: []Document{base, base}, wantResult: archiveResultSame, wantState: evidence.Observed, wantSame: 1},
		{name: "mixed", traces: []Document{base, changed, changed}, wantResult: archiveResultMixed, wantState: evidence.Observed, wantChange: 1, wantSame: 1},
		{name: "incompatible scope", traces: []Document{base, func() Document { value := third; value.Scope = "storage"; return value }()}, wantResult: archiveResultUnknown, wantState: evidence.Unknown, wantUnknown: 1},
		{name: "one entry", traces: []Document{base}, wantResult: archiveResultUnknown, wantState: evidence.Unknown, wantUnknown: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputs := make([]ArchiveInput, 0, len(test.traces))
			for index, document := range test.traces {
				inputs = append(inputs, writeStandaloneArchiveInput(t, root, test.name+string(rune('a'+index)), document, procedure))
			}
			path := filepath.Join(t.TempDir(), "archive.json")
			if _, err := SaveArchive(inputs, path); err != nil {
				t.Fatalf("SaveArchive() error = %v", err)
			}
			answer, err := AskArchive(path, ArchiveQuestionChange)
			if err != nil {
				t.Fatalf("AskArchive() error = %v", err)
			}
			if answer.Result != test.wantResult || answer.EvidenceState != test.wantState || answer.Changed != test.wantChange || answer.Same != test.wantSame || answer.Unknown != test.wantUnknown {
				t.Fatalf("answer = %#v", answer)
			}
		})
	}
}

func TestTraceArchiveRejectsTamperingAndInvalidQuestions(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("c", 64)
	input := writeStandaloneArchiveInput(t, root, "input", validArchiveTrace("region"), procedure)
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	archive, summary, err := ReadArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerArchive(archive, strings.Repeat("d", 64), ArchiveQuestionSources); err == nil {
		t.Fatal("AnswerArchive() accepted a mismatched archive identity")
	}
	if _, err := AskArchive(archivePath, "free-form"); err == nil {
		t.Fatal("AskArchive() accepted an arbitrary question")
	}
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err == nil {
		t.Fatal("SaveArchive() overwrote an existing archive")
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), archive.Entries[0].Session.TraceSHA256, strings.Repeat("e", 64), 1)
	if err := os.WriteFile(archivePath, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyArchive(archivePath); err == nil {
		t.Fatal("VerifyArchive() accepted a tampered trace identity")
	}
	if summary.ArchiveSHA256 == "" {
		t.Fatal("archive summary did not include an identity")
	}
}

func TestTraceArchiveRejectsOversizedEntryCount(t *testing.T) {
	root := t.TempDir()
	input := writeStandaloneArchiveInput(t, root, "input", validArchiveTrace("region"), strings.Repeat("f", 64))
	inputs := make([]ArchiveInput, maxArchiveEntries+1)
	for index := range inputs {
		inputs[index] = input
	}
	if _, err := SaveArchive(inputs, filepath.Join(root, "archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted too many entries")
	}
}

func TestTraceArchiveRejectsMalformedDocumentsAndPaths(t *testing.T) {
	root := t.TempDir()
	input := writeStandaloneArchiveInput(t, root, "input", validArchiveTrace("region"), strings.Repeat("1", 64))
	archivePath := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{input}, archivePath); err != nil {
		t.Fatal(err)
	}
	archive, _, err := ReadArchive(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "oversized", data: bytes.Repeat([]byte("x"), maxArchiveBytes+1)},
		{name: "duplicate", data: []byte(`{"schema_version":1,"schema_version":1}`)},
		{name: "unknown field", data: []byte(`{"schema_version":1,"order_basis":"caller","entries":[],"extra":1}`)},
		{name: "trailing", data: append(append([]byte(nil), valid...), []byte("{}")...)},
		{name: "array", data: []byte(`[]`)},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if _, err := DecodeArchive(mutation.data); err == nil {
				t.Fatal("DecodeArchive() accepted malformed input")
			}
		})
	}
	for _, mutation := range []struct {
		name  string
		apply func(*Archive)
	}{
		{name: "schema", apply: func(value *Archive) { value.SchemaVersion = 3 }},
		{name: "order basis", apply: func(value *Archive) { value.OrderBasis = "chronological" }},
		{name: "position", apply: func(value *Archive) { value.Entries[0].Position = 2 }},
		{name: "session scope", apply: func(value *Archive) { value.Entries[0].Session.Scope = "storage" }},
		{name: "trace redaction", apply: func(value *Archive) { value.Entries[0].Trace.Redacted = false }},
		{name: "paired entry", apply: func(value *Archive) { value.Entries[0].Session.Role = RoleBaseline }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			mutated := archive
			mutated.Entries = append([]ArchiveEntry(nil), archive.Entries...)
			mutated.Entries[0].Session = archive.Entries[0].Session
			mutated.Entries[0].Trace = archive.Entries[0].Trace
			mutation.apply(&mutated)
			data, marshalErr := json.Marshal(mutated)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, decodeErr := DecodeArchive(data); decodeErr == nil {
				t.Fatal("DecodeArchive() accepted invalid archive metadata")
			}
		})
	}
	if _, err := ArchiveSHA256(Archive{}); err == nil {
		t.Fatal("ArchiveSHA256() accepted an invalid archive")
	}
	if _, err := AnswerArchive(archive, "bad", ArchiveQuestionSources); err == nil {
		t.Fatal("AnswerArchive() accepted an invalid identity")
	}
	if _, _, err := ReadArchive(""); err == nil {
		t.Fatal("ReadArchive() accepted an empty path")
	}
	if _, _, err := ReadArchive(filepath.Join(root, "missing.json")); err == nil {
		t.Fatal("ReadArchive() accepted a missing path")
	}
	oversizedPath := filepath.Join(root, "oversized.json")
	if err := os.WriteFile(oversizedPath, bytes.Repeat([]byte("x"), maxArchiveBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadArchive(oversizedPath); err == nil {
		t.Fatal("ReadArchive() accepted an oversized file")
	}
	if _, err := SaveArchive(nil, filepath.Join(root, "empty.json")); err == nil {
		t.Fatal("SaveArchive() accepted no entries")
	}
	if _, err := SaveArchive([]ArchiveInput{input}, ""); err == nil {
		t.Fatal("SaveArchive() accepted an empty output")
	}
	if _, err := SaveArchive([]ArchiveInput{{TracePath: filepath.Join(root, "missing-trace.json"), SessionPath: input.SessionPath}}, filepath.Join(root, "missing-input.json")); err == nil {
		t.Fatal("SaveArchive() accepted a missing trace")
	}
	if _, err := SaveArchive([]ArchiveInput{input}, filepath.Join(archivePath, "child.json")); err == nil {
		t.Fatal("SaveArchive() accepted a file as an output directory")
	}
}

func TestTraceArchiveSummarizesMultipleReviewedSources(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("2", 64)
	android := writeStandaloneArchiveInput(t, root, "android", validArchiveTrace("region"), procedure)
	browserDocument := validArchiveTrace("region")
	browserDocument.Events[0].Source = "browser"
	browserAudit := writeStandaloneArchiveInputWithAdapter(t, root, "browser-audit", browserDocument, "browser-redacted-audit", procedure)
	browserFixture := writeStandaloneArchiveInputWithAdapter(t, root, "browser-fixture", browserDocument, "browser-local-fixture", procedure)
	path := filepath.Join(root, "archive.json")
	if _, err := SaveArchive([]ArchiveInput{browserAudit, browserFixture, android}, path); err != nil {
		t.Fatal(err)
	}
	answer, err := AskArchive(path, ArchiveQuestionSources)
	if err != nil {
		t.Fatal(err)
	}
	if len(answer.Sources) != 3 || answer.Sources[0].Source != "android" || answer.Sources[1].Adapter != "browser-local-fixture" || answer.Sources[2].Adapter != "browser-redacted-audit" {
		t.Fatalf("source summaries = %#v", answer.Sources)
	}
	coverage, err := AskArchive(path, ArchiveQuestionCoverage)
	if err != nil || coverage.Result != archiveResultComplete || coverage.EvidenceState != evidence.Observed {
		t.Fatalf("complete coverage answer = %#v, error = %v", coverage, err)
	}
}

func TestTraceArchiveAdditionalFailurePaths(t *testing.T) {
	root := t.TempDir()
	procedure := strings.Repeat("3", 64)
	input := writeStandaloneArchiveInput(t, root, "input", validArchiveTrace("region"), procedure)
	if _, err := SaveArchive([]ArchiveInput{{TracePath: input.TracePath, SessionPath: filepath.Join(root, "missing-session.json")}}, filepath.Join(root, "missing-session-archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted a missing session")
	}
	invalidSession := filepath.Join(root, "invalid-session.json")
	if err := os.WriteFile(invalidSession, []byte(`{"not":"a session"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchive([]ArchiveInput{{TracePath: input.TracePath, SessionPath: invalidSession}}, filepath.Join(root, "invalid-session-archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted an invalid session")
	}
	invalidTrace := filepath.Join(root, "invalid-trace.json")
	if err := os.WriteFile(invalidTrace, []byte(`{"not":"a trace"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchive([]ArchiveInput{{TracePath: invalidTrace, SessionPath: input.SessionPath}}, filepath.Join(root, "invalid-trace-archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted an invalid trace")
	}
	validSessionData, err := os.ReadFile(input.SessionPath)
	if err != nil {
		t.Fatal(err)
	}
	mismatchedSession := filepath.Join(root, "mismatched-session.json")
	mismatchedSessionData := bytes.Replace(validSessionData, []byte(`"scope":"outbound"`), []byte(`"scope":"storage"`), 1)
	if err := os.WriteFile(mismatchedSession, mismatchedSessionData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchive([]ArchiveInput{{TracePath: input.TracePath, SessionPath: mismatchedSession}}, filepath.Join(root, "mismatched-session-archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted a session that did not bind to its trace")
	}
	pairedTreatment := writeStandaloneArchiveInput(t, root, "paired-treatment", validArchiveTrace("region", "session-id"), procedure)
	baselineSession := filepath.Join(root, "paired-baseline-session.json")
	treatmentSession := filepath.Join(root, "paired-treatment-session-pair.json")
	if _, err := SaveSessionPair(input.TracePath, pairedTreatment.TracePath, baselineSession, treatmentSession, SessionPairInput{
		Adapter:         "android-experiment-001",
		AdapterVersion:  1,
		ProcedureSHA256: procedure,
		Scope:           "outbound",
		Order:           OrderBaselineTreatment,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveArchive([]ArchiveInput{{TracePath: input.TracePath, SessionPath: baselineSession}}, filepath.Join(root, "paired-archive.json")); err == nil {
		t.Fatal("SaveArchive() accepted a paired session")
	}
	if _, err := marshalArchive(Archive{}); err == nil {
		t.Fatal("marshalArchive() accepted an invalid archive")
	}
	if _, err := AnswerArchive(Archive{}, strings.Repeat("4", 64), ArchiveQuestionSources); err == nil {
		t.Fatal("AnswerArchive() accepted an invalid archive")
	}
	missing := filepath.Join(root, "missing-archive.json")
	if _, err := AskArchive(missing, ArchiveQuestionSources); err == nil {
		t.Fatal("AskArchive() accepted a missing archive")
	}
	if _, err := AskAllArchive(missing); err == nil {
		t.Fatal("AskAllArchive() accepted a missing archive")
	}
}

func writeStandaloneArchiveInput(t *testing.T, root, name string, document Document, procedure string) ArchiveInput {
	return writeStandaloneArchiveInputWithAdapter(t, root, name, document, "android-experiment-001", procedure)
}

func writeStandaloneArchiveInputWithAdapter(t *testing.T, root, name string, document Document, adapter, procedure string) ArchiveInput {
	t.Helper()
	tracePath := filepath.Join(root, name+"-trace.json")
	sessionPath := filepath.Join(root, name+"-session.json")
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveSession(tracePath, sessionPath, SessionInput{
		Adapter:         adapter,
		AdapterVersion:  1,
		ProcedureSHA256: procedure,
		Role:            RoleStandalone,
		Order:           OrderStandalone,
	}); err != nil {
		t.Fatal(err)
	}
	return ArchiveInput{TracePath: tracePath, SessionPath: sessionPath}
}

func validArchiveTrace(fields ...string) Document {
	if len(fields) == 0 {
		fields = []string{"region"}
	}
	return Document{
		SchemaVersion: schemaVersion,
		Redacted:      true,
		Scope:         "outbound",
		Completeness:  Complete,
		Events: []Event{{
			Source:      "android",
			Channel:     "network",
			Kind:        "request",
			Destination: "analytics",
			Fields:      fields,
		}},
	}
}
