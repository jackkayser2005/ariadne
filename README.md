# Ariadne

Ariadne is an open-source counterfactual privacy analysis tool. It runs
software in controlled parallel environments, changes one piece of personal
data, and reports which observable behaviors change.

Reports classify conclusions as **observed**, **inferred**, **claimed**, or
**unknown**.

## Experiment 001

The first milestone targets an authorized Android test application:

1. Define two personas that differ by one value.
2. Run the same scripted interaction for both personas.
3. Capture network and app-storage observations.
4. Normalize expected noise.
5. Report outputs influenced by the changed value.
6. Produce a redacted, reproducible export from a verified evidence bundle.

The detailed design and experiment log live in [`docs/`](docs/).

## Reproduce Experiment 001

The complete local procedure is documented in
[`docs/content/docs/experiments/experiment-001.md`](docs/content/docs/experiments/experiment-001.md#reproduce-from-a-fresh-checkout).
It builds the authorized fixture, installs it on one explicitly selected
Android emulator, runs baseline and treatment sessions, and verifies the
resulting evidence bundle.

The same procedure runs on a real API 35 emulator in GitHub Actions. It also
proves that missing targets, modified observations, and mismatched package
provenance prevent evidence publication.

Once a report is saved, Ariadne can re-verify it offline and compare two saved
reflection snapshots without exposing captured values:

```console
go run ./cmd/ariadne experiment ask-archive verify --json <reflection.json>
go run ./cmd/ariadne experiment ask-archive compare --json <older-reflection.json> <newer-reflection.json>
```

The comparison reports `same`, `changed`, or `incomparable` for bounded
per-directory answer states. It does not infer a trend or prove the underlying
evidence.

## Development

Prerequisites:

- Go 1.24 or newer
- Docker Desktop for containerized tools and documentation
- JDK 17
- Android SDK Platform 36 and Build Tools 36.0.0

Preview the documentation:

```console
docker compose up --build docs
```

Then open <http://localhost:1313>.

Build and test the authorized Android fixture:

```console
cd fixture/android
./gradlew testDebugUnitTest createDebugUnitTestCoverageReport lintDebug assembleDebug
```

## Automation

Every push and pull request targeting `main` runs formatting, vetting,
race-enabled tests, and a 90% coverage gate. Changes to Experiment 001's
runner, fixture, or Go implementation also execute the complete workflow on a
real API 35 Android emulator.

Documentation changes merged into `main` are built and published through
GitHub Pages using the repository's **GitHub Actions** publishing source.

## Status

Pre-alpha. We are building Experiment 001 in the
[issue tracker](https://github.com/jackkayser2005/ariadne/issues).

## Safety

Only analyze software, devices, accounts, and data you own or are explicitly
authorized to test. Evidence bundles must redact secrets and unrelated personal
data by default.
