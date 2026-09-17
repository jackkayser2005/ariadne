package browser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackkayser2005/ariadne/internal/bundle"
	"github.com/jackkayser2005/ariadne/internal/jsoncheck"
)

const (
	maxHARBytes         = 8 << 20
	maxHARPathSegments  = 1024
	harValidationOrigin = "https://example.invalid"
)

type harName struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type harEntry struct {
	Request *struct {
		URL      string       `json:"url"`
		Headers  []harName    `json:"headers"`
		Cookies  []harName    `json:"cookies"`
		PostData *harPostData `json:"postData"`
		BodySize *int64       `json:"bodySize"`
	} `json:"request"`
}

// HARDestination groups exported requests by exact origin, without retaining it.
// Labels identify groups only within this report, not organizations.
type HARDestination struct {
	InspectedBodies   int        `json:"inspected_textual_bodies"`
	UninspectedBodies int        `json:"uninspected_body_entries"`
	Matches           []HARMatch `json:"matches"`
	Label             string     `json:"label"`
	Requests          int        `json:"requests"`
	Hints             []string   `json:"field_name_hints"`
	CookieRequests    int        `json:"requests_with_cookie_metadata"`
}

// HARReview is an untrusted export's redacted inventory, not verified disclosure evidence.
type HARReview struct {
	MatchingConfigured bool             `json:"matching_configured"`
	SchemaVersion      int              `json:"schema_version"`
	Requests           int              `json:"requests"`
	InspectedBodies    int              `json:"inspected_textual_bodies"`
	UninspectedBodies  int              `json:"uninspected_body_entries"`
	Destinations       []HARDestination `json:"destinations"`
}

// HARComparison is a bounded, redacted comparison of two supplied HAR files.
// The left and right entry references are scoped to their respective files.
// It is an inventory of observed supported channels, not a privacy, causal, or
// functionality conclusion.
type HARComparison struct {
	MatchingConfigured bool      `json:"matching_configured"`
	SchemaVersion      int       `json:"schema_version"`
	Left               HARReview `json:"left"`
	Right              HARReview `json:"right"`
}

// HARVerificationSummary identifies a structurally valid HAR export without
// retaining its URLs, fields, payloads, or values.
type HARVerificationSummary struct {
	SchemaVersion int    `json:"schema_version"`
	Requests      int    `json:"requests"`
	Destinations  int    `json:"destinations"`
	HARFileSHA256 string `json:"har_file_sha256"`
}
type harLabelResolver struct {
	base      string
	labels    map[string]string
	nextOther int
}

func newHARLabelResolver(origin string) (*harLabelResolver, error) {
	base, err := checkedHAROrigin(origin)
	if err != nil {
		return nil, errors.New("origin must be an absolute HTTP or HTTPS origin without a path, query, or credentials")
	}
	return &harLabelResolver{base: base, labels: map[string]string{}}, nil
}

func (resolver *harLabelResolver) label(origin string) string {
	if label, ok := resolver.labels[origin]; ok {
		return label
	}
	label := "Website origin"
	if origin != resolver.base {
		resolver.nextOther++
		label = fmt.Sprintf("Other origin %d", resolver.nextOther)
	}
	resolver.labels[origin] = label
	return label
}

// InspectHAR summarizes the supported HAR 1.2 request subset locally. It never
// returns URLs, field names, payloads, or values. Hints do not establish disclosure.
func InspectHAR(data []byte, origin string) (HARReview, error) {
	return inspectHAR(data, origin, nil)
}

// VerifyHAR verifies one bounded local HAR export and returns only its
// structural counts and file identity. It does not authenticate the export
// or claim that any request reached a server.
func VerifyHAR(path string) (HARVerificationSummary, error) {
	if strings.TrimSpace(path) == "" {
		return HARVerificationSummary{}, errors.New("HAR path is required")
	}
	data, err := bundle.ReadBoundedFile(path, maxHARBytes)
	if err != nil {
		return HARVerificationSummary{}, errors.New("read HAR")
	}
	review, err := inspectHAR(data, harValidationOrigin, nil)
	if err != nil {
		return HARVerificationSummary{}, errors.New("invalid HAR")
	}
	digest := sha256.Sum256(data)
	return HARVerificationSummary{
		SchemaVersion: review.SchemaVersion,
		Requests:      review.Requests,
		Destinations:  len(review.Destinations),
		HARFileSHA256: hex.EncodeToString(digest[:]),
	}, nil
}
func inspectHAR(data []byte, origin string, rules []harRule) (HARReview, error) {
	resolver, err := newHARLabelResolver(origin)
	if err != nil {
		return HARReview{}, err
	}
	return inspectHARWithResolver(data, rules, resolver)
}

func inspectHARWithResolver(data []byte, rules []harRule, resolver *harLabelResolver) (HARReview, error) {
	fail := func() (HARReview, error) { return HARReview{}, errors.New("unsupported or invalid HAR input") }
	if len(data) > maxHARBytes || !utf8.Valid(data) {
		return fail()
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	// Bound recursion before duplicate-key validation. JSON validity is checked below.
	depth := 0
	quoted := false
	escaped := false
	for _, c := range data {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > 64 {
				return fail()
			}
		case '}', ']':
			depth--
		}
	}
	if !json.Valid(data) || jsoncheck.RejectDuplicateKeys(data) != nil {
		return fail()
	}
	var file struct {
		Log *struct {
			Version string     `json:"version"`
			Entries []harEntry `json:"entries"`
		} `json:"log"`
	}
	if json.Unmarshal(data, &file) != nil || file.Log == nil || file.Log.Version != "1.2" || file.Log.Entries == nil || len(file.Log.Entries) > 10000 {
		return fail()
	}
	result := HARReview{MatchingConfigured: len(rules) > 0, SchemaVersion: 4, Requests: len(file.Log.Entries), Destinations: []HARDestination{}}
	groups := map[string]int{}
	hints := map[int]map[string]bool{}
	for entryIndex, entry := range file.Log.Entries {
		r := entry.Request
		if r == nil || len(r.Headers) > 1024 || len(r.Cookies) > 1024 || (r.PostData != nil && len(r.PostData.Params) > 1024) {
			return fail()
		}
		destination, err := harOrigin(r.URL)
		if err != nil {
			return fail()
		}
		parsed, _ := url.Parse(r.URL)
		query, err := url.ParseQuery(parsed.RawQuery)
		if err != nil || len(query) > 1024 {
			return fail()
		}
		pathSegments, err := harPathSegments(parsed, len(rules) > 0)
		if err != nil {
			return fail()
		}
		index, exists := groups[destination]
		if !exists {
			index = len(result.Destinations)
			groups[destination] = index
			result.Destinations = append(result.Destinations, HARDestination{Label: resolver.label(destination), Hints: []string{}})
			hints[index] = map[string]bool{}
		}
		d := &result.Destinations[index]
		d.Requests++
		var params []harName
		if r.PostData != nil {
			params = r.PostData.Params
		}
		body := harBody{}
		if len(rules) > 0 {
			body = inspectHARBody(r.PostData, r.BodySize)
			if body.inspected {
				d.InspectedBodies++
				result.InspectedBodies++
			}
			if body.gap {
				d.UninspectedBodies++
				result.UninspectedBodies++
			}
		}
		addHARMatchesWithRequestMetadata(d, query, pathSegments, params, r.Headers, r.Cookies, rules, entryIndex+1, body)
		for name := range query {
			if hint := harHint(name); hint != "" {
				hints[index][hint] = true
			}
		}
		cookie := len(r.Cookies) > 0
		for _, header := range r.Headers {
			if strings.EqualFold(header.Name, "Cookie") {
				cookie = true
			}
		}
		if cookie {
			d.CookieRequests++
		}
		if r.PostData != nil {
			for _, param := range r.PostData.Params {
				if hint := harHint(param.Name); hint != "" {
					hints[index][hint] = true
				}
			}
		}
	}
	for i := range result.Destinations {
		for hint := range hints[i] {
			result.Destinations[i].Hints = append(result.Destinations[i].Hints, hint)
		}
		sort.Strings(result.Destinations[i].Hints)
	}
	return result, nil
}

func harPathSegments(parsed *url.URL, decode bool) ([]string, error) {
	path := strings.TrimPrefix(parsed.EscapedPath(), "/")
	segments := strings.Split(path, "/")
	if len(segments) > maxHARPathSegments {
		return nil, errors.New("path segment limit exceeded")
	}
	if !decode {
		return nil, nil
	}
	decoded := make([]string, len(segments))
	for i, segment := range segments {
		value, err := url.PathUnescape(segment)
		if err != nil {
			return nil, errors.New("invalid escaped path segment")
		}
		decoded[i] = value
	}
	return decoded, nil
}

func checkedHAROrigin(origin string) (string, error) {
	base, err := harOrigin(origin)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(origin)
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid origin")
	}
	return base, nil
}

func harOrigin(raw string) (string, error) {
	if len(raw) > 8192 {
		return "", errors.New("invalid URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", errors.New("invalid URL")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid URL")
		}
		port = strconv.Itoa(n)
	}
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port, nil
}

func harHint(name string) string {
	switch strings.ToLower(name) {
	case "email", "email_address":
		return "Email-like field name"
	case "lat", "latitude", "lon", "lng", "longitude", "location":
		return "Location-like field name"
	case "phone", "phone_number":
		return "Phone-like field name"
	case "user_id", "account_id":
		return "Account-like field name"
	default:
		return ""
	}
}

// SaveHARReport reads a bounded local export and writes a standalone, value-free
// explanation without replacing an existing file. No network requests are made.
func SaveHARReport(input, origin, output, rulesPath string) error {
	rendered, err := ExportHARReport(input, origin, rulesPath)
	if err != nil {
		return err
	}
	return writeExclusive(output, rendered)
}

// ExportHARReport rebuilds a standalone redacted explanation with no links to
// the local app. The returned HTML is not an authenticity receipt.
func ExportHARReport(input, origin, rulesPath string) ([]byte, error) {
	return renderHARFile(input, origin, rulesPath, false)
}

// ReadHARReport rebuilds a redacted local-app page from the current bounded
// capture. It does not accept an HTML report as evidence or return raw values.
func ReadHARReport(input, origin, rulesPath string) ([]byte, error) {
	return renderHARFile(input, origin, rulesPath, true)
}

func renderHARFile(input, origin, rulesPath string, local bool) ([]byte, error) {
	rules, err := readHARRules(rulesPath)
	if err != nil {
		return nil, err
	}
	data, err := bundle.ReadBoundedFile(input, maxHARBytes)
	if err != nil {
		return nil, errors.New("cannot read HAR: use a regular file no larger than 8 MiB")
	}
	review, err := inspectHAR(data, origin, rules)
	if err != nil {
		return nil, err
	}
	var rendered bytes.Buffer
	view := struct {
		HARReview
		Local       bool
		Information []harInformation
	}{review, local, informationOverview(review, rules)}
	if err := harReportTemplate.Execute(&rendered, view); err != nil {
		return nil, errors.New("cannot render HAR report")
	}
	return rendered.Bytes(), nil
}

var harReportTemplate = template.Must(template.New("har").Funcs(template.FuncMap{"increment": func(n int) int { return n + 1 }}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>Browser activity, explained · Ariadne</title><style>body{font:17px/1.6 system-ui,sans-serif;background:#f3f8f3;color:#13291f;margin:0}main{max-width:850px;margin:auto;padding:32px 20px}h1{font-size:38px;line-height:1.2}section{background:white;border:1px solid #d5e2d9;border-radius:16px;padding:24px;margin:20px 0}h2{font-size:23px}summary{cursor:pointer;font-weight:bold}li{margin:8px 0}.note{color:#5e7167}.trail{display:flex;flex-wrap:wrap;gap:12px;align-items:center;padding:0;list-style:none}.trail li{background:#dff1e5;padding:10px 14px;border-radius:10px}.trail li+li:before{content:"→ ";color:#5e7167}.destination{background:white;border:1px solid #d5e2d9;border-radius:14px;padding:20px;margin:16px 0}.report-nav{display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin-bottom:18px}.report-nav a{font-weight:650}a{color:#176341}summary:focus-visible,a:focus-visible{outline:3px solid #0f4c32;outline-offset:3px}</style></head><body><main>{{if .Local}}<nav class="report-nav" aria-label="Report navigation"><a href="/">Ariadne / investigations</a><a href="/guide">How Ariadne works</a><a href="/capture-report" download>Save explanation</a></nav>{{end}}<p>Ariadne / saved browser capture</p><h1>What does this browser capture show?</h1><p>The export lists <strong>{{.Requests}} requests</strong> across <strong>{{len .Destinations}} origins</strong>. An origin is a particular web address and port. Different origins can belong to the same company.</p><p class="note">This is an inventory of a supplied file, not a live monitor or independently verified evidence that data reached a server. The file can be edited or incomplete.</p>
{{if .MatchingConfigured}}<section><h2>Follow the information</h2><p>These paths show where supplied test values appear in this file. Each destination comes from the matching request's web origin. They do not show later sharing by a server.</p>{{range .Information}}{{$information:=.}}<article><h3>{{.Title}}</h3>{{if .Destinations}}<p><strong>Found in {{.Requests}} distinct exported request(s)</strong> across {{len .Destinations}} destination(s).</p>{{range .Destinations}}<ol class="trail" aria-label="Recorded test-value path"><li>Browser export</li><li>{{$information.Title}}</li><li><a href="#destination-{{.Index}}">{{.Label}}</a></li></ol>{{end}}{{else}}<p>No exact match in the supported channels. This does not mean the information stayed private.</p>{{end}}</article>{{end}}<details><summary>How are these paths determined?</summary><p>Exact matches are checked in decoded URL query values, exported form parameters, supported textual JSON or form bodies, complete request header values, structured exported request cookie values, and complete URL path segments. Categories come from your test rules. A match confirms presence in this file, not delivery to a server or the authenticity of the export. Header, structured-cookie, and path representations are not proof of sending or receipt, and request headers or cookie values do not establish identity or cookie purpose. A request matched in several channels counts once per category. The category totals can overlap.</p></details></section>{{else}}<section><h2>Start with clues</h2><p>No test values are configured. The details below show field-name clues and cookie metadata; they cannot establish that your personal information was present.</p></section>{{end}}
<h2>Explore request details</h2>
<details><summary>About cookies, clues, and source entries</summary><p class="note">Cookie metadata may represent preferences, login state, or tracking. Its presence alone does not establish a purpose or identify a person; request headers and cookie values do not establish identity.</p><p>The matched values are omitted. Counts overlap when a request contains multiple categories or channels. Entry numbers start at 1 in the export’s request list; they identify positions in this file, not verified network events. Reordering the export changes them.</p><p>These are field-name clues in URL queries or exported form parameters. {{if $.MatchingConfigured}}These clues are separate from the exact matches above.{{else}}Values were not inspected.{{end}} A field called “email” does not prove that an email address was sent.</p><p>Missing body metadata and omitted traffic remain outside the body counts. Missing matches or field-name clues do not mean personal information was absent.</p></details>
{{range $index,$destination:=.Destinations}}<details class="destination" id="destination-{{increment $index}}"><summary>Request details: {{.Label}}</summary><h2>{{.Label}}</h2><p>{{.Requests}} exported request(s). {{.CookieRequests}} include cookie metadata.</p>{{if $.MatchingConfigured}}<p class="note">{{.InspectedBodies}} textual body/bodies inspected; {{.UninspectedBodies}} body entry/entries could not be inspected.</p>{{if .Matches}}<h3>Supplied values found in the capture</h3><ul>{{range .Matches}}<li>{{.Category}} · {{.Channel}} · {{.Requests}} request(s)<details><summary>Show the supporting entries</summary><p>Matched at these positions in the supplied capture: {{range .Entries}}<span>entry {{.}}</span>; {{end}}</p></details></li>{{end}}</ul>{{else}}<p>No supplied values matched in the supported channels. Encodings and uninspected channels may still contain them.</p>{{end}}{{end}}{{if .Hints}}<h3>Clues worth investigating</h3><ul>{{range .Hints}}<li>{{.}}</li>{{end}}</ul>{{else}}<p>No supported field-name clues were found.</p>{{end}}</details>{{end}}
<section><h2>What can I conclude?</h2><p>This capture helps choose what to investigate next. It does not tell us whether sharing less would still make the website work. That needs a controlled comparison like Ariadne's weather experiment.</p><details><summary>What is missing from this view?</summary><ul><li>Raw domains, paths, field names, and values are deliberately omitted. Origin numbers apply only within this report.</li><li>With test rules configured, matching covers decoded query values, complete URL path segments, exported form parameters, supported textual JSON strings and form bodies, complete request header values, and structured exported request cookie values. path substrings, joined segments, repeated encodings, unsupported encodings, binary bodies, response bodies, and arbitrary field names remain outside the matching scope. Header or cookie representations are not proof of sending or receipt, and request headers or cookie values do not establish identity or cookie purpose.</li><li>Omitted requests, cache and worker activity, other apps, and onward server sharing cannot be ruled out.</li><li>The website origin is supplied by the person importing the file. It does not establish company ownership.</li></ul></details></section></main></body></html>`))
