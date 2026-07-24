// Package gate evaluates what a document must be to become a rule.
//
// The requirements are declared once in the store's bundle.yaml, under the
// requires key, and applied at build time, so a compiled bundle contains only
// documents that already qualify and consumers evaluate nothing. A document that
// falls short is not knowledge that is hidden; it simply never becomes a rule
// that loads on its own.
package gate

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Gate is an ordered set of field predicates. A document passes only when every
// predicate holds. The zero Gate has no predicates and passes everything, which
// is the correct default for a store that declares no gate.
type Gate struct {
	fields map[string]Predicate
}

// Predicate constrains a single frontmatter field. Exactly one of In or Not is
// populated, decided by how the field was written in YAML: a sequence is an
// allowlist (the value must be In the list), a mapping "{ not: [...] }" is a
// denylist (the value must be absent from the list).
type Predicate struct {
	In  []string
	Not []string
}

// UnmarshalYAML accepts either a sequence (allowlist) or a "{ not: [...] }"
// mapping (denylist) for a single field.
func (p *Predicate) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&p.In)
	case yaml.MappingNode:
		var m struct {
			Not []string `yaml:"not"`
		}
		if err := node.Decode(&m); err != nil {
			return err
		}
		if m.Not == nil {
			return fmt.Errorf("gate predicate mapping must use the 'not' key")
		}
		p.Not = m.Not
		return nil
	default:
		return fmt.Errorf("gate predicate must be a list or a { not: [...] } mapping")
	}
}

// UnmarshalYAML reads the gate as a mapping of field name to predicate.
func (g *Gate) UnmarshalYAML(node *yaml.Node) error {
	var fields map[string]Predicate
	if err := node.Decode(&fields); err != nil {
		return err
	}
	g.fields = fields
	return nil
}

// Fields returns the constrained field names in sorted order.
func (g Gate) Fields() []string {
	names := make([]string, 0, len(g.fields))
	for k := range g.fields {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Empty reports whether no predicates are declared, in which case everything
// passes. A caller uses it to skip filtering entirely rather than run a no-op
// check over every document.
func (g Gate) Empty() bool {
	return len(g.fields) == 0
}

// Allows reports whether frontmatter satisfies every predicate. When it does
// not, the returned reason names the first failing field, so an author sees one
// command's worth of explanation for why a document did not qualify.
//
// A field may be a scalar (domain, status) or a list (tags). A list matches when
// any of its entries does, which is what makes "tags: [payments]" select every
// document tagged payments among others.
//
// A field absent from frontmatter reads as the empty string. An allowlist then
// rejects it (the empty string is not among the allowed values); a denylist
// accepts it (the empty string is not among the forbidden values). This is what
// makes "visibility: { not: [restricted] }" pass a document that omits
// visibility entirely.
func (g Gate) Allows(frontmatter map[string]any) (bool, string) {
	for _, field := range g.Fields() {
		p := g.fields[field]
		values := fieldValues(frontmatter[field])

		switch {
		case len(p.In) > 0:
			if !anyIn(values, p.In) {
				return false, fmt.Sprintf("%s is %s, requires one of: %s",
					field, describe(values), strings.Join(p.In, ", "))
			}
		case len(p.Not) > 0:
			if anyIn(values, p.Not) {
				return false, fmt.Sprintf("%s is %s, which is excluded",
					field, describe(values))
			}
		}
	}
	return true, ""
}

// fieldValues normalizes a frontmatter value into the strings to match against.
// A missing or non-string value yields one empty string, so an allowlist rejects
// it rather than silently passing.
func fieldValues(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{""}
	}
}

func anyIn(values, list []string) bool {
	for _, v := range values {
		if slices.Contains(list, v) {
			return true
		}
	}
	return false
}

// describe renders the values a field held, for the failure reason.
func describe(values []string) string {
	switch len(values) {
	case 0:
		return "empty"
	case 1:
		return fmt.Sprintf("%q", values[0])
	default:
		quoted := make([]string, 0, len(values))
		for _, v := range values {
			quoted = append(quoted, fmt.Sprintf("%q", v))
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	}
}
