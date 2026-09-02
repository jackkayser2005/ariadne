package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/evidence"
)

type androidAcceptanceFailWriter struct{}

func (androidAcceptanceFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func androidAcceptanceSummaryForTest() bundle.AndroidAcceptanceVerificationSummary {
	return bundle.AndroidAcceptanceVerificationSummary{
		SchemaVersion:            1,
		Workflow:                 "experiment-001-emulator",
		ManifestName:             "experiment-001-email",
		DeclaredVariable:         "email",
		ManifestContractSHA256:   strings.Repeat("c", 64),
		RunEvidenceSHA256:        strings.Repeat("d", 64),
		ReplicationReceiptSHA256: strings.Repeat("e", 64),
		Outcome:                  bundle.ReplicatedChange,
		EvidenceState:            evidence.Observed,
		QuestionID:               "counterfactual-change",
		QuestionState:            evidence.Observed,
		ReviewMethod:             "GET",
		ReviewPath:               "/",
		ReviewStatus:             "self-attested",
		AcceptanceSHA256:         strings.Repeat("f", 64),
	}
}

func TestRunAndroidAcceptanceSave(t *testing.T) {
	summary := androidAcceptanceSummaryForTest()
	args := []string{"--review-self-attested", "run", "replicated", "export", "reflection", "acceptance"}
	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		var got []string
		exitCode := runAndroidAcceptanceSave(args, &stdout, &stderr, func(run, replication, export, reflection, acceptance string, reviewChecked bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			got = []string{run, replication, export, reflection, acceptance}
			if !reviewChecked {
				t.Fatal("reviewChecked = false")
			}
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 {
			t.Fatalf("runAndroidAcceptanceSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "android acceptance record saved") ||
			!strings.Contains(stdout.String(), "outcome: replicated-change") ||
			!strings.Contains(stdout.String(), "review: GET self-attested") {
			t.Fatalf("stdout = %q", stdout.String())
		}
		if strings.Join(got, "|") != "run|replicated|export|reflection|acceptance" {
			t.Fatalf("arguments = %v", got)
		}
	})
	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceSave(append([]string{"--json"}, args...), &stdout, &stderr, func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "\"acceptance_sha256\":\""+summary.AcceptanceSHA256+"\"") {
			t.Fatalf("runAndroidAcceptanceSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("missing review", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceSave([]string{"run", "replicated", "export", "reflection", "acceptance"}, &stdout, &stderr, func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			t.Fatal("save called without review flag")
			return bundle.AndroidAcceptanceVerificationSummary{}, nil
		})
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "--review-self-attested is required") {
			t.Fatalf("runAndroidAcceptanceSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("invalid arguments", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceSave(nil, &stdout, &stderr, nil)
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("runAndroidAcceptanceSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("save error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceSave(args, &stdout, &stderr, func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return bundle.AndroidAcceptanceVerificationSummary{}, errors.New("fixture failed")
		})
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "fixture failed") {
			t.Fatalf("runAndroidAcceptanceSave() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("json output error", func(t *testing.T) {
		exitCode := runAndroidAcceptanceSave(append([]string{"--json"}, args...), androidAcceptanceFailWriter{}, &bytes.Buffer{}, func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 1 {
			t.Fatalf("runAndroidAcceptanceSave() = %d, want 1", exitCode)
		}
	})
	t.Run("human output error", func(t *testing.T) {
		exitCode := runAndroidAcceptanceSave(args, androidAcceptanceFailWriter{}, &bytes.Buffer{}, func(string, string, string, string, string, bool) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 1 {
			t.Fatalf("runAndroidAcceptanceSave() = %d, want 1", exitCode)
		}
	})
}

func TestRunAndroidAcceptanceVerify(t *testing.T) {
	summary := androidAcceptanceSummaryForTest()
	t.Run("human", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"acceptance"}, &stdout, &stderr, func(path string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			if path != "acceptance" {
				t.Fatalf("path = %q", path)
			}
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 ||
			!strings.Contains(stdout.String(), "android acceptance record structurally verified") ||
			!strings.Contains(stdout.String(), "note: this verifies raw-value-free identities") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("json", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"--json", "acceptance"}, &stdout, &stderr, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "\"acceptance_sha256\":\""+summary.AcceptanceSHA256+"\"") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("expected hash", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"--expect-sha256", summary.AcceptanceSHA256, "acceptance"}, &stdout, &stderr, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 0 || stderr.Len() != 0 {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("invalid expected hash", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"--expect-sha256", "bad", "acceptance"}, &stdout, &stderr, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			t.Fatal("verify called with invalid expected hash")
			return bundle.AndroidAcceptanceVerificationSummary{}, nil
		})
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "lowercase 64-character SHA-256") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"--expect-sha256", strings.Repeat("0", 64), "acceptance"}, &stdout, &stderr, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "SHA-256 mismatch") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("verify error", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify([]string{"acceptance"}, &stdout, &stderr, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return bundle.AndroidAcceptanceVerificationSummary{}, errors.New("invalid receipt")
		})
		if exitCode != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid receipt") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("invalid arguments", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		exitCode := runAndroidAcceptanceVerify(nil, &stdout, &stderr, nil)
		if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, stdout=%q, stderr=%q", exitCode, stdout.String(), stderr.String())
		}
	})
	t.Run("json output error", func(t *testing.T) {
		exitCode := runAndroidAcceptanceVerify([]string{"--json", "acceptance"}, androidAcceptanceFailWriter{}, &bytes.Buffer{}, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 1 {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, want 1", exitCode)
		}
	})
	t.Run("human output error", func(t *testing.T) {
		exitCode := runAndroidAcceptanceVerify([]string{"acceptance"}, androidAcceptanceFailWriter{}, &bytes.Buffer{}, func(string) (bundle.AndroidAcceptanceVerificationSummary, error) {
			return summary, nil
		})
		if exitCode != 1 {
			t.Fatalf("runAndroidAcceptanceVerify() = %d, want 1", exitCode)
		}
	})
}

func TestRunRoutesAndroidAcceptance(t *testing.T) {
	for _, args := range [][]string{
		{"experiment", "acceptance", "save"},
		{"experiment", "acceptance", "verify"},
		{"experiment", "acceptance", "unknown"},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := run(args, &stdout, &stderr); exitCode != 2 || !strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("run(%v) = %d, stdout=%q, stderr=%q", args, exitCode, stdout.String(), stderr.String())
		}
	}
}
