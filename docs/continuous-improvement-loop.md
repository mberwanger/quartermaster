---
title: Quartermaster Continuous Improvement Loop
type: feature-spec
status: draft
owner: martin
verified_against_code: 2026-07-24
description: Session telemetry capture, local analysis, and evidence-backed proposals that populate and prune the knowledge store, closing the loop between agent sessions and bundle contents.
---

# Quartermaster Continuous Improvement Loop

## 0. How to read this document

This spec is written to be dropped into a fresh implementation session with no prior conversation. It carries the background, the reasoning, the rejected alternatives, and the open questions, not only the requirements.

**Verification status.** The first draft was reconstructed from conversation and carried `[VERIFY]` markers on every claim about current state. Those markers have been resolved against the source on 2026-07-24, in `quartermaster` at `d7158c3` and `admiral-knowledge` at `f279f46`. Claims about what exists now cite the file that proves them. Claims about what is intended carry no citation and remain the author's judgment.

What the verification changed, in order of consequence:

1. **Open question 1 is closed.** Skills and agents are already document types, and rules are already a selection over documents rather than a type. Section 6.4 was wrong about the enum and about the gap. This unblocks implementation.
2. **The distiller agent does not exist.** Section 2.5 treated it as reusable. It is a name in a target-state diagram. Section 7's faster path to a corpus went with it.
3. **Three worktree defects were live in shipped code**, not merely risks for the new code. They are fixed as of this revision. See section 4.11.
4. **`~/.quartermaster/cache/` is already taken** by the bundle cache. The draft proposed the same path for analyzer memoization.
5. The bundle carries `rulesets.json` and `store.md`, which the draft omitted. `rulesets.json` is where rule-level telemetry has to join.

---

## 1. Problem statement

Quartermaster distributes rules, skills, and knowledge into repositories so that agent sessions start primed rather than rediscovering the environment every time. The distribution mechanism works. The unsolved problem is the payload.

There is currently no way to answer three questions.

1. Is the content in the store actually useful to a session?
2. What is missing that should be there?
3. What is present that should not be?

Today the payload is authored on judgment. Documents are written because they seem valuable. Nothing observes whether an agent read them, whether reading them helped, or whether their absence cost anything. That makes the store unfalsifiable. A bad document and a good document look identical from the outside.

The goal is a flywheel. Sessions produce evidence, evidence produces proposals, a human accepts or rejects proposals, accepted proposals rebuild the bundle, and the next session starts better informed. Repeat until the curve flattens.

The tool already says this out loud. `qm stats` prints that it "cannot tell a document that helped from one an agent opened and discarded" (`cmd/stats.go:38`). This spec is the work of removing that sentence.

### Why this matters more than it appears

Repository context files carry a measured cost. A February 2026 study from ETH (Gloaguen et al., *Evaluating AGENTS.md*, arXiv:2602.11988) ran four coding agents across 438 tasks and found that auto-generated context files reduced task success by roughly three percent, developer-written files improved it by roughly four percent, and both increased inference cost by more than twenty percent. Trace analysis showed instructions were followed faithfully, producing broader exploration and more testing, but context files did not function as effective repository overviews.

Two implications carry directly into this design.

- Content that is discoverable from the codebase does not earn its place in a bundle. The agent can already find it, and putting it in the bundle costs tokens without adding information.
- The one unambiguous positive finding was tool-specific instruction. A repository-specific tool averaged 2.5 calls per instance when named in the context file versus 0.05 when not. Naming a capability is what makes an agent reach for it.

The counterweight is that dynamic context does compound where static context does not. ACE (*Agentic Context Engineering*, arXiv:2510.04618) treats context as an evolving playbook refined from execution feedback through incremental delta updates, reporting roughly a ten percent gain on agent benchmarks without labeled supervision. The distinction that matters is not whether to ship context but whether the context is revised from evidence. This spec is the machinery for revising from evidence.

---

## 2. Background a fresh session needs

### 2.1 Quartermaster

Quartermaster (`qm`) is a Go CLI plus a per-repo YAML manifest. It both compiles bundles and consumes them, which is why profiles were dropped from an earlier design. A producer can emit different bundles rather than a consumer selecting a profile at install time.

`qm init` (`cmd/init.go`) resolves one or more sources, writes `.quartermaster.yaml`, adds gitignore entries, installs git hooks, and runs the first sync. `--source` is repeatable and ordered, and order is precedence: on an id collision the later source wins (`cmd/init.go:51`). So is `--ruleset`, whose requested order is deliberately preserved rather than sorted, because ruleset order is also precedence. Targets are detected from repository markers (`.claude`, `CLAUDE.md`, `.cursor`, `AGENTS.md`) and default to `claude`.

**Correction to the first draft.** `qm init` does not configure usage telemetry in any active sense. It writes `telemetry: true` into the manifest and stops. The hooks it installs are git hooks, `post-checkout` and `post-merge`, which re-sync materialized output that git does not track. **No harness hook is installed by any code path.** The `PostToolUse` wiring that feeds the usage log is a snippet the operator pastes into `.claude/settings.json` by hand, documented in `README.md`.

That matters for section 4.2, which needs a `SessionEnd` hook. There is no existing harness-hook installer to extend. It is new work, and it is the first time Quartermaster will write into a harness settings file.

### 2.2 The knowledge store

`admiral-knowledge` is a plain markdown repository following the Open Knowledge Format (OKF), Google Cloud's vendor-neutral spec for representing knowledge as a directory of markdown files with YAML frontmatter. In OKF, a concept is one file, its identity is its path with the `.md` suffix removed, `index.md` files are optional and carry no frontmatter, and concepts cross-link with ordinary markdown links.

An earlier decision (`0003 additive-over-okf`) established that anything agents need beyond OKF is layered on top of a conformant store rather than changing it, and every addition must be reproducible from the source tree alone and safe to delete. This spec must respect that constraint.

`make bundle` runs `qm bundle build --root store --out dist`. The artifact (`internal/bundle/write.go`) is:

| Path | Contents |
|---|---|
| `meta.json` | `format`, `okf_version`, `name`, `source{repo,commit}`, `docs`, `files`, `rulesets`, `controls`, `store_bytes`, `digest` |
| `catalog.json` | Every distributed document's id, path, and whole frontmatter |
| `rulesets.json` | Compiled rulesets: named ordered lists of document references with resolved scope |
| `store.md` | Every document concatenated in path order, for a consumer that stuffs a prompt |
| `store/` | The store tree, verbatim |
| `controls/` | Canary and control fixtures, partitioned out of everything an agent grounds on |

Bundle format version is `0.3` (`internal/bundle/bundle.go:41`); the OKF version it declares is `0.1`.

Two properties of the digest matter downstream. It is computed over the catalog, the compiled rulesets, `store.md`, and every carried file, and it **excludes the source commit** (`internal/bundle/bundle.go:428`), so two builds of identical content from different commits agree. That is what makes it usable as a cache key and an eval pin. It also means the digest alone cannot distinguish two builds of the same content, and only `source.commit` can. Record both.

`rulesets.json` is the file the first draft omitted and the one section 4.7 depends on. A rule is not a property of a document. It is membership in a compiled ruleset. Nothing else in the artifact can tell you whether a document was resident in a session.

### 2.3 The three artifact types and their consumers

The store distributes three kinds of thing, and they have different consequences depending on who is consuming.

| Artifact | Resident in context | Operator session | Unattended agent |
|---|---|---|---|
| Rules | Yes, every session, budget under roughly 8k | Yes | No |
| Skills | Metadata resident, body on demand | Yes | Possibly, decided per role |
| Knowledge | No, on disk, read on demand | Yes | Yes |

Rules are harness and operator scoped. They describe how work gets done in this repository with this tool. An unattended agent has its own operating instructions from its own system prompt, so shipping rules to it is mostly noise.

Knowledge is consumer agnostic. Why a thing is built a certain way is true regardless of who asks.

Skills sit in between and split by role rather than by harness. If an unattended agent opens PRs and writes decision records, the skills for those procedures apply to it exactly as they apply to an operator.

This split is realized as bundle composition at build time, not as install-time profiles. The producer emits an operator bundle and an agent bundle from the same store.

A refinement the code adds: residency is not binary per document. The `claude` target renders an unscoped rule as always resident and a scoped rule with `paths:` frontmatter, so a scoped rule enters context only when a matching file is open. The real store already uses this: `voice-authoring` declares `scope: ["**/*.md"]`, so three of its four rules cost nothing until markdown is open. Any telemetry claiming a rule was resident in a session has to account for scope, or it will credit rules that never loaded.

### 2.4 Cost model, which is not what it first appears

Knowledge lives on disk and is not loaded into context, so it carries no resident-token cost. The resident slice is rules, plus skill metadata, plus whatever pointer text sits in the rules file. Quartermaster already tracks this against `budget.resident_bytes` and warns on `sync` and `status`. The real store measures 2471 B resident under the `voice` and `voice-authoring` rulesets.

The on-disk store still has a cost, but a different one. It is read amplification and misretrieval. An agent that reads eleven documents before finding the useful one paid real tokens and real wall-clock time. An agent that reads a plausible but wrong document paid worse, because it stopped looking and proceeded confidently. The lever is index quality and description discriminability, not payload size. Two documents whose descriptions do not clearly separate will cost reads forever.

**Critical dependency.** Nothing auto-loads `index.md`. The OKF spec describes it as a navigation aid for progressive disclosure, and no harness treats a file by that name as special. The only things entering a session unbidden are the rules file, nested rules files where the harness supports them, and skill frontmatter. Therefore the rules file must name the store root and state when consulting it is expected. Without those few lines the entire knowledge store is invisible and telemetry will report zero reads, which reads as a quality problem when it is actually a wiring problem.

The `agents-md` target already renders a pointer block naming the bundle digest, the rulesets, and the rule listing. Confirm before the baseline window opens that whichever target a measured repository uses names the knowledge root too. A two-week baseline measured through a store nothing pointed at measures nothing.

### 2.5 What already exists elsewhere, corrected

The first draft listed five things as existing and reusable. Three do, two do not.

**Exists.** The merge gate on the store repository: CI validation (`.github/workflows/validate.yml` runs `qm bundle validate`, the index check, and a packaging step), branch protection, and `CODEOWNERS` routing every content directory to `@admiral-io/platform`. Human merge is the truth anchor and agent-authored content stays draft until a human merges.

**Exists, as declarative configuration rather than as a running process.** The store-honesty model across time, provenance, coverage, and the human anchor is encoded in `store/bundle.yaml` under `requires:`, which admits only `status: active` and `provenance: [verified, decided]` and excludes `visibility: restricted`. The gate is evaluated at build time (`internal/gate`), so a consumer evaluates no predicates and a draft cannot leak into rules. Trust decay is configured in `store/meta/freshness.json`, which sets per-type windows in days since `last_verified`.

**Does not exist.** The distiller agent. `admiral-agents` contains exactly one agent, `cmd/yeoman` and `internal/agents/yeoman`. The distiller appears only in that repository's README, inside a layout the README itself labels "the destination, not what exists today". Its design document in the store, `engineering.change-context-capture`, is `status: draft, provenance: asserted`, so under the store's own gate it does not even qualify as a rule. There is no Sentry support agent and no PLG agent either.

**Does not exist.** The scheduled reviewer worker and the NATS and CloudEvents event fabric. `internal/platform` holds `config`, `github`, `knowledge`, `model`, and `obs`. There is no `events` package. The event fabric was already out of scope for v1; the correction is that it is not merely deferred, it is unbuilt, so no part of this design may assume it as a fallback.

Consequence for this spec: **`qm digest --backfill` over local transcripts is the only path to a corpus.** Section 7's claim that facets could be collected from existing unattended agents instead is false today and costs a new agent to make true.

Two smaller findings, unblocking but worth fixing while nearby. `CODEOWNERS` still routes `/tool/`, which no longer exists after the bundler moved into Quartermaster. And `freshness.json` has no window for `skill` or `agent`, the two types most likely to go stale silently, because a procedure that no longer works still reads as authoritative.

---

## 3. Scope

### 3.1 In scope for v1

Single machine. Local filesystem. Manually invoked analysis. One operator. No network services of any kind.

The loop must be demonstrably valuable at N equals one, or it will not survive contact with anyone else.

### 3.2 Explicit non-goals for v1

- No transport service, no ingest endpoint, no NATS publishing, no message fabric.
- No centralized aggregation across developers or machines.
- No automated PR creation. Proposals land on disk for human handling.
- No statistical claims. Volume at one operator is too low. Output is qualitative clusters with evidence attached.
- No change to how bundles are distributed. **The first draft said "git remains the distribution mechanism", which was already stale.** `oci://`, `git+https://`, `https://`, and `file://` providers all exist, `qm bundle publish` pushes an OCI artifact, and resolved bundles are cached by digest. This feature adds nothing to that surface and depends on none of it beyond reading `meta.json`.

### 3.3 Known future direction, to be designed for but not built

The eventual multi-developer version (target roughly 25 to 30 developers, many working across several repositories) needs exactly one capability that the local version does not have, which is aggregation. A single operator proposing from their own sessions is proposing from a sample of one. What makes a candidate trustworthy is that the same question burned turns for several people across several repositories, and no amount of git access provides that, because the facets never meet.

This matters because it narrows what the future service is. It is an analysis and aggregation service. It is not a proposal service, since git handles the write path better than any service would, permanently.

---

## 4. Design

### 4.1 Pipeline overview

```
session ends
  -> SessionEnd hook appends one line to spool          (automatic, cheap)
  -> qm digest reads transcripts, emits facet records   (manual, one model call per session)
  -> qm gaps clusters facets, checks against store      (manual)
  -> proposals written to disk as draft documents       (manual review)
  -> human moves survivors into knowledge checkout, commits
  -> merges accumulate on master, master is tagged
  -> bundle built and released from the tag
  -> qm update pulls the new bundle
  -> next session starts better informed
```

### 4.2 Capture

Capture is deliberately minimal because the transcript already holds nearly everything.

Claude Code hooks deliver a common envelope on stdin containing `session_id`, `transcript_path`, `cwd`, and `hook_event_name`, across three cadences: once per session (SessionStart, SessionEnd), once per turn (UserPromptSubmit, Stop), and once per tool call (PreToolUse, PostToolUse). The transcript at `transcript_path` is a JSONL file containing the tool calls, tool results, reasoning spans, and timestamps for the session.

Because the transcript already has the tool sequence and the reasoning, do not install per-tool-call hooks for this purpose. They run inside the session while the operator waits, and a slow PostToolUse hook makes every session sluggish for no added information.

The existing `qm usage record` PostToolUse hook is the exception and stays. It answers a question the transcript cannot: it resolves a read path to a document id and to the bundle digests installed at that moment, which is repository state rather than session state. Its cost is bounded by design, and it is documented in the command's own help text as never failing loudly and never blocking.

**SessionEnd hook.** Calls `qm trace record`, which appends one line to the spool and exits zero unconditionally. It must never block and must never fail loudly.

Recorded fields.

| Field | Why |
|---|---|
| `session_id` | Join key |
| `transcript_path` | Where the digest reads from |
| `repo` | Repository identity, from `repo.Identity` and never from the working path |
| `worktree` | The checkout, as a separate dimension |
| `bundle_versions` | The one field the transcript cannot reconstruct |
| `ended_at` | Windowing |

**Bundle version is the single most important captured field.** Without it, later analysis can count what happened but can never attribute a change to a document, because sessions before and after a document landed are indistinguishable. Record every installed bundle, not the first: a repository composes sources in precedence order and nothing cheap knows which one carried a given document.

**SessionStart hook.** Fires once while the operator is already waiting, so a freshness check there is effectively free. It warns when the installed bundle is behind the tracked ref and records staleness into the session record, so that later analysis is not silently measuring stale payloads.

Both hooks need Quartermaster to write into a harness settings file, which it has never done. Section 4.11 and open question 8 decide where that file lives.

### 4.3 Storage layout

User scoped, not repository scoped, because the interesting signal is cross-repository even on one machine.

```
~/.quartermaster/
  cache/<kind>/<digest>/     IN USE: resolved bundles, keyed by digest   (QM_CACHE_DIR)
  usage/<YYYY-MM>.jsonl      IN USE: per-read open events                (QM_USAGE_DIR)
  spool/pending.jsonl        new: append-only session pointers
  facets/<session-id>.json   new: one facet record per digested session
  digested/                  new: analyzer memoization
```

**Correction to the first draft**, which proposed `cache/` for analyzer memoization. That path is the bundle cache (`internal/cache/cache.go`) and a build that cleared it to reclaim analyzer space would silently discard pinned bundles. Use a distinct directory.

Follow the established override convention. Both existing directories honor a `QM_*_DIR` environment variable so that tests and CI stay off a developer's real data. The spool and facet directories must do the same, or the first `qm digest` test run will write into the operator's live corpus.

Plain files. JSONL where it appends. No database until files demonstrably hurt.

### 4.4 Digest

`qm digest` is manually invoked and idempotent. It drains the spool, reads each transcript, makes one model call per session, writes a facet record, and marks the session processed.

- `qm digest --since 7d` for a window.
- `qm digest --backfill` sweeps transcripts already on disk under the harness's own project directory, so a usable corpus exists immediately rather than after weeks of accumulation. This is the recommended first action after implementation, and per section 2.5 it is now the only path to a corpus.
- `qm digest --rerun --since 30d` reprocesses local transcripts when the facet schema changes, so the extraction prompt is not a one-way door.
- `qm digest --show <session>` prints exactly what a facet record contains. This exists for inspectability and matters for adoption later.

Extraction runs locally and only facet records ever leave the machine. Raw transcripts stay local under a retention window. This is both the privacy enforcement point and the reason the schema can evolve without losing history.

**Signal priority.** The agent's own reasoning text is the primary input, not the user's prompts. A reasoning span stating what the agent is trying to establish, followed by a spread of exploratory tool calls, is the shape being detected. User input is secondary and largely uninteresting.

**Honest limitation.** Reasoning text is a rationalization, not a cause. It is an excellent cheap candidate generator and it is not evidence. Reasoning spans may also be absent depending on configuration, so the digest must degrade to tool-sequence analysis when they are.

**Note on backfill and identity.** Harnesses key their project directories off the working path, so a naive backfill sweep sees one repository as three when three worktrees exist. Resolve each transcript's `cwd` through `repo.Identity` before writing the facet, exactly as capture does. This is the one place where the fix in section 4.11 does not apply automatically, because the path is coming from the harness rather than from Quartermaster.

### 4.5 Facet record

This is the wire format and the one genuinely irreversible decision in the spec. Whatever ships to a future aggregation service ships facets. Version it from day one, make it self-contained, and put repository identity in every record because backfilling that later is impossible.

Keep the schema closed-form. A vague extraction prompt yields mush.

```json
{
  "facet_version": 1,
  "session_id": "...",
  "repo": "github.com/admiral/admiral-api",
  "worktree": "/path/to/worktree",
  "branch": "feature/x",
  "harness": "claude-code",
  "model": "...",
  "bundle_versions": [{ "source": "...", "digest": "...", "commit": "..." }],
  "bundle_stale": false,
  "started_at": "...",
  "ended_at": "...",
  "questions": [
    {
      "question": "why does the event envelope keep the raw payload verbatim",
      "resolution": "source_read",
      "resolved": true,
      "tool_calls": 14,
      "elapsed_seconds": 190,
      "store_docs_read": ["engineering/eventing-conventions"],
      "arrived_via": "index"
    }
  ],
  "store_reads": [
    { "doc_id": "...", "arrived_via": "index|direct", "followed_by_further_exploration": true }
  ],
  "resident_rules": ["voice.base"],
  "outcome": "edit_landed|pr_opened|abandoned|unknown",
  "corrections": 2
}
```

Resolution values are `store_read`, `source_read`, `bash_exploration`, `asked_human`, and `unresolved`.

Three changes from the first draft, all forced by verification.

- `repo` is the remote-derived identity, never a path. The shape shown is what `repo.Identity` returns.
- `bundle_versions` carries `commit` alongside `digest`, because the digest deliberately excludes the commit (section 2.2) and two different builds of one content tree share a digest.
- `resident_rules` is new. It is the ruleset membership resolved through scope, and without it section 4.7 cannot produce any rule signal at all. It comes from `rulesets.json` plus the target's scope rendering, not from the documents.

### 4.6 Analysis

`qm gaps` reads a facet window, clusters recurring questions semantically, and produces ranked candidates with session identifiers attached so the operator can read the underlying transcript.

The clustering approach follows the pattern Anthropic published for Clio, which extracts per-conversation facets, clusters them semantically, generates cluster descriptions, and organizes them hierarchically. The value is bottom-up discovery, since top-down evaluation only finds what was already suspected.

**The first analysis step is the one most implementations skip.** For each recurring question, retrieve against the currently installed store before proposing anything. This splits output into two categories with entirely different fixes.

| Category | Meaning | Fix |
|---|---|---|
| Content gap | Store cannot answer the question | Propose a new document |
| Discoverability gap | Store answers it but the agent still burned turns | Fix the index entry, the description, or the rules pointer |

At any meaningful scale discoverability failures will outnumber content failures. Conflating them produces duplicate documents that do not help because the original was already there and simply could not be found.

**Ranking is frequency times non-recoverability, not frequency alone.** Frequency alone builds a stale mirror of the codebase, because "what" questions are the most common thing an agent asks. Structure and behavior are recoverable from source at some token cost and they age fast, so a document about them is a bet that the document stays truer than the code, and that bet loses on a schedule. Rationale, constraints, rejected alternatives, and the reason a thing is weird are not recoverable at any token budget and age slowly. If an agent can answer the question by reading one file, let it read the file.

Third step, deduplicate against open proposals so the same candidate does not surface week after week.

### 4.7 Removal signals

Removal is a first-class output, not an afterthought, and it applies to rules and skills as well as to knowledge documents. There are three distinct reasons with different evidence and different strength.

| Signal | Detection | Strength |
|---|---|---|
| Unused | Document never read across the window | Weak. The topic may simply not have arisen |
| Failed | Document read, agent continued exploring afterward | Strong. It was consulted and did not answer |
| Wrong | Document read, resulting work corrected by the human | Strongest signal in the system |

The wrong-document case deserves emphasis. A store that confidently states something false is worse than a store that is silent, because the agent stops looking and proceeds. This feeds directly into the existing provenance and trust-decay model.

Each artifact type has its own verb, and the telemetry must reflect that.

| Artifact | Positive signal | Negative signal | Joins on |
|---|---|---|---|
| Knowledge | Read, and no further exploration followed | Never read, or read and exploration continued | `catalog.json`, usage log |
| Skill | Invoked, and the task completed | Never invoked, or invoked and abandoned | `type: skill` in frontmatter |
| Rule | Complied with | Violated, or never applicable | `rulesets.json` plus rendered scope |

The join column is the correction. Skills and agents are document types and can be found in the catalog. **Rules cannot**, because no document is a rule. A document becomes a rule by being named in a compiled ruleset, and `voice.base` is `type: policy` in the store. Any implementation that looks for `type: rule` will find nothing and report every rule as unused.

`qm stats` already refuses to propose demotions for exactly this reason (`cmd/stats.go:124`): a resident rule is delivered by the harness rather than opened, so its absence from the read log means nothing. Rule signals must come from the session, through compliance and applicability, not from the usage log. A scoped rule adds a second trap: it is only resident when a matching path is open, so "never applicable" and "never loaded" are different findings and only the first is a removal argument.

### 4.8 Proposals

`qm gaps --drafts ./drafts` emits candidate documents with their evidence in the frontmatter, meaning occurrence count, repositories affected, observed cost, and supporting session identifiers. The operator reviews, moves survivors into the knowledge checkout, and commits normally. Existing CI validation catches malformed content. No GitHub API, no bot identity, no new merge gate.

Evidence in the proposal body is what makes weekly review fast enough that a human will actually do it.

A draft must satisfy the store's schema or it will fail CI on arrival, and the schema requires `id`, `title`, `description`, `domain`, `type`, `status`, `provenance`, and `owner`. Emit `status: draft` and `provenance: asserted`, which is honest for a machine-generated candidate and, under the store's `requires:`, keeps it out of every ruleset until a human promotes it. That is the correct default: a proposal should be retrievable and reviewable without becoming resident in twenty-one repositories.

**Future path, not v1.** Bundle `meta.json` records the source repository and commit, so a later `qm propose` is straightforward: read `meta.json`, learn which repository produced the installed bundle, fetch it to a scratch directory, branch, write the draft, push using the developer's own git credentials, and open a PR. Read from a compiled artifact, write to the source it names. No service is required for this, now or ever.

This requires one schema addition to `meta.json`, an optional contribute target. Verification confirms the addition is genuinely needed rather than merely convenient: `source.repo` is a free-form string passed by `--repo`, and the real store passes `admiral-knowledge`, not a URL. There is nothing to clone. Not every bundle has a writable source in any case. Some are generated, some come from another team. When there is no contribute target, `qm propose` writes the draft locally and says plainly that there is nowhere to send it. Where a candidate spans multiple source bundles, route to whichever bundle's source owns the gap, and ask rather than guess when genuinely ambiguous.

### 4.9 Local overlay tier

A personal knowledge store alongside the pulled bundle, realized as an additional source. `qm init --source` is repeatable and ordered, so this needs no new mechanism, and a `file://` source can point at a source tree that `qm sync` builds on the fly, which is exactly the authoring inner loop a staging tier wants.

The purpose is a staging tier where a document proves itself before anyone else is asked to review it. The risk, correctly identified, is that an uncurated local store grows idiosyncratic to one developer and turns promotion into a single large low-quality proposal.

The mitigation is cadence plus evidence. Promotion is continuous and per-document rather than a batch event, and the telemetry supplies the promotion criterion. A local document read repeatedly across sessions that shortened discovery carries evidence. One never read carries none.

Two things must be specified during implementation.

- **Precedence.** Source order already decides id collisions, later wins, so the mechanism exists. What is undecided is which order an overlay should take and whether the operator should be told when their local document shadowed an upstream one rather than merely finding that it did.
- **Contradiction.** What happens, and what is reported, when a local document contradicts an upstream one without colliding on id. Nothing detects this today. The store carries deliberate contradiction canaries under `controls/` for exactly this kind of detector, which is the natural fixture to build against.

### 4.10 Release

Merge to master runs existing validation and the bundle build. Tag master when a stable point is wanted. `qm update` pulls the ref. Distribution is unchanged by this feature.

### 4.11 Worktrees

Worktrees are in active use and they affect identity, installation, materialization, and concurrency. This is not an edge case to handle later. Verification found three defects of this family already live in shipped code, which is the strongest available argument that the section is not theoretical.

**Identity. Fixed 2026-07-24.** `qm usage record` recorded `Repo` as the absolute filesystem path of the repository root, so every worktree counted as a separate repository in the log that `qm stats` reads. Cross-repository spread is both the ranking signal in section 4.6 and the promotion criterion in `qm stats`, so a document opened in three checkouts of one repository read as a trend across three repositories. `repo.Identity` now derives the name from the remote url, normalizing the ssh and https spellings to one value, and falls back to the shared git directory, which is common to every worktree. The checkout is recorded separately as `worktree`. Facet records must use the same function and must never fall back to a path.

One consequence to accept rather than fix: log lines written before this change carry paths, so the same repository appears under two names until the old events age out of the window.

**Bundle attribution. Fixed 2026-07-24.** The same code recorded only `state.Bundles[0].Digest`, so a repository composing several sources attributed every read to whichever bundle happened to be first. The event now carries every installed bundle as `{source, digest}` pairs, matching `bundle_versions` in the facet schema.

**Hook installation. Fixed 2026-07-24.** `qm init` wrote git hooks into whatever `.git` resolved to, which in a linked worktree is `.git/worktrees/<name>/hooks`. Git resolves hooks against the shared git directory, so those hooks were never run and any worktree initialized that way silently stopped re-syncing after checkout and merge. Hooks now install into the common directory, which is also why they only have to be installed once per repository however many worktrees it has.

**Harness hooks.** Still undecided, and unlike git hooks they are not shared automatically: harness hooks live in project-local settings and are per-worktree unless configured at user level. Decide this deliberately rather than discovering it later, because a worktree that silently records nothing is worse than one that records twice. Whatever is decided, `qm status` should be able to say whether the current worktree is actually recording.

**Materialization.** Writing full bundle content into every worktree duplicates storage and lets copies drift when worktrees are initialized at different times. The content-addressed cache at `~/.quartermaster/cache` already exists and already deduplicates the download across repositories; materialization copies out of it into each worktree. Copying rather than symlinking is correct, since an agent editing what it believes is a local file must not mutate every other worktree.

**Concurrency.** Worktrees exist largely so several agent sessions can run at once, which means simultaneous spool appends. Keep the SessionEnd record small enough to land in a single write and append with `O_APPEND` so concurrent writes do not interleave. The usage log already takes this approach and tolerates a torn line by skipping it rather than failing the file, which is the right precedent. Lock `qm digest` and keep it idempotent by session identifier, because concurrent runs will overlap eventually.

**Local overlay.** User scoped, per section 4.9, for the same reason. Repository scoped fragments the personal store across worktrees and forces the same document to be curated repeatedly.

---

## 5. Metrics

Three, derived from facet records, all sliced by bundle version.

| Metric | Definition | What it tests |
|---|---|---|
| Discovery span | Tool calls and tokens before the first useful action | Whether context reduces flailing. The primary metric |
| Store hit rate | Share of sessions where a store read preceded the first useful action | Whether the store is reachable and relevant |
| Dead weight | Documents never read in the window | Eviction candidates |

**Discovery span is the flywheel's definition of working.** Tracked per repository across bundle versions. If that curve is flat after a quarter, the loop is not working, and that should be discovered from the data rather than inferred from the feeling that it ought to help.

Dead weight is the one metric that already has an implementation. `qm stats` reports "retrievable here, never opened" from the usage log and correctly excludes resident rules from it. Treat that as the existing baseline to improve on rather than as something to rebuild.

**Baseline before payload changes.** Run capture for roughly two weeks with the bundle held constant. Without a baseline the first document added will feel like it helped and there will be nothing to check that feeling against.

**Worktrees make a real counterfactual possible at one operator.** Before-and-after comparison across time confounds the bundle change with the operator's own growing familiarity with the codebase, which is the main reason a solo measurement is usually untrustworthy. Two worktrees on the same repository, same operator, same week, running different bundle versions against comparable tasks removes that confound. This is the only clean comparison available at this scale and it should be the method used to establish that discovery span actually moves rather than merely appears to.

Note that this method depends on the facet record keeping `worktree` distinct from `repo`. The identity fix collapses worktrees for counting purposes, which is right for spread, and the separate field is what keeps the experiment expressible.

**Statistical honesty.** At one operator this is tens of sessions per week. Every number is directional for the first several months. `qm gaps` should optimize for surfacing roughly five clusters worth reading, not for computing a statistic the sample cannot support.

---

## 6. Frontmatter schema audit

A parallel workstream inside this feature, not a separate effort, because the telemetry design depends on what the schema can express.

### 6.1 Why now

Being restrictive is cheap while the store has essentially one consumer. Removal cost scales with how many artifacts encode a field, and that count stops being one the moment facets reference documents by identifier, bundles are published, and repositories pin them. It never goes back down. Bundles are already published by digest, so this window is closing rather than open.

### 6.2 The asymmetry that qualifies the "additions are easy" rule

Additions are easy only for fields a machine can derive later.

- **Derivable fields** such as timestamps, path-derived domain, or digests can be backfilled by script at any time. Be aggressive about cutting these.
- **Judgment fields** such as provenance, owner, or last-verified encode a human decision made at authoring time. Backfilling them means a person re-reading every document and making a call, which costs the same as writing them originally. Be conservative about cutting these.

### 6.3 Audit method, and its result on the current store

For each field, in order.

1. Name the consumer and the branch, meaning the specific code path that reads the field and behaves differently based on its value. Not that it seems useful. If no such path can be named, the field is a deletion candidate.
2. Grep for the field across quartermaster, the grounding gate, retrieval, and the scheduled reviewer. If the only reader is the validator, the field exists solely to satisfy its own schema. That is the strongest deletion signal available.
3. Examine the value distribution across existing documents. A field where every document carries the same value carries no information. A field empty in most documents is already optional in practice regardless of what the schema claims.
4. Decide required versus optional based on whether a document is unusable without it. Distinguish demotion from deletion.
5. Run the same three checks on enum values, not only on fields. An enum value no document uses, or that nothing branches on, is the identical problem one level down.

Steps 1 through 3 have been run. Twenty-two documents carry frontmatter.

| Field | Docs | Consumer that branches |
|---|---|---|
| `id` | 22 | Rulesets, catalog, usage log, link checking. Hard contract |
| `description` | 22 | Rendered into every rule, skill, agent, and index listing |
| `title` | 22 | Skill and agent rendering, index generation |
| `type` | 22 | `skills: type: [skill]` in bundle.yaml, index grouping, freshness windows |
| `status` | 22 | `requires:` gate, via `internal/gate` |
| `provenance` | 22 | `requires:` gate |
| `visibility` | 18 | `requires:` gate, plus a hard exclusion in `bundle.Build` and in `qm bundle explain` |
| `scope` | 3 | Ruleset scope resolution, rendered as `paths:` or `globs:` by the target |
| `skill` | 3 | Skill rendering: name and allowed-tools |
| `agent` | 1 | Agent rendering, and the `bypassPermissions` refusal in `bundle.Build` |
| `supersedes` | 4 | Validator link check only |
| `superseded_by` | 0 | Validator link check only |
| `domain` | 22 | None. Almost every value is `engineering` |
| `owner` | 22 | None. Almost every value is `platform` |
| `created` | 22 | None |
| `timestamp` | 22 | None |
| `tags` | 15 | None |
| `related` | 7 | None |
| `sources` | 2 | None, yet the schema requires it for `reference` and `runbook` |
| `last_verified` | 1 | None in code. `freshness.json` describes a scheduled audit that does not exist |
| `resource` | 1 | None |
| `capability` | 0 | None |

Note that `status`, `provenance`, and `visibility` never appear as literals in the code, because `internal/gate` evaluates arbitrary field predicates declared in `bundle.yaml`. A grep-only audit would wrongly mark all three as unread. Any field named in `requires:` has a consumer by definition.

Unused enum values on the real store: `type` `runbook`, `guide`, and `strategy` at zero documents each; `status` `deprecated` and `superseded` at zero; `domain` `product` and `operations` at zero; `visibility` `restricted` at zero, though its code path is the strictest invariant in the build and should be kept regardless of current usage.

Applying section 6.2 to that table: `capability` and `resource` are clean deletions, being both unused and unread. `domain` and `tags` are derivable, single-valued in practice, and cuttable. `owner`, `provenance`, and `last_verified` are judgment fields, so demote rather than delete even where nothing reads them yet, because a scheduled audit is a plausible near-term consumer of the last of those and re-deriving it costs a human re-reading every document.

### 6.4 The type field, corrected

**The first draft was wrong about this field, and the correction closes blocking open question 1.**

The actual enum is nine values, not seven: `concept`, `reference`, `runbook`, `decision`, `guide`, `policy`, `skill`, `agent`, `strategy`.

The draft asserted that "there is no type for a rule or a skill". Skills and agents are types, and both are branched on. `bundle.yaml` declares `skills: type: [skill]`, which is what makes a skill a directory whose sibling files travel with it as assets exempt from the frontmatter rule. `internal/plan/skill.go` and `internal/plan/agent.go` render them into harness-specific files, and `bundle.Build` refuses to distribute an agent declaring `permission-mode: bypassPermissions`.

There is no `rule` type by design rather than by omission. **A rule is a selection, not a kind of document.** Rulesets name document ids, are compiled at build time against `requires:`, and hold no prose. `voice.base` is `type: policy` and becomes a rule purely by being named in the `voice` ruleset. Deleting every ruleset removes injection and removes no knowledge.

So the draft's recommended option, rules and skills as store documents inheriting eviction, provenance, trust decay, the grounding gate, and the merge gate, is already built and shipped. The refinement is that residency is expressed as selection rather than as type, which is a better answer than the one proposed: a document can be resident in one repository's ruleset and merely retrievable in another without being two documents.

The two orthogonal axes the draft identified are still real, and the ones that remain muddled are `guide`, `policy`, and `strategy`.

- **Lifecycle.** Is this a record of a moment (immutable history) or current truth (living, revisable)?
- **Consumer action.** Does the consumer load this as a rule, install it as a skill, or retrieve it as knowledge?

**Resolved 2026-07-24, and recorded as `records.frontmatter-reduced-to-what-is-read`.** The enum is five values: `concept`, `reference`, `record`, `skill`, `agent`.

`runbook`, `guide`, and `strategy` folded into `reference` and `concept`, none having any documents. `policy` folded into `concept`, which is the change worth explaining: it was the one value with documents behind it, and it still did not survive. What it was trying to say is "this document is meant to be pushed at you," and pushing is decided by ruleset membership, the only place that can act on it. A policy nobody named in a ruleset is a concept with a firmer tone. `voice.base` is now a concept and is still a rule.

`decision` became `record`, because every document in a store is downstream of a decision, so the word separated nothing. What the type marks is a write rule: this is the one kind of document where the repair for being wrong is to write a new one rather than edit this one. That is also the property the freshness audit keys on, since a document that must not be edited cannot go stale.

The required set is six: `id`, `title`, `description`, `type`, `status`, `provenance`. Deleted: `capability`, `resource`, `created`, `related`, `tags`. Demoted to optional: `domain`, `owner`. `freshness.json` follows the new enum and gained the windows it was missing for `skill` and `agent`.

### 6.5 Fields with new or changed jobs

- **Identifier.** Once facet records reference documents, this becomes a hard contract rather than a convenience. It already is one in three places: rulesets reference documents by id, the usage log records id rather than path, and the catalog is keyed on it. Pin it before moving anything else. It must survive file moves, which is the argument for keeping it distinct from the OKF path identity.
- **Description.** The highest-value field in the schema, because it is what an agent reads to decide whether to open a document. It is capped at 320 characters and is rendered into every skill and agent file and every index listing. It deserves a quality bar and possibly a lint check, since non-discriminating descriptions cost reads permanently.
- **Provenance.** Gains a second consumer as a retrieval filter. See section 7.
- **Visibility.** With operator bundles and agent bundles emitted from one store, this becomes the composition control, meaning which bundle a document lands in. It currently has two values and one of them is unused. Worth keeping, worth extending, and worth renaming to state what it now does.
- **Scope.** Undervalued in the first draft, which did not mention it. It is what makes a rule conditional, and section 4.7 cannot distinguish "never applicable" from "never loaded" without it.

### 6.6 Record the outcome

Write the audit result as a decision record, not only as a schema change. The fields cut and the reasoning will be relitigated, and the schema itself does not remember why.

---

## 7. Unattended agents

The knowledge store is intended to serve unattended agents, not only operator sessions, so they can ground automated tasks. That makes the store a production dependency rather than a convenience.

**Correctness bar rises.** A wrong document in an operator session costs a correction that is noticed immediately. The same document in an unattended run costs a merged PR or a bad triage, and nobody notices until later.

**Provenance becomes a retrieval filter, not only metadata.** Operator sessions may ground on asserted content because a human is present to catch problems. Unattended runs should be held to verified and decided, with asserted content invisible to them. Same store, different floor, one query parameter. The mechanism for this already exists in a different shape: `requires:` in `bundle.yaml` is exactly such a floor, applied at build time. An agent bundle with a stricter `requires:` is the cheapest possible implementation and needs no new code, only a second build.

**Coverage honesty matters more.** An unattended agent cannot ask a clarifying question, so returning "we do not have this" is the only safe failure mode. It is the difference between a stalled task and a confidently wrong one.

**Outcome proxy differs.** There is no human correction signal in an unattended run, so the outcome must be derived from whether the work landed or was reverted.

**The two consumer types are complementary and both should feed the same facet stream.**

- Unattended runs are the better gap detector, because no human is quietly filling in what the store failed to provide. A gap surfaces as the agent actually failing.
- Operator runs are the better correctness detector, because correction events only exist where a human is present.

**Correction to the first draft's practical conclusion.** It claimed that existing agents such as Yeoman and the Sentry support agent are already store consumers, so facets could be collected from them rather than waiting on personal session volume, and called that a meaningfully faster path to a usable corpus. Only Yeoman exists. There is no support agent, no PLG agent, and no distiller. Emitting facets from Yeoman is worth doing and is one agent's worth of volume, not a fleet's. Plan on `qm digest --backfill` for the corpus and treat unattended facets as the second source rather than the first.

---

## 8. Seams to preserve for the distributed future

Build none of this now. Do not foreclose any of it either.

1. **The facet record is the wire format.** Versioned, self-contained. Whatever ships later ships facets.
2. **Repository identity in every record from day one**, derived from the remote and never from a path. It cannot be backfilled, and the usage log is the proof: its early lines are now permanently ambiguous about which repository they meant.
3. **The facet writer sits behind a small interface** with a filesystem implementation. Adding an HTTP implementation later is a transport change rather than a rewrite.
4. **Extraction stays local by construction.** If extraction is always local, distribution later is only a decision about where to post files, not an architectural change. It also means raw transcripts never centralize, which is the correct default for content that contains pasted credentials and candid remarks about colleagues.
5. **Harness hook payloads normalize at the edge** into one internal event schema. `qm usage record` already does this in miniature, accepting both the nested and the flat spelling of a file path and yielding nothing for anything unrecognized. The hook surface is large and actively changing, so isolate it and degrade to silence when a payload shape drifts.

---

## 9. Decisions to record separately

The spec is living truth and will be rewritten as implementation drifts. These carry real rejected alternatives and belong in immutable decision records.

**Decision A. Bundle remains the distribution unit; contribution routes through a pointer in bundle metadata back to the source repository.**

Rejected alternatives. Distributing the repository itself, which loses build-time validation, the precomputed catalog, digest pinning, multi-source composition, and the deliberate asymmetry where read access is broader than write access. And standing up a proposal service, which git already does better and permanently.

**Decision B. Facet extraction runs locally and only facet records leave the machine, with raw transcripts retained locally under a window.**

Rejected alternative. Central transcript ingestion, which yields better and re-runnable analysis at the cost of centralizing credentials and candid content, and which will be relitigated the moment the distributed version is discussed.

**Decision C. Repository identity is derived from the remote, and the checkout is recorded as a separate dimension.**

Rejected alternative. The working-tree path, which is what the usage log recorded until 2026-07-24 and which silently multiplied one repository into one per worktree, inflating exactly the cross-repository spread that promotion is judged on.

---

## 10. Open questions for implementation

1. ~~Rules and skills as store documents with their own types, or as a separate source composed at bundle build.~~ **Resolved by verification.** Skills and agents are types; rules are ruleset membership. One pipeline, already built. See section 6.4.
2. ~~The reduced frontmatter schema, output of the section 6.3 audit.~~ **Resolved 2026-07-24.** Five types, six required fields, five fields deleted, two demoted. Migrated and recorded. See section 6.4. **No blocking questions remain.**
3. Whether unattended agents receive skills, and if so which, by role.
4. Precedence and contradiction handling between the local overlay and upstream bundles. Section 4.9.
5. The retention window for raw local transcripts.
6. Whether the digest model call uses a small local model or the same provider, and what the per-session cost is at expected volume. **Partly answered 2026-07-24:** the writer is built and neutral. `qm digest annotate` validates questions against the closed schema and attaches them, caring nothing about which model produced them or where it ran, and `qm digest list --pending` is the work queue. An agent drives it today, which keeps Quartermaster free of model calls and, more usefully, puts the extraction prompt somewhere it can be rewritten in seconds rather than rebuilt. Moving the call into `qm` later reuses the same schema and the same writer.

   **What is still undecided is the payload, not the caller.** Decision B says extraction runs locally and only facet records leave the machine, on the grounds that transcripts hold pasted credentials and candid remarks. Any hosted model, reached from `qm` or through an agent, sends transcript content off the machine. Only a local model keeps Decision B true as written. Decide that before this runs at volume, and rewrite Decision B rather than letting it quietly come to mean something else.
7. What defines "first useful action" precisely enough to compute discovery span consistently across session shapes.
8. ~~Whether harness hooks install per worktree or at user level.~~ **Resolved 2026-07-24.** Neither: `qm init` writes the session hook into the repository's committed `.claude/settings.json`, so git carries it to every worktree and every clone exactly as it carries the manifest, and the machine-wide alternative is not needed. The command is guarded with `command -v qm` so it is inert for anyone without the tool, matching the git hooks. `--no-telemetry` skips it, since the flag already means "do not record". What remains is smaller: `qm status` should report whether the current worktree is actually recording, because a worktree that silently records nothing is still the failure worth catching.
9. Whether `qm gaps` clustering runs locally or calls a model, and the cost implications of re-clustering a growing window each run.
10. **How a repository selects what it gets, which is currently three idioms for four deliveries.** Designed and deliberately deferred; see [Packages](packages.md). The only thing this feature needs from it: a facet record names the packages a session had, and today that field is called `rulesets`. Either name is a rename away, so it does not block section 4.5.

---

## 11. Implementation sequence

1. ~~Verify every `[VERIFY]` claim against the actual code and schema. Correct this document.~~ **Done 2026-07-24.** See section 0.
2. Resolve the remaining blocking open question, the schema reduction. Question 1 is closed.
3. Facet record schema, version 1. Freeze the identifier contract.
4. **`qm trace record` done 2026-07-24**, spooling to `~/.quartermaster/spool/pending.jsonl` with `QM_TRACE_DIR` to override. Identity comes from `repo.Identity`, the branch from the worktree's own git directory, and every installed bundle rather than the first. The SessionEnd hook is documented in the README for hand-wiring, exactly as the usage hook is; installing it automatically waits on open question 8.
5. **`qm digest` done 2026-07-24, structurally.** It derives a facet by counting, matching, and ordering what the transcript already records: the discovery span, which knowledge documents were opened, how each was arrived at, whether opening one ended the search, and what the session produced. No model is called, so a digest costs nothing and can be re-run over the whole corpus whenever the derivation improves. `questions[]` stays empty rather than guessed, and every record says in `source` how it was made.

   That is a deliberate narrowing of what section 4.4 describes, and the reason to record it: Quartermaster's stated commitment is that it makes no model calls and is reproducible from its inputs. A per-session model call breaks that, and open question 6 has not been decided. The seam is left clean, so the semantic pass adds `questions[]` and flips `source` without redoing any of this.

   `--backfill` sweeps the repository it runs in, resolved through `repo.Identity` so every worktree of it is included and the harness's path-keyed project directories never reach a record. `--all` widens it to the machine. Scoping the sweep costs no cross-repository signal, because records accumulate in one directory across runs.
6. Design `qm gaps` against that real corpus rather than against an imagined one.
7. Capture for two weeks with the bundle held constant to establish the discovery-span baseline. Confirm first that the configured target actually names the knowledge root, per section 2.4.
8. `qm gaps` with the content versus discoverability split and drafts output.
9. ~~Frontmatter audit and schema reduction, recorded as a decision.~~ Done 2026-07-24, out of order, because the audit was what closed question 1 and the window for a breaking schema change is closing.
10. Removal signals and `qm prune`. Rule signals join on `rulesets.json`, not on document type.
11. SessionStart freshness check.

---

## 12. References

- Gloaguen et al., *Evaluating AGENTS.md: Are Repository-Level Context Files Helpful for Coding Agents?*, arXiv:2602.11988
- *Agentic Context Engineering: Evolving Contexts for Self-Improving Language Models*, arXiv:2510.04618
- Anthropic, *Clio: Privacy-preserving insights into real-world AI use*, https://www.anthropic.com/research/clio
- Anthropic, *Writing effective tools for AI agents*, https://www.anthropic.com/engineering/writing-tools-for-agents
- Husain and Shankar, error analysis and failure taxonomy method, https://hamel.dev/blog/posts/evals-faq/
- Open Knowledge Format specification, https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
- Claude Code hooks reference, https://code.claude.com/docs/en/hooks