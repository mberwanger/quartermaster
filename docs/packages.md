---
title: Packages
type: design
status: proposed
owner: martin
description: One mechanism for selecting what a repository gets, replacing the four idioms a manifest uses today. A package names documents; type decides how each is delivered; a resident list decides which are pushed.
---

# Packages

**Deferred as of 2026-07-24.** The shape is settled and written down here so it does not have to be rediscovered, but it is not scheduled. The current manifest works, the store has two packages that are pure rulesets, and nothing is blocked on this. Pick it up when hand-maintaining id lists actually hurts, which is when a team arrives with ten skills rather than before.

## The problem

A repository declares four kinds of delivery using three different idioms.

```yaml
bundles:
  - source: oci://ghcr.io/admiral/knowledge:v1
    rulesets: [voice]                          # a name, resolved store-side
    skills:   [engineering.skills.gcp-expert]  # raw document ids
    agents:   [agents.code-reviewer]           # raw document ids
    knowledge: {domain: [engineering]}         # a predicate over frontmatter
```

Three consequences follow, none of them chosen.

**Only rules get producer curation.** The store can say "these are the voice rules." It cannot say "these are the things a billing repository wants." Every repository re-derives that for itself, and they drift.

**Store ids leak into consumer manifests.** Renaming a skill breaks every manifest that names it. Rules are immune, because the ruleset name is the interface and the ids sit behind it. The store's own conventions argue for stable ids precisely because things reference them, and then two of four deliveries reference them from outside the store.

**It does not scale by hand.** A team with ten skills, three agents, and a handful of rules lists sixteen ids per repository and maintains them forever. Adding an eleventh skill is a pull request against every consuming repository.

## The shape

Delivery is already a function of type. A `skill` renders as a skill, an `agent` as an agent, everything else sits on disk and is read on demand. Nothing has to declare which is which.

So a package answers two questions, and only two.

1. Which documents are in it.
2. Which of those are resident.

```yaml
# store/meta/packages.yaml
voice:
  resident: [voice.base]

voice-authoring:
  resident: [voice.base, voice.reference, voice.guide, voice.blog]

billing:
  include:  ["billing/**"]
  resident: [billing.invariants, billing.rounding]
```

```yaml
# .quartermaster.yaml
bundles:
  - source: oci://ghcr.io/admiral/knowledge:v1
    use: [voice, billing]
```

The billing team names one package and gets ten skills, three agents, the billing knowledge tree, and two resident rules. Adding an eleventh skill means dropping a file into `billing/skills/`. No manifest changes, in the store or in any consuming repository.

`include` takes path globs, an id list, or a frontmatter predicate, and defaults to nothing. `resident` takes ids, optionally with scope, exactly as a ruleset entry does today.

## What this replaces

`rulesets.yaml` becomes `packages.yaml`. A ruleset is a package with only a `resident` list, so today's two carry over unchanged in meaning and mostly unchanged in text.

The manifest loses `rulesets`, `skills`, `agents`, and `knowledge`, and gains `use`. Four idioms become one.

`rulesets.json` in the artifact becomes `packages.json`. Format 0.3 becomes 0.4.

## Decisions

**A repository gets only what its packages include.** Today the whole knowledge tree lands minus an optional filter. Under packages, a document belonging to no package ships nowhere. Trees get smaller, an unselected document costs no disk and no retrieval noise, and "what did I select" and "what do I have" stop being different questions. A store that wants the old behavior declares it:

```yaml
everything:
  include: ["**/*.md"]
```

This is the one behavior change rather than a syntax change, and it is the reason for the format bump.

**A package may select agents by glob, like anything else.** An earlier draft carved agents out on the grounds that an agent is a capability grant rather than text, so it should always be named individually. That does not survive contact with how rules already work: a ruleset gains a document and it reaches every consuming repository on the next sync, and a rule is more expensive than an agent because it is resident in every session. Refusing the same propagation for the cheaper case was a rule with no argument behind it. The controls that do the work are elsewhere and stay: the build refuses to distribute an agent that waives the permission prompt, digests are pinned, and updates land as reviewed pull requests.

**Packages are declared in the store, not in the manifest.** The alternative, a predicate in the consuming manifest, is a standing order about the future: give me every billing skill anyone ever writes. A new document then reaches every repository with no review at the point it arrives. A store-side package propagates the same way, but through the store's merge gate and CODEOWNERS, which is the designed path. The cost of enumerating lands on the person adding the document, in the commit where they add it, when they already know what it belongs to.

## What it costs

`internal/ruleset` becomes `internal/package` and grows `include` resolution. `internal/manifest` drops three fields and gains one. `internal/plan` stops resolving skills and agents from manifest lists and starts partitioning a package's documents by type. Targets are untouched: they already take rules, skills, and agents separately, and that split does not change.

The store moves `meta/rulesets.yaml` to `meta/packages.yaml` and adds an `include` to nothing, since both current packages are pure rulesets. Consuming repositories rewrite three manifest keys as one.

Nothing is published and no repository pins a digest that matters, so there is no migration burden beyond this repository's tests, its scaffold, its README, and the one store.
