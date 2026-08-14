package proxy

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

const (
	// Adapter is the reviewed trace-session adapter name for this producer.
	Adapter = "proxy-connect"
	// AdapterVersion is the version of the proxy adapter contract.
	AdapterVersion                = 1
	proxyUsername                 = "ariadne"
	proxyHandshakeTimeout         = 3 * time.Second
	maxHeaderBytes                = 16 << 10
	maxConcurrentProxyConnections = 64
	maxProcessArgs                = 64
	maxProcessArgBytes            = 4 << 10
	maxProcessArgsBytes           = 16 << 10
)

// CaptureSummary identifies the verified procedure and trace produced by the
// proxy without exposing process arguments, credentials, or target details.
type CaptureSummary struct {
	ProcedureSHA256 string                            `json:"procedure_sha256"`
	Trace           portabletrace.VerificationSummary `json:"trace"`
}

// Capture launches one explicitly supplied process with a fresh loopback proxy
// and stores the resulting raw-value-free trace. The proxy never terminates
// TLS or records the tunneled bytes.
func Capture(procedurePath, program string, programArgs []string, outputPath string) (CaptureSummary, error) {
	if strings.TrimSpace(procedurePath) == "" || strings.TrimSpace(outputPath) == "" {
		return CaptureSummary{}, errors.New("proxy capture paths are required")
	}
	procedure, _, err := ReadProcedure(procedurePath)
	if err != nil {
		return CaptureSummary{}, fmt.Errorf("proxy procedure: %w", err)
	}
	procedureSHA256, err := ProcedureSHA256(procedure)
	if err != nil {
		return CaptureSummary{}, errors.New("proxy procedure identity failed")
	}
	if err := validateProgram(program, programArgs); err != nil {
		return CaptureSummary{}, err
	}
	if _, err := os.Stat(outputPath); err == nil {
		return CaptureSummary{}, errors.New("proxy trace output already exists")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return CaptureSummary{}, errors.New("proxy listener failed")
	}
	credential, err := newCredential()
	if err != nil {
		_ = listener.Close()
		return CaptureSummary{}, errors.New("proxy credential failed")
	}
	proxyAddress := listener.Addr().(*net.TCPAddr)
	proxyURL := fmt.Sprintf("http://%s:%s@127.0.0.1:%d", proxyUsername, credential, proxyAddress.Port)

	captureContext, cancel := context.WithTimeout(context.Background(), time.Duration(procedure.DurationMS)*time.Millisecond)
	defer cancel()
	server := newConnectProxy(captureContext, listener, procedure, proxyUsername, credential, (&net.Dialer{
		Timeout: 3 * time.Second,
	}).DialContext)
	serverDone := make(chan struct{})
	go func() {
		server.serve()
		close(serverDone)
	}()

	command := exec.CommandContext(captureContext, program, programArgs...)
	command.Env = proxyEnvironment(proxyURL)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		server.stop()
		<-serverDone
		return CaptureSummary{}, errors.New("proxy process failed to start")
	}
	processDone := make(chan error, 1)
	go func() { processDone <- command.Wait() }()
	select {
	case <-processDone:
	case <-captureContext.Done():
		terminateProcessTree(command.Process)
		<-processDone
	}
	cancel()
	server.stop()
	<-serverDone
	server.wait()

	document := server.document()
	encoded, err := json.Marshal(document)
	if err != nil {
		return CaptureSummary{}, errors.New("proxy trace encoding failed")
	}
	encoded = append(encoded, '\n')
	if err := writeExclusive(outputPath, encoded); err != nil {
		return CaptureSummary{}, fmt.Errorf("proxy trace: %w", err)
	}
	traceSummary, err := portabletrace.Verify(outputPath)
	if err != nil {
		return CaptureSummary{}, errors.New("proxy trace verification failed")
	}
	return CaptureSummary{ProcedureSHA256: procedureSHA256, Trace: traceSummary}, nil
}

func validateProgram(program string, args []string) error {
	if strings.TrimSpace(program) == "" || !filepath.IsAbs(program) {
		return errors.New("proxy program must be an absolute path")
	}
	info, err := os.Stat(program)
	if err != nil || info.IsDir() {
		return errors.New("proxy program is unavailable")
	}
	if len(args) > maxProcessArgs {
		return errors.New("proxy process arguments are invalid")
	}
	total := 0
	for _, arg := range args {
		if len(arg) > maxProcessArgBytes || strings.ContainsRune(arg, '\x00') {
			return errors.New("proxy process arguments are invalid")
		}
		total += len(arg)
		if total > maxProcessArgsBytes {
			return errors.New("proxy process arguments are invalid")
		}
	}
	return nil
}

func newCredential() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func proxyEnvironment(proxyURL string) []string {
	allow := map[string]struct{}{
		"APPDATA": {}, "COMMONPROGRAMFILES": {}, "COMMONPROGRAMFILES(X86)": {},
		"COMSPEC": {}, "HOMEDRIVE": {}, "HOMEPATH": {}, "HOME": {},
		"LANG": {}, "LANGUAGE": {}, "LC_ALL": {}, "LC_CTYPE": {}, "LC_MESSAGES": {},
		"LOCALAPPDATA": {}, "PATH": {}, "PATHEXT": {}, "PROGRAMDATA": {},
		"PROGRAMFILES": {}, "PROGRAMFILES(X86)": {}, "SYSTEMDRIVE": {},
		"SYSTEMROOT": {}, "TEMP": {}, "TMP": {}, "TMPDIR": {}, "TZ": {},
		"USERPROFILE": {}, "WINDIR": {},
	}
	environment := make([]string, 0, len(allow)+8)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, exists := allow[strings.ToUpper(name)]; exists {
			environment = append(environment, entry)
		}
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		environment = append(environment, name+"="+proxyURL)
	}
	environment = append(environment, "NO_PROXY=", "no_proxy=")
	return environment
}

func terminateProcessTree(process *os.Process) {
	if process == nil || runtime.GOOS != "windows" {
		return
	}
	command := exec.Command("taskkill.exe", "/PID", strconv.Itoa(process.Pid), "/T", "/F")
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	_ = command.Run()
}

type connectProxy struct {
	ctx       context.Context
	listener  net.Listener
	procedure Procedure
	username  string
	password  string
	dial      func(context.Context, string, string) (net.Conn, error)
	active    sync.Map
	waitGroup sync.WaitGroup
	slots     chan struct{}
	stopOnce  sync.Once
	stateMu   sync.Mutex
	accepted  int
}

func newConnectProxy(
	ctx context.Context,
	listener net.Listener,
	procedure Procedure,
	username, password string,
	dial func(context.Context, string, string) (net.Conn, error),
) *connectProxy {
	return &connectProxy{
		ctx:       ctx,
		listener:  listener,
		procedure: procedure,
		username:  username,
		password:  password,
		dial:      dial,
		slots:     make(chan struct{}, maxConcurrentProxyConnections),
	}
}

func (server *connectProxy) serve() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		select {
		case server.slots <- struct{}{}:
		case <-server.ctx.Done():
			_ = connection.Close()
			continue
		default:
			writeProxyStatus(connection, 429, "Too Many Requests")
			_ = connection.Close()
			continue
		}
		server.active.Store(connection, struct{}{})
		server.waitGroup.Add(1)
		go func(connection net.Conn) {
			defer server.waitGroup.Done()
			defer server.active.Delete(connection)
			defer func() { <-server.slots }()
			server.handle(connection)
		}(connection)
	}
}

func (server *connectProxy) stop() {
	server.stopOnce.Do(func() {
		_ = server.listener.Close()
		server.active.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
	})
}

func (server *connectProxy) wait() {
	server.waitGroup.Wait()
}

func (server *connectProxy) handle(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(proxyHandshakeTimeout))
	reader := bufio.NewReaderSize(connection, maxHeaderBytes)
	request, err := readConnectRequest(reader)
	if err != nil {
		writeProxyStatus(connection, 400, "Bad Request")
		return
	}
	if request.method != "CONNECT" {
		writeProxyStatus(connection, 405, "Method Not Allowed")
		return
	}
	if !validProxyAuthorization(request.authorization, server.username, server.password) {
		writeProxyStatus(connection, 407, "Proxy Authentication Required")
		return
	}
	if request.target != server.procedure.TargetAuthority {
		writeProxyStatus(connection, 403, "Forbidden")
		return
	}
	if !server.recordAccepted() {
		writeProxyStatus(connection, 429, "Too Many Requests")
		return
	}
	target, err := server.dial(server.ctx, "tcp", server.procedure.TargetAuthority)
	if err != nil {
		writeProxyStatus(connection, 502, "Bad Gateway")
		return
	}
	defer target.Close()
	if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\nConnection: close\r\n\r\n"); err != nil {
		return
	}
	if !validTLSClientHello(reader) {
		return
	}
	_ = connection.SetDeadline(time.Time{})
	_ = target.SetDeadline(time.Time{})
	relay(server.ctx, reader, connection, target)
}

func (server *connectProxy) recordAccepted() bool {
	server.stateMu.Lock()
	defer server.stateMu.Unlock()
	if server.accepted >= server.procedure.MaxEvents {
		return false
	}
	server.accepted++
	return true
}

type connectRequest struct {
	method        string
	target        string
	authorization string
}

func readConnectRequest(reader *bufio.Reader) (connectRequest, error) {
	used := 0
	line, err := readHeaderLine(reader, &used)
	if err != nil {
		return connectRequest{}, err
	}
	parts := strings.Split(line, " ")
	if len(parts) != 3 || parts[2] != "HTTP/1.1" {
		return connectRequest{}, errors.New("proxy request is invalid")
	}
	request := connectRequest{method: parts[0], target: parts[1]}
	for {
		line, err = readHeaderLine(reader, &used)
		if err != nil {
			return connectRequest{}, err
		}
		if line == "" {
			return request, nil
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != name || name == "" {
			return connectRequest{}, errors.New("proxy header is invalid")
		}
		if strings.EqualFold(name, "Proxy-Authorization") {
			if request.authorization != "" {
				return connectRequest{}, errors.New("proxy authorization is duplicated")
			}
			request.authorization = strings.TrimSpace(value)
		}
	}
}

func readHeaderLine(reader *bufio.Reader, used *int) (string, error) {
	line, err := reader.ReadSlice('\n')
	*used += len(line)
	lineString := string(line)
	if err != nil || *used > maxHeaderBytes || !strings.HasSuffix(lineString, "\r\n") {
		return "", errors.New("proxy request is invalid")
	}
	return strings.TrimSuffix(lineString, "\r\n"), nil
}

func validProxyAuthorization(value, username, password string) bool {
	scheme, encoded, ok := strings.Cut(value, " ")
	if !ok || scheme != "Basic" || encoded == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	expected := username + ":" + password
	return subtle.ConstantTimeCompare(decoded, []byte(expected)) == 1
}

func validTLSClientHello(reader *bufio.Reader) bool {
	header, err := reader.Peek(9)
	if err != nil || header[0] != 0x16 || header[1] != 0x03 || header[2] < 0x01 || header[2] > 0x04 {
		return false
	}
	recordLength := int(header[3])<<8 | int(header[4])
	handshakeLength := int(header[6])<<16 | int(header[7])<<8 | int(header[8])
	return recordLength >= 4 && handshakeLength > 0 && handshakeLength <= recordLength-4 && header[5] == 0x01
}

func writeProxyStatus(connection net.Conn, code int, reason string) {
	_, _ = fmt.Fprintf(connection, "HTTP/1.1 %d %s\r\nConnection: close\r\nContent-Length: 0\r\n\r\n", code, reason)
}

func relay(ctx context.Context, reader *bufio.Reader, client, target net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(target, io.MultiReader(reader, client))
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, target)
		done <- struct{}{}
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		_ = target.Close()
	case <-done:
		_ = client.Close()
		_ = target.Close()
	}
	<-done
}

func (server *connectProxy) document() portabletrace.Document {
	server.stateMu.Lock()
	accepted := server.accepted
	server.stateMu.Unlock()
	events := make([]portabletrace.Event, 0, 1)
	if accepted > 0 {
		events = append(events, portabletrace.Event{
			Source:      "proxy",
			Channel:     "network",
			Kind:        "request",
			Destination: "first-party",
			Fields:      []string{"unknown"},
		})
	}
	return portabletrace.Document{
		SchemaVersion: 1,
		Redacted:      true,
		Scope:         server.procedure.Scope,
		Completeness:  portabletrace.Partial,
		Events:        events,
	}
}

func writeExclusive(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
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
