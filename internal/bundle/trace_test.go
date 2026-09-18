package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestBuildAndSaveExperimentTrace(t *testing.T) {
	runDir := makeRun(t, runOptions{
		baselineStorage:      `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		baselineNetworkBody:  `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		treatmentStorage:     `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		treatmentNetworkBody: `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		volatileFields:       []string{"request_id"},
	})
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}

	baseline, err := BuildExperimentTrace(runDir, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Scope != "all" || baseline.Completeness != portabletrace.Complete || len(baseline.Events) != 2 {
		t.Fatalf("baseline trace = %#v", baseline)
	}
	for _, event := range baseline.Events {
		if !strings.Contains(strings.Join(event.Fields, ","), "region") || !strings.Contains(strings.Join(event.Fields, ","), "session-id") {
			t.Fatalf("baseline event fields = %#v", event)
		}
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"baseline-request", "standard", "us-east"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("trace exposed captured value %q: %s", secret, data)
		}
	}

	baselinePath := filepath.Join(t.TempDir(), "baseline-trace.json")
	summary, err := SaveExperimentTrace(runDir, "baseline", baselinePath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Completeness != portabletrace.Complete || summary.Events != 2 || !portabletrace.ValidSHA256(summary.TraceSHA256) {
		t.Fatalf("saved trace summary = %#v", summary)
	}
	if _, err := SaveExperimentTrace(runDir, "baseline", baselinePath); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("trace overwrite error = %v", err)
	}

	treatment, err := BuildExperimentTrace(runDir, "treatment")
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := portabletrace.Compare(baseline, treatment)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Unchanged) != 2 || len(comparison.Differences) != 0 || len(comparison.Unknowns) != 0 {
		t.Fatalf("trace comparison = %#v", comparison)
	}
}

func TestBuildExperimentTraceMarksIncompleteStorageUnknown(t *testing.T) {
	runDir := makeRun(t, runOptions{
		baselineStorage:      `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		baselineNetworkBody:  `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		treatmentStorage:     `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		treatmentNetworkBody: `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		volatileFields:       []string{"request_id"},
		mutateTreatment:      markStorageCaptureFailure,
	})
	if err := os.Remove(filepath.Join(runDir, "treatment", "observations", "storage.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(runDir); err != nil {
		t.Fatal(err)
	}

	treatment, err := BuildExperimentTrace(runDir, "treatment")
	if err != nil {
		t.Fatal(err)
	}
	if treatment.Completeness != portabletrace.Partial || len(treatment.Events) != 1 || treatment.Events[0].Channel != "network" {
		t.Fatalf("incomplete treatment trace = %#v", treatment)
	}
	baseline, err := BuildExperimentTrace(runDir, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	comparison, err := portabletrace.Compare(baseline, treatment)
	if err != nil {
		t.Fatal(err)
	}
	if len(comparison.Differences) != 0 || len(comparison.Unknowns) != 1 || comparison.Unknowns[0].State != "unknown" {
		t.Fatalf("incomplete trace comparison = %#v", comparison)
	}
}

func TestBuildExperimentTraceFailsClosed(t *testing.T) {
	validRun := makeRun(t, runOptions{
		baselineStorage:      `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		baselineNetworkBody:  `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard"}`,
		treatmentStorage:     `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		treatmentNetworkBody: `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized"}`,
		volatileFields:       []string{"request_id"},
	})
	if _, err := BuildExperimentTrace(validRun, "other"); err == nil || !strings.Contains(err.Error(), "baseline or treatment") {
		t.Fatalf("invalid session error = %v", err)
	}

	unsupportedRun := makeRun(t, runOptions{
		baselineStorage:      `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard","new_field":"value"}`,
		baselineNetworkBody:  `{"schema_version":1,"region":"us-east","request_id":"baseline-request","variant":"standard","new_field":"value"}`,
		treatmentStorage:     `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized","new_field":"value"}`,
		treatmentNetworkBody: `{"schema_version":1,"region":"us-east","request_id":"treatment-request","variant":"personalized","new_field":"value"}`,
		volatileFields:       []string{"request_id"},
	})
	if _, err := Write(unsupportedRun); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildExperimentTrace(unsupportedRun, "baseline"); err == nil || !strings.Contains(err.Error(), "observation fields are unsupported") {
		t.Fatalf("unsupported observation error = %v", err)
	}

	if _, err := SaveExperimentTrace(validRun, "baseline", " "); err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("empty trace path error = %v", err)
	}
}

func TestExperimentTraceRejectsMissingRun(t *testing.T) {
	missingRun := filepath.Join(t.TempDir(), "missing")
	if _, err := BuildExperimentTrace(missingRun, "baseline"); err == nil || !strings.Contains(err.Error(), "verify run") {
		t.Fatalf("missing run build error = %v", err)
	}
	if _, err := SaveExperimentTrace(missingRun, "baseline", filepath.Join(t.TempDir(), "trace.json")); err == nil || !strings.Contains(err.Error(), "verify run") {
		t.Fatalf("missing run save error = %v", err)
	}
}

func TestExperimentTraceFieldMappingRejectsUnsupportedInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", data: nil, want: "observation fields are unsupported"},
		{name: "array", data: []byte(`[]`), want: "observation fields are unsupported"},
		{name: "schema", data: []byte(`{"schema_version":2,"region":"east","request_id":"id","variant":"standard"}`), want: "observation fields are unsupported"},
		{name: "unknown", data: []byte(`{"schema_version":1,"region":"east","request_id":"id","variant":"standard","other":"value"}`), want: "observation fields are unsupported"},
		{name: "missing tracking field", data: []byte(`{"schema_version":1,"region":"east","variant":"standard"}`), want: "observation fields are unsupported"},
		{name: "duplicate", data: []byte(`{"schema_version":1,"region":"east","region":"west","request_id":"id","variant":"standard"}`), want: "observation fields are unsupported"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := experimentTraceObservationFields(test.data); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("experimentTraceObservationFields() error = %v, want %q", err, test.want)
			}
		})
	}

	validNetwork := networkJSON(t, `{"schema_version":1,"region":"east","request_id":"id","variant":"standard"}`)
	if _, err := experimentTraceNetworkFields(validNetwork); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte(`{`),
		[]byte(`{"schema_version":1,"method":"POST","path":"/observe","content_type":"application/json","body_base64":"%%%"}`),
		[]byte(`{"schema_version":2,"method":"POST","path":"/observe","content_type":"application/json","body_base64":"e30="}`),
		[]byte(`{"schema_version":1,"method":"GET","path":"/observe","content_type":"application/json","body_base64":"e30="}`),
		[]byte(`{"schema_version":1,"method":"POST","path":"/observe","content_type":"text/plain","body_base64":"e30="}`),
		[]byte(`{"schema_version":1,"method":"POST","path":"/observe","content_type":"application/json","body_base64":"e30="} {}`),
	} {
		if _, err := experimentTraceNetworkFields(data); err == nil || !strings.Contains(err.Error(), "network observation is unsupported") {
			t.Fatalf("experimentTraceNetworkFields(%s) error = %v", data, err)
		}
	}
}
