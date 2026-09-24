package browser

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

func TestJourneyWorkerHookEvaluationDoesNotDelayNextTarget(t *testing.T) {
	finished := make(chan struct{})
	client := protocolFixture(t, func(ctx context.Context, connection *websocket.Conn) {
		var pending *cdpMessage
		secondResumed := false
		reply := func(request cdpMessage) {
			data, _ := json.Marshal(cdpMessage{ID: request.ID, Result: json.RawMessage(`{}`)})
			_ = connection.Write(ctx, websocket.MessageText, data)
		}
		for {
			_, data, err := connection.Read(ctx)
			if err != nil {
				return
			}
			var request cdpMessage
			if json.Unmarshal(data, &request) != nil {
				return
			}
			if request.Session == "second" && request.Method == "Runtime.runIfWaitingForDebugger" {
				secondResumed = true
				if pending != nil {
					reply(*pending)
					pending = nil
					close(finished)
				}
			}
			if request.Session == "first" && request.Method == "Runtime.evaluate" {
				if !secondResumed {
					pending = &request
					continue
				}
				close(finished)
			}
			reply(request)
		}
	})
	capture := &JourneyCapture{client: client, result: CaptureResult{Journey: trace.NewJourney()}}
	t.Cleanup(func() { client.close(); capture.hooks.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := capture.configure(ctx, "first", "worker"); err != nil {
		t.Fatalf("hook evaluation prevented further target setup: %v", err)
	}
	if err := capture.configure(ctx, "second", "worker"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("first worker never resumed after the second target became ready")
	}
}

func TestJourneyClosesWorkerWhenDestinationControlCannotBeInstalled(t *testing.T) {
	for _, limit := range []bool{false, true} {
		t.Run(map[bool]string{false: "command-rejected", true: "session-limit"}[limit], func(t *testing.T) {
			var mu sync.Mutex
			var methods []string
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
					mu.Lock()
					methods = append(methods, request.Method)
					mu.Unlock()
					reply := cdpMessage{ID: request.ID, Result: json.RawMessage(`{}`)}
					if request.Method == "Network.setBlockedURLs" {
						reply.Error = json.RawMessage(`{"code":-32601}`)
					}
					encoded, _ := json.Marshal(reply)
					if connection.Write(ctx, websocket.MessageText, encoded) != nil {
						return
					}
				}
			})
			capture := &JourneyCapture{client: client, options: CaptureOptions{BlockOrigins: []string{"https://collector.invalid"}}, result: CaptureResult{Journey: trace.NewJourney()}, sessions: map[string]captureSession{}}
			if limit {
				for i := range 64 {
					capture.sessions[string(rune('a'+i))] = captureSession{}
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			capture.event(ctx, cdpMessage{Method: "Target.attachedToTarget", Params: json.RawMessage(`{"sessionId":"worker","targetInfo":{"type":"worker","targetId":"target","url":"https://site.invalid/worker.js"}}`)})
			mu.Lock()
			defer mu.Unlock()
			if !slices.Contains(methods, "Target.closeTarget") || slices.Contains(methods, "Runtime.runIfWaitingForDebugger") {
				t.Fatalf("uncontrolled worker resumed: %v", methods)
			}
		})
	}
}
