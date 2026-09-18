package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	portabletrace "github.com/jackkayser2005/ariadne/internal/trace"
)

func TestCaptureRelaysAuthenticatedConnectAndWritesPartialTrace(t *testing.T) {
	target, authority, received := startEchoTarget(t)
	defer target.Close()
	procedurePath := writeProxyProcedure(t, authority)
	outputPath := filepath.Join(t.TempDir(), "nested", "trace.json")

	summary, err := Capture(procedurePath, os.Args[0], []string{"-test.run=TestProxyHelperProcess", "--", "connect", "--target", authority}, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("target did not receive the opaque tunnel bytes")
	}
	if summary.Trace.Completeness != portabletrace.Partial || summary.Trace.Events != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	document, err := portabletrace.Read(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Events) != 1 || document.Events[0].Source != "proxy" ||
		document.Events[0].Channel != "network" || document.Events[0].Kind != "request" ||
		document.Events[0].Destination != "first-party" || document.Events[0].Fields[0] != "unknown" {
		t.Fatalf("document = %#v", document)
	}
	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{authority, "fixture", "Proxy-Authorization", "127.0.0.1"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("trace exposed %q: %s", secret, encoded)
		}
	}
	if _, err := Capture(procedurePath, os.Args[0], nil, outputPath); err == nil {
		t.Fatal("Capture() overwrote an existing trace")
	}
}

func TestProxyRejectsPlaintextOutOfScopeAndBadCredentials(t *testing.T) {
	procedure := Procedure{
		SchemaVersion:   ProcedureSchemaVersion,
		ProcedureID:     ConnectProcedureID,
		Scope:           "outbound",
		DurationMS:      500,
		MaxEvents:       4,
		TargetAuthority: "example.com:443",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dialer := &net.Dialer{Timeout: time.Second}
	server := newConnectProxy(ctx, listener, procedure, "user", "password", dialer.DialContext)
	serverDone := make(chan struct{})
	go func() {
		server.serve()
		close(serverDone)
	}()
	defer func() {
		server.stop()
		<-serverDone
		server.wait()
	}()

	tests := []struct {
		name    string
		request string
		status  string
	}{
		{name: "plaintext", request: "GET http://example.com/ HTTP/1.1\r\nHost: example.com\r\n\r\n", status: "405"},
		{name: "malformed request", request: "not-a-request\r\n\r\n", status: "400"},
		{name: "missing crlf", request: "CONNECT example.com:443 HTTP/1.1\n", status: "400"},
		{name: "oversized header", request: "CONNECT example.com:443 HTTP/1.1\r\nX: " + strings.Repeat("a", maxHeaderBytes) + "\r\n\r\n", status: "400"},
		{name: "invalid header", request: "CONNECT example.com:443 HTTP/1.1\r\nBad Header\r\n\r\n", status: "400"},
		{name: "duplicate auth", request: "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic " + basicAuth("user", "password") + "\r\nProxy-Authorization: Basic " + basicAuth("user", "password") + "\r\n\r\n", status: "400"},
		{name: "invalid auth encoding", request: "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic !!!\r\n\r\n", status: "407"},
		{name: "missing auth", request: "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n", status: "407"},
		{name: "bad auth", request: "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic " + basicAuth("user", "wrong") + "\r\n\r\n", status: "407"},
		{name: "wrong target", request: "CONNECT other.example:443 HTTP/1.1\r\nProxy-Authorization: Basic " + basicAuth("user", "password") + "\r\n\r\n", status: "403"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			connection, err := net.Dial("tcp", listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			if _, err := io.WriteString(connection, test.request); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(connection).ReadString('\n')
			if err != nil || !strings.Contains(line, " "+test.status+" ") {
				t.Fatalf("status = %q, err = %v", line, err)
			}
		})
	}
	idle, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for !proxyHasActive(server) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !proxyHasActive(server) {
		_ = idle.Close()
		t.Fatal("proxy did not register the idle connection")
	}
}

func TestProxyEnforcesEventLimitAndDialFailure(t *testing.T) {
	procedure := Procedure{
		SchemaVersion:   ProcedureSchemaVersion,
		ProcedureID:     ConnectProcedureID,
		Scope:           "outbound",
		DurationMS:      500,
		MaxEvents:       1,
		TargetAuthority: "example.com:443",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newConnectProxy(ctx, listener, procedure, "user", "password", func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("target unavailable")
	})
	serverDone := make(chan struct{})
	go func() {
		server.serve()
		close(serverDone)
	}()
	defer func() {
		server.stop()
		<-serverDone
		server.wait()
	}()
	request := "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic " + basicAuth("user", "password") + "\r\n\r\n"
	for _, want := range []string{"502", "429"} {
		connection, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(connection, request); err != nil {
			connection.Close()
			t.Fatal(err)
		}
		line, err := bufio.NewReader(connection).ReadString('\n')
		connection.Close()
		if err != nil || !strings.Contains(line, " "+want+" ") {
			t.Fatalf("status = %q, err = %v, want %s", line, err, want)
		}
	}
}

func TestProxyRejectsPlaintextTunnel(t *testing.T) {
	procedure := Procedure{
		SchemaVersion:   ProcedureSchemaVersion,
		ProcedureID:     ConnectProcedureID,
		Scope:           "outbound",
		DurationMS:      500,
		MaxEvents:       1,
		TargetAuthority: "example.com:443",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target, targetPeer := net.Pipe()
	defer targetPeer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newConnectProxy(ctx, listener, procedure, "user", "password", func(context.Context, string, string) (net.Conn, error) {
		return target, nil
	})
	serverDone := make(chan struct{})
	go func() {
		server.serve()
		close(serverDone)
	}()
	defer func() {
		server.stop()
		<-serverDone
		server.wait()
	}()
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	request := "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic " + basicAuth("user", "password") + "\r\n\r\n"
	if _, err := io.WriteString(connection, request); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(connection).ReadString('\n'); err != nil || !strings.Contains(line, " 200 ") {
		t.Fatalf("CONNECT status = %q, err = %v", line, err)
	}
	if _, err := io.WriteString(connection, "GET / HTTP/1.1\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("proxy relayed plaintext tunnel data")
	}
}

func TestProxyBoundsConcurrentHandshakes(t *testing.T) {
	procedure := Procedure{
		SchemaVersion:   ProcedureSchemaVersion,
		ProcedureID:     ConnectProcedureID,
		Scope:           "outbound",
		DurationMS:      500,
		MaxEvents:       maxConcurrentProxyConnections,
		TargetAuthority: "example.com:443",
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newConnectProxy(ctx, listener, procedure, "user", "password", func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("unexpected dial")
	})
	serverDone := make(chan struct{})
	go func() {
		server.serve()
		close(serverDone)
	}()
	defer func() {
		server.stop()
		<-serverDone
		server.wait()
	}()
	connections := make([]net.Conn, 0, maxConcurrentProxyConnections)
	for len(connections) < maxConcurrentProxyConnections {
		connection, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	deadline := time.Now().Add(time.Second)
	for proxyActiveCount(server) < maxConcurrentProxyConnections && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if proxyActiveCount(server) != maxConcurrentProxyConnections {
		t.Fatalf("active handshakes = %d", proxyActiveCount(server))
	}
	extra, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	lineReader := bufio.NewReader(extra)
	_ = extra.SetReadDeadline(time.Now().Add(time.Second))
	line, err := lineReader.ReadString('\n')
	extra.Close()
	if err != nil || !strings.Contains(line, " 429 ") {
		t.Fatalf("bounded handshake status = %q, err = %v", line, err)
	}
	for _, connection := range connections {
		_ = connection.Close()
	}
}

func TestCaptureStopsTimedOutProcess(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	outputPath := filepath.Join(t.TempDir(), "timed-out.json")
	summary, err := Capture(procedurePath, os.Args[0], []string{"-test.run=TestProxyHangingProcess", "--", "hang"}, outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Trace.Events != 0 || summary.Trace.Completeness != portabletrace.Partial {
		t.Fatalf("timed-out summary = %#v", summary)
	}
}

func TestProxyHangingProcess(t *testing.T) {
	if !proxyHelperInvocation(os.Args) {
		return
	}
	select {}
}

func TestCaptureRejectsMissingProgramAndOutputDirectory(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	outputPath := filepath.Join(t.TempDir(), "trace.json")
	if _, err := Capture("missing-procedure.json", os.Args[0], nil, outputPath); err == nil {
		t.Fatal("Capture() accepted a missing procedure")
	}
	if _, err := Capture("", os.Args[0], nil, outputPath); err == nil {
		t.Fatal("Capture() accepted empty paths")
	}
	programFile := filepath.Join(t.TempDir(), "program.txt")
	if err := os.WriteFile(programFile, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(procedurePath, programFile, nil, outputPath); err == nil {
		t.Fatal("Capture() accepted a non-executable program")
	}
	parentFile := filepath.Join(t.TempDir(), "parent")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(procedurePath, os.Args[0], []string{"-test.run=TestProxyHelperProcess", "--"}, filepath.Join(parentFile, "trace.json")); err == nil {
		t.Fatal("Capture() wrote through a file parent")
	}
}

func TestProxyEnvironmentRemovesInheritedProxyValues(t *testing.T) {
	t.Setenv("HTTP_PROXY", "old-http")
	t.Setenv("https_proxy", "old-https")
	t.Setenv("NO_PROXY", "old-no-proxy")
	t.Setenv("ARIADNE_SECRET", "do-not-pass")
	environment := proxyEnvironment("http://ariadne:secret@127.0.0.1:1234")
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "old-http") || strings.Contains(joined, "old-https") || strings.Contains(joined, "old-no-proxy") || strings.Contains(joined, "do-not-pass") || !strings.Contains(joined, "HTTP_PROXY=http://ariadne:secret@127.0.0.1:1234") || !strings.Contains(joined, "NO_PROXY=") {
		t.Fatalf("proxy environment = %q", joined)
	}
}

func TestRelayStopsWhenContextIsCanceled(t *testing.T) {
	client, clientPeer := net.Pipe()
	target, targetPeer := net.Pipe()
	defer clientPeer.Close()
	defer targetPeer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		relay(ctx, bufio.NewReader(client), client, target)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("relay did not stop after context cancellation")
	}
}

func TestWriteExclusiveRejectsUnsafeOutputs(t *testing.T) {
	if err := writeExclusive("", nil); err == nil {
		t.Fatal("writeExclusive() accepted an empty path")
	}
	existing := filepath.Join(t.TempDir(), "trace.json")
	if err := os.WriteFile(existing, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusive(existing, []byte("new")); err == nil {
		t.Fatal("writeExclusive() overwrote an existing file")
	}
}

func TestCaptureRejectsUnsafeProcessInputs(t *testing.T) {
	procedurePath := writeProxyProcedure(t, "example.com:443")
	outputPath := filepath.Join(t.TempDir(), "trace.json")
	if _, err := Capture(procedurePath, "relative-program", nil, outputPath); err == nil {
		t.Fatal("Capture() accepted relative program path")
	}
	if _, err := Capture(procedurePath, filepath.Join(t.TempDir(), "missing.exe"), nil, outputPath); err == nil {
		t.Fatal("Capture() accepted missing program")
	}
	args := make([]string, maxProcessArgs+1)
	if _, err := Capture(procedurePath, os.Args[0], args, outputPath); err == nil {
		t.Fatal("Capture() accepted too many process arguments")
	}
}

func TestProxyHelperProcess(t *testing.T) {
	if !proxyHelperInvocation(os.Args) {
		return
	}
	proxyURL, err := url.Parse(os.Getenv("HTTPS_PROXY"))
	if err != nil || proxyURL.User == nil {
		os.Exit(2)
	}
	password, ok := proxyURL.User.Password()
	if !ok {
		os.Exit(2)
	}
	target := ""
	for index, arg := range os.Args {
		if arg == "--target" && index+1 < len(os.Args) {
			target = os.Args[index+1]
			break
		}
	}
	if target == "" {
		os.Exit(2)
	}
	connection, err := net.DialTimeout("tcp", proxyURL.Host, time.Second)
	if err != nil {
		os.Exit(2)
	}
	defer connection.Close()
	authorization := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
	if _, err := fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", target, target, authorization); err != nil {
		os.Exit(2)
	}
	line, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil || !strings.Contains(line, " 200 ") {
		os.Exit(2)
	}
	payload := proxyTestPayload()
	if _, err := connection.Write(payload); err != nil {
		os.Exit(2)
	}
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(connection, response); err != nil || string(response) != string(payload) {
		os.Exit(2)
	}
	os.Exit(0)
}

func startEchoTarget(t *testing.T) (net.Listener, string, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	received := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		payload := proxyTestPayload()
		if _, err := io.ReadFull(connection, payload); err == nil && string(payload) == string(proxyTestPayload()) {
			_, _ = connection.Write(payload)
			close(received)
		}
	}()
	return listener, "localhost:" + strconv.Itoa(port), received
}

func proxyHelperInvocation(args []string) bool {
	for _, arg := range args {
		if arg == "connect" {
			return true
		}
	}
	return false
}

func proxyTestPayload() []byte {
	return append([]byte{0x16, 0x03, 0x03, 0x00, 0x08, 0x01, 0x00, 0x00, 0x04}, []byte("fixture")...)
}

func basicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

func proxyHasActive(server *connectProxy) bool {
	return proxyActiveCount(server) > 0
}

func proxyActiveCount(server *connectProxy) int {
	active := 0
	server.active.Range(func(any, any) bool {
		active++
		return true
	})
	return active
}

func writeProxyProcedure(t *testing.T, authority string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "procedure.json")
	data := []byte(fmt.Sprintf(`{"schema_version":1,"procedure_id":"proxy-connect-v1","scope":"outbound","duration_ms":1000,"max_events":8,"target_authority":%q}`, authority))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
