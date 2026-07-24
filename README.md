# Ariadne

Trace where your data leads.

Ariadne is an open-source counterfactual privacy analysis tool. It runs software in controlled parallel environments, changes one piece of personal data, and reports which observable behaviors change.

Privacy claims are hypotheses. Ariadne records evidence as **observed**, **inferred**, **claimed**, or **unknown**.

## Experiment 001

The first milestone targets an Android app running in a test environment:

1. Define two personas that differ by one value.
2. Run the same scripted interaction for both personas.
3. Capture network and app-storage observations.
4. Normalize expected noise.
5. Report outputs influenced by the changed value.
6. Produce a redacted, reproducible evidence bundle.

Ariadne will be written in Go. The initial implementation will use the standard library wherever practical and prioritize deterministic core logic with focused tests.

## Status

Pre-alpha. The first experiment is being designed in the issue tracker.

## Safety

Only analyze software, devices, accounts, and data you own or are explicitly authorized to test. Evidence bundles must redact secrets and unrelated personal data by default.
