# Ariadne

Ariadne is an open-source counterfactual privacy analysis tool. It runs
software in controlled parallel environments, changes one piece of personal
data, and reports which observable behaviors change.

Reports classify conclusions as **observed**, **inferred**, **claimed**, or
**unknown**.

## Experiment 001

The first milestone targets an authorized Android test application:

1. Define two personas that differ by one value.
2. Run the same scripted interaction for both personas.
3. Capture network and app-storage observations.
4. Normalize expected noise.
5. Report outputs influenced by the changed value.
6. Produce a redacted, reproducible export from a verified evidence bundle.

The runner can also replicate the experiment in both orders. Each requested
replication runs baseline-treatment and treatment-baseline, resetting the
package before every session and recording the order in a raw-value-free
`replication.json` receipt:

```console
go run ./cmd/ariadne experiment replicate --device emulator-5554 --package dev.ariadne.fixture --pairs 1 --output .ariadne/runs/experiment-001-replicated examples/experiment-001.json
go run ./cmd/ariadne experiment report .ariadne/runs/experiment-001-replicated/pair-001-baseline-treatment
go run ./cmd/ariadne experiment report .ariadne/runs/experiment-001-replicated/pair-001-treatment-baseline
go run ./cmd/ariadne experiment replicate verify --json .ariadne/runs/experiment-001-replicated
```

Replication verification classifies the aggregate as `replicated-change`,
`no-change-observed`, `mixed-inconsistent`, or `unknown`. That outcome is
separate from the evidence model: `evidence_state` still reports whether the
captured artifacts support the result. A replicated change is stronger repeat
evidence, not proof of universal causal truth. Verification also rechecks each
complete pair's existing `evidence.json` and `report.md`, and returns a safe
receipt SHA-256 plus one evidence SHA-256 per ordered pair so the aggregate can
be bound back to the files that were checked.

The detailed design and experiment log live in [`docs/`](docs/).
The evidence-backed first-year path is tracked in
[`docs/content/docs/roadmap.md`](docs/content/docs/roadmap.md).
The read-only computer-use acceptance sequence is documented in
[`docs/content/docs/computer-use-acceptance.md`](docs/content/docs/computer-use-acceptance.md).
The source-neutral tracking trace contract is documented in
[`docs/content/docs/tracking-trace.md`](docs/content/docs/tracking-trace.md).

Ariadne now verifies and compares raw-value-free tracking traces from an
authorized source adapter:

```console
go run ./cmd/ariadne trace verify --json <trace.json>
go run ./cmd/ariadne trace compare --json <baseline-trace.json> <treatment-trace.json>
```

These traces contain only verifier-owned logical source, channel, destination,
and data-category labels. They do not contain payloads or URLs. Complete versus
partial source coverage is explicit, so an absent event in a partial capture
remains `unknown` rather than becoming a false absence claim. Browser, desktop,
proxy, and additional Android adapters are not implied by this contract; each
needs an authorized capture procedure and its own redaction tests.

The first browser edge accepts an authorized driver's already-redacted audit and
projects it into the same trace contract:

```console
go run ./cmd/ariadne browser trace --json examples/browser-audit.json .ariadne/browser-trace.json
go run ./cmd/ariadne trace verify --json .ariadne/browser-trace.json
```

The browser adapter accepts only fixed network, cookie, and web-storage labels;
it rejects URLs, payloads, cookie values, arbitrary destinations, and arbitrary
fields. It is a redacted handoff boundary, not browser capture or a universal
sniffer. The authorized driver that produces the audit remains a separate
source-specific concern.

The capture command now provides one explicit process boundary for that driver:

```console
go run ./cmd/ariadne browser capture --json --procedure examples/browser-procedure.json --driver <fixed-redacting-driver> .ariadne/browser-trace.json
```

A validated procedure contains only a catalogued procedure ID, scope, duration,
and event limit. Ariadne sends those bytes to the selected executable on stdin,
accepts exactly one bounded redacted audit on stdout, invokes it without a
shell, and rejects scope mismatches, oversized output, timeouts, and unsafe
audit members. The procedure has no URL, profile path, selector, JavaScript,
header, payload, or `authorized` claim; authorization remains an external
precondition. `examples/browser-procedure.json` is a safe metadata-only
starting point, not a capture configuration.

This is a driver boundary, not browser capture yet. It does not launch Chrome,
reuse a profile, collect cookies, inspect bodies, or prove that an external
driver was authorized. A future concrete driver must use an isolated profile,
an explicit local test target, and its own CDP/redaction tests before it can
be treated as a source producer.

The repository now includes one deterministic local-fixture producer. It takes
an explicit Chrome executable, creates a fresh temporary profile, serves only a
loopback fixture, and emits fixed network labels through the same boundary.
It requires Node 22 or newer:

```console
go run ./cmd/ariadne browser capture --json \
  --procedure examples/browser-local-fixture-procedure.json \
  --driver node \
  --driver-arg cmd/browser-fixture-driver/browser_fixture_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" \
  .ariadne/browser-local-trace.json
```

This producer is fixture evidence only. It does not accept a target URL, reuse
a profile, read cookies/storage/bodies/DOM, or claim coverage of arbitrary
browser sessions. Use `browser-local-fixture` as the session adapter when
binding this trace to provenance.

The same fixture path is exercised by the hosted Windows browser-fixture
workflow, which checks the safe output and confirms that the temporary profile
is removed.

Bind a verified trace to its reviewed adapter and capture procedure without
adding URLs, profile names, or captured values:

```console
go run ./cmd/ariadne trace session create --adapter browser-redacted-audit --adapter-version 1 --procedure-sha256 <procedure-sha256> .ariadne/browser-trace.json .ariadne/browser-session.json
go run ./cmd/ariadne trace session verify --json .ariadne/browser-session.json .ariadne/browser-trace.json
go run ./cmd/ariadne trace session pair create --json --adapter browser-redacted-audit --adapter-version 1 --procedure-sha256 <procedure-sha256> --order baseline-treatment <baseline-trace.json> <treatment-trace.json> <baseline-session.json> <treatment-session.json>
go run ./cmd/ariadne trace session pair verify --json <baseline-session.json> <baseline-trace.json> <treatment-session.json> <treatment-trace.json>
go run ./cmd/ariadne trace session pair compare --json <baseline-session.json> <baseline-trace.json> <treatment-session.json> <treatment-trace.json>
```

The standalone command creates one envelope. The pair command derives one
canonical pair identity from both verified trace identities and shared
provenance, then writes complementary baseline/treatment envelopes. Pair order
is explicit: use `baseline-treatment` or `treatment-baseline`. The envelope checks the trace hash,
source, scope, and completeness, but does not prove authorization, capture
truth, or causal impact. Pair verification additionally requires complementary
roles, distinct trace identities, and matching adapter, procedure, scope,
order, and canonical pair identity. Empty traces retain their declared
completeness, but have no event source to corroborate the adapter assertion.
The current fixed adapter catalog covers the implemented Android and browser
producers; future desktop or proxy producers add their own reviewed labels when
they exist.

The pair comparison command first verifies the session envelopes, then runs
the existing raw-value-free trace comparison and returns both objects together.
Provenance, structural differences, and evidence states remain separate; a
joined comparison is not a causal claim.

The shareable export has its own canonical SHA-256 identity. Verify a received
export structurally, and optionally require the expected identity, with
`experiment export verify --json --expect-sha256 <export-sha256> <export.json>`.
That identity covers only the raw-value-free export content; it does not prove
the underlying evidence.

A verified export can answer its embedded counterfactual question offline with
`experiment export ask --json <export.json> counterfactual-change`. Questions
about capture completeness or source integrity remain unavailable without the
authoritative evidence bundle. Follow one returned safe finding reference with
`experiment export finding --json <export.json> <finding-id>`; comparison values
remain unavailable, and both JSON responses carry the verified source-evidence
and export identities they came from.

## Reproduce Experiment 001

The complete local procedure is documented in
[`docs/content/docs/experiments/experiment-001.md`](docs/content/docs/experiments/experiment-001.md#reproduce-from-a-fresh-checkout).
It builds the authorized fixture, installs it on one explicitly selected
Android emulator, runs baseline and treatment sessions, and verifies the
resulting evidence bundle.

The same procedure runs on a real API 35 emulator in GitHub Actions. It also
proves that missing targets, modified observations, and mismatched package
provenance prevent evidence publication.

The hosted workflow also runs one replicated pair in both orders. The two
ordered pair directories are independently reportable, while the root receipt
checks the reset policy, recorded order, pair completeness, and aggregate
classification without exposing persona values or captured payloads. The
replication verifier rechecks each pair's authoritative outputs and exposes
only their safe SHA-256 identities.

After the report is verified, project one selected session into the portable
tracking trace and compare the two sessions without reopening raw values:

```console
go run ./cmd/ariadne experiment trace --session baseline .ariadne/runs/experiment-001 .ariadne/baseline-trace.json
go run ./cmd/ariadne experiment trace --session treatment .ariadne/runs/experiment-001 .ariadne/treatment-trace.json
go run ./cmd/ariadne trace compare --json .ariadne/baseline-trace.json .ariadne/treatment-trace.json
```

This first producer is deliberately narrow: it re-verifies the Experiment 001
bundle, recognizes only the fixture's known network and private-storage
artifacts, maps the `region` and volatile `request_id` keys to safe category
labels, and omits the experiment's `variant` value. A storage-gap run produces
a `partial` treatment trace, so the missing storage event remains `unknown` in
comparison. Browser, desktop, proxy, and additional Android adapters remain
separate authorized slices.

Once a report is saved, Ariadne can re-verify it offline and compare two saved
reflection snapshots without exposing captured values:

```console
go run ./cmd/ariadne experiment ask-archive verify --json <reflection.json>
go run ./cmd/ariadne experiment ask-archive save --json <archive-root> <question-id> <reflection.json>
go run ./cmd/ariadne experiment ask-archive compare --json <older-reflection.json> <newer-reflection.json>
go run ./cmd/ariadne experiment ask-archive compare-current --json <older-reflection.json> <archive-root>
go run ./cmd/ariadne experiment ask-archive transitions --json <reflection-1.json> <reflection-2.json> ...
go run ./cmd/ariadne experiment ask-archive transitions questions --json
go run ./cmd/ariadne experiment ask-archive transitions ask --json <history.json> [<question-id>]
go run ./cmd/ariadne experiment ask-archive transitions ask repeated --json <history.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all --json <history.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all save --json <history.json> <round.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all verify --json [--expect-sha256 <digest>] <round.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all compare --json <first-round.json> <second-round.json>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt --json <history.json> <question-id>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt save --json <history.json> <question-id> <receipt.json>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt verify --json [--expect-sha256 <digest>] <receipt.json>
go run ./cmd/ariadne experiment ask-archive transitions acceptance save --json <round.json> <receipt.json> <acceptance.json>
go run ./cmd/ariadne experiment ask-archive transitions acceptance verify --json [--expect-sha256 <digest>] <acceptance.json>
go run ./cmd/ariadne experiment ask-archive transitions save --json <reflection-1.json> <reflection-2.json> ... <history.json>
go run ./cmd/ariadne experiment ask-archive transitions verify --json <transitions.json>
```

The comparison reports `same`, `changed`, or `incomparable` for bounded
per-directory answer states. When common entries change, it also names those
safe archive directories and their older/newer answer states. It does not
infer a trend or prove the underlying evidence. The transitions command
applies those same bounded comparisons to each adjacent pair in caller-supplied
order and reports safe reflection identities, aggregate change counts, and any
changed archive directories with their bounded older/newer states. The saved
transition ledger carries those same state changes without observations or
persona values. Current ledgers also carry a safe summary for each supplied
snapshot: its reflection identity and observed, unknown, unavailable, and
checked counts. These summaries make the historical spine inspectable without
reopening raw evidence.
The saved transition ledger can be structurally re-verified and given an
expected content identity before another tool consumes it. Verification also
requires adjacent transitions to share their boundary reflection identity, so
a history cannot silently join unrelated snapshots.

`transitions questions` lists the fixed, raw-value-free questions available for
a verified history in stable order. Use it to discover the question IDs before
asking one; it does not create arbitrary natural-language queries.

`transitions ask` answers a catalog question from the verified history itself.
With no question ID it preserves the original history question; pass any ID
from `transitions questions` to select a fixed question. It returns only the bounded result, 1-based transition indexes
for changed or membership-incomparable boundaries, and safe directory/state
triples for changed entries, each bound to its adjacent reflection identities;
it does not infer chronology or prove the underlying evidence. The legacy
`ask repeated` spelling remains supported. Legacy schema 1
histories retain the indexes and have no per-entry details.

`transitions ask repeated` asks a second fixed question of the verified
history: whether any safe archive entry changed at more than one supplied
boundary. It returns the repeated entry's safe state-change records and
adjacent reflection identities. Schema 1 histories answer `unavailable`, and
the result never establishes chronology or a trend.

`transitions ask <history.json> answer-state-snapshot-summaries` asks which
safe snapshot summaries a verified history recorded. Schema 3 histories return each
snapshot's identity and observed/unknown/unavailable/checked counts; schema 1
and 2 histories answer `unavailable`. It does not infer chronology or prove
the underlying evidence.

`transitions ask <history.json> answer-state-summary-changes` asks whether
those bounded snapshot summaries changed at any supplied boundary. Schema 3
histories return `same` or `changed` plus 1-based boundary indexes; schema 1
and 2 histories answer `unavailable`. This is a bounded comparison, not a
chronology or trend claim.

`transitions ask all <history.json>` verifies the history once and records the
bounded result of every fixed history question in stable catalog order. Its
raw-value-free JSON is a portable question-round receipt; call an individual
question ID for detailed entries or snapshot summaries. Use
`transitions ask all save` to retain that round with exclusive creation; it
returns a canonical round SHA-256, and `transitions ask all verify` checks the
retained round without reopening the source history.

`transitions ask all compare <first-round.json> <second-round.json>` verifies
two retained rounds and compares their fixed bounded results in caller order.
Each round carries the source history-question identity, so rounds from
different source questions are rejected before comparison. The command
returns the two round and history identities plus any changed question IDs; it
does not infer chronology or prove the underlying evidence.

`transitions ask receipt <history.json> <question-id>` verifies the history once
and wraps one selected fixed answer in a portable raw-value-free receipt. The
receipt binds the bounded result and detailed answer to the verified history
SHA-256; use a question ID from `transitions questions` and do not infer
chronology or the underlying evidence from it.

`transitions ask receipt save <history.json> <question-id> <receipt.json>` does
the same verified ask and writes the raw-value-free receipt with exclusive
creation. It returns a receipt SHA-256 and never overwrites an existing
receipt, so later reflection work can retain the exact answer artifact.

`transitions ask receipt verify <receipt.json>` checks a retained receipt
without reopening the source history. It validates the fixed question,
nested answer's counts, indexes, states, ordering, and result consistency,
history digest, and canonical receipt SHA-256; pass `--expect-sha256` to
require a previously recorded receipt identity. This checks the receipt
contract only and does not re-verify the history or prove the underlying
evidence.

`transitions acceptance save <round.json> <receipt.json> <acceptance.json>`
verifies a retained question round and selected receipt, confirms their
history, question, and bounded-result identities agree, and writes only those
identities with exclusive creation. `transitions acceptance verify
<acceptance.json>` checks that raw-value-free binding offline and can require
its canonical SHA-256 with `--expect-sha256`. It does not prove that a UI
driver performed the selection.

New transition ledgers use schema 3 and include those snapshot summaries;
schema 2 ledgers remain readable, and schema 1 ledgers remain readable with
their older state-change limits.

Use `ask-archive save` to create a new snapshot with exclusive file creation;
it never overwrites an existing reflection. The command returns the same
canonical identity used by offline verification, so saved snapshots can feed
the comparison and transition commands without exposing captured values.

Use `transitions save` to persist the verified adjacent-boundary ledger with
the same no-overwrite behavior before opening it in the local history view.
The page shows the same safe snapshot summaries alongside the fixed history
questions, including a direct snapshot-summary question, so a UI driver can
choose a question and retain the identities it was asking about.

The local review page can receive a verified transition ledger, a saved
reflection, an acceptance identity binding, and two retained question rounds
with
`experiment serve --history <history.json> --reflection <reflection.json> --acceptance <acceptance.json> --round-first <first-round.json> --round-second <second-round.json> <archive-root>`.
It renders caller-ordered bounded transitions and re-asks the saved reflection's
fixed question against the current archive, showing only safe comparison counts,
identities, per-directory bounded state changes, and the repeated-change
question, snapshot-summary question, and snapshot-change question when history
is available. The history
panel presents a compact verified question round in fixed catalog order, with
each bounded result and a direct receipt link, so a UI driver can choose a
bounded question without inventing natural language. The page also shows the
question-round SHA-256. The selected receipt renders its stable history and
receipt SHA-256 identities alongside the raw-value-free JSON details.
When `--acceptance` is supplied, the page also reports whether the selected
question and receipt match the saved history, round, and receipt identities.
When the supplied verified history has the saved history identity, the saved
question ID is also a direct link back to that bounded history-question route.
This is a read-only identity comparison; it does not prove that a UI driver
performed the selection.
On 2026-08-10, a Windows Chrome smoke pass opened the loopback page, followed
the saved acceptance link, and rendered `MATCHED` with the raw-value-free
receipt identities visible. That validates the rendered route and accessibility
contract only; it does not prove target-application behavior.
When both `--round-first` and `--round-second` are supplied, the page also
shows which fixed question results changed between those retained rounds,
alongside both round and history identities. The comparison preserves caller
order and does not infer chronology; each changed fixed-question ID links back
to the same bounded history-question route for a repeatable re-check only when
the supplied history identity matches one of the compared rounds.

Stable-ID Android sessions also record a SHA-256 identity for the successful
UI hierarchy used to resolve the manifest-declared control. The raw hierarchy
XML is never retained in session metadata; this identity is control provenance
only and does not prove anything about the target application.

Those links use the validated `history_question_id` query and fail closed for
unknown IDs. An
unavailable comparison remains a generic bounded state; the
page does not turn it into chronology, trend inference, or a claim about the
underlying evidence.

The read-only review page exposes the same canonical SHA-256 identity for the
currently derived archive question report. It is computed in memory, contains
no captured values, and identifies the derived report only; it is not proof of
the underlying evidence or a trend claim.

It can also review one portable export with
`experiment serve --export <export.json> <archive-root>`. The export question
and its safe finding references remain read-only and display their verified
export identities.

## Development

Prerequisites:

- Go 1.24 or newer
- Docker Desktop for containerized tools and documentation
- JDK 17
- Android SDK Platform 36 and Build Tools 36.0.0

Preview the documentation:

```console
docker compose up --build docs
```

Then open <http://localhost:1313>.

Build and test the authorized Android fixture:

```console
cd fixture/android
./gradlew testDebugUnitTest createDebugUnitTestCoverageReport lintDebug assembleDebug
```

## Automation

Every push and pull request targeting `main` runs formatting, vetting,
race-enabled tests, and a 90% coverage gate. Changes to Experiment 001's
runner, fixture, or Go implementation also execute the complete workflow on a
real API 35 Android emulator.

Documentation changes merged into `main` are built and published through
GitHub Pages using the repository's **GitHub Actions** publishing source.

## Status

Pre-alpha. We are building Experiment 001 in the
[issue tracker](https://github.com/jackkayser2005/ariadne/issues).

## Safety

Only analyze software, devices, accounts, and data you own or are explicitly
authorized to test. Evidence bundles must redact secrets and unrelated personal
data by default.
