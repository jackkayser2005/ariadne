package collector

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

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

func TestCollectorRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		target      string
		contentType string
		encoding    string
		body        string
		wantStatus  int
		wantError   string
	}{
		{
			name:        "method",
			method:      http.MethodGet,
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusMethodNotAllowed,
			wantError:   "method",
		},
		{
			name:        "path",
			method:      http.MethodPost,
			target:      "/other",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusNotFound,
			wantError:   "target",
		},
		{
			name:        "query",
			method:      http.MethodPost,
			target:      "/observe?extra=true",
			contentType: "application/json",
			body:        `{}`,
			wantStatus:  http.StatusNotFound,
			wantError:   "target",
		},
		{
			name:        "content encoding",
			method:      http.MethodPost,
			contentType: "application/json",
			encoding:    "gzip",
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantError:   "encoding",
		},
		{
			name:        "content type",
			method:      http.MethodPost,
			contentType: "text/plain",
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantError:   "content type",
		},
		{
			name:        "malformed content type",
			method:      http.MethodPost,
			contentType: `application/json; charset="`,
			body:        `{}`,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantError:   "content type",
		},
		{
			name:        "empty body",
			method:      http.MethodPost,
			contentType: "application/json",
			wantStatus:  http.StatusBadRequest,
			wantError:   "JSON object",
		},
		{
			name:        "non-object",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `[]`,
			wantStatus:  http.StatusBadRequest,
			wantError:   "JSON object",
		},
		{
			name:        "malformed JSON",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"value":`,
			wantStatus:  http.StatusBadRequest,
			wantError:   "JSON object",
		},
		{
			name:        "oversized",
			method:      http.MethodPost,
			contentType: "application/json",
			body:        `{"value":"` + strings.Repeat("x", maxBodyBytes) + `"}`,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantError:   "exceeds",
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

			_, err = collector.Wait(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Wait() error = %v, want containing %q", err, test.wantError)
			}
			if test.body != "" && strings.Contains(err.Error(), test.body) {
				t.Fatalf("Wait() exposed request body: %v", err)
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
