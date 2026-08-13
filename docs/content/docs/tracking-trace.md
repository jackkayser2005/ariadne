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
roles, distinct trace identities, and matching adapter, procedure, scope, order,
and canonical pair identities. An empty trace can still be valid, but its
adapter source is an assertion rather than an event-source corroboration.

## Bigger-picture path

The intended flow is:

1. An authorized source-specific adapter captures within a declared scope.
2. The adapter redacts payloads and maps source details to the trace contract.
3. Ariadne verifies and compares the portable trace documents.
4. Replicated counterfactual experiments repeat the matched comparison in both
   orders before a new capture edge is added. Their aggregate outcome remains
   separate from the evidence state.
5. Existing evidence bundles retain the authoritative artifacts and bounded
   conclusions; the trace is a safe index for asking where a category appeared.

The Experiment 001 Android producer remains the authoritative evidence-backed
edge, and the replicated runner is the evidence gate. The browser audit
producer now supplies the next safe handoff boundary, and session provenance
binds each trace to its reviewed procedure before sources are joined. A real
browser capture driver, then desktop and proxy producers, remain separate
slices requiring an authorized target, a reproducible capture procedure, and
tests proving that the redaction boundary is safe.
