package ui

import (
	"html/template"
	"net/http"

	"github.com/jackkayser2005/ariadne/internal/browser"
	"github.com/jackkayser2005/ariadne/internal/trace"
)

var weatherTemplate = template.Must(template.New("weather").Funcs(template.FuncMap{
	"categoryLabel":      trace.CategoryLabel,
	"categoryMeaning":    trace.CategoryMeaning,
	"destinationLabel":   weatherDestinationLabel,
	"destinationMeaning": weatherDestinationMeaning,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Your information, explained · Ariadne</title>
<style>
:root{color-scheme:light}*{box-sizing:border-box}body{font:17px/1.6 system-ui,sans-serif;margin:0;background:#f3f8f3;color:#13291f}main{max-width:1020px;margin:0 auto;padding:36px 24px 72px}a{color:#176341}.topnav{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:34px}.topnav a:last-child{font-weight:750;text-decoration:none}.topnav a:last-child:hover{text-decoration:underline}h1{font-size:clamp(30px,5vw,48px);line-height:1.15;letter-spacing:-1.4px;margin:20px 0}h2{font-size:25px;line-height:1.3;margin:0 0 12px}h3{font-size:18px;margin:0 0 8px}p{margin:8px 0 16px}.eyebrow{font-size:12px;font-weight:750;letter-spacing:1.5px;text-transform:uppercase;color:#5e7167}.intro{max-width:720px;font-size:19px}.note{color:#5e7167;font-size:14px}.panel{background:white;border:1px solid #d5e2d9;border-radius:18px;padding:28px;margin:24px 0}.answer{border-left:5px solid #176341}.flow{display:flex;align-items:center;gap:12px;list-style:none;padding:0;margin:24px 0}.flow li{flex:1;background:#dff1e5;border-radius:12px;padding:18px}.flow li+li:before{content:'→';margin-right:8px;color:#5e7167}.flow strong{display:block}.options{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px;margin:20px 0}.option{border:1px solid #d5e2d9;border-radius:12px;padding:18px}.result{font-size:25px;font-weight:700;line-height:1.2;margin:16px 0 4px}.limits{background:#fff9e9;border-color:#e1d3ac}details{border-top:1px solid #d5e2d9;padding:16px 0}summary{cursor:pointer;font-weight:650;min-height:32px}summary:focus-visible,a:focus-visible{outline:3px solid #0f4c32;outline-offset:4px}.technical{font-size:14px}.scroll{overflow-x:auto}table{border-collapse:collapse;width:100%}td,th{text-align:left;padding:10px;border-bottom:1px solid #ddd;vertical-align:top}code{overflow-wrap:anywhere;font-size:12px}.tag{display:inline-block;font-size:12px;border:1px solid #d5e2d9;border-radius:99px;padding:3px 10px}.technical p{overflow-wrap:anywhere}@media(max-width:680px){main{padding:24px 16px}.topnav{align-items:flex-start;flex-direction:column;gap:8px;margin-bottom:26px}.panel{padding:20px}.options{grid-template-columns:1fr}.flow{flex-direction:column;align-items:stretch}.flow li+li:before{content:'↓'}h1{letter-spacing:-.7px}}
</style></head><body><main>
<nav class="topnav" aria-label="Ariadne navigation"><a href="/">Ariadne / investigations</a><a href="/guide">How Ariadne works</a></nav>
<header><p class="eyebrow">Your information, explained</p><h1>Where did my information go?</h1><p class="intro">We tested the weather website with a precise location, a city-level location, and location access turned off. Here is what we could see.</p><p class="note">This example uses test locations, not your real location. It covers one website workflow, not everything on your device.</p></header>
<section class="panel answer" aria-labelledby="observed-title"><p class="eyebrow">What we saw</p>
{{range .CandidateEvidence}}{{if eq .Candidate "precise"}}
{{if gt .ResponseBackedSessions 0}}<h2 id="observed-title">Location was sent to the weather service.</h2><p>We saw the test location in a request from the browser, followed by a response from the website.</p>
<ol class="flow" aria-label="Observed information flow"><li><strong>Test browser</strong>Where the request started</li><li><strong>Location</strong>The information in the request</li><li><strong>Weather website</strong>The destination we observed</li></ol>
<p class="note">Seen in {{.ResponseBackedSessions}} of {{.SessionCount}} precise-location sessions. A response does not tell us whether the service stored or shared the location afterward.</p>
{{else}}<h2 id="observed-title">We could not confirm where location was sent.</h2><p>The recorded test did not contain a supported location match with a response. That does not mean the location stayed private.</p>{{end}}
{{end}}{{end}}</section>
<section class="panel" aria-labelledby="less-title"><p class="eyebrow">Could I share less?</p><h2 id="less-title">Here is what still worked.</h2><p>We checked whether the website showed a local forecast. These counts describe this test, not a guarantee about every visit.</p>
<div class="options">{{range .CandidateEvidence}}<article class="option">
{{if eq .Candidate "precise"}}<h3>Precise location</h3><p class="note">A specific point on the map.</p>{{else if eq .Candidate "coarse"}}<h3>City-level location</h3><p class="note">A city-center point instead.</p>{{else}}<h3>Location access off</h3><p class="note">The browser denied location access.</p>{{end}}
<p class="result">{{.AvailableSessions}} / {{.SessionCount}}</p><p>sessions showed a forecast</p>{{if gt .UnknownSessions 0}}<p class="note">{{.UnknownSessions}} session(s) could not be checked.</p>{{end}}
{{if gt .ResponseBackedSessions 0}}<p class="note">Location was observed in a request with a response in {{.ResponseBackedSessions}} session(s).</p>{{else}}<p class="note">No response-backed location match was recorded. Other ways of estimating location, such as an IP address, were not ruled out.</p>{{end}}
</article>{{end}}</div>
{{range .CandidateEvidence}}{{if eq .Candidate "coarse"}}{{if and (gt .SessionCount 0) (eq .AvailableSessions .SessionCount)}}<p><strong>City-level location produced a forecast every time it was tested.</strong> This suggests a useful next comparison; it does not establish that sharing it is safe.</p>{{end}}{{end}}{{end}}
</section>
<section class="panel limits" aria-labelledby="limits-title"><p class="eyebrow">What this means for you</p><h2 id="limits-title">A useful clue, with missing pieces.</h2>
{{if eq .Ladder.SelectionState "unknown"}}<p>We cannot yet recommend a minimum amount of location to share. Some activity was blocked or outside the test's view, so a privacy conclusion would go beyond the evidence.</p>{{else}}<p>The recorded comparison is available below. Even a working lower-detail option does not tell us everything the website does with your information.</p>{{end}}
<p>If a site offers a manual city search, that is a separate option to investigate. Turning off browser location is not the same as hiding your location from a website.</p>
<details><summary>Why can't Ariadne see everything?</summary><p>This test watches a limited set of browser requests. It cannot see what happens inside a company's servers. It also blocks destinations that were not included in the reviewed test.</p>{{range .CandidateEvidence}}{{if eq .Candidate "precise"}}<ul>{{range .VisibilityGaps}}<li>{{.Reason}}</li>{{end}}</ul>{{end}}{{end}}<p>Ariadne keeps those missing pieces visible rather than treating them as proof that nothing happened.</p></details></section>
<section class="panel" aria-labelledby="reading-title"><h2 id="reading-title">How to read an information trail</h2><p>Start with what left the browser, then look at where it went and what happened when we shared less.</p>
<details><summary>What do the evidence labels mean?</summary><dl><dt><strong>Attempted</strong></dt><dd>The browser tried to make a request containing the test location. It may have been blocked before reaching its destination.</dd><dt><strong>Response-backed</strong></dt><dd>We matched the test location in a request and observed a response. That does not prove the service saved it.</dd><dt><strong>Unknown</strong></dt><dd>We do not have enough visibility to answer. It is not a privacy pass or a failure.</dd><dt><strong>Verified evidence</strong></dt><dd>The saved files agree with their recorded checks and derived results. This does not independently prove that the original capture was truthful.</dd></dl></details>
<p class="note">The broader aim is to follow different kinds of personal information across websites and apps. This page currently explains only the saved weather experiment; email, messages, and other apps are not being watched.</p></section>
<details class="panel technical"><summary>Explore the technical evidence</summary>
<p>This section contains the session records and verification IDs behind the explanation. Reverified on every request; the saved evidence has not been rewritten.</p>
<h2>Comparison</h2><p>Selection: <strong>{{.Ladder.SelectionState}}</strong> {{.Ladder.SelectedCandidate}} · Evidence state: {{.Ladder.EvidenceState}}</p>
<div class="scroll"><table><tr><th>Candidate</th><th>Classification</th><th>Outcome</th><th>Unknown pairs</th></tr>{{range .Ladder.CandidateResults}}<tr><td>{{.ID}}</td><td>{{.Classification}}</td><td>{{.Outcome}}</td><td>{{.UnknownPairs}}</td></tr>{{end}}</table></div>
<h2>Observed session outcomes</h2><div class="scroll"><table><tr><th>Candidate</th><th>Forecast available</th><th>Unavailable</th><th>Unknown</th><th>Attempted match</th><th>Response-backed match</th></tr>{{range .CandidateEvidence}}<tr><td>{{.Candidate}}</td><td>{{.AvailableSessions}}</td><td>{{.UnavailableSessions}}</td><td>{{.UnknownSessions}}</td><td>{{.AttemptedSessions}}</td><td>{{.ResponseBackedSessions}}</td></tr>{{end}}</table></div>
<h2>Trace and session evidence</h2><p>Pairs run precise → candidate, then candidate → precise. Browser permission, observed requests, and forecast availability are separate observations.</p>
{{range $i,$s:=.Run.Sessions}}<details><summary>{{.Candidate}} · forecast {{.Functionality}} · {{.Status}}</summary><p>Geolocation permission: {{.Geolocation}}</p>{{range .Observations}}<p>Browser → {{categoryLabel .Category}} (<code>{{.Category}}</code>) → {{destinationLabel .Destination}} (<code>{{.Destination}}</code>): <strong>{{.Stage}}</strong> · {{categoryMeaning .Category}} {{destinationMeaning .Destination}}</p>{{else}}<p>No supported location match observed. Absence remains unknown.</p>{{end}}<p>Visibility limits: {{range .Gaps}}<code>{{.}}</code> {{end}}</p><p>Trace identity: <code>{{index $.Run.TraceSHA256 $i}}</code></p><p>Session binding: <code>{{.ChallengeSHA256}}</code></p><p>Browser executable identity: <code>{{.BrowserSHA256}}</code></p></details>{{end}}
<h2>Reproducible evidence</h2><p>Procedure: <code>{{.Run.ProcedureID}}</code></p><p>Receipt identity: <code>{{.ReceiptSHA256}}</code></p><p>Identities establish consistency, not independent authenticity. This test does not locate the destination on a geographic map.</p></details>
</main></body></html>`))

func weatherDestinationLabel(id string) string {
	switch id {
	case "weather-service":
		return "Weather service"
	case "undeclared":
		return "Blocked destination"
	default:
		return trace.DestinationLabel(id)
	}
}

func weatherDestinationMeaning(id string) string {
	switch id {
	case "weather-service":
		return "The reviewed weather-site boundary used by this procedure."
	case "undeclared":
		return "A destination outside the reviewed allowlist; it was blocked by the procedure."
	default:
		return trace.DestinationMeaning(id)
	}
}

func (h handler) handleWeather(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if h.weatherPath == "" {
		http.NotFound(w, r)
		return
	}
	review, err := browser.VerifyWeather(h.weatherPath)
	if err != nil {
		http.Error(w, "weather investigation unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = weatherTemplate.Execute(w, review)
}
