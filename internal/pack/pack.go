// Package pack reads the named selections a bundle compiles, and resolves them
// against the store.
//
// A package is a named selection of rules, skills, and agents. It contains no
// prose, so deleting every package removes injection and removes no knowledge:
// the documents stay in the retrievable tree either way.
//
// One name is what a repository asks for. That indirection is the point. A team
// with thirty skills lists them once here rather than in every repository that
// wants them, a skill shared by two teams appears in two packages and exists
// once, and adding one is a change in the store rather than a pull request
// against every consumer.
//
// Selections are patterns. A plain id is a pattern with no wildcard, so there is
// one rule rather than two, and `skills.data.*` selects a team's whole set
// without naming its members. Patterns match against ids because ids are the
// interface everywhere else in the tool: rulesets referenced them, manifests
// name them, and a facet record cites one months later.
package pack

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/gate"
)

// File is the parsed packages file: a mapping of package name to its selection.
type File map[string]Package

// Package is one named selection.
//
// Knowledge is deliberately absent. Every document in a bundle is retrievable
// whether or not a package names it, because knowledge on disk costs no context
// and a central store people cannot search is not one. A package decides what
// arrives without being asked for.
type Package struct {
	Rules  []Ref `yaml:"rules"`
	Skills []Ref `yaml:"skills"`
	Agents []Ref `yaml:"agents"`
}

// Ref selects documents, either by id pattern or by a predicate over
// frontmatter.
//
// Both spellings exist because they fail differently. A pattern is legible and
// depends on an id convention holding; a predicate survives documents moving and
// being renamed, at the cost of a field somebody has to maintain. Neither is
// right for every store.
type Ref struct {
	// ID is an id pattern. A `*` matches within one dot-separated segment and
	// `**` matches across segments, so `skills.data.*` takes a team's skills and
	// `skills.**` takes every skill there is.
	ID string
	// Scope overrides where a rule applies, and is meaningless for skills and
	// agents. A pointer, so an absent key and an empty list differ: absent
	// defers to whatever the document declares, and an explicit empty list
	// overrides that back to unscoped.
	Scope *[]string
	// Where selects on frontmatter instead of on the id.
	Where gate.Gate
}

// UnmarshalYAML accepts three spellings, because a list that is mostly bare ids
// should read like a list of bare ids.
//
//   - skills.data.warehouse          a bare id or pattern
//   - {id: eng.go-imports, scope: ["**/*.go"]}
//   - {where: {tags: [platform]}}
func (r *Ref) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&r.ID)
	}

	var form struct {
		ID    string    `yaml:"id"`
		Scope *[]string `yaml:"scope"`
		Where gate.Gate `yaml:"where"`
	}
	if err := node.Decode(&form); err != nil {
		return err
	}
	if form.ID == "" && form.Where.Empty() {
		return fmt.Errorf("a selection needs an id or a where clause")
	}
	if form.ID != "" && !form.Where.Empty() {
		return fmt.Errorf("a selection takes an id or a where clause, not both")
	}

	r.ID, r.Scope, r.Where = form.ID, form.Scope, form.Where
	return nil
}

// Load reads and parses a packages file.
//
// A missing file is an error rather than an empty set. This is only ever called
// for a path the store named in bundle.yaml, so the file not being there means
// the path is wrong, and treating that as "no packages" would build a bundle
// that carries nothing and says nothing about why.
func Load(path string) (File, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from bundle.yaml, chosen by whoever runs the tool
	if err != nil {
		return nil, err
	}

	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Names returns the package names in sorted order, so compilation is
// deterministic.
func (f File) Names() []string {
	names := make([]string, 0, len(f))
	for name := range f {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Compiled is a package resolved to a flat form: every selected document, with
// scope resolved and store paths attached. This is what a bundle carries, so a
// consumer renders it without evaluating any pattern or predicate.
type Compiled struct {
	Name   string `json:"name"`
	Rules  []Doc  `json:"rules,omitempty"`
	Skills []Doc  `json:"skills,omitempty"`
	Agents []Doc  `json:"agents,omitempty"`
}

// Doc is one resolved document. Scope resolution order is the package's
// override, then the document's own default, then none. None means resident:
// the rule loads at the start of every session.
type Doc struct {
	ID    string   `json:"id"`
	Path  string   `json:"path"`
	Scope []string `json:"scope,omitempty"`
}

// Kind names what a selection is being resolved as, so failures can say which
// list the mistake is in.
type Kind string

const (
	KindRule  Kind = "rules"
	KindSkill Kind = "skills"
	KindAgent Kind = "agents"
)

// Options carries what compilation needs to know about the store beyond the
// documents themselves.
type Options struct {
	// Requires is what a document must be before it may become a rule.
	Requires gate.Gate
	// IsSkill and IsAgent report whether a document may be selected as one.
	// They are passed in because what counts as a skill is declared in
	// bundle.yaml rather than known here.
	IsSkill func(doc.Doc) bool
	IsAgent func(doc.Doc) bool
}

// Compile resolves every package against the loaded documents.
//
// Explicit and pattern selections fail differently, and the difference is
// deliberate. Naming an id that does not exist, or that may not become what it
// was selected as, is an author's mistake and fails the build. A pattern that
// matches a document which does not qualify skips it, because the alternative is
// that drafting one document breaks every package that globs its neighbours.
//
// A pattern matching nothing is still an error. It is almost always a typo, and
// silently selecting nothing is how a repository ends up with no skills and no
// explanation.
func Compile(f File, docs []doc.Doc, opts Options) ([]Compiled, error) {
	byID := make(map[string]doc.Doc, len(docs))
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		if id := d.ID(); id != "" {
			byID[id] = d
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	out := make([]Compiled, 0, len(f))
	for _, name := range f.Names() {
		p := f[name]
		c := Compiled{Name: name}

		for _, sel := range []struct {
			kind Kind
			refs []Ref
			into *[]Doc
		}{
			{KindRule, p.Rules, &c.Rules},
			{KindSkill, p.Skills, &c.Skills},
			{KindAgent, p.Agents, &c.Agents},
		} {
			resolved, err := resolve(name, sel.kind, sel.refs, byID, ids, opts)
			if err != nil {
				return nil, err
			}
			*sel.into = resolved
		}
		out = append(out, c)
	}
	return out, nil
}

// resolve turns one list of selections into concrete documents, deduplicated and
// in the order they were selected.
func resolve(pkg string, kind Kind, refs []Ref, byID map[string]doc.Doc, ids []string, opts Options) ([]Doc, error) {
	var out []Doc
	seen := map[string]bool{}

	for _, ref := range refs {
		matched, exact, err := match(pkg, kind, ref, byID, ids)
		if err != nil {
			return nil, err
		}

		for _, id := range matched {
			d := byID[id]

			if ok, reason := qualifies(kind, d, opts); !ok {
				if exact {
					return nil, fmt.Errorf("package %q selects %q as %s, which it cannot be: %s", pkg, id, kind, reason)
				}
				continue // a pattern skips what does not qualify
			}
			if seen[id] {
				continue
			}
			seen[id] = true

			out = append(out, Doc{ID: id, Path: d.Path, Scope: scopeFor(kind, ref, d)})
		}
	}
	return out, nil
}

// match expands one selection into ids, and reports whether it named exactly one
// document rather than describing a set.
func match(pkg string, kind Kind, ref Ref, byID map[string]doc.Doc, ids []string) (matched []string, exact bool, err error) {
	switch {
	case !ref.Where.Empty():
		for _, id := range ids {
			if ok, _ := ref.Where.Allows(byID[id].Frontmatter); ok {
				matched = append(matched, id)
			}
		}
		if len(matched) == 0 {
			return nil, false, fmt.Errorf("package %q has a %s selection whose where clause matches nothing", pkg, kind)
		}
		return matched, false, nil

	case !strings.ContainsAny(ref.ID, "*?["):
		if _, ok := byID[ref.ID]; !ok {
			return nil, true, fmt.Errorf("package %q selects unknown id %q as %s", pkg, ref.ID, kind)
		}
		return []string{ref.ID}, true, nil

	default:
		for _, id := range ids {
			// Ids are dot-separated and the matcher is path-shaped, so the
			// separator is translated rather than reimplemented. A single star
			// then stops at a segment boundary and a double star crosses it,
			// which is the behavior anyone who has written a glob expects.
			ok, err := doublestar.Match(strings.ReplaceAll(ref.ID, ".", "/"), strings.ReplaceAll(id, ".", "/"))
			if err != nil {
				return nil, false, fmt.Errorf("package %q has an unreadable %s pattern %q: %w", pkg, kind, ref.ID, err)
			}
			if ok {
				matched = append(matched, id)
			}
		}
		if len(matched) == 0 {
			return nil, false, fmt.Errorf("package %q has a %s pattern %q that matches nothing", pkg, kind, ref.ID)
		}
		return matched, false, nil
	}
}

// qualifies reports whether a document may be selected as this kind.
func qualifies(kind Kind, d doc.Doc, opts Options) (bool, string) {
	switch kind {
	case KindRule:
		if ok, reason := opts.Requires.Allows(d.Frontmatter); !ok {
			return false, reason
		}
		if d.Restricted() {
			return false, "it is restricted and never leaves the store"
		}
	case KindSkill:
		if opts.IsSkill != nil && !opts.IsSkill(d) {
			return false, "it is not a skill"
		}
	case KindAgent:
		if opts.IsAgent != nil && !opts.IsAgent(d) {
			return false, "it is not an agent"
		}
	}
	return true, ""
}

// scopeFor resolves where a rule applies. Skills and agents have no scope: they
// are chosen when the work matches rather than loaded when a path does.
func scopeFor(kind Kind, ref Ref, d doc.Doc) []string {
	if kind != KindRule {
		return nil
	}
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
