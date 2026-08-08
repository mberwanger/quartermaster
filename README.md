# Quartermaster

Quartermaster (`qm`) keeps the knowledge your coding agents read in sync across every
repository. It pulls a versioned knowledge bundle and writes it out in the shape each
agent tool expects — `.claude/rules/`, `.cursor/rules/`, an `AGENTS.md` block, and a
knowledge tree the agent can open on demand.

You maintain the knowledge in one place. Each repository declares which parts it wants.

Every transformation is deterministic and reproducible from its inputs alone. **No model
call is made at any point.** Quartermaster delivers knowledge that already exists; it does
not author, summarize, or reason.

## Getting started

```
cd my-repo
qm init --source file://../knowledge/store --package engineering
```

There is a runnable store in [`examples/`](examples/) — a walkthrough of one
repository taking two teams' packages, scoping the knowledge it keeps on disk,
and correctly refusing to promote a draft.

`init` resolves the source, detects which agent tools the repository uses, writes
`.quartermaster.yaml`, adds the generated paths to `.gitignore`, installs `post-checkout`
and `post-merge` hooks so the files stay current across branch switches, and runs the
first sync.

After that, `qm sync` is the only command you need day to day — and the hooks usually run
it for you. Sources and packages are listed in precedence order: when two bundles define
the same rule, the later one wins.

## Commands

Run `qm bundle …` in the repository that holds your knowledge. Run everything else in a
repository that consumes it.

| Command | Purpose |
| --- | --- |
| `qm bundle init` | Scaffold a new knowledge store |
| `qm bundle index` | Regenerate directory listings, or check them in CI |
| `qm bundle validate` | Run all compilation checks without emitting anything |
| `qm bundle build` | Compile a source tree into a bundle |
| `qm bundle explain <id>` | Show why a document did or did not become a rule |
| `qm bundle publish` | Push a built bundle to a registry |
| `qm init` | Set a repository up and run the first sync |
| `qm sync` | Resolve, write the files, remove what no longer applies |
| `qm verify` | Fail if generated files were edited or went stale |
| `qm status` | Show what resolves today, and how much is always loaded |
| `qm targets` | List the agent tools this build can write for |
| `qm stats` | Report which knowledge agents actually read |

## How it works

A **bundle** is a snapshot of a knowledge store — markdown with frontmatter — addressed by
a content digest, so a given digest always means the same bytes.

A **package** is a named selection — shaped by team or topic, like `engineering` or
`data-engineering` — carrying the rules, skills, and agents that belong together. It holds
no content of its own; choosing a package decides what your repository is given without
asking, and the documents are in the bundle either way.

Selections are patterns, so a team's skills are `skills.data.*` rather than a list, and
adding one reaches every package that globs it without editing any of them.

Packages are not tied to a particular agent tool — that is what **targets** are for. One
package renders into every target you configure.

Which documents are *allowed* to become rules is declared once, in the store's
`bundle.yaml`, and applied when the bundle is built:

```yaml
requires:
  status: [active]
  provenance: [verified, decided]
```

A draft can't be pulled in by naming it in a package — the build fails and tells you which
requirement it missed. `qm bundle explain <id>` answers the same question for any document.

How a rule loads depends on whether it declares a path scope:

- **no scope** — loaded at the start of every session
- **a scope** like `**/*.go` — loaded only when a matching file is open

Everything else in the bundle is written to `.quartermaster/knowledge/` and enters the
agent's context only if it goes looking. Because always-loaded rules cost context in every
session, `qm status` reports their total against the `budget.resident_bytes` you set.

## Taking only the knowledge you need

By default a repository gets the whole bundle on disk. Once a store serves a whole
organization that is more than any one repository wants, so a bundle entry can narrow it
by matching on any frontmatter field:

```yaml
bundles:
  - source: oci://ghcr.io/org/knowledge:v1
    use: [engineering, billing]
    knowledge:
      domain: [engineering, billing]      # scalar field
      tags: { not: [deprecated] }         # or exclude
```

Scalar fields (`domain`, `owner`) match directly; list fields (`tags`) match when any
entry does. Every field listed must pass.

This is a relevance filter, not a permission one — what may become a rule is still decided
by the store. If you select a package that needs a document your filter excludes,
`qm sync` says so rather than quietly resolving it one way or the other. `qm status`
reports which fields you filtered on, so a partial tree is never mistaken for the whole
store.

Generated files are gitignored and safe to delete — `qm sync` rebuilds them. The one
exception is the `AGENTS.md` block, which is committed; `qm verify` guards it, and
anything you write outside the markers is left alone.

## Finding out what gets read

Writing a document is cheap; knowing whether anyone reads it is not. `qm stats`
reports which documents agents opened, across every repository that records usage:

```
usage  all time · 7 event(s) · 3 document(s) · 2 repositories

most opened
     5  eng.experimental-cache     2 repos   last 2026-07-23

retrievable here, never opened
  billing.money-representation

promotion candidates  (on disk here, opened in most repositories)
  eng.experimental-cache          2/2 repos
```

The log is global rather than per repository, because promotion is a
cross-repository decision: a document opened once in each of twelve repositories
is a stronger candidate than one opened twelve times in a single one.

Recording is opt-in per repository (`telemetry` in the manifest) and is written
by hooks in your agent tool.

`qm init` installs the session hook into the repository's own
`.claude/settings.json`, which is committed. That is deliberate: it travels to
every worktree and every clone the way the manifest does, rather than being
configured once per machine and quietly missing wherever it was not. It is
guarded with `command -v qm`, so it is inert for anyone who does not have the
tool. Pass `--no-telemetry` to skip both the hook and the recording.

The read-level hook is not installed for you, because it runs on every file read
rather than once per session:

```json
{
  "hooks": {
    "PostToolUse": [
      { "matcher": "Read", "hooks": [{ "type": "command", "command": "qm usage record" }] }
    ]
  }
}
```

The `PostToolUse` hook records a document id, every installed bundle, the
repository, the worktree, and a timestamp. It never records prompt or file
content, and anything outside the knowledge tree is ignored.

The `SessionEnd` hook spools one line per session: the session id, where its
transcript is on this machine, the repository, the worktree and branch, and the
bundles installed at the time. It reads no transcript and copies no content. The
bundle set is the reason it exists, since it is the one thing a transcript cannot
reconstruct, and without it a later pass can count what happened but can never
attribute a change to the document that caused it.

Both hooks exit zero whatever happens. A repository that has opted out, a payload
they do not recognise, and an unwritable log are all silent no-ops. Breaking a
session to record a statistic would be a poor trade.

The repository is named by its remote rather than by its path, so every worktree
of one repository counts as one. That is what makes the cross-repository spread
mean anything: a document opened in three checkouts of the same repository is one
repository's worth of evidence, not three.

**What this can and cannot tell you.** It reliably identifies documents nobody
ever opens — which usually means the `description` is wrong, since that is all an
agent sees when it decides whether to open something. It cannot tell a document
that helped from one an agent opened and discarded. And it never proposes a
demotion: an always-loaded rule is delivered by the harness rather than opened,
so its absence from the log means nothing at all. Treat the output as a shortlist
of things to look at, not a verdict.

## Sources

A bundle is resolved by its source scheme. Every scheme yields the same thing — a bundle
plus a content digest — so nothing downstream depends on where it came from.

| Scheme | Resolution |
| --- | --- |
| `oci://` | Pull the artifact from a registry, verifying the digest |
| `file://` | Read the directory in place; a store tree is built on the fly |
| `git+https://` | Fetch a ref; `//subdir` and `#ref` are optional |
| `https://` | Fetch a tarball |

Remote sources are cached by digest under `~/.quartermaster/cache` (override with
`QM_CACHE_DIR`), so many repositories on the same bundle pull it once. `file://` is never
cached: a local tree is what you edit, and every save has to be visible immediately.

### Credentials

Quartermaster keeps no credentials of its own. Each remote scheme uses the same
credentials the tool it wraps already uses, so a registry you can `docker pull`
from and a repository you can `git clone` both work with no extra setup.

| Scheme | Authenticates with |
| --- | --- |
| `oci://` | The Docker credential store — `docker login ghcr.io` locally, or `docker/login-action` in CI. Anonymous when none is configured, which is right for a public registry. |
| `git+https://` | Git's own credentials — a credential helper or a cached token. It never prompts, so a private repository with no cached credentials fails cleanly rather than hanging a git hook. |
| `https://` | Nothing. Built for a public, versioned release asset. |

A private source in a headless environment therefore needs those credentials
present: a registry login in the agent's image, or git credentials in its
environment. The library takes them explicitly instead, for a deployed agent
that carries its own token rather than an ambient login:

```go
bundle, _ := qm.Open("oci://ghcr.io/org/knowledge:v1", qm.WithToken(os.Getenv("GITHUB_TOKEN")))
```

`WithToken` is a bearer token for any remote scheme; `WithBasicAuth` is a
username and password, as a registry login stores. A token is passed to git
through the environment, not the command line, so it does not appear in the
process list.

## As a library

An agent can consume a bundle directly instead of reading files a sync wrote.
The `qm` package is the same implementation the CLI uses, so a document rendered
into an agent's instructions is the same text it would become as a rule file.

```go
bundle, _ := qm.Open("oci://ghcr.io/org/knowledge:v1")

instruction, _  := bundle.Rules("engineering", "billing")      // the agent's system prompt
catalog := bundle.Catalog()                                     // tool: list by id + description
doc, _  := bundle.Document("eng.error-handling")                // tool: fetch one
```

It imports no agent framework: an agent wires these into its own instructions and
tools. Pin `bundle.Digest()` in the agent's configuration so knowledge and code
version independently.

## Development

```
make build    # build a snapshot binary for the current platform
make test     # run unit tests
make lint     # run golangci-lint
```

## License

Apache-2.0. See [LICENSE](LICENSE).