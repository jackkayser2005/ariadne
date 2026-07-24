---
title: Releases
weight: 2
---

# Release and Tagging Policy

Ariadne uses [Semantic Versioning](https://semver.org/) with tags in the form
`vMAJOR.MINOR.PATCH`.

Before `v1.0.0`:

- increment `MINOR` for a new usable capability or breaking contract change
- increment `PATCH` for a compatible fix or documentation correction shipped
  with code
- use `-alpha.N`, `-beta.N`, and `-rc.N` for prereleases, for example
  `v0.1.0-alpha.1`

Create tags only from protected `main` after required CI passes. Use annotated
tags:

```console
git switch main
git pull --ff-only
git tag -a v0.1.0-alpha.1 -m "Ariadne v0.1.0-alpha.1"
git push origin v0.1.0-alpha.1
```

Every tag must have a matching GitHub Release whose notes use the headings
`Added`, `Changed`, `Fixed`, and `Security` as applicable. Documentation-only
changes do not require a release.

Published tags are immutable. Never move, overwrite, or delete one to correct
a mistake; publish the next patch or prerelease instead.
