package ui

import (
	"html/template"
	"net/http"

	"github.com/jackkayser2005/ariadne/internal/trace"
)

var guideTemplate = template.Must(template.New("guide").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>How Ariadne works · Ariadne</title>
  <style>
    :root { color-scheme: light; --ink: #13291f; --muted: #5e7167; --line: #d5e2d9; --paper: #f3f8f3; --card: #fffefa; --accent: #176341; --accent-strong: #0f4c32; --accent-soft: #dff1e5; --warning: #8a5a00; --warning-soft: #fff3d5; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; background: linear-gradient(180deg, #e8f3ea 0, var(--paper) 360px); color: var(--ink); font: 17px/1.6 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { width: min(980px, calc(100% - 32px)); margin: 0 auto; padding: 28px 0 64px; }
    nav { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 56px; }
    nav a { color: var(--accent-strong); font-weight: 750; text-decoration: none; }
    nav a:focus-visible, a:focus-visible, summary:focus-visible { outline: 3px solid #9ad6aa; outline-offset: 4px; }
    .brand { color: var(--ink); font-size: 14px; font-weight: 850; letter-spacing: .16em; }
    .eyebrow { color: var(--muted); font-size: 13px; font-weight: 800; letter-spacing: .12em; text-transform: uppercase; }
    h1 { max-width: 760px; margin: 12px 0 16px; font-size: clamp(36px, 7vw, 64px); letter-spacing: -.055em; line-height: 1.02; }
    h2 { margin: 0 0 12px; font-size: 26px; letter-spacing: -.025em; }
    h3 { margin: 0 0 6px; font-size: 19px; }
    .lede { max-width: 720px; color: var(--muted); font-size: 20px; }
    .panel { margin-top: 24px; border: 1px solid var(--line); border-radius: 18px; background: var(--card); padding: 26px; box-shadow: 0 10px 28px rgba(23,99,65,.08); }
    .answer { border-left: 5px solid var(--accent); }
    .flow { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin: 22px 0 4px; padding: 0; list-style: none; }
    .flow li { min-height: 126px; border-radius: 14px; background: var(--accent-soft); padding: 16px; }
    .flow strong, .flow span { display: block; }
    .flow strong { margin-bottom: 5px; }
    .flow span, .muted { color: var(--muted); font-size: 14px; }
    .cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
    .card { border: 1px solid var(--line); border-radius: 14px; padding: 18px; }
    .card strong { color: var(--accent-strong); }
    .unknown { background: var(--warning-soft); border-color: #e1d3ac; }
    .unknown strong { color: var(--warning); }
    .links { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 18px; }
    .button { display: inline-flex; align-items: center; gap: 10px; border: 1px solid var(--accent); border-radius: 10px; background: var(--accent); color: #fff; padding: 10px 14px; font-weight: 750; text-decoration: none; }
    .button:hover { background: var(--accent-strong); border-color: var(--accent-strong); }
    details { margin-top: 18px; border-top: 1px solid var(--line); padding-top: 16px; }
    summary { cursor: pointer; font-weight: 750; }
    footer { margin-top: 44px; border-top: 1px solid var(--line); padding-top: 16px; color: var(--muted); font-size: 14px; }
    @media (max-width: 760px) { .flow, .cards { grid-template-columns: 1fr 1fr; } }
    @media (max-width: 540px) { main { width: min(100% - 24px, 980px); } nav { display: block; margin-bottom: 38px; } nav a:last-child { display: block; margin-top: 10px; } .flow, .cards { grid-template-columns: 1fr; } .panel { padding: 20px; } }
  </style>
</head>
<body>
<main>
  <nav aria-label="Ariadne navigation"><a class="brand" href="/">ARIADNE</a><a href="/">Back to investigations</a></nav>
  <header>
    <p class="eyebrow">A plain-language guide</p>
    <h1>See the information trail without reading the raw data.</h1>
    <p class="lede">Ariadne is a careful way to ask what changed when one declared piece of information changed. It keeps the explanation readable and the saved evidence checkable.</p>
  </header>

  <section class="panel answer" aria-labelledby="flow-title">
    <p class="eyebrow">The Ariadne workflow</p>
    <h2 id="flow-title">Four steps turn a capture into an explanation.</h2>
    <ol class="flow">
      <li><strong>1 · Investigate</strong><span>Run one reviewed interaction or open a saved capture.</span></li>
      <li><strong>2 · Compare</strong><span>Change one declared value and check what differs.</span></li>
      <li><strong>3 · Trace</strong><span>Follow a supported category to a recorded destination.</span></li>
      <li><strong>4 · Verify</strong><span>Recheck the identities before relying on the result.</span></li>
    </ol>
  </section>

  <section class="panel" aria-labelledby="read-title">
    <p class="eyebrow">Reading a result</p>
    <h2 id="read-title">The labels are deliberately modest.</h2>
    <div class="cards">
      <article class="card"><h3><strong>Observed</strong></h3><p>Supported evidence appeared in the checked channel. This says what Ariadne saw, not what happened later on a server.</p></article>
      <article class="card unknown"><h3><strong>Unknown</strong></h3><p>The capture was incomplete, unsupported, or blocked. Unknown means the evidence cannot answer the question yet.</p></article>
      <article class="card"><h3><strong>Verified</strong></h3><p>The saved files agree with their recorded identities. Verification protects the bundle from drift; it does not make a source truthful.</p></article>
    </div>
  </section>

  <section class="panel" aria-labelledby="categories-title">
    <p class="eyebrow">Safe categories</p>
    <h2 id="categories-title">A category is a label, not the value.</h2>
    <p>Ariadne groups raw information into a small reviewed vocabulary before it reaches a report. The actual coordinates, IDs, addresses, and messages stay out of this guide.</p>
    <div class="cards">
      <article class="card"><h3><strong>Location</strong></h3><p>Coordinates or a broader region label.</p></article>
      <article class="card"><h3><strong>Account or device</strong></h3><p>An identifier that can connect activity to an account or device.</p></article>
      <article class="card"><h3><strong>Contact or session</strong></h3><p>Email, phone, cookie, or session labels used to group activity.</p></article>
    </div>
  </section>

  <section class="panel" aria-labelledby="catalog-title">
    <p class="eyebrow">Reviewed vocabulary</p>
    <h2 id="catalog-title">The labels have a small, fixed meaning.</h2>
    <p>These are the category labels Ariadne can safely carry into a report. The code is kept beside each plain-language explanation so a technical reader can match it to the evidence. Destination labels such as analytics and advertising describe reviewed boundaries; they do not identify an organization.</p>
    <details>
      <summary>See all reviewed category labels</summary>
      <div class="cards">
      {{range .Categories}}
        <article class="card"><h3><strong>{{.Label}}</strong></h3><p><code>{{.ID}}</code></p><p>{{.Description}}</p></article>
      {{end}}
      </div>
    </details>
  </section>

  <section class="panel" aria-labelledby="start-title">
    <p class="eyebrow">Start with a saved investigation</p>
    <h2 id="start-title">Choose the question you want answered.</h2>
    {{if .WeatherAvailable}}<p>For the current weather example, Ariadne compares precise, city-level, and denied location access. It reports the observed request path separately from whether a local forecast still worked.</p>{{else}}<p>Open a saved investigation from the evidence configured for this review.</p>{{end}}
    <div class="links">{{if .WeatherAvailable}}<a class="button" href="/weather">Open weather investigation <span aria-hidden="true">→</span></a>{{end}}<a class="button" href="/">Browse all saved evidence <span aria-hidden="true">→</span></a></div>
    <details><summary>What Ariadne does not claim</summary><p class="muted">A local review page cannot see a company's private server logs, unsupported browser channels, or activity that was never captured. It does not monitor the rest of your device, identify an organization from a domain label, or turn a changed result into a causal proof.</p></details>
  </section>

  <footer>Raw captured values stay outside this guide. Open the technical details on an investigation page only when you need the verifier-derived identities behind its plain-language answer.</footer>
</main>
</body>
</html>`))

func (h handler) handleGuide(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/guide" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Categories       []trace.CategoryDefinition
		WeatherAvailable bool
	}{trace.CategoryDefinitions(), h.weatherPath != ""}
	if err := guideTemplate.Execute(w, data); err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
	}
}
