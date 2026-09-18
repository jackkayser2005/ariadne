package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJourneyCaptureMultiOriginSyntheticJourney(t *testing.T) {
	if os.Getenv("ARIADNE_BROWSER_TESTS") != "1" {
		t.Skip("set ARIADNE_BROWSER_TESTS=1 for installed Chrome/Edge acceptance")
	}
	var requests chan string = make(chan string, 16)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		select {
		case requests <- r.URL.RawQuery:
		default:
		}
		fmt.Fprint(w, "ok")
	}))
	defer destination.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/worker.js" {
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprintf(w, `onmessage=async e=>{const hash=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(e.data)))).map(x=>x.toString(16).padStart(2,'0')).join('');await fetch(%q+'/hashed?v='+hash);postMessage(e.data);};`, destination.URL)
			return
		}
		fmt.Fprintf(w, `<!doctype html><title>Local journey fixture</title><label>Email <input id="email"></label><button id="send" type="button">Send test input</button><script>
const worker=new Worker('/worker.js');document.querySelector('#send').onclick=async()=>{const value=document.querySelector('#email').value;localStorage.setItem('fixture',value);document.cookie='fixture='+encodeURIComponent(value);worker.postMessage(localStorage.getItem('fixture'));await fetch(%q+'/exact?v='+encodeURIComponent(value));await fetch(%q+'/encoded?v='+btoa(value));await fetch(%q+'/negative?v=negative-control');document.body.dataset.finished='yes';};</script>`, destination.URL, destination.URL, destination.URL)
	}))
	defer site.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	markers := []SyntheticMarker{{ID: "m1", Category: "email", Value: "ariadne-fixture@example.invalid"}}
	capture, err := StartCapture(ctx, CaptureOptions{URL: site.URL, Markers: markers, Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Stop(true)
	waitFixtureReady(t, capture, `!!document.querySelector('#send')?.onclick`)
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = capture.FillSynthetic(ctx, "#email", "m1")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Only the deterministic, local fixture has an automated submit action.
	if err := capture.client.call(ctx, capture.session, "Runtime.evaluate", map[string]any{"expression": "document.querySelector('#send').click()"}, nil); err != nil {
		t.Fatal(err)
	}
	for received := 0; received < 4; received++ {
		select {
		case <-requests:
		case <-time.After(8 * time.Second):
			var status any
			_ = capture.client.call(ctx, capture.session, "Runtime.evaluate", map[string]any{"expression": "JSON.stringify({ready:document.readyState,worker:typeof worker,finished:document.body.dataset.finished})", "returnByValue": true}, &status)
			capture.client.mu.Lock()
			readError := capture.client.readError
			capture.client.mu.Unlock()
			t.Fatalf("fixture stopped at %d requests; connection error %v; status %#v; safe observations %#v", received, readError, status, capture.Snapshot())
		case <-ctx.Done():
			t.Fatal("fixture did not finish")
		}
	}
	for {
		result := capture.Snapshot()
		found := false
		for _, o := range result.Journey.Observations {
			for _, m := range o.Matches {
				if m.Encoding == "sha256" {
					found = true
				}
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker SHA-256 request was not observed")
		}
		time.Sleep(30 * time.Millisecond)
	}
	result, err := capture.Stop(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(capture.process.profile); !os.IsNotExist(err) {
		t.Fatal("fresh profile retained")
	}
	if err := result.Journey.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"input", "storage-write", "storage-read", "worker-send", "request"} {
		found := false
		for _, o := range result.Journey.Observations {
			if o.Kind == kind && len(o.Matches) > 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s observation", kind)
		}
	}
	for _, encoding := range []string{"exact", "url", "base64", "sha256"} {
		found := false
		for _, o := range result.Journey.Observations {
			for _, m := range o.Matches {
				if m.Encoding == encoding {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("missing %s match", encoding)
		}
	}
	data, err := result.Journey.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), markers[0].Value) || strings.Contains(string(data), destination.URL) || strings.Contains(string(data), "negative-control") {
		t.Fatal("raw values leaked")
	}
	if len(result.Destinations) != 2 {
		t.Fatalf("destinations: %#v", result.Destinations)
	}
	// A response only references its request. It must not claim that request bytes
	// were present in the response or observed server-side.
	for _, o := range result.Journey.Observations {
		if o.Kind == "response" && len(o.Matches) != 0 {
			t.Fatal("response invented a payload match")
		}
	}
	encoded, _ := json.Marshal(result.Journey)
	t.Logf("%d observations, %d private destinations, %d artifact bytes", len(result.Journey.Observations), len(result.Destinations), len(encoded))
}
