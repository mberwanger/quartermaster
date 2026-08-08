---
id: eng.experimental-cache
title: A caching approach we are still arguing about
description: An unfinished proposal, kept in the store so the thinking is not lost. It is distributed and readable, but no package may turn it into a rule.
domain: engineering
type: concept
status: draft
provenance: asserted
owner: platform
tags: [go, performance]
---

# A caching approach we are still arguing about

This document exists to show what a draft does in this model.

It **is** distributed: it lands in `.quartermaster/knowledge/` like everything
else, and an agent that goes looking will find it — with `status: draft` and
`provenance: asserted` at the top, so the uncertainty travels with the claim.

It **cannot** become a rule. `requires` in `bundle.yaml` asks for `status:
active` and a provenance of `verified` or `decided`. Naming this id in a package
fails the build rather than quietly loading an unfinished idea into every
session:

```
package "engineering" references "eng.experimental-cache", which does not meet
the requirements: status is "draft", requires one of: active
```

Try it with `qm bundle explain eng.experimental-cache`.