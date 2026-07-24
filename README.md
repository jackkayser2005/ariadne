# Ariadne

Trace where your data leads.

Ariadne is an open-source counterfactual privacy analysis tool. It runs
software in controlled parallel environments, changes one piece of personal
data, and reports which observable behaviors change.

Privacy claims are hypotheses. Ariadne records evidence as **observed**,
**inferred**, **claimed**, or **unknown**.

## Experiment 001

The first milestone targets an authorized Android test application:

1. Define two personas that differ by one value.
2. Run the same scripted interaction for both personas.
3. Capture network and app-storage observations.
4. Normalize expected noise.
5. Report outputs influenced by the changed value.
6. Produce a redacted, reproducible evidence bundle.

The detailed design and experiment log live in [`docs/`](docs/).

## Development

Prerequisites:

- Go 1.24 or newer
- Docker Desktop for containerized tools and documentation
- Android SDK Platform Tools when work on the device runner begins

Preview the documentation:

```console
docker compose up --build docs
```

Then open <http://localhost:1313>.

## Automation

Every push and pull request targeting `main` runs formatting, vetting,
race-enabled tests, and a 90% coverage gate.

Documentation changes merged into `main` are built and published through
GitHub Pages using the repository's **GitHub Actions** publishing source.

## Status

Pre-alpha. We are building Experiment 001 in the
[issue tracker](https://github.com/jackkayser2005/ariadne/issues).

## Safety

Only analyze software, devices, accounts, and data you own or are explicitly
authorized to test. Evidence bundles must redact secrets and unrelated personal
data by default.
