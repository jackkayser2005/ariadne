package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/experiment"
)

const usage = `usage:
  ariadne validate <manifest.json>
  ariadne android check [--adb <path>] --device <serial> --package <package>
  ariadne experiment run [--adb <path>] --device <serial> --package <package> --output <directory> <manifest.json>
  ariadne experiment report <run-directory>
  ariadne experiment verify <run-directory>
  ariadne experiment finding [--json] <run-directory> <finding-id>
  ariadne experiment ask [--json] <run-directory> <question-id>
  ariadne experiment questions [--json]
`

const adbCheckTimeout = 10 * time.Second
const experimentRunTimeout = 60 * time.Second

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
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "run" {
		return runExperiment(args[2:], stdout, stderr, adb.Check, adb.RunPair)
	}
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "report" {
		return runReport(args[2:], stdout, stderr, bundle.Write)
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
	if len(args) >= 2 && args[0] == "experiment" && args[1] == "questions" {
		return runQuestions(args[2:], stdout, stderr, bundle.Questions)
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

type targetChecker func(context.Context, string, string, string) (adb.Target, error)
type pairRunner func(
	context.Context,
	string,
	adb.Target,
	experiment.Manifest,
	string,
) error
type bundleWriter func(string) (bundle.Summary, error)
type bundleFinder func(string, string) (bundle.Finding, error)
type bundleAsker func(string, string) (bundle.Answer, error)
type bundleQuestionLister func() []bundle.Question

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

func runVerify(
	args []string,
	stdout, stderr io.Writer,
	verify bundleWriter,
) int {
	if len(args) != 1 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	summary, err := verify(args[0])
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment verify: %v\n", err)
		return 1
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
	flags := flag.NewFlagSet("experiment finding", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	finding, err := find(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(finding); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: write output: %v\n", err)
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
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: write output: %v\n", err)
		return 1
	}
	if finding.Classification != "" {
		if _, err = fmt.Fprintf(stdout, "classification: %s\n", finding.Classification); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: write output: %v\n", err)
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
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: write output: %v\n", err)
		return 1
	}
	for _, reference := range finding.Evidence {
		if _, err = fmt.Fprintf(stdout, "- %s\n", reference); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment finding: write output: %v\n", err)
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
	flags := flag.NewFlagSet("experiment ask", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonOutput := flags.Bool("json", false, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	answer, err := ask(flags.Arg(0), flags.Arg(1))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask: %v\n", err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(stdout).Encode(answer); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask: write output: %v\n", err)
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
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask: write output: %v\n", err)
		return 1
	}
	for _, findingID := range answer.FindingIDs {
		if _, err = fmt.Fprintf(stdout, "- %s\n", findingID); err != nil {
			_, _ = fmt.Fprintf(stderr, "ariadne: experiment ask: write output: %v\n", err)
			return 1
		}
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
