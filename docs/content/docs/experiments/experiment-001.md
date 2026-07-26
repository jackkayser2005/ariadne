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
`email` and `region` string extras, writes `files/observation.json`, and exits.
It requests Android's normal `INTERNET` permission. It sends no request unless
the runner supplies `collector_port`; when supplied, it posts the same JSON to
IPv4 loopback. Cleartext traffic to other destinations is denied.

For the example manifest, the stored `variant` is `standard` for the baseline
email and `personalized` for the treatment email. Ariadne does not contain this
rule.

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
```

The output directory must not exist before `experiment run`. A successful final
command reports `experiment-001-email` with one difference. `report.md` must
show `variant` changing from `standard` to `personalized`, `region` remaining
stable, and six verified artifacts.

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
  writes no report until both sessions and all artifact integrity checks pass.
- If `ariadne_modified` is unexpectedly `true`, inspect `git status --short`
  before treating the run as reproducible from the recorded revision alone.

## Run isolated sessions

After installing the fixture, run:

```console
go run ./cmd/ariadne experiment run --device emulator-5554 --package dev.ariadne.fixture --output .ariadne/runs/experiment-001 examples/experiment-001.json
```

The output directory must not already exist. Ariadne clears the selected
package before each session, starts `.MainActivity` with the persona fields,
and captures the raw session artifacts.

After both sessions succeed, verify the artifacts and write the evidence
outputs:

```console
go run ./cmd/ariadne experiment report .ariadne/runs/experiment-001
```

The report command refuses existing `evidence.json` or `report.md` files. The
completed directory contains:

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
captured artifact. Session schema 3 also records `status` as `complete` or
`incomplete`. Incomplete sessions record only a controlled `failure_stage`;
they never persist raw errors. Metadata excludes persona values, command
arguments, raw APK bytes, and raw ADB output.

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
normalization and comparison. `report.md` is the concise human-readable view.
For the fixture, it reports one observed `variant` difference supported by both
storage and network artifacts.

The real-emulator workflow also runs failure checks against copies of the
completed run. It requires Ariadne to reject a nonexistent package, modified
observation bytes, and baseline/treatment package-digest disagreement. Failed
report attempts must not create `evidence.json` or `report.md`, write to stdout,
or disclose persona values. Only the untouched successful run is uploaded.

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
- Capture gaps stop report generation instead of being silently omitted.
- A new contributor can reproduce the result.

The current experiment does not emit a partial report that classifies capture
gaps as `unknown`. That requires an evidence format for incomplete runs and is
deferred until after this complete-run proof.

## Non-goals

- Testing third-party applications without authorization
- Defeating certificate pinning
- Running a general-purpose Android malware sandbox
- Supporting multiple capture backends before the first experiment works
