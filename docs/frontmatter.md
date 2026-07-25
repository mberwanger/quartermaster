---
title: Frontmatter
type: reference
description: Which frontmatter fields Quartermaster reads, which are yours to define, and what happens when one is missing. Written because the contract is real, spread across five files, and was documented nowhere.
---

# Frontmatter

Quartermaster ships no schema. It validates a store against the schema that
store declares, which is what lets a team add whatever fields it wants.

That is true and it is not the whole truth. The tool reads about a dozen field
names directly, in Go, and a store that renames or drops one loses the behavior
attached to it. Some of those losses are loud. Most are silent, which is the
reason for this page.

Two rules explain nearly everything here:

- **The schema is checked against the store's rules.** You write it, CI enforces
  it, and Quartermaster never second-guesses it.
- **The fields below are checked against Quartermaster's.** They are not in any
  schema, and the tool behaves differently depending on whether they are there.

## What is required to exist at all

Less than people expect. A directory of markdown with none of these files
validates and builds.

| File | Required | If declared but missing |
|---|---|---|
| `bundle.yaml` | No. Without it every `**/*.md` is a document, nothing is excluded, and there is no gate | n/a |
| the schema named by `schema:` | No, unless the key is present | **Error**, and the build stops |
| the rulesets file named by `rulesets:` | No, unless the key is present | **Error**, and the build stops |
| `index.md` | No | n/a |
| knowledge, skills, agents | No. A store with none of them builds a valid bundle | n/a |

Omitting a key means "there is none of this." Naming a file that is not there
means the path is wrong, and both now fail rather than one of them quietly
producing a bundle with no rulesets in it.

## What Quartermaster reads

| Field | Read by | Missing means |
|---|---|---|
| `id` | rulesets, the catalog, the usage log, facet records | **Validation error.** Everything downstream joins on it |
| `scope` | `internal/ruleset/ruleset.go:138` | The rule is **resident**, loading every session, rather than scoped. Silent |
| `visibility` | `internal/bundle/bundle.go:298`, compared against `restricted` | Nothing can be withheld from the bundle. Silent |
| `skill` block | `internal/plan/skill.go:54` | `name` falls back to the last segment of the id; `allowed-tools` is simply absent |
| `agent` block | `internal/plan/agent.go:25` | The agent renders with no tools, model, or permission mode |
| `agent.permission-mode` | `internal/bundle/bundle.go:318` | A bundle refuses to carry `bypassPermissions`. This one is loud |
| `title` | skill and agent rendering, index listings | Renders empty |
| `description` | every renderer, and index listings | Renders empty. This is the field an agent reads to decide whether to open a document, so an empty one is the most expensive silent loss here |
| `supersedes`, `superseded_by` | `internal/validate/validate.go:134` | Supersede links go unchecked |
| `type` | `internal/index/index.go:167`, and conventionally the `skills:` gate | Documents group under "Other" in listings, and a skill stops being recognized as one |

`id` is the only entry enforced independently of your schema. The rest degrade,
and most of them degrade quietly.

**The silent ones are worth reading twice.** A rule that loses its `scope` does
not fail; it starts loading in every session, and the first sign is a budget
warning or a bill. A document that loses `visibility` does not fail; it simply
becomes shippable. Neither shows up in CI, because your schema is the only thing
CI checks and your schema is what changed.

## What is yours

Everything else, and more of the tool than the table suggests.

**Which fields decide eligibility.** `requires:` in `bundle.yaml` names the
fields a document must satisfy before a ruleset may turn it into a rule. The
fields are yours: Admiral's store gates on `status`, `provenance`, and
`visibility`, but nothing in the tool knows those names. An omitted or empty
`requires:` means everything qualifies.

**What counts as a skill.** The `skills:` gate is an ordinary field predicate.
`type: [skill]` is a convention, not a rule.

**Any field you like.** `domain`, `owner`, `tags`, `last_verified`. Quartermaster
carries them through into the catalog untouched, and a consuming repository can
filter its knowledge tree on any of them.

## Three selectors, and only one is frontmatter

Worth separating, because they are easy to conflate:

| Mechanism | Selects on | Decided by |
|---|---|---|
| `include` / `exclude` | **path globs** | the store |
| `requires:` | **frontmatter fields** | the store picks which |
| rulesets | **document ids** | the store |

So what ships is a path question, what may become a rule is a frontmatter
question, and what actually becomes one is a naming question.

## The base schema

`qm bundle init` scaffolds a schema containing exactly the fields above. That is
where the "base schema" lives: a default you are handed, not a contract that is
enforced.

Which means it can drift. Delete `scope` from your schema and the tool keeps
honoring a `scope:` your own CI now rejects. Delete `visibility` and you can no
longer express the one exclusion the build enforces unconditionally. Neither
produces an error, because both files are behaving exactly as designed and the
disagreement is between them.

If you edit the scaffolded schema, edit it by adding.
