package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/experiment"
	"github.com/jackkayser2005/ariadne/internal/trace"
	"github.com/jackkayser2005/ariadne/internal/ui"
)

const usage = `usage:
  ariadne validate <manifest.json>
  ariadne android check [--adb <path>] --device <serial> --package <package>
	ariadne trace verify [--json] [--expect-sha256 <digest>] <trace.json>
	ariadne trace compare [--json] <baseline-trace.json> <treatment-trace.json>
	ariadne trace session create [--json] --adapter <adapter> --procedure-sha256 <digest> [--adapter-version <n>] <trace.json> <session.json>
	ariadne trace session verify [--json] [--expect-sha256 <digest>] <session.json> <trace.json>
	ariadne trace session pair create [--json] --adapter <adapter> --procedure-sha256 <digest> [--adapter-version <n>] --order <baseline-treatment|treatment-baseline> <baseline-trace.json> <treatment-trace.json> <baseline-session.json> <treatment-session.json>
	ariadne trace session pair verify [--json] <baseline-session.json> <baseline-trace.json> <treatment-session.json> <treatment-trace.json>
	ariadne trace session pair compare [--json] <baseline-session.json> <baseline-trace.json> <treatment-session.json> <treatment-trace.json>
	ariadne browser trace [--json] <redacted-browser-audit.json> <trace.json>
	ariadne browser capture [--json] --procedure <procedure.json> --driver <executable> [--driver-arg <arg>] <trace.json>
	ariadne experiment run [--adb <path>] --device <serial> --package <package> --output <directory> <manifest.json>
	ariadne experiment replicate [--adb <path>] --device <serial> --package <package> --pairs <n> --output <directory> <manifest.json>
	ariadne experiment replicate verify [--json] <replicated-directory>
	ariadne experiment trace [--json] --session <baseline|treatment> <run-directory> <trace.json>
  ariadne experiment report <run-directory>
  ariadne experiment export <run-directory> <export.json>
  ariadne experiment export verify [--json] [--expect-sha256 <digest>] <export.json>
  ariadne experiment export ask [--json] <export.json> <question-id>
  ariadne experiment export finding [--json] <export.json> <finding-id>
  ariadne experiment verify [--json] <run-directory>
  ariadne experiment finding [--json] <run-directory> <finding-id>
  ariadne experiment ask [--json] <run-directory> <question-id>
  ariadne experiment ask-archive [--json] <archive-root> <question-id>
  ariadne experiment ask-archive save [--json] <archive-root> <question-id> <report.json>
  ariadne experiment ask-archive compare [--json] <older-report.json> <newer-report.json>
  ariadne experiment ask-archive compare-current [--json] <older-report.json> <archive-root>
  ariadne experiment ask-archive transitions [--json] <report-1.json> <report-2.json> ...
  ariadne experiment ask-archive transitions questions [--json]
  ariadne experiment ask-archive transitions ask [--json] <history.json> [<question-id>]
  ariadne experiment ask-archive transitions ask repeated [--json] <history.json>
  ariadne experiment ask-archive transitions ask all [--json] <history.json>
  ariadne experiment ask-archive transitions ask all save [--json] <history.json> <round.json>
  ariadne experiment ask-archive transitions ask all verify [--json] [--expect-sha256 <digest>] <round.json>
  ariadne experiment ask-archive transitions ask all compare [--json] <first-round.json> <second-round.json>
  ariadne experiment ask-archive transitions acceptance save [--json] <round.json> <receipt.json> <acceptance.json>
  ariadne experiment ask-archive transitions acceptance verify [--json] [--expect-sha256 <digest>] <acceptance.json>
  ariadne experiment ask-archive transitions ask receipt [--json] <history.json> <question-id>
  ariadne experiment ask-archive transitions ask receipt save [--json] <history.json> <question-id> <receipt.json>
  ariadne experiment ask-archive transitions ask receipt verify [--json] [--expect-sha256 <digest>] <receipt.json>
  ariadne experiment ask-archive transitions save [--json] <report-1.json> <report-2.json> ... <history.json>
  ariadne experiment ask-archive transitions verify [--json] [--expect-sha256 <digest>] <history.json>
  ariadne experiment ask-archive verify [--json] [--expect-sha256 <digest>] <report.json>
  ariadne experiment questions [--json]
  ariadne experiment list [--json] <archive-root>
  ariadne experiment serve [--addr <address>] [--history <history.json>] [--reflection <report.json>] [--export <export.json>] [--acceptance <acceptance.json>] [--round-first <round.json> --round-second <round.json>] <archive-root>
`

const adbCheckTimeout = 10 * time.Second
const experimentRunTimeout = 60 * time.Second
const experimentReplicateTimeout = 16 * experimentRunTimeout

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[0] == "validate" {
		return runValidate(args[1], stdout, stderr)
	}
	if len(args) >= 2 && args[0] == "android" && args[1] == "check" {
		return runAndroidCheck(args[2:], stdout, stderr, adb.Check)
	}
	if len(args) >= 2 && args[0] == "trace" && args[1] == "verify" {
		return runTraceVerify(args[2:], stdout, stderr, trace.Verify)
	}
	if len(args) >= 2 && args[0] == "trace" && args[1] == "compare" {
		return runTraceCompare(args[2:], stdout, stderr, trace.CompareFiles)
	}
	if len(args) >= 3 && args[0] == "trace" && args[1] == "session" {
		if len(args) >= 4 && args[2] == "pair" {
			if args[3] == "create" {
				return runTraceSessionPairCreate(args[4:], stdout, stderr, trace.SaveSessionPair)
			}
			if args[3] == "verify" {
				return runTraceSessionPairVerify(args[4:], stdout, stderr, trace.VerifySessionPair)
			}
			if args[3] == "compare" {
				return runTraceSessionPairCompare(args[4:], stdout, stderr, trace.CompareSessionPair)
			}
		}
		if args[2] == "create" {
			return runTraceSessionCreate(args[3:], stdout, stderr, trace.SaveSession)
		}
		if args[2] == "verify" {
			return runTraceSessionVerify(args[3:], stdout, stderr, trace.VerifySession)
		}
	}
	if len(args) >= 2 && args[0] == "browser" && args[1] == "trace" {
		return runBrowserTrace(args[2:], stdout, stderr, browser.SaveTrace)
	}
	if len(args) >= 2 && args[0] == "browser" && args[1] == "capture" {
		return runBrowserCapture(args[2:], stdout, stderr, func(procedurePath, driverPath string, driverArgs []string, outputPath string) (browser.CaptureSummary, error) {
			return browser.Capture(procedurePath, driverPath, driverArgs, outputPath)
		})
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "run" {
		return runExperiment(args[2:], stdout, stderr, adb.Check, adb.RunPair)
	}
	if len(args) >= 3 && args[0] == "experiment" && args[1] == "replicate" && args[2] == "verify" {
		return runReplicateVerify(args[3:], stdout, stderr, bundle.VerifyReplicated)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "replicate" {
		return runExperimentReplicate(args[2:], stdout, stderr, adb.Check, adb.RunReplicated)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "trace" {
		return runExperimentTrace(args[2:], stdout, stderr, bundle.SaveExperimentTrace)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "report" {
		return runReport(args[2:], stdout, stderr, bundle.Write)
	}
	if len(args) >= 3 && args[0] == "experiment" && args[1] == "export" && args[2] == "verify" {
		return runExportVerify(args[3:], stdout, stderr, bundle.VerifyExport)
	}
	if len(args) >= 3 && args[0] == "experiment" && args[1] == "export" && args[2] == "ask" {
		return runExportAsk(args[3:], stdout, stderr, bundle.AskExport)
	}
	if len(args) >= 3 && args[0] == "experiment" && args[1] == "export" && args[2] == "finding" {
		return runExportFinding(args[3:], stdout, stderr, bundle.FindExport)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "export" {
		return runExport(args[2:], stdout, stderr, bundle.Export)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "verify" {
		return runVerify(args[2:], stdout, stderr, bundle.Verify)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "finding" {
		return runFinding(args[2:], stdout, stderr, bundle.Find)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "ask" {
		return runAsk(args[2:], stdout, stderr, bundle.Ask)
	}
	if len(args) >= 3 && args[0] == "experiment" && args[1] == "ask-archive" && args[2] == "verify" {
		return runAskArchiveVerify(args[3:], stdout, stderr, bundle.VerifyArchiveQuestionReport)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "ask-archive" {
		if len(args) >= 3 && args[2] == "save" {
			return runAskArchiveSave(args[3:], stdout, stderr, bundle.SaveArchiveQuestionReport)
		}
		if len(args) >= 4 && args[2] == "transitions" && args[3] == "save" {
			return runAskArchiveTransitionsSave(args[4:], stdout, stderr, bundle.SaveArchiveQuestionTransitionHistory)
		}
		if len(args) >= 4 && args[2] == "transitions" && args[3] == "acceptance" {
			if len(args) >= 5 && args[4] == "save" {
				return runAskArchiveTransitionsAcceptanceSave(args[5:], stdout, stderr, bundle.SaveArchiveQuestionTransitionHistoryAcceptanceRecord)
			}
			if len(args) >= 5 && args[4] == "verify" {
				return runAskArchiveTransitionsAcceptanceVerify(args[5:], stdout, stderr, bundle.VerifyArchiveQuestionTransitionHistoryAcceptanceRecord)
			}
			_, _ = io.WriteString(stderr, usage)
			return 2
		}
		if len(args) >= 4 && args[2] == "transitions" && args[3] == "questions" {
			return runQuestions(args[4:], stdout, stderr, bundle.ArchiveQuestionTransitionHistoryQuestions)
		}
		if len(args) >= 4 && args[2] == "transitions" && args[3] == "ask" {
			if len(args) >= 5 && args[4] == "repeated" {
				return runAskArchiveTransitionsAskRepeated(args[5:], stdout, stderr, bundle.AskArchiveQuestionTransitionHistoryRepeated)
			}
			if len(args) >= 5 && args[4] == "all" {
				if len(args) >= 6 && args[5] == "save" {
					return runAskArchiveTransitionsAskAllSave(args[6:], stdout, stderr, bundle.SaveArchiveQuestionTransitionHistoryQuestionRound)
				}
				if len(args) >= 6 && args[5] == "verify" {
					return runAskArchiveTransitionsAskAllVerify(args[6:], stdout, stderr, bundle.VerifyArchiveQuestionTransitionHistoryQuestionRound)
				}
				if len(args) >= 6 && args[5] == "compare" {
					return runAskArchiveTransitionsAskAllCompare(args[6:], stdout, stderr, bundle.CompareArchiveQuestionTransitionHistoryQuestionRounds)
				}
				return runAskArchiveTransitionsAskAll(args[5:], stdout, stderr, bundle.AskArchiveQuestionTransitionHistoryQuestionRound)
			}
			if len(args) >= 5 && args[4] == "receipt" {
				if len(args) >= 6 && args[5] == "save" {
					return runAskArchiveTransitionsAskReceiptSave(args[6:], stdout, stderr, bundle.SaveArchiveQuestionTransitionHistoryAnswerReceipt)
				}
				if len(args) >= 6 && args[5] == "verify" {
					return runAskArchiveTransitionsAskReceiptVerify(args[6:], stdout, stderr, bundle.VerifyArchiveQuestionTransitionHistoryAnswerReceipt)
				}
				return runAskArchiveTransitionsAskReceipt(args[5:], stdout, stderr, bundle.AskArchiveQuestionTransitionHistoryReceipt)
			}
			if len(args) >= 6 {
				return runAskArchiveTransitionsAskQuestion(args[4:], stdout, stderr, bundle.AskArchiveQuestionTransitionHistory, bundle.AskArchiveQuestionTransitionHistoryRepeated, bundle.AskArchiveQuestionTransitionHistorySnapshots, bundle.AskArchiveQuestionTransitionHistorySummary)
			}
			return runAskArchiveTransitionsAsk(args[4:], stdout, stderr, bundle.AskArchiveQuestionTransitionHistory)
		}
		if len(args) >= 4 && args[2] == "transitions" && args[3] == "verify" {
			return runAskArchiveTransitionsVerify(args[4:], stdout, stderr, bundle.VerifyArchiveQuestionTransitionHistory)
		}
		if len(args) >= 3 && args[2] == "compare-current" {
			return runAskArchiveCompareCurrent(args[3:], stdout, stderr, bundle.CompareArchiveQuestionReportWithArchive)
		}
		if len(args) >= 3 && args[2] == "compare" {
			return runAskArchiveCompare(args[3:], stdout, stderr, bundle.CompareArchiveQuestionReports)
		}
		if len(args) >= 3 && args[2] == "transitions" {
			return runAskArchiveTransitions(args[3:], stdout, stderr, bundle.CompareArchiveQuestionHistory)
		}
		return runAskArchive(args[2:], stdout, stderr, bundle.AskArchive)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "questions" {
		return runQuestions(args[2:], stdout, stderr, bundle.Questions)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "list" {
		return runList(args[2:], stdout, stderr, bundle.Index)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "serve" {
		return runServe(args[2:], stdout, stderr, http.ListenAndServe)
	}

	_, _ = io.WriteString(stderr, usage)
	return 2
}

func runValidate(path string, stdout, stderr io.Writer) int {
	file, err := os.Open(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: validate: open manifest: %v\n", err)
		return 1
	}
	defer file.Close()

	manifest, err := experiment.Decode(file)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: validate: %v\n", err)
		return 1
	}
	contractDigest := manifest.ContractDigest()

	_, err = fmt.Fprintf(
		stdout,
		"valid manifest\nname: %s\nschema_version: %d\nvariable: %s\npersona_fields: %d\nmanifest_contract_sha256: %s\n",
		manifest.Name,
		manifest.SchemaVersion,
		manifest.Variable,
		len(manifest.Baseline),
		contractDigest,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: validate: write output: %v\n", err)
		return 1
	}
	return 0
}

func runBrowserTrace(
	args []string,
	stdout, stderr io.Writer,
	save browserTraceSaver,
) int {
	flags := flag.NewFlagSet("browser trace", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser trace: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: browser trace: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"browser trace complete\nscope: %s\ncompleteness: %s\nevents: %d\ntrace_sha256: %s\n",
		summary.Scope,
		summary.Completeness,
		summary.Events,
		summary.TraceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser trace: write output: %v\n", err)
		return 1
	}
	return 0
}

type browserCaptureRunner func(string, string, []string, string) (browser.CaptureSummary, error)

func runBrowserCapture(
	args []string,
	stdout, stderr io.Writer,
	capture browserCaptureRunner,
) int {
	flags := flag.NewFlagSet("browser capture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	procedurePath := flags.String("procedure", "", "")
	driverPath := flags.String("driver", "", "")
	var driverArgs []string
	flags.Func("driver-arg", "", func(value string) error {
		driverArgs = append(driverArgs, value)
		return nil
	})
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 || *procedurePath == "" || *driverPath == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := capture(*procedurePath, *driverPath, driverArgs, flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser capture: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: browser capture: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"browser capture complete\nprocedure_sha256: %s\nscope: %s\ncompleteness: %s\nevents: %d\ntrace_sha256: %s\n",
		summary.ProcedureSHA256,
		summary.Trace.Scope,
		summary.Trace.Completeness,
		summary.Trace.Events,
		summary.Trace.TraceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: browser capture: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceVerify(
	args []string,
	stdout, stderr io.Writer,
	verify traceVerifier,
) int {
	flags := flag.NewFlagSet("trace verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectSHA256Set := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectSHA256Set = true
		}
	})
	if expectSHA256Set && !trace.ValidSHA256(*expectSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: trace verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace verify: %v\n", err)
		return 1
	}
	if expectSHA256Set && *expectSHA256 != summary.TraceSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace verify: trace SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"trace verified\nscope: %s\ncompleteness: %s\nevents: %d\ntrace_sha256: %s\n",
		summary.Scope,
		summary.Completeness,
		summary.Events,
		summary.TraceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceCompare(
	args []string,
	stdout, stderr io.Writer,
	compare traceComparer,
) int {
	flags := flag.NewFlagSet("trace compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	comparison, err := compare(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace compare: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(comparison); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace compare: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"trace compared\nscope: %s\nbaseline_completeness: %s\ntreatment_completeness: %s\nunchanged: %d\ndifferences: %d\nunknowns: %d\n",
		comparison.Scope,
		comparison.BaselineCompleteness,
		comparison.TreatmentCompleteness,
		len(comparison.Unchanged),
		len(comparison.Differences),
		len(comparison.Unknowns),
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace compare: write output: %v\n", err)
		return 1
	}
	for _, difference := range comparison.Differences {
		if _, err := fmt.Fprintf(
			stdout,
			"- source: %s\n  channel: %s\n  kind: %s\n  destination: %s\n  change: %s\n  state: %s\n",
			difference.Source,
			difference.Channel,
			difference.Kind,
			difference.Destination,
			difference.KindOfChange,
			difference.State,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace compare: write output: %v\n", err)
			return 1
		}
	}
	for _, unknown := range comparison.Unknowns {
		if _, err := fmt.Fprintf(
			stdout,
			"- source: %s\n  channel: %s\n  kind: %s\n  destination: %s\n  state: %s\n  reason: %s\n",
			unknown.Source,
			unknown.Channel,
			unknown.Kind,
			unknown.Destination,
			unknown.State,
			unknown.Reason,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace compare: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runTraceSessionCreate(
	args []string,
	stdout, stderr io.Writer,
	save traceSessionSaver,
) int {
	flags := flag.NewFlagSet("trace session create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	adapter := flags.String("adapter", "", "")
	adapterVersion := flags.Int("adapter-version", 1, "")
	procedureSHA256 := flags.String("procedure-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 || *adapter == "" || *procedureSHA256 == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), trace.SessionInput{
		Adapter:         *adapter,
		AdapterVersion:  *adapterVersion,
		ProcedureSHA256: *procedureSHA256,
		Role:            trace.RoleStandalone,
		Order:           trace.OrderStandalone,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session create: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace session create: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceSessionSummary(stdout, "trace session complete", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session create: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceSessionVerify(
	args []string,
	stdout, stderr io.Writer,
	verify traceSessionVerifier,
) int {
	flags := flag.NewFlagSet("trace session verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectSHA256Set := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectSHA256Set = true
		}
	})
	if expectSHA256Set && !trace.ValidSHA256(*expectSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: trace session verify: expected SHA-256 must be 64 lowercase hexadecimal characters\n")
		return 1
	}
	summary, err := verify(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session verify: %v\n", err)
		return 1
	}
	if expectSHA256Set && *expectSHA256 != summary.SessionSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: trace session verify: session SHA-256 does not match expected identity\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace session verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceSessionSummary(stdout, "trace session verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceSessionPairVerify(
	args []string,
	stdout, stderr io.Writer,
	verify traceSessionPairVerifier,
) int {
	flags := flag.NewFlagSet("trace session pair verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 4 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := verify(flags.Arg(0), flags.Arg(1), flags.Arg(2), flags.Arg(3))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceSessionPairSummary(stdout, "trace session pair verified", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runTraceSessionPairCreate(
	args []string,
	stdout, stderr io.Writer,
	save traceSessionPairSaver,
) int {
	flags := flag.NewFlagSet("trace session pair create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	adapter := flags.String("adapter", "", "")
	adapterVersion := flags.Int("adapter-version", 1, "")
	procedureSHA256 := flags.String("procedure-sha256", "", "")
	order := flags.String("order", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 4 || *adapter == "" || *procedureSHA256 == "" || *order == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2), flags.Arg(3), trace.SessionPairInput{
		Adapter:         *adapter,
		AdapterVersion:  *adapterVersion,
		ProcedureSHA256: *procedureSHA256,
		Order:           *order,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair create: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair create: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if err := writeTraceSessionPairSummary(stdout, "trace session pair complete", summary); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair create: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceSessionPairSummary(stdout io.Writer, heading string, summary trace.SessionPairVerificationSummary) error {
	_, err := fmt.Fprintf(
		stdout,
		"%s\nsource: %s\nadapter: %s\nadapter_version: %d\nprocedure_sha256: %s\nscope: %s\norder: %s\nbaseline_completeness: %s\ntreatment_completeness: %s\nbaseline_trace_sha256: %s\ntreatment_trace_sha256: %s\nbaseline_session_sha256: %s\ntreatment_session_sha256: %s\npair_sha256: %s\n",
		heading,
		summary.Source,
		summary.Adapter,
		summary.AdapterVersion,
		summary.ProcedureSHA256,
		summary.Scope,
		summary.Order,
		summary.BaselineCompleteness,
		summary.TreatmentCompleteness,
		summary.BaselineTraceSHA256,
		summary.TreatmentTraceSHA256,
		summary.BaselineSessionSHA256,
		summary.TreatmentSessionSHA256,
		summary.PairSHA256,
	)
	return err
}

func runTraceSessionPairCompare(
	args []string,
	stdout, stderr io.Writer,
	compare traceSessionPairComparer,
) int {
	flags := flag.NewFlagSet("trace session pair compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 4 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	result, err := compare(flags.Arg(0), flags.Arg(1), flags.Arg(2), flags.Arg(3))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair compare: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair compare: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"trace session pair compared\npair_sha256: %s\nsource: %s\nscope: %s\norder: %s\nunchanged: %d\ndifferences: %d\nunknowns: %d\n",
		result.Pair.PairSHA256,
		result.Pair.Source,
		result.Pair.Scope,
		result.Pair.Order,
		len(result.Comparison.Unchanged),
		len(result.Comparison.Differences),
		len(result.Comparison.Unknowns),
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: trace session pair compare: write output: %v\n", err)
		return 1
	}
	return 0
}

func writeTraceSessionSummary(stdout io.Writer, heading string, summary trace.SessionVerificationSummary) error {
	_, err := fmt.Fprintf(
		stdout,
		"%s\nsource: %s\nadapter: %s\nadapter_version: %d\nprocedure_sha256: %s\nscope: %s\ncompleteness: %s\nrole: %s\norder: %s\npair_sha256: %s\ntrace_sha256: %s\nsession_sha256: %s\n",
		heading,
		summary.Source,
		summary.Adapter,
		summary.AdapterVersion,
		summary.ProcedureSHA256,
		summary.Scope,
		summary.Completeness,
		summary.Role,
		summary.Order,
		summary.PairSHA256,
		summary.TraceSHA256,
		summary.SessionSHA256,
	)
	return err
}

type targetChecker func(context.Context, string, string, string) (adb.Target, error)
type traceVerifier func(string) (trace.VerificationSummary, error)
type traceComparer func(string, string) (trace.Comparison, error)
type traceSessionSaver func(string, string, trace.SessionInput) (trace.SessionVerificationSummary, error)
type traceSessionVerifier func(string, string) (trace.SessionVerificationSummary, error)
type traceSessionPairSaver func(string, string, string, string, trace.SessionPairInput) (trace.SessionPairVerificationSummary, error)
type traceSessionPairVerifier func(string, string, string, string) (trace.SessionPairVerificationSummary, error)
type traceSessionPairComparer func(string, string, string, string) (trace.SessionPairComparison, error)
type experimentTraceSaver func(string, string, string) (trace.VerificationSummary, error)
type browserTraceSaver func(string, string) (trace.VerificationSummary, error)
type pairRunner func(
	context.Context,
	string,
	adb.Target,
	experiment.Manifest,
	string,
) error
type replicatedRunner func(
	context.Context,
	string,
	adb.Target,
	experiment.Manifest,
	string,
	int,
) error
type bundleWriter func(string) (bundle.Summary, error)
type replicatedVerifier func(string) (bundle.ReplicatedExperimentSummary, error)
type bundleExporter func(string, string) (bundle.ExportSummary, error)
type bundleExportVerifier func(string) (bundle.ExportVerificationSummary, error)
type bundleFinder func(string, string) (bundle.Finding, error)
type bundleAsker func(string, string) (bundle.Answer, error)
type bundleArchiveQuestionAsker func(string, string) (bundle.ArchiveQuestionReport, error)
type bundleArchiveQuestionSaver func(string, string, string) (bundle.ArchiveQuestionVerificationSummary, error)
type bundleArchiveQuestionReportVerifier func(string) (bundle.ArchiveQuestionVerificationSummary, error)
type bundleArchiveQuestionReportComparer func(string, string) (bundle.ArchiveQuestionComparison, error)
type bundleArchiveQuestionTransitionComparer func([]string) (bundle.ArchiveQuestionTransitionHistory, error)
type bundleArchiveQuestionHistorySaver func([]string, string) (bundle.ArchiveQuestionTransitionVerificationSummary, error)
type bundleArchiveQuestionTransitionVerifier func(string) (bundle.ArchiveQuestionTransitionVerificationSummary, error)
type bundleArchiveQuestionTransitionAsker func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error)
type bundleArchiveQuestionTransitionRepeatedAsker func(string) (bundle.ArchiveQuestionTransitionHistoryRepeatedAnswer, error)
type bundleArchiveQuestionTransitionSnapshotAsker func(string) (bundle.ArchiveQuestionTransitionHistorySnapshotAnswer, error)
type bundleArchiveQuestionTransitionSummaryAsker func(string) (bundle.ArchiveQuestionTransitionHistorySummaryAnswer, error)
type bundleArchiveQuestionTransitionRoundAsker func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundAnswer, error)
type bundleArchiveQuestionTransitionRoundSaver func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error)
type bundleArchiveQuestionTransitionRoundVerifier func(string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundVerificationSummary, error)
type bundleArchiveQuestionTransitionRoundComparer func(string, string) (bundle.ArchiveQuestionTransitionHistoryQuestionRoundComparison, error)
type bundleArchiveQuestionAcceptanceSaver func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error)
type bundleArchiveQuestionAcceptanceVerifier func(string) (bundle.ArchiveQuestionTransitionHistoryAcceptanceVerificationSummary, error)
type bundleArchiveQuestionTransitionReceiptAsker func(string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceipt, error)
type bundleArchiveQuestionTransitionReceiptSaver func(string, string, string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptSaveSummary, error)
type bundleArchiveQuestionTransitionReceiptVerifier func(string) (bundle.ArchiveQuestionTransitionHistoryAnswerReceiptVerificationSummary, error)
type bundleQuestionLister func() []bundle.Question
type bundleArchiveIndexer func(string) ([]bundle.ArchiveEntry, error)
type uiServer func(string, http.Handler) error

func runAndroidCheck(
	args []string,
	stdout, stderr io.Writer,
	check targetChecker,
) int {
	flags := flag.NewFlagSet("android check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("adb", "adb", "")
	device := flags.String("device", "", "")
	packageName := flags.String("package", "", "")
	if err := flags.Parse(args); err != nil ||
		flags.NArg() != 0 ||
		*device == "" ||
		*packageName == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), adbCheckTimeout)
	defer cancel()

	target, err := check(ctx, *binary, *device, *packageName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: android check: %v\n", err)
		return 1
	}
	target.AriadneRevision, target.AriadneModified = buildIdentity()

	_, err = fmt.Fprintf(
		stdout,
		"android target ready\n"+
			"adb_version: %s\n"+
			"device: %s\n"+
			"android_api: %d\n"+
			"architecture: %s\n"+
			"package: %s\n"+
			"package_version_code: %d\n"+
			"package_sha256: %s\n"+
			"ariadne_revision: %s\n"+
			"ariadne_modified: %t\n",
		target.Version,
		target.Device,
		target.AndroidAPI,
		target.Architecture,
		target.Package,
		target.PackageVersionCode,
		target.PackageSHA256,
		target.AriadneRevision,
		target.AriadneModified,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: android check: write output: %v\n", err)
		return 1
	}
	return 0
}

func runExperiment(
	args []string,
	stdout, stderr io.Writer,
	check targetChecker,
	runPair pairRunner,
) int {
	flags := flag.NewFlagSet("experiment run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("adb", "adb", "")
	device := flags.String("device", "", "")
	packageName := flags.String("package", "", "")
	outputDir := flags.String("output", "", "")
	if err := flags.Parse(args); err != nil ||
		flags.NArg() != 1 ||
		*device == "" ||
		*packageName == "" ||
		*outputDir == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	file, err := os.Open(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment run: open manifest: %v\n", err)
		return 1
	}
	defer file.Close()

	manifest, err := experiment.Decode(file)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment run: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), experimentRunTimeout)
	defer cancel()

	target, err := check(ctx, *binary, *device, *packageName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment run: %v\n", err)
		return 1
	}
	target.AriadneRevision, target.AriadneModified = buildIdentity()
	if err := runPair(ctx, *binary, target, manifest, *outputDir); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment run: %v\n", err)
		return 1
	}

	_, err = fmt.Fprintf(
		stdout,
		"experiment complete\nname: %s\nruns: 2\n",
		manifest.Name,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment run: write output: %v\n", err)
		return 1
	}
	return 0
}

func runExperimentReplicate(
	args []string,
	stdout, stderr io.Writer,
	check targetChecker,
	run replicatedRunner,
) int {
	flags := flag.NewFlagSet("experiment replicate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	binary := flags.String("adb", "adb", "")
	device := flags.String("device", "", "")
	packageName := flags.String("package", "", "")
	pairs := flags.Int("pairs", 0, "")
	outputDir := flags.String("output", "", "")
	if err := flags.Parse(args); err != nil ||
		flags.NArg() != 1 ||
		*device == "" ||
		*packageName == "" ||
		*pairs < 1 ||
		*outputDir == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	file, err := os.Open(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate: open manifest: %v\n", err)
		return 1
	}
	defer file.Close()

	manifest, err := experiment.Decode(file)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), experimentReplicateTimeout)
	defer cancel()
	target, err := check(ctx, *binary, *device, *packageName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate: %v\n", err)
		return 1
	}
	target.AriadneRevision, target.AriadneModified = buildIdentity()
	if err := run(ctx, *binary, target, manifest, *outputDir, *pairs); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate: %v\n", err)
		return 1
	}

	_, err = fmt.Fprintf(
		stdout,
		"experiment replication complete\nname: %s\npairs_per_order: %d\nruns: %d\norder: %s, %s\nreset_policy: %s\n",
		manifest.Name,
		*pairs,
		*pairs*2,
		adb.ReplicationOrderBaselineTreatment,
		adb.ReplicationOrderTreatmentBaseline,
		adb.ReplicationResetPolicy,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate: write output: %v\n", err)
		return 1
	}
	return 0
}

func runReplicateVerify(
	args []string,
	stdout, stderr io.Writer,
	verify replicatedVerifier,
) int {
	flags := flag.NewFlagSet("experiment replicate verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	_, err = fmt.Fprintf(
		stdout,
		"replicated experiment verified\nname: %s\nreceipt_sha256: %s\noutcome: %s\nevidence_state: %s\npairs: %d\npairs_per_order: %d\ncompleted_pairs: %d\nchanged_pairs: %d\nno_change_pairs: %d\nunknown_pairs: %d\n",
		summary.ManifestName,
		summary.ReceiptSHA256,
		summary.Outcome,
		summary.EvidenceState,
		summary.Pairs,
		summary.PairsPerOrder,
		summary.CompletedPairs,
		summary.ChangedPairs,
		summary.NoChangePairs,
		summary.UnknownPairs,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment replicate verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runExperimentTrace(
	args []string,
	stdout, stderr io.Writer,
	save experimentTraceSaver,
) int {
	flags := flag.NewFlagSet("experiment trace", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	session := flags.String("session", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 || *session == "" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	summary, err := save(flags.Arg(0), *session, flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment trace: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment trace: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"experiment trace complete\nsession: %s\nscope: %s\ncompleteness: %s\nevents: %d\ntrace_sha256: %s\n",
		*session,
		summary.Scope,
		summary.Completeness,
		summary.Events,
		summary.TraceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment trace: write output: %v\n", err)
		return 1
	}
	return 0
}

func buildIdentity() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", false
	}
	return identityFromSettings(info.Settings)
}

func identityFromSettings(settings []debug.BuildSetting) (string, bool) {
	revision := "unknown"
	modified := false
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			if validRevision(setting.Value) {
				revision = setting.Value
			}
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validReflectionSHA256(value string) bool {
	return len(value) == 64 && validRevision(value)
}

func runReport(
	args []string,
	stdout, stderr io.Writer,
	write bundleWriter,
) int {
	if len(args) != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := write(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment report: %v\n", err)
		return 1
	}
	_, err = fmt.Fprintf(
		stdout,
		"evidence bundle complete\nname: %s\ndifferences: %d\nunknowns: %d\n",
		summary.ManifestName,
		summary.Differences,
		summary.Unknowns,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment report: write output: %v\n", err)
		return 1
	}
	return 0
}

func runExport(
	args []string,
	stdout, stderr io.Writer,
	export bundleExporter,
) int {
	if len(args) != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := export(args[0], args[1])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment export: %v\n", err)
		return 1
	}
	_, err = fmt.Fprintf(
		stdout,
		"redacted export complete\nsource_evidence_sha256: %s\nexport_sha256: %s\n",
		summary.SourceEvidenceSHA256,
		summary.ExportSHA256,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment export: write output: %v\n", err)
		return 1
	}
	return 0
}

func runExportVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleExportVerifier,
) int {
	flags := flag.NewFlagSet("experiment export verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment export verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment export verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.ExportSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment export verify: redacted export SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment export verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err = fmt.Fprintf(
		stdout,
		"redacted export verified\nschema_version: %d\nsource_evidence_sha256: %s\nexport_sha256: %s\n",
		summary.SchemaVersion,
		summary.SourceEvidenceSHA256,
		summary.ExportSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment export verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleWriter,
) int {
	flags := flag.NewFlagSet("experiment verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment verify: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	_, err = fmt.Fprintf(
		stdout,
		"evidence bundle verified\nname: %s\ndifferences: %d\nunknowns: %d\n",
		summary.ManifestName,
		summary.Differences,
		summary.Unknowns,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runFinding(
	args []string,
	stdout, stderr io.Writer,
	find bundleFinder,
) int {
	return runFindingCommand(args, "experiment finding", stdout, stderr, find)
}

func runExportFinding(
	args []string,
	stdout, stderr io.Writer,
	find bundleFinder,
) int {
	return runFindingCommand(args, "experiment export finding", stdout, stderr, find)
}

func runFindingCommand(
	args []string,
	command string,
	stdout, stderr io.Writer,
	find bundleFinder,
) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	finding, err := find(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: %s: %v\n", command, err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(finding); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
			return 1
		}
		return 0
	}
	if _, err = fmt.Fprintf(
		stdout,
		"finding verified\nquestion: %s\nanswer_state: %s\nkind: %s\n",
		finding.Question,
		finding.AnswerState,
		finding.Kind,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
		return 1
	}
	if finding.Classification != "" {
		if _, err = fmt.Fprintf(stdout, "classification: %s\n", finding.Classification); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
			return 1
		}
	}
	if _, err = fmt.Fprintf(
		stdout,
		"id: %s\nfield: %s\nstate: %s\nevidence:\n",
		finding.ID,
		finding.Field,
		finding.State,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
		return 1
	}
	for _, reference := range finding.Evidence {
		if _, err = fmt.Fprintf(stdout, "- %s\n", reference); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
			return 1
		}
	}
	return 0
}

func runAsk(
	args []string,
	stdout, stderr io.Writer,
	ask bundleAsker,
) int {
	return runQuestion(args, "experiment ask", stdout, stderr, ask)
}

func runExportAsk(
	args []string,
	stdout, stderr io.Writer,
	ask bundleAsker,
) int {
	return runQuestion(args, "experiment export ask", stdout, stderr, ask)
}

func runQuestion(
	args []string,
	command string,
	stdout, stderr io.Writer,
	ask bundleAsker,
) int {
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: %s: %v\n", command, err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
			return 1
		}
		return 0
	}
	if _, err = fmt.Fprintf(
		stdout,
		"question answered\nid: %s\nquestion: %s\nanswer_state: %s\nfindings:\n",
		answer.QuestionID,
		answer.Question,
		answer.State,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
		return 1
	}
	for _, findingID := range answer.FindingIDs {
		if _, err = fmt.Fprintf(stdout, "- %s\n", findingID); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: %s: write output: %v\n", command, err)
			return 1
		}
	}
	return 0
}

func runAskArchive(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	report, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive question answered\nid: %s\nquestion: %s\nobserved: %d\nunknown: %d\nunavailable: %d\nchecked: %d\nresults:\n",
		report.QuestionID,
		report.Question,
		report.Summary.Observed,
		report.Summary.Unknown,
		report.Summary.Unavailable,
		report.Summary.Checked,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
		return 1
	}
	for _, result := range report.Results {
		if _, err := fmt.Fprintf(
			stdout,
			"- directory: %s\n  manifest_name: %s\n",
			result.Directory,
			result.ManifestName,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
			return 1
		}
		if result.RecordedAt != "" {
			if _, err := fmt.Fprintf(stdout, "  recorded_at: %s\n", result.RecordedAt); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
				return 1
			}
		}
		if !result.Available {
			if _, err := io.WriteString(stdout, "  answer_state: unavailable\n"); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
				return 1
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "  answer_state: %s\n  findings:\n", result.Answer.State); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
			return 1
		}
		if result.Answer.Reason != "" {
			if _, err := fmt.Fprintf(stdout, "  reason: %s\n", result.Answer.Reason); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
				return 1
			}
		}
		for _, findingID := range result.Answer.FindingIDs {
			if _, err := fmt.Fprintf(stdout, "  - %s\n", findingID); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive: write output: %v\n", err)
				return 1
			}
		}
	}
	return 0
}

func runAskArchiveSave(
	args []string,
	stdout, stderr io.Writer,
	save bundleArchiveQuestionSaver,
) int {
	flags := flag.NewFlagSet("experiment ask-archive save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive question saved\nschema_version: %d\nquestion_id: %s\nchecked: %d\nreflection_sha256: %s\n",
		summary.SchemaVersion,
		summary.QuestionID,
		summary.Checked,
		summary.ReflectionSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveCompare(
	args []string,
	stdout, stderr io.Writer,
	compare bundleArchiveQuestionReportComparer,
) int {
	return runAskArchiveComparison(args, "compare", stdout, stderr, compare)
}

func runAskArchiveCompareCurrent(
	args []string,
	stdout, stderr io.Writer,
	compare bundleArchiveQuestionReportComparer,
) int {
	return runAskArchiveComparison(args, "compare-current", stdout, stderr, compare)
}

func runAskArchiveComparison(
	args []string,
	command string,
	stdout, stderr io.Writer,
	compare bundleArchiveQuestionReportComparer,
) int {
	flags := flag.NewFlagSet("experiment ask-archive "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	comparison, err := compare(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: %v\n", command, err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(comparison); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: write output: %v\n", command, err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection comparison complete\ncomparison_id: %s\ncomparison_question: %s\nquestion_id: %s\nquestion: %s\nresult: %s\nolder_reflection_sha256: %s\nnewer_reflection_sha256: %s\ncompared: %d\nchanged: %d\nolder_only: %d\nnewer_only: %d\n",
		comparison.ComparisonID,
		comparison.ComparisonQuestion,
		comparison.QuestionID,
		comparison.Question,
		comparison.Result,
		comparison.OlderReflectionSHA256,
		comparison.NewerReflectionSHA256,
		comparison.Compared,
		comparison.Changed,
		comparison.OlderOnly,
		comparison.NewerOnly,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: write output: %v\n", command, err)
		return 1
	}
	if len(comparison.StateChanges) > 0 {
		if _, err := io.WriteString(stdout, "state_changes:\n"); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: write output: %v\n", command, err)
			return 1
		}
		for _, change := range comparison.StateChanges {
			if _, err := fmt.Fprintf(stdout, "- directory: %s\n  older_state: %s\n  newer_state: %s\n", change.Directory, change.OlderState, change.NewerState); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: write output: %v\n", command, err)
				return 1
			}
		}
	}
	if _, err := io.WriteString(stdout, "note: this compares only bounded answer states; it does not infer a trend or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive %s: write output: %v\n", command, err)
		return 1
	}
	return 0
}

func runAskArchiveTransitions(
	args []string,
	stdout, stderr io.Writer,
	compareHistory bundleArchiveQuestionTransitionComparer,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() < 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	history, err := compareHistory(flags.Args())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(history); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection transitions complete\nhistory_id: %s\nhistory_question: %s\nquestion_id: %s\nquestion: %s\norder_basis: %s\nsnapshots: %d\ntransitions: %d\n",
		history.HistoryID,
		history.HistoryQuestion,
		history.QuestionID,
		history.Question,
		history.OrderBasis,
		history.Snapshots,
		len(history.Transitions),
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
		return 1
	}
	if len(history.SnapshotSummaries) > 0 {
		if _, err := io.WriteString(stdout, "snapshot_summaries:\n"); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
			return 1
		}
		for index, snapshot := range history.SnapshotSummaries {
			if _, err := fmt.Fprintf(
				stdout,
				"- snapshot: %d\n  reflection_sha256: %s\n  observed: %d\n  unknown: %d\n  unavailable: %d\n  checked: %d\n",
				index+1,
				snapshot.ReflectionSHA256,
				snapshot.Observed,
				snapshot.Unknown,
				snapshot.Unavailable,
				snapshot.Checked,
			); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
				return 1
			}
		}
	}
	for index, transition := range history.Transitions {
		if _, err := fmt.Fprintf(
			stdout,
			"- transition: %d\n  from_reflection_sha256: %s\n  to_reflection_sha256: %s\n  result: %s\n  compared: %d\n  changed: %d\n  from_only: %d\n  to_only: %d\n",
			index+1,
			transition.FromReflectionSHA256,
			transition.ToReflectionSHA256,
			transition.Result,
			transition.Compared,
			transition.Changed,
			transition.FromOnly,
			transition.ToOnly,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
			return 1
		}
		if len(transition.StateChanges) > 0 {
			if _, err := io.WriteString(stdout, "  state_changes:\n"); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
				return 1
			}
			for _, change := range transition.StateChanges {
				if _, err := fmt.Fprintf(stdout, "  - directory: %s\n    older_state: %s\n    newer_state: %s\n", change.Directory, change.OlderState, change.NewerState); err != nil {
					_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
					return 1
				}
			}
		}
	}
	if _, err := io.WriteString(stdout, "note: transitions follow caller-supplied order; incomparable membership is not a change claim, and this does not infer a trend or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAsk(
	args []string,
	stdout, stderr io.Writer,
	ask func(string) (bundle.ArchiveQuestionTransitionHistoryAnswer, error),
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection history question answered\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\ntransitions: %d\nchanged_transitions:\n",
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		answer.Transitions,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	for _, transition := range answer.ChangedTransitions {
		if _, err := fmt.Fprintf(stdout, "- %d\n", transition); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "changed_entries:\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	for _, entry := range answer.ChangedEntries {
		if _, err := fmt.Fprintf(stdout, "- transition: %d\n  from_reflection_sha256: %s\n  to_reflection_sha256: %s\n  directory: %s\n  older_state: %s\n  newer_state: %s\n", entry.Transition, entry.FromReflectionSHA256, entry.ToReflectionSHA256, entry.Directory, entry.OlderState, entry.NewerState); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "incomparable_transitions:\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	for _, transition := range answer.IncomparableTransitions {
		if _, err := fmt.Fprintf(stdout, "- %d\n", transition); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this answers only the verified history structure; it does not infer chronology or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskAll(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionTransitionRoundAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask all", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	round, err := ask(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(round); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection question round answered\ntransition_history_sha256: %s\nquestions:\n",
		round.TransitionHistorySHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all: write output: %v\n", err)
		return 1
	}
	for _, question := range round.Questions {
		if _, err := fmt.Fprintf(stdout, "- question_id: %s\n  question: %s\n  result: %s\n", question.QuestionID, question.Question, question.Result); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this records fixed bounded question results only; inspect an individual question for details, and do not infer chronology or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskAllSave(
	args []string,
	stdout, stderr io.Writer,
	save bundleArchiveQuestionTransitionRoundSaver,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask all save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := save(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection question round saved\nschema_version: %d\ntransition_history_sha256: %s\nquestions: %d\nround_sha256: %s\n",
		summary.SchemaVersion,
		summary.TransitionHistorySHA256,
		summary.Questions,
		summary.RoundSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskAllVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleArchiveQuestionTransitionRoundVerifier,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask all verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions ask all verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.RoundSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions ask all verify: question round SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection question round structurally verified\nschema_version: %d\ntransition_history_sha256: %s\nquestions: %d\nround_sha256: %s\nnote: this verifies the raw-value-free question round contract; it does not re-verify the transition history or prove the underlying evidence\n",
		summary.SchemaVersion,
		summary.TransitionHistorySHA256,
		summary.Questions,
		summary.RoundSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskAllCompare(
	args []string,
	stdout, stderr io.Writer,
	compare bundleArchiveQuestionTransitionRoundComparer,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask all compare", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	comparison, err := compare(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all compare: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(comparison); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all compare: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection question round comparison complete\nschema_version: %d\ncomparison_id: %s\ncomparison_question: %s\norder_basis: %s\nresult: %s\nfirst_round_sha256: %s\nsecond_round_sha256: %s\nfirst_transition_history_sha256: %s\nsecond_transition_history_sha256: %s\ncompared: %d\nchanged: %d\nchanged_questions:\n",
		comparison.SchemaVersion,
		comparison.ComparisonID,
		comparison.ComparisonQuestion,
		comparison.OrderBasis,
		comparison.Result,
		comparison.FirstRoundSHA256,
		comparison.SecondRoundSHA256,
		comparison.FirstTransitionHistorySHA256,
		comparison.SecondTransitionHistorySHA256,
		comparison.Compared,
		comparison.Changed,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all compare: write output: %v\n", err)
		return 1
	}
	for _, change := range comparison.ChangedQuestions {
		if _, err := fmt.Fprintf(stdout, "- question_id: %s\n  first_result: %s\n  second_result: %s\n", change.QuestionID, change.FirstResult, change.SecondResult); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all compare: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this compares fixed bounded question results in caller order; it does not infer chronology or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask all compare: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAcceptanceSave(
	args []string,
	stdout, stderr io.Writer,
	save bundleArchiveQuestionAcceptanceSaver,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions acceptance save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive question acceptance record saved\nschema_version: %d\ntransition_history_sha256: %s\nquestion_round_sha256: %s\nquestion_id: %s\nreceipt_sha256: %s\nacceptance_sha256: %s\n",
		summary.SchemaVersion,
		summary.TransitionHistorySHA256,
		summary.QuestionRoundSHA256,
		summary.QuestionID,
		summary.ReceiptSHA256,
		summary.AcceptanceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAcceptanceVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleArchiveQuestionAcceptanceVerifier,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions acceptance verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions acceptance verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.AcceptanceSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions acceptance verify: acceptance SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive question acceptance record structurally verified\nschema_version: %d\ntransition_history_sha256: %s\nquestion_round_sha256: %s\nquestion_id: %s\nreceipt_sha256: %s\nacceptance_sha256: %s\nnote: this verifies the raw-value-free identity binding; it does not prove that a UI driver performed the selection\n",
		summary.SchemaVersion,
		summary.TransitionHistorySHA256,
		summary.QuestionRoundSHA256,
		summary.QuestionID,
		summary.ReceiptSHA256,
		summary.AcceptanceSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions acceptance verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskReceipt(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionTransitionReceiptAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	receipt, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(receipt); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection answer receipt\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\nnote: use --json for the portable raw-value-free answer details; this does not infer chronology or prove the underlying evidence\n",
		receipt.SchemaVersion,
		receipt.QuestionID,
		receipt.Question,
		receipt.Result,
		receipt.TransitionHistorySHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskReceiptSave(
	args []string,
	stdout, stderr io.Writer,
	save bundleArchiveQuestionTransitionReceiptSaver,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask receipt save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := save(flags.Arg(0), flags.Arg(1), flags.Arg(2))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection answer receipt saved\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\nreceipt_sha256: %s\n",
		summary.SchemaVersion,
		summary.QuestionID,
		summary.Question,
		summary.Result,
		summary.TransitionHistorySHA256,
		summary.ReceiptSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskQuestion(
	args []string,
	stdout, stderr io.Writer,
	askTransition bundleArchiveQuestionTransitionAsker,
	askRepeated bundleArchiveQuestionTransitionRepeatedAsker,
	askSnapshots bundleArchiveQuestionTransitionSnapshotAsker,
	askSummary bundleArchiveQuestionTransitionSummaryAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || (flags.NArg() != 1 && flags.NArg() != 2) {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	questions := bundle.ArchiveQuestionTransitionHistoryQuestions()
	questionID := questions[0].ID
	if flags.NArg() == 2 {
		questionID = flags.Arg(1)
	}
	askArgs := []string{flags.Arg(0)}
	if *jsonOutput {
		askArgs = append([]string{"--json"}, askArgs...)
	}
	switch questionID {
	case questions[0].ID:
		return runAskArchiveTransitionsAsk(askArgs, stdout, stderr, askTransition)
	case questions[1].ID:
		return runAskArchiveTransitionsAskRepeated(askArgs, stdout, stderr, askRepeated)
	case questions[2].ID:
		return runAskArchiveTransitionsAskSnapshots(askArgs, stdout, stderr, askSnapshots)
	case questions[3].ID:
		return runAskArchiveTransitionsAskSummary(askArgs, stdout, stderr, askSummary)
	default:
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions ask: question ID is invalid\n")
		return 2
	}
}

func runAskArchiveTransitionsAskSummary(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionTransitionSummaryAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection summary-change question answered\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\ntransitions: %d\nchanged_transitions:\n",
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		answer.Transitions,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	for _, transition := range answer.ChangedTransitions {
		if _, err := fmt.Fprintf(stdout, "- %d\n", transition); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this answers only safe snapshot-summary structure; it does not infer chronology or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskSnapshots(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionTransitionSnapshotAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection snapshot-summary question answered\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\nsnapshots: %d\nsnapshot_summaries:\n",
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		answer.Snapshots,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	for index, snapshot := range answer.SnapshotSummaries {
		if _, err := fmt.Fprintf(
			stdout,
			"- snapshot: %d\n  reflection_sha256: %s\n  observed: %d\n  unknown: %d\n  unavailable: %d\n  checked: %d\n",
			index+1,
			snapshot.ReflectionSHA256,
			snapshot.Observed,
			snapshot.Unknown,
			snapshot.Unavailable,
			snapshot.Checked,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
			return 1
		}
	}
	if _, err := io.WriteString(stdout, "note: this answers only safe snapshot summaries; it does not infer chronology or prove the underlying evidence\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskRepeated(
	args []string,
	stdout, stderr io.Writer,
	ask bundleArchiveQuestionTransitionRepeatedAsker,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask repeated", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection repeated-change question answered\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\ntransitions: %d\nrepeated_entries:\n",
		answer.QuestionID,
		answer.Question,
		answer.Result,
		answer.TransitionHistorySHA256,
		answer.Transitions,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: write output: %v\n", err)
		return 1
	}
	for _, entry := range answer.RepeatedEntries {
		if _, err := fmt.Fprintf(stdout, "- directory: %s\n  changes:\n", entry.Directory); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: write output: %v\n", err)
			return 1
		}
		for _, change := range entry.Changes {
			if _, err := fmt.Fprintf(stdout, "  - transition: %d\n    from_reflection_sha256: %s\n    to_reflection_sha256: %s\n    older_state: %s\n    newer_state: %s\n", change.Transition, change.FromReflectionSHA256, change.ToReflectionSHA256, change.OlderState, change.NewerState); err != nil {
				_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: write output: %v\n", err)
				return 1
			}
		}
	}
	if _, err := io.WriteString(stdout, "note: this reports repeated verified state-change records only; it does not infer chronology or a trend\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask repeated: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsAskReceiptVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleArchiveQuestionTransitionReceiptVerifier,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions ask receipt verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions ask receipt verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.ReceiptSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions ask receipt verify: answer receipt SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection answer receipt structurally verified\nschema_version: %d\nquestion_id: %s\nquestion: %s\nresult: %s\ntransition_history_sha256: %s\nreceipt_sha256: %s\nnote: this verifies the raw-value-free receipt contract; it does not re-verify the transition history or prove the underlying evidence\n",
		summary.SchemaVersion,
		summary.QuestionID,
		summary.Question,
		summary.Result,
		summary.TransitionHistorySHA256,
		summary.ReceiptSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions ask receipt verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsSave(
	args []string,
	stdout, stderr io.Writer,
	save bundleArchiveQuestionHistorySaver,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions save", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() < 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	paths := flags.Args()
	historyPath := paths[len(paths)-1]
	reportPaths := paths[:len(paths)-1]
	summary, err := save(reportPaths, historyPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions save: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions save: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection transitions saved\nschema_version: %d\nhistory_id: %s\nhistory_question: %s\nquestion_id: %s\norder_basis: %s\nsnapshots: %d\ntransitions: %d\ntransition_history_sha256: %s\n",
		summary.SchemaVersion,
		summary.HistoryID,
		summary.HistoryQuestion,
		summary.QuestionID,
		summary.OrderBasis,
		summary.Snapshots,
		summary.Transitions,
		summary.TransitionHistorySHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions save: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveTransitionsVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleArchiveQuestionTransitionVerifier,
) int {
	flags := flag.NewFlagSet("experiment ask-archive transitions verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.TransitionHistorySHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive transitions verify: transition history SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive reflection transitions structurally verified\nschema_version: %d\nhistory_id: %s\nhistory_question: %s\nquestion_id: %s\norder_basis: %s\nsnapshots: %d\ntransitions: %d\ntransition_history_sha256: %s\nnote: this verifies the derived transition contract; it does not prove the underlying evidence or chronology\n",
		summary.SchemaVersion,
		summary.HistoryID,
		summary.HistoryQuestion,
		summary.QuestionID,
		summary.OrderBasis,
		summary.Snapshots,
		summary.Transitions,
		summary.TransitionHistorySHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive transitions verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runAskArchiveVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleArchiveQuestionReportVerifier,
) int {
	flags := flag.NewFlagSet("experiment ask-archive verify", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	expectedSHA256 := flags.String("expect-sha256", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	expectedSHA256Provided := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "expect-sha256" {
			expectedSHA256Provided = true
		}
	})
	if expectedSHA256Provided && !validReflectionSHA256(*expectedSHA256) {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive verify: expect-sha256 must be a lowercase 64-character SHA-256 digest\n")
		return 2
	}

	summary, err := verify(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive verify: %v\n", err)
		return 1
	}
	if expectedSHA256Provided && summary.ReflectionSHA256 != *expectedSHA256 {
		_, _ = io.WriteString(stderr, "ariadne: experiment ask-archive verify: reflection SHA-256 mismatch\n")
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(summary); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive verify: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := fmt.Fprintf(
		stdout,
		"archive question report structurally verified\nschema_version: %d\nquestion_id: %s\nchecked: %d\nreflection_sha256: %s\nnote: this identifies the canonical safe reflection content; it does not prove the underlying evidence\n",
		summary.SchemaVersion,
		summary.QuestionID,
		summary.Checked,
		summary.ReflectionSHA256,
	); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask-archive verify: write output: %v\n", err)
		return 1
	}
	return 0
}

func runQuestions(
	args []string,
	stdout, stderr io.Writer,
	list bundleQuestionLister,
) int {
	flags := flag.NewFlagSet("experiment questions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	questions := list()
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(questions); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment questions: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "question catalog\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment questions: write output: %v\n", err)
		return 1
	}
	for _, question := range questions {
		if _, err := fmt.Fprintf(
			stdout,
			"- id: %s\n  question: %s\n",
			question.ID,
			question.Text,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment questions: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runList(
	args []string,
	stdout, stderr io.Writer,
	index bundleArchiveIndexer,
) int {
	flags := flag.NewFlagSet("experiment list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	entries, err := index(flags.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment list: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(entries); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment list: write output: %v\n", err)
			return 1
		}
		return 0
	}
	if _, err := io.WriteString(stdout, "archived bundles\n"); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment list: write output: %v\n", err)
		return 1
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintf(
			stdout,
			"- directory: %s\n  manifest_name: %s\n  differences: %d\n  unknowns: %d\n",
			entry.Directory,
			entry.ManifestName,
			entry.Differences,
			entry.Unknowns,
		); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment list: write output: %v\n", err)
			return 1
		}
	}
	return 0
}

func runServe(
	args []string,
	stdout, stderr io.Writer,
	serve uiServer,
) int {
	flags := flag.NewFlagSet("experiment serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("addr", "127.0.0.1:8787", "")
	historyPath := flags.String("history", "", "")
	reflectionPath := flags.String("reflection", "", "")
	exportPath := flags.String("export", "", "")
	acceptancePath := flags.String("acceptance", "", "")
	roundFirstPath := flags.String("round-first", "", "")
	roundSecondPath := flags.String("round-second", "", "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	if !loopbackAddress(*address) {
		_, _ = io.WriteString(stderr, "ariadne: experiment serve: address must use a loopback IP\n")
		return 2
	}
	if *acceptancePath != "" && *historyPath == "" {
		_, _ = io.WriteString(stderr, "ariadne: experiment serve: --acceptance requires --history\n")
		return 2
	}
	if (*roundFirstPath == "") != (*roundSecondPath == "") {
		_, _ = io.WriteString(stderr, "ariadne: experiment serve: --round-first and --round-second must be supplied together\n")
		return 2
	}
	if _, err := fmt.Fprintf(stdout, "ariadne: review UI listening at http://%s/\n", *address); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment serve: write output: %v\n", err)
		return 1
	}
	reviewHandler := ui.HandlerWithReviewAndExportAndAcceptanceAndQuestionRounds(flags.Arg(0), *historyPath, *reflectionPath, *exportPath, *acceptancePath, *roundFirstPath, *roundSecondPath)
	if err := serve(*address, reviewHandler); err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment serve: %v\n", err)
		return 1
	}
	return 0
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
