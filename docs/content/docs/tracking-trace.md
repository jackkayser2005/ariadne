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
writing this document. The current repository provides the contract and
verifier, not adapters for every browser, application, proxy, or desktop
runtime.

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

## Bigger-picture path

The intended flow is:

1. An authorized browser, Android, desktop, or proxy-specific adapter captures
   within a declared scope.
2. The adapter redacts payloads and maps source details to the trace contract.
3. Ariadne verifies and compares the portable trace documents.
4. Existing evidence bundles retain the authoritative artifacts and bounded
   conclusions; the trace is a safe index for asking where a category appeared.

Only the contract and CLI verifier are implemented today. A new adapter should
be added when there is an authorized target, a reproducible capture procedure,
and tests proving that its redaction boundary is safe.
