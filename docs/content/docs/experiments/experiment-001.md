---
title: Experiment 001
weight: 1
---

## Question

Can Ariadne prove that changing one persona value influences an observable
output from an authorized Android test application?

## Controlled variable

Two personas will differ in exactly one declared value. The specific value will
be selected with the fixture application so that the expected influence is
known without being hard-coded into Ariadne.

## Manifest v1

The first manifest is intentionally flat:

```json
{
  "schema_version": 1,
  "name": "experiment-001-email",
  "variable": "email",
  "baseline": {
    "email": "baseline@example.invalid",
    "region": "us-east"
  },
  "treatment": {
    "email": "treatment@example.invalid",
    "region": "us-east"
  }
}
```

Both personas must contain the same string keys. Exactly one value must differ,
and its key must equal `variable`. Nested values and non-string persona values
are outside v1.

The parser will reject inputs larger than 64 KiB, duplicate JSON keys, unknown
top-level fields, and trailing data. Validation errors may name fields but must
never include persona values.

## Validate a manifest

Run:

```console
go run ./cmd/ariadne validate examples/experiment-001.json
```

Successful validation prints stable metadata without displaying persona values:

```text
valid manifest
name: experiment-001-email
schema_version: 1
variable: email
persona_fields: 2
```

## Android target preflight

Before a session, verify one explicitly selected device and package:

```console
go run ./cmd/ariadne android check --device emulator-5554 --package dev.ariadne.fixture
```

Use `--adb <path>` when `adb` is not on `PATH`. A successful check prints:

```text
android target ready
adb_version: 1.0.41
device: emulator-5554
package: dev.ariadne.fixture
```

The check does not enumerate devices, inspect package data, or run the
experiment.

## Authorized fixture

The fixture package is `dev.ariadne.fixture`. Its exported activity accepts
`email` and `region` string extras, writes `files/observation.json`, and exits.
It requests no Android permissions and sends no network traffic.

For the example manifest, the stored `variant` is `standard` for the baseline
email and `personalized` for the treatment email. Ariadne does not contain this
rule.

## Run isolated sessions

After installing the fixture, run:

```console
go run ./cmd/ariadne experiment run --device emulator-5554 --package dev.ariadne.fixture --output .ariadne/runs/experiment-001 examples/experiment-001.json
```

The output directory must not already exist. Ariadne clears the selected
package before each session, starts `.MainActivity` with the persona fields,
and writes:

```text
.ariadne/runs/experiment-001/
├── baseline/
│   ├── observations/storage.json
│   └── session.json
└── treatment/
    ├── observations/storage.json
    └── session.json
```

Session metadata includes the selected device, package, ADB version, timestamps,
step status, exit codes, and a SHA-256 record for each captured artifact. It
excludes persona values, command arguments, and raw ADB output.

The storage artifact is the exact bounded JSON read from the fixture's private
`files/observation.json` through Android's `run-as` command. Capture fails if
the package is not debuggable, the file is missing, the output is not a JSON
object, or it exceeds 64 KiB.

## Procedure

1. Verify the selected emulator, package, and fixture version.
2. Reset the application to a known state.
3. Inject the baseline persona and execute the scripted interaction.
4. Capture authorized network and app-storage observations.
5. Repeat from the same known state with the treatment persona.
6. Normalize declared volatile fields.
7. Compare observations and generate an evidence bundle.

## Success criteria

- One expected persona-dependent difference is reported.
- Known timestamp or identifier noise is not reported as causal.
- Every finding links to raw evidence.
- Capture gaps appear as unknowns.
- A new contributor can reproduce the result.

## Non-goals

- Testing third-party applications without authorization
- Defeating certificate pinning
- Running a general-purpose Android malware sandbox
- Supporting multiple capture backends before the first experiment works
