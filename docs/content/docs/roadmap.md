---
title: First-year roadmap
weight: 3
---

# First-year roadmap

Ariadne's first year is a sequence of trustworthy vertical slices, not a race
to add target adapters. The invariant is unchanged: change one declared value,
run the same authorized interaction, preserve the evidence path, normalize
only declared noise, and report bounded conclusions without exposing captured
values.

## Current position

The repository currently has five evidence-backed layers, several separate
browser source-boundary layers, and a portable trace reflection layer:

1. **Investigation core.** Experiment 001 defines personas, captures network
   and storage observations, validates provenance, compares answer states, and
   preserves `observed`, `inferred`, `claimed`, and `unknown` boundaries.
2. **Portable evidence.** Verified reports and redacted exports have stable
   identities, finding references, provenance context, and offline structural
   verification. Raw artifacts remain outside the review surface.
3. **Historical reflection.** Saved reflections can be compared in caller
   order, persisted as a contiguous transition ledger, and reviewed through
   fixed questions. The current catalog exposes transition changes, repeated
   safe entry changes, per-snapshot safe summaries, and summary changes across
   supplied boundaries. A verified history can also emit a portable question
   round containing the bounded result of every fixed question, or a portable
   per-question receipt containing its detailed raw-value-free answer. Saved
   receipts can be independently checked for fixed-question identity and
   canonical content identity without reopening the source history. The complete
   fixed question round can also be retained and independently checked by its
   canonical identity without reopening the source history. Two retained rounds
   can also be compared in caller order, returning only the fixed question IDs
   whose bounded results changed and the identities that bind both rounds to
   their histories. A raw-value-free acceptance record can now bind one retained
   round to one selected receipt after confirming their history, question, and
   bounded-result identities agree. The local review server can consume that
   record and report whether a selected fixed question matches those identities.
   It can also compare two retained question rounds in caller order, showing
   only fixed questions whose bounded results changed and both round/history
   identity pairs.
4. **Source-neutral tracking trace.** Authorized adapters can now exchange a
   strict, raw-value-free trace document containing logical source, channel,
   destination, and data-category labels. The first concrete producer now
   re-verifies an Experiment 001 Android session and projects its known network
   and private-storage paths into that contract. Complete versus partial
   coverage is explicit, so structural absence remains `unknown` when a source
   did not claim complete capture. A bounded session envelope now binds each
   trace to a current fixed adapter, reviewed procedure identity, and optional
   counterfactual role/order without adding captured values or claiming capture
   truth. Pair creation derives a canonical identity from both verified trace
   identities; pair verification requires complementary roles, separate trace
   paths, and matching provenance before two sessions are treated as one
   matched pair. Identical normalized trace content can therefore represent a
   valid no-change pair without weakening path or provenance checks.
   A provenance-aware pair comparison now joins that verified metadata to the
   structural trace result while preserving evidence states. An empty trace
   still leaves source corroboration unavailable.
5. **Replicated counterfactual experiments.** The Android runner can execute
   matched baseline/treatment pairs in both `baseline-treatment` and
   `treatment-baseline` order, resetting before every session and recording the
   order in a safe root receipt. The verifier aggregates complete pair results
   as `replicated-change`, `no-change-observed`, `mixed-inconsistent`, or
   `unknown`. That outcome is intentionally separate from the evidence model's
   `evidence_state`; an aggregate classification is not a causal proof. The
   verifier also rechecks each pair's authoritative evidence outputs and
   exposes only the root receipt and pair evidence SHA-256 identities.
6. **Browser audit handoff (not evidence-backed capture).** An authorized
   browser driver can provide a bounded, already-redacted audit that Ariadne
   projects into the same
   source-neutral trace contract. The producer accepts only reviewed network,
   cookie, and web-storage labels and remains explicit about partial coverage;
   it does not launch a browser or act as a universal sniffer.
7. **Browser driver protocol (not browser capture).** The CLI now invokes one
   explicitly selected executable without a shell, sends it a bounded
   raw-value-free procedure, caps stdout/stderr and runtime, and immediately
   validates the one redacted audit it returns. The procedure contains only a
   catalogued ID, scope, duration, and event limit; it cannot name a URL,
   profile, selector, script, header, payload, or authorization claim. This is
   the boundary for a future isolated driver, not evidence that a browser was
   captured.
8. **Local browser fixture producer (fixture-scoped).** A Node 22 CDP helper
   now launches an explicitly supplied Chrome executable with an ephemeral
   profile, serves a loopback-only fixture, and emits only fixed network labels
   after discarding URLs and values. It proves the process boundary and cleanup
   path against one deterministic target. It is not a user-session adapter or
   universal browser capture.
9. **Replicated local browser fixture (fixture-scoped).** The fixture runner
   now executes matched baseline/treatment sessions in both orders, supplies
   the fixed variant inside the driver boundary, creates a fresh profile before
   every session, and records a raw-value-free receipt. Verification binds each
   pair to portable trace sessions and classifies the aggregate as
   `replicated-change`, `no-change-observed`, `mixed-inconsistent`, or
   `unknown`, with `evidence_state` kept separate. This is a deterministic
   smoke path, not a user-browser adapter.
10. **Source-neutral trace archive questions.** Caller-ordered standalone
    sessions from reviewed adapters can now be retained in one portable,
    raw-value-free archive. Fixed questions report complete versus partial
    coverage, safe category change across compatible adjacent snapshots, and
    represented source adapters. Every answer is bound to the archive identity;
    partial or incompatible boundaries remain `unknown`, and archive order is
    not inferred chronology. The loopback review page can now expose the same
    archive through a separate `/trace-archive` route, with one verified
    in-memory question round and separate outcome/evidence-state rendering.
    The fixed round can also be saved and independently verified, with a
    selected raw-value-free receipt bound to both archive and round identities.
    The route can consume that saved round through `--trace-round`; when both
    live and saved inputs are supplied, identity drift fails closed.
11. **Source-neutral replicated trace ledger.** Already-produced matched trace
    sessions can now be embedded in a portable ledger with caller-recorded
    reset assertions and explicit `baseline-treatment` and
    `treatment-baseline` orders. Verification rechecks embedded session and
    trace identities, recomputes each comparison, requires equal nonzero order
    counts, and classifies the aggregate as `replicated-change`,
    `no-change-observed`, `mixed-inconsistent`, or `unknown` separately from
    `evidence_state`. Its fixed outcome, support, and order-consistency
    questions can be answered directly or retained as an independently
    verifiable question round and selected receipt. The loopback review page
    exposes the same fixed questions alongside the safe aggregate and pair
    identities through `/trace-replication`. This is an aggregation and
    re-verification boundary, not a runner, capture adapter, chronology model,
    or causal proof.
12. **Isolated authorized browser target producer.** A `browser-target-v1`
    procedure can name one canonical HTTPS origin and bind that origin into the
    procedure identity. The Node 22 driver launches a fresh Chromium profile,
    allows only that hostname through its resolver boundary, blocks requests
    whose URL origin is not exactly the declared origin, observes bounded
    page-load network metadata, maps only fixed field labels, and discards
    URLs, values, cookies, storage, DOM, headers, and bodies. Unsupported or
    blocked activity remains partial and therefore `unknown`. This is a
    single-origin producer, not a browser-history reader, existing-profile
     observer, universal sniffer, or proof of authorization.
13. **Loopback non-MITM proxy producer.** A `proxy-connect-v1` procedure can
     name one canonical lower-case DNS authority and bind that authority into
     the procedure identity. Ariadne launches one explicitly supplied,
     proxy-aware executable without a shell, gives it a fresh authenticated
     loopback proxy, accepts only that authority over `CONNECT`, and relays
     encrypted bytes without TLS interception. It retains only a partial,
     raw-value-free proxy network event; plaintext HTTP, other authorities, IP
     literals, malformed input, and over-limit requests are rejected. This is
     one authorized process boundary, not universal network tracing or proof of
     authorization.
14. **Replicated proxy process boundary.** The proxy runner now executes one
     executable with shared arguments and one final controlled baseline or
     treatment argument, in both explicit orders. Every session creates a new
     process, loopback proxy, and credential; the receipt retains only safe
     executable, order, completion, and provenance identities, while omitting
     the deterministic procedure digest from the root receipt.
     Verification recomputes complete pairs and classifies the aggregate as
     `replicated-change`, `no-change-observed`, `mixed-inconsistent`, or
     `unknown` separately from `evidence_state`. Partial proxy visibility can
     preserve an observed same outcome while reporting unknown evidence; an
     absent event remains unknown. This asserts only process/proxy reset, not
     remote-state reset, authorization, universal tracing, or causality.

The local review page is intentionally read-only. Its compact question round
and receipt links, including stable history and receipt identities, are a
bounded UI-control surface for a computer-use driver;
deterministic UI tests remain the authoritative local check, and computer-use
is an orientation or acceptance aid rather than evidence about the target
application. The authorized Android runner also retains a SHA-256 identity of
the successful UI hierarchy used to resolve its declared control, without
retaining raw hierarchy XML; this is control provenance, not target evidence.
The page also exposes the stable question-round identity, so an acceptance pass
can retain both the round and the selected answer receipt identities. The
acceptance record preserves that identity binding, but it is not proof that a
computer-use driver performed the selection.
The concrete read-only sequence is documented in
[`computer-use-acceptance.md`](computer-use-acceptance.md).

## Year-one path

### Months 1–3: prove the evidence chain

Keep one authorized Android investigation reproducible. Preserve hostile-input
validation, explicit unknown states, declared volatile-field normalization,
stable finding identities, and real-emulator CI evidence.

### Months 4–6: make evidence portable and inspectable

Keep reports and exports raw-value-free while carrying enough verifier-derived
provenance to explain what was checked. Add no database or hosted control
plane; local bundles remain authoritative.

### Months 7–9: make reflection repeatable

Turn the historical reflection spine into a durable question workflow. The
fixed question round and individual answer receipts are now independently
retainable and verifiable. The raw-value-free acceptance record now binds a
retained round and selected receipt for offline handoff. The remaining focused
slice has now been exercised in an authorized Windows Chrome smoke pass for
choosing a fixed question, reading its receipt, and checking the matched
identity status. That pass validates the rendered route only; any new answer
must remain bound to a verified history and must say
`unavailable` when the history lacks the required schema.

### Months 10–12: expand only after the contract holds

Use the two-order replicated Android experiment and the fixed local-browser
fixture as evidence gates. They test whether a reported change survives
repeated resets and reversed session order while keeping source-specific
capture narrow. The browser audit producer, explicit driver protocol,
isolated single-origin target producer, loopback proxy producer, and replicated
proxy process boundary are handoff slices: keep them redacted, while the
deterministic comparison, provenance, redaction, and question engine stay small
and portable. Retain
trace question rounds and selected receipts only as bounded identities, and
use the source-neutral replication ledger to re-verify already-produced pairs,
not as a new capture store.
Session provenance is the join point for later source-specific runs. The
isolated browser-target producer is now one narrow user-authorized target
slice, and the proxy producer is one narrow user-authorized authority slice;
neither proves target behavior, authorization, or capture truth. Desktop and
broader proxy coverage remain separate future slices with their own
authorization model, isolated target, and redaction tests; the local fixture,
target producer, and proxy producer are not universal coverage.
The trace archive is now the portable reflection surface for those reviewed
snapshots, but it is not a chronology model or universal sniffer. The trace
contract remains the handoff boundary for any future source producer. The
replication question round is now the durable reflection surface for asking
what the repeated comparison established without turning the ledger into a
capture store.

## Acceptance gates

- Go changes pass formatting, build, vet, race-enabled tests, and at least 90%
  statement coverage.
- Fixture or runner changes pass the hosted real-emulator workflow when local
  Android tooling is unavailable.
- The local browser fixture producer and its two-order replication pass the
  hosted Windows Chrome workflow, including redaction and temporary-profile
  cleanup.
- The browser-target procedure and driver pass syntax, canonical-origin,
  isolated-profile, resolver-boundary, bounded-network, and raw-value redaction
  checks. A local test must not substitute for authorization or target behavior.
- The proxy-connect procedure and producer pass syntax, canonical-authority,
  loopback-authentication, CONNECT-only, opaque-relay, bounded-process, and
  raw-value redaction checks. A local test must not substitute for authorization
  or target behavior.
- The proxy replication runner passes both-order, fresh-process/proxy reset,
  safe-receipt, incomplete-run, provenance-tamper, aggregate-classification,
  and separate outcome/evidence-state checks. It must not claim remote-state
  reset or retain process arguments, condition values, authority, credentials,
  or tunnel data.
- Source-neutral trace archives verify their embedded standalone sessions and
  answer only the fixed coverage, change, and source questions; partial or
  incompatible boundaries stay `unknown`. Saved question rounds and selected
  receipts verify independently without reopening the source archive.
- Source-neutral replication ledgers verify embedded matched sessions and
  comparisons, retain both explicit pair orders and reset assertions, and
  require balanced orders before reporting a non-`unknown` aggregate. Their
  fixed question rounds and selected receipts verify independently while
  preserving outcome and evidence-state semantics.
- UI changes pass deterministic handler tests; use computer-use only when a
  concrete rendered-flow check is needed and the host supports it.
- No review page, answer, export, or log exposes captured values, secrets, or
  unrelated personal data. Missing visibility remains `unknown` or
  `unavailable`.

Anything beyond these gates is a proposal, not first-year progress.
