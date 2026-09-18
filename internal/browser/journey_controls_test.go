package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func requireJourneyBrowser(t *testing.T) {
	t.Helper()
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("set ARIADNE_BROWSER_TESTS=1 for installed Chrome/Edge acceptance")
	}
}

func waitJourney(t *testing.T, capture *JourneyCapture, predicate func(CaptureResult) bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if predicate(capture.Snapshot()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("recording did not reach expected state: %+v", capture.Snapshot())
}

func evaluateFixture(t *testing.T, capture *JourneyCapture, expression string) any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var response struct {
		Result           struct{ Value any }
		ExceptionDetails json.RawMessage
	}
	if err := capture.client.call(ctx, capture.session, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &response); err != nil || len(response.ExceptionDetails) > 0 {
		t.Fatalf("fixture evaluation: %v, %s", err, response.ExceptionDetails)
	}
	return response.Result.Value
}

func TestJourneyControlsBlockPageAndWorkerRequests(t *testing.T) {
	requireJourneyBrowser(t)
	var received atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		fmt.Fprint(w, "received")
	}))
	defer destination.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/worker.js" {
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprintf(w, `onmessage=async e=>{try{await fetch(%q+'/?worker='+encodeURIComponent(e.data))}catch{postMessage('blocked')}}`, destination.URL)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<input id="email"><input id="password" type="password"><input id="check" type="checkbox"><input id="readonly" readonly><script>const worker=new Worker('/worker.js');document.querySelector('#email').oninput=e=>{worker.postMessage(e.target.value);fetch(%q+'/?page='+encodeURIComponent(e.target.value)).catch(()=>{});};</script>`, destination.URL)
	}))
	defer site.Close()
	capture, err := StartCapture(context.Background(), CaptureOptions{URL: site.URL, Markers: GenerateMarkers(), Headless: true, Location: "deny", BlockOrigins: []string{destination.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop(true)
	waitJourney(t, capture, func(result CaptureResult) bool {
		for _, o := range result.Journey.Observations {
			if o.Kind == "response" {
				return true
			}
		}
		return false
	})
	for _, selector := range []string{"#password", "#check", "#readonly", "input", "[", strings.Repeat("a", 513), ""} {
		if err := capture.FillSynthetic(context.Background(), selector, "m1"); err == nil {
			t.Errorf("filled unsupported selector %q", selector)
		}
	}
	if err := capture.FillSynthetic(context.Background(), "#email", "m99"); err == nil {
		t.Fatal("filled unknown marker")
	}
	if err := capture.FillSynthetic(context.Background(), "#email", "m1"); err != nil {
		t.Fatal(err)
	}
	waitJourney(t, capture, func(result CaptureResult) bool {
		count := 0
		for _, o := range result.Journey.Observations {
			if o.Kind == "blocked" && len(o.Matches) > 0 {
				count++
			}
		}
		return count >= 2
	})
	if received.Load() != 0 {
		t.Fatalf("blocked destination received %d requests", received.Load())
	}
	permission := evaluateFixture(t, capture, `navigator.permissions.query({name:'geolocation'}).then(x=>x.state)`)
	if permission != "denied" {
		t.Fatalf("geolocation permission: %v", permission)
	}
	_, err = capture.Stop(false)
	if err != nil {
		t.Fatal(err)
	}
}

func TestJourneyControlsBlockOutOfScopeNavigation(t *testing.T) {
	requireJourneyBrowser(t)
	var reached atomic.Int32
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); fmt.Fprint(w, "out of scope") }))
	defer outside.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<input autofocus><p>Local fixture</p>`)
	}))
	defer site.Close()
	capture, err := StartCapture(context.Background(), CaptureOptions{URL: site.URL, Markers: GenerateMarkers(), Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop(true)
	waitJourney(t, capture, func(r CaptureResult) bool { return len(r.Journey.Observations) >= 2 })
	if err := capture.client.call(context.Background(), capture.session, "Page.navigate", map[string]any{"url": outside.URL}, nil); err != nil {
		t.Fatal(err)
	}
	waitJourney(t, capture, func(r CaptureResult) bool { return slices.Contains(r.Journey.Gaps, "out-of-scope-navigation") })
	if reached.Load() != 0 {
		t.Fatal("out-of-scope navigation reached the server")
	}
}

func TestJourneyWebSocketTextAndBinaryVisibility(t *testing.T) {
	requireJourneyBrowser(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/socket" {
			connection, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer connection.CloseNow()
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			kind, data, err := connection.Read(ctx)
			if err != nil {
				return
			}
			_ = connection.Write(ctx, kind, data)
			_ = connection.Write(ctx, websocket.MessageBinary, []byte{1, 2, 3})
			_, _, _ = connection.Read(ctx)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<input id="email" autofocus><script>const socket=new WebSocket('ws://'+location.host+'/socket');document.querySelector('input').oninput=e=>socket.send(e.target.value);</script>`)
	}))
	defer site.Close()
	capture, err := StartCapture(context.Background(), CaptureOptions{URL: site.URL, Markers: GenerateMarkers(), Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop(true)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if evaluateFixture(t, capture, `typeof socket!=='undefined'&&socket.readyState===1`) == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("socket unavailable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := capture.FillSynthetic(context.Background(), "#email", "m1"); err != nil {
		t.Fatal(err)
	}
	waitJourney(t, capture, func(r CaptureResult) bool {
		received := false
		for _, o := range r.Journey.Observations {
			if o.Kind == "websocket-received" && len(o.Matches) > 0 {
				received = true
			}
		}
		return received && slices.Contains(r.Journey.Gaps, "unsupported-payload")
	})
}

func TestJourneyCancellationAndLostBrowserCleanProfiles(t *testing.T) {
	requireJourneyBrowser(t)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "local fixture") }))
	defer site.Close()
	for _, mode := range []string{"cancel", "disconnect"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			capture, err := StartCapture(ctx, CaptureOptions{URL: site.URL, Markers: GenerateMarkers(), Headless: true})
			if err != nil {
				t.Fatal(err)
			}
			defer capture.Stop(true)
			if mode == "cancel" {
				cancel()
			} else {
				capture.client.close()
			}
			select {
			case <-capture.done:
			case <-time.After(8 * time.Second):
				t.Fatal("capture retained after shutdown")
			}
			result, err := capture.Stop(mode == "cancel")
			if err != nil {
				t.Fatal(err)
			}
			gap := "cancelled"
			if mode == "disconnect" {
				gap = "browser-crashed"
			}
			if !slices.Contains(result.Journey.Gaps, gap) {
				t.Fatalf("missing %s", gap)
			}
			if _, err := os.Stat(capture.process.profile); !os.IsNotExist(err) {
				t.Fatal("profile retained")
			}
			if capture.Recording() {
				t.Fatal("closed capture is recording")
			}
		})
	}
}

func TestJourneyCaptureRejectsInvalidControlsBeforeLaunching(t *testing.T) {
	for _, options := range []CaptureOptions{
		{URL: "file:///private"},
		{URL: "https://fixture.invalid", Location: "invalid"},
		{URL: "https://fixture.invalid", BlockOrigins: make([]string, 65)},
		{URL: "https://fixture.invalid", BlockOrigins: []string{"https://fixture.invalid/path"}},
		{URL: "https://fixture.invalid"},
	} {
		if _, err := StartCapture(context.Background(), options); err == nil {
			t.Fatal("accepted invalid controls")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StartCapture(ctx, CaptureOptions{}); err == nil {
		t.Fatal("launched cancelled capture")
	}
}

func TestJourneyFramesXHRBeaconAndSyntheticLocation(t *testing.T) {
	requireJourneyBrowser(t)
	marker := "frame-fixture@example.invalid"
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/send" {
			fmt.Fprint(w, "ok")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<script>onmessage=e=>{localStorage.setItem('fixture',e.data);document.cookie='fixture='+e.data;const x=new XMLHttpRequest();x.open('POST','/send');x.send(e.data);navigator.sendBeacon('/send',e.data);};parent.postMessage('ready','*');</script>`)
	}))
	defer frame.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<script>onmessage=e=>{if(e.data==='ready')e.source.postMessage(%q,'*')};</script><iframe src=%q></iframe>`, marker, frame.URL)
	}))
	defer site.Close()
	capture, err := StartCapture(context.Background(), CaptureOptions{URL: site.URL, Headless: true, Location: "approximate", Markers: []SyntheticMarker{{ID: "m1", Category: "email", Value: marker}}})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop(true)
	waitJourney(t, capture, func(r CaptureResult) bool {
		found := map[string]bool{}
		for _, o := range r.Journey.Observations {
			if o.Context == "frame" && len(o.Matches) > 0 {
				found[o.Kind] = true
			}
		}
		return found["storage-write"] && found["xhr"] && found["beacon"]
	})
	location := evaluateFixture(t, capture, `new Promise(resolve=>navigator.geolocation.getCurrentPosition(p=>resolve([p.coords.latitude,p.coords.longitude,p.coords.accuracy]),()=>resolve('denied')))`)
	values, ok := location.([]any)
	if !ok || len(values) != 3 || values[0] != float64(38.9) || values[1] != float64(-77.0) || values[2] != float64(10000) {
		t.Fatalf("unexpected lab location: %v", location)
	}
}
