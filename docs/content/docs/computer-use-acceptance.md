---
title: Computer-use acceptance path
weight: 4
---

# Computer-use acceptance path

This is a read-only orientation check for the local review page. It proves
that a UI driver can choose one fixed historical question, read its bounded
receipt, and retain the displayed identities. It is not evidence about the
target application and must not retain screenshots or page content containing
captured values.

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

## Driver sequence

1. Open the printed loopback URL in the authorized local browser window.
2. Locate the region labelled `Verified history question round`.
3. Choose one link labelled `Ask verified history question <question-id>`.
4. Read the visible history SHA-256, question-round SHA-256, selected question
   ID, receipt SHA-256, and the raw-value-free receipt JSON.
5. Check that the selected receipt panel is labelled `Portable history answer
   receipt` and that its result is one of the fixed bounded states.
6. Check that `Portable question acceptance` reports `matched` after selecting
   the question named by the saved acceptance record.
7. Retain only the question ID and the three identities. Do not submit forms,
   follow mutation controls, or copy page text beyond the bounded receipt.

The page exposes accessible labels and stable receipt panel IDs for this
sequence. A driver may use those labels or the visible question IDs; it must
not invent natural-language questions or infer chronology from the ledger.

## Acceptance record

Record the round SHA-256, history SHA-256, selected question ID, receipt
SHA-256, acceptance SHA-256, and the verifier command exit status. The
acceptance artifact is a raw-value-free identity binding made after the round
and receipt have been checked; it is useful for retaining what the pass was
intended to select, but its verifier does not prove that a UI driver performed
the selection. The server-side `matched` status compares only the safe
identities; it is not UI proof. Mark the UI check `not run` if the computer-use host cannot
enumerate or activate a window. Deterministic handler tests and the offline
verifiers remain authoritative until a real rendered-flow check is available.
