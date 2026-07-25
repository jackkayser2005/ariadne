// Package collector receives authorized local fixture observations.
package collector

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	maxBodyBytes   = 64 << 10
	maxHeaderBytes = 16 << 10
)

// Observation is the bounded subset of one captured HTTP request.
type Observation struct {
	SchemaVersion int    `json:"schema_version"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	ContentType   string `json:"content_type"`
	BodyBase64    string `json:"body_base64"`
}

// Collector accepts one observation on an ephemeral IPv4 loopback port.
type Collector struct {
	listener net.Listener
	server   *http.Server
	result   chan captureResult
	claimed  atomic.Bool
}

type captureResult struct {
	observation Observation
	err         error
}

// Start binds a collector to an ephemeral IPv4 loopback port.
func Start() (*Collector, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}

	collector := &Collector{
		listener: listener,
		result:   make(chan captureResult, 1),
	}
	collector.server = &http.Server{
		Handler:           http.HandlerFunc(collector.handle),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	go func() {
		err := collector.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			collector.deliver(captureResult{err: errors.New("collector stopped")})
		}
	}()
	return collector, nil
}

// Port returns the host loopback port selected by Start.
func (c *Collector) Port() int {
	return c.listener.Addr().(*net.TCPAddr).Port
}

// Wait returns the first request or the context error.
func (c *Collector) Wait(ctx context.Context) (Observation, error) {
	select {
	case result := <-c.result:
		return result.observation, result.err
	case <-ctx.Done():
		return Observation{}, fmt.Errorf("wait for network observation: %w", ctx.Err())
	}
}

// Close stops the loopback server.
func (c *Collector) Close() error {
	if err := c.server.Close(); err != nil {
		return fmt.Errorf("close collector: %w", err)
	}
	return nil
}

func (c *Collector) handle(response http.ResponseWriter, request *http.Request) {
	if !c.claimed.CompareAndSwap(false, true) {
		http.Error(response, "observation already received", http.StatusConflict)
		return
	}

	if request.Method != http.MethodPost {
		c.reject(response, http.StatusMethodNotAllowed, "request method is not POST")
		return
	}
	if request.URL.Path != "/observe" || request.URL.RawQuery != "" {
		c.reject(response, http.StatusNotFound, "request target is not /observe")
		return
	}
	if request.Header.Get("Content-Encoding") != "" {
		c.reject(response, http.StatusUnsupportedMediaType, "content encoding is not supported")
		return
	}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		c.reject(response, http.StatusUnsupportedMediaType, "content type is not application/json")
		return
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		c.reject(response, http.StatusBadRequest, "read request body")
		return
	}
	if len(body) > maxBodyBytes {
		c.reject(response, http.StatusRequestEntityTooLarge, "request body exceeds 65536-byte limit")
		return
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(body) {
		c.reject(response, http.StatusBadRequest, "request body is not a valid JSON object")
		return
	}

	c.result <- captureResult{observation: Observation{
		SchemaVersion: 1,
		Method:        request.Method,
		Path:          request.URL.Path,
		ContentType:   contentType,
		BodyBase64:    base64.StdEncoding.EncodeToString(body),
	}}
	response.WriteHeader(http.StatusNoContent)
}

func (c *Collector) reject(response http.ResponseWriter, status int, message string) {
	c.result <- captureResult{err: errors.New(message)}
	http.Error(response, "invalid observation", status)
}

func (c *Collector) deliver(result captureResult) {
	if c.claimed.CompareAndSwap(false, true) {
		c.result <- result
	}
}
