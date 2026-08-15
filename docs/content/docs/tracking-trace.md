---
title: Source-neutral tracking traces
weight: 5
---

# Source-neutral tracking traces

Ariadne can verify and compare a small, raw-value-free trace document from an
authorized source adapter. The contract is deliberately narrower than a
packet sniffer: it records where a labelled data category was observed, not
the payload, URL, cookie value, account identifier, or page content.

## Contract

Each trace declares a safe scope and whether the source adapter covered that
scope completely:

```json
{
  "schema_version": 1,
  "redacted": true,
  "scope": "outbound",
  "completeness": "complete",
  "events": [
    {
      "source": "browser",
      "channel": "network",
      "kind": "request",
      "destination": "analytics",
      "fields": ["device-id", "region"]
    }
  ]
}
```

`source`, `channel`, `kind`, and `destination` come from Ariadne's reviewed
safe catalog; they are not URLs or raw source strings. `fields` are
data-category labels such as `device-id`, `consent`, or `region`; they are not
values. Duplicate event
identities, duplicate fields, unknown JSON members, oversized documents, and
unsafe identifiers are rejected.

An adapter must remove payloads and source-specific identifiers before
writing this document. The repository includes concrete producers for the
authorized Experiment 001 Android fixture and for an already-redacted browser
audit; they are described below. These are source-specific handoff boundaries,
not universal capture adapters.

## Verify and compare

Verify one document and retain its canonical identity:

```console
go run ./cmd/ariadne trace verify --json <trace.json>
go run ./cmd/ariadne trace verify --json --expect-sha256 <trace-sha256> <trace.json>
```

Compare two traces from the same declared scope:

```console
go run ./cmd/ariadne trace compare --json <baseline-trace.json> <treatment-trace.json>
```

The comparison reports unchanged event identities, structural additions,
removals, and field-set changes. Added or removed events are `observed` only
when both traces declare `complete` coverage. If either side is `partial`,
absence is reported as `unknown`. A structural trace result does not prove
what a payload contained or why a source sent it; the authoritative evidence
bundle remains the place for those claims.

## Experiment 001 Android producer

After `experiment report` and `experiment verify` succeed, the first producer
re-verifies the bundle and projects one selected session:

```console
go run ./cmd/ariadne experiment trace --session baseline <run-directory> <baseline-trace.json>
go run ./cmd/ariadne experiment trace --session treatment <run-directory> <treatment-trace.json>
go run ./cmd/ariadne trace verify --json <baseline-trace.json>
go run ./cmd/ariadne trace compare --json <baseline-trace.json> <treatment-trace.json>
```

The mapping is fixed in code. The fixture's known network request becomes
`android / network / request / first-party`; a captured private-storage write
becomes `android / app-storage / storage-write / first-party`. Known observation
keys map only to `region` and `session-id`; `variant` is intentionally omitted
because it is an experiment outcome, not a tracking category. Values, URLs,
request-identifier values, package names, and unknown keys never enter the
trace.

An incomplete treatment session emits only its verified network event and
marks the document `partial`. Comparing it with a complete baseline therefore
reports the missing storage event as `unknown`.

## Authorized browser audit producer

An authorized browser driver can hand Ariadne a bounded audit after removing
URLs, payloads, cookie values, page content, and source-specific identifiers:

```console
go run ./cmd/ariadne browser trace --json <redacted-browser-audit.json> <trace.json>
go run ./cmd/ariadne trace verify --json <trace.json>
```

The input has the same declared scope and completeness vocabulary, but its
events omit `source`; the producer sets `source` to the reviewed `browser`
label. It accepts only `network` requests/responses/beacons, `cookie` writes,
and `web-storage` writes, with the fixed destination and data-category catalogs.
Unknown JSON members, duplicate identities, arbitrary labels, URLs, and
payload-shaped fields are rejected. A partial audit remains partial, so missing
browser events remain `unknown` during comparison.

This producer validates the redaction and handoff boundary. It does not launch
a browser, inspect a user's session, or claim that the supplied audit is true;
the authorized capture driver is responsible for producing the audit within its
declared scope.

## Explicit browser driver boundary

Ariadne can now invoke one explicitly selected capture executable through a
small process boundary:

```console
go run ./cmd/ariadne browser capture --json \
  --procedure examples/browser-procedure.json \
  --driver <fixed-redacting-driver> \
  <trace.json>
```

The procedure is bounded and raw-value-free:

```json
{
  "schema_version": 1,
  "procedure_id": "browser-audit-v1",
  "scope": "outbound",
  "duration_ms": 5000,
  "max_events": 64
}
```

The command sends the validated procedure bytes to the executable on stdin and
accepts exactly one bounded redacted browser audit on stdout. It invokes the
executable without a shell, enforces the procedure timeout and event limit,
requires the audit scope to match, and never returns driver diagnostics. The
procedure catalog intentionally has no URL, profile path, selector, script,
header, payload, or `authorized` field. Authorization is an external
precondition, and the selected executable is responsible for its own isolation
and redaction.

This boundary alone is not a browser capture implementation. Ariadne does not
reuse a profile, inspect cookies, retain response bodies, or infer that an
executable was authorized. The local fixture below provides the deterministic
CDP redaction proof; the target mode reuses that bounded collector while
adding an exact declared-origin request boundary.

### Local fixture producer

The repository also includes one deterministic producer for a local fixture:

```console
go run ./cmd/ariadne browser capture --json \
  --procedure examples/browser-local-fixture-procedure.json \
  --driver node \
  --driver-arg cmd/browser-fixture-driver/browser_fixture_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" \
  <trace.json>
```

The Node 22 driver launches the explicitly supplied Chrome executable with a fresh
temporary profile, binds a local fixture server to loopback, listens only to
bounded CDP network events, maps known query-key names to the fixed field
catalog, and discards URLs and values before writing the audit. It does not
read cookies, storage values, response bodies, DOM content, or arbitrary
targets. Unsupported activity becomes partial rather than a completeness
claim. The generated trace can be bound with adapter `browser-local-fixture`.
Direct capture uses the fixture's baseline behavior unless the fixed driver is
invoked by the replication runner.

This proves one local producer path and its cleanup/redaction behavior; it is
not evidence about a user's browser or a general browser capture capability.

### Isolated authorized HTTPS target producer

The repository also includes a narrow producer for one explicitly authorized
HTTPS origin. Replace the reserved example origin in
`examples/browser-target-procedure.json` before use; do not commit a personal
target procedure:

```console
go run ./cmd/ariadne browser capture --json \
  --procedure examples/browser-target-procedure.json \
  --driver node \
  --driver-arg cmd/browser-fixture-driver/browser_fixture_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" \
  <trace.json>
```

`browser-target-v1` carries only one canonical HTTPS origin, which is included
in the procedure identity. The Node 22 driver launches a new Chromium process
with a fresh temporary profile, allows only that hostname through its resolver
boundary, blocks requests whose URL origin is not exactly the declared origin,
observes bounded page-load network metadata, maps recognized query keys to
fixed fields, and discards URLs, cookies, storage, DOM, headers, bodies, and
values. Requests outside the target origin or unsupported activity make the
trace partial; they never become an affirmative third-party claim.

This producer is a single-origin, page-load metadata boundary. It is not a
browser history reader, an existing-profile observer, a universal network
sniffer, or proof of target authorization, capture truth, or causal impact.
The resulting trace uses the existing session, pair, replication-ledger,
question-round, and receipt verification paths.

### Loopback CONNECT proxy producer

The repository also includes a narrow `proxy-connect-v1` producer for one
explicitly authorized HTTPS authority. Replace the reserved authority in
`examples/proxy-connect-procedure.json` before use; do not commit a personal
target procedure:

```console
go run ./cmd/ariadne proxy capture --json \
  --procedure examples/proxy-connect-procedure.json \
  --program "C:\\Path\\to\\authorized-app.exe" \
  --program-arg <arg> \
  <trace.json>
```

The command launches exactly the supplied executable without a shell, passes
only common runtime path/locale variables plus the proxy variables, and
injects a fresh one-time credential for a loopback HTTP proxy into the child
environment. The proxy accepts only authenticated `CONNECT` requests to the
procedure's canonical lower-case DNS authority. It rejects plaintext HTTP,
other authorities, IP literals, malformed headers, and requests beyond the
declared event limit. It relays the tunnel opaquely; it does not terminate TLS,
install a CA, or retain URLs, hostnames, headers, bodies, cookies, credentials,
or process arguments. The trace is always `partial`, and an accepted tunnel
emits only `proxy` / `network` / `request` / `first-party` / `unknown`.

This is a single-authority, non-MITM boundary for a proxy-aware child process,
not a universal network sniffer or proof of authorization, capture truth, or
causal impact. Bind a verified trace with adapter `proxy-connect`; the existing
session, pair, replication-ledger, and question paths remain source-neutral.

### Replicated proxy process boundary

The proxy producer also has a source-specific replication runner. It accepts
one executable, shared arguments, and one final controlled argument for each
condition:

```console
go run ./cmd/ariadne proxy replicate --json \
  --procedure examples/proxy-connect-procedure.json \
  --program "C:\\Path\to\\authorized-app.exe" \
  --shared-arg <shared-arg> \
  --baseline-arg <baseline-value> \
  --treatment-arg <treatment-value> \
  --pairs 2 \
  --output .ariadne/proxy-replicated
go run ./cmd/ariadne proxy replicate verify --json \
  .ariadne/proxy-replicated
```

Each requested pair runs both `baseline-treatment` and
`treatment-baseline`. Each of the four sessions in a two-pair run invokes
`proxy.Capture`, so it receives a fresh child process, loopback listener, and
credential. This is a process/proxy reset assertion only; it does not reset a
remote account, server, filesystem, or device state. The runner stages a
private run-local executable copy and uses its digest to bind every session to
the same bytes. The receipt stores the executable SHA-256, order, completion,
and provenance identities, but not the deterministic procedure digest. It never
stores the executable path, arguments, condition values, target authority,
proxy credential, environment, or tunnel data. Procedure-bound session
envelopes remain the provenance join point for later verification.

Verification recomputes every complete session pair from the trace and session
files. It reports `replicated-change`, `no-change-observed`,
`mixed-inconsistent`, or `unknown` separately from `evidence_state`. A partial
proxy trace can therefore support an observed same event while retaining
`evidence_state: unknown`; a missing event remains an unknown outcome because
partial capture cannot establish absence. An interrupted run leaves a bounded
`incomplete` receipt rather than being promoted to a completed result. This
runner is a narrow authorized process boundary, not universal tracing or a
causal proof.

### Replicated local fixture

The fixture also has a counterfactual runner that owns the two fixed variants:

```console
go run ./cmd/ariadne browser fixture replicate --json \
  --procedure examples/browser-local-fixture-procedure.json \
  --driver node \
  --driver-arg cmd/browser-fixture-driver/browser_fixture_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" \
  --pairs 2 \
  --output .ariadne/browser-fixture-replicated
go run ./cmd/ariadne browser fixture replicate verify --json \
  .ariadne/browser-fixture-replicated
```

Each pair runs `baseline-treatment` and `treatment-baseline`. Every session
gets a fresh ephemeral browser profile; the runner appends the fixed variant
inside that process boundary and rejects caller-supplied variant overrides.
The root `replication.json` receipt contains only the reviewed procedure
identity, reset policy, pair/order metadata, and completion status. Each pair
contains provenance-bound baseline and treatment traces/sessions. Verification
reuses the portable session-pair check and structural comparison, then reports
`replicated-change`, `no-change-observed`, `mixed-inconsistent`, or `unknown`
separately from `evidence_state`. Two separate trace files may have identical
normalized content; that is a valid `no-change-observed` result.

The checked-in fixture intentionally marks unsupported or failed activity as
partial. Its hosted smoke therefore expects the safe aggregate `unknown` and
`evidence_state: unknown`; complete synthetic traces exercise replicated change,
no change, and mixed outcomes without changing the live producer's coverage
claim.

This remains a deterministic local-fixture smoke path. It is not a user-session
adapter, a universal browser sniffer, or evidence about arbitrary browser data.

## Session provenance

A trace can be retained with a small provenance envelope after it verifies:

```console
go run ./cmd/ariadne trace session create \
  --adapter browser-redacted-audit \
  --adapter-version 1 \
  --procedure-sha256 <procedure-sha256> \
  <trace.json> <session.json>
go run ./cmd/ariadne trace session verify --json <session.json> <trace.json>
go run ./cmd/ariadne trace session pair create --json \
  --adapter browser-redacted-audit \
  --adapter-version 1 \
  --procedure-sha256 <procedure-sha256> \
  --order baseline-treatment \
  <baseline-trace.json> <treatment-trace.json> \
  <baseline-session.json> <treatment-session.json>
go run ./cmd/ariadne trace session pair verify --json \
  <baseline-session.json> <baseline-trace.json> \
  <treatment-session.json> <treatment-trace.json>
go run ./cmd/ariadne trace session pair compare --json \
  <baseline-session.json> <baseline-trace.json> \
  <treatment-session.json> <treatment-trace.json>
```

The envelope records only a fixed adapter label and version, a reviewed
procedure SHA-256, the trace SHA-256, scope, completeness, and a standalone or
counterfactual role. The pair-create command derives one canonical pair
identity from both verified trace identities and shared provenance, then writes
both sides with either `baseline-treatment` or `treatment-baseline` order. Both
sides of a matched pair share the pair identity; their role and order remain
separate fields.

Verification re-reads the trace and fails closed when its canonical identity,
source, scope, or completeness no longer agrees. The envelope does not prove
that a driver was authorized, that its capture is complete in the real world,
or that a counterfactual result is causal. Fixed adapter labels keep this
joinable without accepting profile names, URLs, device identifiers, or free-form
metadata. Pair verification also requires complementary baseline/treatment
roles, separate trace paths, and matching adapter, procedure, scope, order, and
canonical pair identities. Identical normalized trace content remains valid
for a no-change pair. An empty trace can still be valid, but its
adapter source is an assertion rather than an event-source corroboration.

The pair comparison command verifies the four inputs as one complementary
session pair before invoking the ordinary trace comparison. Its result nests
the verified `pair` metadata beside the structural `comparison`, so a changed
or missing event never becomes a provenance claim and an `unknown` evidence
state is not turned into an observed outcome.

## Source-neutral replicated trace ledger

Already-produced matched pairs can be retained in one portable ledger without
adding a source-specific runner to the trace package. Each input group is a
baseline trace, treatment trace, baseline session, and treatment session. The
caller records whether the required reset was confirmed before that pair's
sessions:

```console
go run ./cmd/ariadne trace replication save --json \
  --reset-confirmed 1 --reset-confirmed 2 \
  .ariadne/trace-replication.json \
  <baseline-1-trace.json> <treatment-1-trace.json> \
  <baseline-1-session.json> <treatment-1-session.json> \
  <baseline-2-trace.json> <treatment-2-trace.json> \
  <baseline-2-session.json> <treatment-2-session.json>
go run ./cmd/ariadne trace replication verify --json \
  --expect-sha256 <ledger-sha256> .ariadne/trace-replication.json
```

The ledger embeds only normalized traces and provenance-bound sessions. It
retains each pair's explicit `baseline-treatment` or `treatment-baseline`
order, the reset assertion, pair identity, completeness, and safe comparison
counts. Verification recomputes the pair identities and comparisons from the
embedded documents, requires all pairs to share reviewed source, adapter,
procedure, and scope, and requires equal nonzero counts of both orders before
classifying the aggregate.

The aggregate outcome is one of `replicated-change`, `no-change-observed`,
`mixed-inconsistent`, or `unknown`. It remains separate from `evidence_state`:
an observed outcome does not make the trace capture truthful, and an unknown
evidence state does not become `no-change-observed`. Missing reset confirmation,
partial capture, an incomplete comparison, an unbalanced order set, or a
malformed ledger yields the safe `unknown` outcome. The reset policy is
`caller-confirmed-before-each-session`; that record is an assertion, not proof
that a source was reset.

Repeat `--reset-confirmed <pair-index>` once for each confirmed pair. An
omitted pair index remains unconfirmed, so mixed reset support can be retained
without overstating the whole ledger.

The ledger also has a fixed question surface for looking back without reopening
source-specific inputs:

```console
go run ./cmd/ariadne trace replication questions --json
go run ./cmd/ariadne trace replication ask all --json .ariadne/trace-replication.json
go run ./cmd/ariadne trace replication ask all save --json \
  .ariadne/trace-replication.json .ariadne/trace-replication-round.json
go run ./cmd/ariadne trace replication ask all verify --json \
  --expect-sha256 <round-sha256> .ariadne/trace-replication-round.json
go run ./cmd/ariadne trace replication ask receipt save --json \
  .ariadne/trace-replication-round.json replication-outcome \
  .ariadne/trace-replication-receipt.json
go run ./cmd/ariadne trace replication ask receipt verify --json \
  --expect-sha256 <receipt-sha256> .ariadne/trace-replication-receipt.json
```

The three fixed questions cover aggregate outcome, reset/comparison support,
and consistency across both explicit orders. A question round is bound to the
ledger SHA-256; a selected receipt is bound to both the ledger and round
identities. Their result and `evidence_state` remain separate, and offline
verification does not reopen source paths or captured values.

## Portable cross-run replication study

When the same counterfactual has been repeated independently, a study joins
the resulting ledgers without becoming a capture or scheduling service. The
caller supplies a private SHA-256 commitment for the counterfactual and one
saved question round for each ledger:

```console
go run ./cmd/ariadne trace study save --json \
  --contrast-sha256 <private-contrast-sha256> \
  .ariadne/trace-study.json \
  .ariadne/trace-replication-1.json .ariadne/trace-replication-1-round.json \
  .ariadne/trace-replication-2.json .ariadne/trace-replication-2-round.json
go run ./cmd/ariadne trace study verify --json \
  --expect-sha256 <study-sha256> .ariadne/trace-study.json
```

Study creation re-verifies every embedded ledger and question round, binds the
round to that ledger's canonical identity, rejects duplicate ledger or round
identities, and requires shared source, adapter, adapter version, procedure
digest, and scope provenance. It requires 2--8 runs, keeps `order_basis:
caller`, and does not interpret caller order as chronology. A run is supported
only when its pair orders are balanced, all resets are confirmed, all pairs are
complete, and no comparison support is unknown.

The study outcome is `replicated-change` when every supported run observes a
safe category difference, `no-change-observed` when every supported run
observes no safe category difference, `mixed-inconsistent` when supported runs
disagree, and `unknown` when any run lacks support. `evidence_state` is
calculated independently and becomes `unknown` whenever the study cannot
support its aggregate. The commitment binds the study to a caller's private
counterfactual identity but stores neither that value nor target identifiers.
The study does not execute resets, infer chronology, add statistics, capture a
browser, or turn repeated comparison into universal causal attribution.

This ledger is intentionally not a runner, capture adapter, chronology model,
database, statistical model, or causal proof. Source-specific producers remain
responsible for authorization, isolation, reset execution, redaction, and
capture completeness. The ledger is the portable aggregation and
re-verification boundary after those producers have emitted reviewed sessions.

## Fixed study questions and read-only review

The study exposes three fixed questions after offline verification. They ask for
the aggregate outcome, whether every run has balanced/reset-confirmed/complete
support with observed evidence, and whether supported runs agree. The complete
answer set can be retained as a durable round, and one selected answer can be
retained as a receipt:

```console
go run ./cmd/ariadne trace study questions --json
go run ./cmd/ariadne trace study ask --json .ariadne/trace-study.json study-outcome
go run ./cmd/ariadne trace study ask all --json .ariadne/trace-study.json
go run ./cmd/ariadne trace study ask all save --json \
  .ariadne/trace-study.json .ariadne/trace-study-round.json
go run ./cmd/ariadne trace study ask all verify --json \
  --expect-sha256 <round-sha256> .ariadne/trace-study-round.json
go run ./cmd/ariadne trace study ask receipt save --json \
  .ariadne/trace-study-round.json study-outcome \
  .ariadne/trace-study-receipt.json
go run ./cmd/ariadne trace study ask receipt verify --json \
  --expect-sha256 <receipt-sha256> .ariadne/trace-study-receipt.json
```

The answer's `result`, aggregate `outcome`, and `evidence_state` remain
separate. The round contains only the fixed answers and the study SHA-256; the
receipt embeds the bounded round and binds it to both the study and round
identities. Both can be verified offline without reopening source paths or
captured values. Identity drift fails closed.

To compare two retained study reflections without reopening source paths:

```console
go run ./cmd/ariadne trace study ask all compare --json \
  .ariadne/first-study.json .ariadne/first-study-round.json \
  .ariadne/second-study.json .ariadne/second-study-round.json
```

The command re-verifies both studies and their rounds, then requires matching
private counterfactual commitments and reviewed source provenance. A valid
compatible comparison is `same` or `changed`; a valid boundary mismatch is
`incomparable`. Changed entries contain only fixed question IDs, bounded first
and second projections, and fixed change kinds. The supplied order is retained
as caller order, not treated as chronology or a causal direction. The command
is a reflection comparison, not trend analysis, authorization proof, or a
capture service.

To expose the same verified identities through the local loopback server:

```console
go run ./cmd/ariadne experiment serve \
  --trace-study .ariadne/trace-study.json \
  --trace-study-round .ariadne/trace-study-round.json \
  --trace-study-receipt .ariadne/trace-study-receipt.json \
  <archive-root>
```

Open `/trace-study` from the review index. The GET-only route shows the private
commitment identity, caller order, aggregate counts, fixed answers, and each
ledger/question-round identity. When supplied, it re-verifies the saved round
and receipt against the study identities. A fixed `?question_id=` can select a
bounded in-memory receipt when no saved receipt is supplied. It never renders
input paths, payloads, URLs, or captured values, and fails closed as `trace
study unavailable`.

## Caller-ordered trace archive questions

Standalone trace sessions from any currently reviewed adapter can be retained
in one portable archive. Creation verifies each session against its trace and
stores only the normalized trace document plus its raw-value-free provenance
envelope; input paths are not retained. The order is explicitly the caller's
order, not inferred chronology.

```console
go run ./cmd/ariadne trace archive create --json \
  --trace baseline-trace.json --session baseline-session.json \
  --trace treatment-trace.json --session treatment-session.json \
  .ariadne/trace-archive.json
go run ./cmd/ariadne trace archive verify --json .ariadne/trace-archive.json
go run ./cmd/ariadne trace archive questions --json
go run ./cmd/ariadne trace archive ask all --json .ariadne/trace-archive.json
go run ./cmd/ariadne trace archive ask all save --json \
  .ariadne/trace-archive.json .ariadne/trace-round.json
go run ./cmd/ariadne trace archive ask all verify --json \
  .ariadne/trace-round.json
go run ./cmd/ariadne trace archive ask receipt save --json \
  .ariadne/trace-round.json trace-change .ariadne/trace-receipt.json
go run ./cmd/ariadne trace archive ask receipt verify --json \
  .ariadne/trace-receipt.json
```

The fixed archive questions are deliberately small:

- `trace-coverage` reports `complete` only when every retained trace declares
  complete coverage; otherwise it returns `unknown` with
  `evidence_state: unknown`.
- `trace-change` compares adjacent entries only when their reviewed source,
  adapter, procedure, and scope match. It returns `changed`, `same`, `mixed`,
  or `unknown`; partial boundaries remain unknown even when their visible
  labels happen to match.
- `trace-sources` reports the reviewed source/adapter identities represented in
  the archive without reopening source values.

The archive and every answer carry a canonical SHA-256 identity. The archive is
a review index, not a replacement for authoritative evidence bundles, a
chronology model, a natural-language question engine, or a universal capture
service. Future authorized desktop or proxy producers can enter this same
surface only after their own adapter and redaction contracts exist.

A question round is a durable, raw-value-free answer set bound to the archive
SHA-256 and caller order. A selected receipt is bound to both the archive and
round identities. `ask all verify` and `ask receipt verify` validate those
documents without reopening the source archive; an outcome such as `changed`,
`same`, or `unknown` remains separate from its `evidence_state`.

## Read-only trace archive review

The local review page can expose the archive as a separate reflection route:

```console
go run ./cmd/ariadne experiment serve \
  --trace-archive .ariadne/trace-archive.json \
  <archive-root>
```

To render a saved question round without reopening the source archive, use:

```console
go run ./cmd/ariadne experiment serve \
  --trace-round .ariadne/trace-round.json \
  <archive-root>
```

Supplying both `--trace-archive` and `--trace-round` makes the route verify
that the live archive and saved round identities agree before rendering.

Open `/trace-archive` from the loopback page. The route re-verifies one archive
and answers all three fixed questions from that same in-memory document. It
shows the caller-order basis, archive SHA-256, complete/partial counts,
reviewed source adapters, and each question's outcome beside its separate
`evidence_state`. It never renders the configured input path, raw trace labels,
or captured values, and a tampered or malformed archive returns only the
generic `trace archive unavailable` state.

This route is intentionally not folded into `/?question_id=...`: bundle
questions and source-neutral trace questions have different identities and
different evidence contracts. The page offers no free-form question box and
the route remains GET-only, so a computer-use driver can inspect the fixed
question round without inventing a question or causing a mutation.

The same loopback server can expose a verified replicated ledger with:

```console
go run ./cmd/ariadne experiment serve \
  --trace-replication .ariadne/trace-replication.json \
  <archive-root>
```

Open `/trace-replication` to review the aggregate outcome, the three fixed
questions, separate evidence state, both explicit order counts, reset
assertions, pair identities, and safe difference/unknown counts. The route is
GET-only and does not render the configured ledger path, source paths, URLs,
payloads, or captured values.
Malformed or tampered input produces only `trace replication unavailable`.

## Portable cross-source case package

Once standalone archives or replicated ledgers have their fixed question
rounds, they can be joined into one bounded, caller-ordered package:

```console
go run ./cmd/ariadne trace case save --json \
  .ariadne/case.json \
  trace-archive .ariadne/trace-archive.json .ariadne/trace-archive-round.json \
  trace-replication .ariadne/trace-replication.json .ariadne/trace-replication-round.json
go run ./cmd/ariadne trace case verify --json .ariadne/case.json
go run ./cmd/ariadne trace case ask all --json .ariadne/case.json
go run ./cmd/ariadne trace case ask all save --json .ariadne/case.json .ariadne/case-round.json
go run ./cmd/ariadne trace case ask all verify --json .ariadne/case-round.json
go run ./cmd/ariadne trace case ask all compare --json .ariadne/first-case-round.json .ariadne/second-case-round.json
```

Each entry embeds either one verified `trace-archive` and its matching archive
question round, or one verified `trace-replication` ledger and its matching
replication question round. The case verifier recomputes every child artifact
and round identity, rejects duplicate child identities, and answers only from
the verified embedded summaries. The case has its own canonical SHA-256, and a
saved case question round has a separate identity bound to it.

The fixed questions ask which reviewed source/adapter boundaries are
represented, which replicated outcomes are retained, and whether any child
conclusion remains `unknown` or incompletely supported. Outcomes remain
separate from `evidence_state`: a retained `unknown` outcome is a valid
reflection result, not a malformed package. The package keeps caller order but
does not infer chronology, and it stores no source paths, target identifiers,
process arguments, or captured values. It is a durable reflection/index
boundary, not a database, capture runner, universal sniffer, cross-source
causal attribution engine, or natural-language question interface.

To compare two retained case question rounds, use the bounded caller-ordered
comparison:

```console
go run ./cmd/ariadne trace case ask all compare --json \
  .ariadne/first-case-round.json .ariadne/second-case-round.json
```

Both rounds are verified before comparison. The result is `same` or `changed`
and includes only fixed question IDs whose safe projections differ, together
with the round/case identities and bounded change kinds. Question `result`,
replicated child `outcome`, and `evidence_state` remain separate; a changed
projection is not a chronology, causal, improvement, or regression claim.
Different case identities are valid. Fixed answer reasons are integrity-checked
and are not emitted as change kinds; round hashes identify the complete
retained documents. Paths, target IDs, arguments, and captured values never
enter the comparison.

### Read-only trace-case review

Expose the verified package through the loopback review page when a
computer-use driver needs to inspect the joined reflection:

```console
go run ./cmd/ariadne experiment serve \
  --trace-case .ariadne/case.json \
  <archive-root>
```

Open `/trace-case` to see the case SHA-256, caller order basis, archive and
replication counts, safe reviewed source/adapter summaries, each child
artifact and question-round identity, and the three fixed case answers. The
fixed case answers use a question `result` such as `available`, `supported`,
or `unknown`; they are not replicated outcomes. Child entries remain in caller
order; the page does not describe entries as earlier or later. Replicated
child outcomes are rendered separately from their `evidence_state`, so
`unknown` remains missing support rather than a no-change claim. The route
re-verifies the full embedded package and recomputes answers
before rendering, accepts only `GET`, and returns only
`trace case unavailable` for malformed, tampered, or identity-inconsistent
input. It never renders the configured path, source-specific arguments,
target identifiers, URLs, or captured values. This is a read-only orientation
surface, not proof that a computer-use driver selected an answer or that
cross-source behavior is causal.

## Bigger-picture path

The intended flow is:

1. An authorized source-specific adapter captures within a declared scope.
2. The adapter redacts payloads and maps source details to the trace contract.
3. Ariadne verifies and compares the portable trace documents.
4. Replicated counterfactual experiments repeat the matched comparison in both
   orders before a new capture edge is added. Their aggregate outcome remains
   separate from the evidence state.
5. A caller-ordered trace archive retains normalized standalone snapshots and
   answers fixed coverage, change, and source questions across reviewed
   adapters.
6. A source-neutral replication ledger re-verifies already-produced matched
   pairs in both explicit orders, records caller reset assertions, and
   classifies the aggregate without adding a runner or capture adapter.
7. Existing evidence bundles retain the authoritative artifacts and bounded
   conclusions; the trace is a safe index for asking where a category appeared.
8. The isolated browser-target producer can add one explicitly authorized
   HTTPS origin to the same portable path without attaching to a user's
    existing profile or exposing hostnames and values.
9. The loopback proxy producer can add one explicitly authorized HTTPS
   authority through an opaque, non-MITM CONNECT boundary without retaining
   hostnames, URLs, or tunneled values.
10. The case package and its read-only `/trace-case` route can join those
    verified archives and replicated ledgers with their fixed question rounds,
    giving a computer-use driver one stable raw-value-free object to inspect
    and ask about without reopening source paths or inferring chronology.

The Experiment 001 Android producer remains the authoritative evidence-backed
edge, and the replicated runner is the evidence gate. The browser audit and
isolated browser-target producers now supply two safe browser boundaries; the
loopback proxy producer supplies one narrow process/network boundary. Session
provenance binds each trace to its reviewed procedure before sources are joined.
Desktop and broader proxy coverage remain separate slices requiring their own
authorization model, reproducible capture procedure, and redaction tests.
