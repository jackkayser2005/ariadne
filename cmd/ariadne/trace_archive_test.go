package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestRunTraceArchiveCreate(t *testing.T) {
	want := trace.ArchiveVerificationSummary{
		SchemaVersion: 1,
		OrderBasis:    "caller",
		Entries:       2,
		Complete:      2,
		ArchiveSHA256: strings.Repeat("a", 64),
		Sources: []trace.ArchiveSourceSummary{{
			Source:  "android",
			Adapter: "android-experiment-001",
			Entries: 2,
		}},
	}
	save := func(inputs []trace.ArchiveInput, output string) (trace.ArchiveVerificationSummary, error) {
		if output != "archive.json" || len(inputs) != 2 || inputs[0].TracePath != "one-trace.json" || inputs[0].SessionPath != "one-session.json" || inputs[1].TracePath != "two-trace.json" || inputs[1].SessionPath != "two-session.json" {
			t.Fatalf("SaveArchive() inputs = %#v, output = %q", inputs, output)
		}
		return want, nil
	}
	args := []string{"--trace", "one-trace.json", "--session", "one-session.json", "--trace", "two-trace.json", "--session", "two-session.json", "archive.json"}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceArchiveCreate(args, &stdout, &stderr, save); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("human create = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, wantText := range []string{"trace archive complete", "order_basis: caller", "entries: 2", "source: android", "adapter: android-experiment-001", "archive_sha256: " + want.ArchiveSHA256} {
		if !strings.Contains(stdout.String(), wantText) {
			t.Fatalf("human output missing %q: %s", wantText, stdout.String())
		}
	}
	stdout.Reset()
	if exitCode := runTraceArchiveCreate(append([]string{"--json"}, args...), &stdout, &stderr, save); exitCode != 0 {
		t.Fatalf("JSON create = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.ArchiveVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON create = %#v, err=%v", got, err)
	}
	for _, invalid := range [][]string{
		{"archive.json"},
		{"--trace", "one-trace.json", "archive.json"},
		{"--trace", "one-trace.json", "--session", "one-session.json", "--session", "two-session.json", "archive.json"},
	} {
		stdout.Reset()
		stderr.Reset()
		if exitCode := runTraceArchiveCreate(invalid, &stdout, &stderr, save); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid create %v = %d, stdout=%q, stderr=%q", invalid, exitCode, stdout.String(), stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceArchiveCreate(args, &stdout, &stderr, func([]trace.ArchiveInput, string) (trace.ArchiveVerificationSummary, error) {
		return trace.ArchiveVerificationSummary{}, errors.New("archive save failed safely")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "archive save failed safely") {
		t.Fatalf("create error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	if exitCode := runTraceArchiveCreate(append([]string{"--json"}, args...), browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON create write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveCreate(args, browserErrorWriter{}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human create write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveCreate(args, &failAfterWriter{failAt: 2}, &stderr, save); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("summary detail write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceArchiveCreateAdapterRuns(t *testing.T) {
	want := trace.ArchiveVerificationSummary{
		SchemaVersion: 2,
		OrderBasis:    "caller",
		Entries:       2,
		Complete:      2,
		ArchiveSHA256: strings.Repeat("e", 64),
	}
	saveRuns := func(runDirs []string, output string) (trace.ArchiveVerificationSummary, error) {
		if output != "adapter-archive.json" || !reflect.DeepEqual(runDirs, []string{"one-run", "two-run"}) {
			t.Fatalf("SaveSourceAdapterArchive() inputs = %#v, output = %q", runDirs, output)
		}
		return want, nil
	}
	saveLegacy := func([]trace.ArchiveInput, string) (trace.ArchiveVerificationSummary, error) {
		t.Fatal("SaveArchive called for adapter-run input")
		return trace.ArchiveVerificationSummary{}, nil
	}
	args := []string{"--run", "one-run", "--run", "two-run", "adapter-archive.json"}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceArchiveCreate(args, &stdout, &stderr, saveLegacy, saveRuns); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("human adapter create = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, wantText := range []string{"schema_version: 2", "entries: 2", "archive_sha256: " + want.ArchiveSHA256} {
		if !strings.Contains(stdout.String(), wantText) {
			t.Fatalf("human adapter output missing %q: %s", wantText, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceArchiveCreate(append([]string{"--json"}, args...), &stdout, &stderr, saveLegacy, saveRuns); exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("JSON adapter create = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.ArchiveVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON adapter create = %#v, err=%v", got, err)
	}
	for _, invalid := range [][]string{
		{"archive.json"},
		{"--run", "one-run", "--trace", "trace.json", "--session", "session.json", "archive.json"},
		{"--run", "one-run", "--session", "session.json", "archive.json"},
	} {
		stdout.Reset()
		stderr.Reset()
		if exitCode := runTraceArchiveCreate(invalid, &stdout, &stderr, saveLegacy, saveRuns); exitCode != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("invalid adapter create %v = %d, stdout=%q, stderr=%q", invalid, exitCode, stdout.String(), stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceArchiveCreate(args, &stdout, &stderr, saveLegacy, func([]string, string) (trace.ArchiveVerificationSummary, error) {
		return trace.ArchiveVerificationSummary{}, errors.New("adapter archive save failed safely")
	}); exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "adapter archive save failed safely") {
		t.Fatalf("adapter create error = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
}
func TestRunTraceArchiveVerify(t *testing.T) {
	want := trace.ArchiveVerificationSummary{SchemaVersion: 1, OrderBasis: "caller", Entries: 2, ArchiveSHA256: strings.Repeat("b", 64)}
	verify := func(path string) (trace.ArchiveVerificationSummary, error) {
		if path != "archive.json" {
			t.Fatalf("VerifyArchive() path = %q", path)
		}
		return want, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceArchiveVerify([]string{"--expect-sha256", want.ArchiveSHA256, "archive.json"}, &stdout, &stderr, verify); exitCode != 0 || !strings.Contains(stdout.String(), "trace archive verified") {
		t.Fatalf("human verify = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveVerify([]string{"--json", "archive.json"}, &stdout, &stderr, verify); exitCode != 0 {
		t.Fatalf("JSON verify = %d, stderr=%q", exitCode, stderr.String())
	}
	var got trace.ArchiveVerificationSummary
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON verify = %#v, err=%v", got, err)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: nil, want: "usage:"},
		{args: []string{"--expect-sha256", "bad", "archive.json"}, want: "expected SHA-256"},
		{args: []string{"--expect-sha256", strings.Repeat("c", 64), "archive.json"}, want: "does not match"},
	} {
		stdout.Reset()
		stderr.Reset()
		if exitCode := runTraceArchiveVerify(test.args, &stdout, &stderr, verify); exitCode != 1 && !(test.args == nil && exitCode == 2) {
			t.Fatalf("verify args %v = %d, stderr=%q", test.args, exitCode, stderr.String())
		}
		if !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("verify args %v missing %q: %s", test.args, test.want, stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if exitCode := runTraceArchiveVerify([]string{"archive.json"}, &stdout, &stderr, func(string) (trace.ArchiveVerificationSummary, error) {
		return trace.ArchiveVerificationSummary{}, errors.New("archive verify failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "archive verify failed safely") {
		t.Fatalf("verify error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveVerify([]string{"--json", "archive.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveVerify([]string{"archive.json"}, browserErrorWriter{}, &stderr, verify); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("human verify write error = %d, stderr=%q", exitCode, stderr.String())
	}
}

func TestRunTraceArchiveQuestionsAndAsk(t *testing.T) {
	catalog := []trace.ArchiveQuestion{{ID: trace.ArchiveQuestionCoverage, Text: "coverage question"}}
	answer := trace.ArchiveAnswer{SchemaVersion: 1, QuestionID: trace.ArchiveQuestionCoverage, Question: "coverage question", Result: "complete", EvidenceState: "observed", Reason: "safe reason", ArchiveSHA256: strings.Repeat("d", 64), Entries: 2, Sources: []trace.ArchiveSourceSummary{{Source: "android", Adapter: "android-experiment-001", Entries: 2}}}
	all := []trace.ArchiveAnswer{answer}
	questions := func() []trace.ArchiveQuestion { return catalog }
	ask := func(path, questionID string) (trace.ArchiveAnswer, error) {
		if path != "archive.json" || questionID != trace.ArchiveQuestionCoverage {
			t.Fatalf("AskArchive() args = %q, %q", path, questionID)
		}
		return answer, nil
	}
	askAll := func(path string) ([]trace.ArchiveAnswer, error) {
		if path != "archive.json" {
			t.Fatalf("AskAllArchive() path = %q", path)
		}
		return all, nil
	}
	var stdout, stderr bytes.Buffer
	if exitCode := runTraceArchiveQuestions(nil, &stdout, &stderr, questions); exitCode != 0 || !strings.Contains(stdout.String(), "trace archive question catalog") || !strings.Contains(stdout.String(), catalog[0].ID) {
		t.Fatalf("human questions = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveQuestions([]string{"--json"}, &stdout, &stderr, questions); exitCode != 0 {
		t.Fatalf("JSON questions = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotCatalog []trace.ArchiveQuestion
	if err := json.Unmarshal(stdout.Bytes(), &gotCatalog); err != nil || !reflect.DeepEqual(gotCatalog, catalog) {
		t.Fatalf("JSON questions = %#v, err=%v", gotCatalog, err)
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAsk([]string{"archive.json", trace.ArchiveQuestionCoverage}, &stdout, &stderr, ask); exitCode != 0 || !strings.Contains(stdout.String(), "result: complete") || !strings.Contains(stdout.String(), "reason: safe reason") {
		t.Fatalf("human ask = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAsk([]string{"--json", "archive.json", trace.ArchiveQuestionCoverage}, &stdout, &stderr, ask); exitCode != 0 {
		t.Fatalf("JSON ask = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotAnswer trace.ArchiveAnswer
	if err := json.Unmarshal(stdout.Bytes(), &gotAnswer); err != nil || !reflect.DeepEqual(gotAnswer, answer) {
		t.Fatalf("JSON ask = %#v, err=%v", gotAnswer, err)
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskAll([]string{"archive.json"}, &stdout, &stderr, askAll); exitCode != 0 || !strings.Contains(stdout.String(), "trace archive questions answered") {
		t.Fatalf("human ask all = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if exitCode := runTraceArchiveAskAll([]string{"--json", "archive.json"}, &stdout, &stderr, askAll); exitCode != 0 {
		t.Fatalf("JSON ask all = %d, stderr=%q", exitCode, stderr.String())
	}
	var gotAll []trace.ArchiveAnswer
	if err := json.Unmarshal(stdout.Bytes(), &gotAll); err != nil || !reflect.DeepEqual(gotAll, all) {
		t.Fatalf("JSON ask all = %#v, err=%v", gotAll, err)
	}
	answerWithoutReason := answer
	answerWithoutReason.Reason = ""
	if exitCode := runTraceArchiveAsk([]string{"archive.json", trace.ArchiveQuestionCoverage}, &stdout, &stderr, func(string, string) (trace.ArchiveAnswer, error) {
		return answerWithoutReason, nil
	}); exitCode != 0 || strings.Contains(stdout.String(), "reason:") {
		t.Fatalf("answer without reason = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	for _, test := range []struct {
		name string
		args []string
		call func([]string, ioWriter, ioWriter) int
	}{
		{name: "questions usage", args: []string{"extra"}, call: func(args []string, out, errOut ioWriter) int {
			return runTraceArchiveQuestions(args, out, errOut, questions)
		}},
		{name: "ask usage", args: []string{"archive.json"}, call: func(args []string, out, errOut ioWriter) int { return runTraceArchiveAsk(args, out, errOut, ask) }},
		{name: "ask all usage", args: nil, call: func(args []string, out, errOut ioWriter) int { return runTraceArchiveAskAll(args, out, errOut, askAll) }},
	} {
		var out, errOut bytes.Buffer
		if got := test.call(test.args, &out, &errOut); got != 2 || out.Len() != 0 || errOut.Len() == 0 {
			t.Fatalf("%s = %d, stdout=%q, stderr=%q", test.name, got, out.String(), errOut.String())
		}
	}
	if exitCode := runTraceArchiveQuestions(nil, browserErrorWriter{}, &stderr, questions); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("questions write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveQuestions([]string{"--json"}, browserErrorWriter{}, &stderr, questions); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON questions write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAsk([]string{"archive.json", trace.ArchiveQuestionCoverage}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAsk([]string{"--json", "archive.json", trace.ArchiveQuestionCoverage}, browserErrorWriter{}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON ask write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskAll([]string{"archive.json"}, browserErrorWriter{}, &stderr, askAll); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("ask all write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskAll([]string{"--json", "archive.json"}, browserErrorWriter{}, &stderr, askAll); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("JSON ask all write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAsk([]string{"archive.json", trace.ArchiveQuestionCoverage}, &failAfterWriter{failAt: 2}, &stderr, ask); exitCode != 1 || !strings.Contains(stderr.String(), "write output") {
		t.Fatalf("answer detail write error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAsk([]string{"archive.json", trace.ArchiveQuestionCoverage}, &stdout, &stderr, func(string, string) (trace.ArchiveAnswer, error) {
		return trace.ArchiveAnswer{}, errors.New("question failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "question failed safely") {
		t.Fatalf("ask error = %d, stderr=%q", exitCode, stderr.String())
	}
	if exitCode := runTraceArchiveAskAll([]string{"archive.json"}, &stdout, &stderr, func(string) ([]trace.ArchiveAnswer, error) {
		return nil, errors.New("all questions failed safely")
	}); exitCode != 1 || !strings.Contains(stderr.String(), "all questions failed safely") {
		t.Fatalf("ask all error = %d, stderr=%q", exitCode, stderr.String())
	}
}

type ioWriter interface {
	Write([]byte) (int, error)
}
