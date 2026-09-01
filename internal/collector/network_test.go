package collector

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCollectorDeliversOnlyFirstResult(t *testing.T) {
	collector := &Collector{result: make(chan captureResult, 1)}
	collector.deliver(captureResult{})
	collector.deliver(captureResult{observation: Observation{Method: http.MethodGet}})
	select {
	case <-collector.result:
	default:
		t.Fatal("collector did not deliver the first result")
	}
	if len(collector.result) != 0 {
		t.Fatal("collector delivered a second result")
	}
}

func TestCollectorRejectsBodyReadErrorsWithoutClaimingSlot(t *testing.T) {
	collector := &Collector{result: make(chan captureResult, 1)}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/observe", nil)
	request.Header.Set("Content-Type", "application/json")
	request.Body = failingBody{}
	response := httptest.NewRecorder()

	collector.handle(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if collector.claimed.Load() {
		t.Fatal("body read failure claimed the collector")
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("body read failed") }
func (failingBody) Close() error             { return nil }
func TestCollectorCapturesOneRequest(t *testing.T) {
	collector, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()

	const body = `{"schema_version":1,"variant":"standard"}`
	request, err := http.NewRequest(
		http.MethodPost,
		collectorURL(collector),
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", response.StatusCode)
	}

	observation, err := collector.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(observation.BodyBase64)
	if err != nil {
		t.Fatal(err)
	}
	if observation.SchemaVersion != 1 ||
		observation.Method != http.MethodPost ||
		observation.Path != "/observe" ||
		observation.ContentType != "application/json" ||
		string(decoded) != body {
		t.Fatalf("observation = %#v, body = %q", observation, decoded)
	}

	second, err := http.Post(
		collectorURL(collector),
		"application/json",
		strings.NewReader(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("second status = %d", second.StatusCode)
	}
}

func TestCollectorRejectsInvalidRequestsWithoutClaimingSlot(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		target      string
		contentType string
		encoding    string
		body        string
		wantStatus  int
	}{
		{
			name:        "method",
			method:      http.MethodGet,
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusMethodNotAllowed,
		},
		{
			name:        "path",
			method:      http.MethodPost,
			target:      "/other",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "query",
			method:      http.MethodPost,
			target:      "/observe?extra=true",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "content encoding",
			method:      http.MethodPost,
			contentType: "application/json",
			encoding:    "gzip",
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "content type",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "malformed content type",
			method:      http.MethodPost,
			contentType: `application/json; charset="`,
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "empty body",
			method:      http.MethodPost,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "non-object",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[]`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "malformed JSON",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"value":`,
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "oversized",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"value":"` + strings.Repeat("x", maxBodyBytes) + `"}`,
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector, err := Start()
			if err != nil {
				t.Fatal(err)
			}
			defer collector.Close()

			target := test.target
			if target == "" {
				target = "/observe"
			}
			request, err := http.NewRequest(
				test.method,
				"http://127.0.0.1:"+portString(collector.Port())+target,
				strings.NewReader(test.body),
			)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", test.contentType)
			if test.encoding != "" {
				request.Header.Set("Content-Encoding", test.encoding)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}

			validRequest, err := http.NewRequest(
				http.MethodPost,
				collectorURL(collector),
				strings.NewReader(`{"valid":true}`),
			)
			if err != nil {
				t.Fatal(err)
			}
			validRequest.Header.Set("Content-Type", "application/json")
			validResponse, err := http.DefaultClient.Do(validRequest)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, validResponse.Body)
			validResponse.Body.Close()
			if validResponse.StatusCode != http.StatusNoContent {
				t.Fatalf("valid request status = %d", validResponse.StatusCode)
			}
			observation, err := collector.Wait(context.Background())
			if err != nil {
				t.Fatalf("Wait() error after invalid request = %v", err)
			}
			if observation.Method != http.MethodPost || observation.Path != "/observe" {
				t.Fatalf("observation after invalid request = %#v", observation)
			}
		})
	}
}

func TestCollectorWaitContext(t *testing.T) {
	collector, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = collector.Wait(ctx)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Wait() error = %v", err)
	}
}

func collectorURL(collector *Collector) string {
	return "http://127.0.0.1:" + portString(collector.Port()) + "/observe"
}

func portString(port int) string {
	return strconv.Itoa(port)
}
