package browser

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

//go:embed journey_hooks.js
var journeyHooks string

// DestinationName is private local context, excluded from portable evidence.
type DestinationName struct {
	Alias  string `json:"alias"`
	Origin string `json:"origin"`
}

// InvestigationStep retains a bounded structural selector, never typed values.
// Clicks and submissions require a human checkpoint before any replay.
type InvestigationStep struct {
	Kind       string `json:"kind"`
	Selector   string `json:"selector,omitempty"`
	Marker     string `json:"marker,omitempty"`
	Checkpoint bool   `json:"checkpoint"`
}

// CaptureOptions describes one fresh, bounded experiment.
type CaptureOptions struct {
	URL          string
	Markers      []SyntheticMarker
	BlockOrigins []string
	Location     string // unchanged, deny, or approximate (lab-only synthetic location)
	Headless     bool
}

// CaptureResult separates portable evidence from private destination context.
type CaptureResult struct {
	Journey      trace.Journey
	Destinations []DestinationName
	Steps        []InvestigationStep
}

type requestObservation struct {
	destination, reference string
	matches                []trace.JourneyMatch
}
type captureSession struct {
	context, origin string
	ready           bool
}

// JourneyCapture owns one isolated Chrome/Edge process and its transient raw observations.
type JourneyCapture struct {
	mu                             sync.Mutex
	stopMu                         sync.Mutex
	result                         CaptureResult
	options                        CaptureOptions
	matcher                        *MarkerMatcher
	client                         *cdpClient
	process                        *browserProcess
	cancel                         context.CancelFunc
	done                           chan struct{}
	ready                          chan error
	session, target, frame, origin string
	sessions                       map[string]captureSession
	contexts                       map[string]captureSession
	requests                       map[string]requestObservation
	references                     int
	stopping, stopped              bool
	cleanupError                   error
}

// StartCapture opens a fresh visible profile by default and instruments it before
// navigating. A failed start closes the browser and removes its temporary profile.
func StartCapture(ctx context.Context, options CaptureOptions) (*JourneyCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options.Markers = slices.Clone(options.Markers)
	options.BlockOrigins = slices.Clone(options.BlockOrigins)
	u, err := InvestigationURL(options.URL)
	if err != nil {
		return nil, err
	}
	if options.Location == "" {
		options.Location = "unchanged"
	}
	if !slices.Contains([]string{"unchanged", "deny", "approximate"}, options.Location) || len(options.BlockOrigins) > 64 {
		return nil, errors.New("investigation controls are invalid")
	}
	for _, origin := range options.BlockOrigins {
		parsed, err := InvestigationURL(origin)
		if err != nil || parsed.Scheme+"://"+parsed.Host != origin {
			return nil, errors.New("blocked destinations must be exact HTTP(S) origins")
		}
	}
	matcher, err := NewMarkerMatcher(options.Markers)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithTimeout(ctx, 20*time.Minute)
	process, endpoint, err := launchBrowser(lifetime, options.Headless)
	if err != nil {
		cancel()
		return nil, err
	}
	client, err := connectCDP(lifetime, endpoint)
	if err != nil {
		cancel()
		_ = process.close()
		return nil, err
	}
	c := &JourneyCapture{options: options, matcher: matcher, client: client, process: process, cancel: cancel, done: make(chan struct{}), ready: make(chan error, 1), origin: u.Scheme + "://" + u.Host, sessions: map[string]captureSession{}, contexts: map[string]captureSession{}, requests: map[string]requestObservation{}}
	c.result = CaptureResult{Journey: trace.NewJourney(), Destinations: []DestinationName{{Alias: "d1", Origin: c.origin}}, Steps: []InvestigationStep{}}
	c.result.Journey.AddGap("server-side-unobservable")
	c.result.Journey.AddGap("unmatched-information")
	// Browser APIs cannot attest that page-owned hooks remain installed, or expose
	// every response body, storage mechanism, encrypted payload, or service worker.
	// Until those boundaries are proven, absence is explicitly unknown.
	c.result.Journey.AddGap("instrumentation-unavailable")
	var created struct {
		TargetID string `json:"targetId"`
	}
	err = client.call(lifetime, "", "Target.createTarget", map[string]any{"url": "about:blank"}, &created)
	if err != nil {
		client.close()
		cancel()
		_ = process.close()
		return nil, err
	}
	c.target = created.TargetID
	go c.collect(lifetime)
	err = client.call(lifetime, "", "Browser.setDownloadBehavior", map[string]any{"behavior": "deny"}, nil)
	if err == nil {
		err = client.call(lifetime, "", "Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}, nil)
	}
	if err == nil {
		select {
		case err = <-c.ready:
		case <-lifetime.Done():
			err = lifetime.Err()
		case <-time.After(15 * time.Second):
			err = errCDP
		}
	}
	if err == nil {
		err = client.call(lifetime, c.session, "Page.navigate", map[string]any{"url": u.String()}, nil)
	}
	if err != nil {
		_, _ = c.Stop(true)
		return nil, fmt.Errorf("browser recording could not start: %w", err)
	}
	go func() {
		select {
		case <-lifetime.Done():
			_, _ = c.Stop(true)
		case <-c.done:
			_, _ = c.Stop(false)
		}
	}()
	return c, nil
}

func (c *JourneyCapture) configure(ctx context.Context, session, kind string) error {
	for _, command := range []struct {
		method string
		params any
	}{
		{"Runtime.enable", map[string]any{}},
		{"Runtime.addBinding", map[string]any{"name": "__ariadneObserve"}},
		{"Network.enable", map[string]any{"maxTotalBufferSize": 1048576, "maxResourceBufferSize": 262144, "maxPostDataSize": 262144}},
		{"Target.setAutoAttach", map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true}},
	} {
		if err := c.client.call(ctx, session, command.method, command.params, nil); err != nil {
			return err
		}
	}
	patterns := []string{}
	for _, origin := range c.options.BlockOrigins {
		patterns = append(patterns, origin+"/*")
		if strings.HasPrefix(origin, "https://") {
			patterns = append(patterns, "wss://"+strings.TrimPrefix(origin, "https://")+"/*")
		} else {
			patterns = append(patterns, "ws://"+strings.TrimPrefix(origin, "http://")+"/*")
		}
	}
	if len(patterns) > 0 {
		if err := c.client.call(ctx, session, "Network.setBlockedURLs", map[string]any{"urls": patterns}, nil); err != nil {
			return err
		}
	}
	if kind != "worker" {
		if err := c.client.call(ctx, session, "Page.enable", map[string]any{}, nil); err != nil {
			return err
		}
		if err := c.client.call(ctx, session, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": journeyHooks}, nil); err != nil {
			return err
		}
		if err := c.client.call(ctx, session, "Fetch.enable", map[string]any{"patterns": []map[string]string{{"urlPattern": "*", "requestStage": "Request"}}}, nil); err != nil {
			return err
		}
		if c.options.Location == "deny" {
			if err := c.client.call(ctx, "", "Browser.setPermission", map[string]any{"permission": map[string]string{"name": "geolocation"}, "setting": "denied", "origin": c.origin}, nil); err != nil {
				return err
			}
		}
		if c.options.Location == "approximate" {
			if err := c.client.call(ctx, session, "Emulation.setGeolocationOverride", map[string]any{"latitude": 38.9, "longitude": -77.0, "accuracy": 10000}, nil); err != nil {
				return err
			}
			if err := c.client.call(ctx, "", "Browser.setPermission", map[string]any{"permission": map[string]string{"name": "geolocation"}, "setting": "granted", "origin": c.origin}, nil); err != nil {
				return err
			}
		}
	}
	// A target waiting for the debugger cannot execute Runtime.evaluate. Page
	// hooks run on the next navigation; workers need evaluation after resuming.
	if kind == "worker" {
		c.gap("worker-unavailable")
	}
	if err := c.client.call(ctx, session, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil); err != nil {
		return err
	}
	if kind == "page" {
		return nil
	}
	return c.client.call(ctx, session, "Runtime.evaluate", map[string]any{"expression": journeyHooks}, nil)
}

func (c *JourneyCapture) collect(ctx context.Context) {
	defer close(c.done)
	// Target setup may wait for a renderer, while that renderer waits for an
	// intercepted request. Keep request continuations independent of setup and
	// keep the observation reader moving. Both queues are bounded; overload closes
	// the connection and is retained as missing visibility.
	targets, requests := make(chan cdpMessage, 64), make(chan cdpMessage, 64)
	var controls sync.WaitGroup
	for _, queue := range []chan cdpMessage{targets, requests} {
		controls.Add(1)
		go func() {
			defer controls.Done()
			for message := range queue {
				c.event(ctx, message)
			}
		}()
	}
	for message := range c.client.events {
		var queue chan cdpMessage
		switch message.Method {
		case "Target.attachedToTarget", "Target.targetCreated":
			queue = targets
		case "Fetch.requestPaused":
			queue = requests
		default:
			c.event(ctx, message)
			continue
		}
		select {
		case queue <- message:
		default:
			c.gap("event-limit")
			c.client.cancel()
		}
	}
	close(targets)
	close(requests)
	controls.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopping {
		c.result.Journey.AddGap("browser-crashed")
	}
	c.client.mu.Lock()
	overflow := c.client.overflow
	c.client.mu.Unlock()
	if overflow {
		c.result.Journey.AddGap("event-limit")
	}
}

func (c *JourneyCapture) event(ctx context.Context, m cdpMessage) {
	if m.Method == "Target.attachedToTarget" {
		var p struct {
			SessionID  string                               `json:"sessionId"`
			TargetInfo struct{ Type, URL, TargetID string } `json:"targetInfo"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("instrumentation-unavailable")
			return
		}
		c.mu.Lock()
		_, exists := c.sessions[p.SessionID]
		count := len(c.sessions)
		c.mu.Unlock()
		if exists {
			return
		}
		if p.TargetInfo.Type == "page" {
			if p.TargetInfo.TargetID != c.target {
				if p.TargetInfo.URL != "about:blank" {
					c.gap("out-of-scope-navigation")
				}
				_ = c.client.call(ctx, "", "Target.closeTarget", map[string]any{"targetId": p.TargetInfo.TargetID}, nil)
			} else {
				c.mu.Lock()
				alreadyAttached := c.session != ""
				c.mu.Unlock()
				if alreadyAttached {
					c.gap("browser-crashed")
					_ = c.client.call(ctx, "", "Target.closeTarget", map[string]any{"targetId": c.target}, nil)
					return
				}
				c.mu.Lock()
				c.session = p.SessionID
				c.sessions[p.SessionID] = captureSession{context: "page", origin: c.origin}
				c.mu.Unlock()
				err := c.configure(ctx, p.SessionID, "page")
				var tree struct {
					FrameTree struct{ Frame struct{ ID string } }
				}
				if err == nil {
					err = c.client.call(ctx, p.SessionID, "Page.getFrameTree", map[string]any{}, &tree)
				}
				c.mu.Lock()
				c.frame = tree.FrameTree.Frame.ID
				c.mu.Unlock()
				if err == nil {
					err = c.client.call(ctx, p.SessionID, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil)
				}
				c.ready <- err
			}
			return
		}
		kind := "worker"
		if p.TargetInfo.Type == "iframe" {
			kind = "frame"
		}
		if count >= 64 || !slices.Contains([]string{"worker", "shared_worker", "service_worker", "iframe"}, p.TargetInfo.Type) {
			c.gap("worker-unavailable")
			_ = c.client.call(ctx, p.SessionID, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil)
			return
		}
		c.mu.Lock()
		c.sessions[p.SessionID] = captureSession{context: kind, origin: p.TargetInfo.URL}
		c.mu.Unlock()
		if c.configure(ctx, p.SessionID, kind) != nil {
			c.gap(kind + "-unavailable")
		}
		if c.client.call(ctx, p.SessionID, "Runtime.runIfWaitingForDebugger", map[string]any{}, nil) != nil {
			c.gap(kind + "-unavailable")
		}
		return
	}
	if m.Method == "Fetch.requestPaused" {
		var p struct {
			RequestID, FrameID, ResourceType string
			Request                          struct{ URL string }
		}
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("instrumentation-unavailable")
			return
		}
		c.mu.Lock()
		mainFrame := c.frame
		mainSession := c.session
		c.mu.Unlock()
		u, err := InvestigationURL(strings.Split(p.Request.URL, "#")[0])
		outside := err != nil || u.Scheme+"://"+u.Host != c.origin
		block := err != nil
		if err == nil && slices.Contains(c.options.BlockOrigins, u.Scheme+"://"+u.Host) {
			block = true
		}
		if p.ResourceType == "Document" && m.Session == mainSession && p.FrameID == mainFrame && outside {
			c.gap("out-of-scope-navigation")
			block = true
		}
		method := "Fetch.continueRequest"
		params := map[string]any{"requestId": p.RequestID}
		if block {
			method = "Fetch.failRequest"
			params["errorReason"] = "BlockedByClient"
			c.observe("blocked", "network", p.Request.URL, "", nil)
		}
		if c.client.call(ctx, m.Session, method, params, nil) != nil {
			c.gap("instrumentation-unavailable")
		}
		return
	}
	if m.Method == "Target.targetCreated" {
		var p struct {
			TargetInfo struct{ TargetID, Type, URL string }
		}
		if json.Unmarshal(m.Params, &p) == nil && p.TargetInfo.Type == "page" && p.TargetInfo.TargetID != c.target {
			if p.TargetInfo.URL != "about:blank" {
				c.gap("out-of-scope-navigation")
			}
			_ = c.client.call(ctx, "", "Target.closeTarget", map[string]any{"targetId": p.TargetInfo.TargetID}, nil)
		}
		return
	}
	c.mu.Lock()
	session, known := c.sessions[m.Session]
	c.mu.Unlock()
	if !known {
		return
	}
	switch m.Method {
	case "Runtime.executionContextCreated":
		var p struct {
			Context struct {
				ID      int
				Origin  string
				AuxData struct{ FrameID string }
			}
		}
		if json.Unmarshal(m.Params, &p) != nil || p.Context.ID <= 0 {
			c.gap("instrumentation-unavailable")
			return
		}
		c.mu.Lock()
		if len(c.contexts) >= 256 {
			c.result.Journey.AddGap("frame-unavailable")
		} else {
			where := session.context
			if c.frame != "" && p.Context.AuxData.FrameID != "" && p.Context.AuxData.FrameID != c.frame {
				where = "frame"
			}
			c.contexts[m.Session+":"+strconv.Itoa(p.Context.ID)] = captureSession{context: where, origin: p.Context.Origin}
		}
		c.mu.Unlock()
	case "Runtime.executionContextDestroyed", "Runtime.executionContextsCleared":
		var p struct{ ExecutionContextID int }
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("instrumentation-unavailable")
			return
		}
		c.mu.Lock()
		if m.Method == "Runtime.executionContextDestroyed" {
			delete(c.contexts, m.Session+":"+strconv.Itoa(p.ExecutionContextID))
		} else {
			for key := range c.contexts {
				if strings.HasPrefix(key, m.Session+":") {
					delete(c.contexts, key)
				}
			}
		}
		c.mu.Unlock()
	case "Runtime.bindingCalled":
		var p struct {
			Name, Payload      string
			ExecutionContextID int
		}
		var observation struct {
			Kind, Payload, URL, Selector, Gap string
			Checkpoint                        bool
		}
		if json.Unmarshal(m.Params, &p) != nil || p.Name != "__ariadneObserve" || len(p.Payload) > 384<<10 || json.Unmarshal([]byte(p.Payload), &observation) != nil {
			c.gap("unsupported-payload")
			return
		}
		if p.ExecutionContextID > 0 {
			c.mu.Lock()
			where, found := c.contexts[m.Session+":"+strconv.Itoa(p.ExecutionContextID)]
			c.mu.Unlock()
			if found {
				session.context = where.context
			} else {
				c.gap("frame-unavailable")
			}
		}
		if observation.Gap != "" {
			c.gap(observation.Gap)
			return
		}
		if observation.Kind == "ready" {
			c.mu.Lock()
			registered := c.sessions[m.Session]
			registered.ready = true
			c.sessions[m.Session] = registered
			c.mu.Unlock()
			return
		}
		if observation.Kind == "checkpoint" {
			c.mu.Lock()
			c.step(InvestigationStep{Kind: "submit", Checkpoint: true})
			c.mu.Unlock()
			return
		}
		if !slices.Contains([]string{"input", "click", "storage-read", "storage-write", "cookie-read", "cookie-write", "worker-send", "worker-receive", "message-receive", "fetch", "xhr", "beacon"}, observation.Kind) {
			c.gap("unsupported-payload")
			return
		}
		matches, unavailable := c.matcher.Match(observation.Payload)
		if unavailable {
			c.gap("size-limit")
		}
		c.observe(observation.Kind, session.context, observation.URL, "", matches)
		if observation.Kind == "input" || observation.Kind == "click" {
			step := InvestigationStep{Kind: observation.Kind, Selector: observation.Selector, Checkpoint: observation.Kind == "click" || len(matches) == 0}
			if len(matches) > 0 {
				step.Marker = matches[0].Marker
			}
			c.mu.Lock()
			c.step(step)
			c.mu.Unlock()
		}
	case "Network.requestWillBeSent":
		var p struct {
			RequestID string
			Request   struct {
				URL, PostData string
				HasPostData   bool
				Headers       map[string]any
			}
			RedirectResponse json.RawMessage
		}
		if json.Unmarshal(m.Params, &p) != nil || p.RequestID == "" {
			c.gap("instrumentation-unavailable")
			return
		}
		if p.Request.HasPostData && p.Request.PostData == "" {
			c.gap("unsupported-payload")
		}
		headers, _ := json.Marshal(p.Request.Headers)
		matches, unavailable := c.matcher.Match(p.Request.URL + "\n" + p.Request.PostData + "\n" + string(headers))
		if unavailable {
			c.gap("size-limit")
		}
		key := m.Session + ":" + p.RequestID
		c.mu.Lock()
		previous, exists := c.requests[key]
		if len(p.RedirectResponse) > 0 && exists {
			_ = c.result.Journey.Append(trace.JourneyObservation{Kind: "redirect", Context: "network", Destination: previous.destination, Reference: previous.reference, Matches: previous.matches})
		}
		if len(c.requests) >= 4096 {
			c.result.Journey.AddGap("event-limit")
			c.mu.Unlock()
			return
		}
		destination := c.destination(p.Request.URL)
		reference := c.reference()
		if destination != "" && reference != "" {
			c.requests[key] = requestObservation{destination: destination, reference: reference, matches: matches}
			_ = c.result.Journey.Append(trace.JourneyObservation{Kind: "request", Context: "network", Destination: destination, Reference: reference, Matches: matches})
		}
		c.mu.Unlock()
	case "Network.responseReceived", "Network.loadingFailed":
		var p struct {
			RequestID     string
			Canceled      bool
			BlockedReason string
		}
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("instrumentation-unavailable")
			return
		}
		c.mu.Lock()
		request, ok := c.requests[m.Session+":"+p.RequestID]
		if !ok {
			c.result.Journey.AddGap("instrumentation-unavailable")
		} else if m.Method == "Network.responseReceived" {
			_ = c.result.Journey.Append(trace.JourneyObservation{Kind: "response", Context: "network", Destination: request.destination, Reference: request.reference, Matches: []trace.JourneyMatch{}})
		} else if p.BlockedReason == "inspector" {
			_ = c.result.Journey.Append(trace.JourneyObservation{Kind: "blocked", Context: "network", Destination: request.destination, Reference: request.reference, Matches: request.matches})
		} else {
			c.result.Journey.AddGap("request-failed")
		}
		c.mu.Unlock()
	case "Network.webSocketCreated":
		var p struct{ RequestID, URL string }
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("unsupported-payload")
			return
		}
		c.mu.Lock()
		if len(c.requests) < 4096 {
			destination, reference := c.destination(strings.Replace(strings.Replace(p.URL, "wss://", "https://", 1), "ws://", "http://", 1)), c.reference()
			if destination != "" && reference != "" {
				c.requests[m.Session+":"+p.RequestID] = requestObservation{destination: destination, reference: reference}
			}
		}
		c.mu.Unlock()
	case "Network.webSocketFrameSent", "Network.webSocketFrameReceived":
		var p struct {
			RequestID string
			Response  struct {
				Opcode      int
				PayloadData string
			}
		}
		if json.Unmarshal(m.Params, &p) != nil {
			c.gap("unsupported-payload")
			return
		}
		if p.Response.Opcode != 1 {
			c.gap("unsupported-payload")
			return
		}
		payload := p.Response.PayloadData
		matches, unavailable := c.matcher.Match(payload)
		if unavailable {
			c.gap("size-limit")
		}
		kind := "websocket-sent"
		if m.Method == "Network.webSocketFrameReceived" {
			kind = "websocket-received"
		}
		c.mu.Lock()
		request, ok := c.requests[m.Session+":"+p.RequestID]
		if ok {
			_ = c.result.Journey.Append(trace.JourneyObservation{Kind: kind, Context: "network", Destination: request.destination, Reference: request.reference, Matches: matches})
		} else {
			c.result.Journey.AddGap("instrumentation-unavailable")
		}
		c.mu.Unlock()
	case "Inspector.targetCrashed", "Target.targetCrashed":
		c.gap("browser-crashed")
	}
}

func (c *JourneyCapture) gap(reason string) {
	c.mu.Lock()
	c.result.Journey.AddGap(reason)
	c.mu.Unlock()
}
func (c *JourneyCapture) reference() string {
	c.references++
	if c.references > trace.MaxJourneyObservations*16 {
		c.result.Journey.AddGap("event-limit")
		return ""
	}
	return "r" + strconv.Itoa(c.references)
}
func (c *JourneyCapture) destination(raw string) string {
	u, err := InvestigationURL(strings.Split(raw, "#")[0])
	if err != nil {
		c.result.Journey.AddGap("unsupported-payload")
		return ""
	}
	origin := u.Scheme + "://" + strings.ToLower(u.Host)
	for _, destination := range c.result.Destinations {
		if destination.Origin == origin {
			return destination.Alias
		}
	}
	if len(c.result.Destinations) >= 256 {
		c.result.Journey.AddGap("size-limit")
		return ""
	}
	alias := "d" + strconv.Itoa(len(c.result.Destinations)+1)
	c.result.Destinations = append(c.result.Destinations, DestinationName{Alias: alias, Origin: origin})
	return alias
}
func (c *JourneyCapture) observe(kind, where, raw, reference string, matches []trace.JourneyMatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	destination := c.destination(raw)
	if reference == "" {
		reference = c.reference()
	}
	if matches == nil {
		matches = []trace.JourneyMatch{}
	}
	if destination != "" && reference != "" {
		if c.result.Journey.Append(trace.JourneyObservation{Kind: kind, Context: where, Destination: destination, Reference: reference, Matches: matches}) != nil {
			c.result.Journey.AddGap("event-limit")
		}
	}
}
func (c *JourneyCapture) step(step InvestigationStep) {
	if len(c.result.Steps) >= 64 || len(step.Selector) > 512 {
		c.result.Journey.AddGap("manual-checkpoint")
		return
	}
	c.result.Steps = append(c.result.Steps, step)
}

// Snapshot returns detached safe observations, with no raw payloads or marker values.
func (c *JourneyCapture) Snapshot() CaptureResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := c.result
	result.Journey.Observations = slices.Clone(c.result.Journey.Observations)
	for i := range result.Journey.Observations {
		result.Journey.Observations[i].Matches = slices.Clone(result.Journey.Observations[i].Matches)
	}
	result.Journey.Gaps = slices.Clone(c.result.Journey.Gaps)
	result.Journey.LinkMatches()
	result.Destinations = slices.Clone(c.result.Destinations)
	result.Steps = slices.Clone(c.result.Steps)
	return result
}

// Recording reports whether the browser connection is still collecting events.
func (c *JourneyCapture) Recording() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// Stop ends recording, closes the fresh profile, and waits for collector cleanup.
// Cancellation preserves no exported evidence unless the caller explicitly saves it.
func (c *JourneyCapture) Stop(cancelled bool) (CaptureResult, error) {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	if c.stopped {
		return c.Snapshot(), c.cleanupError
	}
	c.mu.Lock()
	c.stopping = true
	if cancelled {
		c.result.Journey.AddGap("cancelled")
	}
	c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = c.client.call(ctx, "", "Browser.close", map[string]any{}, nil)
	cancel()
	c.client.close()
	c.cancel()
	<-c.done
	c.cleanupError = c.process.close()
	c.stopped = true
	return c.Snapshot(), c.cleanupError
}

// FillSynthetic enters only a generated marker into one structural selector.
// It never submits a form, clicks a control, or accepts an arbitrary script.
func (c *JourneyCapture) FillSynthetic(ctx context.Context, selector, markerID string) error {
	if len(selector) == 0 || len(selector) > 512 {
		return errors.New("choose a single input field")
	}
	marker := SyntheticMarker{}
	for _, item := range c.options.Markers {
		if item.ID == markerID {
			marker = item
		}
	}
	if marker.ID == "" {
		return errors.New("synthetic input is unavailable")
	}
	selectorJSON, _ := json.Marshal(selector)
	valueJSON, _ := json.Marshal(marker.Value)
	expression := `(() => { const fields=document.querySelectorAll(` + string(selectorJSON) + `);if(fields.length!==1) return false;const el=fields[0];if(el.disabled || el.readOnly || !(el instanceof HTMLTextAreaElement || (el instanceof HTMLInputElement && ['text','email','url','tel','search'].includes(el.type))))return false;el.value=` + string(valueJSON) + `;el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));return true; })()`
	var result struct {
		Result           struct{ Value bool }
		ExceptionDetails json.RawMessage
	}
	if c.client.call(ctx, c.session, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &result) != nil || !result.Result.Value || len(result.ExceptionDetails) > 0 {
		return errors.New("input could not be filled; select one text field in the recording browser")
	}
	return nil
}
