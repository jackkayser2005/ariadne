# Repository Guidelines

## Project Structure & Module Organization

Ariadne is a Go project for counterfactual privacy analysis. Production code
lives under `internal/`; the initial evidence vocabulary is in
`internal/evidence/`. Keep tests beside their package as `*_test.go`.

Documentation source lives in `docs/content/`. Hugo configuration and its
isolated theme module are in `docs/hugo.yaml` and `docs/go.mod`. Container
configuration is limited to `docs/Dockerfile` and `compose.yaml`. GitHub Actions
workflows live in `.github/workflows/`.

Add a directory only when a working vertical slice needs it. Do not scaffold
future adapters, plugins, or services.

## Build, Test, and Development Commands

- `go build ./...` compiles every current Go package.
- `go fmt ./...` applies standard Go formatting.
- `go vet ./...` runs Go's static checks.
- `go test -race -covermode=atomic -coverprofile=coverage.out ./...` runs tests,
  the race detector, and writes coverage data.
- `docker compose up --build docs` serves documentation at
  `http://localhost:1313`.

Docker Desktop is required only for the documentation container today.

## Coding Style & Naming Conventions

Follow `gofmt`; do not hand-align Go code. Package names are short, lowercase,
and singular. Exported identifiers require useful Go doc comments. Prefer
concrete functions and standard-library types over interfaces with one
implementation. Wrap errors with actionable context and avoid global mutable
state.

## Testing Guidelines

Use Go's standard `testing` package. Name tests after behavior, for example
`TestStateValid`. Keep total statement coverage at or above 90%; coverage is a
floor, not a substitute for boundary and failure-path tests. Run race-enabled
tests before opening a pull request. Parsers and capture readers must test
malformed, oversized, and adversarial input.

## Commit & Pull Request Guidelines

History currently follows short typed subjects such as
`docs: introduce Ariadne`. Use `<type>: <imperative summary>` with types such as
`feat`, `fix`, `test`, `docs`, or `ci`.

Pull requests should link an issue, describe observable behavior, list
verification commands, and update relevant documentation. Include screenshots
only for visual documentation changes. Keep each pull request focused enough
to review independently.

## Security & Agent Instructions

Operate only on explicitly authorized software, devices, accounts, and data.
Never commit `.env`, captures, evidence runs, keys, tokens, or personal data.
Treat captured content as hostile: validate sizes and paths, avoid shell
interpolation, redact before logging, and report missing visibility as
`unknown`. Do not weaken validation, redaction, accessibility, or tests to
reduce code size.
