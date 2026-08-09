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

## Manifest

The first manifest is intentionally flat:

```json
{
  "schema_version": 3,
  "name": "experiment-001-email",
  "variable": "email",
  "volatile_fields": [
    "request_id"
  ],
  "tap_resource_id": "dev.ariadne.fixture:id/observe_button",
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
are not supported by either current manifest schema.

The parser will reject inputs larger than 64 KiB, duplicate JSON keys, unknown
top-level fields, and trailing data. Validation errors may name fields but must
never include persona values.

Manifest schema 2 adds an optional `volatile_fields` array of unique observation
field names. Schema 3 adds the required `tap_resource_id` stable resource ID for
one authorized fixture action. Schema 1 and 2 remain accepted for launch-only
sessions. The list is limited to 64 names and is stored in sorted order with
each session. Resource IDs are bounded ASCII identifiers; coordinates are never
stored in the manifest.

## Validate a manifest

Run:

```console
go run ./cmd/ariadne validate examples/experiment-001.json
```

Successful validation prints stable metadata without displaying persona values:

```text
valid manifest
name: experiment-001-email
schema_version: 3
variable: email
persona_fields: 2
manifest_contract_sha256: <64 lowercase hexadecimal characters>
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
android_api: 35
architecture: x86_64
package: dev.ariadne.fixture
package_version_code: 1
package_sha256: <64 lowercase hexadecimal characters>
ariadne_revision: <Git commit or unknown>
ariadne_modified: false
```

The check does not enumerate devices or run the experiment. It reads the
selected device's API and primary ABI, reads the selected package's version
code, and streams its installed base APK into SHA-256. The APK is not retained
or logged. The current check rejects split APK sets and installed APKs larger
than 256 MiB.

## Authorized fixture

The fixture package is `dev.ariadne.fixture`. Its exported activity accepts
`email` and `region` string extras, renders the `observe_button` control, and
waits for that control before writing `files/observation.json` and exiting. The
runner resolves the exact declared resource ID from a bounded UI hierarchy and
taps its center; it does not use coordinates from the manifest or arbitrary
shell commands. The activity requests Android's normal `INTERNET` permission.
It sends no request unless the runner supplies `collector_port`; when supplied,
it posts the same JSON to IPv4 loopback. Cleartext traffic to other destinations
is denied.

For the example manifest, the stored `variant` is `standard` for the baseline
email and `personalized` for the treatment email. Ariadne does not contain this
rule. The fixture also generates a fresh `request_id` for every session and
uses the same observation bytes for storage and network capture.

## Reproduce from a fresh checkout

Requirements:

- Go 1.24 or newer
- JDK 17
- Android SDK Platform 36, Build Tools 36.0.0, and `adb` on `PATH`
- one running API 35 Google APIs emulator

Run all commands from the repository root. Replace `emulator-5554` with the
serial of the emulator you explicitly selected. Android Studio's Device Manager
can create and start the emulator; `adb devices` shows its serial. Ariadne does
not enumerate devices. The GitHub workflow uses x86_64, while a local emulator
may use its host's native ABI; the selected ABI is recorded in the evidence.

Build and test the fixture on Windows:

```console
fixture\android\gradlew.bat --no-daemon --project-dir fixture\android lintDebug testDebugUnitTest createDebugUnitTestCoverageReport assembleDebug
```

On Linux or macOS:

```console
chmod +x fixture/android/gradlew
fixture/android/gradlew --no-daemon --project-dir fixture/android lintDebug testDebugUnitTest createDebugUnitTestCoverageReport assembleDebug
```

Install the resulting debug APK:

```console
adb -s emulator-5554 install -r fixture/android/app/build/outputs/apk/debug/app-debug.apk
```

Validate the manifest, verify the selected target, and execute the experiment:

```console
go run ./cmd/ariadne validate examples/experiment-001.json
go run ./cmd/ariadne android check --device emulator-5554 --package dev.ariadne.fixture
go run ./cmd/ariadne experiment run --device emulator-5554 --package dev.ariadne.fixture --output .ariadne/runs/experiment-001 examples/experiment-001.json
go run ./cmd/ariadne experiment report .ariadne/runs/experiment-001
go run ./cmd/ariadne experiment verify .ariadne/runs/experiment-001
go run ./cmd/ariadne experiment verify --json .ariadne/runs/experiment-001
go run ./cmd/ariadne experiment list --json .ariadne/runs
go run ./cmd/ariadne experiment serve --addr 127.0.0.1:8787 .ariadne/runs
go run ./cmd/ariadne experiment finding .ariadne/runs/experiment-001 <finding-id-from-evidence.json>
go run ./cmd/ariadne experiment questions
go run ./cmd/ariadne experiment questions --json
go run ./cmd/ariadne experiment ask .ariadne/runs/experiment-001 counterfactual-change
go run ./cmd/ariadne experiment ask --json .ariadne/runs/experiment-001 counterfactual-change
go run ./cmd/ariadne experiment finding --json .ariadne/runs/experiment-001 <finding-id-from-evidence.json>
```

The output directory must not exist before `experiment run`. A successful final
command reports `experiment-001-email` with one difference. `report.md` must
show `variant` changing from `standard` to `personalized`, `region` remaining
stable, `request_id` normalized, and six verified artifacts.

In `evidence.json`, `target.package_sha256` must equal the SHA-256 of the built
APK, `target.ariadne_revision` must equal `git rev-parse HEAD`, and
`target.ariadne_modified` must reflect whether the checkout had source changes
when `go run` built Ariadne.

### Common failures

- If target preflight fails, confirm the selected serial with `adb devices` and
  reinstall the fixture on that device.
- If the run directory already exists, choose a new output path. Ariadne does
  not overwrite prior sessions or reports.
- If the installed-package hash is unexpected, rebuild and reinstall the same
  APK before running either session.
- If report generation fails, inspect each `session.json` step status. Ariadne
  fails closed for unsupported capture shapes or artifact integrity failures.
- Verification can use `--json` for the stable raw-value-free fields
  `manifest_name`, `differences`, and `unknowns`; it remains non-destructive.
- `experiment list --json <archive-root>` inspects only immediate child
  directories, rejects symbolic links, and returns only relative directory
  names plus verified summary fields.
- `experiment serve <archive-root>` starts a localhost-only, read-only review
  page at `http://127.0.0.1:8787/`; only loopback IP addresses are accepted, and
  it lists verified bundles and links to the same bounded questions and finding
  references without rendering observations. The archive page can re-check one
  fixed bounded question across all verified bundles. A bundle page also shows
  safe provenance context: its bounded question and answer state, manifest contract
  digest, verified baseline start time in UTC, recorded Ariadne revision, and
  modified-worktree flag, followed by a
  re-verified board for the fixed question catalog. Question and finding detail
  pages retain that same context after following a link. Available archive-lens
  results show the same safe contract, UTC recorded time, revision, and
  working-tree context.
- If `ariadne_modified` is unexpectedly `true`, inspect `git status --short`
  before treating the run as reproducible from the recorded revision alone.
- Finding lookup re-verifies the bundle first and prints only the question,
  answer state, stable ID, field, and source references. It rejects unknown
  IDs, tampered artifacts, and malformed bundles without writing output. Add
  `--json` for the same raw-value-free fields in deterministic machine-readable
  order.
- The bounded question catalog currently supports `counterfactual-change`,
  `capture-complete`, and `source-integrity`. It returns deterministic answer
  states and finding IDs, and rejects any other question ID. Use
  `experiment questions --json` to enumerate those IDs and their safe display
  text before asking one.

## Run isolated sessions

After installing the fixture, run:

```console
go run ./cmd/ariadne experiment run --device emulator-5554 --package dev.ariadne.fixture --output .ariadne/runs/experiment-001 examples/experiment-001.json
```

The output directory must not already exist. Ariadne clears the selected
package before each session, starts `.MainActivity` with the persona fields,
performs the one manifest-declared resource-ID interaction, and captures the
raw session artifacts.

After both sessions succeed, verify the artifacts and write the evidence
outputs:

```console
go run ./cmd/ariadne experiment report .ariadne/runs/experiment-001
go run ./cmd/ariadne experiment verify .ariadne/runs/experiment-001
go run ./cmd/ariadne experiment finding .ariadne/runs/experiment-001 <finding-id-from-evidence.json>
go run ./cmd/ariadne experiment ask .ariadne/runs/experiment-001 counterfactual-change
```

The report command refuses existing `evidence.json` or `report.md` files. The
verify command is non-destructive: it rechecks the sessions, artifact hashes,
normalization inputs, and existing output bytes without rerunning capture or
rewriting either file. The finding command uses that same verification path and
returns the safe question state plus source references without returning raw
observation or persona values. The
question command is a fixed, deterministic catalog rather than an arbitrary
natural-language answerer; it can be rerun after archival and returns the same
answer state and finding IDs. Add `--json` for a stable, raw-value-free object
with the same fields plus `reason` for an unknown finding when one is available.
Unknown answers also include the verifier-owned `reason` when one is available;
complete answers omit it. Human-readable output remains the default. The
catalog command exposes the same fixed questions without needing a run
directory, so a caller can enumerate before asking. The completed directory
contains:

```text
.ariadne/runs/experiment-001/
|-- evidence.json
|-- report.md
|-- baseline/
|   |-- observations/
|   |   |-- network.json
|   |   `-- storage.json
|   `-- session.json
`-- treatment/
    |-- observations/
    |   |-- network.json
    |   `-- storage.json
    `-- session.json
```

Session metadata includes the selected device, Android API, architecture,
package version and SHA-256, Ariadne Git revision and modified state, ADB
version, timestamps, step status, exit codes, and a SHA-256 record for each
captured artifact. Session schemas 3 and 4 remain readable for launch-only
runs; schema 5 remains readable for stable-ID runs without a contract digest;
current schema 6 records `tap_resource_id`, the structural
`manifest_contract_sha256`, and the successful `interact` step. All current
schemas record `status` as `complete` or `incomplete`. Incomplete sessions
record only a controlled `failure_stage`; they never persist raw errors.
Metadata excludes persona values, command arguments, raw APK bytes, and raw
ADB output. Schemas 4, 5, and 6 also record the manifest's sorted
`volatile_fields` declaration without observation values.

The storage artifact is the exact bounded JSON read from the fixture's private
`files/observation.json` through Android's `run-as` command. Capture fails if
the package is not debuggable, the file is missing, the output is not a JSON
object, or it exceeds 64 KiB.

For each session, Ariadne binds an ephemeral IPv4 loopback port and temporarily
maps the same port into the selected device with `adb reverse`. The network
artifact records one `POST /observe` request's method, path, media type, and
exact body encoded as base64. It does not collect unrelated request headers.
The body must be a JSON object no larger than 64 KiB. Ariadne removes the
reverse mapping before the session ends, including after capture failures.

`evidence.json` verifies matching target provenance, session order, successful
step records, artifact sizes, and SHA-256 digests before recording the
normalization and comparison. Current evidence schema 7 also records the
manifest contract digest, the safe question `Did changing <variable> influence
an observed output?`, its answer state, and deterministic SHA-256 IDs for each
difference or unknown. Each finding ID includes its source path and the digest
of the immutable artifact it references; it never includes observed values. A
complete pair is `observed`; the supported treatment-storage gap is `unknown`.
The manifest contract digest covers only schema
version, manifest name, declared variable, persona field names, volatile
fields, and the stable tap resource ID; it never includes persona values.
Legacy session-schema-4 and schema-5 runs continue to produce readable
evidence. `report.md` is the concise human-readable view.
For the fixture, it reports one observed `variant` difference supported by both
storage and network artifacts. The differing raw `request_id` values remain in
those artifacts but are not copied into `evidence.json` or `report.md`.

Observation schema 1 is a bounded JSON object containing `schema_version: 1`
and 1 to 64 string fields. Field names are restricted so evidence references
remain unambiguous. Ariadne requires storage and network fields to agree within
each session, then compares the sorted union of baseline and treatment fields.
Each observed difference is classified as `added`, `removed`, or `changed`;
equal fields are listed as stable. A declared volatile field is removed from
comparison only when both sessions captured it. The field is then listed in
`comparison.normalized_fields` and the applied rule is recorded in
`normalizations`. A one-sided volatile field remains an added or removed
finding. Raw artifacts are never rewritten.

If the baseline completes but treatment storage capture fails after treatment
network capture succeeds, reporting preserves the five verified artifacts and
classifies every field found in either available session as `unknown`. It does
not compare the available network value or claim that any field changed or
stayed stable. Other incomplete session shapes remain unsupported and stop
report generation.
The authorized fixture proof is declared in
`examples/experiment-001-storage-gap.json` and runs in the real-emulator
workflow.

The real-emulator workflow also runs failure checks against copies of the
completed run. It requires Ariadne to reject a nonexistent package, modified
observation bytes, and baseline/treatment package-digest disagreement. Failed
report attempts must not create `evidence.json` or `report.md`, write to stdout,
or disclose persona values. The workflow uploads both the untouched complete
run and the verified treatment-storage-gap run.

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
- The fixture's unique per-session `request_id` is recorded as normalized, not
  reported as causal.
- Every finding links to raw evidence.
- The supported treatment-storage gap is reported as `unknown`; other gaps stop
  report generation instead of being silently omitted.
- A new contributor can reproduce the result.

## Non-goals

- Testing third-party applications without authorization
- Defeating certificate pinning
- Running a general-purpose Android malware sandbox
- Supporting multiple capture backends before the first experiment works
