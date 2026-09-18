package ui

import (
	"html/template"
	"net/http"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

var sourceAdapterTemplate = template.Must(template.New("source-adapter").Funcs(template.FuncMap{
	"sourceAdapterMeaning": sourceAdapterMeaning,
	"categoryLabel":        trace.CategoryLabel,
	"categoryMeaning":      trace.CategoryMeaning,
	"destinationLabel":     trace.DestinationLabel,
	"destinationMeaning":   trace.DestinationMeaning,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Source adapter · Ariadne</title>
<style>
:root{color-scheme:light}*{box-sizing:border-box}body{font:16px/1.6 system-ui,sans-serif;margin:0;background:#f3f8f3;color:#13291f}main{max-width:900px;margin:0 auto;padding:32px 22px 64px}a{color:#176341}.topnav{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:34px}.topnav a:last-child{font-weight:750;text-decoration:none}.topnav a:last-child:hover{text-decoration:underline}.eyebrow{font-size:12px;font-weight:800;letter-spacing:1.5px;text-transform:uppercase;color:#5e7167}h1{font-size:clamp(32px,6vw,52px);line-height:1.08;letter-spacing:-1.5px;margin:18px 0 12px}.lede{font-size:19px;color:#5e7167;max-width:680px}.panel{background:#fffefa;border:1px solid #d5e2d9;border-radius:18px;padding:24px;margin:22px 0;box-shadow:0 10px 28px rgba(23,99,65,.07)}.flow{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin:20px 0}.step{background:#dff1e5;border-radius:12px;padding:16px}.step strong{display:block}.step span{display:block;color:#5e7167;font-size:14px}.answer{border-left:5px solid #176341}.meaning{max-width:720px;font-size:20px;line-height:1.35;color:#0f4c32}.button{display:inline-flex;align-items:center;gap:12px;border:1px solid #176341;border-radius:10px;color:#fff;background:#176341;padding:10px 13px;font-weight:700;text-decoration:none}.button:hover{background:#0f4c32}.warning{background:#fff9e9;border-color:#e1d3ac}.tag{display:inline-block;border:1px solid #d5e2d9;border-radius:999px;padding:4px 10px;font-size:13px;color:#176341}.unknown{color:#8a5a00}.path{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px;margin:20px 0;padding:0;list-style:none}.path li{position:relative;min-height:92px;border:1px solid #d5e2d9;border-radius:14px;background:#fffefa;padding:14px}.path li+li:before{position:absolute;left:-9px;top:35px;color:#176341;content:"→";font-weight:800}.path-label{display:block;color:#5e7167;font-size:11px;font-weight:800;letter-spacing:1px;text-transform:uppercase}.path strong{display:block;margin-top:6px;overflow-wrap:anywhere}.path small{display:block;margin-top:4px;color:#5e7167;font-size:12px}.path-tags{display:flex;flex-wrap:wrap;gap:6px;margin-top:7px}.path-tags .tag strong{display:block;font-size:12px}.path-tags .tag code{display:block;color:#5e7167;font-size:11px}.path-destination code{display:block;color:#5e7167;font-size:11px}.path-destination small{line-height:1.35}.scroll{overflow-x:auto}table{border-collapse:collapse;width:100%}th,td{text-align:left;padding:10px;border-bottom:1px solid #d5e2d9;vertical-align:top}code{overflow-wrap:anywhere;font-size:12px}summary{cursor:pointer;font-weight:700;min-height:30px}:focus-visible{outline:3px solid #0f4c32;outline-offset:3px}@media(max-width:650px){main{padding:24px 16px}.topnav{align-items:flex-start;flex-direction:column;gap:8px;margin-bottom:26px}.flow{grid-template-columns:1fr}.path{grid-template-columns:1fr}.path li+li:before{left:15px;top:-12px;content:"↓"}.panel{padding:20px}}
</style></head><body><main>
<nav class="topnav" aria-label="Ariadne navigation"><a href="/">Ariadne / investigations</a><a href="/guide">How Ariadne works</a></nav>
<header><p class="eyebrow">A redacted information trail</p><h1>What did this source observe?</h1><p class="lede">This page explains one authorized adapter run in plain language. It shows the safe labels Ariadne retained, not the original payloads or executable details.</p></header>
<section class="panel answer"><p class="eyebrow">In plain language</p><p class="meaning">{{sourceAdapterMeaning .Receipt.Completeness .Receipt.Events}}</p><p class="eyebrow">How to read it</p><div class="flow"><div class="step"><strong>1 · Source</strong><span>{{.Receipt.Source}} adapter was allowed to report observations.</span></div><div class="step"><strong>2 · Redacted trace</strong><span>{{.Trace.Events}} safe event(s) were retained.</span></div><div class="step"><strong>3 · Evidence</strong><span>The receipt binds the trace and session identities.</span></div></div>
<p><strong>{{.Trace.Events}} observation(s)</strong> were recorded for the <strong>{{.Receipt.Scope}}</strong> scope. Completeness: <span class="tag">{{.Trace.Completeness}}</span></p>
<a class="button" href="#observed-paths">See the observed paths <span aria-hidden="true">&rarr;</span></a>
</section>
<section class="panel" id="observed-paths"><p class="eyebrow">Observed paths</p><h2>Where the safe labels appeared</h2><p>Read each path as source → reviewed category → destination. These are labels only; the original payloads and values are not here.</p><div>{{range .TraceEvents}}<ol class="path" aria-label="Recorded source-adapter path"><li><span class="path-label">Source</span><strong>{{.Source}}</strong><small>{{.Channel}} / {{.Kind}}</small></li><li><span class="path-label">Reviewed category</span><div class="path-tags">{{range .Fields}}<span class="tag"><strong>{{categoryLabel .}}</strong><code>{{.}}</code></span>{{end}}</div><small>{{range .Fields}}{{categoryMeaning .}} {{end}}</small></li><li class="path-destination"><span class="path-label">Destination</span><strong>{{destinationLabel .Destination}}</strong><code>{{.Destination}}</code><small>{{destinationMeaning .Destination}}</small><small>recorded boundary</small></li></ol>{{else}}<p class="unknown">No safe event paths were retained.</p>{{end}}</div></section>

<section class="panel"><p class="eyebrow">What this proves</p><h2>The saved files agree with each other.</h2><p>Receipt, trace, and session identities were rechecked when this page opened. That proves consistency of this bundle; it does not prove that an external service was truthful or reveal what happened after a response.</p>
<table><tr><th>Adapter</th><td>{{.Receipt.Adapter}} v{{.Receipt.AdapterVersion}}</td></tr><tr><th>Source label</th><td>{{.Receipt.Source}}</td></tr><tr><th>Scope</th><td>{{.Receipt.Scope}}</td></tr><tr><th>Completeness</th><td>{{.Receipt.Completeness}}</td></tr><tr><th>Provenance</th><td>{{if .Receipt.ProvenanceSHA256}}verified{{else}}<span class="unknown">unavailable in this receipt</span>{{end}}</td></tr><tr><th>Replay</th><td><span class="unknown">unavailable</span> — portable runs do not carry the procedure or executable bytes.</td></tr></table>
</section>
<section class="panel warning"><p class="eyebrow">What remains unknown</p><h2>A redacted trace is a starting point.</h2><p>Event labels do not include captured values, request bodies, or URLs. They also cannot show server-side storage, onward sharing, unsupported channels, or anything the adapter did not report. Ariadne keeps those limits visible instead of turning them into a privacy guarantee.</p>
<details><summary>Technical identities</summary><dl><dt>receipt</dt><dd><code>{{.ReceiptSHA256}}</code></dd><dt>trace</dt><dd><code>{{.Trace.TraceSHA256}}</code></dd><dt>session</dt><dd><code>{{.Session.SessionSHA256}}</code></dd>{{if .Receipt.ProvenanceSHA256}}<dt>provenance</dt><dd><code>{{.Receipt.ProvenanceSHA256}}</code></dd>{{end}}</dl></details>
</section>
</main></body></html>`))

func sourceAdapterMeaning(completeness string, events int) string {
	switch {
	case completeness == trace.Partial && events == 0:
		return "No supported observations were reported; missing channels remain unknown."
	case completeness == trace.Partial:
		return "Some reviewed labels were found, but missing channels remain unknown."
	case completeness == trace.Complete && events == 0:
		return "No supported observations were reported; this is not proof that nothing left the source."
	case completeness == trace.Complete:
		return "The source reported reviewed labels from the channels this run could inspect."
	default:
		return "Coverage is not fully described; missing or unsupported activity remains unknown."
	}
}

func (h handler) handleSourceAdapter(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/source-adapter" || h.sourceAdapterPath == "" {
		http.NotFound(w, r)
		return
	}
	if h.sourceAdapterVerify == nil {
		http.Error(w, "source adapter unavailable", http.StatusUnprocessableEntity)
		return
	}
	summary, err := h.sourceAdapterVerify(h.sourceAdapterPath)
	if err != nil {
		http.Error(w, "source adapter unavailable", http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := sourceAdapterTemplate.Execute(w, summary); err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
	}
}
