package ui

import "testing"

func TestTraceReadingMeaningsStayFixedAndBounded(t *testing.T) {
	outcomes := map[string]string{
		"replicated-change":   "Every complete matched pair reported a safe structural difference.",
		"no-change-observed":  "Every complete matched pair reported no safe structural difference.",
		"mixed-inconsistent":  "Complete matched pairs disagreed about whether a safe structural difference appeared.",
		"unknown":             "The retained pairs did not provide enough support for one outcome.",
		"complete":            "Every retained trace met the question's coverage condition.",
		"available":           "The reviewed source adapter appears in the retained archive.",
		"changed":             "At least one compared trace reported a category change.",
		"same":                "The compared trace categories stayed the same in the supplied order.",
		"mixed":               "Compared traces did not all agree on one structural result.",
		"overlap-observed":    "A reviewed category appeared across multiple source or adapter boundaries.",
		"no-overlap-observed": "No reviewed category appeared across multiple source or adapter boundaries.",
		"supported":           "Every retained run met the support checks for this question.",
		"consistent":          "Supported runs agreed on the reported outcome.",
		"inconsistent":        "Supported runs did not agree on the reported outcome.",
		"future":              "This outcome is outside the reviewed vocabulary, so Ariadne withholds an interpretation.",
	}
	for value, want := range outcomes {
		if got := traceOutcomeMeaning(value); got != want {
			t.Errorf("traceOutcomeMeaning(%q) = %q, want %q", value, got, want)
		}
	}
	evidence := map[string]string{
		"observed": "The retained evidence directly contains the reported structural result.",
		"inferred": "The evidence supports the result indirectly; it is not a direct observation.",
		"claimed":  "The result comes from a source claim rather than a direct observation.",
		"unknown":  "The retained evidence cannot establish what happened.",
		"future":   "The evidence state is outside the reviewed vocabulary.",
	}
	for value, want := range evidence {
		if got := traceEvidenceMeaning(value); got != want {
			t.Errorf("traceEvidenceMeaning(%q) = %q, want %q", value, got, want)
		}
	}
	coverage := map[string]string{
		"complete": "The declared channels were covered for this retained trace.",
		"partial":  "Some declared channels were missing, so absence remains unknown.",
		"unknown":  "The retained traces do not support a complete coverage conclusion.",
		"future":   "Coverage is not described by this artifact.",
	}
	for value, want := range coverage {
		if got := traceCoverageMeaning(value); got != want {
			t.Errorf("traceCoverageMeaning(%q) = %q, want %q", value, got, want)
		}
	}
	comparisons := map[string]string{
		"same":         "The fixed projections stayed the same in the supplied order.",
		"changed":      "At least one fixed projection changed in the supplied order.",
		"incomparable": "The two projections were not compatible enough for a safe comparison.",
		"future":       "The comparison could not support a safe interpretation.",
	}
	for value, want := range comparisons {
		if got := traceComparisonMeaning(value); got != want {
			t.Errorf("traceComparisonMeaning(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestMinimizationMeaningsStayFixedAndBounded(t *testing.T) {
	selections := map[string]string{
		"selected":                "Ariadne found a least-disclosing candidate that met the tested bar.",
		"no-sufficient-candidate": "No candidate met the tested functionality bar with consistent observed evidence.",
		"unknown":                 "The ladder cannot select a candidate because some comparisons are unknown or incomplete.",
		"future":                  "The selection state is outside the reviewed vocabulary.",
	}
	for value, want := range selections {
		if got := minimizationSelectionMeaning(value); got != want {
			t.Errorf("minimizationSelectionMeaning(%q) = %q, want %q", value, got, want)
		}
	}
	classifications := map[string]string{
		"sufficient":         "This candidate kept the tested behavior available in every supported comparison.",
		"insufficient":       "This candidate changed the tested behavior, so it did not meet the functionality requirement.",
		"mixed-inconsistent": "Repeated comparisons disagreed about whether this candidate kept the tested behavior available.",
		"unknown":            "The retained evidence could not establish whether this candidate kept the tested behavior available.",
		"future":             "The functionality classification is outside the reviewed vocabulary.",
	}
	for value, want := range classifications {
		if got := minimizationClassificationMeaning(value); got != want {
			t.Errorf("minimizationClassificationMeaning(%q) = %q, want %q", value, got, want)
		}
	}
}
