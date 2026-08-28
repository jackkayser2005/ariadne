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
**Current security slice (Android input authentication).** The runner now writes
one bounded canonical fixture input through an app-private stdin boundary rather
than process arguments or exported activity extras. The debug fixture launcher
is protected by Android's `android.permission.DUMP`. A one-shot challenge binds
the package, role, explicit pair order, reviewed procedure digest, and trace
session; network and storage evidence must carry the same challenge. Session
receipts retain only a challenge commitment, and failed or unverifiable
boundaries remain incomplete/unknown. The first minimum-disclosure slice now
defines an ordered exact/city/omitted Android ladder, runs each lower-
disclosure candidate through the existing two-order engine, and emits a
raw-value-free receipt with separate candidate outcomes and evidence states.
Selection is withheld for any mixed or unknown candidate and is described only
as the minimum tested sufficient disclosure. The loopback review server now
accepts that verified minimization directory through `--minimization` and
renders a separate GET-only `/minimization` view. It projects only safe
candidate IDs, ladder order, counts, receipt identities, functionality
classifications, replicated outcomes, and evidence states; it re-verifies the
receipt and child replications on each request, binds the canonical configured loopback Host
authority, and uses no-store/security headers. This closes the first
observe/authenticate/reduce/replay/compare/verify slice into a usable local
inspection surface without turning the desktop page into a capture service.

**Current reflection slice (minimization questions).** The minimum-disclosure
ladder now has a fixed question catalog for selection and support. A verified
run can produce a portable question round, and a selected answer can be
retained as a receipt. The round projects only candidate IDs, classifications,
counterfactual outcomes, evidence states, counts, and child receipt
identities; it omits local paths and input values. Round and receipt hashes
bind the artifacts to the minimization identity, and the loopback page
rechecks any supplied saved artifacts before rendering them. This closes the
minimization path from verify to ask to retain to review while keeping
question result, counterfactual outcome, and evidence state separate. It
does not add cross-study comparison or universal tracing.

**Current browser minimization slice.** The source-neutral ladder contract
now separates candidate decisions and portable receipts from source
execution. The local browser adapter binds the reviewed `reference` and
`omitted` candidate IDs to a synthetic `account-id`, runs fresh-profile
pairs in both orders, and compares only
`all-non-disclosure-fields-equal-v1` for functionality. Ordinary trace
comparison still reports the account-id disclosure; minimization may call
the candidate sufficient only when non-disclosure fields remain observed and
equal. Receipts retain safe IDs, provenance, outcomes, evidence states,
counts, and child receipt identities, never values, URLs, driver arguments,
profiles, or captured events. This is a fixture handoff, not universal
tracing.

The same `--minimization` flag and `/minimization` route now accept this source-neutral receipt through the browser adapter's explicit criterion-specific verifier. The review projection adds safe adapter and procedure provenance. The same
source-neutral fixed question catalog now answers browser ladders, and the
browser adapter verifies the ladder before producing a question answer or
saved question round. Saved rounds and selected receipts remain shared,
raw-value-free artifacts; offline verification checks structure and identity
without reopening the browser source. This extends the verify -> ask -> retain
-> review path across the first two source boundaries without adding universal
tracing.

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
15. **Portable cross-source case package.** Verified trace archives and
     replicated ledgers can now be embedded, in caller order, with their
     matching fixed question rounds. The case verifier recomputes every child
     artifact and round identity, rejects duplicate child identities, and
     answers fixed questions about represented source boundaries, retained
     replicated outcomes, and unknown or incompletely supported conclusions.
     The case and its question round have separate canonical identities. The
     loopback review page can now expose the verified package through a
     separate GET-only `/trace-case` route, preserving caller order and
     rendering child identities, safe source summaries, fixed question
     results, replicated child outcomes, and separate evidence-state fields.
     Two retained case question rounds can also be verified and compared in
     caller order, reporting only bounded question projections that changed;
     this preserves the distinction between question results, replicated
     outcomes, evidence state, and the caller's supplied order.
     This is a durable
     reflection/index boundary, not chronology inference, a database, a
     capture runner, a universal sniffer, or cross-source causal attribution.
16. **Portable cross-run replication study.** Independent source-neutral
    replication ledgers can now be joined under a caller-supplied private
    counterfactual commitment, with each saved question round bound to its
    ledger identity. Study verification rejects duplicate artifacts and
    cross-run provenance drift, preserves caller order without chronology
    inference, and reports `replicated-change`, `no-change-observed`,
    `mixed-inconsistent`, or `unknown` separately from `evidence_state`.
    Any unsupported run keeps the aggregate `unknown`; this is a reproducible
    comparison boundary, not a reset runner, browser capture service,
    statistical model, or causal proof.

17. **Replication study review surface.** The fixed `study-outcome`,
    `study-support`, and `cross-run-consistency` questions now close the
    save -> verify -> ask -> review loop for a portable study. The CLI can
    list or answer them, and the GET-only `/trace-study` route renders the
    commitment, caller order, safe counts, aggregate/evidence-state split,
    and every embedded ledger/question-round identity without paths or
    captured values.
18. **Durable replication-study question artifacts.** The complete fixed
    answer set can now be saved as a raw-value-free question round, verified
    offline by its study and canonical SHA-256 identities, and reduced to one
    selected receipt that embeds the verified round. The loopback route can
    re-verify those artifacts and optionally select a bounded in-memory receipt
    by fixed question ID. Result, aggregate outcome, and evidence state stay
    separate; identity drift fails closed.

19. **Cross-study reflection comparison.** Two independently retained study
    question rounds can now be re-verified against their supplied studies and
    compared in caller order. Compatible investigations require the same
    private counterfactual commitment and reviewed source provenance. The
    bounded result is `same` or `changed`; valid incompatible boundaries return
    `incomparable`, with changed question IDs and only result, outcome,
    evidence-state, or support-count changes. This is a reflection comparison,
    not chronology, trend inference, authorization proof, or causal evidence.

20. **Cross-study reflection review surface.** The same verified comparison can
    now be exposed through a GET-only `/trace-study-comparison` page with
    fixed caller-order identities, separate result/outcome/evidence-state
    projections, and generic failure behavior. This is a read-only UI
    acceptance surface, not a capture service, chronology model, authorization
    proof, or causal claim.
21. **Cross-source disclosure map.** A verified case can now derive a
    deterministic, raw-value-free map grouped by reviewed data category. Each
    observation retains only source/adapter/channel/kind/destination labels, a
    retained-trace count, and `observed` evidence state. Aggregate
    `coverage_state` becomes `unknown` when any contributing trace is partial;
    the map never infers absence, correlates identities, or becomes a second
    persisted evidence store. The `trace case map` command and `/trace-case`
    projection make the practical question “where did this category appear?”
    inspectable across reviewed boundaries.
22. **Disclosure-map question rounds and receipts.** The map now has a fixed
    `disclosure-map-coverage` question and a
    `cross-boundary-category-overlap` question. Coverage is `complete` only with
    complete retained traces; positive overlap is `overlap-observed` with
    `observed` evidence even when aggregate coverage is unknown; no overlap
    under partial coverage is `unknown`. Complete no-overlap is
    `no-overlap-observed`. Saved rounds and selected receipts carry canonical
    case/round/receipt identities and safe category plus source/adapter
    boundary summaries, never values, event identifiers, URLs, paths, or
    arguments. Offline verification checks those supplied documents' schema,
    canonical identities, and internal binding without reopening source inputs;
    it does not authenticate the original case unless that case is separately
    verified.
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
The same page can expose a verified cross-source case through `/trace-case`;
that route is a bounded inspection surface for the caller-ordered child
artifacts and fixed case questions, not proof of UI selection or causality.
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

Use the two-order replicated Android experiment and its read-only minimization receipt review, together with the fixed local-browser
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
Browser fixture minimization is now a fixed local handoff and acceptance
gate: it binds explicit candidate IDs, a criterion-aware functionality
comparison, fresh profiles, both pair orders, and raw-value-free child
receipts. It does not broaden the project to user-browser capture or a
universal proxy.

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
capture store. The case package is now the reflection join: it preserves caller
order across verified archives and ledgers, answers only its fixed bounded
catalog, and retains `unknown` child conclusions without turning them into
causal attribution. Two retained case rounds can be compared before any UI
surface is added, with `same`/`changed` semantics and separate result,
outcome, and evidence-state fields. Its `/trace-case` route makes that package
inspectable by the read-only computer-use acceptance path, and its derived
cross-source disclosure map shows only reviewed category appearances and safe
retained-trace counts; its fixed questions and selected raw-value-free receipt
projection make that reflection reusable without exposing source inputs. Real
GUI execution remains a separate evidence gate.
The portable replication study now provides the next cross-run reflection
boundary: it joins independently identified ledgers and their fixed rounds
under one private commitment, while retaining unknown support instead of
promoting repeated outcomes into causal claims. Its fixed question round and
selected receipt now provide the durable handoff for asking those bounded
questions again without reopening source paths or captured values.

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
- Hosted browser fixture minimization verifies the fixed plan, explicit
  candidate binding, criterion-aware result, raw-value-free receipt, and
  unknown classification on partial or mismatched child evidence.
  The same hosted slice also exercises the shared GET-only `/minimization` review projection and checks that adapter/procedure provenance is visible without paths, values, URLs, driver arguments, profiles, or captured events.
- Cross-source case packages re-verify embedded archives or ledgers and their
  matching question rounds, reject duplicate child identities, preserve caller
  order without chronology inference, and keep retained outcomes separate from
  `evidence_state`.
- Derived case disclosure maps group only reviewed safe categories and
  destinations, count retained trace documents rather than sources, preserve
  observed labels separately from aggregate partial-coverage `unknown`, and
  never persist or render raw values, URLs, paths, arguments, or identities.
- Case-round comparisons re-verify both saved rounds and emit only bounded
  question projections; they do not treat a changed question result as a
  causal, chronological, improvement, or regression claim.
- Cross-run replication studies re-verify every embedded ledger and matching
  question round, reject duplicate identities and provenance drift, require
  balanced reset-confirmed orders in every run before reporting a supported
  aggregate, and keep unsupported outcomes separate from `evidence_state`.
  A private counterfactual commitment is an identity binding, not retained
  target data or proof of causality.
  The fixed study questions, `trace study ask all`, durable round/receipt
  save-and-verify commands, and GET-only `/trace-study` review route must retain
  the same outcome/evidence-state separation and fail closed without rendering
  study paths, payloads, URLs, or captured data.
- Study-round comparisons re-verify both supplied studies and rounds, require
  exact round-to-study answer binding plus matching private commitment and
  reviewed source provenance, and return `same`, `changed`, or
  `incomparable` without exposing paths or source values. Caller order remains
  non-chronological, and changed results remain separate from outcome and
  `evidence_state`.
- The study comparison review route requires both verified study/round pairs,
  is GET-only, renders only bounded comparison projections, escapes dynamic
  output, and fails closed without paths, payloads, URLs, captured values, or
  detailed errors.
- UI changes pass deterministic handler tests; use computer-use only when a
  concrete rendered-flow check is needed and the host supports it.
- No review page, answer, export, or log exposes captured values, secrets, or
  unrelated personal data. Missing visibility remains `unknown` or
  `unavailable`.

Anything beyond these gates is a proposal, not first-year progress.
