---
title: Evidence model
weight: 2
---

Ariadne uses four states:

| State | Meaning |
| --- | --- |
| **Observed** | The captured artifacts directly contain the reported behavior. |
| **Inferred** | Evidence supports the conclusion, but it was not directly captured. |
| **Claimed** | A vendor, operator, or other source states the behavior. |
| **Unknown** | Available capture cannot establish what happened. |

A report may contain multiple states for one question. For example, a privacy
policy can claim that a value is not retained while a controlled run observes
that value in local storage.

Findings must link to immutable artifact hashes. Normalized output must retain
a path back to its raw source. In current Experiment 001 bundles, every
difference and unknown also has a deterministic SHA-256 finding ID derived
from its state, source paths, and referenced artifact hashes; observed values
are excluded from the ID. Re-verifying the same bundle therefore preserves
the identity, while changing a source artifact changes the verified output.
