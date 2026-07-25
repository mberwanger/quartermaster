---
title: Walkthrough
type: guide
description: Build a bundle from a store, install it into a scratch repository, and watch every moving part. Copy-pasteable, sandboxed, and safe to run repeatedly.
---

# Walkthrough

`scripts/acceptance.sh` proves this works. This walks you through the same
ground slowly, so you can see why each step does what it does and poke at it.

Everything below writes into one temporary directory and four redirected state
directories. Nothing touches your real cache, usage log, spool, or facets.

## Setup

```bash
export QM_TRIAL="$(mktemp -d)"
export QM_CACHE_DIR="$QM_TRIAL/cache"
export QM_USAGE_DIR="$QM_TRIAL/usage"
export QM_TRACE_DIR="$QM_TRIAL/spool"
export QM_FACET_DIR="$QM_TRIAL/facets"

STORE=~/Development/admiral/admiral-knowledge/store
cd ~/Development/mberwanger/quartermaster && go build -o "$QM_TRIAL/qm" .
QM="$QM_TRIAL/qm"

echo "$QM_TRIAL"
```

When you are done: `rm -rf "$QM_TRIAL"` and open a new shell.

---

## 1. The producer side: a store becomes a bundle

A store is plain markdown with frontmatter. Nothing about it is Quartermaster
specific, and deleting every Quartermaster file would leave a working knowledge
repository behind.

```bash
$QM bundle validate --root "$STORE"
$QM bundle build --root "$STORE" --out "$QM_TRIAL/dist" --repo admiral-knowledge --commit local
ls "$QM_TRIAL/dist"
```

Six things come out:

| File | What it is |
|---|---|
| `meta.json` | Format version, source repo and commit, counts, and the digest |
| `catalog.json` | Every distributed document's frontmatter, for orientation |
| `rulesets.json` | The compiled rulesets: names, document ids, resolved scope |
| `store.md` | Every document concatenated, for an agent that stuffs a prompt |
| `store/` | The store tree, carried verbatim |
| `controls/` | Canary fixtures, partitioned away from anything an agent grounds on |

**The first thing worth understanding.** The bundle is addressed by digest, and
the digest deliberately excludes the commit:

```bash
jq '{digest, source, docs, rulesets}' "$QM_TRIAL/dist/meta.json"
```

Two builds of identical content from different commits produce the same digest.
That is what makes it usable as a cache key and as a pin: a repository that says
it is on `sha256:b752…` is making a claim about content, not about history.

**The second.** Compare what the store holds with what the bundle carries:

```bash
find "$STORE" -name '*.md' | wc -l          # everything in the store
jq 'length' "$QM_TRIAL/dist/catalog.json"    # what ships
```

The difference is `bundle.yaml`'s `exclude`. `meta/**` and
`engineering/knowledge/**` describe how the store itself operates, so they are no
use on a consuming repository's disk.

**The third, and the one that surprises people.** Look at what a ruleset
actually is:

```bash
jq '.' "$QM_TRIAL/dist/rulesets.json"
```

No prose. A ruleset is a named list of document ids with optional scope. The
documents live in the store either way; the ruleset only decides which ones get
pushed at an agent. Delete every ruleset and you remove all injection and lose no
knowledge.

---

## 2. The consumer side: a repository takes delivery

```bash
REPO="$QM_TRIAL/repo"
mkdir -p "$REPO" && git -C "$REPO" init -q
git -C "$REPO" remote add origin git@github.com:example/demo.git
git -C "$REPO" commit -q --allow-empty -m initial

$QM init --dir "$REPO" --source "file://$STORE" --target claude --ruleset voice
```

Read the output. It reports the digest it resolved, the rulesets it applied, how
many rules are resident versus scoped, the resident byte count, and how many
documents landed as retrievable.

What `init` wrote:

```bash
cat "$REPO/.quartermaster.yaml"
git -C "$REPO" diff --stat HEAD -- .gitignore
ls "$REPO/.git/hooks/" | grep post-
cat "$REPO/.claude/settings.json"
```

- **`.quartermaster.yaml`** is the only file you edit. It names sources,
  rulesets, targets, and whether telemetry is on.
- **`.gitignore`** gains the generated paths, because generated output is not
  committed.
- **git hooks** `post-checkout` and `post-merge` run `qm sync --quiet`. They
  exist because generated output is gitignored, so git will not update it when
  you switch branches, and a branch pinning a different digest would otherwise
  leave you with stale rules and no signal.
- **`.claude/settings.json`** gains a `SessionEnd` hook. It is guarded with
  `command -v qm`, so it is inert for anyone who has not installed the tool.

---

## 3. What landed, and why it landed there

```bash
find "$REPO/.claude" "$REPO/.quartermaster" -type f | sort
```

Four different destinations, and the difference between them is the whole design.

**Rules** in `.claude/rules/qm/`. Open one:

```bash
cat "$REPO/.claude/rules/qm/voice.base.md"
```

The body is the document's prose verbatim, with a generated header naming the
source id and the bundle digest. No summarizing happens: a rule is the document,
not a paraphrase of it, so there is never a second statement of the same claim
with nothing checking that the two agree.

**Knowledge** in `.quartermaster/knowledge/`, the whole tree:

```bash
ls "$REPO/.quartermaster/knowledge/"
```

This costs no context. It sits on disk and enters a session only when an agent
opens a file. Everything in the bundle is retrievable; a ruleset is what makes a
subset additionally resident.

**State** in `.quartermaster/state.json`:

```bash
jq '{bundles, targets, files: (.files | length)}' "$REPO/.quartermaster/state.json"
```

This is what makes pruning possible. Generated files are gitignored, so nothing
else on the machine knows they were ours.

### Resident versus scoped

```bash
$QM init --dir "$REPO" --force --source "file://$STORE" --target claude --ruleset voice-authoring
head -6 "$REPO/.claude/rules/qm/voice.reference.md"
```

`voice-authoring` selects four documents, and three declare
`scope: ["**/*.md"]`. Those render with a `paths:` frontmatter field, so the
harness loads them only when markdown is open. The fourth has no scope and loads
every session.

That distinction is the budget. `qm status` will tell you what you are spending:

```bash
$QM status --dir "$REPO"
```

---

## 4. Skills and agents are opted into by name

Rules arrive by ruleset. Skills and agents do not: a repository names each one it
wants, because a skill costs resident metadata in every session and an agent is a
capability grant rather than text.

Open `.quartermaster.yaml` and add two lines under the bundle entry, indented to
match `rulesets:`:

```yaml
    skills: [skills.go-interfaces]
    agents: [agents.code-reviewer]
```

Or do it in place:

```bash
python3 - "$REPO/.quartermaster.yaml" <<'PY'
import sys, pathlib
p = pathlib.Path(sys.argv[1]); s = p.read_text()
s = s.replace("    rulesets: [voice-authoring]",
              "    rulesets: [voice-authoring]\n    skills: [skills.go-interfaces]\n    agents: [agents.code-reviewer]")
p.write_text(s)
PY
```

Then:

```bash
$QM sync --dir "$REPO"
find "$REPO/.claude/skills" "$REPO/.claude/agents" -type f | sort
```

Two things to notice.

**A skill is a directory** and it carries its own `.gitignore` that ignores
itself. Generated skills sit beside hand-written ones in `.claude/skills/`, so no
single pattern at the repository root could separate them.

**An agent is filed by its document id**, at
`.claude/agents/qm/agents.code-reviewer.md`, while the skill is filed by its
declared name. The id makes a generated agent unable to collide with one you
wrote. The two content types disagree with each other here, which is a real
inconsistency and is asserted as-is in the acceptance test rather than papered
over.

---

## 5. Drift, and why `verify` exists

Generated files are not committed, so nothing normally notices if one changes.

```bash
echo "an edit nobody asked for" >> "$REPO/.claude/rules/qm/voice.base.md"
$QM verify --dir "$REPO"; echo "exit: $?"
```

It fails and names the file. Repair it:

```bash
$QM sync --dir "$REPO" && $QM verify --dir "$REPO" && echo "clean"
```

Deletion is caught the same way:

```bash
rm "$REPO/.claude/rules/qm/voice.base.md"
$QM verify --dir "$REPO"; echo "exit: $?"
$QM sync --dir "$REPO"
```

`verify` is what belongs in CI. `sync` is what belongs in a git hook.

---

## 6. One payload, several harnesses

```bash
python3 - "$REPO/.quartermaster.yaml" <<'PY'
import sys, pathlib
p = pathlib.Path(sys.argv[1])
p.write_text(p.read_text().replace("targets:\n  - claude\n", "targets:\n  - claude\n  - cursor\n  - agents-md\n"))
PY

$QM sync --dir "$REPO"
ls "$REPO/.cursor/rules/qm/" && head -20 "$REPO/AGENTS.md"
```

The same documents, three shapes. Cursor takes `.mdc` files with `globs` and
`alwaysApply`; Claude takes `paths:`. `AGENTS.md` is different in kind: it is
**committed**, and it holds a managed block spliced between markers rather than a
generated file. The block is a pointer — the bundle digest, the rulesets, the
rules and their scope — not the rules inlined. Anything you write outside the
markers survives every sync.

`qm targets` lists what is available.

---

## 7. Pruning

Remove a ruleset and the files it produced go away:

```bash
python3 - "$REPO/.quartermaster.yaml" <<'PY'
import sys, pathlib, re
p = pathlib.Path(sys.argv[1])
p.write_text(re.sub(r"^\s*rulesets:.*$", "    rulesets: []", p.read_text(), flags=re.M))
PY

$QM sync --dir "$REPO"
find "$REPO/.claude/rules/qm" -name '*.md' | wc -l
ls "$REPO/.quartermaster/knowledge/" | head -3
```

Sync reports what it removed (`pruned 8`), the rule count is now zero, and the
knowledge tree is untouched. Removing a ruleset removes
injection, not knowledge, which is the same point `rulesets.json` made in step 1
from the other direction.

---

## 8. Telemetry, and what it will and will not tell you

Two hooks, both opt-in per repository through `telemetry:` in the manifest, both
silent no-ops when anything is off.

```bash
DOC=$(find "$REPO/.quartermaster/knowledge" -name '*.md' ! -name index.md | head -1)
$QM usage record "$DOC"
cat "$QM_USAGE_DIR"/*.jsonl | jq -c '{id, repo, worktree}'
```

Note `repo` is `github.com/example/demo`, derived from the remote rather than
from the path. Every worktree of one repository counts as one, which is what
makes the cross-repository spread mean anything.

```bash
printf '{"session_id":"walkthrough-1","cwd":"%s","hook_event_name":"SessionEnd"}' "$REPO" | $QM trace record
jq -c '{session_id, repo, branch, bundles}' "$QM_TRACE_DIR/pending.jsonl"
```

A read outside the knowledge tree records nothing:

```bash
$QM usage record "$REPO/.quartermaster.yaml"; echo "exit: $?"
```

`qm stats` reads the log. It will tell you which documents nobody opens, which
usually means the description is wrong, since that is all an agent sees when it
decides whether to open something. It cannot tell a document that helped from one
an agent opened and discarded, and it never proposes demoting a rule, because a
resident rule is delivered by the harness rather than opened.

---

## Things worth trying

- **Two sources.** Add a second `--source` and watch precedence: on an id
  collision, the later source wins. Order is declared, never inferred.
- **Break the pin.** Edit `digest:` in the manifest to something wrong and run
  `qm status`. It warns rather than silently resolving.
- **A budget.** Add `budget:\n  resident_bytes: 1000` and sync. It warns when the
  resident set exceeds it.
- **A knowledge filter.** Add `knowledge: {domain: [engineering]}` under the
  bundle and see the tree shrink. This is a relevance filter, not a permission
  one: what may become a rule is still decided by the store.
- **`qm bundle explain <id>`**, run in the store, tells you whether a document
  qualifies to become a rule and why not if it does not.
- **A file:// source pointed at a source tree** gets built on the fly, which is
  the authoring inner loop. CI should refuse `file://`.

## Cleanup

```bash
rm -rf "$QM_TRIAL"
```
