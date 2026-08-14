---
title: Computer-use acceptance path
weight: 4
---

# Computer-use acceptance path

This is a read-only orientation check for the local review page. It proves
that a UI driver can choose one fixed historical question, read its bounded
receipt, and retain the displayed identities. It is not evidence about the
target application and must not retain screenshots or page content containing
captured values. An optional trace-archive pass below follows the same rule
for source-neutral trace reflections.

## Prepare the bounded artifacts

Create and independently verify the fixed question round and one selected
answer receipt before opening the page:

```console
go run ./cmd/ariadne experiment ask-archive transitions ask all save --json <history.json> <round.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all verify --json --expect-sha256 <round-sha256> <round.json>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt save --json <history.json> <question-id> <receipt.json>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt verify --json --expect-sha256 <receipt-sha256> <receipt.json>
go run ./cmd/ariadne experiment ask-archive transitions acceptance save --json <round.json> <receipt.json> <acceptance.json>
go run ./cmd/ariadne experiment ask-archive transitions acceptance verify --json --expect-sha256 <acceptance-sha256> <acceptance.json>
```

Start the local review page with the same verified history:

```console
go run ./cmd/ariadne experiment serve --history <history.json> --acceptance <acceptance.json> <archive-root>
```

The server accepts loopback addresses only and is read-only.
To include a comparison of two retained rounds, supply both optional paths:

```console
go run ./cmd/ariadne experiment serve --history <history.json> --acceptance <acceptance.json> --round-first <first-round.json> --round-second <second-round.json> <archive-root>
```

To expose the separate source-neutral trace reflection route, add the verified
portable archive:

```console
go run ./cmd/ariadne experiment serve --trace-archive <trace-archive.json> <archive-root>
```

Or point the same read-only route at a previously verified question round:

```console
go run ./cmd/ariadne experiment serve --trace-round <trace-round.json> <archive-root>
```

This adds a link to `/trace-archive`; it does not change the bundle question
route or merge trace identities into the evidence archive. When both flags are
supplied, the route fails closed unless the live archive and saved question
round identities agree.

## Driver sequence

1. Open the printed loopback URL in the authorized local browser window.
2. Locate the region labelled `Verified history question round`.
3. If `Portable question acceptance` exposes `Ask accepted history question
   <question-id>`, follow that link to select the saved bound question; it is
   shown only when the supplied history identity matches the saved record.
   Otherwise, choose one link labelled `Ask verified history question
   <question-id>`.
4. Read the visible history SHA-256, question-round SHA-256, selected question
   ID, receipt SHA-256, and the raw-value-free receipt JSON.
5. Check that the selected receipt panel is labelled `Portable history answer
   receipt` and that its result is one of the fixed bounded states.
6. Check that `Portable question acceptance` reports `matched` after selecting
   the question named by the saved acceptance record.
7. If retained-round comparison was enabled, inspect `Retained question
   rounds` for bounded result changes and both round identities; preserve the
   caller-supplied order and do not infer chronology. If a changed question is
   present and the supplied history identity matches one of the compared rounds,
   use its `Ask changed retained question <question-id>` link to re-ask that
   fixed question through the same bounded route.
8. Retain only the question ID and the three identities. Do not submit forms,
   follow mutation controls, or copy page text beyond the bounded receipt.

### Optional trace-archive pass

1. From the review index, open `Open trace archive review`.
2. Check `Verified archive identity`, the `caller` order basis, entry and
   complete/partial counts, and the archive SHA-256. If a saved round is
   configured, retain its question-round SHA-256 as well.
3. Check `Reviewed source adapters`, then read all three fixed question IDs:
   `trace-coverage`, `trace-change`, and `trace-sources`.
4. For each question, retain the displayed `outcome` and its separate
   `evidence state`. Treat `unknown` as missing support, not as `same` or
   `no-change-observed`.
5. Do not invent a question, infer chronology from caller order, or retain
   local archive paths or captured values. A failed verification should show
   only `trace archive unavailable`.

The page exposes accessible labels and stable receipt panel IDs for this
sequence. A driver may use those labels or the visible question IDs; it must
not invent natural-language questions or infer chronology from the ledger.

## Recorded rendered-flow check

On 2026-08-10, an authorized Windows Chrome smoke pass opened the loopback
review page from a synthetic raw-value-free transition history and acceptance
record, followed `Ask accepted history question answer-state-transitions`, and
observed `MATCHED` with the raw-value-free receipt and stable identities
visible. No mutation control was used. This proves the rendered route,
accessible label, and read-only receipt flow; it does not prove the target
application or the underlying evidence. The real Android capture workflow
remains the authority for target-specific evidence.

## Acceptance record

Record the round SHA-256, history SHA-256, selected question ID, receipt
SHA-256, acceptance SHA-256, and the verifier command exit status. The
acceptance artifact is a raw-value-free identity binding made after the round
and receipt have been checked; it is useful for retaining what the pass was
intended to select, but its verifier does not prove that a UI driver performed
the selection. The server-side `matched` status compares only the safe
identities; it is not UI proof. The optional retained-round panel compares
fixed bounded results only; it is not a trend or chronology claim. Mark the UI
check `not run` if the computer-use host cannot
enumerate or activate a window. Deterministic handler tests and the offline
verifiers remain authoritative alongside the recorded rendered-flow check; a
new host or build should repeat the UI check before relying on its result.
