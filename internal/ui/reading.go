package ui

import "fmt"

func traceOutcomeMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "replicated-change":
		return "Every complete matched pair reported a safe structural difference."
	case "no-change-observed":
		return "Every complete matched pair reported no safe structural difference."
	case "mixed-inconsistent":
		return "Complete matched pairs disagreed about whether a safe structural difference appeared."
	case "unknown":
		return "The retained pairs did not provide enough support for one outcome."
	case "complete":
		return "Every retained trace met the question's coverage condition."
	case "available":
		return "The reviewed source adapter appears in the retained archive."
	case "changed":
		return "At least one compared trace reported a category change."
	case "same":
		return "The compared trace categories stayed the same in the supplied order."
	case "mixed":
		return "Compared traces did not all agree on one structural result."
	case "overlap-observed":
		return "A reviewed category appeared across multiple source or adapter boundaries."
	case "no-overlap-observed":
		return "No reviewed category appeared across multiple source or adapter boundaries."
	case "supported":
		return "Every retained run met the support checks for this question."
	case "consistent":
		return "Supported runs agreed on the reported outcome."
	case "inconsistent":
		return "Supported runs did not agree on the reported outcome."
	default:
		return "This outcome is outside the reviewed vocabulary, so Ariadne withholds an interpretation."
	}
}

func traceEvidenceMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "observed":
		return "The retained evidence directly contains the reported structural result."
	case "inferred":
		return "The evidence supports the result indirectly; it is not a direct observation."
	case "claimed":
		return "The result comes from a source claim rather than a direct observation."
	case "unknown":
		return "The retained evidence cannot establish what happened."
	default:
		return "The evidence state is outside the reviewed vocabulary."
	}
}

func traceCoverageMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "complete", "observed":
		return "The declared channels were covered for this retained trace."
	case "partial":
		return "Some declared channels were missing, so absence remains unknown."
	case "unknown":
		return "The retained traces do not support a complete coverage conclusion."
	default:
		return "Coverage is not described by this artifact."
	}
}

func traceComparisonMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "same":
		return "The fixed projections stayed the same in the supplied order."
	case "changed":
		return "At least one fixed projection changed in the supplied order."
	case "incomparable":
		return "The two projections were not compatible enough for a safe comparison."
	default:
		return "The comparison could not support a safe interpretation."
	}
}

func minimizationSelectionMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "selected":
		return "Ariadne found a least-disclosing candidate that met the tested bar."
	case "no-sufficient-candidate":
		return "No candidate met the tested functionality bar with consistent observed evidence."
	case "unknown":
		return "The ladder cannot select a candidate because some comparisons are unknown or incomplete."
	default:
		return "The selection state is outside the reviewed vocabulary."
	}
}

func minimizationClassificationMeaning(value any) string {
	switch fmt.Sprint(value) {
	case "sufficient":
		return "This candidate kept the tested behavior available in every supported comparison."
	case "insufficient":
		return "This candidate changed the tested behavior, so it did not meet the functionality requirement."
	case "mixed-inconsistent":
		return "Repeated comparisons disagreed about whether this candidate kept the tested behavior available."
	case "unknown":
		return "The retained evidence could not establish whether this candidate kept the tested behavior available."
	default:
		return "The functionality classification is outside the reviewed vocabulary."
	}
}
