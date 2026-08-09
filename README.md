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
The evidence-backed first-year path is tracked in
[`docs/content/docs/roadmap.md`](docs/content/docs/roadmap.md).

The shareable export has its own canonical SHA-256 identity. Verify a received
export structurally, and optionally require the expected identity, with
`experiment export verify --json --expect-sha256 <export-sha256> <export.json>`.
That identity covers only the raw-value-free export content; it does not prove
the underlying evidence.

A verified export can answer its embedded counterfactual question offline with
`experiment export ask --json <export.json> counterfactual-change`. Questions
about capture completeness or source integrity remain unavailable without the
authoritative evidence bundle. Follow one returned safe finding reference with
`experiment export finding --json <export.json> <finding-id>`; comparison values
remain unavailable, and both JSON responses carry the verified source-evidence
and export identities they came from.

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
go run ./cmd/ariadne experiment ask-archive save --json <archive-root> <question-id> <reflection.json>
go run ./cmd/ariadne experiment ask-archive compare --json <older-reflection.json> <newer-reflection.json>
go run ./cmd/ariadne experiment ask-archive compare-current --json <older-reflection.json> <archive-root>
go run ./cmd/ariadne experiment ask-archive transitions --json <reflection-1.json> <reflection-2.json> ...
go run ./cmd/ariadne experiment ask-archive transitions questions --json
go run ./cmd/ariadne experiment ask-archive transitions ask --json <history.json> [<question-id>]
go run ./cmd/ariadne experiment ask-archive transitions ask repeated --json <history.json>
go run ./cmd/ariadne experiment ask-archive transitions ask all --json <history.json>
go run ./cmd/ariadne experiment ask-archive transitions ask receipt --json <history.json> <question-id>
go run ./cmd/ariadne experiment ask-archive transitions save --json <reflection-1.json> <reflection-2.json> ... <history.json>
go run ./cmd/ariadne experiment ask-archive transitions verify --json <transitions.json>
```

The comparison reports `same`, `changed`, or `incomparable` for bounded
per-directory answer states. When common entries change, it also names those
safe archive directories and their older/newer answer states. It does not
infer a trend or prove the underlying evidence. The transitions command
applies those same bounded comparisons to each adjacent pair in caller-supplied
order and reports safe reflection identities, aggregate change counts, and any
changed archive directories with their bounded older/newer states. The saved
transition ledger carries those same state changes without observations or
persona values. Current ledgers also carry a safe summary for each supplied
snapshot: its reflection identity and observed, unknown, unavailable, and
checked counts. These summaries make the historical spine inspectable without
reopening raw evidence.
The saved transition ledger can be structurally re-verified and given an
expected content identity before another tool consumes it. Verification also
requires adjacent transitions to share their boundary reflection identity, so
a history cannot silently join unrelated snapshots.

`transitions questions` lists the fixed, raw-value-free questions available for
a verified history in stable order. Use it to discover the question IDs before
asking one; it does not create arbitrary natural-language queries.

`transitions ask` answers a catalog question from the verified history itself.
With no question ID it preserves the original history question; pass any ID
from `transitions questions` to select a fixed question. It returns only the bounded result, 1-based transition indexes
for changed or membership-incomparable boundaries, and safe directory/state
triples for changed entries, each bound to its adjacent reflection identities;
it does not infer chronology or prove the underlying evidence. The legacy
`ask repeated` spelling remains supported. Legacy schema 1
histories retain the indexes and have no per-entry details.

`transitions ask repeated` asks a second fixed question of the verified
history: whether any safe archive entry changed at more than one supplied
boundary. It returns the repeated entry's safe state-change records and
adjacent reflection identities. Schema 1 histories answer `unavailable`, and
the result never establishes chronology or a trend.

`transitions ask <history.json> answer-state-snapshot-summaries` asks which
safe snapshot summaries a verified history recorded. Schema 3 histories return each
snapshot's identity and observed/unknown/unavailable/checked counts; schema 1
and 2 histories answer `unavailable`. It does not infer chronology or prove
the underlying evidence.

`transitions ask <history.json> answer-state-summary-changes` asks whether
those bounded snapshot summaries changed at any supplied boundary. Schema 3
histories return `same` or `changed` plus 1-based boundary indexes; schema 1
and 2 histories answer `unavailable`. This is a bounded comparison, not a
chronology or trend claim.

`transitions ask all <history.json>` verifies the history once and records the
bounded result of every fixed history question in stable catalog order. Its
raw-value-free JSON is a portable question-round receipt; call an individual
question ID for detailed entries or snapshot summaries.

`transitions ask receipt <history.json> <question-id>` verifies the history once
and wraps one selected fixed answer in a portable raw-value-free receipt. The
receipt binds the bounded result and detailed answer to the verified history
SHA-256; use a question ID from `transitions questions` and do not infer
chronology or the underlying evidence from it.

New transition ledgers use schema 3 and include those snapshot summaries;
schema 2 ledgers remain readable, and schema 1 ledgers remain readable with
their older state-change limits.

Use `ask-archive save` to create a new snapshot with exclusive file creation;
it never overwrites an existing reflection. The command returns the same
canonical identity used by offline verification, so saved snapshots can feed
the comparison and transition commands without exposing captured values.

Use `transitions save` to persist the verified adjacent-boundary ledger with
the same no-overwrite behavior before opening it in the local history view.
The page shows the same safe snapshot summaries alongside the fixed history
questions, including a direct snapshot-summary question, so a UI driver can
choose a question and retain the identities it was asking about.

The local review page can receive a verified transition ledger and a saved
reflection with
`experiment serve --history <history.json> --reflection <reflection.json> <archive-root>`.
It renders caller-ordered bounded transitions and re-asks the saved reflection's
fixed question against the current archive, showing only safe comparison counts,
identities, per-directory bounded state changes, and the repeated-change
question, snapshot-summary question, and snapshot-change question when history
is available. The history
panel also lists those fixed question IDs and links directly to each answer, so
a UI driver can choose a bounded question without inventing natural language.
Those links use the validated `history_question_id` query and fail closed for
unknown IDs. An
unavailable comparison remains a generic bounded state; the
page does not turn it into chronology, trend inference, or a claim about the
underlying evidence.

The read-only review page exposes the same canonical SHA-256 identity for the
currently derived archive question report. It is computed in memory, contains
no captured values, and identifies the derived report only; it is not proof of
the underlying evidence or a trend claim.

It can also review one portable export with
`experiment serve --export <export.json> <archive-root>`. The export question
and its safe finding references remain read-only and display their verified
export identities.

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
