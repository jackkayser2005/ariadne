package trace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	// SourceAdapterProcedureSchemaVersion is the version of the generic
	// source-adapter procedure sent to an adapter process.
	SourceAdapterProcedureSchemaVersion = 1
	// SourceAdapterRequestSchemaVersion is the version of the one-shot request
	// sent to an adapter process.
	SourceAdapterRequestSchemaVersion = 1
	// SourceAdapterResponseSchemaVersion is the version of the generic
	// source-adapter response returned by an adapter process.
	SourceAdapterResponseSchemaVersion = 1
	// SourceAdapterReceiptSchemaVersion is the version of the portable receipt
	// written for one generic source-adapter run.
	SourceAdapterReceiptSchemaVersion = 1

	maxSourceAdapterProcedureBytes  = 16 << 10
	maxSourceAdapterResponseBytes   = maxDocumentBytes + 32<<10
	maxSourceAdapterReceiptBytes    = 64 << 10
	maxSourceAdapterStderrBytes     = 64 << 10
	maxSourceAdapterExecutableBytes = 256 << 20
	minSourceAdapterDurationMS      = 100
	maxSourceAdapterDurationMS      = 5 * 60 * 1000
	sourceAdapterWaitDelay          = time.Second
	sourceAdapterTerminateTimeout   = time.Second
	maxSourceAdapterIDBytes         = 64
	maxSourceAdapterArgs            = 64
	maxSourceAdapterArgBytes        = 4 << 10
	maxSourceAdapterArgsBytes       = 16 << 10
	sourceAdapterIDPrefix           = "external-"
	sourceAdapterTraceFile          = "trace.json"
	sourceAdapterSessionFile        = "session.json"
	sourceAdapterReceiptFile        = "receipt.json"
)

// SourceAdapterProcedure is the bounded, raw-value-free input shared with one
// explicitly selected source adapter executable.
type SourceAdapterProcedure struct {
	SchemaVersion  int    `json:"schema_version"`
	Adapter        string `json:"adapter"`
	AdapterVersion int    `json:"adapter_version"`
	Source         string `json:"source"`
	Scope          string `json:"scope"`
	DurationMS     int    `json:"duration_ms"`
	MaxEvents      int    `json:"max_events"`
}

// SourceAdapterRequest is the one-shot stdin message sent to an adapter. The
// challenge is transient process input and is never written to a receipt.
type SourceAdapterRequest struct {
	SchemaVersion   int                    `json:"schema_version"`
	Challenge       string                 `json:"challenge"`
	ProcedureSHA256 string                 `json:"procedure_sha256"`
	Procedure       SourceAdapterProcedure `json:"procedure"`
}

// SourceAdapterResponse is the one bounded stdout message accepted from an
// adapter after it has removed payloads, URLs, and source-specific values.
type SourceAdapterResponse struct {
	SchemaVersion   int      `json:"schema_version"`
	Challenge       string   `json:"challenge"`
	ProcedureSHA256 string   `json:"procedure_sha256"`
	Trace           Document `json:"trace"`
}

// SourceAdapterReceipt is a portable identity binding for one adapter run.
// It contains no challenge, executable, procedure, or trace values.
type SourceAdapterReceipt struct {
	SchemaVersion    int    `json:"schema_version"`
	Adapter          string `json:"adapter"`
	AdapterVersion   int    `json:"adapter_version"`
	Source           string `json:"source"`
	Scope            string `json:"scope"`
	Completeness     string `json:"completeness"`
	Events           int    `json:"events"`
	ProcedureSHA256  string `json:"procedure_sha256"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ChallengeSHA256  string `json:"challenge_sha256"`
	TraceSHA256      string `json:"trace_sha256"`
	SessionSHA256    string `json:"session_sha256"`
}

// SourceAdapterRunSummary identifies a verified adapter run without exposing
// adapter output or source-specific values.
type SourceAdapterRunSummary struct {
	ReceiptSHA256 string                     `json:"receipt_sha256"`
	Receipt       SourceAdapterReceipt       `json:"receipt"`
	Trace         VerificationSummary        `json:"trace"`
	Session       SessionVerificationSummary `json:"session"`
}

// ReadSourceAdapterProcedure reads and validates one bounded procedure file.
func ReadSourceAdapterProcedure(path string) (SourceAdapterProcedure, []byte, error) {
	if strings.TrimSpace(path) == "" {
		return SourceAdapterProcedure{}, nil, errors.New("source adapter procedure path is required")
	}
	data, err := readSourceAdapterFile(path, maxSourceAdapterProcedureBytes)
	if err != nil {
		return SourceAdapterProcedure{}, nil, errors.New("read procedure")
	}
	procedure, err := DecodeSourceAdapterProcedure(data)
	if err != nil {
		return SourceAdapterProcedure{}, nil, err
	}
	return procedure, data, nil
}

// DecodeSourceAdapterProcedure validates one bounded source-adapter
// procedure without exposing hostile input in its errors.
func DecodeSourceAdapterProcedure(data []byte) (SourceAdapterProcedure, error) {
	if len(data) == 0 || len(data) > maxSourceAdapterProcedureBytes || !utf8.Valid(data) {
		return SourceAdapterProcedure{}, errors.New("source adapter procedure is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return SourceAdapterProcedure{}, errors.New("source adapter procedure is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var procedure SourceAdapterProcedure
	if err := decoder.Decode(&procedure); err != nil {
		return SourceAdapterProcedure{}, errors.New("source adapter procedure is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SourceAdapterProcedure{}, errors.New("source adapter procedure is invalid")
	}
	if err := validateSourceAdapterProcedure(procedure); err != nil {
		return SourceAdapterProcedure{}, err
	}
	return procedure, nil
}

// SourceAdapterProcedureSHA256 returns the canonical identity of a validated
// source-adapter procedure.
func SourceAdapterProcedureSHA256(procedure SourceAdapterProcedure) (string, error) {
	if err := validateSourceAdapterProcedure(procedure); err != nil {
		return "", err
	}
	data, err := json.Marshal(procedure)
	if err != nil {
		return "", errors.New("source adapter procedure identity failed")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// DecodeSourceAdapterResponse validates one bounded response and its nested
// trace. The challenge and procedure binding are checked by RunSourceAdapter.
func DecodeSourceAdapterResponse(data []byte) (SourceAdapterResponse, error) {
	if len(data) == 0 || len(data) > maxSourceAdapterResponseBytes || !utf8.Valid(data) {
		return SourceAdapterResponse{}, errors.New("source adapter response is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return SourceAdapterResponse{}, errors.New("source adapter response is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var response SourceAdapterResponse
	if err := decoder.Decode(&response); err != nil {
		return SourceAdapterResponse{}, errors.New("source adapter response is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SourceAdapterResponse{}, errors.New("source adapter response is invalid")
	}
	if response.SchemaVersion != SourceAdapterResponseSchemaVersion ||
		!validChallenge(response.Challenge) || !ValidSHA256(response.ProcedureSHA256) {
		return SourceAdapterResponse{}, errors.New("source adapter response is invalid")
	}
	if _, err := SHA256(response.Trace); err != nil {
		return SourceAdapterResponse{}, errors.New("source adapter response trace is invalid")
	}
	return response, nil
}

// DecodeSourceAdapterReceipt validates a bounded portable adapter receipt.
func DecodeSourceAdapterReceipt(data []byte) (SourceAdapterReceipt, error) {
	if len(data) == 0 || len(data) > maxSourceAdapterReceiptBytes || !utf8.Valid(data) {
		return SourceAdapterReceipt{}, errors.New("source adapter receipt is invalid")
	}
	if err := jsoncheck.RejectDuplicateKeys(data); err != nil {
		return SourceAdapterReceipt{}, errors.New("source adapter receipt is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var receipt SourceAdapterReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return SourceAdapterReceipt{}, errors.New("source adapter receipt is invalid")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SourceAdapterReceipt{}, errors.New("source adapter receipt is invalid")
	}
	if err := validateSourceAdapterReceipt(receipt); err != nil {
		return SourceAdapterReceipt{}, err
	}
	return receipt, nil
}

// SourceAdapterReceiptSHA256 returns the canonical identity of a valid
// source-adapter receipt.
func SourceAdapterReceiptSHA256(receipt SourceAdapterReceipt) (string, error) {
	if err := validateSourceAdapterReceipt(receipt); err != nil {
		return "", err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return "", errors.New("source adapter receipt identity failed")
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// RunSourceAdapter invokes one explicitly selected adapter executable through
// the bounded stdin/stdout contract and atomically publishes trace.json,
// session.json, and receipt.json.
func RunSourceAdapter(procedurePath, executable string, args []string, outputDir string) (SourceAdapterRunSummary, error) {
	return runSourceAdapterWithRunner(procedurePath, executable, args, outputDir, runSourceAdapterProcess)
}

type sourceAdapterProcessRunner func(context.Context, string, []string, []byte) ([]byte, error)

func runSourceAdapterWithRunner(procedurePath, executable string, args []string, outputDir string, run sourceAdapterProcessRunner) (SourceAdapterRunSummary, error) {
	if strings.TrimSpace(procedurePath) == "" || strings.TrimSpace(executable) == "" || strings.TrimSpace(outputDir) == "" {
		return SourceAdapterRunSummary{}, errors.New("source adapter paths and driver are required")
	}
	procedure, _, err := ReadSourceAdapterProcedure(procedurePath)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter procedure: %w", err)
	}
	procedureSHA256, err := SourceAdapterProcedureSHA256(procedure)
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter procedure identity failed")
	}
	if err := validateSourceAdapterCommand(executable, args); err != nil {
		return SourceAdapterRunSummary{}, err
	}
	executableSHA256, err := sourceAdapterExecutableSHA256(executable)
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter executable identity failed")
	}
	challenge, challengeSHA256, err := newSourceAdapterChallenge()
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter challenge failed")
	}
	requestData, err := json.Marshal(SourceAdapterRequest{
		SchemaVersion:   SourceAdapterRequestSchemaVersion,
		Challenge:       challenge,
		ProcedureSHA256: procedureSHA256,
		Procedure:       procedure,
	})
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter request encoding failed")
	}
	requestData = append(requestData, '\n')
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(procedure.DurationMS)*time.Millisecond)
	defer cancel()
	responseData, err := run(ctx, executable, args, requestData)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter process: %w", err)
	}
	response, err := DecodeSourceAdapterResponse(responseData)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter response: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(response.Challenge), []byte(challenge)) != 1 {
		return SourceAdapterRunSummary{}, errors.New("source adapter response challenge does not match request")
	}
	if response.ProcedureSHA256 != procedureSHA256 {
		return SourceAdapterRunSummary{}, errors.New("source adapter response procedure does not match request")
	}
	if response.Trace.Scope != procedure.Scope || len(response.Trace.Events) > procedure.MaxEvents {
		return SourceAdapterRunSummary{}, errors.New("source adapter response does not match procedure")
	}
	for _, event := range response.Trace.Events {
		if event.Source != procedure.Source {
			return SourceAdapterRunSummary{}, errors.New("source adapter response source does not match procedure")
		}
	}
	traceSHA256, err := SHA256(response.Trace)
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter trace identity failed")
	}

	cleanOutputDir := filepath.Clean(outputDir)
	if _, err := os.Lstat(cleanOutputDir); err == nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return SourceAdapterRunSummary{}, errors.New("source adapter output is unavailable")
	}
	parent := filepath.Dir(cleanOutputDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter output directory failed")
	}
	temporaryDir, err := os.MkdirTemp(parent, ".ariadne-source-adapter-*")
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter temporary directory failed")
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporaryDir)
		}
	}()
	tracePath := filepath.Join(temporaryDir, sourceAdapterTraceFile)
	if err := writeSourceAdapterJSON(tracePath, response.Trace); err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter trace: %w", err)
	}
	sessionSummary, err := SaveSession(tracePath, filepath.Join(temporaryDir, sourceAdapterSessionFile), SessionInput{
		Adapter:         procedure.Adapter,
		AdapterVersion:  procedure.AdapterVersion,
		Source:          procedure.Source,
		ProcedureSHA256: procedureSHA256,
		Role:            RoleStandalone,
		Order:           OrderStandalone,
	})
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter session: %w", err)
	}
	receipt := SourceAdapterReceipt{
		SchemaVersion:    SourceAdapterReceiptSchemaVersion,
		Adapter:          procedure.Adapter,
		AdapterVersion:   procedure.AdapterVersion,
		Source:           procedure.Source,
		Scope:            response.Trace.Scope,
		Completeness:     response.Trace.Completeness,
		Events:           len(response.Trace.Events),
		ProcedureSHA256:  procedureSHA256,
		ExecutableSHA256: executableSHA256,
		ChallengeSHA256:  challengeSHA256,
		TraceSHA256:      traceSHA256,
		SessionSHA256:    sessionSummary.SessionSHA256,
	}
	if err := writeSourceAdapterJSON(filepath.Join(temporaryDir, sourceAdapterReceiptFile), receipt); err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter receipt: %w", err)
	}
	verified, err := verifySourceAdapterRun(temporaryDir)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter verification: %w", err)
	}
	if err := os.Rename(temporaryDir, cleanOutputDir); err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter output publish failed")
	}
	published = true
	return verified, nil
}

// VerifySourceAdapterRun verifies a copied adapter run without reopening the
// procedure or launching the source adapter again.
func VerifySourceAdapterRun(rootDir string) (SourceAdapterRunSummary, error) {
	if strings.TrimSpace(rootDir) == "" {
		return SourceAdapterRunSummary{}, errors.New("source adapter run directory is required")
	}
	return verifySourceAdapterRun(rootDir)
}

func verifySourceAdapterRun(rootDir string) (SourceAdapterRunSummary, error) {
	if err := validateSourceAdapterRunDirectory(rootDir); err != nil {
		return SourceAdapterRunSummary{}, err
	}
	receiptData, err := readSourceAdapterFile(filepath.Join(rootDir, sourceAdapterReceiptFile), maxSourceAdapterReceiptBytes)
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter receipt read failed")
	}
	receipt, err := DecodeSourceAdapterReceipt(receiptData)
	if err != nil {
		return SourceAdapterRunSummary{}, err
	}
	tracePath := filepath.Join(rootDir, sourceAdapterTraceFile)
	sessionPath := filepath.Join(rootDir, sourceAdapterSessionFile)
	traceSummary, err := Verify(tracePath)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter trace: %w", err)
	}
	sessionSummary, err := VerifySession(sessionPath, tracePath)
	if err != nil {
		return SourceAdapterRunSummary{}, fmt.Errorf("source adapter session: %w", err)
	}
	if receipt.Adapter != sessionSummary.Adapter || receipt.AdapterVersion != sessionSummary.AdapterVersion ||
		receipt.Source != sessionSummary.Source || receipt.Scope != sessionSummary.Scope ||
		receipt.Completeness != sessionSummary.Completeness || receipt.ProcedureSHA256 != sessionSummary.ProcedureSHA256 ||
		receipt.TraceSHA256 != traceSummary.TraceSHA256 || receipt.SessionSHA256 != sessionSummary.SessionSHA256 ||
		receipt.Events != traceSummary.Events {
		return SourceAdapterRunSummary{}, errors.New("source adapter receipt does not match verified artifacts")
	}
	receiptSHA256, err := SourceAdapterReceiptSHA256(receipt)
	if err != nil {
		return SourceAdapterRunSummary{}, errors.New("source adapter receipt identity failed")
	}
	return SourceAdapterRunSummary{ReceiptSHA256: receiptSHA256, Receipt: receipt, Trace: traceSummary, Session: sessionSummary}, nil
}

func validateSourceAdapterProcedure(procedure SourceAdapterProcedure) error {
	if procedure.SchemaVersion != SourceAdapterProcedureSchemaVersion || !validExternalAdapter(procedure.Adapter) ||
		procedure.AdapterVersion < 1 || procedure.AdapterVersion > maxAdapterVersion || !validSource(procedure.Source) ||
		!validScope(procedure.Scope) || procedure.DurationMS < minSourceAdapterDurationMS ||
		procedure.DurationMS > maxSourceAdapterDurationMS || procedure.MaxEvents < 1 || procedure.MaxEvents > maxEvents {
		return errors.New("source adapter procedure is invalid")
	}
	return nil
}

func validateSourceAdapterReceipt(receipt SourceAdapterReceipt) error {
	if receipt.SchemaVersion != SourceAdapterReceiptSchemaVersion || !validExternalAdapter(receipt.Adapter) ||
		receipt.AdapterVersion < 1 || receipt.AdapterVersion > maxAdapterVersion ||
		!validAdapterSource(receipt.Adapter, receipt.Source) || !validScope(receipt.Scope) ||
		(receipt.Completeness != Complete && receipt.Completeness != Partial) || receipt.Events < 0 ||
		receipt.Events > maxEvents || !ValidSHA256(receipt.ProcedureSHA256) ||
		!ValidSHA256(receipt.ExecutableSHA256) || !ValidSHA256(receipt.ChallengeSHA256) ||
		!ValidSHA256(receipt.TraceSHA256) || !ValidSHA256(receipt.SessionSHA256) {
		return errors.New("source adapter receipt is invalid")
	}
	return nil
}

func validExternalAdapter(value string) bool {
	if len(value) <= len(sourceAdapterIDPrefix) || len(value) > maxSourceAdapterIDBytes || !strings.HasPrefix(value, sourceAdapterIDPrefix) {
		return false
	}
	if strings.HasSuffix(value, "-") {
		return false
	}
	for _, character := range value[len(sourceAdapterIDPrefix):] {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func validAdapterSource(adapter, source string) bool {
	expected, ok := adapterSource(adapter)
	if !ok || !validSource(source) {
		return false
	}
	return expected == "" || expected == source
}

func validChallenge(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func newSourceAdapterChallenge() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	challenge := hex.EncodeToString(raw)
	digest := sha256.Sum256(raw)
	return challenge, hex.EncodeToString(digest[:]), nil
}

func validateSourceAdapterCommand(executable string, args []string) error {
	if strings.TrimSpace(executable) == "" || !filepath.IsAbs(executable) || strings.ContainsRune(executable, '\x00') {
		return errors.New("source adapter driver must be an absolute path")
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("source adapter driver is unavailable")
	}
	if len(args) > maxSourceAdapterArgs {
		return errors.New("source adapter process arguments are invalid")
	}
	total := 0
	for _, arg := range args {
		if len(arg) > maxSourceAdapterArgBytes || strings.ContainsRune(arg, '\x00') || total > maxSourceAdapterArgsBytes-len(arg) {
			return errors.New("source adapter process arguments are invalid")
		}
		total += len(arg)
	}
	return nil
}

func sourceAdapterExecutableSHA256(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("executable is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	count, err := io.Copy(hasher, io.LimitReader(file, maxSourceAdapterExecutableBytes+1))
	if err != nil || count > maxSourceAdapterExecutableBytes {
		return "", errors.New("executable exceeds limit")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func runSourceAdapterProcess(ctx context.Context, executable string, args []string, request []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.WaitDelay = sourceAdapterWaitDelay
	command.Env = sourceAdapterEnvironment()
	command.Stdin = bytes.NewReader(request)
	stdout := &sourceAdapterBoundedBuffer{limit: maxSourceAdapterResponseBytes}
	stderr := &sourceAdapterBoundedBuffer{limit: maxSourceAdapterStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, errors.New("adapter failed to start")
	}
	processDone := make(chan error, 1)
	go func() { processDone <- command.Wait() }()
	select {
	case runErr := <-processDone:
		if stdout.overflow {
			return nil, errors.New("adapter output exceeds limit")
		}
		if stderr.overflow {
			return nil, errors.New("adapter diagnostics exceed limit")
		}
		if runErr != nil {
			return nil, errors.New("adapter failed")
		}
		return append([]byte(nil), stdout.Bytes()...), nil
	case <-ctx.Done():
		terminateSourceAdapterProcess(command.Process)
		<-processDone
		return nil, errors.New("adapter timed out")
	}
}

func sourceAdapterEnvironment() []string {
	allowed := map[string]struct{}{
		"APPDATA": {}, "COMMONPROGRAMFILES": {}, "COMMONPROGRAMFILES(X86)": {},
		"COMSPEC": {}, "HOME": {}, "HOMEDRIVE": {}, "HOMEPATH": {},
		"LANG": {}, "LANGUAGE": {}, "LC_ALL": {}, "LC_CTYPE": {}, "LC_MESSAGES": {},
		"LOCALAPPDATA": {}, "PATH": {}, "PATHEXT": {}, "PROGRAMDATA": {},
		"PROGRAMFILES": {}, "PROGRAMFILES(X86)": {}, "SYSTEMDRIVE": {},
		"SYSTEMROOT": {}, "TEMP": {}, "TMP": {}, "TMPDIR": {}, "TZ": {},
		"USERPROFILE": {}, "WINDIR": {},
	}
	result := make([]string, 0, len(allowed))
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[strings.ToUpper(name)]; ok {
			result = append(result, entry)
		}
	}
	return result
}

func terminateSourceAdapterProcess(process *os.Process) {
	if process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		cleanupContext, cancel := context.WithTimeout(context.Background(), sourceAdapterTerminateTimeout)
		defer cancel()
		command := exec.CommandContext(cleanupContext, "taskkill.exe", "/PID", strconv.Itoa(process.Pid), "/T", "/F")
		command.WaitDelay = sourceAdapterWaitDelay
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err == nil {
			return
		}
	}
	_ = process.Kill()
}

type sourceAdapterBoundedBuffer struct {
	data     []byte
	limit    int
	overflow bool
}

func (buffer *sourceAdapterBoundedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - buffer.Len()
	if remaining <= 0 {
		buffer.overflow = true
		return 0, errors.New("buffer limit")
	}
	if len(data) > remaining {
		buffer.data = append(buffer.data, data[:remaining]...)
		buffer.overflow = true
		return remaining, errors.New("buffer limit")
	}
	buffer.data = append(buffer.data, data...)
	return len(data), nil
}

func (buffer *sourceAdapterBoundedBuffer) ReadFrom(reader io.Reader) (int64, error) {
	var total int64
	chunk := make([]byte, 32<<10)
	for {
		count, err := reader.Read(chunk)
		if count > 0 {
			written, writeErr := buffer.Write(chunk[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func (buffer *sourceAdapterBoundedBuffer) Bytes() []byte {
	return buffer.data
}

func (buffer *sourceAdapterBoundedBuffer) Len() int {
	return len(buffer.data)
}

func (buffer *sourceAdapterBoundedBuffer) String() string {
	return string(buffer.data)
}
func readSourceAdapterFile(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("input exceeds limit")
	}
	return data, nil
}

func writeSourceAdapterJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return errors.New("encode output")
	}
	data = append(data, '\n')
	if err := writeSourceAdapterExclusive(path, data); err != nil {
		return err
	}
	return nil
}

func writeSourceAdapterExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("create output directory")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("create output")
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return errors.New("write output")
	}
	if err := file.Sync(); err != nil {
		return errors.New("sync output")
	}
	if err := file.Close(); err != nil {
		return errors.New("close output")
	}
	remove = false
	return nil
}

func validateSourceAdapterRunDirectory(rootDir string) error {
	info, err := os.Lstat(rootDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("source adapter run directory is invalid")
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil || len(entries) != 3 {
		return errors.New("source adapter run directory is invalid")
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.Name()]; exists {
			return errors.New("source adapter run directory is invalid")
		}
		seen[entry.Name()] = struct{}{}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source adapter run directory contains symbolic links")
		}
		path := filepath.Join(rootDir, entry.Name())
		entryInfo, err := os.Lstat(path)
		if err != nil || !entryInfo.Mode().IsRegular() {
			return errors.New("source adapter run directory is invalid")
		}
	}
	for _, name := range []string{sourceAdapterTraceFile, sourceAdapterSessionFile, sourceAdapterReceiptFile} {
		if _, ok := seen[name]; !ok {
			return errors.New("source adapter run directory is invalid")
		}
	}
	return nil
}
