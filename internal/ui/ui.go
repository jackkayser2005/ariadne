// Package ui serves the read-only Ariadne evidence review surface.
package ui

import (
	"html/template"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

type handler struct {
	root      string
	index     func(string) ([]bundle.ArchiveEntry, error)
	verify    func(string) (bundle.Summary, error)
	questions func() []bundle.Question
	ask       func(string, string) (bundle.Answer, error)
	find      func(string, string) (bundle.Finding, error)
}

type archiveQuestionResult struct {
	Directory    string
	ManifestName string
	Summary      bundle.Summary
	Answer       bundle.Answer
	Available    bool
}

type pageData struct {
	View             string
	Title            string
	Directory        string
	Entries          []bundle.ArchiveEntry
	Questions        []bundle.Question
	SelectedQuestion bundle.Question
	ArchiveAnswers   []archiveQuestionResult
	Summary          bundle.Summary
	Answers          []bundle.Answer
	Answer           bundle.Answer
	Finding          bundle.Finding
}

// Handler returns a read-only HTTP handler for one explicitly supplied archive root.
func Handler(archiveRoot string) http.Handler {
	return newHandler(handler{
		root:      archiveRoot,
		index:     bundle.Index,
		verify:    bundle.Verify,
		questions: bundle.Questions,
		ask:       bundle.Ask,
		find:      bundle.Find,
	})
}

func newHandler(h handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/run", h.handleRun)
	mux.HandleFunc("/ask", h.handleAsk)
	mux.HandleFunc("/finding", h.handleFinding)
	mux.HandleFunc("/favicon.ico", handleFavicon)
	return mux
}

func handleFavicon(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	entries, err := h.index(h.root)
	if err != nil {
		http.Error(w, "archive unavailable", http.StatusInternalServerError)
		return
	}
	var questions []bundle.Question
	if h.questions != nil {
		questions = h.questions()
	}
	selectedQuestion, questionID := bundle.Question{}, r.URL.Query().Get("question_id")
	var archiveAnswers []archiveQuestionResult
	if questionID != "" {
		var ok bool
		selectedQuestion, ok = questionForID(questions, questionID)
		if !ok {
			http.Error(w, "question not found", http.StatusNotFound)
			return
		}
		archiveAnswers = make([]archiveQuestionResult, 0, len(entries))
		for _, entry := range entries {
			runDir := filepath.Join(h.root, entry.Directory)
			summary, verifyErr := h.verify(runDir)
			answer := bundle.Answer{}
			askErr := verifyErr
			if verifyErr == nil {
				answer, askErr = h.ask(runDir, questionID)
			}
			archiveAnswers = append(archiveAnswers, archiveQuestionResult{
				Directory:    entry.Directory,
				ManifestName: entry.ManifestName,
				Summary:      summary,
				Answer:       answer,
				Available:    askErr == nil,
			})
		}
	}
	render(w, pageData{
		View:             "index",
		Title:            "Ariadne — evidence review",
		Entries:          entries,
		Questions:        questions,
		SelectedQuestion: selectedQuestion,
		ArchiveAnswers:   archiveAnswers,
	})
}

func (h handler) handleRun(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	runDir, ok := h.bundlePath(directory)
	if !ok {
		http.Error(w, "bundle not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	questions := h.questions()
	answers := make([]bundle.Answer, 0, len(questions))
	for _, question := range questions {
		answer, err := h.ask(runDir, question.ID)
		if err != nil {
			answers = nil
			break
		}
		answers = append(answers, answer)
	}
	render(w, pageData{
		View:      "run",
		Title:     "Bundle review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Answers:   answers,
	})
}

func (h handler) handleAsk(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	questionID := r.URL.Query().Get("question_id")
	runDir, ok := h.bundlePath(directory)
	if !ok || questionID == "" {
		http.Error(w, "question not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	answer, err := h.ask(runDir, questionID)
	if err != nil {
		http.Error(w, "question unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:      "ask",
		Title:     "Question review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Answer:    answer,
	})
}

func (h handler) handleFinding(w http.ResponseWriter, r *http.Request) {
	if !getOnly(w, r) {
		return
	}
	directory := r.URL.Query().Get("directory")
	findingID := r.URL.Query().Get("finding_id")
	runDir, ok := h.bundlePath(directory)
	if !ok || findingID == "" {
		http.Error(w, "finding not found", http.StatusNotFound)
		return
	}
	summary, err := h.verify(runDir)
	if err != nil {
		http.Error(w, "bundle is no longer verifiable", http.StatusUnprocessableEntity)
		return
	}
	finding, err := h.find(runDir, findingID)
	if err != nil {
		http.Error(w, "finding unavailable", http.StatusUnprocessableEntity)
		return
	}
	render(w, pageData{
		View:      "finding",
		Title:     "Finding review — Ariadne",
		Directory: directory,
		Summary:   summary,
		Finding:   finding,
	})
}

func (h handler) bundlePath(directory string) (string, bool) {
	if directory == "" {
		return "", false
	}
	entries, err := h.index(h.root)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.Directory == directory {
			return filepath.Join(h.root, entry.Directory), true
		}
	}
	return "", false
}

func questionForID(questions []bundle.Question, id string) (bundle.Question, bool) {
	for _, question := range questions {
		if question.ID == id {
			return question, true
		}
	}
	return bundle.Question{}, false
}

func getOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func render(w http.ResponseWriter, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplate.Execute(w, data); err != nil {
		http.Error(w, "page unavailable", http.StatusInternalServerError)
	}
}

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"query": func(value string) template.URL { return template.URL(url.QueryEscape(value)) },
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root { color-scheme: light; --ink: #16202a; --muted: #64717d; --line: #d8e0e5; --paper: #f7f9fa; --card: #fff; --accent: #0b6e69; --accent-soft: #e1f2ef; --warning: #8a5a00; --warning-soft: #fff3d5; }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--paper); color: var(--ink); font: 16px/1.55 system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { width: min(980px, calc(100% - 32px)); margin: 0 auto; padding: 28px 0 56px; }
    header { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; padding-bottom: 56px; }
    .brand { color: var(--ink); font-size: 14px; font-weight: 800; letter-spacing: .16em; text-decoration: none; }
    .context, .eyebrow, .directory, .metric-label, footer { color: var(--muted); font-size: 13px; }
    .hero { max-width: 680px; padding-bottom: 42px; }
    h1 { max-width: 760px; margin: 0 0 12px; font-size: clamp(34px, 6vw, 58px); letter-spacing: -.05em; line-height: 1.02; }
    h2 { margin: 0; font-size: 22px; letter-spacing: -.02em; }
    h3 { margin: 4px 0 2px; font-size: 20px; }
    p { margin: 0 0 18px; }
    .lede { color: var(--muted); font-size: 19px; }
    .section-head { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; margin: 0 0 14px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: 14px; }
    .card, .panel { border: 1px solid var(--line); border-radius: 16px; background: var(--card); padding: 22px; box-shadow: 0 8px 24px rgba(22,32,42,.04); }
    .card { display: flex; flex-direction: column; min-height: 230px; }
    .card .button { margin-top: auto; }
    .directory { word-break: break-word; }
    .metrics { display: flex; gap: 22px; margin: 22px 0; }
    .metric { display: grid; gap: 2px; }
    .metric-value { font-size: 27px; font-weight: 750; line-height: 1; }
    .button { display: inline-flex; align-items: center; justify-content: space-between; gap: 12px; border: 1px solid var(--accent); border-radius: 10px; color: var(--accent); background: transparent; padding: 10px 13px; font-weight: 700; text-decoration: none; }
    .button:hover { color: #fff; background: var(--accent); }
    .question-links { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 18px; }
    .question-list { display: grid; gap: 10px; margin-top: 18px; }
    .back { display: inline-block; margin-bottom: 26px; color: var(--accent); font-weight: 700; text-decoration: none; }
    .status { display: inline-block; border-radius: 999px; background: var(--accent-soft); color: var(--accent); padding: 5px 11px; font-size: 13px; font-weight: 800; text-transform: uppercase; letter-spacing: .08em; }
    .status-unknown { background: var(--warning-soft); color: var(--warning); }
    .status-unavailable { background: var(--line); color: var(--muted); }
    .question { max-width: 700px; margin: 20px 0 28px; font-size: 25px; letter-spacing: -.02em; }
    dl { display: grid; grid-template-columns: 150px 1fr; gap: 10px 18px; margin: 20px 0 30px; }
    dt { color: var(--muted); font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: .08em; }
    dd { margin: 0; overflow-wrap: anywhere; }
    ul { margin: 12px 0 26px; padding-left: 22px; }
    li { margin: 6px 0; overflow-wrap: anywhere; }
    a.finding { color: var(--accent); font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 13px; }
    .empty { color: var(--muted); border: 1px dashed var(--line); border-radius: 14px; padding: 24px; }
    footer { border-top: 1px solid var(--line); margin-top: 52px; padding-top: 16px; }
    @media (max-width: 560px) { header { display: block; padding-bottom: 38px; } .context { display: block; margin-top: 8px; } dl { grid-template-columns: 1fr; gap: 3px; } dd { margin-bottom: 10px; } }
  </style>
</head>
<body>
<main>
  <header>
    <a class="brand" href="/">ARIADNE</a>
    <span class="context">counterfactual evidence review · read only</span>
  </header>

  {{define "provenance"}}
  {{if or .Summary.Question .Summary.AnswerState .Summary.ManifestContractSHA256 .Summary.AriadneRevision .Summary.RecordedAt}}
  <section class="panel">
    <h2>Verified provenance</h2>
    <dl>
      {{with .Summary.Question}}<dt>question</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.AnswerState}}<dt>answer state</dt><dd><span class="status status-{{.}}">{{.}}</span></dd>{{end}}
      {{with .Summary.ManifestContractSHA256}}<dt>manifest contract</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.RecordedAt}}<dt>recorded (UTC)</dt><dd>{{.}}</dd>{{end}}
      {{with .Summary.AriadneRevision}}<dt>Ariadne revision</dt><dd>{{.}}</dd><dt>working tree</dt><dd>{{if $.Summary.AriadneModified}}modified{{else}}clean{{end}}</dd>{{end}}
    </dl>
    <p class="context">This context is structural metadata only; observations and persona values are not rendered.</p>
  </section>
  {{end}}
  {{end}}

  {{if eq .View "index"}}
    <section class="hero">
      <p class="eyebrow">verified archive</p>
      <h1>Review what changed.</h1>
      <p class="lede">Start from a verified bundle, ask one bounded question, and follow its safe finding references. Captured values never appear here.</p>
    </section>
    <section class="panel">
      <div class="section-head"><h2>Ask across this archive</h2><span class="context">fixed, read only</span></div>
      <p class="context">Choose one bounded question to re-check against every verified bundle.</p>
      <div class="question-links" aria-label="Bounded questions">
      {{range .Questions}}<a class="button" href="/?question_id={{query .ID}}">{{.Text}}</a>{{end}}
      </div>
    </section>
    {{if .SelectedQuestion.ID}}
    <section>
      <div class="section-head"><h2>Question lens</h2><span class="context">re-verified now</span></div>
      <p class="question">{{.SelectedQuestion.Text}}</p>
      <div class="grid">
      {{range .ArchiveAnswers}}
        <article class="card">
          <p class="eyebrow">bundle</p>
          <h3>{{.ManifestName}}</h3>
          <p class="directory">{{.Directory}}</p>
          {{if .Available}}
            <span class="status status-{{.Answer.State}}">{{.Answer.State}}</span>
            {{with .Answer.Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
            {{template "provenance" .}}
            <a class="button" href="/ask?directory={{query .Directory}}&amp;question_id={{query $.SelectedQuestion.ID}}">Open answer details <span aria-hidden="true">→</span></a>
          {{else}}
            <span class="status status-unavailable">unavailable</span>
            <p class="context">This bundle does not support the current bounded question.</p>
          {{end}}
        </article>
      {{end}}
      </div>
    </section>
    {{end}}
    <section>
      <div class="section-head"><h2>Archived bundles</h2><span class="context">{{len .Entries}} available</span></div>
      {{if .Entries}}
        <div class="grid">
        {{range .Entries}}
          <article class="card">
            <p class="eyebrow">manifest</p>
            <h3>{{.ManifestName}}</h3>
            <p class="directory">{{.Directory}}</p>
            <div class="metrics">
              <span class="metric"><strong class="metric-value">{{.Differences}}</strong><span class="metric-label">differences</span></span>
              <span class="metric"><strong class="metric-value">{{.Unknowns}}</strong><span class="metric-label">unknowns</span></span>
            </div>
            <a class="button" href="/run?directory={{query .Directory}}">Open review <span aria-hidden="true">→</span></a>
          </article>
        {{end}}
        </div>
      {{else}}
        <p class="empty">No verified bundles are available in this archive root.</p>
      {{end}}

  {{else if eq .View "run"}}
    <a class="back" href="/">← All bundles</a>
    <p class="eyebrow">{{.Summary.ManifestName}} · verified</p>
    <h1>{{.Directory}}</h1>
    <div class="metrics panel">
      <span class="metric"><strong class="metric-value">{{.Summary.Differences}}</strong><span class="metric-label">differences</span></span>
      <span class="metric"><strong class="metric-value">{{.Summary.Unknowns}}</strong><span class="metric-label">unknowns</span></span>
    </div>
    {{template "provenance" .}}
    {{if .Answers}}
    <section>
      <div class="section-head"><h2>Bounded question board</h2><span class="context">re-verified now</span></div>
      <div class="question-list">
      {{range .Answers}}
        <article class="panel question-card">
          <div class="section-head"><h3>{{.Question}}</h3><span class="status status-{{.State}}">{{.State}}</span></div>
          {{with .Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
          {{if .FindingIDs}}
          <p class="context">Referenced findings</p>
          <ul>
          {{range .FindingIDs}}<li><a class="finding" href="/finding?directory={{query $.Directory}}&amp;finding_id={{query .}}">{{.}}</a></li>{{end}}
          </ul>
          {{else}}<p class="context">No finding references were produced for this question.</p>{{end}}
          <a class="button" href="/ask?directory={{query $.Directory}}&amp;question_id={{query .QuestionID}}">Open answer details <span aria-hidden="true">→</span></a>
        </article>
      {{end}}
      </div>
    </section>
    {{else}}
    <section class="panel">
      <h2>Bounded questions unavailable</h2>
      <p class="context">This verified bundle does not support the current question catalog. Its safe summary remains available.</p>
    </section>
    {{end}}

  {{else if eq .View "ask"}}
    <a class="back" href="/run?directory={{query .Directory}}">← Bundle review</a>
    <p class="eyebrow">bounded question · {{.Directory}}</p>
    <h1>Question result</h1>
    <p class="question">{{.Answer.Question}}</p>
    <span class="status status-{{.Answer.State}}">{{.Answer.State}}</span>
    {{with .Answer.Reason}}<p class="context">Why unknown: {{.}}</p>{{end}}
    {{template "provenance" .}}
    <section class="panel" style="margin-top: 28px">
      <h2>Referenced findings</h2>
      {{if .Answer.FindingIDs}}
        <ul>
        {{range .Answer.FindingIDs}}
          <li><a class="finding" href="/finding?directory={{query $.Directory}}&amp;finding_id={{query .}}">{{.}}</a></li>
        {{end}}
        </ul>
      {{else}}
        <p class="context">No finding references were produced for this question.</p>
      {{end}}
    </section>

  {{else if eq .View "finding"}}
    <a class="back" href="/run?directory={{query .Directory}}">← Bundle review</a>
    <p class="eyebrow">{{.Finding.Kind}} · {{.Finding.State}} · {{.Directory}}</p>
    <h1>Finding detail</h1>
    <p class="lede">This is a verified, raw-value-free finding reference.</p>
    {{template "provenance" .}}
    <section class="panel">
      <dl>
        <dt>id</dt><dd>{{.Finding.ID}}</dd>
        <dt>field</dt><dd>{{.Finding.Field}}</dd>
        {{with .Finding.Classification}}<dt>classification</dt><dd>{{.}}</dd>{{end}}
        <dt>answer state</dt><dd>{{.Finding.AnswerState}}</dd>
        <dt>finding state</dt><dd>{{.Finding.State}}</dd>
        {{with .Finding.Reason}}<dt>reason</dt><dd>{{.}}</dd>{{end}}
      </dl>
      <h2>Evidence sources</h2>
      <ul>
      {{range .Finding.Evidence}}<li>{{.}}</li>{{end}}
      </ul>
      <p class="context">Observed values and captured payloads are intentionally not rendered.</p>
    </section>
  {{end}}

  <footer>Read-only review surface. Raw observations are never rendered.</footer>
</main>
</body>
</html>`))
