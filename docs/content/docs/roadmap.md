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
   canonical content identity without reopening the source history.

The local review page is intentionally read-only. Its compact question round
and receipt links are a bounded UI-control surface for a computer-use driver;
deterministic UI tests remain the authoritative local check, and computer-use
is an orientation or acceptance aid rather than evidence about the target
application.

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
next focused slice after independently verifiable answer receipts is a
computer-use acceptance path for choosing fixed questions, reading their
receipts, and retaining their identities. Any new answer must remain bound to
a verified history and must say `unavailable` when the history lacks the
required schema.

### Months 10–12: expand only after the contract holds

Evaluate one additional authorized target or capture edge only after the
evidence and reflection contracts are stable. Target-specific runners belong at
the edges; the deterministic comparison, provenance, redaction, and question
engine stay small and portable.

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
