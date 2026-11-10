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
`.qm.yaml`, adds the generated paths to `.gitignore`, installs the hooks that keep the
files current, and runs the first sync.

After that, `qm sync` is the only command you need day to day — and the hooks run it for
you: `post-checkout` and `post-merge` after git moves the tree, and a Claude Code
`SessionStart` hook when a session opens, so an agent reads what the store publishes
today without anyone syncing by hand. Sources and packages are listed in precedence
order: when two bundles define the same rule, the later one wins.

### Staying current, or pinning

By default the manifest names a tag and follows it; every sync records the digest it
resolved to in `.qm/state.json`, and `qm status` shows it. Two ways to hold still:

- `qm init --pin`, or a `digest:` line on a bundle entry, records the bundle's own
  content digest. A sync still follows the tag but warns when the result differs, so
  a repository that wants to move deliberately sees the move. Bump it like a dependency.
- A registry digest in the source, `oci://host/repo@sha256:…`, is the hard pin. It
  names one artifact, and nothing can move it. Use it where reproducibility is the
  point, such as an evaluation that compares two bundles.

A repository set up by an earlier release, with `.quartermaster.yaml` and a
`.quartermaster/` tree, is migrated by its next `qm sync`: the manifest is renamed,
the tree regenerated under `.qm/`, and the `.gitignore` entry retargeted. Commit the
rename and the settings file it reports.

## Commands

Run `qm bundle …` in the repository that holds your knowledge. Run everything else in a
repository that consumes it.

| Command | Purpose |
| --- | --- |
| `qm bundle init` | Scaffold a new store |
| `qm bundle index` | Regenerate directory listings, or check them in CI |
| `qm bundle validate` | Run all compilation checks without emitting anything |
| `qm bundle build` | Compile a source tree into a bundle |
| `qm bundle explain <id>` | Show why a document did or did not become a rule |
| `qm bundle publish` | Push a built bundle to a registry |
| `qm bundle pull <source>` | Resolve any source and write the artifact out, with no consuming repository |
| `qm init` | Set a repository up and run the first sync |
| `qm sync` | Resolve, write the files, remove what no longer applies |
| `qm verify` | Fail if generated files were edited or went stale |
| `qm status` | Show what resolves today, and how much is always loaded |
| `qm find <term>` | Search the delivered knowledge by id, title, and description |
| `qm targets` | List the agent tools this build can write for |

## How it works

A **bundle** is a snapshot of a store — markdown with frontmatter — addressed by
a content digest, so a given digest always means the same bytes.

A **package** is a named selection — shaped by team or topic, like `engineering` or
`data-engineering` — carrying the rules and capabilities that belong together. It holds
no content of its own; choosing a package decides what your repository is given without
asking, and the documents are in the bundle either way.

A **capability** is a skill or an agent. A skill loads instructions on demand, when the
work matches what its description says it is for; an agent delegates work with its own
tools, model, and permissions. Both are reached for by the agent when they apply, which is
what separates them from a rule — a rule is pushed, whether it turns out to be needed or
not.

Selections are patterns, so a team's skills are `skills.data.*` rather than a list, and
adding one reaches every package that globs it without editing any of them.

Packages are not tied to a particular agent tool — that is what **targets** are for. One
package renders into every target you configure.

Which documents are *allowed* to become rules is declared once, in the store's
`bundle.yaml`, and applied when the bundle is built:

```yaml
requires:
  status: [stable]
```

A draft can't be pulled in by naming it in a package — the build fails and tells you which
requirement it missed. `qm bundle explain <id>` answers the same question for any document.

How a rule loads depends on whether it declares a path scope:

- **no scope** — loaded at the start of every session
- **a scope** like `**/*.go` — loaded only when a matching file is open

Everything else in the bundle is written to `.qm/knowledge/` and enters the
agent's context only if it goes looking. Because always-loaded rules cost context in every
session, `qm status` reports their total against the `budget.resident_bytes` you set.

A skill is loaded only when the agent asks for it, and an agent editing a test file does
not reliably ask for the testing skill. A skill that declares a `scope` therefore gets a
few-line path-scoped rule beside it that names the skill when a matching file is open.
The rule carries none of the skill's body and costs nothing against the budget.

### How an agent finds it

Every sync writes one resident rule of its own, `qm.knowledge`, generated from what it
delivered: where the tree is, how many documents each directory holds, which sources and
packages they came from, and how to search. It tells the agent to read the delivered copy
rather than another checkout of the store, and that the copy is current because the hooks
keep it so.

`qm find <term>` is the search it names. Every term must match; a hit in the id or title
outranks one in the description, which outranks one in the body; a superseded document is
left out. A grep finds every mention, this finds the document.

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

## Sources

A bundle is resolved by its source scheme. Every scheme yields the same thing — a bundle
plus a content digest — so nothing downstream depends on where it came from.

| Scheme | Resolution |
| --- | --- |
| `oci://` | Pull the artifact from a registry, verifying the digest |
| `file://` | Read the directory in place; a store tree is built on the fly |
| `git+https://` | Fetch a ref; `//subdir` and `#ref` are optional |
| `https://` | Fetch a tarball |

Remote sources are cached by digest under `~/.qm/cache` (override with
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