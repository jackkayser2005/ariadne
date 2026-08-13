package browser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	minCaptureDurationMS = 100
	maxCaptureDurationMS = 5 * 60 * 1000
	maxDriverStderrBytes = 64 << 10
	captureCleanupGrace  = 15 * time.Second
)

// CaptureSummary identifies the verified procedure and trace produced by a
// driver without exposing driver output or source-specific data.
type CaptureSummary struct {
	ProcedureSHA256 string                            `json:"procedure_sha256"`
	Trace           portabletrace.VerificationSummary `json:"trace"`
}

// Capture invokes one explicitly selected driver and stores its redacted
// audit as a portable trace. The driver receives the validated procedure JSON
// on stdin and must write exactly one redacted Audit JSON document to stdout.
func Capture(procedurePath, driverPath string, driverArgs []string, outputPath string) (CaptureSummary, error) {
	return captureWithRunner(procedurePath, driverPath, driverArgs, outputPath, runDriver)
}

type captureRunner func(context.Context, string, []string, []byte) ([]byte, error)

func captureWithRunner(procedurePath, driverPath string, driverArgs []string, outputPath string, run captureRunner) (CaptureSummary, error) {
	if strings.TrimSpace(procedurePath) == "" || strings.TrimSpace(driverPath) == "" || strings.TrimSpace(outputPath) == "" {
		return CaptureSummary{}, errors.New("browser capture paths and driver are required")
	}
	procedure, procedureData, err := ReadProcedure(procedurePath)
	if err != nil {
		return CaptureSummary{}, fmt.Errorf("browser procedure: %w", err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		return CaptureSummary{}, errors.New("browser procedure identity failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(procedure.DurationMS)*time.Millisecond+captureCleanupGrace)
	defer cancel()
	auditData, err := run(ctx, driverPath, driverArgs, procedureData)
	if err != nil {
		return CaptureSummary{}, fmt.Errorf("browser capture driver: %w", err)
	}
	audit, err := Decode(auditData)
	if err != nil {
		return CaptureSummary{}, fmt.Errorf("browser capture output: %w", err)
	}
	if audit.Scope != procedure.Scope || len(audit.Events) > procedure.MaxEvents {
		return CaptureSummary{}, errors.New("browser capture output does not match procedure")
	}

	traceSummary, err := saveAudit(audit, outputPath)
	if err != nil {
		return CaptureSummary{}, err
	}
	return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: traceSummary}, nil
}

func runDriver(ctx context.Context, driverPath string, driverArgs []string, procedureData []byte) ([]byte, error) {
	if strings.TrimSpace(driverPath) == "" {
		return nil, errors.New("driver is required")
	}
	command := exec.CommandContext(ctx, driverPath, driverArgs...)
	command.Stdin = bytes.NewReader(procedureData)
	stdout := &boundedBuffer{limit: maxAuditBytes}
	stderr := &boundedBuffer{limit: maxDriverStderrBytes}
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	if stdout.overflow {
		return nil, errors.New("driver output exceeds limit")
	}
	if stderr.overflow {
		return nil, errors.New("driver diagnostics exceed limit")
	}
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("driver timed out")
		}
		return nil, errors.New("driver failed")
	}
	return stdout.Bytes(), nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - buffer.Len()
	if remaining <= 0 {
		buffer.overflow = true
		return 0, errors.New("buffer limit")
	}
	if len(data) > remaining {
		_, _ = buffer.Buffer.Write(data[:remaining])
		buffer.overflow = true
		return remaining, errors.New("buffer limit")
	}
	return buffer.Buffer.Write(data)
}

func (buffer *boundedBuffer) ReadFrom(reader io.Reader) (int64, error) {
	var chunk [32 << 10]byte
	var total int64
	for {
		count, readErr := reader.Read(chunk[:])
		if count > 0 {
			written, writeErr := buffer.Write(chunk[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func saveAudit(audit Audit, outputPath string) (portabletrace.VerificationSummary, error) {
	document := documentFromAudit(audit)
	encoded, err := json.Marshal(document)
	if err != nil {
		return portabletrace.VerificationSummary{}, errors.New("browser trace encoding failed")
	}
	encoded = append(encoded, '\n')
	if err := writeExclusive(outputPath, encoded); err != nil {
		return portabletrace.VerificationSummary{}, fmt.Errorf("browser trace: %w", err)
	}
	summary, err := portabletrace.Verify(outputPath)
	if err != nil {
		return portabletrace.VerificationSummary{}, errors.New("browser trace verification failed")
	}
	return summary, nil
}

func documentFromAudit(audit Audit) portabletrace.Document {
	events := make([]portabletrace.Event, len(audit.Events))
	for index, event := range audit.Events {
		events[index] = portabletrace.Event{
			Source:      browserTraceSource,
			Channel:     event.Channel,
			Kind:        event.Kind,
			Destination: event.Destination,
			Fields:      append([]string(nil), event.Fields...),
		}
	}
	return portabletrace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         audit.Scope,
		Completeness:  audit.Completeness,
		Events:        events,
	}
}

func procedureDigest(procedure Procedure) (string, error) {
	data, err := json.Marshal(procedure)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
