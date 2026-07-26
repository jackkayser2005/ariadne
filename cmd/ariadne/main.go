package main

import (
	"context"
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

	_, err = fmt.Fprintf(
		stdout,
		"valid manifest\nname: %s\nschema_version: %d\nvariable: %s\npersona_fields: %d\n",
		manifest.Name,
		manifest.SchemaVersion,
		manifest.Variable,
		len(manifest.Baseline),
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
		"evidence bundle complete\nname: %s\ndifferences: %d\n",
		summary.ManifestName,
		summary.Differences,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: experiment report: write output: %v\n", err)
		return 1
	}
	return 0
}
