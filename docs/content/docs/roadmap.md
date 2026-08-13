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

The repository currently has three evidence-backed layers:

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
   destination, and data-category labels. Complete versus partial coverage is
   explicit, so structural absence remains `unknown` when a source did not
   claim complete capture.

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

Evaluate one additional authorized target or capture edge only after the
evidence and reflection contracts are stable. The first edge is now bounded UI
control provenance: a successful Android interaction records only the
hierarchy digest that located the declared resource ID. Target-specific runners
belong at the edges; the deterministic comparison, provenance, redaction, and
question engine stay small and portable. The source-neutral trace contract is
the handoff boundary for that future work; it does not itself add browser,
desktop, proxy, or universal network adapters.

## Acceptance gates

- Go changes pass formatting, build, vet, race-enabled tests, and at least 90%
  statement coverage.
- Fixture or runner changes pass the hosted real-emulator workflow when local
  Android tooling is unavailable.
- UI changes pass deterministic handler tests; use computer-use only when a
  concrete rendered-flow check is needed and the host supports it.
- No review page, answer, export, or log exposes captured values, secrets, or
  unrelated personal data. Missing visibility remains `unknown` or
  `unavailable`.

Anything beyond these gates is a proposal, not first-year progress.
