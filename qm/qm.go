// Package qm is Quartermaster's library surface: the bundle semantics, exposed
// so an agent can consume knowledge directly rather than by reading files a sync
// wrote.
//
// It is the same implementation the CLI uses. A rule delivered into a coding
// session as a file and the same document rendered here into an instruction
// string are the same text, produced once, rather than two renderings that
// drift.
//
// The surface matches how an agent uses knowledge. Rules renders the documents a
// set of rulesets selects into one string, for the agent's own instruction (its
// system prompt). Catalog and Document are the two halves of progressive
// disclosure: an agent lists what exists by id and description, then fetches the
// one it wants. Scope does not appear, because a library agent has no file-open
// event to hang a glob on; a document is either rendered into the instruction or
// fetched on demand.
//
// This package depends on no agent framework. An agent assigns Rules to its own
// instruction and wires Catalog and Document into its own tools.
package qm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/pack"
	"github.com/mberwanger/quartermaster/internal/provider"
	"github.com/mberwanger/quartermaster/internal/target"
)

// Bundle is a resolved knowledge bundle.
type Bundle struct {
	b        *bundle.Bundle
	bodyByID map[string][]byte
	pathByID map[string]string
}

// Option configures how a bundle is resolved: the credentials for a remote
// source. Without any, each scheme falls back to the ambient credentials the CLI
// uses — the Docker store for oci, git's own credentials for git, none for
// https — which is right for a developer or CI, but a deployed agent that must
// present its own token uses these.
type Option func(*provider.Auth)

// WithToken authenticates with a bearer token: an OCI access token, an https
// Authorization bearer, or a git bearer header.
func WithToken(token string) Option {
	return func(a *provider.Auth) { a.Token = token }
}

// WithBasicAuth authenticates with a username and password, as `docker login`
// stores for a registry, or as an https or git endpoint expects.
func WithBasicAuth(username, password string) Option {
	return func(a *provider.Auth) { a.Username, a.Password = username, password }
}

// Open resolves a bundle from a source: oci://, file://, git+https://, or
// https://. A relative file:// path resolves against the working directory.
func Open(source string, opts ...Option) (*Bundle, error) {
	return OpenAt(source, ".", opts...)
}

// OpenAt is Open with an explicit base directory for relative file:// sources,
// which a service resolving a path from its own configuration needs.
func OpenAt(source, baseDir string, opts ...Option) (*Bundle, error) {
	var auth provider.Auth
	for _, opt := range opts {
		opt(&auth)
	}

	b, err := provider.Resolve(source, baseDir, auth)
	if err != nil {
		return nil, err
	}

	bodyByPath := make(map[string][]byte, len(b.Files))
	for _, f := range b.Files {
		bodyByPath[f.Path] = f.Body
	}
	bodyByID := make(map[string][]byte, len(b.Catalog))
	pathByID := make(map[string]string, len(b.Catalog))
	for _, e := range b.Catalog {
		if body, ok := bodyByPath[e.Path]; ok {
			bodyByID[e.ID] = body
			pathByID[e.ID] = e.Path
		}
	}

	return &Bundle{b: b, bodyByID: bodyByID, pathByID: pathByID}, nil
}

// Digest is the bundle's content digest. Pin it in an agent's configuration so
// knowledge and code version independently.
func (b *Bundle) Digest() string { return b.b.Meta.Digest }

// Packages lists the package names the bundle offers, sorted.
func (b *Bundle) Packages() []string {
	names := make([]string, 0, len(b.b.Packages))
	for _, rs := range b.b.Packages {
		names = append(names, rs.Name)
	}
	sort.Strings(names)
	return names
}

// Entry is one document in the catalog: enough for an agent to decide whether to
// fetch it, and nothing more.
type Entry struct {
	ID          string
	Title       string
	Description string
}

// Catalog lists every document, for an agent orienting on what exists before
// fetching anything. This is the first of the two progressive-disclosure tools.
func (b *Bundle) Catalog() []Entry {
	out := make([]Entry, 0, len(b.b.Catalog))
	for _, e := range b.b.Catalog {
		out = append(out, Entry{
			ID:          e.ID,
			Title:       str(e.Frontmatter["title"]),
			Description: str(e.Frontmatter["description"]),
		})
	}
	return out
}

// Document returns a document's prose by id. This is the second
// progressive-disclosure tool: an agent fetches the one document the catalog led
// it to. Frontmatter is stripped, because it is metadata for the store rather
// than content for the agent.
func (b *Bundle) Document(id string) ([]byte, error) {
	body, ok := b.bodyByID[id]
	if !ok {
		return nil, fmt.Errorf("no document %q in bundle", id)
	}
	return doc.Prose(body), nil
}

// Rules renders the documents the named rulesets select into a single string, to
// assign to an agent's instruction (its system prompt). A document named by more
// than one ruleset appears once, and the order follows the rulesets as given, so
// precedence is the caller's to state.
//
// Scope is ignored: a library agent has no file-open event, so every selected
// document is rendered rather than waiting on a glob. The text of each is exactly
// what the same document becomes as a rule file.
func (b *Bundle) Rules(rulesets ...string) (string, error) {
	byName := make(map[string]bool, len(rulesets))

	var sections []string
	seen := make(map[string]bool)

	for _, name := range rulesets {
		if byName[name] {
			continue
		}
		byName[name] = true

		rs, ok := b.pkg(name)
		if !ok {
			return "", fmt.Errorf("bundle has no package %q", name)
		}
		// Rules only. A skill is loaded when the work matches and an agent is
		// spawned, so neither belongs in an instruction string that is paid for
		// in every request.
		for _, cd := range rs.Rules {
			if seen[cd.ID] {
				continue
			}
			seen[cd.ID] = true

			body, ok := b.bodyByID[cd.ID]
			if !ok {
				return "", fmt.Errorf("package %q references %s, absent from the bundle", name, cd.ID)
			}
			sections = append(sections, string(target.RuleBody(doc.Prose(body))))
		}
	}

	return strings.Join(sections, "\n\n"), nil
}

func (b *Bundle) pkg(name string) (pack.Compiled, bool) {
	for _, rs := range b.b.Packages {
		if rs.Name == name {
			return rs, true
		}
	}
	return pack.Compiled{}, false
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
