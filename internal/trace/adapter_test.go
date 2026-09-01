package trace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	const prefix = "--ariadne-source-adapter-mode="
	for _, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, prefix) {
			continue
		}
		mode := strings.TrimPrefix(arg, prefix)
		if mode == "timeout" {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		if mode == "descendant" {
			holder := exec.Command(os.Args[0], "--ariadne-source-adapter-mode=holder")
			holder.Stdout = os.Stdout
			holder.Stderr = os.Stderr
			if err := holder.Start(); err != nil {
				os.Exit(2)
			}
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		if mode == "holder" {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}
		requestData, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		if mode == "stdout-overflow" {
			_, _ = io.WriteString(os.Stdout, strings.Repeat("x", maxSourceAdapterResponseBytes+1))
			os.Exit(0)
		}
		if mode == "stderr-overflow" {
			_, _ = io.WriteString(os.Stderr, strings.Repeat("x", maxSourceAdapterStderrBytes+1))
			os.Exit(0)
		}
		if mode == "failure" {
			os.Exit(1)
		}
		var request SourceAdapterRequest
		if err := json.Unmarshal(requestData, &request); err != nil {
			os.Exit(2)
		}
		if mode == "malformed" {
			_, _ = io.WriteString(os.Stdout, "not-json")
			os.Exit(0)
		}
		response := sourceAdapterTestResponse(request)
		switch mode {
		case "challenge":
			response.Challenge = strings.Repeat("a", 64)
		case "procedure":
			response.ProcedureSHA256 = strings.Repeat("b", 64)
		case "scope":
			response.Trace.Scope = "storage"
		case "source":
			response.Trace.Events[0].Source = "secret"
		case "events":
			response.Trace.Events = make([]Event, request.Procedure.MaxEvents+1)
			for index := range response.Trace.Events {
				response.Trace.Events[index] = Event{
					Source: request.Procedure.Source, Channel: "network", Kind: "request",
					Destination: "analytics", Fields: []string{"region"},
				}
			}
		case "trailing":
			data, _ := json.Marshal(response)
			_, _ = os.Stdout.Write(append(data, []byte("{}")...))
			os.Exit(0)
		}
		_ = json.NewEncoder(os.Stdout).Encode(response)
		os.Exit(0)
	}
}

func TestRunSourceAdapterPublishesAndVerifiesRawFreeRun(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 2)
	driverPath := sourceAdapterTestDriver(t)
	outputDir := filepath.Join(t.TempDir(), "run")
	summary, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Receipt.Adapter != "external-desktop-v1" || summary.Receipt.Source != "desktop" ||
		summary.Receipt.Scope != "outbound" || summary.Receipt.Completeness != Complete || summary.Receipt.Events != 1 ||
		!ValidSHA256(summary.ReceiptSHA256) || !ValidSHA256(summary.Receipt.ChallengeSHA256) {
		t.Fatalf("summary = %#v", summary)
	}
	for _, name := range []string{sourceAdapterTraceFile, sourceAdapterSessionFile, sourceAdapterReceiptFile} {
		info, err := os.Lstat(filepath.Join(outputDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("artifact %q = %#v, err = %v", name, info, err)
		}
	}
	for _, name := range []string{sourceAdapterTraceFile, sourceAdapterSessionFile, sourceAdapterReceiptFile} {
		data, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"challenge":`) || strings.Contains(string(data), `"procedure":`) || strings.Contains(string(data), driverPath) {
			t.Fatalf("portable artifact %q exposed transient input: %s", name, data)
		}
	}
	verified, err := VerifySourceAdapterRun(outputDir)
	if err != nil || verified.ReceiptSHA256 != summary.ReceiptSHA256 {
		t.Fatalf("VerifySourceAdapterRun() = %#v, err = %v", verified, err)
	}
	if _, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), outputDir); err == nil {
		t.Fatal("RunSourceAdapter overwrote an existing run")
	}
	if _, err := VerifySourceAdapterRun(""); err == nil {
		t.Fatal("VerifySourceAdapterRun accepted an empty directory")
	}
}

func TestRunSourceAdapterBindsChallengeProcedureAndScope(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 1)
	driverPath := sourceAdapterTestDriver(t)
	for _, mode := range []string{"challenge", "procedure", "scope", "source", "events", "malformed", "trailing"} {
		t.Run(mode, func(t *testing.T) {
			_, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs(mode), filepath.Join(t.TempDir(), "run"))
			if err == nil {
				t.Fatal("RunSourceAdapter accepted an unbound adapter response")
			}
		})
	}
}

func TestRunSourceAdapterWithRunnerBindsRequestAndPublishes(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 2)
	driverPath := sourceAdapterTestDriver(t)
	outputDir := filepath.Join(t.TempDir(), "run")
	var request SourceAdapterRequest
	summary, err := runSourceAdapterWithRunner(procedurePath, driverPath, nil, outputDir, func(ctx context.Context, executable string, args []string, data []byte) ([]byte, error) {
		if ctx.Err() != nil || executable != driverPath || len(args) != 0 {
			t.Fatalf("runner inputs = %q, %#v, %v", executable, args, ctx.Err())
		}
		if err := json.Unmarshal(data, &request); err != nil {
			t.Fatal(err)
		}
		response := sourceAdapterTestResponse(request)
		return json.Marshal(response)
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.SchemaVersion != SourceAdapterRequestSchemaVersion || !validChallenge(request.Challenge) ||
		request.Procedure.Adapter != "external-desktop-v1" || !ValidSHA256(request.ProcedureSHA256) ||
		summary.Receipt.ProcedureSHA256 != request.ProcedureSHA256 {
		t.Fatalf("request/summary = %#v, %#v", request, summary)
	}
}

func TestRunSourceAdapterRejectsExecutableDrift(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 1)
	driverPath := filepath.Join(t.TempDir(), "driver")
	if err := os.WriteFile(driverPath, []byte("before"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := runSourceAdapterWithRunner(procedurePath, driverPath, nil, filepath.Join(t.TempDir(), "run"), func(_ context.Context, _ string, _ []string, data []byte) ([]byte, error) {
		if err := os.WriteFile(driverPath, []byte("after"), 0o700); err != nil {
			t.Fatal(err)
		}
		var request SourceAdapterRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		return json.Marshal(sourceAdapterTestResponse(request))
	})
	if err == nil || !strings.Contains(err.Error(), "executable changed") {
		t.Fatalf("runSourceAdapterWithRunner() error = %v", err)
	}
}

func TestGenericSourceAdapterFlowsThroughPairsAndReplication(t *testing.T) {
	procedure := strings.Repeat("a", 64)
	traceDocument := Document{
		SchemaVersion: 1, Redacted: true, Scope: "outbound", Completeness: Complete,
		Events: []Event{{Source: "desktop", Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
	}
	baselineTrace := writeTrace(t, traceDocument)
	treatmentTrace := writeTrace(t, traceDocument)
	inputs := make([]ReplicationPairInput, 0, 2)
	for index, order := range []string{OrderBaselineTreatment, OrderTreatmentBaseline} {
		root := filepath.Join(t.TempDir(), "pair")
		baselineSession := filepath.Join(root, "baseline-session.json")
		treatmentSession := filepath.Join(root, "treatment-session.json")
		if _, err := SaveSessionPair(baselineTrace, treatmentTrace, baselineSession, treatmentSession, SessionPairInput{
			Adapter: "external-desktop-v1", AdapterVersion: 1, Source: "desktop", ProcedureSHA256: procedure,
			Scope: "outbound", Order: order,
		}); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if _, err := VerifySessionPair(baselineSession, baselineTrace, treatmentSession, treatmentTrace); err != nil {
				t.Fatal(err)
			}
		}
		inputs = append(inputs, ReplicationPairInput{
			BaselineTracePath: baselineTrace, TreatmentTracePath: treatmentTrace,
			BaselineSessionPath: baselineSession, TreatmentSessionPath: treatmentSession,
			ResetConfirmed: true,
		})
	}
	ledgerPath := filepath.Join(t.TempDir(), "replication.json")
	summary, err := SaveReplicationLedger(inputs, ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Outcome != NoChangeObserved || summary.EvidenceState != "observed" || !summary.OrderBalanced {
		t.Fatalf("generic adapter ledger summary = %#v", summary)
	}
	verified, err := VerifyReplicationLedger(ledgerPath)
	if err != nil || verified.LedgerSHA256 != summary.LedgerSHA256 {
		t.Fatalf("generic adapter ledger verification = %#v, err = %v", verified, err)
	}
}
func TestRunSourceAdapterRejectsInvalidInputsAndDrivers(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 2)
	driverPath := sourceAdapterTestDriver(t)
	validOutput := filepath.Join(t.TempDir(), "run")
	for _, test := range []struct {
		name      string
		procedure string
		driver    string
		output    string
		want      string
	}{
		{name: "missing procedure", procedure: filepath.Join(t.TempDir(), "missing.json"), driver: driverPath, output: validOutput, want: "procedure"},
		{name: "relative driver", procedure: procedurePath, driver: "driver", output: filepath.Join(t.TempDir(), "run"), want: "absolute path"},
		{name: "missing driver", procedure: procedurePath, driver: filepath.Join(t.TempDir(), "missing.exe"), output: filepath.Join(t.TempDir(), "run"), want: "unavailable"},
		{name: "empty output", procedure: procedurePath, driver: driverPath, output: "", want: "paths"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := RunSourceAdapter(test.procedure, test.driver, nil, test.output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if _, err := RunSourceAdapter(procedurePath, driverPath, []string{strings.Repeat("x", maxSourceAdapterArgBytes+1)}, filepath.Join(t.TempDir(), "run")); err == nil {
		t.Fatal("RunSourceAdapter accepted an oversized argument")
	}
	if _, err := RunSourceAdapter(procedurePath, driverPath, []string{"bad\x00arg"}, filepath.Join(t.TempDir(), "run")); err == nil {
		t.Fatal("RunSourceAdapter accepted a NUL argument")
	}
	args := make([]string, maxSourceAdapterArgs+1)
	if _, err := RunSourceAdapter(procedurePath, driverPath, args, filepath.Join(t.TempDir(), "run")); err == nil {
		t.Fatal("RunSourceAdapter accepted too many arguments")
	}
	if _, err := RunSourceAdapter(procedurePath, driverPath, nil, driverPath); err == nil {
		t.Fatal("RunSourceAdapter accepted a file as output directory")
	}
}

func TestRunSourceAdapterProcessBoundsOutputDiagnosticsAndTimeout(t *testing.T) {
	driverPath := sourceAdapterTestDriver(t)
	for _, test := range []struct {
		name string
		mode string
		want string
	}{
		{name: "stdout", mode: "stdout-overflow", want: "output exceeds limit"},
		{name: "stderr", mode: "stderr-overflow", want: "diagnostics exceed limit"},
		{name: "failure", mode: "failure", want: "adapter failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runSourceAdapterProcess(context.Background(), driverPath, sourceAdapterTestDriverArgs(test.mode), nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runSourceAdapterProcess() output length = %d, error = %v, want %q", len(output), err, test.want)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := runSourceAdapterProcess(ctx, driverPath, sourceAdapterTestDriverArgs("timeout"), nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}
	if _, err := runSourceAdapterProcess(context.Background(), filepath.Join(t.TempDir(), "missing.exe"), nil, nil); err == nil || !strings.Contains(err.Error(), "start") {
		t.Fatalf("missing driver error = %v", err)
	}
}

func TestRunSourceAdapterProcessTimeoutDoesNotWaitForDescendant(t *testing.T) {
	driverPath := sourceAdapterTestDriver(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := runSourceAdapterProcess(ctx, driverPath, sourceAdapterTestDriverArgs("descendant"), nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("descendant timeout error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("descendant timeout took %s", elapsed)
	}
}

func TestDecodeSourceAdapterContractsRejectHostileData(t *testing.T) {
	validProcedure := string(sourceAdapterProcedureJSON(1000, 2))
	for _, data := range []string{
		"",
		strings.Replace(validProcedure, `"scope"`, `"private":"value","scope"`, 1),
		strings.Replace(validProcedure, `{"schema_version":1,`, `{"schema_version":1,"schema_version":1,`, 1),
		validProcedure + "{}",
		strings.Replace(validProcedure, `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(validProcedure, `"adapter":"external-desktop-v1"`, `"adapter":"desktop"`, 1),
		strings.Replace(validProcedure, `"source":"desktop"`, `"source":"secret"`, 1),
		strings.Replace(validProcedure, `"duration_ms":1000`, `"duration_ms":99`, 1),
		strings.Replace(validProcedure, `"max_events":2`, `"max_events":0`, 1),
	} {
		if _, err := DecodeSourceAdapterProcedure([]byte(data)); err == nil {
			t.Fatalf("DecodeSourceAdapterProcedure accepted %q", data)
		}
	}
	if _, err := DecodeSourceAdapterProcedure([]byte{'{', 0xff}); err == nil {
		t.Fatal("DecodeSourceAdapterProcedure accepted invalid UTF-8")
	}
	if _, err := DecodeSourceAdapterProcedure([]byte(strings.Repeat("x", maxSourceAdapterProcedureBytes+1))); err == nil {
		t.Fatal("DecodeSourceAdapterProcedure accepted oversized input")
	}

	procedure, err := DecodeSourceAdapterProcedure(sourceAdapterProcedureJSON(1000, 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadSourceAdapterProcedure(""); err == nil {
		t.Fatal("ReadSourceAdapterProcedure accepted empty path")
	}
	if _, _, err := ReadSourceAdapterProcedure(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadSourceAdapterProcedure accepted missing path")
	}
	if digest, err := SourceAdapterProcedureSHA256(procedure); err != nil || !ValidSHA256(digest) {
		t.Fatalf("SourceAdapterProcedureSHA256() = %q, err = %v", digest, err)
	}
	if _, err := SourceAdapterProcedureSHA256(SourceAdapterProcedure{}); err == nil {
		t.Fatal("SourceAdapterProcedureSHA256 accepted invalid procedure")
	}

	request := SourceAdapterRequest{SchemaVersion: SourceAdapterRequestSchemaVersion, Challenge: strings.Repeat("a", 64), ProcedureSHA256: strings.Repeat("b", 64), Procedure: procedure}
	response := sourceAdapterTestResponse(request)
	responseData, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSourceAdapterResponse(responseData); err != nil || decoded.Trace.Scope != "outbound" {
		t.Fatalf("DecodeSourceAdapterResponse() = %#v, err = %v", decoded, err)
	}
	for _, data := range [][]byte{
		{}, []byte("not-json"), append(append([]byte(nil), responseData...), []byte("{}")...),
		[]byte(strings.Replace(string(responseData), `"schema_version":1`, `"schema_version":2`, 1)),
		[]byte(strings.Replace(string(responseData), `"challenge":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"challenge":"bad"`, 1)),
		[]byte(strings.Replace(string(responseData), `"procedure_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`, `"procedure_sha256":"bad"`, 1)),
		[]byte(strings.Replace(string(responseData), `"trace":`, `"trace":{"schema_version":2},"ignored":`, 1)),
	} {
		if _, err := DecodeSourceAdapterResponse(data); err == nil {
			t.Fatalf("DecodeSourceAdapterResponse accepted hostile data: %s", data)
		}
	}
	if _, err := DecodeSourceAdapterResponse([]byte(strings.Repeat("x", maxSourceAdapterResponseBytes+1))); err == nil {
		t.Fatal("DecodeSourceAdapterResponse accepted oversized input")
	}

	receipt := SourceAdapterReceipt{
		SchemaVersion: SourceAdapterReceiptSchemaVersion, Adapter: procedure.Adapter, AdapterVersion: procedure.AdapterVersion,
		Source: procedure.Source, Scope: procedure.Scope, Completeness: Complete, Events: 1,
		ProcedureSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ChallengeSHA256: strings.Repeat("c", 64),
		TraceSHA256: strings.Repeat("d", 64), SessionSHA256: strings.Repeat("e", 64),
	}
	receiptData, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSourceAdapterReceipt(receiptData); err != nil || decoded.Adapter != receipt.Adapter {
		t.Fatalf("DecodeSourceAdapterReceipt() = %#v, err = %v", decoded, err)
	}
	if digest, err := SourceAdapterReceiptSHA256(receipt); err != nil || !ValidSHA256(digest) {
		t.Fatalf("SourceAdapterReceiptSHA256() = %q, err = %v", digest, err)
	}
	for _, data := range [][]byte{
		{}, []byte("not-json"), append(append([]byte(nil), receiptData...), []byte("{}")...),
		[]byte(strings.Replace(string(receiptData), `"adapter":"external-desktop-v1"`, `"adapter":"desktop"`, 1)),
		[]byte(strings.Replace(string(receiptData), `"source":"desktop"`, `"source":"secret"`, 1)),
		[]byte(strings.Replace(string(receiptData), `"completeness":"complete"`, `"completeness":"unknown"`, 1)),
	} {
		if _, err := DecodeSourceAdapterReceipt(data); err == nil {
			t.Fatalf("DecodeSourceAdapterReceipt accepted hostile data: %s", data)
		}
	}
	if _, err := DecodeSourceAdapterReceipt([]byte(strings.Repeat("x", maxSourceAdapterReceiptBytes+1))); err == nil {
		t.Fatal("DecodeSourceAdapterReceipt accepted oversized input")
	}
}

func TestSourceAdapterFileAndReceiptFailurePaths(t *testing.T) {
	invalidPath := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadSourceAdapterProcedure(invalidPath); err == nil {
		t.Fatal("ReadSourceAdapterProcedure accepted malformed input")
	}
	oversizedPath := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(oversizedPath, []byte(strings.Repeat("x", maxSourceAdapterProcedureBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadSourceAdapterProcedure(oversizedPath); err == nil {
		t.Fatal("ReadSourceAdapterProcedure accepted oversized input")
	}
	if _, err := readSourceAdapterFile(t.TempDir(), maxSourceAdapterReceiptBytes); err == nil {
		t.Fatal("readSourceAdapterFile accepted a directory")
	}
	if _, err := readSourceAdapterFile(filepath.Join(t.TempDir(), "missing"), -1); err == nil {
		t.Fatal("readSourceAdapterFile accepted a negative limit")
	}
	rootPath := filepath.VolumeName(os.Args[0]) + string(filepath.Separator)
	if info, err := sourceAdapterLstatNoSymlinkPath(rootPath); err != nil || info.Mode().IsRegular() {
		t.Fatalf("sourceAdapterLstatNoSymlinkPath(%q) = %#v, err = %v", rootPath, info, err)
	}
	if _, err := SourceAdapterReceiptSHA256(SourceAdapterReceipt{}); err == nil {
		t.Fatal("SourceAdapterReceiptSHA256 accepted an invalid receipt")
	}
	if err := writeSourceAdapterJSON(filepath.Join(t.TempDir(), "invalid.json"), make(chan int)); err == nil {
		t.Fatal("writeSourceAdapterJSON accepted an unsupported value")
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceAdapterExclusive(filepath.Join(parentFile, "child"), []byte("data")); err == nil {
		t.Fatal("writeSourceAdapterExclusive accepted a file parent")
	}
	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(existing, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceAdapterExclusive(existing, []byte("replacement")); err == nil {
		t.Fatal("writeSourceAdapterExclusive overwrote an existing file")
	}
}

func TestVerifySourceAdapterRunRejectsReceiptAndRootTampering(t *testing.T) {
	procedurePath := writeSourceAdapterProcedure(t, 5000, 2)
	driverPath := sourceAdapterTestDriver(t)
	root := filepath.Join(t.TempDir(), "run")
	if _, err := RunSourceAdapter(procedurePath, driverPath, sourceAdapterTestDriverArgs("success"), root); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, sourceAdapterReceiptFile)
	original, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	receipt := SourceAdapterReceipt{}
	if err := json.Unmarshal(original, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.TraceSHA256 = strings.Repeat("f", 64)
	tampered, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySourceAdapterRun(root); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("VerifySourceAdapterRun accepted a tampered receipt: %v", err)
	}
	if err := os.WriteFile(receiptPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySourceAdapterRun(root); err == nil {
		t.Fatal("VerifySourceAdapterRun accepted an invalid receipt")
	}
	if err := os.WriteFile(receiptPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySourceAdapterRun(root); err != nil {
		t.Fatal(err)
	}
	fileRoot := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(fileRoot, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySourceAdapterRun(fileRoot); err == nil {
		t.Fatal("VerifySourceAdapterRun accepted a file root")
	}
}

func TestSourceAdapterReadFromReturnsReaderErrors(t *testing.T) {
	buffer := sourceAdapterBoundedBuffer{limit: 32}
	if _, err := buffer.ReadFrom(sourceAdapterErrorReader{}); err == nil {
		t.Fatal("ReadFrom accepted a reader error")
	}
}

type sourceAdapterErrorReader struct{}

func (sourceAdapterErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("reader failed")
}
func TestSourceAdapterDirectoryAndBufferGuards(t *testing.T) {
	var exhausted sourceAdapterBoundedBuffer
	if _, err := exhausted.Write([]byte("x")); err == nil || !exhausted.overflow {
		t.Fatal("zero-limit buffer accepted a write")
	}

	var buffer sourceAdapterBoundedBuffer
	buffer.limit = 2
	if _, err := buffer.Write([]byte("ab")); err != nil || buffer.String() != "ab" {
		t.Fatalf("exact buffer write = %q, err = %v", buffer.String(), err)
	}
	if _, err := buffer.Write([]byte("c")); err == nil || !buffer.overflow || buffer.String() != "ab" {
		t.Fatalf("overflow buffer write = %q, err = %v", buffer.String(), err)
	}
	if _, err := buffer.Write([]byte("d")); err == nil || !buffer.overflow {
		t.Fatal("bounded buffer accepted a second overflow")
	}

	partial := sourceAdapterBoundedBuffer{limit: 2}
	if written, err := partial.Write([]byte("abc")); err == nil || written != 2 || partial.String() != "ab" || !partial.overflow {
		t.Fatalf("partial overflow write = %q, written = %d, err = %v", partial.String(), written, err)
	}
	if err := validateSourceAdapterRunDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("directory validator accepted a missing directory")
	}
	root := t.TempDir()
	if err := validateSourceAdapterRunDirectory(root); err == nil {
		t.Fatal("directory validator accepted an empty directory")
	}
	for _, name := range []string{sourceAdapterTraceFile, sourceAdapterSessionFile, sourceAdapterReceiptFile} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateSourceAdapterRunDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "extra"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateSourceAdapterRunDirectory(root); err == nil {
		t.Fatal("directory validator accepted an extra file")
	}
}

func TestSourceAdapterCommandAndIdentityGuards(t *testing.T) {
	driverPath := sourceAdapterTestDriver(t)
	if err := validateSourceAdapterCommand(driverPath, nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		driver string
		args   []string
	}{
		{name: "empty", driver: "", args: nil},
		{name: "relative", driver: "driver", args: nil},
		{name: "missing", driver: filepath.Join(t.TempDir(), "missing"), args: nil},
		{name: "too many", driver: driverPath, args: make([]string, maxSourceAdapterArgs+1)},
		{name: "oversized", driver: driverPath, args: []string{strings.Repeat("x", maxSourceAdapterArgBytes+1)}},
		{name: "nul", driver: driverPath, args: []string{"x\x00y"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateSourceAdapterCommand(test.driver, test.args); err == nil {
				t.Fatal("validateSourceAdapterCommand accepted invalid input")
			}
		})
	}
	if digest, err := sourceAdapterExecutableSHA256(driverPath); err != nil || !ValidSHA256(digest) {
		t.Fatalf("sourceAdapterExecutableSHA256() = %q, err = %v", digest, err)
	}
	if _, err := sourceAdapterExecutableSHA256(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("sourceAdapterExecutableSHA256 accepted missing executable")
	}
	if _, err := sourceAdapterExecutableSHA256(t.TempDir()); err == nil {
		t.Fatal("sourceAdapterExecutableSHA256 accepted directory")
	}
	for _, value := range []string{"", "external-", "external-Upper", "external-a_", "external-a-", "external-" + strings.Repeat("a", 60)} {
		if validExternalAdapter(value) {
			t.Fatalf("validExternalAdapter(%q) = true", value)
		}
	}
	for _, value := range []string{"external-a", "external-desktop-v1", "external-a1-b2"} {
		if !validExternalAdapter(value) {
			t.Fatalf("validExternalAdapter(%q) = false", value)
		}
	}
	if _, ok := adapterSource("external-desktop-v1"); !ok {
		t.Fatal("adapterSource rejected external adapter")
	}
	if !validAdapterSource("external-desktop-v1", "desktop") || validAdapterSource("external-desktop-v1", "secret") {
		t.Fatal("validAdapterSource external mapping is incorrect")
	}
	if _, ok := sessionSourceForAdapter("external-desktop-v1", "desktop"); !ok {
		t.Fatal("sessionSourceForAdapter rejected valid source")
	}
	if _, ok := sessionSourceForAdapter("external-desktop-v1", ""); ok {
		t.Fatal("sessionSourceForAdapter accepted empty source")
	}
}

func TestSourceAdapterExecutableSizeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized-driver")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Truncate(int64(maxSourceAdapterExecutableBytes) + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceAdapterExecutableSHA256(path); err == nil {
		t.Fatal("sourceAdapterExecutableSHA256 accepted an oversized executable")
	}
}

func TestSourceAdapterPathSafetyRejectsIrregular(t *testing.T) {
	if err := sourceAdapterPathSafetyError(sourceAdapterFileInfo{mode: os.ModeIrregular}); err == nil {
		t.Fatal("sourceAdapterPathSafetyError accepted an irregular path component")
	}
}

type sourceAdapterFileInfo struct {
	mode os.FileMode
}

func (info sourceAdapterFileInfo) Name() string      { return "test" }
func (sourceAdapterFileInfo) Size() int64            { return 0 }
func (info sourceAdapterFileInfo) Mode() os.FileMode { return info.mode }
func (sourceAdapterFileInfo) ModTime() time.Time     { return time.Time{} }
func (info sourceAdapterFileInfo) IsDir() bool       { return info.mode.IsDir() }
func (sourceAdapterFileInfo) Sys() any               { return nil }

func TestSourceAdapterPortableDirectoryRejectsSymlinkWhenSupported(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "target"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readSourceAdapterFile(link, maxSourceAdapterReceiptBytes); err == nil {
		t.Fatal("readSourceAdapterFile accepted a symlink")
	}
	if err := validateSourceAdapterRunDirectory(root); err == nil {
		t.Fatal("directory validator accepted a symlink")
	}
}

func sourceAdapterTestResponse(request SourceAdapterRequest) SourceAdapterResponse {
	return SourceAdapterResponse{
		SchemaVersion: SourceAdapterResponseSchemaVersion,
		Challenge:     request.Challenge, ProcedureSHA256: request.ProcedureSHA256,
		Trace: Document{
			SchemaVersion: 1, Redacted: true, Scope: request.Procedure.Scope, Completeness: Complete,
			Events: []Event{{Source: request.Procedure.Source, Channel: "network", Kind: "request", Destination: "analytics", Fields: []string{"region"}}},
		},
	}
}

func sourceAdapterTestDriver(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func sourceAdapterTestDriverArgs(mode string) []string {
	return []string{"-test.run=^$", "--", "--ariadne-source-adapter-mode=" + mode}
}

func writeSourceAdapterProcedure(t *testing.T, durationMS, maxEvents int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "procedure.json")
	if err := os.WriteFile(path, sourceAdapterProcedureJSON(durationMS, maxEvents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func sourceAdapterProcedureJSON(durationMS, maxEvents int) []byte {
	data, _ := json.Marshal(SourceAdapterProcedure{
		SchemaVersion: SourceAdapterProcedureSchemaVersion, Adapter: "external-desktop-v1", AdapterVersion: 1,
		Source: "desktop", Scope: "outbound", DurationMS: durationMS, MaxEvents: maxEvents,
	})
	return data
}

func TestSourceAdapterChallengeCommitment(t *testing.T) {
	challenge, commitment, err := newSourceAdapterChallenge()
	if err != nil || !validChallenge(challenge) || !ValidSHA256(commitment) {
		t.Fatalf("newSourceAdapterChallenge() = %q, %q, %v", challenge, commitment, err)
	}
	raw, err := hex.DecodeString(challenge)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if commitment != hex.EncodeToString(digest[:]) {
		t.Fatalf("challenge commitment = %q, want %x", commitment, digest)
	}
}

func TestSourceAdapterReceiptProvenanceBinding(t *testing.T) {
	procedure := SourceAdapterProcedure{
		SchemaVersion:  SourceAdapterProcedureSchemaVersion,
		Adapter:        "external-desktop-v1",
		AdapterVersion: 1,
		Source:         "desktop",
		Scope:          "outbound",
		DurationMS:     5000,
		MaxEvents:      1,
	}
	procedureSHA256, err := SourceAdapterProcedureSHA256(procedure)
	if err != nil {
		t.Fatal(err)
	}
	provenanceSHA256, err := sourceAdapterProvenanceSHA256(
		procedure.Adapter,
		procedure.AdapterVersion,
		procedure.Source,
		procedure.Scope,
		procedureSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt := SourceAdapterReceipt{
		SchemaVersion:    SourceAdapterReceiptSchemaVersion,
		Adapter:          procedure.Adapter,
		AdapterVersion:   procedure.AdapterVersion,
		Source:           procedure.Source,
		Scope:            procedure.Scope,
		Completeness:     Complete,
		Events:           1,
		ProcedureSHA256:  procedureSHA256,
		ProvenanceSHA256: provenanceSHA256,
		ExecutableSHA256: strings.Repeat("b", 64),
		ChallengeSHA256:  strings.Repeat("c", 64),
		TraceSHA256:      strings.Repeat("d", 64),
		SessionSHA256:    strings.Repeat("e", 64),
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSourceAdapterReceipt(data); err != nil {
		t.Fatalf("DecodeSourceAdapterReceipt() error = %v", err)
	}
	tampered := receipt
	tampered.ProvenanceSHA256 = strings.Repeat("f", 64)
	data, err = json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSourceAdapterReceipt(data); err == nil {
		t.Fatal("DecodeSourceAdapterReceipt() accepted a tampered provenance digest")
	}
	legacy := receipt
	legacy.ProvenanceSHA256 = ""
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSourceAdapterReceipt(data); err != nil || decoded.ProvenanceSHA256 != "" {
		t.Fatalf("legacy receipt = %#v, error = %v", decoded, err)
	}
}
