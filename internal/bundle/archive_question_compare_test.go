package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/evidence"
)

func TestCompareArchiveQuestionReports(t *testing.T) {
	olderPath, olderReport := savedArchiveQuestionReport(t, "a-run")
	same, err := CompareArchiveQuestionReports(olderPath, olderPath)
	if err != nil {
		t.Fatal(err)
	}
	if same.SchemaVersion != 2 || same.Result != "same" || same.Compared != 1 || same.Changed != 0 || same.OlderOnly != 0 || same.NewerOnly != 0 || len(same.StateChanges) != 0 {
		t.Fatalf("same comparison = %#v", same)
	}
	if same.OlderReflectionSHA256 != same.NewerReflectionSHA256 || !validDigest(same.OlderReflectionSHA256) {
		t.Fatalf("same comparison identities = %#v", same)
	}

	olderReport.Results[0].Answer.State = evidence.Unknown
	olderReport.Summary.Observed = 0
	olderReport.Summary.Unknown = 1
	changedPath := writeArchiveQuestionReport(t, olderReport)
	changed, err := CompareArchiveQuestionReports(olderPath, changedPath)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Result != "changed" || changed.Compared != 1 || changed.Changed != 1 || len(changed.StateChanges) != 1 || changed.StateChanges[0].Directory != "a-run" || changed.StateChanges[0].OlderState != "observed" || changed.StateChanges[0].NewerState != "unknown" {
		t.Fatalf("changed comparison = %#v", changed)
	}

	newerPath, _ := savedArchiveQuestionReport(t, "a-run", "b-run")
	incomparable, err := CompareArchiveQuestionReports(olderPath, newerPath)
	if err != nil {
		t.Fatal(err)
	}
	if incomparable.Result != "incomparable" || incomparable.Compared != 1 || incomparable.OlderOnly != 0 || incomparable.NewerOnly != 1 {
		t.Fatalf("incomparable comparison = %#v", incomparable)
	}

	data, err := json.Marshal(incomparable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("comparison exposed a raw value: %s", data)
	}
}

func TestCompareArchiveQuestionStateChangesAreSorted(t *testing.T) {
	older := ArchiveQuestionReport{
		QuestionID: "counterfactual-change",
		Question:   "Did changing the declared variable influence an observed output?",
		Results: []ArchiveQuestionResult{
			{Directory: "b-run", Available: true, Answer: &Answer{State: evidence.Observed}},
			{Directory: "a-run", Available: true, Answer: &Answer{State: evidence.Unknown}},
		},
	}
	newer := ArchiveQuestionReport{
		QuestionID: older.QuestionID,
		Question:   older.Question,
		Results: []ArchiveQuestionResult{
			{Directory: "b-run", Available: true, Answer: &Answer{State: evidence.Unknown}},
			{Directory: "a-run", Available: true, Answer: &Answer{State: evidence.Observed}},
		},
	}
	comparison := compareVerifiedArchiveQuestionReports(
		older,
		ArchiveQuestionVerificationSummary{ReflectionSHA256: strings.Repeat("a", 64)},
		newer,
		ArchiveQuestionVerificationSummary{ReflectionSHA256: strings.Repeat("b", 64)},
	)
	if len(comparison.StateChanges) != 2 || comparison.StateChanges[0].Directory != "a-run" || comparison.StateChanges[1].Directory != "b-run" {
		t.Fatalf("state changes = %#v", comparison.StateChanges)
	}
}

func TestCompareArchiveQuestionReportsRejectsMismatchedQuestions(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})
	newerReport, err := AskArchive(root, "capture-complete")
	if err != nil {
		t.Fatal(err)
	}
	newerPath := writeArchiveQuestionReport(t, newerReport)
	if _, err := CompareArchiveQuestionReports(olderPath, newerPath); err == nil || !strings.Contains(err.Error(), "questions do not match") {
		t.Fatalf("CompareArchiveQuestionReports() error = %v", err)
	}
}

func TestCompareArchiveQuestionReportsRejectsInvalidOlderReport(t *testing.T) {
	if _, err := CompareArchiveQuestionReports(" ", "newer.json"); err == nil || !strings.Contains(err.Error(), "older reflection") {
		t.Fatalf("CompareArchiveQuestionReports() error = %v", err)
	}
}

func TestCompareArchiveQuestionReportWithArchive(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})

	comparison, err := CompareArchiveQuestionReportWithArchive(olderPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Result != "same" || comparison.Compared != 1 || comparison.Changed != 0 ||
		comparison.OlderOnly != 0 || comparison.NewerOnly != 0 ||
		!validDigest(comparison.OlderReflectionSHA256) || !validDigest(comparison.NewerReflectionSHA256) {
		t.Fatalf("live comparison = %#v", comparison)
	}
}

func TestCompareArchiveQuestionReportWithArchiveRejectsInvalidInput(t *testing.T) {
	if _, err := CompareArchiveQuestionReportWithArchive(" ", t.TempDir()); err == nil || !strings.Contains(err.Error(), "older reflection") {
		t.Fatalf("CompareArchiveQuestionReportWithArchive() older error = %v", err)
	}
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	if _, err := CompareArchiveQuestionReportWithArchive(olderPath, filepath.Join(t.TempDir(), "missing")); err == nil || !strings.Contains(err.Error(), "current archive") {
		t.Fatalf("CompareArchiveQuestionReportWithArchive() current error = %v", err)
	}
}

func TestCompareArchiveQuestionHistory(t *testing.T) {
	olderPath, olderReport := savedArchiveQuestionReport(t, "a-run")
	olderReport.Results[0].Answer.State = evidence.Unknown
	olderReport.Summary.Observed = 0
	olderReport.Summary.Unknown = 1
	changedPath := writeArchiveQuestionReport(t, olderReport)
	newerPath, _ := savedArchiveQuestionReport(t, "a-run", "b-run")

	history, err := CompareArchiveQuestionHistory([]string{olderPath, changedPath, newerPath})
	if err != nil {
		t.Fatal(err)
	}
	if history.SchemaVersion != 3 || history.HistoryID != "answer-state-transitions" ||
		history.HistoryQuestion != "At which supplied boundaries did the bounded answer state change?" ||
		history.QuestionID != "counterfactual-change" || history.OrderBasis != "caller" ||
		history.Snapshots != 3 || len(history.Transitions) != 2 || len(history.SnapshotSummaries) != 3 {
		t.Fatalf("history metadata = %#v", history)
	}
	if history.SnapshotSummaries[0].ReflectionSHA256 != history.Transitions[0].FromReflectionSHA256 ||
		history.SnapshotSummaries[1].ReflectionSHA256 != history.Transitions[0].ToReflectionSHA256 ||
		history.SnapshotSummaries[2].ReflectionSHA256 != history.Transitions[1].ToReflectionSHA256 ||
		history.SnapshotSummaries[0].Observed != 1 || history.SnapshotSummaries[1].Unknown != 1 ||
		history.SnapshotSummaries[2].Checked != 2 {
		t.Fatalf("snapshot summaries = %#v", history.SnapshotSummaries)
	}
	first := history.Transitions[0]
	if first.Result != "changed" || first.Compared != 1 || first.Changed != 1 || first.FromOnly != 0 || first.ToOnly != 0 || len(first.StateChanges) != 1 || first.StateChanges[0].Directory != "a-run" || first.StateChanges[0].OlderState != "observed" || first.StateChanges[0].NewerState != "unknown" {
		t.Fatalf("changed transition = %#v", first)
	}
	second := history.Transitions[1]
	if second.Result != "incomparable" || second.Compared != 1 || second.Changed != 1 || second.FromOnly != 0 || second.ToOnly != 1 || len(second.StateChanges) != 1 || second.StateChanges[0].Directory != "a-run" || second.StateChanges[0].OlderState != "unknown" || second.StateChanges[0].NewerState != "observed" {
		t.Fatalf("incomparable transition = %#v", second)
	}
	for _, transition := range history.Transitions {
		if !validDigest(transition.FromReflectionSHA256) || !validDigest(transition.ToReflectionSHA256) {
			t.Fatalf("transition identities = %#v", transition)
		}
	}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "standard") || strings.Contains(string(data), "personalized") {
		t.Fatalf("history exposed a raw value: %s", data)
	}
}

func TestAnswerArchiveQuestionTransitionHistory(t *testing.T) {
	history := ArchiveQuestionTransitionHistory{
		HistoryID:       "answer-state-transitions",
		HistoryQuestion: "At which supplied boundaries did the bounded answer state change?",
		Transitions: []ArchiveQuestionTransition{
			{Result: "same"},
			{FromReflectionSHA256: strings.Repeat("d", 64), ToReflectionSHA256: strings.Repeat("e", 64), Result: "changed", Changed: 1, StateChanges: []ArchiveQuestionStateChange{{Directory: "b-run", OlderState: "observed", NewerState: "unknown"}}},
			{Result: "incomparable"},
			{FromReflectionSHA256: strings.Repeat("f", 64), ToReflectionSHA256: strings.Repeat("g", 64), Result: "incomparable", Changed: 1, StateChanges: []ArchiveQuestionStateChange{{Directory: "a-run", OlderState: "unknown", NewerState: "unavailable"}}},
		},
	}
	answer := AnswerArchiveQuestionTransitionHistory(history, strings.Repeat("a", 64))
	if answer.SchemaVersion != 1 || answer.QuestionID != history.HistoryID || answer.Question != history.HistoryQuestion || answer.Result != "changed" || answer.TransitionHistorySHA256 != strings.Repeat("a", 64) || answer.Transitions != 4 {
		t.Fatalf("history answer metadata = %#v", answer)
	}
	if !reflect.DeepEqual(answer.ChangedTransitions, []int{2, 4}) || !reflect.DeepEqual(answer.IncomparableTransitions, []int{3, 4}) {
		t.Fatalf("history answer indexes = %#v", answer)
	}
	wantEntries := []ArchiveQuestionTransitionHistoryChange{
		{Transition: 2, FromReflectionSHA256: strings.Repeat("d", 64), ToReflectionSHA256: strings.Repeat("e", 64), Directory: "b-run", OlderState: "observed", NewerState: "unknown"},
		{Transition: 4, FromReflectionSHA256: strings.Repeat("f", 64), ToReflectionSHA256: strings.Repeat("g", 64), Directory: "a-run", OlderState: "unknown", NewerState: "unavailable"},
	}
	if !reflect.DeepEqual(answer.ChangedEntries, wantEntries) {
		t.Fatalf("history answer entries = %#v", answer.ChangedEntries)
	}

	answer = AnswerArchiveQuestionTransitionHistory(ArchiveQuestionTransitionHistory{
		HistoryID:       history.HistoryID,
		HistoryQuestion: history.HistoryQuestion,
		Transitions:     []ArchiveQuestionTransition{{Result: "incomparable"}},
	}, strings.Repeat("b", 64))
	if answer.Result != "incomparable" || len(answer.ChangedTransitions) != 0 || !reflect.DeepEqual(answer.IncomparableTransitions, []int{1}) || len(answer.ChangedEntries) != 0 {
		t.Fatalf("incomparable history answer = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistory(ArchiveQuestionTransitionHistory{
		HistoryID:       history.HistoryID,
		HistoryQuestion: history.HistoryQuestion,
		Transitions:     []ArchiveQuestionTransition{{Result: "same"}},
	}, strings.Repeat("c", 64))
	if answer.Result != "same" || len(answer.ChangedTransitions) != 0 || len(answer.IncomparableTransitions) != 0 || len(answer.ChangedEntries) != 0 {
		t.Fatalf("same history answer = %#v", answer)
	}
}

func TestAnswerArchiveQuestionTransitionHistoryRepeated(t *testing.T) {
	history := ArchiveQuestionTransitionHistory{
		SchemaVersion: 2,
		Transitions: []ArchiveQuestionTransition{
			{
				FromReflectionSHA256: strings.Repeat("a", 64),
				ToReflectionSHA256:   strings.Repeat("b", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              2,
				StateChanges: []ArchiveQuestionStateChange{
					{Directory: "b-run", OlderState: "observed", NewerState: "unknown"},
					{Directory: "z-run", OlderState: "observed", NewerState: "unknown"},
				},
			},
			{
				FromReflectionSHA256: strings.Repeat("b", 64),
				ToReflectionSHA256:   strings.Repeat("c", 64),
				Result:               "changed",
				Compared:             2,
				Changed:              2,
				StateChanges: []ArchiveQuestionStateChange{
					{Directory: "a-run", OlderState: "unknown", NewerState: "observed"},
					{Directory: "b-run", OlderState: "unknown", NewerState: "unavailable"},
				},
			},
			{
				FromReflectionSHA256: strings.Repeat("c", 64),
				ToReflectionSHA256:   strings.Repeat("d", 64),
				Result:               "changed",
				Compared:             1,
				Changed:              1,
				StateChanges:         []ArchiveQuestionStateChange{{Directory: "a-run", OlderState: "observed", NewerState: "unknown"}},
			},
		},
	}
	answer := AnswerArchiveQuestionTransitionHistoryRepeated(history, strings.Repeat("e", 64))
	if answer.SchemaVersion != 1 || answer.QuestionID != "answer-state-repeated-changes" || answer.Question != "Did any safe archive entry change at more than one supplied boundary?" || answer.Result != "repeated" || answer.TransitionHistorySHA256 != strings.Repeat("e", 64) || answer.Transitions != 3 {
		t.Fatalf("repeated history answer metadata = %#v", answer)
	}
	want := []ArchiveQuestionTransitionHistoryRepeatedChange{
		{
			Directory: "a-run",
			Changes: []ArchiveQuestionTransitionHistoryChange{
				{Transition: 2, FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64), Directory: "a-run", OlderState: "unknown", NewerState: "observed"},
				{Transition: 3, FromReflectionSHA256: strings.Repeat("c", 64), ToReflectionSHA256: strings.Repeat("d", 64), Directory: "a-run", OlderState: "observed", NewerState: "unknown"},
			},
		},
		{
			Directory: "b-run",
			Changes: []ArchiveQuestionTransitionHistoryChange{
				{Transition: 1, FromReflectionSHA256: strings.Repeat("a", 64), ToReflectionSHA256: strings.Repeat("b", 64), Directory: "b-run", OlderState: "observed", NewerState: "unknown"},
				{Transition: 2, FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64), Directory: "b-run", OlderState: "unknown", NewerState: "unavailable"},
			},
		},
	}
	if !reflect.DeepEqual(answer.RepeatedEntries, want) {
		t.Fatalf("repeated history answer entries = %#v", answer.RepeatedEntries)
	}

	answer = AnswerArchiveQuestionTransitionHistoryRepeated(ArchiveQuestionTransitionHistory{
		SchemaVersion: 2,
		Transitions:   []ArchiveQuestionTransition{{Result: "changed", Changed: 1, StateChanges: []ArchiveQuestionStateChange{{Directory: "run-001", OlderState: "observed", NewerState: "unknown"}}}},
	}, strings.Repeat("f", 64))
	if answer.Result != "none" || len(answer.RepeatedEntries) != 0 {
		t.Fatalf("non-repeated history answer = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistoryRepeated(ArchiveQuestionTransitionHistory{
		SchemaVersion: 1,
		Transitions:   []ArchiveQuestionTransition{{Result: "changed", Changed: 1}},
	}, strings.Repeat("g", 64))
	if answer.Result != "unavailable" || len(answer.RepeatedEntries) != 0 {
		t.Fatalf("legacy repeated history answer = %#v", answer)
	}
}

func TestArchiveQuestionTransitionHistoryQuestions(t *testing.T) {
	want := []Question{
		{ID: "answer-state-transitions", Text: "At which supplied boundaries did the bounded answer state change?"},
		{ID: "answer-state-repeated-changes", Text: "Did any safe archive entry change at more than one supplied boundary?"},
		{ID: "answer-state-snapshot-summaries", Text: "What bounded answer-state summary did each supplied reflection snapshot record?"},
		{ID: "answer-state-summary-changes", Text: "Did the bounded answer-state summary change at any supplied boundary?"},
	}
	if got := ArchiveQuestionTransitionHistoryQuestions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ArchiveQuestionTransitionHistoryQuestions() = %#v, want %#v", got, want)
	}
}

func TestAnswerArchiveQuestionTransitionHistorySnapshots(t *testing.T) {
	history := ArchiveQuestionTransitionHistory{
		SchemaVersion: 3,
		Snapshots:     2,
		SnapshotSummaries: []ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
		},
	}
	answer := AnswerArchiveQuestionTransitionHistorySnapshots(history, strings.Repeat("c", 64))
	if answer.SchemaVersion != 1 || answer.QuestionID != "answer-state-snapshot-summaries" || answer.Question != "What bounded answer-state summary did each supplied reflection snapshot record?" || answer.Result != "available" || answer.TransitionHistorySHA256 != strings.Repeat("c", 64) || answer.Snapshots != 2 || !reflect.DeepEqual(answer.SnapshotSummaries, history.SnapshotSummaries) {
		t.Fatalf("snapshot history answer = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistorySnapshots(ArchiveQuestionTransitionHistory{
		SchemaVersion: 2,
		Snapshots:     2,
	}, strings.Repeat("d", 64))
	if answer.Result != "unavailable" || answer.Snapshots != 2 || len(answer.SnapshotSummaries) != 0 {
		t.Fatalf("legacy snapshot history answer = %#v", answer)
	}
}

func TestAskArchiveQuestionTransitionHistorySnapshots(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 3
	history.SnapshotSummaries = []ArchiveQuestionTransitionSnapshot{
		{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
		{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
	}
	path := writeArchiveQuestionTransitionHistory(t, history)
	answer, err := AskArchiveQuestionTransitionHistorySnapshots(path)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "available" || !validDigest(answer.TransitionHistorySHA256) || len(answer.SnapshotSummaries) != 2 {
		t.Fatalf("AskArchiveQuestionTransitionHistorySnapshots() = %#v", answer)
	}
	if _, err := AskArchiveQuestionTransitionHistorySnapshots(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistorySnapshots() empty path error = %v", err)
	}
}

func TestAnswerArchiveQuestionTransitionHistorySummary(t *testing.T) {
	history := ArchiveQuestionTransitionHistory{
		SchemaVersion: 3,
		Transitions: []ArchiveQuestionTransition{
			{FromReflectionSHA256: strings.Repeat("a", 64), ToReflectionSHA256: strings.Repeat("b", 64)},
			{FromReflectionSHA256: strings.Repeat("b", 64), ToReflectionSHA256: strings.Repeat("c", 64)},
		},
		SnapshotSummaries: []ArchiveQuestionTransitionSnapshot{
			{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
			{ReflectionSHA256: strings.Repeat("c", 64), Unknown: 1, Checked: 1},
		},
	}
	answer := AnswerArchiveQuestionTransitionHistorySummary(history, strings.Repeat("d", 64))
	if answer.SchemaVersion != 1 || answer.QuestionID != "answer-state-summary-changes" || answer.Question != "Did the bounded answer-state summary change at any supplied boundary?" || answer.Result != "changed" || answer.TransitionHistorySHA256 != strings.Repeat("d", 64) || answer.Transitions != 2 || !reflect.DeepEqual(answer.ChangedTransitions, []int{1}) {
		t.Fatalf("summary history answer = %#v", answer)
	}

	history.SnapshotSummaries[1] = history.SnapshotSummaries[0]
	history.SnapshotSummaries[2] = history.SnapshotSummaries[0]
	answer = AnswerArchiveQuestionTransitionHistorySummary(history, strings.Repeat("e", 64))
	if answer.Result != "same" || len(answer.ChangedTransitions) != 0 {
		t.Fatalf("same summary history answer = %#v", answer)
	}

	answer = AnswerArchiveQuestionTransitionHistorySummary(ArchiveQuestionTransitionHistory{
		SchemaVersion: 2,
		Transitions:   history.Transitions,
	}, strings.Repeat("f", 64))
	if answer.Result != "unavailable" || answer.Transitions != 2 || len(answer.ChangedTransitions) != 0 {
		t.Fatalf("legacy summary history answer = %#v", answer)
	}
}

func TestAskArchiveQuestionTransitionHistorySummary(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 3
	history.SnapshotSummaries = []ArchiveQuestionTransitionSnapshot{
		{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
		{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
	}
	path := writeArchiveQuestionTransitionHistory(t, history)
	answer, err := AskArchiveQuestionTransitionHistorySummary(path)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "changed" || !validDigest(answer.TransitionHistorySHA256) || !reflect.DeepEqual(answer.ChangedTransitions, []int{1}) {
		t.Fatalf("AskArchiveQuestionTransitionHistorySummary() = %#v", answer)
	}
	if _, err := AskArchiveQuestionTransitionHistorySummary(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistorySummary() empty path error = %v", err)
	}
}

func TestAnswerArchiveQuestionTransitionHistoryQuestionRound(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 3
	history.SnapshotSummaries = []ArchiveQuestionTransitionSnapshot{
		{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
		{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
	}
	round := AnswerArchiveQuestionTransitionHistoryQuestionRound(history, strings.Repeat("c", 64))
	if round.SchemaVersion != 2 || round.HistoryQuestionID != "counterfactual-change" || round.TransitionHistorySHA256 != strings.Repeat("c", 64) || len(round.Questions) != 4 {
		t.Fatalf("question round = %#v", round)
	}
	want := []ArchiveQuestionTransitionHistoryQuestionRoundItem{
		{QuestionID: "answer-state-transitions", Question: "At which supplied boundaries did the bounded answer state change?", Result: "changed"},
		{QuestionID: "answer-state-repeated-changes", Question: "Did any safe archive entry change at more than one supplied boundary?", Result: "none"},
		{QuestionID: "answer-state-snapshot-summaries", Question: "What bounded answer-state summary did each supplied reflection snapshot record?", Result: "available"},
		{QuestionID: "answer-state-summary-changes", Question: "Did the bounded answer-state summary change at any supplied boundary?", Result: "changed"},
	}
	if !reflect.DeepEqual(round.Questions, want) {
		t.Fatalf("question round questions = %#v, want %#v", round.Questions, want)
	}

	legacy := validArchiveQuestionTransitionHistory()
	round = AnswerArchiveQuestionTransitionHistoryQuestionRound(legacy, strings.Repeat("d", 64))
	if round.Questions[2].Result != "unavailable" || round.Questions[3].Result != "unavailable" {
		t.Fatalf("legacy question round = %#v", round)
	}
}

func TestAskArchiveQuestionTransitionHistoryQuestionRound(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 3
	history.SnapshotSummaries = []ArchiveQuestionTransitionSnapshot{
		{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
		{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
	}
	path := writeArchiveQuestionTransitionHistory(t, history)
	round, err := AskArchiveQuestionTransitionHistoryQuestionRound(path)
	if err != nil {
		t.Fatal(err)
	}
	if !validDigest(round.TransitionHistorySHA256) || len(round.Questions) != 4 {
		t.Fatalf("AskArchiveQuestionTransitionHistoryQuestionRound() = %#v", round)
	}
	if _, err := AskArchiveQuestionTransitionHistoryQuestionRound(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistoryQuestionRound() empty path error = %v", err)
	}
}

func TestAnswerArchiveQuestionTransitionHistoryReceipt(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	history.SchemaVersion = 3
	history.SnapshotSummaries = []ArchiveQuestionTransitionSnapshot{
		{ReflectionSHA256: strings.Repeat("a", 64), Observed: 1, Checked: 1},
		{ReflectionSHA256: strings.Repeat("b", 64), Unknown: 1, Checked: 1},
	}
	receipt, err := AnswerArchiveQuestionTransitionHistoryReceipt(history, strings.Repeat("c", 64), "answer-state-summary-changes")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.QuestionID != "answer-state-summary-changes" || receipt.Question != "Did the bounded answer-state summary change at any supplied boundary?" || receipt.Result != "changed" || receipt.TransitionHistorySHA256 != strings.Repeat("c", 64) {
		t.Fatalf("receipt metadata = %#v", receipt)
	}
	var detail ArchiveQuestionTransitionHistorySummaryAnswer
	if err := json.Unmarshal(receipt.Answer, &detail); err != nil {
		t.Fatalf("receipt answer JSON: %v", err)
	}
	if detail.Result != "changed" || !reflect.DeepEqual(detail.ChangedTransitions, []int{1}) {
		t.Fatalf("receipt answer = %#v", detail)
	}
	if strings.Contains(string(receipt.Answer), "standard") || strings.Contains(string(receipt.Answer), "personalized") {
		t.Fatalf("receipt exposed a captured value: %s", receipt.Answer)
	}
	if _, err := AnswerArchiveQuestionTransitionHistoryReceipt(history, "digest", "not-a-question"); err == nil || !strings.Contains(err.Error(), "question ID is invalid") {
		t.Fatalf("invalid receipt question error = %v", err)
	}
}

func TestAskArchiveQuestionTransitionHistoryReceipt(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	path := writeArchiveQuestionTransitionHistory(t, history)
	receipt, err := AskArchiveQuestionTransitionHistoryReceipt(path, "answer-state-transitions")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result != "changed" || !validDigest(receipt.TransitionHistorySHA256) || len(receipt.Answer) == 0 {
		t.Fatalf("AskArchiveQuestionTransitionHistoryReceipt() = %#v", receipt)
	}
	if _, err := AskArchiveQuestionTransitionHistoryReceipt(" ", "answer-state-transitions"); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty receipt path error = %v", err)
	}
}

func TestSaveArchiveQuestionTransitionHistoryAnswerReceipt(t *testing.T) {
	historyPath := writeArchiveQuestionTransitionHistory(t, validArchiveQuestionTransitionHistory())
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	summary, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(historyPath, "answer-state-transitions", receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 1 || summary.QuestionID != "answer-state-transitions" || summary.Result != "changed" || !validDigest(summary.TransitionHistorySHA256) || !validDigest(summary.ReceiptSHA256) {
		t.Fatalf("SaveArchiveQuestionTransitionHistoryAnswerReceipt() = %#v", summary)
	}
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt ArchiveQuestionTransitionHistoryAnswerReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	receiptSHA256, err := archiveQuestionTransitionHistoryAnswerReceiptSHA256(receipt)
	if err != nil || receiptSHA256 != summary.ReceiptSHA256 {
		t.Fatalf("saved receipt identity = %q, %v; summary = %q", receiptSHA256, err, summary.ReceiptSHA256)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(historyPath, "answer-state-transitions", receiptPath); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("receipt save overwrite error = %v", err)
	}
	if _, err := SaveArchiveQuestionTransitionHistoryAnswerReceipt(historyPath, "answer-state-transitions", " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty receipt path error = %v", err)
	}
}

func TestAskArchiveQuestionTransitionHistory(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	path := writeArchiveQuestionTransitionHistory(t, history)
	answer, err := AskArchiveQuestionTransitionHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "changed" || !validDigest(answer.TransitionHistorySHA256) || !reflect.DeepEqual(answer.ChangedTransitions, []int{1}) || !reflect.DeepEqual(answer.ChangedEntries, []ArchiveQuestionTransitionHistoryChange{{Transition: 1, FromReflectionSHA256: strings.Repeat("a", 64), ToReflectionSHA256: strings.Repeat("b", 64), Directory: "run-001", OlderState: "observed", NewerState: "unknown"}}) {
		t.Fatalf("AskArchiveQuestionTransitionHistory() = %#v", answer)
	}
	if _, err := AskArchiveQuestionTransitionHistory(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistory() empty path error = %v", err)
	}
}

func TestAskArchiveQuestionTransitionHistoryRepeated(t *testing.T) {
	history := validArchiveQuestionTransitionHistory()
	path := writeArchiveQuestionTransitionHistory(t, history)
	answer, err := AskArchiveQuestionTransitionHistoryRepeated(path)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "none" || !validDigest(answer.TransitionHistorySHA256) || answer.Transitions != 1 {
		t.Fatalf("AskArchiveQuestionTransitionHistoryRepeated() = %#v", answer)
	}

	history.SchemaVersion = 1
	history.Transitions[0].StateChanges = nil
	legacyPath := writeArchiveQuestionTransitionHistory(t, history)
	answer, err = AskArchiveQuestionTransitionHistoryRepeated(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Result != "unavailable" || !validDigest(answer.TransitionHistorySHA256) {
		t.Fatalf("legacy AskArchiveQuestionTransitionHistoryRepeated() = %#v", answer)
	}
	if _, err := AskArchiveQuestionTransitionHistoryRepeated(" "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("AskArchiveQuestionTransitionHistoryRepeated() empty path error = %v", err)
	}
}

func TestCompareArchiveQuestionHistoryRejectsInvalidInput(t *testing.T) {
	if _, err := CompareArchiveQuestionHistory(nil); err == nil || !strings.Contains(err.Error(), "at least two") {
		t.Fatalf("CompareArchiveQuestionHistory() short input error = %v", err)
	}
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	if _, err := CompareArchiveQuestionHistory([]string{olderPath, "missing.json"}); err == nil || !strings.Contains(err.Error(), "reflection 2") {
		t.Fatalf("CompareArchiveQuestionHistory() invalid second report error = %v", err)
	}
}

func TestCompareArchiveQuestionHistoryRejectsMismatchedQuestions(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	root := t.TempDir()
	archiveRun(t, root, "a-run", runOptions{})
	newerReport, err := AskArchive(root, "capture-complete")
	if err != nil {
		t.Fatal(err)
	}
	newerPath := writeArchiveQuestionReport(t, newerReport)
	if _, err := CompareArchiveQuestionHistory([]string{olderPath, newerPath}); err == nil || !strings.Contains(err.Error(), "questions do not match") {
		t.Fatalf("CompareArchiveQuestionHistory() mismatch error = %v", err)
	}
}

func TestSaveArchiveQuestionTransitionHistory(t *testing.T) {
	olderPath, _ := savedArchiveQuestionReport(t, "a-run")
	newerPath, _ := savedArchiveQuestionReport(t, "a-run")
	historyPath := filepath.Join(t.TempDir(), "transitions.json")

	summary, err := SaveArchiveQuestionTransitionHistory([]string{olderPath, newerPath}, historyPath)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyArchiveQuestionTransitionHistory(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary != verified || summary.HistoryID != "answer-state-transitions" || summary.Snapshots != 2 || summary.Transitions != 1 || !validDigest(summary.TransitionHistorySHA256) {
		t.Fatalf("SaveArchiveQuestionTransitionHistory() summary = %#v, verified = %#v", summary, verified)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("saved transition history does not end with a newline: %q", data)
	}
	for _, rawValue := range []string{"standard", "personalized", "baseline@example.invalid", "treatment@example.invalid", "emulator-5554"} {
		if strings.Contains(string(data), rawValue) {
			t.Fatalf("SaveArchiveQuestionTransitionHistory() exposed raw value %q: %s", rawValue, data)
		}
	}
	if _, err := SaveArchiveQuestionTransitionHistory([]string{olderPath, newerPath}, historyPath); err == nil || !strings.Contains(err.Error(), "file exists") {
		t.Fatalf("second SaveArchiveQuestionTransitionHistory() error = %v", err)
	}
}

func TestSaveArchiveQuestionTransitionHistoryRequiresPath(t *testing.T) {
	if _, err := SaveArchiveQuestionTransitionHistory(nil, " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("SaveArchiveQuestionTransitionHistory() error = %v", err)
	}
}
