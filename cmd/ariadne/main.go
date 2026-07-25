package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jackkayser2005/ariadne/internal/experiment"
)

const usage = "usage: ariadne validate <manifest.json>\n"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "validate" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}

	file, err := os.Open(args[1])
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
