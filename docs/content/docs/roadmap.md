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

The repository currently has five evidence-backed layers, two separate browser
source-boundary layers, and a portable trace reflection layer:

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
    not inferred chronology.

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
capture narrow. The browser audit producer and explicit driver protocol are
handoff slices: keep them redacted, while the deterministic comparison,
provenance, redaction, and question engine stay small and portable.
Session provenance is the join point for later source-specific runs. A real
browser capture driver for a user-authorized target, desktop producer, or proxy
producer is still a separate future slice with its own authorized procedure,
isolated target, and redaction tests; the local fixture is not that capability.
The trace archive is now the portable reflection surface for those reviewed
snapshots, but it is not a chronology model or universal sniffer. The trace
contract remains the handoff boundary for any future source producer.

## Acceptance gates

- Go changes pass formatting, build, vet, race-enabled tests, and at least 90%
  statement coverage.
- Fixture or runner changes pass the hosted real-emulator workflow when local
  Android tooling is unavailable.
- The local browser fixture producer and its two-order replication pass the
  hosted Windows Chrome workflow, including redaction and temporary-profile
  cleanup.
- Source-neutral trace archives verify their embedded standalone sessions and
  answer only the fixed coverage, change, and source questions; partial or
  incompatible boundaries stay `unknown`.
- UI changes pass deterministic handler tests; use computer-use only when a
  concrete rendered-flow check is needed and the host supports it.
- No review page, answer, export, or log exposes captured values, secrets, or
  unrelated personal data. Missing visibility remains `unknown` or
  `unavailable`.

Anything beyond these gates is a proposal, not first-year progress.
