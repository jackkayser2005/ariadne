---
title: Day 1 Path
weight: 1
---

# Day 1: Establish the Experiment Contract

## Outcome

End the day with one runnable command:

```console
ariadne validate examples/experiment-001.json
```

The command must load an experiment manifest, verify that its baseline and
treatment personas differ in exactly one declared value, and print a stable
summary. This establishes Ariadne's first trustworthy contract before Android,
capture, or reporting complexity is introduced.

## Path

### 1. Establish the baseline

- Remove redundant documentation files agreed during cleanup.
- Add the open-source license and security contact.
- Commit and push the repository bootstrap.
- Confirm CI and GitHub Pages complete successfully.

### 2. Define the manifest

Use JSON and Go's standard library. The first schema contains:

- schema version
- experiment name
- declared variable
- baseline persona
- treatment persona

Reject unknown fields, missing values, duplicate fields, unsupported schema
versions, and personas with zero or multiple differences.

### 3. Build the validator

- Add the thin command entry point under `cmd/ariadne/`.
- Keep parsing and validation in `internal/experiment/`.
- Return actionable errors without exposing persona values unnecessarily.
- Produce deterministic output suitable for tests and future automation.

### 4. Prove behavior

Add focused tests for:

- one valid difference
- malformed JSON
- unknown fields
- zero differences
- multiple differences
- missing declared variable
- oversized input

Run:

```console
go fmt ./...
go vet ./...
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go run ./cmd/ariadne validate examples/experiment-001.json
```

Coverage must remain at or above 90%.

## Day 1 Non-Goals

- ADB or Android emulator control
- network or storage capture
- normalization and causal comparison
- plugins, databases, services, or a graphical interface

## Done

Day 1 is complete when a fresh checkout can validate the example manifest,
reject every documented invalid case, pass CI, and explain the contract in the
published documentation.
