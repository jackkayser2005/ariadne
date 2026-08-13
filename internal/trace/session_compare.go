package trace

import "errors"

type traceFileComparer func(string, string) (Comparison, error)

// SessionPairComparison is a provenance-bound structural comparison. The
// session pair and trace comparison remain separate objects so provenance,
// structural outcome, and evidence state cannot be conflated.
type SessionPairComparison struct {
	SchemaVersion int                            `json:"schema_version"`
	Pair          SessionPairVerificationSummary `json:"pair"`
	Comparison    Comparison                     `json:"comparison"`
}

// CompareSessionPair verifies a complementary session pair and compares the
// two bound traces without exposing payloads or source-specific identifiers.
func CompareSessionPair(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath string) (SessionPairComparison, error) {
	return compareSessionPair(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath, VerifySessionPair, CompareFiles)
}

func compareSessionPair(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath string, verify sessionPairVerifier, compare traceFileComparer) (SessionPairComparison, error) {
	pair, err := verify(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath)
	if err != nil {
		return SessionPairComparison{}, err
	}
	comparison, err := compare(baselineTracePath, treatmentTracePath)
	if err != nil {
		return SessionPairComparison{}, err
	}
	verifiedPair, err := verify(baselineSessionPath, baselineTracePath, treatmentSessionPath, treatmentTracePath)
	if err != nil {
		return SessionPairComparison{}, err
	}
	if verifiedPair != pair {
		return SessionPairComparison{}, errors.New("session pair identities changed during comparison")
	}
	if comparison.Scope != verifiedPair.Scope || comparison.BaselineTraceSHA256 != verifiedPair.BaselineTraceSHA256 || comparison.TreatmentTraceSHA256 != verifiedPair.TreatmentTraceSHA256 {
		return SessionPairComparison{}, errors.New("session pair comparison identities changed during verification")
	}
	return SessionPairComparison{
		SchemaVersion: verifiedPair.SchemaVersion,
		Pair:          verifiedPair,
		Comparison:    comparison,
	}, nil
}
