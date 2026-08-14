// Package ui serves the read-only Ariadne evidence review surface.
package ui

import (
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

type handler struct {
	root                 string
	index                func(string) ([]bundle.ArchiveEntry, error)
	verify               func(string) (bundle.Summary, error)
	questions            func() []bundle.Question
	ask                  func(string, string) (bundle.Answer, error)
	askArchive           func(string, string) (bundle.ArchiveQuestionReport, error)
	history              func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error)
	acceptance           func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error)
	compareRounds        func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error)
	compareCurrent       func() (bundle.ArchiveQuestionComparison, error)
	find                 func(string, string) (bundle.Finding, error)
	exportPath           string
	exportAsk            func(string, string) (bundle.Answer, error)
	exportFind           func(string, string) (bundle.Finding, error)
	traceArchivePath     string
	traceArchiveRead     func(string) (trace.Archive, trace.ArchiveVerificationSummary, error)
	traceRoundPath       string
	traceRoundRead       func(string) (trace.ArchiveQuestionRound, trace.ArchiveQuestionRoundVerificationSummary, error)
	traceReplicationPath string
	traceReplicationRead func(string) (trace.ReplicationLedger, trace.ReplicationLedgerVerificationSummary, error)
}

type archiveQuestionResult struct {
	Directory    string
	ManifestName string
	Summary      bundle.Summary
	Answer       bundle.Answer
	Available    bool
}

type archiveQuestionSummary struct {
	Total       int
	Observed    int
	Unknown     int
	Unavailable int
}

type traceReplicationPairData struct {
	Position              int
	Order                 string
	ResetConfirmed        bool
	PairSHA256            string
	BaselineCompleteness  string
	TreatmentCompleteness string
	Differences           int
	Unknowns              int
	EvidenceState         evidence.State
}

type pageData struct {
	View                               string
	Title                              string
	Directory                          string
	Entries                            []bundle.ArchiveEntry
	Questions                          []bundle.Question
	SelectedQuestion                   bundle.Question
	ArchiveAnswers                     []archiveQuestionResult
	ArchiveSummary                     archiveQuestionSummary
	Summary                            bundle.Summary
	Answers                            []bundle.Answer
	Answer                             bundle.Answer
	Finding                            bundle.Finding
	ExportConfigured                   bool
	ExportAnswer                       bundle.Answer
	ExportFinding                      bundle.Finding
	ExportSourceEvidenceSHA256         string
	ExportSHA256                       string
	CurrentReflectionRequested         bool
	CurrentReflectionAvailable         bool
	CurrentReflectionSHA256            string
	CurrentReflectionChecked           int
	ReflectionHistoryRequested         bool
	ReflectionHistoryAvailable         bool
	ReflectionHistory                  bundle.ArchiveQuestionTransitionHistory
	ReflectionHistorySummary           bundle.ArchiveQuestionTransitionVerificationSummary
	ReflectionHistoryQuestions         []bundle.Question
	ReflectionHistoryQuestionRound     bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer
	ReflectionQuestionRoundSHA256      string
	ReflectionHistoryQuestionID        string
	ReflectionHistoryAnswer            bundle.ArchiveQuestionTransitionHistoryAnswer
	ReflectionHistoryRepeatedAnswer    bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer
	ReflectionHistorySnapshotAnswer    bundle.ArchiveQuestionTransitionHistorySnapshotAnswer
	ReflectionHistorySummaryAnswer     bundle.ArchiveQuestionTransitionHistorySummaryAnswer
	ReflectionHistoryReceiptAvailable  bool
	ReflectionHistoryReceipt           bundle.ArchiveQuestionTransitionHistoryAnswerReceipt
	ReflectionHistoryReceiptSHA256     string
	ReflectionHistoryReceiptJSON       string
	AcceptanceRecordRequested          bool
	AcceptanceRecordAvailable          bool
	AcceptanceRecord                   bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary
	AcceptanceRecordStatus             string
	QuestionRoundComparisonRequested   bool
	QuestionRoundComparisonAvailable   bool
	QuestionRoundComparison            bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison
	SavedReflectionComparisonRequested bool
	SavedReflectionComparisonAvailable bool
	SavedReflectionComparison          bundle.ArchiveQuestionComparison
	TraceArchiveConfigured             bool
	TraceArchiveRoundSaved             bool
	TraceArchiveRoundSHA256            string
	TraceArchiveSummary                trace.ArchiveVerificationSummary
	TraceArchiveAnswers                []trace.ArchiveAnswer
	TraceReplicationConfigured         bool
	TraceReplicationSummary            trace.ReplicationLedgerVerificationSummary
	TraceReplicationAnswers            []trace.ReplicationAnswer
	TraceReplicationPairs              []traceReplicationPairData
}

// Handler returns a read-only HTTP handler for one explicitly supplied archive root.
func Handler(archiveRoot string) http.Handler {
	return HandlerWithReview(archiveRoot, "", "")
}

// HandlerWithHistory returns a read-only HTTP handler that also shows one
// structurally verified saved reflection history.
func HandlerWithHistory(archiveRoot, historyPath string) http.Handler {
	return HandlerWithReview(archiveRoot, historyPath, "")
}

// HandlerWithReview returns a read-only HTTP handler that can show one
// structurally verified saved reflection history and one bounded comparison
// between a saved reflection and the current archive.
func HandlerWithReview(archiveRoot, historyPath, reflectionPath string) http.Handler {
	return HandlerWithReviewAndExport(archiveRoot, historyPath, reflectionPath, "")
}

// HandlerWithReviewAndExport returns a read-only review handler with optional
// verified history, current-reflection comparison, and one portable redacted
// export.
func HandlerWithReviewAndExport(archiveRoot, historyPath, reflectionPath, exportPath string) http.Handler {
	return HandlerWithReviewAndExportAndAcceptance(archiveRoot, historyPath, reflectionPath, exportPath, "")
}

// HandlerWithReviewAndExportAndAcceptance returns a read-only review handler
// with optional verified history, current-reflection comparison, portable
// redacted export, and an offline acceptance identity binding.
func HandlerWithReviewAndExportAndAcceptance(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath string) http.Handler {
	return HandlerWithReviewAndExportAndAcceptanceAndQuestionRounds(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, "", "")
}

// HandlerWithReviewAndExportAndAcceptanceAndQuestionRounds returns a
// read-only review handler with optional saved question-round comparison.
func HandlerWithReviewAndExportAndAcceptanceAndQuestionRounds(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath string) http.Handler {
	return HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchive(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, "")
}

// HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchive
// returns a read-only review handler with an optional portable trace archive
// reflection surface.
func HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchive(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, traceArchivePath string) http.Handler {
	return HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchiveRound(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, traceArchivePath, "")
}

// HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchiveRound
// returns a read-only review handler with an optional live trace archive and
// optional saved trace question round.
func HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceArchiveRound(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, traceArchivePath, traceRoundPath string) http.Handler {
	return HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceReplication(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, traceArchivePath, traceRoundPath, "")
}

// HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceReplication
// returns a read-only review handler with an optional source-neutral
// replicated trace ledger.
func HandlerWithReviewAndExportAndAcceptanceAndQuestionRoundsAndTraceReplication(archiveRoot, historyPath, reflectionPath, exportPath, acceptancePath, firstRoundPath, secondRoundPath, traceArchivePath, traceRoundPath, traceReplicationPath string) http.Handler {
	h := archiveHandler(archiveRoot)
	if historyPath != "" {
		h.history = func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return bundle.ReadArchiveQuestionTransitionHistory(historyPath)
		}
	}
	if reflectionPath != "" {
		h.compareCurrent = func() (bundle.ArchiveQuestionComparison, error) {
			return bundle.CompareArchiveQuestionReportWithArchive(reflectionPath, archiveRoot)
		}
	}
	if exportPath != "" {
		h.exportPath = exportPath
		h.exportAsk = bundle.AskExport
		h.exportFind = bundle.FindExport
	}
	if acceptancePath != "" {
		h.acceptance = func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return bundle.VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(acceptancePath)
		}
	}
	if firstRoundPath != "" && secondRoundPath != "" {
		h.compareRounds = func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return bundle.CompareArchiveQuestionTransitionHistoryQuestionRounds(firstRoundPath, secondRoundPath)
		}
	}
	if traceArchivePath != "" {
		h.traceArchivePath = traceArchivePath
		h.traceArchiveRead = trace.ReadArchive
	}
	if traceRoundPath != "" {
		h.traceRoundPath = traceRoundPath
		h.traceRoundRead = trace.ReadArchiveQuestionRound
	}
	if traceReplicationPath != "" {
		h.traceReplicationPath = traceReplicationPath
		h.traceReplicationRead = trace.ReadReplicationLedger
	}
	return newHandler(h)
}

func archiveHandler(archiveRoot string) handler {
	return handler{
		root:       archiveRoot,
		index:      bundle.Index,
		verify:     bundle.Verify,
		questions:  bundle.Questions,
		ask:        bundle.Ask,
		askArchive: bundle.AskArchive,
		find:       bundle.Find,
	}
}

func newHandler(h handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/run", h.handleRun)
	mux.HandleFunc("/ask", h.handleAsk)
	mux.HandleFunc("/finding", h.handleFinding)
	mux.HandleFunc("/export-ask", h.handleExportAsk)
	mux.HandleFunc("/export-finding", h.handleExportFinding)
	mux.HandleFunc("/trace-archive", h.handleTraceArchive)
	mux.HandleFunc("/trace-replication", h.handleTraceReplication)
	mux.HandleFunc("/favicon.ico", handleFavicon)
	return mux
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	entries, err := h.index(h.root)
	if err != nil {
		http.Error(w, "archive unavailable", http.StatusInternalServerError)
		return
	}
	var questions []bundle.Question
	if h.questions != nil {
		questions = h.questions()
	}
	selectedQuestion, questionID := bundle.Question{}, r.URL.Query().Get("question_id")
	var archiveAnswers []archiveQuestionResult
	var archiveSummary archiveQuestionSummary
	currentReflectionRequested := questionID != "" && h.askArchive != nil
	currentReflectionAvailable := false
	currentReflectionSHA256 := ""
	currentReflectionChecked := 0
	reflectionHistoryRequested := h.history != nil
	reflectionHistoryAvailable := false
	var reflectionHistory bundle.ArchiveQuestionTransitionHistory
	var reflectionHistorySummary bundle.ArchiveQuestionTransitionVerificationSummary
	var reflectionHistoryQuestions []bundle.Question
	var reflectionHistoryQuestionRound bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer
	reflectionHistoryQuestionRoundSHA256 := ""
	reflectionHistoryQuestionID := r.URL.Query().Get("history_question_id")
	var reflectionHistoryAnswer bundle.ArchiveQuestionTransitionHistoryAnswer
	var reflectionHistoryRepeatedAnswer bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer
	var reflectionHistorySnapshotAnswer bundle.ArchiveQuestionTransitionHistorySnapshotAnswer
	var reflectionHistorySummaryAnswer bundle.ArchiveQuestionTransitionHistorySummaryAnswer
	var reflectionHistoryReceipt bundle.ArchiveQuestionTransitionHistoryAnswerReceipt
	reflectionHistoryReceiptAvailable := false
	reflectionHistoryReceiptSHA256 := ""
	reflectionHistoryReceiptJSON := ""
	acceptanceRecordRequested := h.acceptance != nil
	acceptanceRecordAvailable := false
	var acceptanceRecord bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary
	acceptanceRecordStatus := ""
	questionRoundComparisonRequested := h.compareRounds != nil
	questionRoundComparisonAvailable := false
	var questionRoundComparison bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison
	savedReflectionComparisonRequested := h.compareCurrent != nil
	savedReflectionComparisonAvailable := false
	var savedReflectionComparison bundle.ArchiveQuestionComparison
	if reflectionHistoryQuestionID != "" {
		if _, ok := questionForID(bundle.ArchiveQuestionTransitionHistoryQuestions(), reflectionHistoryQuestionID); !ok {
			http.Error(w, "history question not found", http.StatusNotFound)
			return
		}
	}
	if h.history != nil {
		reflectionHistoryQuestions = bundle.ArchiveQuestionTransitionHistoryQuestions()
		var historyErr error
		reflectionHistory, reflectionHistorySummary, historyErr = h.history()
		reflectionHistoryAvailable = historyErr == nil
		if reflectionHistoryAvailable {
			reflectionHistoryAnswer = bundle.AnswerArchiveQuestionTransitionHistory(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256)
			reflectionHistoryRepeatedAnswer = bundle.AnswerArchiveQuestionTransitionHistoryRepeated(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256)
			reflectionHistorySnapshotAnswer = bundle.AnswerArchiveQuestionTransitionHistorySnapshots(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256)
			reflectionHistorySummaryAnswer = bundle.AnswerArchiveQuestionTransitionHistorySummary(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256)
			reflectionHistoryQuestionRound = bundle.AnswerArchiveQuestionTransitionHistoryQuestionRound(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256)
			if roundSHA256, roundSHAErr := bundle.ArchiveQuestionTransitionHistoryQuestionRoundSHA256(reflectionHistoryQuestionRound); roundSHAErr == nil {
				reflectionHistoryQuestionRoundSHA256 = roundSHA256
			}
			if reflectionHistoryQuestionID != "" {
				var receiptErr error
				reflectionHistoryReceipt, receiptErr = bundle.AnswerArchiveQuestionTransitionHistoryReceipt(reflectionHistory, reflectionHistorySummary.TransitionHistorySHA256, reflectionHistoryQuestionID)
				if receiptErr == nil {
					var receiptSHAErr error
					reflectionHistoryReceiptSHA256, receiptSHAErr = bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSHA256(reflectionHistoryReceipt)
					reflectionHistoryReceiptJSONBytes, marshalErr := json.MarshalIndent(reflectionHistoryReceipt, "", "  ")
					if receiptSHAErr == nil && marshalErr == nil {
						reflectionHistoryReceiptJSON = string(reflectionHistoryReceiptJSONBytes)
						reflectionHistoryReceiptAvailable = true
					}
				}
			}
		}
	}
	if h.acceptance != nil {
		var acceptanceErr error
		acceptanceRecord, acceptanceErr = h.acceptance()
		acceptanceRecordAvailable = acceptanceErr == nil
		switch {
		case !acceptanceRecordAvailable:
			acceptanceRecordStatus = "unavailable"
		case !reflectionHistoryAvailable:
			acceptanceRecordStatus = "history unavailable"
		case reflectionHistoryQuestionID == "":
			acceptanceRecordStatus = "select bound question"
		case acceptanceRecordMatches(
			acceptanceRecord,
			reflectionHistorySummary,
			reflectionHistoryQuestionRoundSHA256,
			reflectionHistoryQuestionID,
			reflectionHistoryReceiptAvailable,
			reflectionHistoryReceiptSHA256,
		):
			acceptanceRecordStatus = "matched"
		default:
			acceptanceRecordStatus = "mismatch"
		}
	}
	if h.compareRounds != nil {
		var comparisonErr error
		questionRoundComparison, comparisonErr = h.compareRounds()
		questionRoundComparisonAvailable = comparisonErr == nil
	}
	if h.compareCurrent != nil {
		var comparisonErr error
		savedReflectionComparison, comparisonErr = h.compareCurrent()
		savedReflectionComparisonAvailable = comparisonErr == nil
	}
	if questionID != "" {
		var ok bool
		selectedQuestion, ok = questionForID(questions, questionID)
		if !ok {
			http.Error(w, "question not found", http.StatusNotFound)
			return
		}
		if h.askArchive != nil {
			currentReport, reportErr := h.askArchive(h.root, questionID)
			if reportErr == nil {
				currentReflectionSHA256, reportErr = bundle.ArchiveQuestionReportReflectionSHA256(currentReport)
				if reportErr == nil {
					currentReflectionChecked = currentReport.Summary.Checked
					currentReflectionAvailable = true
				}
			}
		}
		archiveAnswers = make([]archiveQuestionResult, 0, len(entries))
		for _, entry := range entries {
			runDir := filepath.Join(h.root, entry.Directory)
			summary, verifyErr := h.verify(runDir)
			answer := bundle.Answer{}
			askErr := verifyErr
			if verifyErr == nil {
				answer, askErr = h.ask(runDir, questionID)
			}
			archiveAnswers = append(archiveAnswers, archiveQuestionResult{
				Directory:    entry.Directory,
				ManifestName: entry.ManifestName,
				Summary:      summary,
				Answer:       answer,
				Available:    askErr == nil,
			})
		}
		sortArchiveQuestionResults(archiveAnswers)
		archiveSummary = summarizeArchiveAnswers(archiveAnswers)
	}
	render(w, pageData{
		View:                               "index",
		Title:                              "Ariadne — evidence review",
		Entries:                            entries,
		Questions:                          questions,
		SelectedQuestion:                   selectedQuestion,
		ArchiveAnswers:                     archiveAnswers,
		ArchiveSummary:                     archiveSummary,
		CurrentReflectionRequested:         currentReflectionRequested,
		CurrentReflectionAvailable:         currentReflectionAvailable,
		CurrentReflectionSHA256:            currentReflectionSHA256,
		CurrentReflectionChecked:           currentReflectionChecked,
		ReflectionHistoryRequested:         reflectionHistoryRequested,
		ReflectionHistoryAvailable:         reflectionHistoryAvailable,
		ReflectionHistory:                  reflectionHistory,
		ReflectionHistorySummary:           reflectionHistorySummary,
		ReflectionHistoryQuestions:         reflectionHistoryQuestions,
		ReflectionHistoryQuestionRound:     reflectionHistoryQuestionRound,
		ReflectionQuestionRoundSHA256:      reflectionHistoryQuestionRoundSHA256,
		ReflectionHistoryQuestionID:        reflectionHistoryQuestionID,
		ReflectionHistoryAnswer:            reflectionHistoryAnswer,
		ReflectionHistoryRepeatedAnswer:    reflectionHistoryRepeatedAnswer,
		ReflectionHistorySnapshotAnswer:    reflectionHistorySnapshotAnswer,
		ReflectionHistorySummaryAnswer:     reflectionHistorySummaryAnswer,
		ReflectionHistoryReceiptAvailable:  reflectionHistoryReceiptAvailable,
		ReflectionHistoryReceipt:           reflectionHistoryReceipt,
		ReflectionHistoryReceiptSHA256:     reflectionHistoryReceiptSHA256,
		ReflectionHistoryReceiptJSON:       reflectionHistoryReceiptJSON,
		AcceptanceRecordRequested:          acceptanceRecordRequested,
		AcceptanceRecordAvailable:          acceptanceRecordAvailable,
		AcceptanceRecord:                   acceptanceRecord,
		AcceptanceRecordStatus:             acceptanceRecordStatus,
		QuestionRoundComparisonRequested:   questionRoundComparisonRequested,
		QuestionRoundComparisonAvailable:   questionRoundComparisonAvailable,
		QuestionRoundComparison:            questionRoundComparison,
		SavedReflectionComparisonRequested: savedReflectionComparisonRequested,
		SavedReflectionComparisonAvailable: savedReflectionComparisonAvailable,
		SavedReflectionComparison:          savedReflectionComparison,
		ExportConfigured:                   h.exportAsk != nil && h.exportFind != nil,
		TraceArchiveConfigured:             h.traceArchiveConfigured(),
		TraceArchiveRoundSaved:             h.traceRoundPath != "",
		TraceReplicationConfigured:         h.traceReplicationConfigured(),
	})
}

func (h handler) traceArchiveConfigured() bool {
	return h.traceArchivePath != "" || h.traceRoundPath != ""
}

func (h handler) traceReplicationConfigured() bool {
	return h.traceReplicationPath != ""
}

func (h handler) readTraceArchive() (trace.ArchiveVerificationSummary, trace.ArchiveQuestionRoundVerificationSummary, []trace.ArchiveAnswer, error) {
	if !h.traceArchiveConfigured() {
		return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, errors.New("trace archive is not configured")
	}
	var summary trace.ArchiveVerificationSummary
	var round trace.ArchiveQuestionRound
	var roundSummary trace.ArchiveQuestionRoundVerificationSummary
	var liveRoundSHA256 string
	if h.traceArchivePath != "" {
		if h.traceArchiveRead == nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, errors.New("trace archive reader is unavailable")
		}
		archive, archiveSummary, err := h.traceArchiveRead(h.traceArchivePath)
		if err != nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, err
		}
		var roundErr error
		round, roundErr = trace.AnswerArchiveQuestionRound(archive, archiveSummary)
		if roundErr != nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, roundErr
		}
		roundSHA256, roundSHAErr := trace.ArchiveQuestionRoundSHA256(round)
		if roundSHAErr != nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, roundSHAErr
		}
		liveRoundSHA256 = roundSHA256
		summary = archiveSummary
		roundSummary = trace.ArchiveQuestionRoundVerificationSummary{
			SchemaVersion: round.SchemaVersion,
			ArchiveSHA256: round.ArchiveSHA256,
			Questions:     len(round.Answers),
			RoundSHA256:   roundSHA256,
		}
	}
	if h.traceRoundPath != "" {
		if h.traceRoundRead == nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, errors.New("trace archive question round reader is unavailable")
		}
		var savedSummary trace.ArchiveQuestionRoundVerificationSummary
		var err error
		round, savedSummary, err = h.traceRoundRead(h.traceRoundPath)
		if err != nil {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, err
		}
		if summary.ArchiveSHA256 != "" && summary.ArchiveSHA256 != savedSummary.ArchiveSHA256 {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, errors.New("trace archive question round archive identity does not match archive")
		}
		if liveRoundSHA256 != "" && liveRoundSHA256 != savedSummary.RoundSHA256 {
			return trace.ArchiveVerificationSummary{}, trace.ArchiveQuestionRoundVerificationSummary{}, nil, errors.New("trace archive question round identity does not match archive")
		}
		roundSummary = savedSummary
		if summary.ArchiveSHA256 == "" {
			summary = trace.ArchiveVerificationSummary{
				SchemaVersion: round.SchemaVersion,
				OrderBasis:    round.OrderBasis,
				Entries:       round.Entries,
				Complete:      round.Complete,
				Partial:       round.Partial,
				Sources:       round.Sources,
				ArchiveSHA256: round.ArchiveSHA256,
			}
		}
	}
	return summary, roundSummary, round.Answers, nil
}

func (h handler) handleTraceArchive(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/trace-archive" || !h.traceArchiveConfigured() {
		http.NotFound(w, r)
		return
	}
	summary, roundSummary, answers, err := h.readTraceArchive()
	if err != nil {
		http.Error(w, "trace archive unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:                    "trace-archive",
		Title:                   "Trace archive review — Ariadne",
		TraceArchiveConfigured:  true,
		TraceArchiveRoundSaved:  h.traceRoundPath != "",
		TraceArchiveRoundSHA256: roundSummary.RoundSHA256,
		TraceArchiveSummary:     summary,
		TraceArchiveAnswers:     answers,
	})
}

func (h handler) handleTraceReplication(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/trace-replication" || !h.traceReplicationConfigured() {
		http.NotFound(w, r)
		return
	}
	if h.traceReplicationRead == nil {
		http.Error(w, "trace replication unavailable", http.StatusUnprocessableEntity)
		return
	}
	ledger, summary, err := h.traceReplicationRead(h.traceReplicationPath)
	if err != nil {
		http.Error(w, "trace replication unavailable", http.StatusUnprocessableEntity)
		return
	}
	answers, err := trace.AnswerAllReplicationQuestionsFromSummary(summary)
	if err != nil {
		http.Error(w, "trace replication unavailable", http.StatusUnprocessableEntity)
		return
	}
	pairs := make([]traceReplicationPairData, 0, len(ledger.Pairs))
	for _, pair := range ledger.Pairs {
		pairs = append(pairs, traceReplicationPairData{
			Position:              pair.Position,
			Order:                 pair.Pair.Order,
			ResetConfirmed:        pair.ResetConfirmed,
			PairSHA256:            pair.Pair.PairSHA256,
			BaselineCompleteness:  pair.Pair.BaselineCompleteness,
			TreatmentCompleteness: pair.Pair.TreatmentCompleteness,
			Differences:           len(pair.Comparison.Differences),
			Unknowns:              len(pair.Comparison.Unknowns),
			EvidenceState:         trace.ReplicationPairEvidenceState(pair),
		})
	}
	render(w, pageData{
		View:                       "trace-replication",
		Title:                      "Trace replication review — Ariadne",
		TraceReplicationConfigured: true,
		TraceReplicationSummary:    summary,
		TraceReplicationAnswers:    answers,
		TraceReplicationPairs:      pairs,
	})
}

func sortArchiveQuestionResults(results []archiveQuestionResult) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if left.Summary.RecordedAt == right.Summary.RecordedAt {
			return left.Directory < right.Directory
		}
		if left.Summary.RecordedAt == "" {
			return false
		}
		if right.Summary.RecordedAt == "" {
			return true
		}
		leftTime, leftErr := time.Parse(time.RFC3339Nano, left.Summary.RecordedAt)
		rightTime, rightErr := time.Parse(time.RFC3339Nano, right.Summary.RecordedAt)
		if leftErr == nil && rightErr == nil && !leftTime.Equal(rightTime) {
			return leftTime.Before(rightTime)
		}
		return left.Summary.RecordedAt < right.Summary.RecordedAt
	})
}

func summarizeArchiveAnswers(results []archiveQuestionResult) archiveQuestionSummary {
	summary := archiveQuestionSummary{Total: len(results)}
	for _, result := range results {
		if !result.Available {
			summary.Unavailable++
			continue
		}
		switch result.Answer.State {
		case evidence.Observed:
			summary.Observed++
		case evidence.Unknown:
			summary.Unknown++
		}
	}
	return summary
}

func acceptanceRecordMatches(
	record bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary,
	historySummary bundle.ArchiveQuestionTransitionVerificationSummary,
	questionRoundSHA256, questionID string,
	receiptAvailable bool,
	receiptSHA256 string,
) bool {
	return record.TransitionHistorySHA256 == historySummary.TransitionHistorySHA256 &&
		record.QuestionRoundSHA256 == questionRoundSHA256 &&
		record.QuestionID == questionID &&
		receiptAvailable &&
		record.ReceiptSHA256 == receiptSHA256
}

func (h handler) handleRun(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	runDir, ok := h.bundlePath(directory)
	if !ok {
		http.Error(w, "bundle not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	questions := h.questions()
	answers := make([]bundle.Answer, 0, len(questions))
	for _, question := range questions {
		answer, err := h.ask(runDir, question.ID)
		if err != nil {
			answers = nil
			break
		}
		answers = append(answers, answer)
	}
	render(w, pageData{
		View:      "run",
		Title:     "Bundle review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Answers:   answers,
	})
}

func (h handler) handleAsk(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	questionID := r.URL.Query().Get("question_id")
	runDir, ok := h.bundlePath(directory)
	if !ok || questionID == "" {
		http.Error(w, "question not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	answer, err := h.ask(runDir, questionID)
	if err != nil {
		http.Error(w, "question unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:      "ask",
		Title:     "Question review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Answer:    answer,
	})
}

func (h handler) handleFinding(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	findingID := r.URL.Query().Get("finding_id")
	runDir, ok := h.bundlePath(directory)
	if !ok || findingID == "" {
		http.Error(w, "finding not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	finding, err := h.find(runDir, findingID)
	if err != nil {
		http.Error(w, "finding unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:      "finding",
		Title:     "Finding review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Finding:   finding,
	})
}

func (h handler) handleExportAsk(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	questionID := r.URL.Query().Get("question_id")
	if h.exportAsk == nil || questionID == "" {
		http.Error(w, "question not found", http.StatusNotFound)
		return
	}
	questions := []bundle.Question{}
	if h.questions != nil {
		questions = h.questions()
	}
	if _, ok := questionForID(questions, questionID); !ok {
		http.Error(w, "question not found", http.StatusNotFound)
		return
	}
	answer, err := h.exportAsk(h.exportPath, questionID)
	if err != nil {
		http.Error(w, "export question unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:                       "export-ask",
		Title:                      "Portable export question — Ariadne",
		ExportConfigured:           true,
		ExportAnswer:               answer,
		ExportSourceEvidenceSHA256: answer.SourceEvidenceSHA256,
		ExportSHA256:               answer.ExportSHA256,
	})
}

func (h handler) handleExportFinding(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	findingID := r.URL.Query().Get("finding_id")
	if h.exportFind == nil || findingID == "" {
		http.Error(w, "finding not found", http.StatusNotFound)
		return
	}
	finding, err := h.exportFind(h.exportPath, findingID)
	if err != nil {
		http.Error(w, "export finding unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:                       "export-finding",
		Title:                      "Portable export finding — Ariadne",
		ExportConfigured:           true,
		ExportFinding:              finding,
		ExportSourceEvidenceSHA256: finding.SourceEvidenceSHA256,
		ExportSHA256:               finding.ExportSHA256,
	})
}

func (h handler) bundlePath(directory string) (string, bool) {
	if directory == "" {
		return "", false
	}
	entries, err := h.index(h.root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.Directory == directory {
			return filepath.Join(h.root, entry.Directory), true
		}
	}
	return "", false
}

func questionForID(questions []bundle.Question, id string) (bundle.Question, bool) {
	for _, question := range questions {
		if question.ID == id {
			return question, true
		}
	}
	return bundle.Question{}, false
}

func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func render(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
	}
}

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"query": func(value string) template.URL { return template.URL(url.QueryEscape(value)) },
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root { color-scheme: light; --ink: #16202a; --muted: #64717d; --line: #d8e0e5; --paper: #f7f9fa; --card: #fff; --accent: #0b6e69; --accent-soft: #e1f2ef; --warning: #8a5a00; --warning-soft: #fff3d5; }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--paper); color: var(--ink); font: 16px/1.55 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { width: min(980px, calc(100% - 32px)); margin: 0 auto; padding: 28px 0 56px; }
    header { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; padding-bottom: 56px; }
    .brand { color: var(--ink); font-size: 14px; font-weight: 800; letter-spacing: .16em; text-decoration: none; }
    .context, .eyebrow, .directory, .metric-label, footer { color: var(--muted); font-size: 13px; }
    .hero { max-width: 680px; padding-bottom: 42px; }
    h1 { max-width: 760px; margin: 0 0 12px; font-size: clamp(34px, 6vw, 58px); letter-spacing: -.05em; line-height: 1.02; }
    h2 { margin: 0; font-size: 22px; letter-spacing: -.02em; }
    h3 { margin: 4px 0 2px; font-size: 20px; }
    p { margin: 0 0 18px; }
    .lede { color: var(--muted); font-size: 19px; }
    .section-head { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; margin: 0 0 14px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 14px; }
    .card, .panel { border: 1px solid var(--line); border-radius: 16px; background: var(--card); padding: 22px; box-shadow: 0 8px 24px rgba(22,32,42,.04); }
    .card { display: flex; flex-direction: column; min-height: 230px; }
    .card .button { margin-top: auto; }
    .directory { word-break: break-word; }
    .metrics { display: flex; gap: 22px; margin: 22px 0; }
    .metric { display: grid; gap: 2px; }
    .metric-value { font-size: 27px; font-weight: 750; line-height: 1; }
    .button { display: inline-flex; align-items: center; justify-content: space-between; gap: 12px; border: 1px solid var(--accent); border-radius: 10px; color: var(--accent); background: transparent; padding: 10px 13px; font-weight: 700; text-decoration: none; }
    .button:hover { color: #fff; background: var(--accent); }
    .question-links { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 18px; }
    .question-list { display: grid; gap: 10px; margin-top: 18px; }
    .back { display: inline-block; margin-bottom: 26px; color: var(--accent); font-weight: 700; text-decoration: none; }
    .status { display: inline-block; border-radius: 999px; background: var(--accent-soft); color: var(--accent); padding: 5px 11px; font-size: 13px; font-weight: 800; text-transform: uppercase; letter-spacing: .08em; }
    .status-unknown { background: var(--warning-soft); color: var(--warning); }
    .status-unavailable { background: var(--line); color: var(--muted); }
    .question { max-width: 700px; margin: 20px 0 28px; font-size: 25px; letter-spacing: -.02em; }
    dl { display: grid; grid-template-columns: 150px 1fr; gap: 10px 18px; margin: 20px 0 30px; }
    dt { color: var(--muted); font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: .08em; }
    dd { margin: 0; overflow-wrap: anywhere; }
    ul { margin: 12px 0 26px; padding-left: 22px; }
    li { margin: 6px 0; overflow-wrap: anywhere; }
    pre { max-height: 420px; overflow: auto; border: 1px solid var(--line); border-radius: 10px; background: var(--paper); padding: 14px; font: 12px/1.45 ui-monospace, SFMono-Regular, Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
    a.finding { color: var(--accent); font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; }
    .empty { color: var(--muted); border: 1px dashed var(--line); border-radius: 14px; padding: 24px; }
    footer { border-top: 1px solid var(--line); margin-top: 52px; padding-top: 16px; }
    @media (max-width: 560px) { header { display: block; padding-bottom: 38px; } .context { display: block; margin-top: 8px; } dl { grid-template-columns: 1fr; gap: 3px; } dd { margin-bottom: 10px; } }
  </style>
</head>
<body>
<main>
  <header>
    <a class="brand" href="/">ARIADNE</a>
    <span class="context">counterfactual evidence review · read only</span>
  </header>

  {{define "provenance"}}
  {{if or .Summary.Question .Summary.AnswerState .Summary.ManifestContractSHA256 .Summary.AriadneRevision .Summary.RecordedAt}}
  <section class="panel">
    <h2>Verified provenance</h2>
    <dl>
      {{with .Summary.Question}}<dt>question</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.AnswerState}}<dt>answer state</dt><dd><span class="status status-{{.}}">{{.}}</span></dd>{{end}}
      {{with .Summary.ManifestContractSHA256}}<dt>manifest contract</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.RecordedAt}}<dt>recorded (UTC)</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.TargetPackage}}<dt>target package</dt><dd>{{.}}</dd>{{end}}
      {{if .Summary.TargetAndroidAPI}}<dt>Android API</dt><dd>{{.Summary.TargetAndroidAPI}}</dd>{{end}}
      {{with .Summary.TargetArchitecture}}<dt>architecture</dt><dd>{{.}}</dd>{{end}}
      {{if .Summary.TargetPackageVersionCode}}<dt>package version</dt><dd>{{.Summary.TargetPackageVersionCode}}</dd>{{end}}
      {{with .Summary.TargetPackageSHA256}}<dt>package SHA-256</dt><dd>{{.}}</dd>{{end}}
      {{if .Summary.Normalizations}}<dt>normalization</dt><dd><ul>{{range .Summary.Normalizations}}<li>{{.}}</li>{{end}}</ul></dd>{{end}}
      {{with .Summary.AriadneRevision}}<dt>Ariadne revision</dt><dd>{{.}}</dd><dt>working tree</dt><dd>{{if $.Summary.AriadneModified}}modified{{else}}clean{{end}}</dd>{{end}}
    </dl>
    <p class="context">This context is structural metadata only; observations and persona values are not rendered.</p>
  </section>
  {{end}}
  {{end}}

  {{define "export-identity"}}
  {{if or .ExportSourceEvidenceSHA256 .ExportSHA256}}
  <section class="panel">
    <h2>Portable export identity</h2>
    <dl>
      {{with .ExportSourceEvidenceSHA256}}<dt>source evidence SHA-256</dt><dd>{{.}}</dd>{{end}}
      {{with .ExportSHA256}}<dt>export SHA-256</dt><dd>{{.}}</dd>{{end}}
    </dl>
    <p class="context">These identities bind this answer or finding to the verified raw-value-free export. They do not prove the underlying evidence.</p>
  </section>
  {{end}}
  {{end}}

  {{if eq .View "index"}}
    <section class="hero">
      <p class="eyebrow">verified archive</p>
      <h1>Review what changed.</h1>
      <p class="lede">Start from a verified bundle, ask one bounded question, and follow its safe finding references. Captured values never appear here.</p>
    </section>
    <section class="panel">
      <div class="section-head"><h2>Ask across this archive</h2><span class="context">fixed, read only</span></div>
      <p class="context">Choose one bounded question to re-check against every verified bundle.</p>
      <div class="question-links" aria-label="Bounded questions">
      {{range .Questions}}<a class="button" href="/?question_id={{query .ID}}">{{.Text}}</a>{{end}}
      </div>
    </section>
    {{if .ExportConfigured}}
    <section class="panel">
      <div class="section-head"><h2>Portable redacted export</h2><span class="context">verified, offline</span></div>
      <p class="context">Ask the export's fixed counterfactual question and follow safe finding references without opening captured artifacts.</p>
      <a class="button" href="/export-ask?question_id=counterfactual-change">Ask the portable export <span aria-hidden="true">&rarr;</span></a>
    </section>
    {{end}}
    {{if .TraceArchiveConfigured}}
    <section class="panel" id="trace-archive-orientation" aria-label="Portable trace archive">
      <div class="section-head"><h2>Portable trace archive</h2><span class="context">{{if .TraceArchiveRoundSaved}}saved question round{{else}}verified at open{{end}}</span></div>
      <p class="context">Review caller-ordered trace snapshots and their three fixed source-neutral questions. Outcome and evidence state remain separate; captured values are never rendered.</p>
      <a class="button" href="/trace-archive">Open trace archive review <span aria-hidden="true">&rarr;</span></a>
    </section>
    {{end}}
    {{if .TraceReplicationConfigured}}
    <section class="panel" id="trace-replication-orientation" aria-label="Replicated trace ledger">
      <div class="section-head"><h2>Replicated trace ledger</h2><span class="context">verified, read only</span></div>
      <p class="context">Review matched baseline/treatment pairs in their recorded orders. The aggregate outcome and evidence state remain separate; reset assertions do not prove source behavior.</p>
      <a class="button" href="/trace-replication">Open replicated trace review <span aria-hidden="true">&rarr;</span></a>
    </section>
    {{end}}
    {{if and .ReflectionHistoryRequested (or (not .SelectedQuestion.ID) (eq .SelectedQuestion.ID .ReflectionHistory.QuestionID))}}
    <section class="panel">
      <div class="section-head"><h2>Saved reflection history</h2><span class="context">verified ledger</span></div>
      {{if .AcceptanceRecordRequested}}
      <section class="panel" id="history-acceptance-record" aria-label="Portable question acceptance record">
        <div class="section-head"><h3>Portable question acceptance</h3><span class="status">{{.AcceptanceRecordStatus}}</span></div>
        {{if .AcceptanceRecordAvailable}}
        <dl>
          <dt>question ID</dt><dd>{{if and $.ReflectionHistoryRequested $.ReflectionHistoryAvailable (eq $.ReflectionHistorySummary.TransitionHistorySHA256 .AcceptanceRecord.TransitionHistorySHA256)}}<a href="/?history_question_id={{query .AcceptanceRecord.QuestionID}}" aria-label="Ask accepted history question {{.AcceptanceRecord.QuestionID}}"><code>{{.AcceptanceRecord.QuestionID}}</code></a>{{else}}<code>{{.AcceptanceRecord.QuestionID}}</code>{{end}}</dd>
          <dt>history SHA-256</dt><dd>{{.AcceptanceRecord.TransitionHistorySHA256}}</dd>
          <dt>question round SHA-256</dt><dd>{{.AcceptanceRecord.QuestionRoundSHA256}}</dd>
          <dt>receipt SHA-256</dt><dd>{{.AcceptanceRecord.ReceiptSHA256}}</dd>
          <dt>acceptance SHA-256</dt><dd>{{.AcceptanceRecord.AcceptanceSHA256}}</dd>
        </dl>
        {{end}}
        <p class="context">This compares the selected read-only receipt with a saved raw-value-free identity binding. It does not prove that a UI driver performed the selection.</p>
      </section>
      {{end}}
      {{if .ReflectionHistoryAvailable}}
      <p class="context">{{.ReflectionHistory.Question}}</p>
      <div class="section-head"><h3>Question round</h3><span class="context">fixed, verified, read only</span></div>
      <div class="question-list" aria-label="Verified history question round">
      {{range .ReflectionHistoryQuestionRound.Questions}}<a class="button" href="/?history_question_id={{query .QuestionID}}" aria-label="Ask verified history question {{.QuestionID}}"{{if eq $.ReflectionHistoryQuestionID .QuestionID}} aria-current="page"{{end}}><span><code>{{.QuestionID}}</code><br>{{.Question}}</span><span class="status status-{{.Result}}">{{.Result}}</span></a>{{end}}
      </div>
      <dl>
        <dt>order basis</dt><dd>{{.ReflectionHistory.OrderBasis}}</dd>
        <dt>snapshots</dt><dd>{{.ReflectionHistorySummary.Snapshots}}</dd>
        <dt>transitions</dt><dd>{{.ReflectionHistorySummary.Transitions}}</dd>
        <dt>history SHA-256</dt><dd>{{.ReflectionHistorySummary.TransitionHistorySHA256}}</dd>
        <dt>question round SHA-256</dt><dd>{{.ReflectionQuestionRoundSHA256}}</dd>
      </dl>
      {{if .ReflectionHistoryReceiptAvailable}}
      <section class="panel" id="history-answer-receipt-{{.ReflectionHistoryReceipt.QuestionID}}" aria-label="Portable history answer receipt">
        <div class="section-head"><h3>Portable answer receipt</h3><span class="status">raw-value-free</span></div>
        <dl>
          <dt>receipt schema</dt><dd>{{.ReflectionHistoryReceipt.SchemaVersion}}</dd>
          <dt>question ID</dt><dd><code>{{.ReflectionHistoryReceipt.QuestionID}}</code></dd>
          <dt>result</dt><dd><span class="status status-{{.ReflectionHistoryReceipt.Result}}">{{.ReflectionHistoryReceipt.Result}}</span></dd>
          <dt>history SHA-256</dt><dd>{{.ReflectionHistoryReceipt.TransitionHistorySHA256}}</dd>
          <dt>receipt SHA-256</dt><dd>{{.ReflectionHistoryReceiptSHA256}}</dd>
        </dl>
        <pre aria-label="Portable history answer receipt JSON">{{.ReflectionHistoryReceiptJSON}}</pre>
        <p class="context">This receipt binds the selected bounded answer to the verified history identity. It does not infer chronology or prove the underlying evidence.</p>
      </section>
      {{end}}
      {{if or (eq .ReflectionHistoryQuestionID "") (eq .ReflectionHistoryQuestionID .ReflectionHistoryAnswer.QuestionID)}}
      <section id="history-question-{{.ReflectionHistoryAnswer.QuestionID}}">
      <div class="section-head"><h3>History question</h3><span class="status">{{.ReflectionHistoryAnswer.Result}}</span></div>
      <p class="context">{{.ReflectionHistoryAnswer.Question}}</p>
      <p class="context">changed transitions:</p>
      <ul aria-label="Changed history transitions">
      {{range .ReflectionHistoryAnswer.ChangedTransitions}}<li>transition {{.}}</li>{{end}}
      </ul>
      <p class="context">changed entries:</p>
      <ul aria-label="Changed history entries">
      {{range .ReflectionHistoryAnswer.ChangedEntries}}<li>transition {{.Transition}}: {{.Directory}}: {{.OlderState}} &rarr; {{.NewerState}}<br><span class="context">from {{.FromReflectionSHA256}} to {{.ToReflectionSHA256}}</span></li>{{end}}
      </ul>
      <p class="context">incomparable transitions:</p>
      <ul aria-label="Incomparable history transitions">
      {{range .ReflectionHistoryAnswer.IncomparableTransitions}}<li>transition {{.}}</li>{{end}}
      </ul>
      </section>
      {{end}}
      {{if or (eq .ReflectionHistoryQuestionID "") (eq .ReflectionHistoryQuestionID .ReflectionHistoryRepeatedAnswer.QuestionID)}}
      <section id="history-question-{{.ReflectionHistoryRepeatedAnswer.QuestionID}}">
      <div class="section-head"><h3>Repeated-change question</h3><span class="status status-{{.ReflectionHistoryRepeatedAnswer.Result}}">{{.ReflectionHistoryRepeatedAnswer.Result}}</span></div>
      <p class="context">{{.ReflectionHistoryRepeatedAnswer.Question}}</p>
      {{if eq .ReflectionHistoryRepeatedAnswer.Result "unavailable"}}
      <p class="context">Repeated state changes are unavailable for legacy histories without verified state-change details.</p>
      {{else}}
      <p class="context">repeated entries:</p>
      <ul aria-label="Repeated changed history entries">
      {{range .ReflectionHistoryRepeatedAnswer.RepeatedEntries}}<li>{{.Directory}}<ul>{{range .Changes}}<li>transition {{.Transition}}: {{.OlderState}} &rarr; {{.NewerState}}<br><span class="context">from {{.FromReflectionSHA256}} to {{.ToReflectionSHA256}}</span></li>{{end}}</ul></li>{{else}}<li>none</li>{{end}}
      </ul>
      {{end}}
      </section>
      {{end}}
      {{if or (eq .ReflectionHistoryQuestionID "") (eq .ReflectionHistoryQuestionID .ReflectionHistorySnapshotAnswer.QuestionID)}}
      <section id="history-question-{{.ReflectionHistorySnapshotAnswer.QuestionID}}">
      <div class="section-head"><h3>Snapshot-summary question</h3><span class="status status-{{.ReflectionHistorySnapshotAnswer.Result}}">{{.ReflectionHistorySnapshotAnswer.Result}}</span></div>
      <p class="context">{{.ReflectionHistorySnapshotAnswer.Question}}</p>
      {{if eq .ReflectionHistorySnapshotAnswer.Result "unavailable"}}
      <p class="context">Snapshot summaries are unavailable for legacy histories that predate schema 3.</p>
      {{else}}
      <p class="context">safe snapshot summaries:</p>
      <ul aria-label="Saved reflection snapshot summaries">
      {{range .ReflectionHistorySnapshotAnswer.SnapshotSummaries}}<li><span class="context">{{.ReflectionSHA256}}</span>: observed {{.Observed}}, unknown {{.Unknown}}, unavailable {{.Unavailable}}, checked {{.Checked}}</li>{{end}}
      </ul>
      {{end}}
      </section>
      {{end}}
      {{if or (eq .ReflectionHistoryQuestionID "") (eq .ReflectionHistoryQuestionID .ReflectionHistorySummaryAnswer.QuestionID)}}
      <section id="history-question-{{.ReflectionHistorySummaryAnswer.QuestionID}}">
      <div class="section-head"><h3>Snapshot-change question</h3><span class="status status-{{.ReflectionHistorySummaryAnswer.Result}}">{{.ReflectionHistorySummaryAnswer.Result}}</span></div>
      <p class="context">{{.ReflectionHistorySummaryAnswer.Question}}</p>
      {{if eq .ReflectionHistorySummaryAnswer.Result "unavailable"}}
      <p class="context">Snapshot-summary changes are unavailable for legacy histories that predate schema 3.</p>
      {{else}}
      <p class="context">changed summary boundaries:</p>
      <ul aria-label="Changed snapshot summary boundaries">
      {{range .ReflectionHistorySummaryAnswer.ChangedTransitions}}<li>transition {{.}}</li>{{else}}<li>none</li>{{end}}
      </ul>
      {{end}}
      </section>
      {{end}}
      <ul aria-label="Saved reflection transitions">
      {{range .ReflectionHistory.Transitions}}
        <li><span class="status">{{.Result}}</span> compared {{.Compared}}, changed {{.Changed}}, from-only {{.FromOnly}}, to-only {{.ToOnly}}<br><span class="context">from {{.FromReflectionSHA256}} to {{.ToReflectionSHA256}}</span>
        {{if .StateChanges}}<br><span class="context">changed archive entries:</span><ul aria-label="Changed archive entries">{{range .StateChanges}}<li>{{.Directory}}: {{.OlderState}} &rarr; {{.NewerState}}</li>{{end}}</ul>{{end}}</li>
      {{end}}
      </ul>
      <p class="context">The ledger follows caller-supplied order. It records bounded state changes only; it does not establish chronology or infer a trend.</p>
      {{else}}
      <p class="context">Saved reflection history is unavailable. The internal verification error is not rendered.</p>
      {{end}}
    </section>
    {{end}}
    {{if .QuestionRoundComparisonRequested}}
    <section class="panel" id="history-question-round-comparison" aria-label="Retained question round comparison">
      <div class="section-head"><h2>Retained question rounds</h2><span class="context">fixed, read only</span></div>
      {{if .QuestionRoundComparisonAvailable}}
      <p class="context">{{.QuestionRoundComparison.ComparisonQuestion}}</p>
      <dl>
        <dt>result</dt><dd><span class="status status-{{.QuestionRoundComparison.Result}}">{{.QuestionRoundComparison.Result}}</span></dd>
        <dt>order basis</dt><dd>{{.QuestionRoundComparison.OrderBasis}}</dd>
        <dt>first round SHA-256</dt><dd>{{.QuestionRoundComparison.FirstRoundSHA256}}</dd>
        <dt>second round SHA-256</dt><dd>{{.QuestionRoundComparison.SecondRoundSHA256}}</dd>
        <dt>first history SHA-256</dt><dd>{{.QuestionRoundComparison.FirstTransitionHistorySHA256}}</dd>
        <dt>second history SHA-256</dt><dd>{{.QuestionRoundComparison.SecondTransitionHistorySHA256}}</dd>
        <dt>compared</dt><dd>{{.QuestionRoundComparison.Compared}}</dd>
        <dt>changed</dt><dd>{{.QuestionRoundComparison.Changed}}</dd>
      </dl>
      <p class="context">changed fixed questions:</p>
      <ul aria-label="Changed retained questions">
      {{range .QuestionRoundComparison.ChangedQuestions}}<li>{{if and $.ReflectionHistoryRequested $.ReflectionHistoryAvailable (or (eq $.ReflectionHistorySummary.TransitionHistorySHA256 $.QuestionRoundComparison.FirstTransitionHistorySHA256) (eq $.ReflectionHistorySummary.TransitionHistorySHA256 $.QuestionRoundComparison.SecondTransitionHistorySHA256))}}<a href="/?history_question_id={{query .QuestionID}}" aria-label="Ask changed retained question {{.QuestionID}}"><code>{{.QuestionID}}</code></a>{{else}}<code>{{.QuestionID}}</code>{{end}}: {{.FirstResult}} &rarr; {{.SecondResult}}</li>{{else}}<li>none</li>{{end}}
      </ul>
      <p class="context">This compares bounded question results in caller order. It does not establish chronology, infer a trend, or prove the underlying evidence.</p>
      {{else}}
      <p class="context">Retained question round comparison is unavailable. The internal verification error is not rendered.</p>
      {{end}}
    </section>
    {{end}}
    {{if and .SavedReflectionComparisonRequested (or (not .SelectedQuestion.ID) (eq .SelectedQuestion.ID .SavedReflectionComparison.QuestionID))}}
    <section class="panel">
      <div class="section-head"><h2>Saved reflection vs current</h2><span class="context">bounded comparison</span></div>
      {{if .SavedReflectionComparisonAvailable}}
      <p class="context">{{.SavedReflectionComparison.Question}}</p>
      <dl>
        <dt>comparison question</dt><dd>{{.SavedReflectionComparison.ComparisonQuestion}}</dd>
        <dt>result</dt><dd><span class="status">{{.SavedReflectionComparison.Result}}</span></dd>
        <dt>saved reflection SHA-256</dt><dd>{{.SavedReflectionComparison.OlderReflectionSHA256}}</dd>
        <dt>current reflection SHA-256</dt><dd>{{.SavedReflectionComparison.NewerReflectionSHA256}}</dd>
        <dt>compared</dt><dd>{{.SavedReflectionComparison.Compared}}</dd>
        <dt>changed</dt><dd>{{.SavedReflectionComparison.Changed}}</dd>
        <dt>saved-only</dt><dd>{{.SavedReflectionComparison.OlderOnly}}</dd>
        <dt>current-only</dt><dd>{{.SavedReflectionComparison.NewerOnly}}</dd>
      </dl>
      {{if .SavedReflectionComparison.StateChanges}}
      <h3>Changed archive entries</h3>
      <ul>
      {{range .SavedReflectionComparison.StateChanges}}<li>{{.Directory}}: {{.OlderState}} &rarr; {{.NewerState}}</li>{{end}}
      </ul>
      {{end}}
      <p class="context">This compares bounded answer states only. It does not establish chronology, infer a trend, or prove the underlying evidence.</p>
      {{else}}
      <p class="context">Saved reflection comparison is unavailable. The internal verification error is not rendered.</p>
      {{end}}
    </section>
    {{end}}
    {{if .SelectedQuestion.ID}}
    <section>
      <div class="section-head"><h2>Question lens</h2><span class="context">re-verified now, oldest first</span></div>
      <p class="question">{{.SelectedQuestion.Text}}</p>
      <p class="context">Dated results are ordered by verified recording time; undated bundles follow.</p>
      <div class="metrics panel" aria-label="Archive question summary">
        <span class="metric"><strong class="metric-value">{{.ArchiveSummary.Observed}}</strong><span class="metric-label">observed</span></span>
        <span class="metric"><strong class="metric-value">{{.ArchiveSummary.Unknown}}</strong><span class="metric-label">unknown</span></span>
        <span class="metric"><strong class="metric-value">{{.ArchiveSummary.Unavailable}}</strong><span class="metric-label">unavailable</span></span>
        <span class="metric"><strong class="metric-value">{{.ArchiveSummary.Total}}</strong><span class="metric-label">checked</span></span>
      </div>
      {{if .CurrentReflectionRequested}}
      <section class="panel">
        <div class="section-head"><h2>Current reflection</h2><span class="context">derived in memory</span></div>
        {{if .CurrentReflectionAvailable}}
        <dl>
          <dt>reflection SHA-256</dt><dd>{{.CurrentReflectionSHA256}}</dd>
          <dt>bundles checked</dt><dd>{{.CurrentReflectionChecked}}</dd>
        </dl>
        <p class="context">This digest identifies the current raw-value-free answer report. It is not a truth claim, chronology, or trend.</p>
        {{else}}
        <p class="context">The current reflection could not be derived from this archive. Individual result cards remain bounded re-checks.</p>
        {{end}}
      </section>
      {{end}}
      <div class="grid">
      {{range .ArchiveAnswers}}
        <article class="card">
          <p class="eyebrow">bundle</p>
          <h3>{{.ManifestName}}</h3>
          <p class="directory">{{.Directory}}</p>
          {{if .Available}}
            <span class="status status-{{.Answer.State}}">{{.Answer.State}}</span>
            {{with .Answer.Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
            {{template "provenance" .}}
            <a class="button" href="/ask?directory={{query .Directory}}&amp;question_id={{query $.SelectedQuestion.ID}}">Open answer details <span aria-hidden="true">→</span></a>
          {{else}}
            <span class="status status-unavailable">unavailable</span>
            <p class="context">This bundle does not support the current bounded question.</p>
          {{end}}
        </article>
      {{end}}
      </div>
    </section>
    {{end}}
    <section>
      <div class="section-head"><h2>Archived bundles</h2><span class="context">{{len .Entries}} available</span></div>
      {{if .Entries}}
        <div class="grid">
        {{range .Entries}}
          <article class="card">
            <p class="eyebrow">manifest</p>
            <h3>{{.ManifestName}}</h3>
            <p class="directory">{{.Directory}}</p>
            <div class="metrics">
              <span class="metric"><strong class="metric-value">{{.Differences}}</strong><span class="metric-label">differences</span></span>
              <span class="metric"><strong class="metric-value">{{.Unknowns}}</strong><span class="metric-label">unknowns</span></span>
            </div>
            <a class="button" href="/run?directory={{query .Directory}}">Open review <span aria-hidden="true">→</span></a>
          </article>
        {{end}}
        </div>
      {{else}}
        <p class="empty">No verified bundles are available in this archive root.</p>
      {{end}}

  {{else if eq .View "trace-archive"}}
    <a class="back" href="/">&larr; Review archive</a>
    <p class="eyebrow">portable trace archive &middot; verified</p>
    <h1>Trace archive reflection</h1>
    <p class="lede">Ask the same fixed questions across a caller-ordered sequence of standalone trace snapshots. This is a review surface for safe categories, not a chronology or raw-payload viewer.</p>
    <section class="panel" aria-label="Verified trace archive identity">
      <div class="section-head"><h2>Verified archive identity</h2><span class="status">raw-value-free</span></div>
      <dl>
        <dt>order basis</dt><dd>{{.TraceArchiveSummary.OrderBasis}}</dd>
        <dt>entries</dt><dd>{{.TraceArchiveSummary.Entries}}</dd>
        <dt>complete</dt><dd>{{.TraceArchiveSummary.Complete}}</dd>
        <dt>partial</dt><dd>{{.TraceArchiveSummary.Partial}}</dd>
        <dt>archive SHA-256</dt><dd>{{.TraceArchiveSummary.ArchiveSHA256}}</dd>
        <dt>question round SHA-256</dt><dd>{{.TraceArchiveRoundSHA256}}</dd>
      </dl>
      <h3>Reviewed source adapters</h3>
      <ul aria-label="Reviewed trace sources">
      {{range .TraceArchiveSummary.Sources}}<li>{{.Source}} / {{.Adapter}}: {{.Entries}} entries</li>{{else}}<li>none</li>{{end}}
      </ul>
      <p class="context">{{if .TraceArchiveRoundSaved}}This saved question round can be re-verified without reopening the source archive.{{else}}This question round was derived in memory from the verified archive.{{end}} The identities do not prove the underlying source or infer chronology.</p>
    </section>
    <section class="panel" aria-label="Trace archive question round">
      <div class="section-head"><h2>Fixed trace questions</h2><span class="context">caller-ordered, read only</span></div>
      <div class="question-list">
      {{range .TraceArchiveAnswers}}
        <article class="panel" id="trace-question-{{.QuestionID}}">
          <div class="section-head"><h3><code>{{.QuestionID}}</code></h3><span class="status status-{{.Result}}">{{.Result}}</span></div>
          <p class="question">{{.Question}}</p>
          <dl>
            <dt>outcome</dt><dd><span class="status status-{{.Result}}">{{.Result}}</span></dd>
            <dt>evidence state</dt><dd><span class="status status-{{.EvidenceState}}">{{.EvidenceState}}</span></dd>
            <dt>archive SHA-256</dt><dd>{{.ArchiveSHA256}}</dd>
            <dt>entries</dt><dd>{{.Entries}}</dd>
            <dt>compared</dt><dd>{{.Compared}}</dd>
            <dt>changed</dt><dd>{{.Changed}}</dd>
            <dt>same</dt><dd>{{.Same}}</dd>
            <dt>unknown</dt><dd>{{.Unknown}}</dd>
          </dl>
          {{with .Reason}}<p class="context">{{.}}</p>{{end}}
        </article>
      {{end}}
      </div>
      <p class="context">The outcome answers the bounded question; evidence state qualifies the support available for that answer. A result of <code>unknown</code> is not treated as no change.</p>
    </section>

  {{else if eq .View "trace-replication"}}
    <a class="back" href="/">&larr; Review archive</a>
    <p class="eyebrow">source-neutral replicated trace ledger &middot; verified</p>
    <h1>Replicated trace reflection</h1>
    <p class="lede">Review already-produced matched pairs across both explicit orders. This is a bounded aggregate of safe trace comparisons, not a runner, chronology model, or causal proof.</p>
    <section class="panel" aria-label="Verified replicated trace ledger identity">
      <div class="section-head"><h2>Verified replication identity</h2><span class="status status-raw-value-free">raw-value-free</span></div>
      <dl>
        <dt>pairs</dt><dd>{{.TraceReplicationSummary.Pairs}}</dd>
        <dt>baseline &rarr; treatment</dt><dd>{{.TraceReplicationSummary.BaselineTreatmentPairs}}</dd>
        <dt>treatment &rarr; baseline</dt><dd>{{.TraceReplicationSummary.TreatmentBaselinePairs}}</dd>
        <dt>reset-confirmed pairs</dt><dd>{{.TraceReplicationSummary.ResetConfirmedPairs}}</dd>
        <dt>complete pairs</dt><dd>{{.TraceReplicationSummary.CompletePairs}}</dd>
        <dt>order balanced</dt><dd>{{.TraceReplicationSummary.OrderBalanced}}</dd>
        <dt>outcome</dt><dd><span class="status status-{{.TraceReplicationSummary.Outcome}}">{{.TraceReplicationSummary.Outcome}}</span></dd>
        <dt>evidence state</dt><dd><span class="status status-{{.TraceReplicationSummary.EvidenceState}}">{{.TraceReplicationSummary.EvidenceState}}</span></dd>
        <dt>ledger SHA-256</dt><dd>{{.TraceReplicationSummary.LedgerSHA256}}</dd>
      </dl>
      <p class="context">{{.TraceReplicationSummary.Reason}} Reset policy: <code>{{.TraceReplicationSummary.ResetPolicy}}</code>. The recorded reset is a caller assertion, not proof that a source was reset.</p>
    </section>
    <section class="panel" aria-label="Replicated trace questions">
      <div class="section-head"><h2>Fixed questions</h2><span class="context">re-verified now</span></div>
      <div class="question-list">
      {{range .TraceReplicationAnswers}}
        <article class="panel question-card" id="trace-replication-question-{{.QuestionID}}">
          <div class="section-head"><h3>{{.Question}}</h3><span class="status status-{{.EvidenceState}}">{{.Result}}</span></div>
          <dl>
            <dt>outcome</dt><dd>{{.Outcome}}</dd>
            <dt>evidence state</dt><dd>{{.EvidenceState}}</dd>
          </dl>
          {{with .Reason}}<p class="context">{{.}}</p>{{end}}
        </article>
      {{end}}
      </div>
      <p class="context">Question results remain separate from evidence state. The board is read-only and exposes no source paths or captured values.</p>
    </section>
    <section class="panel" aria-label="Replicated trace pairs">
      <div class="section-head"><h2>Matched pairs</h2><span class="context">caller-recorded order</span></div>
      <div class="question-list">
      {{range .TraceReplicationPairs}}
        <article class="panel" id="trace-replication-pair-{{.Position}}">
          <div class="section-head"><h3>pair {{.Position}}</h3><span class="status status-{{if eq .EvidenceState "unknown"}}unknown{{else if .Differences}}changed{{else}}same{{end}}">{{if eq .EvidenceState "unknown"}}unknown{{else if .Differences}}changed{{else}}same{{end}}</span></div>
          <dl>
            <dt>order</dt><dd>{{.Order}}</dd>
            <dt>reset confirmed</dt><dd>{{.ResetConfirmed}}</dd>
            <dt>pair SHA-256</dt><dd>{{.PairSHA256}}</dd>
            <dt>baseline completeness</dt><dd>{{.BaselineCompleteness}}</dd>
            <dt>treatment completeness</dt><dd>{{.TreatmentCompleteness}}</dd>
            <dt>differences</dt><dd>{{.Differences}}</dd>
            <dt>unknowns</dt><dd>{{.Unknowns}}</dd>
            <dt>evidence state</dt><dd><span class="status status-{{.EvidenceState}}">{{.EvidenceState}}</span></dd>
          </dl>
        </article>
      {{end}}
      </div>
      <p class="context">Each pair comparison is recomputed from the embedded normalized traces during verification. No source paths, payloads, URLs, or captured values are rendered.</p>
    </section>

  {{else if eq .View "run"}}
    <a class="back" href="/">← All bundles</a>
    <p class="eyebrow">{{.Summary.ManifestName}} · verified</p>
    <h1>{{.Directory}}</h1>
    <div class="metrics panel">
      <span class="metric"><strong class="metric-value">{{.Summary.Differences}}</strong><span class="metric-label">differences</span></span>
      <span class="metric"><strong class="metric-value">{{.Summary.Unknowns}}</strong><span class="metric-label">unknowns</span></span>
    </div>
    {{template "provenance" .}}
    {{if .Answers}}
    <section>
      <div class="section-head"><h2>Bounded question board</h2><span class="context">re-verified now</span></div>
      <div class="question-list">
      {{range .Answers}}
        <article class="panel question-card">
          <div class="section-head"><h3>{{.Question}}</h3><span class="status status-{{.State}}">{{.State}}</span></div>
          {{with .Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
          {{if .FindingIDs}}
          <p class="context">Referenced findings</p>
          <ul>
          {{range .FindingIDs}}<li><a class="finding" href="/finding?directory={{query $.Directory}}&amp;finding_id={{query .}}">{{.}}</a></li>{{end}}
          </ul>
          {{else}}<p class="context">No finding references were produced for this question.</p>{{end}}
          <a class="button" href="/ask?directory={{query $.Directory}}&amp;question_id={{query .QuestionID}}">Open answer details <span aria-hidden="true">→</span></a>
        </article>
      {{end}}
      </div>
    </section>
    {{else}}
    <section class="panel">
      <h2>Bounded questions unavailable</h2>
      <p class="context">This verified bundle does not support the current question catalog. Its safe summary remains available.</p>
    </section>
    {{end}}

  {{else if eq .View "ask"}}
    <a class="back" href="/run?directory={{query .Directory}}">← Bundle review</a>
    <p class="eyebrow">bounded question · {{.Directory}}</p>
    <h1>Question result</h1>
    <p class="question">{{.Answer.Question}}</p>
    <span class="status status-{{.Answer.State}}">{{.Answer.State}}</span>
    {{with .Answer.Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
    {{template "provenance" .}}
    <section class="panel" style="margin-top: 28px">
      <h2>Referenced findings</h2>
      {{if .Answer.FindingIDs}}
        <ul>
        {{range .Answer.FindingIDs}}
          <li><a class="finding" href="/finding?directory={{query $.Directory}}&amp;finding_id={{query .}}">{{.}}</a></li>
        {{end}}
        </ul>
      {{else}}
        <p class="context">No finding references were produced for this question.</p>
      {{end}}
    </section>

  {{else if eq .View "finding"}}
    <a class="back" href="/run?directory={{query .Directory}}">← Bundle review</a>
    <p class="eyebrow">{{.Finding.Kind}} · {{.Finding.State}} · {{.Directory}}</p>
    <h1>Finding detail</h1>
    <p class="lede">This is a verified, raw-value-free finding reference.</p>
    {{template "provenance" .}}
    <section class="panel">
      <dl>
        <dt>id</dt><dd>{{.Finding.ID}}</dd>
        <dt>field</dt><dd>{{.Finding.Field}}</dd>
        {{with .Finding.Classification}}<dt>classification</dt><dd>{{.}}</dd>{{end}}
        <dt>answer state</dt><dd>{{.Finding.AnswerState}}</dd>
        <dt>finding state</dt><dd>{{.Finding.State}}</dd>
        {{with .Finding.Reason}}<dt>reason</dt><dd>{{.}}</dd>{{end}}
      </dl>
      <h2>Evidence sources</h2>
      <ul>
      {{range .Finding.Evidence}}<li>{{.}}</li>{{end}}
      </ul>
      <p class="context">Observed values and captured payloads are intentionally not rendered.</p>
    </section>

  {{else if eq .View "export-ask"}}
    <a class="back" href="/">&larr; Review archive</a>
    <p class="eyebrow">portable redacted export &middot; verified</p>
    <h1>Question result</h1>
    <p class="question">{{.ExportAnswer.Question}}</p>
    <span class="status status-{{.ExportAnswer.State}}">{{.ExportAnswer.State}}</span>
    {{with .ExportAnswer.Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
    {{template "export-identity" .}}
    <section class="panel" style="margin-top: 28px">
      <h2>Referenced findings</h2>
      {{if .ExportAnswer.FindingIDs}}
        <ul>
        {{range .ExportAnswer.FindingIDs}}
          <li><a class="finding" href="/export-finding?finding_id={{query .}}">{{.}}</a></li>
        {{end}}
        </ul>
      {{else}}
        <p class="context">No finding references were produced for this question.</p>
      {{end}}
    </section>

  {{else if eq .View "export-finding"}}
    <a class="back" href="/export-ask?question_id=counterfactual-change">&larr; Export question</a>
    <p class="eyebrow">{{.ExportFinding.Kind}} &middot; {{.ExportFinding.State}} &middot; portable export</p>
    <h1>Finding detail</h1>
    <p class="lede">This is a verified, raw-value-free finding reference from a portable export.</p>
    {{template "export-identity" .}}
    <section class="panel">
      <dl>
        <dt>id</dt><dd>{{.ExportFinding.ID}}</dd>
        <dt>field</dt><dd>{{.ExportFinding.Field}}</dd>
        {{with .ExportFinding.Classification}}<dt>classification</dt><dd>{{.}}</dd>{{end}}
        <dt>answer state</dt><dd>{{.ExportFinding.AnswerState}}</dd>
        <dt>finding state</dt><dd>{{.ExportFinding.State}}</dd>
        {{with .ExportFinding.Reason}}<dt>reason</dt><dd>{{.}}</dd>{{end}}
      </dl>
      <h2>Evidence sources</h2>
      <ul>
      {{range .ExportFinding.Evidence}}<li>{{.}}</li>{{end}}
      </ul>
      <p class="context">Observed values and captured payloads are intentionally not rendered.</p>
    </section>
  {{end}}

  <footer>Read-only review surface. Raw observations are never rendered.</footer>
</main>
</body>
</html>`))
