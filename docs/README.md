# Ariadne documentation

The documentation is part of the product. Keep claims tied to evidence and
update the relevant page in the same pull request as a behavioral change.

## Preview

From the repository root:

```console
docker compose up --build docs
```

The site is available at <http://localhost:1313>.

To use a locally installed extended Hugo build instead:

```console
cd docs
hugo server --buildDrafts --disableFastRender
```

Changes under `docs/` are built and published by GitHub Actions when they reach
`main`. Pull requests must pass the documentation build before merge.

## Structure

- `content/docs/concepts/`: stable vocabulary and mental models
- `content/docs/experiments/`: hypotheses, procedures, and results
- `content/docs/contributing/`: contributor workflows

Do not document planned abstractions as though they exist.
