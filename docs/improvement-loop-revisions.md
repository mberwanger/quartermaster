# Improvement Loop: Revisions From First Use

## 0. How to read this document

This is a companion to `continuous-improvement-loop.md`, not a replacement. That
spec was written before the loop had ever been run. This one records what
changed on first contact with a real corpus, and it amends sections 4.4, 4.5,
4.6 and 4.8 of the original.

Nothing here is built. The command surface in section 2 is a rename of shipped
code; sections 5 and 6 are new behavior. Claims about current state cite the
file that proves them. Claims about intent are judgment and carry no citation.

The occasional `ruleset` below inherits the parent spec's vocabulary; see its
"Superseded since this verification" note for the current `package` naming.

**What first use was.** Fifteen facet records across two repositories, three of
them annotated by hand through the `digest-questions` skill, carrying eleven
questions between them. That is not a corpus and no number below is a
measurement. It was enough to find four things wrong, which is what a first run
is for.

---

## 1. The four findings

1. `digest` is the wrong name, and it collides with the tool's central noun.
2. Exactly one step in the pipeline wants a human, and it is not the one that
   currently has a human in it.
3. A candidate the operator declines has nowhere to be recorded, so it returns
   at the top of the ranking every run.

The third is the consequential one. It is section 6.

Section 5 records a hypothesis about ranking that first use raised and did not
support. It is kept because the reasoning that killed it is worth not repeating.

---

## 2. Command surface

`digest` means sha256 everywhere else in this tool. The first line of `qm --help`
is "pulls a versioned knowledge bundle by digest," and bundle identity is the
product. A second, unrelated meaning for the same word in the same CLI is not
survivable, and the word carries no hint of transcripts or sessions besides.

The group is about sessions. Singular, because the existing convention in this
CLI is that **groups are singular and leaf list-commands are plural**:
`qm bundle` holds six subcommands, `qm targets` and `qm stats` and `qm gaps`
print lists.

| Now | Becomes |
|---|---|
| `qm digest` | `qm session analyze` |
| `qm digest list` | `qm session list` |
| `qm digest annotate` | `qm session annotate`, hidden |

**`annotate` becomes hidden.** It reads JSON on stdin and attaches questions to a
record; no operator will ever type it. `qm` already has this category and states
it plainly — `qm trace record` and `qm usage record` are both `Hidden: true`
with the comment "a machine entry point for harness hooks, not a human one"
(`cmd/trace.go`, `cmd/usage.go`). `annotate` is the same species, differing only
in that a model calls it rather than a hook. Hiding it also disposes of the
complaint that the name does not communicate: it does not need to.

`qm gaps` stays where it is and stays plural. It is a leaf that prints a ranked
list, and it is about the store rather than about sessions.

---

## 3. Which steps take a human

| Step | Human? | Where it lives |
|---|---|---|
| trace | no | harness hook |
| analyze | no | `qm`, deterministic |
| annotate | no | model, batch |
| gaps | no | `qm`, deterministic |
| **promote** | **yes** | **skill** |

The `digest-questions` skill is written as a conversation and is not one. A run
of it issues four queries, reads the output, writes JSON, and pipes it to
`annotate`. It asks the operator nothing, and the skill's own instructions
forbid the interactive moves — do not resume the session, do not annotate the
session you are in. It is a pure function from transcript to questions that
happens to be executing inside a chat window, which costs an interactive context
window per session and only works while somebody is sitting in Claude Code.

Promotion is the opposite and should be the conversation, for a reason worth
stating: **it is the only step where a mistake is durable.** A bad facet record
is re-derived for free, and the original spec makes that cheapness a design
property. A bad question is re-annotated; re-annotating replaces rather than
appends. A bad document is synced into every repository that installs the bundle
and read by every agent until somebody notices. The asymmetry is what earns the
human, and it argues the human belongs at the end rather than in the middle.

---

## 4. Where the model call belongs

Annotation is a batch transform, so the obvious move is to put it in the binary:
`qm session analyze --questions`, one pass, `source` still recording which
fields came from the model and which from counting.

**The obstacle is not the model call, it is the transcript.** The entire capture
design exists to keep session content on the machine. `trace` records a pointer
and says why in its package comment: raw transcripts hold pasted credentials and
candid remarks, and they stay put. Having `qm` POST them to a vendor inverts the
one commitment the capture side actually makes. A local model does not have this
problem and is the only version of this that is obviously safe.

The secondary objection is weaker than it looks. `qm --help` claims "No model
call is made at any point," but read in context that promise is about the pull
and materialize path, which must be reproducible because it writes files into a
repository. The analysis path is already non-deterministic the moment a model
writes the questions — the model is merely outside the binary today. The claim
needs narrowing to the sync path regardless of what is decided here, because as
written it is already misleading about the loop.

**Either way, one behavior from the conversational version must survive.** During
first use the annotator dropped a question — whether a Prometheus collector can
scrape a loopback-bound port — because none of the seven resolution values
describes "the model already knew." The operator learned this only because a
human was in the loop to be told. A batch annotator makes that call silently on
every run, so it must emit what it discarded and why. Otherwise the closed
resolution set never comes under pressure and simply eats the cases it cannot
express.

---

## 5. A rejected addition, and why it was rejected

First use produced a candidate for a new resolution value, `prior_art`, meaning
the answer came from reading another repository the organization owns. It was
proposed, drafted, and cut. The reasoning is recorded because the same proposal
will arrive again.

**What suggested it.** Session `f9e776be` produced two questions of the same
kind — how this organization does deployment plumbing:

| Question | Resolution | Ranks |
|---|---|---|
| what layout do the existing service Helm charts in this organization use | `source_read` | low |
| why does the aquasecurity/trivy-action step fail to install the trivy binary | `external_docs` | high |

The first was answered by reading a sibling repository. The second went to
GitHub issues and ranks near the top of the content gaps. The operator's
judgment is that neither is worth a document: the organization's method for this
class of work is prior art, and a question settled in the prior repository is
not re-litigated in the next one.

**Why the proposal failed.** Three reasons, in increasing order of force.

It was drawn from one remark in conversation and one session. The resolution set
is closed on purpose — `annotate` rejects anything outside the table, because an
open set cannot be clustered — so every value added dilutes the mechanism it
exists to serve. The bar for adding one is higher than a single observation.

It contradicts the method the original spec adopts. Section 4.6 cites Clio for
bottom-up discovery, on the grounds that top-down evaluation only finds what was
already suspected. Inventing a category from a conversation is top-down
invention wearing the loop's clothes.

**And it would not have caught the case that motivated it.** The proposed
detector was the mixed cluster: some occurrences resolving `prior_art` and
others resolving `external_docs`, which would say the answer existed inside the
organization and the agent failed to find it. The trivy cluster has no
`prior_art` occurrence to mix with — both of its occurrences went outside. The
helm question, meanwhile, resolved `source_read` and already ranks low, which is
the correct outcome under the model as it stands. Neither case is a demonstrated
failure of the current ranking, and the proposed fix addresses neither.

What actually identified the trivy cluster as not worth writing was the operator
saying so. That is not a gap in the enum. It is the decline path, and it is
section 6.

**What survives is a hypothesis, not a finding.** Non-recoverability is
evaluated against the repository the session ran in, and it is possible the
right boundary is everything the organization already owns. Nothing in the
corpus yet shows the narrower boundary producing a wrong answer. If the
boundary is wrong, the decline log will show it: a reason that recurs across
declines is evidence, and a category discovered that way has survived contact
with more than one case. Categories should be read out of the decline log, never
written into the enum ahead of it.

---

## 6. Declined candidates must stay declined

Section 4.6 deduplicates against open proposals. It has no notion of a candidate
that was considered and rejected, which means a cluster the operator has already
judged not worth writing returns at the top of the ranking every run, because
its evidence is unchanged and its resolution values still read as
non-recoverable.

Promotion therefore needs three outcomes, not two: write it, defer it, decline
it. A decline records the cluster key, the reason, and the date, and `qm gaps`
filters against that record the way it filters against open proposals.

**The reason matters more than the flag, and it is the only part of this design
that scales.** A decline is where operator judgment enters the system as free
text rather than as a category somebody guessed in advance. "Prior art in
`<repo>`" is one such reason; there will be others, and the point is that none
of them has to be predicted. A reason that recurs across many declines has
earned a category. A reason that appears once was an offhand remark, and the
decline log holds it at no cost and without distorting the enum. This is the
mechanism section 5 should have proposed instead of a new resolution value.

---

## 7. What this does not settle

- Whether the resolution set needs any addition at all. Section 5 rejected the
  one candidate first use produced. The honest position is that eleven questions
  is too few to know, and that the decline log is how the question gets
  answered rather than argued.
- Whether promotion is one skill or several. Researching an unanswered cluster,
  deciding placement, and writing to schema are different jobs.
- Where a decline log lives. It is operator judgment about a shared store, which
  is the first artifact in this system that is neither user-scoped telemetry nor
  bundle content.

---

## 8. Current state, verified 2026-07-24

- `qm gaps` builds, is registered (`cmd/root.go:84`), and runs. Against the
  15-record corpus it reports every question as `seen 1×`: nothing recurs yet,
  so nothing clusters. That is the expected output at this volume and is the
  concrete form of section 7's caution.
- The `PostToolUse` read hook is deliberately not installed by `qm init`
  (`README.md:146`), so `qm stats` has no data unless an operator adds it. Every
  facet record in the corpus reports zero store reads, which is a consequence of
  this rather than a finding about the store.
- Session capture works. `SessionEnd` runs `qm trace record`, and the spool is
  user-scoped so one machine's corpus spans repositories
  (`internal/repo/harness.go`, `internal/trace/trace.go`).

---

## 9. Spawning the annotator

Annotation today is three steps across two tools, and only the first two are
discoverable. The operator runs `qm digest --backfill`, reads a session
identifier out of `qm digest list --pending`, opens a *new* harness session, and
types `/digest-questions <id>`. Nothing in `qm`'s output says that third step
happens somewhere else, so the operator has to already know. Section 3 argued
annotation is a batch transform wearing a conversation's clothes; this is what
that costs in practice.

**`qm session analyze --annotate` should spawn the harness itself**, headless,
once per pending session — `claude -p "/digest-questions <id>"` or the
equivalent for whatever harness produced the transcript. The spool already
records `harness` per session precisely because transcript shape depends on it
(`internal/trace/trace.go`), so the same field can dispatch the spawn.

This is deliberately not the in-binary model call section 4 examined, and it
avoids that design's problems rather than solving them:

| Concern | In-binary API call | Spawned harness |
|---|---|---|
| Annotation logic | Second copy, drifts from the skill | Skill remains the only definition |
| Credentials | `qm` needs keys and provider config | Harness already has them |
| Transcript exposure | New egress path | Goes nowhere it was not already going |
| Batch | Yes | Yes |

**The hazard is recursion, and it is live.** A spawned annotation session ends,
`SessionEnd` fires, `qm trace record` spools it, and it becomes a pending record
that wants annotating. `internal/trace/trace.go` has no exclusion of any kind.
Left alone this generates work for itself without bound, and it contaminates the
corpus with sessions whose questions are about annotating rather than about the
repository's actual work — which is the same corpus `qm gaps` ranks.

This is not hypothetical. The session that produced this document is a
`digest-questions` run, and it will be spooled on exit like any other.

Fix it at the capture edge rather than downstream: `qm` sets a marker in the
spawned process's environment, and `trace record` returns silently when it sees
one. Filtering later would mean every consumer reimplements the same exclusion
while the records accumulate anyway. The skill's existing "do not annotate the
session you are in" rule does not help here — that governs the reader, not the
spool.

Spawning agents from a CLI that has never done so is a real change in what the
tool is, so it should be opt-in, and it should report how many sessions it is
about to annotate before it does.

---

## 10. Drafts, and the two exits

`qm gaps --drafts <dir>` already writes candidate documents and already stamps
them `status: draft` with `provenance: asserted`, which under the store's
`requires:` keeps them out of every ruleset until a person promotes one. That
gate is what makes staging a draft anywhere safe, and it is the part that is
finished.

What is missing is the convention around it.

**A default location.** `.quartermaster/drafts/`, beside the materialized
`.quartermaster/knowledge/`. Gitignored, through the block `qm init` already
manages: drafts in the `git status` of a working repository are noise, because
that repository is not the knowledge store and is not where they will be
committed.

**The two exits, neither of which is built.** Both are already designed in the
original spec, and the operator's choice between them is a real decision rather
than a formality:

| Destination | Meaning | Spec |
|---|---|---|
| The shared knowledge store | Everyone consuming the bundle gets it | 4.8, `qm propose` |
| The operator's own repository | Local overlay, no distribution | 4.9 |

The staging folder is the missing middle between them. Without it, the end of
the promotion conversation is a set of conclusions that live in a terminal
scrollback, and closing the window loses them.

**Placement must ride in the draft's frontmatter.** The promotion conversation
produces three judgments, not one: which bundle carries it, which ruleset makes
it relevant, and whether it is resident or fetched on demand. Residency is the
one with a price, since `qm status` tracks a resident budget and every
auto-loaded document spends from it. If those three do not travel with the file,
the operator re-derives them at the moment of moving it, which is exactly where
the reasoning that produced them is no longer at hand.

---

## 11. Quartermaster distributes to five harnesses and learns from one

This is the most consequential thing first use surfaced, and it is invisible
from inside a Claude Code session.

**Outbound, the tool is already harness-agnostic.** `internal/target/` holds a
renderer per harness — `claude.go`, `cursor.go`, `codex.go`, `copilot.go`,
`agentsmd.go` — and the package comment states the design: one canonical
document is translated per target, so "adding a harness is one renderer here,
not an edit in every consuming repository."

**Inbound, there is exactly one of everything.** `internal/repo/harness.go`
hardcodes `.claude/settings.json` and the `SessionEnd` event. `internal/
transcript/transcript.go:117` sets `Harness: "claude-code"` as a literal rather
than a dispatch. Every facet record in the corpus, and therefore everything
`qm gaps` will ever rank, comes from operators who happen to use Claude Code.

The consequence is a bound on the loop rather than a missing feature: the
improvement loop can only learn at the rate of one harness's users, no matter
how widely the bundles are installed.

### 11.1 Claude Code is the reference implementation, deliberately

This is not a gap to close now. Building the loop against one harness and
getting its shape right is cheaper than generalizing over three half-understood
transcript formats, and the resulting design is the template. The roadmap
position is: agnostic eventually, Claude Code first, and the other harnesses
enter through the same seam the renderers already use.

**The harness population this is aimed at**, in rough order of how much it
matters, is Cursor and Claude Code first, then OpenCode, then Codex. Copilot is
present in the renderers but is the least used in practice and should not drive
design. The existing target list reflects an earlier guess at that population
rather than the current one.

**OpenCode has no target and no reader.** It does not appear anywhere in the
repository — not in `internal/target/`, not in `qm targets`, not in a comment.
That is the sharpest form of the asymmetry in this section: the harness with
real usage and no representation at all, on either axis. Whether the existing
`agents-md` target already serves it in part, through the `AGENTS.md`
convention, is unverified and is the first thing to check, because if so the
outbound gap is much smaller than it looks and only capture is genuinely
missing.

### 11.2 Capability differences are already handled, and the pattern transfers

The renderers do not lowest-common-denominator. Each emits only what its harness
understands, and says so in a comment:

| Target | Renders | Does not |
|---|---|---|
| `cursor` | rules, skills | agents |
| `claude` | rules, skills, agents | — |
| *OpenCode* | *nothing; no target exists* | *everything* |
| `codex` | skills | rules, which it has no notion of |
| `agents-md` | a managed block in `AGENTS.md` | — |
| `copilot` | rules | skills and agents, which it has no concept of |

A harness lacking a concept is handled by omission, not by degrading the
concept for everyone. Cursor gets rules and skills and no agents; Codex gets
skills and no rules; neither arrangement costs the other anything.

**The same rule should govern capture.** A harness with no session-end hook
simply does not contribute sessions; it still receives knowledge. Distribution
and capture are separate axes and a harness may sit on one without the other,
which is the state every harness except Claude Code is in today.

Where a difference is not merely absence but a genuine behavioral divergence, it
gets recorded rather than smoothed over. `internal/target/crossharness_test.go`
is where that already happens for rendering.

### 11.3 Open question: how the annotator learns which harness to spawn

Section 9 proposes that `qm session analyze --annotate` spawn a one-shot harness
session. That requires knowing which harness to spawn, and it is worth being
precise that this is **not the same question** as which harness wrote the
transcript.

Which harness *wrote* a transcript is per-session and already recorded, in
`trace.Session.Harness`. Which harness can *perform* an annotation is a property
of the operator's machine: reading a Cursor transcript does not require Cursor,
only something that can read JSONL and follow the skill.

That distinction settles where the setting lives. Materialization targets are
detected from the repository and recorded in the committed manifest, correctly,
because what a repository should contain is a team decision. The annotator is
the opposite: two people sharing one repository may run different harnesses, and
neither should be writing that into a committed file. **It belongs in
user-scoped configuration, not the manifest.**

Candidates, unresolved: ask at `qm init` and store it per user; a user-level
setting with no prompt; or probe `PATH` and use what is found. A probe as the
default with an explicit override is the least ceremony, and failing loudly when
nothing is found is better than picking one silently.

A further possibility worth noting but not pursuing yet: the annotation skill is
itself a document, and rendering one document into per-harness shapes is what
this tool does. A portable annotator may be mostly an application of machinery
that already exists.