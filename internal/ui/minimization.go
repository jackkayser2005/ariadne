package ui

import (
	"net"
	"net/http"
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
	render(w, pageData{
		View:                   "minimization",
		Title:                  "Minimization review — Ariadne",
		MinimizationConfigured: true,
		Minimization:           minimizationReview(summary, receiptSHA256),
	})
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
