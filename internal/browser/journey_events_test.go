package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

func eventCapture(t *testing.T) *JourneyCapture {
	t.Helper()
	markers := []SyntheticMarker{{ID: "m1", Category: "email", Value: "event-fixture@example.invalid"}}
	matcher, err := NewMarkerMatcher(markers)
	if err != nil {
		t.Fatal(err)
	}
	return &JourneyCapture{matcher: matcher, options: CaptureOptions{Markers: markers}, result: CaptureResult{Journey: trace.NewJourney(), Destinations: []DestinationName{}, Steps: []InvestigationStep{}}, sessions: map[string]captureSession{"page": {context: "page", origin: "https://fixture.invalid"}}, contexts: map[string]captureSession{}, requests: map[string]requestObservation{}}
}

func emitCaptureEvent(c *JourneyCapture, method string, value any) {
	data, _ := json.Marshal(value)
	c.event(context.Background(), cdpMessage{Method: method, Session: "page", Params: data})
}

func TestJourneyRequestReferencesBlockingAndGaps(t *testing.T) {
	c := eventCapture(t)
	request := map[string]any{"requestId": "one", "request": map[string]any{"url": "https://fixture.invalid/?v=event-fixture@example.invalid"}}
	emitCaptureEvent(c, "Network.requestWillBeSent", request)
	emitCaptureEvent(c, "Network.loadingFailed", map[string]any{"requestId": "one", "blockedReason": "inspector"})
	request["redirectResponse"] = map[string]any{"status": 302}
	emitCaptureEvent(c, "Network.requestWillBeSent", request)
	emitCaptureEvent(c, "Network.responseReceived", map[string]any{"requestId": "one"})
	result := c.Snapshot()
	if len(result.Journey.Observations) != 5 {
		t.Fatalf("observations: %+v", result.Journey.Observations)
	}
	blocked := result.Journey.Observations[1]
	if blocked.Kind != "blocked" || blocked.Reference != result.Journey.Observations[0].Reference || len(blocked.Matches) != 1 {
		t.Fatal("blocked attempt lost supporting reference")
	}
	if result.Journey.Observations[2].Kind != "redirect" || len(result.Journey.Observations[4].Matches) != 0 {
		t.Fatal("redirect/response semantics changed")
	}
	if err := result.Journey.Validate(); err != nil {
		t.Fatal(err)
	}
	emitCaptureEvent(c, "Network.loadingFailed", map[string]any{"requestId": "one"})
	emitCaptureEvent(c, "Network.responseReceived", map[string]any{"requestId": "missing"})
	emitCaptureEvent(c, "Network.requestWillBeSent", map[string]any{"requestId": "post", "request": map[string]any{"url": "https://fixture.invalid", "hasPostData": true}})
	for _, gap := range []string{"request-failed", "instrumentation-unavailable", "unsupported-payload"} {
		if !slices.Contains(c.Snapshot().Journey.Gaps, gap) {
			t.Errorf("missing %s", gap)
		}
	}
}

func TestJourneyWebSocketPayloadBoundaries(t *testing.T) {
	c := eventCapture(t)
	emitCaptureEvent(c, "Network.webSocketCreated", map[string]any{"requestId": "socket", "url": "wss://fixture.invalid/socket"})
	for _, method := range []string{"Network.webSocketFrameSent", "Network.webSocketFrameReceived"} {
		emitCaptureEvent(c, method, map[string]any{"requestId": "socket", "response": map[string]any{"opcode": 1, "payloadData": "event-fixture@example.invalid"}})
	}
	for _, o := range c.Snapshot().Journey.Observations {
		if len(o.Matches) != 1 || o.Destination != "d1" {
			t.Fatal("supported text frame lost match")
		}
	}
	emitCaptureEvent(c, "Network.webSocketFrameSent", map[string]any{"requestId": "socket", "response": map[string]any{"opcode": 2, "payloadData": "aGVsbG8="}})
	emitCaptureEvent(c, "Network.webSocketFrameSent", map[string]any{"requestId": "unknown", "response": map[string]any{"opcode": 1}})
	emitCaptureEvent(c, "Network.webSocketFrameReceived", map[string]any{"requestId": "socket", "response": map[string]any{"opcode": 1, "payloadData": strings.Repeat("x", 300<<10)}})
	for _, gap := range []string{"unsupported-payload", "instrumentation-unavailable", "size-limit"} {
		if !slices.Contains(c.Snapshot().Journey.Gaps, gap) {
			t.Errorf("missing %s", gap)
		}
	}
}

func TestJourneyHooksNeverPersistRawPayloadsAndBoundSteps(t *testing.T) {
	c := eventCapture(t)
	emit := func(value any) {
		data, _ := json.Marshal(value)
		emitCaptureEvent(c, "Runtime.bindingCalled", map[string]any{"name": "__ariadneObserve", "payload": string(data)})
	}
	emit(map[string]any{"kind": "ready"})
	emit(map[string]any{"kind": "input", "url": "https://fixture.invalid", "selector": "input:nth-of-type(1)", "payload": "event-fixture@example.invalid"})
	emit(map[string]any{"kind": "click", "url": "https://fixture.invalid", "selector": "button", "payload": "secret-unmatched-input"})
	emit(map[string]any{"kind": "checkpoint"})
	emit(map[string]any{"kind": "unsupported"})
	emit(map[string]any{"gap": "unreviewed-hostile-gap"})
	emit(map[string]any{"kind": "fetch", "url": "https://fixture.invalid", "payload": strings.Repeat("x", 300<<10)})
	emitCaptureEvent(c, "Runtime.bindingCalled", map[string]any{"name": "wrong", "payload": "{}"})
	result := c.Snapshot()
	if len(result.Steps) != 3 || result.Steps[0].Marker != "m1" || result.Steps[0].Checkpoint || !result.Steps[1].Checkpoint || !result.Steps[2].Checkpoint {
		t.Fatalf("steps: %+v", result.Steps)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "secret-unmatched-input") || strings.Contains(string(data), "event-fixture@example.invalid") {
		t.Fatal("retained raw payload")
	}
	result.Steps[0].Marker = "changed"
	result.Journey.Observations[0].Matches[0].Marker = "changed"
	if c.Snapshot().Steps[0].Marker != "m1" || c.Snapshot().Journey.Observations[0].Matches[0].Marker != "m1" {
		t.Fatal("snapshot shares mutable state")
	}
	for range 70 {
		emit(map[string]any{"kind": "checkpoint"})
	}
	if len(c.Snapshot().Steps) != 64 || !slices.Contains(c.Snapshot().Journey.Gaps, "manual-checkpoint") {
		t.Fatal("steps exceeded their bound")
	}
	c.step(InvestigationStep{Selector: strings.Repeat("x", 513)})
}

func TestJourneyMalformedAndBoundedEvents(t *testing.T) {
	for _, method := range []string{"Target.attachedToTarget", "Fetch.requestPaused", "Runtime.bindingCalled", "Network.requestWillBeSent", "Network.responseReceived", "Network.webSocketCreated", "Network.webSocketFrameSent"} {
		c := eventCapture(t)
		c.event(context.Background(), cdpMessage{Method: method, Session: "page", Params: json.RawMessage(`{`)})
		if c.Snapshot().Journey.Completeness != trace.Partial {
			t.Errorf("%s did not record a gap", method)
		}
	}
	c := eventCapture(t)
	for range trace.MaxJourneyObservations + 1 {
		c.observe("input", "page", "https://fixture.invalid", "", nil)
	}
	if len(c.Snapshot().Journey.Observations) != trace.MaxJourneyObservations || !slices.Contains(c.Snapshot().Journey.Gaps, "event-limit") {
		t.Fatal("unbounded observations")
	}
	c.references = trace.MaxJourneyObservations * 16
	if c.reference() != "" {
		t.Fatal("unbounded references")
	}
	for i := 0; i < 257; i++ {
		c.destination(fmt.Sprintf("https://host%d.invalid", i))
	}
	if len(c.Snapshot().Destinations) != 256 {
		t.Fatal("unbounded destinations")
	}
	if c.destination("file:///private") != "" {
		t.Fatal("accepted unsupported destination")
	}
	c.requests = make(map[string]requestObservation)
	for i := 0; i < 4096; i++ {
		c.requests[fmt.Sprint(i)] = requestObservation{}
	}
	emitCaptureEvent(c, "Network.requestWillBeSent", map[string]any{"requestId": "overflow", "request": map[string]any{"url": "https://fixture.invalid"}})
	if len(c.requests) != 4096 {
		t.Fatal("unbounded request registry")
	}
}

func TestJourneyFrameContextLifecycleIsBounded(t *testing.T) {
	c := eventCapture(t)
	c.frame = "main"
	create := func(id int, frame string) {
		emitCaptureEvent(c, "Runtime.executionContextCreated", map[string]any{"context": map[string]any{"id": id, "origin": "https://frame.invalid", "auxData": map[string]any{"frameId": frame}}})
	}
	create(1, "main")
	create(2, "child")
	emit := func(id int, kind string) {
		data, _ := json.Marshal(map[string]any{"kind": kind, "url": "https://frame.invalid", "payload": "event-fixture@example.invalid"})
		emitCaptureEvent(c, "Runtime.bindingCalled", map[string]any{"name": "__ariadneObserve", "payload": string(data), "executionContextId": id})
	}
	emit(2, "ready")
	emit(2, "storage-read")
	emit(1, "cookie-read")
	result := c.Snapshot()
	if result.Journey.Observations[0].Context != "frame" || result.Journey.Observations[1].Context != "page" || c.sessions["page"].context != "page" {
		t.Fatal("frame context contaminated its parent")
	}
	emitCaptureEvent(c, "Runtime.executionContextDestroyed", map[string]any{"executionContextId": 2})
	emit(2, "input")
	if !slices.Contains(c.Snapshot().Journey.Gaps, "frame-unavailable") {
		t.Fatal("missing context was not explicit")
	}
	for i := 2; i <= 257; i++ {
		create(i, "child")
	}
	if len(c.contexts) != 256 {
		t.Fatal("unbounded context registry")
	}
	emitCaptureEvent(c, "Runtime.executionContextsCleared", map[string]any{})
	if len(c.contexts) != 0 {
		t.Fatal("contexts retained across navigation")
	}
	for _, method := range []string{"Runtime.executionContextCreated", "Runtime.executionContextDestroyed"} {
		c.event(context.Background(), cdpMessage{Method: method, Session: "page", Params: json.RawMessage(`{`)})
	}
	create(0, "invalid")
}
