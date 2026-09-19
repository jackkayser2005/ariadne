package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func protocolFixture(t *testing.T, serve func(context.Context, *websocket.Conn)) *cdpClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connection, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer connection.CloseNow()
		serve(ctx, connection)
	}))
	client, err := connectCDP(ctx, strings.Replace(server.URL, "http:", "ws:", 1))
	if err != nil {
		cancel()
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); client.close(); server.Close() })
	return client
}

func TestCDPConcurrentCallsAndRedactedErrors(t *testing.T) {
	client := protocolFixture(t, func(ctx context.Context, connection *websocket.Conn) {
		for {
			_, data, err := connection.Read(ctx)
			if err != nil {
				return
			}
			var request cdpMessage
			if json.Unmarshal(data, &request) != nil {
				return
			}
			reply := cdpMessage{ID: request.ID, Result: json.RawMessage(`{"value":true}`)}
			switch request.Method {
			case "reject":
				reply.Error = json.RawMessage(`{"message":"private-secret"}`)
			case "invalid-result":
				reply.Result = json.RawMessage(`[]`)
			case "stall":
				continue
			}
			encoded, _ := json.Marshal(reply)
			if connection.Write(ctx, websocket.MessageText, encoded) != nil {
				return
			}
		}
	})
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var result struct{ Value bool }
			if err := client.call(context.Background(), "page", "ok", map[string]any{}, &result); err != nil || !result.Value {
				t.Errorf("concurrent call: %v, %+v", err, result)
			}
		}()
	}
	wg.Wait()
	for _, method := range []string{"reject", "invalid-result"} {
		var result struct{ Value bool }
		if err := client.call(context.Background(), "", method, nil, &result); !errors.Is(err, errCDP) || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if err := client.call(context.Background(), "", "ok", make(chan int), nil); !errors.Is(err, errCDP) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.call(ctx, "", "stall", nil, nil); !errors.Is(err, errCDP) {
		t.Fatal(err)
	}
	client.mu.Lock()
	pending := len(client.pending)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatal("timed out calls retained")
	}
	client.close()
	if err := client.call(context.Background(), "", "ok", nil, nil); !errors.Is(err, errCDP) {
		t.Fatal(err)
	}
}

func TestCDPRejectsMalformedOversizedAndFloodedEvents(t *testing.T) {
	for _, scenario := range []string{"malformed", "binary", "oversized", "flood"} {
		t.Run(scenario, func(t *testing.T) {
			client := protocolFixture(t, func(ctx context.Context, connection *websocket.Conn) {
				kind, data := websocket.MessageText, []byte("{")
				count := 1
				switch scenario {
				case "binary":
					kind = websocket.MessageBinary
				case "oversized":
					data = []byte(strings.Repeat("x", 800<<10))
				case "flood":
					data = []byte(`{"method":"event","params":{}}`)
					count = 130
				}
				for range count {
					if connection.Write(ctx, kind, data) != nil {
						return
					}
				}
				<-ctx.Done()
			})
			select {
			case <-client.done:
			case <-time.After(3 * time.Second):
				t.Fatal("invalid transport did not close")
			}
			client.mu.Lock()
			overflow := client.overflow
			client.mu.Unlock()
			if (scenario == "flood") != overflow {
				t.Fatalf("overflow = %v", overflow)
			}
		})
	}
	if _, err := connectCDP(context.Background(), "http://invalid"); !errors.Is(err, errCDP) {
		t.Fatal(err)
	}
}

func TestCDPCloseReleasesPendingCall(t *testing.T) {
	received := make(chan struct{})
	client := protocolFixture(t, func(ctx context.Context, connection *websocket.Conn) {
		if _, _, err := connection.Read(ctx); err == nil {
			close(received)
		}
		<-ctx.Done()
	})
	result := make(chan error, 1)
	go func() { result <- client.call(context.Background(), "", "stall", nil, nil) }()
	<-received
	client.close()
	select {
	case err := <-result:
		if !errors.Is(err, errCDP) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending call retained after connection close")
	}
}
