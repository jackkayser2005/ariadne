package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackkayser2005/ariadne/internal/adb"
	"github.com/jackkayser2005/ariadne/internal/experiment"
)

const usage = `usage:
  ariadne validate <manifest.json>
  ariadne android check [--adb <path>] --device <serial> --package <package>
`

const adbCheckTimeout = 10 * time.Second

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

	_, err = fmt.Fprintf(
		stdout,
		"android target ready\nadb_version: %s\ndevice: %s\npackage: %s\n",
		target.Version,
		target.Device,
		target.Package,
	)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "ariadne: android check: write output: %v\n", err)
		return 1
	}
	return 0
}
