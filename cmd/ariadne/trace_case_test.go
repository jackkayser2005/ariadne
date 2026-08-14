package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceCaseCommands(t *testing.T) {
	want := trace.CaseVerificationSummary{
		SchemaVersion: 1, OrderBasis: "caller", Entries: 2, Archives: 1, Replications: 1,
		Sources:    []trace.CaseSourceSummary{{Source: "android", Adapter: "android-experiment-001", Entries: 2}},
		Outcomes:   []trace.CaseOutcomeSummary{{Position: 2, Outcome: trace.ReplicatedChange, EvidenceState: evidence.Observed, Pairs: 2}},
		CaseSHA256: strings.Repeat("a", 64),
	}
	var stdout, stderr strings.Builder
	save := func(inputs []trace.CaseInput, output string) (trace.CaseVerificationSummary, error) {
		if len(inputs) != 2 || inputs[0].Kind != trace.CaseEntryTraceArchive || inputs[1].QuestionRoundPath != "replication-round.json" || output != "case.json" {
			t.Fatalf("case inputs = %#v, output = %q", inputs, output)
		}
		return want, nil
	}
	args := []string{"--json", "case.json", trace.CaseEntryTraceArchive, "archive.json", "archive-round.json", trace.CaseEntryTraceReplication, "ledger.json", "replication-round.json"}
	if exitCode := runTraceCaseSave(args, &stdout, &stderr, save); exitCode != 0 || !strings.Contains(stdout.String(), `"case_sha256"`) || stderr.Len() != 0 {
		t.Fatalf("runTraceCaseSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceCaseVerify([]string{"--expect-sha256", want.CaseSHA256, "case.json"}, &stdout, &stderr, func(string) (trace.CaseVerificationSummary, error) { return want, nil }); exitCode != 0 || !strings.Contains(stdout.String(), "trace case verified") || stderr.Len() != 0 {
		t.Fatalf("runTraceCaseVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseQuestions(nil, &stdout, &stderr, trace.CaseQuestions); exitCode != 0 || !strings.Contains(stdout.String(), "trace case question catalog") {
		t.Fatalf("runTraceCaseQuestions() = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseQuestions([]string{"--json"}, &stdout, &stderr, trace.CaseQuestions); exitCode != 0 || !strings.Contains(stdout.String(), trace.CaseQuestionSupport) {
		t.Fatalf("runTraceCaseQuestions(--json) = %d, stdout=%q", exitCode, stdout.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseAsk([]string{"case.json", trace.CaseQuestionSources}, &stdout, &stderr, func(string, string) (trace.CaseAnswer, error) {
		return trace.CaseAnswer{QuestionID: trace.CaseQuestionSources, Question: "sources", Result: "available", EvidenceState: evidence.Observed, Reason: "safe reason", Sources: want.Sources}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "trace case question answered") || !strings.Contains(stdout.String(), "reason: safe reason") {
		t.Fatalf("runTraceCaseAsk() = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseAsk([]string{"--json", "case.json", trace.CaseQuestionSources}, &stdout, &stderr, func(string, string) (trace.CaseAnswer, error) {
		return trace.CaseAnswer{QuestionID: trace.CaseQuestionSources, Result: "available"}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), `"question_id"`) {
		t.Fatalf("runTraceCaseAsk(--json) = %d, stdout=%q", exitCode, stdout.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseAskAll([]string{"case.json"}, &stdout, &stderr, func(string) ([]trace.CaseAnswer, error) {
		return []trace.CaseAnswer{{QuestionID: trace.CaseQuestionSources, Question: "sources", Result: "available", EvidenceState: evidence.Observed, Sources: want.Sources}}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "trace case questions answered") {
		t.Fatalf("runTraceCaseAskAll() = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseAskAll([]string{"--json", "case.json"}, &stdout, &stderr, func(string) ([]trace.CaseAnswer, error) {
		return []trace.CaseAnswer{{QuestionID: trace.CaseQuestionSources, Result: "available"}}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), `"question_id"`) {
		t.Fatalf("runTraceCaseAskAll(--json) = %d, stdout=%q", exitCode, stdout.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseAskAllSave([]string{"case.json", "round.json"}, &stdout, &stderr, func(string, string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{SchemaVersion: 1, Questions: 3, CaseSHA256: want.CaseSHA256, RoundSHA256: strings.Repeat("b", 64)}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "trace case question round saved") {
		t.Fatalf("runTraceCaseAskAllSave() = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseAskAllSave([]string{"--json", "case.json", "round.json"}, &stdout, &stderr, func(string, string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{Questions: 3}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), `"questions"`) {
		t.Fatalf("runTraceCaseAskAllSave(--json) = %d, stdout=%q", exitCode, stdout.String())
	}

	stdout.Reset()
	if exitCode := runTraceCaseAskAllVerify([]string{"--expect-sha256", strings.Repeat("c", 64), "round.json"}, &stdout, &stderr, func(string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{SchemaVersion: 1, Questions: 3, CaseSHA256: want.CaseSHA256, RoundSHA256: strings.Repeat("c", 64)}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), "trace case question round verified") {
		t.Fatalf("runTraceCaseAskAllVerify() = %d, stdout=%q", exitCode, stdout.String())
	}
	stdout.Reset()
	if exitCode := runTraceCaseAskAllVerify([]string{"--json", "round.json"}, &stdout, &stderr, func(string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{Questions: 3}, nil
	}); exitCode != 0 || !strings.Contains(stdout.String(), `"questions"`) {
		t.Fatalf("runTraceCaseAskAllVerify(--json) = %d, stdout=%q", exitCode, stdout.String())
	}
}

func TestRunTraceCaseFailuresAndWriters(t *testing.T) {
	for name, run := range map[string]func(*strings.Builder, *strings.Builder) int{
		"save usage": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseSave([]string{"case.json"}, stdout, stderr, func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error) {
				return trace.CaseVerificationSummary{}, nil
			})
		},
		"verify bad digest": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseVerify([]string{"--expect-sha256", "bad", "case.json"}, stdout, stderr, func(string) (trace.CaseVerificationSummary, error) { return trace.CaseVerificationSummary{}, nil })
		},
		"verify mismatch": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseVerify([]string{"--expect-sha256", strings.Repeat("a", 64), "case.json"}, stdout, stderr, func(string) (trace.CaseVerificationSummary, error) {
				return trace.CaseVerificationSummary{CaseSHA256: strings.Repeat("b", 64)}, nil
			})
		},
		"verify error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseVerify([]string{"case.json"}, stdout, stderr, func(string) (trace.CaseVerificationSummary, error) {
				return trace.CaseVerificationSummary{}, errors.New("verify failed safely")
			})
		},
		"questions usage": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseQuestions([]string{"extra"}, stdout, stderr, trace.CaseQuestions)
		},
		"ask error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseAsk([]string{"case.json", "question"}, stdout, stderr, func(string, string) (trace.CaseAnswer, error) {
				return trace.CaseAnswer{}, errors.New("ask failed safely")
			})
		},
		"all error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseAskAll([]string{"case.json"}, stdout, stderr, func(string) ([]trace.CaseAnswer, error) { return nil, errors.New("all failed safely") })
		},
		"round save error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseAskAllSave([]string{"case.json", "round.json"}, stdout, stderr, func(string, string) (trace.CaseQuestionRoundVerificationSummary, error) {
				return trace.CaseQuestionRoundVerificationSummary{}, errors.New("save failed safely")
			})
		},
		"round verify error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseAskAllVerify([]string{"round.json"}, stdout, stderr, func(string) (trace.CaseQuestionRoundVerificationSummary, error) {
				return trace.CaseQuestionRoundVerificationSummary{}, errors.New("verify failed safely")
			})
		},
		"save error": func(stdout, stderr *strings.Builder) int {
			return runTraceCaseSave([]string{"case.json", trace.CaseEntryTraceArchive, "archive.json", "round.json"}, stdout, stderr, func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error) {
				return trace.CaseVerificationSummary{}, errors.New("save failed safely")
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if exitCode := run(&stdout, &stderr); exitCode != 1 && name != "save usage" && name != "questions usage" {
				t.Fatalf("exit code = %d", exitCode)
			}
		})
	}
	if exitCode := runTraceCaseSave([]string{"case.json", trace.CaseEntryTraceArchive, "archive.json", "round.json"}, failingWriter{}, &strings.Builder{}, func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error) {
		return trace.CaseVerificationSummary{}, errors.New("save failed")
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseSave() error = %d", exitCode)
	}
	if err := writeTraceCaseSummary(failingWriter{}, "heading", trace.CaseVerificationSummary{}); err == nil {
		t.Fatal("writeTraceCaseSummary() accepted a failing writer")
	}
	if err := writeTraceCaseAnswer(failingWriter{}, trace.CaseAnswer{}); err == nil {
		t.Fatal("writeTraceCaseAnswer() accepted a failing writer")
	}
	if err := writeTraceCaseRoundSummary(failingWriter{}, "heading", trace.CaseQuestionRoundVerificationSummary{}); err == nil {
		t.Fatal("writeTraceCaseRoundSummary() accepted a failing writer")
	}
	if exitCode := runTraceCaseSave([]string{"case.json", trace.CaseEntryTraceArchive, "archive.json", "round.json"}, failingWriter{}, &strings.Builder{}, func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error) {
		return trace.CaseVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseSave() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseVerify([]string{"case.json"}, failingWriter{}, &strings.Builder{}, func(string) (trace.CaseVerificationSummary, error) { return trace.CaseVerificationSummary{}, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseVerify() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseQuestions(nil, failingWriter{}, &strings.Builder{}, trace.CaseQuestions); exitCode != 1 {
		t.Fatalf("runTraceCaseQuestions() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAsk([]string{"case.json", trace.CaseQuestionSources}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseAnswer, error) { return trace.CaseAnswer{}, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseAsk() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAll([]string{"case.json"}, failingWriter{}, &strings.Builder{}, func(string) ([]trace.CaseAnswer, error) { return nil, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAll() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAllSave([]string{"case.json", "round.json"}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAllSave() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAllVerify([]string{"round.json"}, failingWriter{}, &strings.Builder{}, func(string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAllVerify() writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseSave([]string{"--json", "case.json", trace.CaseEntryTraceArchive, "archive.json", "round.json"}, failingWriter{}, &strings.Builder{}, func([]trace.CaseInput, string) (trace.CaseVerificationSummary, error) {
		return trace.CaseVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseSave() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseVerify([]string{"--json", "case.json"}, failingWriter{}, &strings.Builder{}, func(string) (trace.CaseVerificationSummary, error) { return trace.CaseVerificationSummary{}, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseVerify() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseQuestions([]string{"--json"}, failingWriter{}, &strings.Builder{}, trace.CaseQuestions); exitCode != 1 {
		t.Fatalf("runTraceCaseQuestions() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAsk([]string{"--json", "case.json", trace.CaseQuestionSources}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseAnswer, error) { return trace.CaseAnswer{}, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseAsk() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAll([]string{"--json", "case.json"}, failingWriter{}, &strings.Builder{}, func(string) ([]trace.CaseAnswer, error) { return nil, nil }); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAll() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAllSave([]string{"--json", "case.json", "round.json"}, failingWriter{}, &strings.Builder{}, func(string, string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAllSave() JSON writer error = %d", exitCode)
	}
	if exitCode := runTraceCaseAskAllVerify([]string{"--json", "round.json"}, failingWriter{}, &strings.Builder{}, func(string) (trace.CaseQuestionRoundVerificationSummary, error) {
		return trace.CaseQuestionRoundVerificationSummary{}, nil
	}); exitCode != 1 {
		t.Fatalf("runTraceCaseAskAllVerify() JSON writer error = %d", exitCode)
	}
	if err := writeTraceCaseSummary(&caseFailAfterWriter{failAfter: 1}, "heading", trace.CaseVerificationSummary{Sources: []trace.CaseSourceSummary{{Source: "android", Adapter: "fixture", Entries: 1}}}); err == nil {
		t.Fatal("writeTraceCaseSummary() accepted a writer failing in source output")
	}
	if err := writeTraceCaseAnswer(&caseFailAfterWriter{failAfter: 1}, trace.CaseAnswer{Reason: "safe", Sources: []trace.CaseSourceSummary{{Source: "android", Adapter: "fixture", Entries: 1}}}); err == nil {
		t.Fatal("writeTraceCaseAnswer() accepted a writer failing in source output")
	}
	if err := writeTraceCaseAnswer(&caseFailAfterWriter{failAfter: 1}, trace.CaseAnswer{Outcomes: []trace.CaseOutcomeSummary{{Position: 1}}}); err == nil {
		t.Fatal("writeTraceCaseAnswer() accepted a writer failing in outcome output")
	}
	if err := writeTraceCaseAnswer(&caseFailAfterWriter{failAfter: 2}, trace.CaseAnswer{Reason: "safe", Sources: []trace.CaseSourceSummary{{Source: "android", Adapter: "fixture", Entries: 1}}}); err == nil {
		t.Fatal("writeTraceCaseAnswer() accepted a writer failing after the reason")
	}
	if err := writeTraceCaseSummary(&caseFailAfterWriter{failAfter: 1}, "heading", trace.CaseVerificationSummary{Outcomes: []trace.CaseOutcomeSummary{{Position: 1}}}); err == nil {
		t.Fatal("writeTraceCaseSummary() accepted a writer failing in outcome output")
	}
}

func TestRunTraceCaseDispatch(t *testing.T) {
	var stdout, stderr strings.Builder
	if exitCode := run([]string{"trace", "case", "questions", "--json"}, &stdout, &stderr); exitCode != 0 || !strings.Contains(stdout.String(), trace.CaseQuestionSources) || stderr.Len() != 0 {
		t.Fatalf("run() case dispatch = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}

type caseFailAfterWriter struct {
	failAfter int
	writes    int
}

func (w *caseFailAfterWriter) Write(data []byte) (int, error) {
	if w.writes >= w.failAfter {
		return 0, errors.New("write failed safely")
	}
	w.writes++
	return len(data), nil
}
