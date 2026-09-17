package browser

import (
	"bytes"
	"errors"
	"html/template"

	"github.com/jackkayser2005/ariadne/internal/bundle"
)

type harComparisonSummaryOrigin struct {
	Label string
	Row   int
}

type harComparisonSummarySide struct {
	Title        string
	Requests     int
	Destinations int
	Origins      []harComparisonSummaryOrigin
	Observed     bool
}

type harComparisonSummaryCategory struct {
	Title string
	Sides []harComparisonSummarySide
}

type harComparisonSummary struct {
	Categories []harComparisonSummaryCategory
	AnyMatches bool
}

func comparisonSummaryFor(comparison HARComparison, rowIndexes map[string]int) harComparisonSummary {
	categories := []struct{ id, title string }{
		{"email", "Email test value"},
		{"phone", "Phone test value"},
		{"account-id", "Account identifier test value"},
	}
	reviews := []HARReview{comparison.Left, comparison.Right}
	titles := []string{"First capture", "Second capture"}
	result := harComparisonSummary{}
	for _, category := range categories {
		item := harComparisonSummaryCategory{Title: category.title}
		for sideIndex, review := range reviews {
			side := harComparisonSummarySide{Title: titles[sideIndex]}
			entries := map[int]struct{}{}
			seenOrigins := map[string]struct{}{}
			for _, destination := range review.Destinations {
				destinationEntries := map[int]struct{}{}
				for _, match := range destination.Matches {
					if match.Category != category.id {
						continue
					}
					for _, entry := range match.Entries {
						entries[entry] = struct{}{}
						destinationEntries[entry] = struct{}{}
					}
				}
				if len(destinationEntries) == 0 {
					continue
				}
				if _, exists := seenOrigins[destination.Label]; exists {
					continue
				}
				seenOrigins[destination.Label] = struct{}{}
				side.Destinations++
				if row, ok := rowIndexes[destination.Label]; ok {
					side.Origins = append(side.Origins, harComparisonSummaryOrigin{Label: destination.Label, Row: row})
				}
			}
			side.Requests = len(entries)
			side.Observed = side.Requests > 0
			if side.Observed {
				result.AnyMatches = true
			}
			item.Sides = append(item.Sides, side)
		}
		if item.Sides[0].Observed || item.Sides[1].Observed {
			result.Categories = append(result.Categories, item)
		}
	}
	return result
}

// CompareHARFiles inventories two current exports using the same supplied rules
// and a shared private origin resolver. It does not establish causality.
func CompareHARFiles(leftPath, rightPath, origin, rulesPath string) (HARComparison, error) {
	if rulesPath == "" {
		return HARComparison{}, errors.New("comparison requires test-value rules")
	}
	rules, err := readHARRules(rulesPath)
	if err != nil {
		return HARComparison{}, err
	}
	resolver, err := newHARLabelResolver(origin)
	if err != nil {
		return HARComparison{}, err
	}
	reviews := make([]HARReview, 2)
	for i, path := range []string{leftPath, rightPath} {
		data, err := bundle.ReadBoundedFile(path, maxHARBytes)
		if err != nil {
			return HARComparison{}, errors.New("cannot read comparison capture: require regular files no larger than 8 MiB")
		}
		reviews[i], err = inspectHARWithResolver(data, rules, resolver)
		if err != nil {
			return HARComparison{}, errors.New("invalid comparison capture")
		}
	}
	return HARComparison{SchemaVersion: 1, MatchingConfigured: true, Left: reviews[0], Right: reviews[1]}, nil
}

// ReadHARComparisonReport rebuilds a value-free comparison page for the local app.
func ReadHARComparisonReport(leftPath, rightPath, origin, rulesPath string) ([]byte, error) {
	return renderHARComparison(leftPath, rightPath, origin, rulesPath, true)
}

// SaveHARComparisonReport writes a new standalone explanation without replacing output.
func SaveHARComparisonReport(leftPath, rightPath, origin, outputPath, rulesPath string) error {
	report, err := ExportHARComparisonReport(leftPath, rightPath, origin, rulesPath)
	if err != nil {
		return err
	}
	return writeExclusive(outputPath, report)
}

// ExportHARComparisonReport rebuilds the standalone redacted pair explanation.
// It includes file-scoped references, not the private source exports or rules.
func ExportHARComparisonReport(leftPath, rightPath, origin, rulesPath string) ([]byte, error) {
	return renderHARComparison(leftPath, rightPath, origin, rulesPath, false)
}

func renderHARComparison(leftPath, rightPath, origin, rulesPath string, local bool) ([]byte, error) {
	comparison, err := CompareHARFiles(leftPath, rightPath, origin, rulesPath)
	if err != nil {
		return nil, err
	}
	type side struct {
		Title  string
		Review HARReview
	}
	type cell struct {
		Title       string
		Destination *HARDestination
	}
	type row struct {
		Index int
		Label string
		Cells []cell
	}
	view := struct {
		Local   bool
		Sides   []side
		Summary harComparisonSummary
		Rows    []row
	}{Local: local, Sides: []side{{"First capture", comparison.Left}, {"Second capture", comparison.Right}}}
	positions := map[string]int{}
	for sideIndex, side := range view.Sides {
		for i := range side.Review.Destinations {
			destination := &side.Review.Destinations[i]
			position, exists := positions[destination.Label]
			if !exists {
				position = len(view.Rows)
				positions[destination.Label] = position
				view.Rows = append(view.Rows, row{Index: position + 1, Label: destination.Label, Cells: []cell{{Title: "First capture"}, {Title: "Second capture"}}})
			}
			view.Rows[position].Cells[sideIndex].Destination = destination
		}
	}
	rowIndexes := map[string]int{}
	for _, row := range view.Rows {
		rowIndexes[row.Label] = row.Index
	}
	view.Summary = comparisonSummaryFor(comparison, rowIndexes)
	var out bytes.Buffer
	if err := harComparisonTemplate.Execute(&out, view); err != nil {
		return nil, errors.New("cannot render comparison")
	}
	return out.Bytes(), nil
}

var harComparisonTemplate = template.Must(template.New("har-comparison").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>Compare browser captures · Ariadne</title><style>body{font:17px/1.6 system-ui,sans-serif;background:#f3f8f3;color:#13291f;margin:0}main{max-width:1150px;margin:auto;padding:32px 20px}h1{font-size:38px;line-height:1.2}h2{font-size:24px}h3{font-size:19px}h4{font-size:17px}.sides{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}section,details.origin-row{background:white;border:1px solid #d5e2d9;border-radius:16px;padding:24px;margin:20px 0}article{border-top:1px solid #d5e2d9;padding:12px 0}summary{cursor:pointer;font-weight:bold}summary:focus-visible,a:focus-visible{outline:3px solid #0f4c32;outline-offset:3px}li{margin:8px 0}.note{color:#5e7167}.summary-side{border-top:0;padding:0}.empty{background:#fff8e8;border:1px solid #d8c89d;border-radius:12px;padding:16px}a{color:#176341}@media(max-width:760px){.sides{grid-template-columns:1fr}}</style></head><body><main>{{if .Local}}<a href="/" aria-label="Back to Ariadne investigations">Ariadne / investigations</a> · <a href="/capture-comparison-report" download>Save comparison</a>{{end}}<h1>What appears in each capture?</h1><p>Each file is an exported list of browser requests. The summary shows where the supplied test information appeared in the checked parts of each file. A destination is a web address listed in a request. Names are hidden in this report.</p><p class="note">These are editable browser exports. Differences do not prove that a privacy setting caused a change, that a website still worked, or that information reached a server.</p><section><h2>Where did the supplied information appear?</h2><div class="sides">{{range .Sides}}<article class="summary-side"><h3>{{.Title}}</h3><p>{{.Review.Requests}} exported request(s) across {{len .Review.Destinations}} observed destination(s).</p><p class="note"><strong>Request contents we could check:</strong> {{.Review.InspectedBodies}} request body/bodies checked; {{.Review.UninspectedBodies}} request bodies unavailable or unsupported. A body is content attached to a request.</p>{{if eq .Review.Requests 0}}<p>This file contains no exported requests. It does not establish that no activity occurred.</p>{{end}}</article>{{end}}</div>{{if .Summary.AnyMatches}}{{range .Summary.Categories}}<article><h3>{{.Title}}</h3><div class="sides">{{range .Sides}}<div><h4>{{.Title}}</h4>{{if .Observed}}<p><strong>{{.Requests}} distinct matching request(s)</strong> across <strong>{{.Destinations}} matched destination(s)</strong>.</p><ul>{{range .Origins}}<li><a href="#origin-row-{{.Row}}">{{.Label}}</a></li>{{end}}</ul>{{else}}<p>This category was <strong>not observed in the checked parts of this file</strong>. This is inconclusive; an omitted request or unsupported channel could contain it.</p>{{end}}</div>{{end}}</div></article>{{end}}{{else}}<div class="empty"><h3>No exact supplied test category matched in either file</h3><p>No supplied test category matched in the checked parts of either file. This is inconclusive: exported files can omit requests or channels, so this does not show that information stayed private.</p><p>Field-name clues appear only in the collapsed origin details and do not count as matches.</p></div>{{end}}<details><summary>How this summary is determined</summary><p>Only exact supplied test values in supported URL path segment, URL query, exported form parameter, JSON string, form-body, request header, or exported request cookie channels count. A request that matches in several channels counts once for that category. Labels use the same normalized web origin within this pair, even when requests are reordered. Field-name clues do not create a match.</p></details></section>
{{range .Rows}}<details class="origin-row" id="origin-row-{{.Index}}"><summary>Origin details: {{.Label}}</summary><h2>{{.Label}}</h2><div class="sides">{{range .Cells}}<article><h3>{{.Title}}</h3>{{with .Destination}}<p>{{.Requests}} request(s); {{.CookieRequests}} with cookie metadata.</p>{{if .Matches}}<p><strong>Supplied values found in this file</strong></p><ul>{{range .Matches}}<li>{{.Category}} · {{.Channel}} · {{.Requests}} request(s)<details><summary>Supporting entries in this capture</summary><p>{{range .Entries}}entry {{.}}; {{end}}</p></details></li>{{end}}</ul>{{else}}<p>No supplied value was observed in this origin's supported channels. Missing visibility may explain the absence.</p>{{end}}{{if .Hints}}<details><summary>Weaker field-name clues</summary><ul>{{range .Hints}}<li>{{.}}</li>{{end}}</ul><p>These names do not establish that the corresponding personal value was present.</p></details>{{end}}<p class="note">Body gaps here: {{.UninspectedBodies}}.</p>{{else}}<p>This origin was not observed in this capture. The export may be incomplete.</p>{{end}}</article>{{end}}</div></details>{{end}}
<section><h2>What remains unknown?</h2><p>An origin absent from one side was not observed in that file. It may have been omitted by the export. Labels identify normalized web origins only within this pair; the same label means the same normalized origin even if requests appear in a different order. Labels do not identify companies or onward sharing. Entry numbers start at 1 separately in each file and change if that file is reordered.</p><p>Supported matches cover complete decoded URL path segments, URL query values, exported form parameters, JSON strings, form-body values, request header values, and exported request cookie values. A cookie or header match does not identify a person or explain the purpose of a cookie. Unsupported encodings, other channels, missing metadata and server handling remain unknown. Counts can overlap across categories and channels.</p><p>Raw domains, values and file paths are omitted. This comparison cannot select a minimum amount of information to share; that requires a controlled functionality experiment.</p></section></main></body></html>`))
