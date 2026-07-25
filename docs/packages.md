---
title: Packages
type: design
status: active
owner: martin
description: One mechanism for selecting what a repository gets, replacing the four idioms a manifest uses today. A package names documents; type decides how each is delivered; a resident list decides which are pushed.
---

# Packages

**Built 2026-07-25.** This document described the design before it existed; what shipped follows it with two changes, both noted in place below: selections are patterns rather than plain id lists, and knowledge is never package-scoped.

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

Delivery is already a function of type: a `skill` renders as a skill, an `agent`
as an agent, everything else sits on disk. So a package names what it wants and
the tool knows what to do with each.

```yaml
# meta/packages.yaml
engineering:
  rules:
    - engineering.commit-messages
  skills:
    - skills.engineering.*

data-engineering:
  rules:
    - engineering.commit-messages
    - id: engineering.go-imports
      scope: ["**/*.go"]
  skills:
    - skills.engineering.*
    - skills.data.*
  agents:
    - agents.doc-reviewer
```

```yaml
# .quartermaster.yaml
bundles:
  - source: oci://ghcr.io/org/knowledge:latest
    use: [data-engineering]
```

**Selections are patterns, and a plain id is a pattern with no wildcard.** That
is the change from the original design, and it is the one that makes this worth
having: a team with thirty skills writes `skills.data.*` rather than thirty
lines, and adding a thirty-first reaches every package that globs it without
anyone editing a file.

A `*` matches within one dot-separated segment and `**` crosses them, so
`skills.data.*` takes a team's set and `skills.**` takes every skill there is.
Ids are matched rather than paths, because ids are already the interface
everywhere else: manifests name them, and a facet record cites one months later.

A `where` clause selects on frontmatter instead, for stores whose grouping should
survive documents being renamed and moved:

```yaml
platform:
  skills:
    - where: {tags: [platform]}
```

## What this replaces

`rulesets.yaml` becomes `packages.yaml`, and a ruleset becomes a package with
only a `rules` list. The manifest loses `rulesets`, `skills`, and `agents`, and
gains `use`. `rulesets.json` in the artifact becomes `packages.json`. Format 0.3
becomes 0.4.

## Decisions

**Knowledge is not selectable.** The original design said a document belonging to
no package ships nowhere. That was wrong for the case this exists to serve: a
central store is one people can search, and knowledge on disk costs no context.
Every document in a bundle stays retrievable whatever a repository selects, and a
package decides only what arrives without being asked for. The manifest's
`knowledge` filter still trims the tree for repositories that want less of it.

**Explicit and pattern selections fail differently.** Naming an id that does not
exist, or naming a concept as a skill, is the author's mistake and fails the
build. A pattern that sweeps up something which does not qualify skips it,
because otherwise setting one document to `draft` would break every package
globbing its neighbours. A pattern matching *nothing* still fails: that is almost
always a typo, and selecting nothing silently is how a repository ends up with no
skills and no explanation.

**A package may select agents by pattern, like anything else.** An earlier draft
carved agents out on the grounds that an agent is a capability grant rather than
text. That does not survive contact with how rules already work: a package gains
a rule and it reaches every consuming repository on the next update, and a rule is
more expensive than an agent because it is resident in every session. The
controls that do the work are elsewhere and stay: the build refuses to distribute
an agent that waives the permission prompt, digests are pinned, and updates land
as reviewed pull requests.

**Packages are declared in the store, not in the manifest.** A predicate in the
consuming manifest would be a standing order about the future: give me every
skill anyone ever writes for this team. A store-side package propagates the same
way, but through the store's merge gate and CODEOWNERS, which is the designed
path.

**Restricted documents are refused as any kind**, not only as rules. A restricted
document never enters the bundle, so selecting it as a skill would emit a package
pointing at a file that is not there.
