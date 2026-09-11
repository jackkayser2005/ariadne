---
title: Ariadne audit and verification report
weight: 5
---

This report records the current Ariadne audit baseline as of 2026-09-11. It
covers the local Go implementation, browser and proxy boundaries, portable
evidence verification, the green local review flow, and one live weather-site
investigation. It describes what the checks establish and keeps unsupported
claims as explicit gaps.

## Verification baseline

The following checks passed in the working tree:

~~~console
go fmt ./...
go build ./...
go vet ./...
go test ./...
go test -race -covermode=atomic -coverprofile=coverage.out ./...
~~~

The race-enabled run passed for all 15 Go packages and produced 90.1% total
statement coverage. Package coverage is not a proof of complete behavior: the
regression tests still target hostile files, malformed and oversized input,
redaction, path replacement, timeouts, cleanup, redirects, undeclared browser
origins, denied permissions, and incomplete captures.
The unified validator also inventories bounded HAR exports and generic
source-adapter runs, while leaving their boundary and replay guarantees
unavailable unless a separate controlled procedure supplies them.

The dependency baseline was checked with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` on Go 1.26.8. It completed successfully and reported `No vulnerabilities found.` This is a point-in-time result from the scanner database fetched during the run; repeat it before release.

## Findings and remediations

| Severity | Boundary | Finding | Remediation and regression coverage |
| --- | --- | --- | --- |
| High | Proxy process execution | A program path could be validated through a symlink and opened again during staging. | Validation now walks with `Lstat`, requires a regular file, and rechecks the opened identity before hashing or copying. `TestProxyProgramRejectsSymlinkPaths` covers leaf and ancestor links. |
| High | Source-adapter process execution | Hashing a requested executable before launching its path left a replacement window. | Real source-adapter runs now stage a bounded copy from a verified handle and execute that copy; the staged digest is rechecked before publication. TestStageSourceAdapterExecutableBindsBytesAndRejectsSymlinkPaths covers byte binding and cleanup. |
| High | Artifact and procedure reads | A path replacement could make a parser follow a symlink or reparse point. | Shared bounded reads reject symlink and irregular components and compare the opened file identity. Browser, proxy, ladder, plan, Android binding, and trace readers use this contract; package tests cover hostile paths. |
| High | Evidence binding | A copied receipt could be presented without proving that its child artifacts still matched. | Verification recomputes receipt, child, session, procedure, and binding identities before review. Mismatches fail closed and remain visible in the CLI and local UI. |
| Medium | Browser network scope | A live page could redirect or call an undeclared destination. | The weather driver pins reviewed HTTPS origins, blocks undeclared destinations, records the resulting gap, and never silently expands the allowlist. |
| Medium | Output and parser bounds | Hostile captures, driver output, request bodies, and plans could consume unbounded memory or time. | Readers and process pipes enforce size limits; procedures enforce duration and event limits; unsupported or truncated channels become `unknown`. |
| Medium | Redaction | Raw values, URLs, browser paths, and request bodies could leak into portable review output. | Reports retain fixed category and destination labels plus verifier-derived references. The live weather bundle was scanned for coordinates, URLs, and local paths before review. |

These severities describe impact at the local evidence boundary. They do not
claim that a remote service is compromised or that a captured event is
independently authentic.

## Measured performance

The existing HAR benchmarks provide reproducible local baselines:

- A near-limit synthetic capture with 10,000 requests (about 7.7 MB) completes
  in 94–103 ms and allocates about 19.9 MB per operation on Windows amd64.
- A full-report fixture with 10,000 distinct origins reduced HTML output from
  13,026,880 to 7,537,754 bytes and benchmark allocation from about 111.3 MB
  to 90.3 MB after report-template consolidation. Isolated process working set
  fell from 64.9–67.2 MB to 43.4–45.5 MB; elapsed time remained 205–216 ms
  before and 207–210 ms after.

These are synthetic, machine-local measurements. They cover parser and report
work, not browser layout, concurrent reviews, or remote network time.
See [browser capture review](browser-capture-review.md) for the commands and
fixture details.

## Live website investigation

The versioned `browser-weather-location-v1` procedure ran eight fresh-profile
sessions against the National Weather Service beta site. Precise, city-level,
and denied-location candidates were run with identical interaction and
bounded duration. Precise sessions showed forecasts in 4/4 sessions, city-level
sessions in 2/2, and denied sessions in 0/2. Supported attempted and
response-backed location matches were observed for the first two candidates.

The receipt
`4f3cf95a01ad0d413e979281a405561690cc2997b23d8617a4800f85bdf1d820` verifies
the retained redacted artifacts. Selection remains `unknown`: blocked origins,
capture-incomplete, unsupported-channel, and server-side-unobservable gaps
apply to the run, so Ariadne withholds a minimum-disclosure recommendation.
The forecast result is kept separate from exact weather-content equality, and
the driver cannot observe server-side storage or onward sharing.

The local GET-only pages at `/weather` and `/source-adapter` re-verify their saved artifacts on each request and
start with plain-language explanations. The CLI's default output reports the
same counts and limits without printing coordinates or URLs. The technical
identities remain available under the collapsed evidence section.

## Remaining coverage gaps and next phases

The current evidence does not cover worker traffic, unsupported encodings,
missing browser events, server-side processing, or arbitrary personal browsing.
A denied geolocation permission records configuration; it does not prove that a
page never attempted another location estimate. Geographic infrastructure
mapping remains a separate enrichment slice and is not inferred from this
capture.

The supported product flow is now:

`investigation -> comparison -> trace -> evidence`

The next phases can add reviewed data categories and more deterministic
fixtures while preserving portable local evidence bundles as the authority.
Hosted accounts, remote capture, broader native-app capture, and arbitrary
workflow scripting remain later work.