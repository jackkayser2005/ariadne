package ui

import (
	"errors"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
	"github.com/jackkayser2005/ariadne/internal/minimize"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

// ReviewOptions configures the optional, read-only artifacts exposed by the
// loopback review server.
type ReviewOptions struct {
	ArchiveRoot               string
	HistoryPath               string
	ReflectionPath            string
	ExportPath                string
	AcceptancePath            string
	FirstRoundPath            string
	SecondRoundPath           string
	TraceArchivePath          string
	TraceRoundPath            string
	TraceReplicationPath      string
	TraceCasePath             string
	TraceStudyPath            string
	TraceStudyRoundPath       string
	TraceStudyReceiptPath     string
	TraceStudySecondPath      string
	TraceStudyRoundSecondPath string
	MinimizationPath          string
	MinimizationRoundPath     string
	MinimizationReceiptPath   string
	ExpectedHost              string
}

func reviewHandler(options ReviewOptions) http.Handler {
	h := archiveHandler(options.ArchiveRoot)
	if options.HistoryPath != "" {
		h.history = func() (bundle.ArchiveQuestionTransitionHistory, bundle.ArchiveQuestionTransitionVerificationSummary, error) {
			return bundle.ReadArchiveQuestionTransitionHistory(options.HistoryPath)
		}
	}
	if options.ReflectionPath != "" {
		h.compareCurrent = func() (bundle.ArchiveQuestionComparison, error) {
			return bundle.CompareArchiveQuestionReportWithArchive(options.ReflectionPath, options.ArchiveRoot)
		}
	}
	if options.ExportPath != "" {
		h.exportPath = options.ExportPath
		h.exportAsk = bundle.AskExport
		h.exportFind = bundle.FindExport
	}
	if options.AcceptancePath != "" {
		h.acceptance = func() (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error) {
			return bundle.VerifyArchiveQuestionTransitionHistoryAcceptanceRecord(options.AcceptancePath)
		}
	}
	if options.FirstRoundPath != "" && options.SecondRoundPath != "" {
		h.compareRounds = func() (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error) {
			return bundle.CompareArchiveQuestionTransitionHistoryQuestionRounds(options.FirstRoundPath, options.SecondRoundPath)
		}
	}
	if options.TraceArchivePath != "" {
		h.traceArchivePath = options.TraceArchivePath
		h.traceArchiveRead = trace.ReadArchive
	}
	if options.TraceRoundPath != "" {
		h.traceRoundPath = options.TraceRoundPath
		h.traceRoundRead = trace.ReadArchiveQuestionRound
	}
	if options.TraceReplicationPath != "" {
		h.traceReplicationPath = options.TraceReplicationPath
		h.traceReplicationRead = trace.ReadReplicationLedger
	}
	if options.TraceCasePath != "" {
		h.traceCasePath = options.TraceCasePath
		h.traceCaseRead = trace.ReadCase
	}
	if options.TraceStudyPath != "" {
		h.traceStudyPath = options.TraceStudyPath
		h.traceStudyRead = trace.ReadReplicationStudy
	}
	if options.TraceStudyRoundPath != "" {
		h.traceStudyRoundPath = options.TraceStudyRoundPath
		h.traceStudyRoundRead = trace.ReadReplicationStudyQuestionRound
	}
	if options.TraceStudyReceiptPath != "" {
		h.traceStudyReceiptPath = options.TraceStudyReceiptPath
		h.traceStudyReceiptRead = trace.ReadReplicationStudyQuestionReceipt
	}
	if options.TraceStudyPath != "" && options.TraceStudyRoundPath != "" && options.TraceStudySecondPath != "" && options.TraceStudyRoundSecondPath != "" {
		h.traceStudyComparison = func() (trace.ReplicationStudyQuestionRoundComparison, error) {
			return trace.CompareReplicationStudyQuestionRounds(options.TraceStudyPath, options.TraceStudyRoundPath, options.TraceStudySecondPath, options.TraceStudyRoundSecondPath)
		}
	}
	if options.MinimizationPath != "" {
		h.minimizationPath = options.MinimizationPath
		h.minimizationVerify = minimize.VerifyWithIdentity
	}
	if options.MinimizationRoundPath != "" {
		h.minimizationRoundPath = options.MinimizationRoundPath
		h.minimizationRoundRead = minimize.ReadMinimizationQuestionRound
	}
	if options.MinimizationReceiptPath != "" {
		h.minimizationReceiptPath = options.MinimizationReceiptPath
		h.minimizationReceiptRead = minimize.ReadMinimizationQuestionReceipt
	}
	return newHandlerWithHost(h, options.ExpectedHost)
}

// HandlerWithReviewOptions returns a read-only review handler configured from
// one options value. Existing constructor helpers remain available for callers
// that prefer their compatibility-oriented signatures.
func HandlerWithReviewOptions(options ReviewOptions) http.Handler {
	return reviewHandler(options)
}

type minimizationReviewData struct {
	SchemaVersion          int
	ReceiptSHA256          string
	PlanName               string
	Variable               string
	ReferenceCandidate     string
	FunctionalityCriterion string
	PairsPerOrder          int
	EvidenceState          evidence.State
	SelectionState         minimize.SelectionState
	SelectedCandidate      string
	Candidates             []minimizationCandidateData
	Questions              []minimize.MinimizationQuestionAnswer
	RoundSaved             bool
	RoundSummary           minimize.MinimizationQuestionRoundVerificationSummary
	ReceiptAvailable       bool
	ReceiptSaved           bool
	ReceiptSummary         minimize.MinimizationQuestionReceiptVerificationSummary
	SelectedQuestionID     string
}

type minimizationCandidateData struct {
	ID             string
	Classification minimize.CandidateClassification
	Outcome        bundle.ReplicatedOutcome
	EvidenceState  evidence.State
	ReceiptSHA256  string
	Pairs          int
	PairsPerOrder  int
	CompletedPairs int
	ChangedPairs   int
	NoChangePairs  int
	UnknownPairs   int
}

func minimizationReview(summary minimize.MinimizationSummary, receiptSHA256 string) minimizationReviewData {
	candidates := make([]minimizationCandidateData, 0, len(summary.CandidateResults))
	for _, result := range summary.CandidateResults {
		candidates = append(candidates, minimizationCandidateData{
			ID:             result.ID,
			Classification: result.Classification,
			Outcome:        result.Outcome,
			EvidenceState:  result.EvidenceState,
			ReceiptSHA256:  result.ReceiptSHA256,
			Pairs:          result.Pairs,
			PairsPerOrder:  result.PairsPerOrder,
			CompletedPairs: result.CompletedPairs,
			ChangedPairs:   result.ChangedPairs,
			NoChangePairs:  result.NoChangePairs,
			UnknownPairs:   result.UnknownPairs,
		})
	}
	return minimizationReviewData{
		SchemaVersion:          summary.SchemaVersion,
		ReceiptSHA256:          receiptSHA256,
		PlanName:               summary.PlanName,
		Variable:               summary.Variable,
		ReferenceCandidate:     summary.ReferenceCandidate,
		FunctionalityCriterion: summary.FunctionalityCriterion,
		PairsPerOrder:          summary.PairsPerOrder,
		EvidenceState:          summary.EvidenceState,
		SelectionState:         summary.SelectionState,
		SelectedCandidate:      summary.SelectedCandidate,
		Candidates:             candidates,
	}
}

func (h handler) handleMinimization(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/minimization" || h.minimizationPath == "" {
		http.NotFound(w, r)
		return
	}
	if h.minimizationVerify == nil {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	summary, receiptSHA256, err := h.minimizationVerify(h.minimizationPath)
	if err != nil {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	answers, err := minimize.AnswerAllMinimizationQuestions(summary, receiptSHA256)
	if err != nil {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	round, err := minimize.AnswerMinimizationQuestionRound(summary, receiptSHA256)
	if err != nil {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	roundSHA256, err := minimize.MinimizationQuestionRoundSHA256(round)
	if err != nil {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	roundSummary := minimize.MinimizationQuestionRoundVerificationSummary{
		SchemaVersion:      round.SchemaVersion,
		MinimizationSHA256: round.MinimizationSHA256,
		Questions:          len(round.Answers),
		Candidates:         len(round.Candidates),
		RoundSHA256:        roundSHA256,
	}
	if h.minimizationRoundPath != "" {
		if h.minimizationRoundRead == nil {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		savedRound, savedRoundSummary, readErr := h.minimizationRoundRead(h.minimizationRoundPath)
		if readErr != nil ||
			savedRound.MinimizationSHA256 != receiptSHA256 ||
			savedRoundSummary.MinimizationSHA256 != receiptSHA256 ||
			!slices.Equal(savedRound.Answers, round.Answers) ||
			!slices.Equal(savedRound.Candidates, round.Candidates) {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		round = savedRound
		roundSummary = savedRoundSummary
	}
	requestedQuestionID := r.URL.Query().Get("question_id")
	selectedQuestionID := selectedMinimizationQuestionID(requestedQuestionID)
	if requestedQuestionID != "" && selectedQuestionID == "" {
		http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
		return
	}
	receiptSummary := minimize.MinimizationQuestionReceiptVerificationSummary{}
	receiptAvailable := false
	if h.minimizationReceiptPath != "" {
		if h.minimizationReceiptRead == nil {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		receipt, savedReceiptSummary, readErr := h.minimizationReceiptRead(h.minimizationReceiptPath)
		if readErr != nil ||
			receipt.MinimizationSHA256 != receiptSHA256 ||
			receipt.RoundSHA256 != roundSummary.RoundSHA256 ||
			(selectedQuestionID != "" && selectedQuestionID != receipt.QuestionID) {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		selectedQuestionID = receipt.QuestionID
		receiptSummary = savedReceiptSummary
		receiptAvailable = true
	} else if selectedQuestionID != "" {
		answer, answerErr := minimizationAnswerFromRound(round, selectedQuestionID)
		if answerErr != nil {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		receipt := minimize.MinimizationQuestionReceipt{
			MinimizationQuestionAnswer: answer,
			RoundSHA256:                roundSummary.RoundSHA256,
			Round:                      round,
		}
		questionReceiptSHA256, receiptErr := minimize.MinimizationQuestionReceiptSHA256(receipt)
		if receiptErr != nil {
			http.Error(w, "minimization unavailable", http.StatusUnprocessableEntity)
			return
		}
		receiptSummary = minimize.MinimizationQuestionReceiptVerificationSummary{
			SchemaVersion:       receipt.SchemaVersion,
			QuestionID:          receipt.QuestionID,
			Question:            receipt.Question,
			Result:              receipt.Result,
			EvidenceState:       receipt.EvidenceState,
			MinimizationSHA256:  receipt.MinimizationSHA256,
			RoundSHA256:         receipt.RoundSHA256,
			ReceiptSHA256:       questionReceiptSHA256,
			SelectionState:      receipt.SelectionState,
			SelectedCandidate:   receipt.SelectedCandidate,
			CandidateCount:      receipt.CandidateCount,
			SupportedCandidates: receipt.SupportedCandidates,
			UnknownCandidates:   receipt.UnknownCandidates,
		}
		receiptAvailable = true
	}
	review := minimizationReview(summary, receiptSHA256)
	review.Questions = answers
	review.RoundSaved = h.minimizationRoundPath != ""
	review.RoundSummary = roundSummary
	review.ReceiptAvailable = receiptAvailable
	review.ReceiptSaved = h.minimizationReceiptPath != ""
	review.ReceiptSummary = receiptSummary
	review.SelectedQuestionID = selectedQuestionID
	render(w, pageData{
		View:                   "minimization",
		Title:                  "Minimization review — Ariadne",
		MinimizationConfigured: true,
		Minimization:           review,
	})
}

func selectedMinimizationQuestionID(value string) string {
	for _, question := range minimize.MinimizationQuestions() {
		if question.ID == value {
			return value
		}
	}
	return ""
}

func minimizationAnswerFromRound(round minimize.MinimizationQuestionRound, questionID string) (minimize.MinimizationQuestionAnswer, error) {
	for index, question := range minimize.MinimizationQuestions() {
		if question.ID == questionID {
			return round.Answers[index], nil
		}
	}
	return minimize.MinimizationQuestionAnswer{}, errors.New("minimization question ID is invalid")
}
func secureReviewHandler(next http.Handler, expectedHost string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if expectedHost != "" && !reviewHostMatches(expectedHost, r.Host) {
			http.Error(w, "host not allowed", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func reviewHostMatches(expected, actual string) bool {
	expectedIP, expectedPort, expectedOK := reviewAuthority(expected)
	actualIP, actualPort, actualOK := reviewAuthority(actual)
	return expectedOK && actualOK && expectedPort == actualPort && expectedIP.Equal(actualIP)
}

func reviewAuthority(value string) (net.IP, int, bool) {
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		trimmed := strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
		ip := net.ParseIP(trimmed)
		if ip == nil {
			return nil, 0, false
		}
		return ip, 80, true
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 || strconv.Itoa(portNumber) != port {
		return nil, 0, false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, 0, false
	}
	return ip, portNumber, true
}
