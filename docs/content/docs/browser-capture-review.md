---
title: "Understand a saved browser capture"
---

Ariadne can create a local, standalone explanation from a HAR 1.2 browser
export. This extends review beyond the fixed weather example. It does not
start a browser, make requests, or monitor your device.

```powershell
go run ./cmd/ariadne browser inspect-har --origin https://example.com --output review.html capture.har
```

Use a capture you are authorized to inspect. Open `review.html` locally. The
output must be a new file. Keep the original capture private: exports can
contain account information and credentials. The generated report omits raw
domains, paths, field names, values, cookies, and payloads.

The supplied origin identifies the website being investigated. Requests to
that exact scheme, host and port are grouped as **Website origin**; other
origins receive report-local numbers. Different origins do not establish
different companies. Group numbers cannot be compared between reports.

The report lists request counts, requests containing cookie metadata, and
fixed category hints from URL query names and exported form parameter names.
It never treats a location-like or email-like name as evidence that the
corresponding personal value was transmitted. Without test rules it does not analyze values.

This is an inventory of an editable export, not a verified capture receipt or
counterfactual experiment. Cache use, successful delivery, functionality,
server handling, and missing capture channels are not established. Without test rules, bodies are not inspected for personal data. Binary bodies,
partial path values, unsupported path encodings, response contents, and response
header values remain outside inspection. No hints means no supported hints, not privacy.

The supported request subset uses `log.version`, `log.entries`,
`request.url`, `request.headers`, `request.cookies`, and
`request.postData.params`, `request.postData.text`, its MIME type and encoding
markers, and `request.bodySize`. Other export properties are ignored. This is not a
full HAR conformance validator. Files are limited to 8 MiB, 10,000 entries,
64 levels of JSON nesting, 8,192-byte URLs, and 1,024 supported metadata items
per request list. Malformed input and duplicate JSON keys are rejected with
value-free errors. The parser uses the historical HAR 1.2 object vocabulary;
the abandoned [W3C draft](https://w3c.github.io/web-performance/specs/HAR/Overview.html)
is reference material, not a current standard or a conformance claim.

## Review inside the local app

```powershell
go run ./cmd/ariadne experiment serve --addr 127.0.0.1:8787 --har capture.har --har-origin https://example.com path/to/archive
```

The archive must be an existing directory for ordinary Ariadne evidence
bundles; it may be empty. The landing page links to **Explain this capture**
at `/capture`. The server reads only the capture configured at startup;
request parameters cannot select files or origins. Both capture flags are
required together. Each GET rebuilds the bounded report from the current file;
malformed or replaced content produces an unavailable page, never a stale
success. No upload endpoint or raw file download is added.

## Match supplied test values

Create a private rules file for synthetic values used in your authorized test:

```json
{"schema_version":1,"synthetic":true,"rules":[{"category":"email","value":"test-person@example.test"}]}
```

Use `--test-values rules.json` with `browser inspect-har`, or
`--har-test-values rules.json` with `experiment serve`. Keep values out of
command arguments. The report shows category, channel, and matching request
counts, never the matched values or rules-file contents.

The current categories are `email`, `phone`, and `account-id`. The category
and synthetic declaration are supplied by the investigator, not independently
validated by Ariadne. Rules require 1–16 unique values, each 8–256 UTF-8 bytes
without whitespace or control characters, in a regular file at most 16 KiB.
A longer distinct value reduces accidental matches but does not prove origin
or causality. Coordinate pairs remain part of the dedicated weather workflow.

Matching is case-sensitive equality of a complete URL query value after one
URL decoding pass, an exported form parameter value, a supported textual body
string, a complete request header value, or an exported request cookie value.
No substring matching, recursive decoding, or guessing is performed. Repeated
matches of the same category and channel within one request count once.
Counts across categories and channels can overlap. The same report keeps
field-name clues separate from value matches. No match remains inconclusive.

A match establishes only that the supplied value occurs in the editable
capture's supported request representation. It does not prove delivery,
server storage, authenticity, personal identity, or minimum sufficient
sharing. Raw captures and test rules are read locally and not copied into
the HTML output. Every local review reloads both; invalid rules fail closed.


### Follow a match to its source

Expand **Show the supporting entries** beneath an exact match to find its
one-based positions in the original capture's `log.entries` array. Entry 1
means `log.entries[0]`. The channel tells you whether the match came from the URL query, exported
form parameters, a JSON body string, a form-body value, a request header value,
or an exported request cookie value. The report retains no raw URLs,
parameter names, or values. Open the original privately if you need to inspect
those; the local server never serves it.

These positions are derived from the current parsed file and collected once
per matching category/channel/request. They are not authenticity receipts or
stable identifiers across captures. Editing or reordering the export can
change them. Field-name clues remain separate and do not inherit the stronger
exact-match claim.

### Large-capture measurement

On Windows amd64 with a Ryzen 7 7800X3D, three single-iteration runs of
`BenchmarkHARReviewNearLimit` processed 10,000 requests (about 7.7 MB) in
94–103 ms, allocating about 19.9 MB per operation. All 10,000 match references
were retained. This measures parser processing, not browser rendering or peak
working set; it is a local baseline, not a cross-machine performance promise.

```powershell
go test ./internal/browser -run '^$' -bench BenchmarkHARReviewNearLimit -benchmem -benchtime 1x -count 3
```

### Textual request bodies

With test rules configured, the importer also checks complete string values
in JSON bodies (`application/json` or `application/*+json`) and decoded values
in `application/x-www-form-urlencoded` bodies. JSON keys and numeric values
are not matched. An optional charset must be UTF-8. Only the textual export
representation is interpreted; this is not proof of on-wire encoding.

Bodies are limited to 64 KiB, JSON nesting to 32, and form field names to
1,024. Duplicate JSON keys, malformed content, unsupported MIME types,
nonempty `encoding` or `_encoding` markers, and oversized bodies produce an
uninspected-body count, not a no-disclosure finding. A positive `bodySize`
without exported body content also produces a gap. Missing body metadata and
omitted traffic cannot be counted and remain explicit general limitations.
Exported form parameters can still have separate matches when body text is
unavailable; the gap does not erase observations in other supported channels.

## Request headers and cookies

With test rules configured, Ariadne also checks complete request header values
and the structured `request.cookies` values supplied by the export. These
appear as **Request header value** and **Request cookie value**, with the same
destination labels and file-scoped entry references as other matches.

Matching is case-sensitive equality of the entire value. Header and cookie
values are not URL-decoded. Ariadne does not split a `Cookie` header, remove
an `Authorization` prefix, or treat an identifying name as a match. Repeated
matching headers or cookies within one request count once per category and
channel. The information overview deduplicates that request across channels.

For example, a supplied account test value found in an exported request cookie
links that request to its destination. It does not establish who the account
belongs to, what the cookie does, successful delivery, or later server use.
Raw names and values stay out of reports. Missing or transformed export values
remain inconclusive. Without test rules, values are not checked.

Redacted inventories use schema version 4 for the expanded channel vocabulary.
The input format remains HAR 1.2; existing standalone HTML reports remain
readable. Header and cookie lists keep the 1,024-item per-request limit.

## Identifiers in web address paths

An identifier can appear in a web address path, such as `/accounts/account-12345`.
With test rules configured, Ariadne checks complete path segments and labels
matches **URL path segment**. They use the same destination links and source
entry positions as other channels, with no raw path retained in the report.

Each segment is percent-decoded once after splitting on literal `/` characters.
For a supplied `account-12345`, `/accounts/account%2D12345` can match;
`/accounts/prefix-account-12345` and `/accounts/account%252D12345` cannot.
An encoded slash stays inside its segment and does not create extra segments.
A `+` stays a plus sign. Fragments and host names are not searched.

Matching uses case-sensitive equality of the whole decoded segment. It does
not join segments, match substrings, or establish how a server interpreted the
address. A request with repeated matching segments counts once for its category
and channel; the information overview also deduplicates it across other channels.

The existing 8,192-byte URL limit remains. Paths are limited to 1,024
slash-separated segments, including empty ones, whether or not test rules are
configured. Invalid or oversized paths fail the import with a value-free error.
Missing matches remain inconclusive.

## Compare two captures

```powershell
go run ./cmd/ariadne browser compare-har --origin https://example.com --test-values rules.json --output comparison.html first.har second.har
```

For the local app, add `--har-second second.har` to the existing `experiment
serve --har first.har --har-origin https://example.com --har-test-values
rules.json` command. The landing page links to **Compare these captures** at
`/capture-compare`. Both files must use the same supplied test rules and
website origin; comparison requires the rules file. Output must be new.

The parser privately normalizes and labels origins across the pair, so
reordered requests do not mix up destinations. Identical anonymous labels
from independently generated reports must never be joined. Labels in a
comparison are meaningful only within that pair. Different origins still do
not establish different companies.

The explanation shows each file's requests, body visibility gaps, exact
category/channel matches and file-scoped entry positions. It reloads both
configured files and the rules for every GET. URL parameters cannot select
other files, and invalid input produces an unavailable view. Empty captures
and absent matches stay inconclusive. The comparison does not establish
chronology, identical interactions, successful delivery, changed functionality,
causality, or minimum sufficient disclosure.

The local comparison page now starts with a plain-language answer. A match means a supplied test value appears in the checked export data; an empty result remains inconclusive. The three-step guide then points to the matching file, the reviewed destination label, and the visibility limits before the detailed origin sections.

## Follow information before requests

When test rules are configured, the capture page starts with one trail for
each configured data category. Each trail links to the request group for a
matched destination. Counts deduplicate request entries across matching
channels within a category; totals across categories may overlap. No field
name alone creates a trail. A configured category with no match remains
explicitly inconclusive.

The arrows describe relationships in the supplied export, not observed
onward sharing or independently authenticated network delivery. Request-level
counts, channel details and source references remain available in expandable
panels below the overview.

## Read the comparison at a glance

The comparison starts with information categories matched in at least one
file. Each side shows how many distinct request entries contained a supplied
test value and which destinations those entries reference. A request matched
in several channels is counted once within its category. Totals across
categories can still overlap.

Follow a destination link, then expand its supporting entries. Detailed request
groups start collapsed, while body inspection gaps remain visible beside the
overview. Categories without a match on one side remain inconclusive; a
smaller count does not establish less real-world disclosure or a working
website. If neither file contains a supported match, the page explains that
there is insufficient information to compare the supplied values.

## Save an explanation from the app

Choose **Save explanation** on `/capture` or **Save comparison** on
`/capture-compare`. The app rebuilds the report from the current configured
inputs and downloads standalone HTML. It uses the same redacted rendering as
the CLI export; the file retains trails, source-entry positions, and stated
limits, without raw captures, test-rule values, domains, or local file paths.
The saved copy has no links back to the running local app and needs no scripts
or remote resources.

These downloads use fixed GET-only endpoints (`/capture-report` and
`/capture-comparison-report`). URL parameters cannot choose source files or
filenames. Invalid or missing configured inputs produce an error, not a stale
download. A saved explanation is an editable report, not a signed receipt or
independent proof that the underlying capture is authentic.

### Full-report rendering and peak memory

These measurements describe the September 7 report-template consolidation,
before the header and cookie matching extension.

A separate 7,398,927-byte fixture uses 10,000 distinct origins, each with one
exact test-value match. It exercises file validation, parsing, information
trails, HTML rendering, and export. Consolidating repeated explanatory prose
into one shared legend preserved all 10,000 destination anchors and source
entry references while reducing output and memory costs:

| Metric | Before | After |
| --- | ---: | ---: |
| HTML bytes | 13,026,880 | 7,537,754 |
| Benchmark allocated bytes per operation (approximately) | 111.3 MB | 90.3 MB |
| Isolated export process peak working set, three runs | 64.9–67.2 MB | 43.4–45.5 MB |
| Full-report benchmark elapsed, three runs | 205–216 ms | 207–210 ms |

These Windows/amd64 measurements used a Ryzen 7 7800X3D. MB above is decimal.
Allocation totals differ from peak resident memory. Peak working set comes
from the operating system's process counter, polled during an isolated CLI
export, and includes the Go runtime and startup. Fixture construction happens
outside that measured process. Files are synthetic and retained under a new
`.cache/har-perf-*` directory for inspection. Timing does not establish a
speedup; these measurements do not cover browser layout or concurrent reviews.

```powershell
go test ./internal/browser -run '^$' -bench BenchmarkHARFullReport -benchmem -benchtime 1x -count 3
go build -o .cache/ariadne-perf.exe ./cmd/ariadne
./.github/scripts/measure-har-report.ps1 -Executable .cache/ariadne-perf.exe
```

Input bounds, fresh reads, match semantics, and request references are
unchanged. Shared explanations remain available beside the detailed groups.
