// Package ruleset reads the named selections a bundle compiles, and resolves
// them against the store.
//
// A ruleset is a named selection of documents, with optional scope. It contains
// no prose. Deleting every ruleset removes injection and removes no knowledge,
// because the documents themselves stay in the retrievable tree.
package ruleset

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/gate"
)

// File is the parsed rulesets file: a mapping of ruleset name to its selection.
type File map[string]Ruleset

// Ruleset is one named selection.
type Ruleset struct {
	Docs []DocRef `yaml:"docs"`
}

// DocRef references one document by id, optionally overriding its scope for this
// ruleset. Scope is a property of the consumer, not of the knowledge, so the
// same document is unscoped in one ruleset and glob-scoped in another.
//
// Scope is a pointer so that an absent key and an empty list mean different
// things. Absent defers to whatever the document declares; an explicit empty
// list overrides that default back to unscoped, which is how a ruleset makes a
// document that usually applies to Go files load in every session instead.
type DocRef struct {
	ID    string    `yaml:"id"`
	Scope *[]string `yaml:"scope"`
}

// Load reads and parses a rulesets file. A missing path is not an error: a
// bundle may declare no rulesets, in which case nothing is injected.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from bundle.yaml, chosen by whoever runs the tool
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return nil, err
	}

	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Names returns the ruleset names in sorted order, so compilation is
// deterministic.
func (f File) Names() []string {
	names := make([]string, 0, len(f))
	for name := range f {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Compiled is a ruleset resolved to a flat form: every referenced document, in
// order, with its scope resolved and its store path attached. This is what a
// bundle carries, so a consumer renders without evaluating any predicate.
type Compiled struct {
	Name string        `json:"name"`
	Docs []CompiledDoc `json:"docs"`
}

// CompiledDoc is one resolved document in a compiled ruleset. Scope resolution
// order is ruleset override, then document default, then none. None means
// resident: the rule loads at the start of every session.
type CompiledDoc struct {
	ID    string   `json:"id"`
	Path  string   `json:"path"`
	Scope []string `json:"scope,omitempty"`
}

// Compile resolves every ruleset against the loaded documents and the gate.
//
// A reference to an unknown id is an error: the author named a document that
// does not exist. A reference to a document that does not pass the gate is an
// error: the gate decides what may become a rule, and a draft cannot be
// smuggled into a ruleset by naming it directly. Both are the author's to fix,
// so both fail the build rather than warn.
func Compile(f File, docs []doc.Doc, g gate.Gate) ([]Compiled, error) {
	byID := make(map[string]doc.Doc, len(docs))
	for _, d := range docs {
		if id := d.ID(); id != "" {
			byID[id] = d
		}
	}

	out := make([]Compiled, 0, len(f))
	for _, name := range f.Names() {
		rs := f[name]
		c := Compiled{Name: name, Docs: make([]CompiledDoc, 0, len(rs.Docs))}

		for _, ref := range rs.Docs {
			d, ok := byID[ref.ID]
			if !ok {
				return nil, fmt.Errorf("ruleset %q references unknown id %q", name, ref.ID)
			}
			if allowed, reason := g.Allows(d.Frontmatter); !allowed {
				return nil, fmt.Errorf("ruleset %q references %q, which does not meet the requirements: %s", name, ref.ID, reason)
			}

			c.Docs = append(c.Docs, CompiledDoc{
				ID:    ref.ID,
				Path:  d.Path,
				Scope: resolveScope(ref, d),
			})
		}
		out = append(out, c)
	}
	return out, nil
}

// resolveScope applies the resolution order: an explicit ruleset override,
// then the document's declared default, then none. An override that is present
// but empty resolves to none, which is the point of distinguishing it from an
// absent one.
func resolveScope(ref DocRef, d doc.Doc) []string {
	if ref.Scope != nil {
		return *ref.Scope
	}
	if scope := stringList(d.Frontmatter["scope"]); len(scope) > 0 {
		return scope
	}
	return nil
}

func stringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
