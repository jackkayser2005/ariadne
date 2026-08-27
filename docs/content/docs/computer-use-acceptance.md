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

To review an already-verified replicated trace ledger, use the separate
source-neutral route:

```console
go run ./cmd/ariadne experiment serve --trace-replication <trace-replication.json> <archive-root>
```

To review a verified cross-source case package, use the same read-only server:

```console
go run ./cmd/ariadne experiment serve --trace-case <case.json> <archive-root>
```
To review a verified portable replication study, use the separate read-only
study route:

```console
go run ./cmd/ariadne experiment serve --trace-study <study.json> <archive-root>
```

Before opening the study route, the complete fixed answer set and one selected
answer can be retained and independently verified without reopening source
paths or captured values:

```console
go run ./cmd/ariadne trace study ask all save --json <study.json> <round.json>
go run ./cmd/ariadne trace study ask all verify --json --expect-sha256 <round-sha256> <round.json>
go run ./cmd/ariadne trace study ask receipt save --json <round.json> <question-id> <receipt.json>
go run ./cmd/ariadne trace study ask receipt verify --json --expect-sha256 <receipt-sha256> <receipt.json>
```

To expose those verified artifacts in the same read-only route:

```console
go run ./cmd/ariadne experiment serve --trace-study <study.json> --trace-study-round <round.json> --trace-study-receipt <receipt.json> <archive-root>
```


To expose the same read-only surface for two retained study reflections, supply
both study/round pairs:

```console
go run ./cmd/ariadne experiment serve \
  --trace-study <first-study.json> \
  --trace-study-round <first-round.json> \
  --trace-study-second <second-study.json> \
  --trace-study-round-second <second-round.json> \
  <archive-root>
```
These options add links to the separate trace review routes; they do not
change the bundle question route or merge trace identities into the evidence
archive. When both archive flags are supplied, `/trace-archive` fails closed
unless the live archive and saved question round identities agree.

## Minimum-disclosure review

After a verified Android minimization run, expose its raw-value-free receipt in
that same loopback server:

```console
go run ./cmd/ariadne experiment minimize verify --json <minimization-directory>
go run ./cmd/ariadne experiment serve \
  --minimization <minimization-directory> \
  <archive-root>
```

Open `/minimization` from the printed loopback authority. Confirm the page
shows the candidate IDs in recorded ladder order, the selected candidate (only
when the selection state is `selected`), the root receipt SHA-256, and separate
functionality classification, counterfactual outcome, and evidence-state
fields. A mixed, incomplete, or unknown candidate must leave the selection
unestablished. The page is a read-only projection: it never renders plan
values, personas, manifests, local paths, URLs, challenges, or captured
observations. Computer-use may orient to this page, but the verifier and
its deterministic tests remain the authoritative checks.
To retain the fixed minimization reflection and expose the durable identities:

~~~console
go run ./cmd/ariadne experiment minimize ask all save --json <minimization-directory> <round.json>
go run ./cmd/ariadne experiment minimize ask all verify --json --expect-sha256 <round-sha256> <round.json>
go run ./cmd/ariadne experiment minimize ask receipt save --json <round.json> <question-id> <receipt.json>
go run ./cmd/ariadne experiment minimize ask receipt verify --json --expect-sha256 <receipt-sha256> <receipt.json>
go run ./cmd/ariadne experiment serve --minimization <minimization-directory> --minimization-round <round.json> --minimization-receipt <receipt.json> <archive-root>
~~~

The page then shows the two fixed question cards, the saved round identity,
and the selected receipt identity. A question result remains separate from
its evidence state, and the candidate ladder remains the source of
counterfactual outcomes. If the current minimization receipt, saved round,
or selected receipt disagrees, the page returns a generic unavailable state.
The round and receipt artifacts are raw-value-free and are verified
structurally; computer-use must not infer a privacy assurance from their
presence alone.

## Browser fixture minimization handoff

The browser minimization slice is a CLI plus a read-only review handoff, not a
computer-use capture surface. Run and verify the bounded fixture before opening
any read-only review page:

```console
go run ./cmd/ariadne browser fixture minimize --json \
  --plan examples/browser-account-minimize.json \
  --procedure examples/browser-local-fixture-procedure.json \
  --driver node \
  --driver-arg cmd/browser-fixture-driver/browser_fixture_driver.mjs \
  --driver-arg --browser \
  --driver-arg "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" \
  --pairs 1 \
  --output .ariadne/browser-account-minimize
go run ./cmd/ariadne browser fixture minimize verify --json \
  .ariadne/browser-account-minimize
```

Confirm the verified receipt contains only the fixed candidate IDs,
criterion, pair counts, outcomes, evidence states, provenance, and child
receipt identities. The receipt must not contain the synthetic value, URLs,
driver arguments, profile paths, or captured events. A partial, failed, or
provenance-mismatched child is `unknown`; do not use computer-use to infer
sufficiency from an incomplete run.

Expose the verified browser ladder through the existing minimization review route:

```console
go run ./cmd/ariadne experiment serve --minimization .ariadne/browser-account-minimize <archive-root>
```

Open `/minimization` from the printed loopback authority. Confirm that adapter, procedure SHA-256, scope, reset policy, candidate order, classification, counterfactual outcome, and evidence state are visible. The page must not render candidate values, local paths, URLs, driver arguments, profiles, or captured events. Browser receipts do not expose the legacy Android question cards until a source-neutral question catalog exists.

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

### Optional replicated-trace pass

1. From the review index, open `Open replicated trace review`.
2. Check the aggregate outcome and its separate `evidence state`, both explicit
   order counts, reset-confirmed count, order-balance status, and ledger
   SHA-256.
3. Read the three fixed question cards: `replication-outcome`,
   `replication-support`, and `replication-consistency`. Retain the displayed
   result and its separate evidence state; do not treat `unknown` as no change.
4. Inspect the safe pair cards for caller-recorded order, reset assertion,
   pair SHA-256, completeness, differences, and unknowns. Treat `unknown` as
   missing support, not as no change.
5. Do not retain the configured ledger path or infer that the recorded reset
   proves a source reset. A failed verification should show only
   `trace replication unavailable`.

The same three answers can be retained before opening the page with
`trace replication ask all save`, and a selected answer can be retained with
`trace replication ask receipt save`. Verify the round and receipt SHA-256
identities offline before the rendered check; those artifacts are identities
and bounded answers, not proof that a UI driver performed the selection.

### Optional cross-source case pass

1. From the review index, open `Open trace case review`.
2. Check `Verified case identity`, the `caller` order basis, archive and
   replicated-ledger counts, unknown-entry count, and the case SHA-256.
3. Check the safe reviewed source/adapter summaries and the caller-ordered
   child entries. Do not describe their positions as earlier or later.
4. Read the three fixed question IDs: `case-sources`, `case-outcomes`, and
   `case-support`. For each, retain the displayed result and its separate
   evidence state. Treat `unknown` as missing support, not as no change or
   cross-source causality.
5. Inspect child artifact and matching question-round identities only. Do not
   retain the configured case path, target identifiers, process arguments,
   URLs, or captured values. A failed verification should show only
   `trace case unavailable`.

### Optional replication-study pass

1. From the review index, open `Open replication study review`.
2. Check `Verified replication study identity`, the private commitment SHA-256,
   `caller` order basis, run/pair/support counts, aggregate outcome, separate
   `evidence state`, and study SHA-256.
3. Read the three fixed question IDs: `study-outcome`, `study-support`, and
   `cross-run-consistency`. Retain each displayed `result`, aggregate
   `outcome`, and separate `evidence_state`; treat `unknown` as missing
   support, not as no change.
4. If a durable round is configured, check its study SHA-256, round SHA-256,
   answer count, and fixed-question identities. If a durable receipt is
   configured, check its selected question ID plus study, round, and receipt
   SHA-256 identities. A fixed `?question_id=` may select one bounded
   in-memory receipt when no saved receipt is supplied.
5. Inspect each independent run's ledger and question-round SHA-256 only.
   Do not describe caller positions as chronology or retain local paths,
   payloads, URLs, or captured values. A failed verification should show only
   `trace study unavailable`.

### Optional replication-study comparison pass

1. From the review index, open `Open retained study comparison`.
2. Check `Verified study comparison`, the comparison result, the valid-result
   labels, caller order, both study/round identities, and compared/changed
   counts.
3. Inspect changed question cards only. Keep first/second `result`, aggregate
   `outcome`, separate `evidence_state`, and fixed change kinds distinct.
4. Do not retain configured paths, raw data, payloads, URLs, or captured values;
   do not infer chronology, trend, authorization, or causality. A failed
   verification should show only `trace study comparison unavailable`.
This pass checks the rendered, read-only reflection surface only. It does not
claim that a computer-use driver selected an answer, that caller order is
chronology, or that retained results establish causal behavior across sources.

The isolated `browser-target-v1` producer is not part of this rendered
acceptance pass. If it is used separately, keep the target procedure and
browser run outside the review page, retain only the procedure and trace
identities, and do not open an external target in a computer-use acceptance
session. Its HTTPS origin is an authorization boundary, not evidence that the
target or the capture was authorized.

The `proxy-connect-v1` producer is also outside this rendered acceptance pass.
If it is used separately, keep the explicitly supplied process, proxy
procedure, and target run outside the review page, retain only the reviewed
procedure and trace identities, and do not treat the loopback proxy as proof of
authorization, capture truth, or target behavior. Its canonical authority is a
scope boundary, not a retained hostname or a reason to expose tunneled data.

The `proxy replicate` runner is outside this rendered acceptance pass as well.
Verify its receipt from the command line, inspect the two explicit orders and
the separate aggregate/evidence-state results, and retain only the procedure,
executable, receipt, and pair identities. Its fresh process/proxy reset does
not reset remote state, and an `unknown` result must not be relabeled as no
change merely because the UI is available.

## Recorded rendered-flow check

On 2026-08-10, an authorized Windows Chrome smoke pass opened the loopback
review page from a synthetic raw-value-free transition history and acceptance
record, followed `Ask accepted history question answer-state-transitions`, and
observed `MATCHED` with the raw-value-free receipt and stable identities
visible. No mutation control was used. This proves the rendered route,
accessible label, and read-only receipt flow; it does not prove the target
application or the underlying evidence. The real Android capture workflow
remains the authority for target-specific evidence. That workflow now delivers
personas and the collector port through a bounded app-private one-shot input,
not activity extras; the fixture deletes it after consumption. Its session
challenge is checked against both local evidence channels, while only the
commitment is retained in the session receipt. A failed challenge check is
incomplete/unknown, never a privacy assurance.

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
