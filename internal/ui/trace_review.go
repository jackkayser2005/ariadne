package ui

import (
	"fmt"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func readStandaloneTrace(path string) (trace.Document, trace.VerificationSummary, error) {
	document, err := trace.Read(path)
	if err != nil {
		return trace.Document{}, trace.VerificationSummary{}, err
	}
	digest, err := trace.SHA256(document)
	if err != nil {
		return trace.Document{}, trace.VerificationSummary{}, err
	}
	return document, trace.VerificationSummary{
		SchemaVersion: document.SchemaVersion,
		Redacted:      document.Redacted,
		Scope:         document.Scope,
		Completeness:  document.Completeness,
		Events:        len(document.Events),
		TraceSHA256:   digest,
	}, nil
}

func traceSourceLabel(value any) string {
	switch fmt.Sprint(value) {
	case "android":
		return "Android app"
	case "browser":
		return "Browser"
	case "desktop":
		return "Desktop app"
	case "proxy":
		return "Network proxy"
	default:
		return "Reviewed source"
	}
}

func traceChannelLabel(value any) string {
	switch fmt.Sprint(value) {
	case "app-storage":
		return "App storage"
	case "cookie":
		return "Browser cookie"
	case "network":
		return "Network"
	case "web-storage":
		return "Web storage"
	default:
		return "Reviewed channel"
	}
}

func traceKindLabel(value any) string {
	switch fmt.Sprint(value) {
	case "beacon":
		return "Background beacon"
	case "cookie-write":
		return "Cookie write"
	case "request":
		return "Request"
	case "response":
		return "Response"
	case "storage-write":
		return "Storage write"
	default:
		return "Reviewed event"
	}
}
