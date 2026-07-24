// Package plan computes what a sync would materialize, without writing anything.
//
// It is the one implementation of the consumer pipeline: resolve the manifest's
// bundles, select the named rulesets, render the targets, and return the full
// set of output files. qm sync writes and prunes what plan produces; qm verify
// diffs it against the working tree; qm status reports it. Sharing one
// implementation is what keeps those three commands from disagreeing about what
// a repository should contain.
package plan

import (
	"fmt"
	"maps"
	"path"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/provider"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/target"
)

// KnowledgeDir is where the retrievable tree is materialized, relative to the
// repository root. It is target-independent: the knowledge is the same for every
// harness.
var KnowledgeDir = path.Join(state.Dir, "knowledge")

// Result is everything a consumer command needs from a resolved manifest.
type Result struct {
	// Outputs is every file this sync would write, repository-relative, with its
	// content. It is the union of the knowledge tree and every target's rules.
	Outputs map[string][]byte
	// Docs are the selected documents, deduplicated by id with later bundles
	// winning.
	Docs []target.Doc
	// Skills are the selected skills, each with the assets that travel with it.
	Skills []target.Skill
	// Agents are the selected agent definitions.
	Agents []target.Agent
	// Knowledge is the retrievable tree subset of Outputs.
	Knowledge map[string][]byte
	// Blocks are managed regions to splice into committed files (e.g. AGENTS.md).
	// They are kept out of Outputs because they are spliced, not replaced, and
	// never pruned.
	Blocks []target.Block
	// Bundles records each resolved source and its digest, for state and status.
	Bundles []state.Bundle
	// Targets is the target names the manifest declared.
	Targets []string
	// Warnings are non-fatal notices: digest mismatches, budget overruns.
	Warnings []string
	// ResidentBytes is the total prose size of unscoped (resident) documents.
	ResidentBytes int
	// Budget is the manifest's declared budget.
	Budget manifest.Budget
}

// Compute resolves the manifest in dir and produces the full output set.
func Compute(dir string) (*Result, error) {
	m, err := manifest.Load(dir)
	if err != nil {
		return nil, err
	}

	targets, err := ResolveTargets(m.Targets)
	if err != nil {
		return nil, err
	}

	r := &Result{
		Knowledge: make(map[string][]byte),
		Targets:   m.Targets,
		Budget:    m.Budget,
	}

	byID := make(map[string]target.Doc)
	var order []string

	for _, mb := range m.Bundles {
		b, err := provider.Resolve(mb.Source, dir)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", mb.Source, err)
		}
		if mb.Digest != "" && b.Meta.Digest != mb.Digest {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s resolved to %s, manifest pins %s", mb.Source, b.Meta.Digest, mb.Digest))
		}

		bodyByPath := make(map[string][]byte, len(b.Files))
		for _, f := range b.Files {
			bodyByPath[f.Path] = f.Body
		}
		descByID := make(map[string]string, len(b.Catalog))
		fmByPath := make(map[string]map[string]any, len(b.Catalog))
		for _, e := range b.Catalog {
			descByID[e.ID], _ = e.Frontmatter["description"].(string)
			fmByPath[e.Path] = e.Frontmatter
		}

		for _, name := range mb.Rulesets {
			rs, ok := findRuleset(b, name)
			if !ok {
				return nil, fmt.Errorf("%s has no ruleset %q", mb.Source, name)
			}
			for _, cd := range rs.Docs {
				body, ok := bodyByPath[cd.Path]
				if !ok {
					return nil, fmt.Errorf("ruleset %q references %s at %s, absent from the store tree", name, cd.ID, cd.Path)
				}
				// Selecting a ruleset whose document the knowledge filter drops
				// is a contradiction. Materializing the rule would ignore the
				// filter; dropping it silently would ignore the ruleset. Say so
				// instead, and let the author resolve it.
				if !mb.Knowledge.Empty() {
					if allowed, reason := mb.Knowledge.Allows(fmByPath[cd.Path]); !allowed {
						return nil, fmt.Errorf("ruleset %q needs %s, which the knowledge filter excludes: %s",
							name, cd.ID, reason)
					}
				}
				d := target.Doc{
					ID:          cd.ID,
					Path:        cd.Path,
					Description: descByID[cd.ID],
					Scope:       cd.Scope,
					Prose:       doc.Prose(body),
					Digest:      b.Meta.Digest,
					Commit:      b.Meta.Source.Commit,
				}
				if _, seen := byID[cd.ID]; !seen {
					order = append(order, cd.ID)
				}
				byID[cd.ID] = d
			}
		}

		for _, id := range mb.Skills {
			s, err := resolveSkill(b, id, bodyByPath)
			if err != nil {
				return nil, err
			}
			s.Digest, s.Commit = b.Meta.Digest, b.Meta.Source.Commit
			r.Skills = append(r.Skills, *s)
		}

		for _, id := range mb.Agents {
			a, err := resolveAgent(b, id, bodyByPath)
			if err != nil {
				return nil, err
			}
			a.Digest, a.Commit = b.Meta.Digest, b.Meta.Source.Commit
			r.Agents = append(r.Agents, *a)
		}

		for _, f := range b.Files {
			if !mb.Knowledge.Empty() {
				// Only catalog documents can be judged. The reserved index
				// listings are dropped once a filter is on, because a listing
				// that names documents this repository no longer carries points
				// at files that are not there.
				fm, isDoc := fmByPath[f.Path]
				if !isDoc {
					continue
				}
				if allowed, _ := mb.Knowledge.Allows(fm); !allowed {
					continue
				}
			}
			r.Knowledge[path.Join(KnowledgeDir, f.Path)] = f.Body
		}

		r.Bundles = append(r.Bundles, state.Bundle{
			Source:    mb.Source,
			Digest:    b.Meta.Digest,
			Rulesets:  mb.Rulesets,
			Knowledge: mb.Knowledge.Fields(),
		})
	}

	for _, id := range order {
		d := byID[id]
		r.Docs = append(r.Docs, d)
		if len(d.Scope) == 0 {
			r.ResidentBytes += len(d.Prose)
		}
	}

	// Assemble the output set: knowledge tree plus every target's rules.
	r.Outputs = make(map[string][]byte, len(r.Knowledge))
	maps.Copy(r.Outputs, r.Knowledge)

	in := target.Input{
		Docs:    r.Docs,
		Skills:  r.Skills,
		Agents:  r.Agents,
		Bundles: bundlesForTarget(r.Bundles),
	}
	for _, t := range targets {
		out, err := t.Render(in)
		if err != nil {
			return nil, fmt.Errorf("target %s: %w", t.Name(), err)
		}
		for _, f := range out.Files {
			r.Outputs[f.Path] = f.Body
		}
		r.Blocks = append(r.Blocks, out.Blocks...)
	}

	if r.Budget.ResidentBytes > 0 && r.ResidentBytes > r.Budget.ResidentBytes {
		r.Warnings = append(r.Warnings, fmt.Sprintf("resident set is %d B, over the %d B budget", r.ResidentBytes, r.Budget.ResidentBytes))
	}

	return r, nil
}
