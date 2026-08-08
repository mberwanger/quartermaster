# Example store

A small knowledge store that exercises every part of the model. Everything below
runs against it as-is.

```
examples/store/
  bundle.yaml                what may become a rule
  meta/packages.yaml         the named selections
  engineering/               shared by every team
  billing/                   one team
  eligibility/               another team
```

## A billing repository

Billing wants the shared engineering rules plus its own, and has no use for
another team's documents on disk:

```yaml
bundles:
  - source: file://../quartermaster/examples/store
    use: [engineering, billing]
    knowledge:
      domain: [engineering, billing]
targets:
  - claude
```

```
qm sync
  → rules       2 resident, 3 scoped
  → knowledge   6 docs retrievable
```

What that produced:

| Document | On disk | Rule |
| --- | --- | --- |
| `eng.api-versioning` | yes | **always loaded** — declares no scope |
| `eng.error-handling` | yes | loaded when a `**/*.go` file is open |
| `eng.logging` | yes | loaded when a `**/*.go` file is open |
| `billing.money-representation` | yes | **always loaded** |
| `billing.payment-retries` | yes | loaded under `internal/billing/**` |
| `eng.experimental-cache` | yes | never — it is a `draft` |
| `eligibility.coverage-effective-dates` | **no** | never — filtered out |

Three separate things are visible there:

- **The eligibility document is not on the disk at all.** The `knowledge` filter
  scoped the tree by domain. One store can serve an organization without every
  repository carrying all of it.
- **The draft is on disk but is not a rule.** It stays readable, with
  `status: draft` at the top so the uncertainty travels with it, and `requires`
  in `bundle.yaml` stops any package promoting it. Try
  `qm bundle explain eng.experimental-cache`.
- **Only two of five rules are always loaded.** The rest cost nothing until a
  matching file is open.

## The same document, different residency

`eng.logging` declares `scope: ["**/*.go"]`, so it normally loads only for Go.
The `incident-review` package overrides that scope to nothing:

```yaml
incident-review:
  rules:
    - id: eng.logging
      scope: []
```

```
use: [engineering]        → 1 resident, 2 scoped   (logging is Go-scoped)
use: [incident-review]    → 1 resident, 0 scoped   (logging always loads)
```

Same document, same text, one implementation. Scope is a property of the
consumer, not of the knowledge.

## Things worth trying

```
qm bundle validate --root examples/store
qm bundle explain --root examples/store eng.experimental-cache
qm bundle explain --root examples/store eligibility.coverage-effective-dates
qm status
```

Ask for a package whose document your filter excludes and the contradiction is
reported rather than resolved for you:

```
Error: package "eligibility" needs eligibility.coverage-effective-dates,
which the knowledge filter excludes: domain is "eligibility",
requires one of: engineering, billing
```