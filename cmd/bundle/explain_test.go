package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/pack"
	"github.com/mberwanger/quartermaster/internal/preflight"
)

// explainFixture is a store carrying one document of each kind, plus a draft
// skill and a selection made by predicate rather than by id. Those are the three
// shapes explain used to answer wrongly, and the base fixture has none of them:
// it is rules all the way down, which is why nothing caught it.
func explainFixture(t *testing.T) string {
	t.Helper()
	root := copyStoreFixture(t)

	// The base store declares no skills gate, which would let anything be
	// selected as a skill and make the test prove nothing.
	appendTo(t, filepath.Join(root, "bundle.yaml"), "skills:\n  type: [skill]\n")

	// Each skill gets its own directory. A skill is a directory rather than a
	// file, so anything sitting beside one is an asset and not a document of its
	// own, which would quietly remove the agent from this fixture.
	writeDoc(t, root, "capabilities/skills/review/skill.md", `---
id: skills.review
title: Review a diff
description: Read a change the way a reviewer would, in a fixed order.
type: skill
status: stable
skill:
  name: review
---

# Review a diff
`)

	// Draft on purpose, and deliberately not selected by any package, so the
	// fixture still compiles. The test that selects it does so on its own copy.
	writeDoc(t, root, "capabilities/skills/draft-review/skill.md", `---
id: skills.draft-review
title: Review a diff, unfinished
description: An unfinished skill, kept to prove the rule gate does not govern skills.
type: skill
status: draft
skill:
  name: draft-review
---

# Unfinished
`)

	writeDoc(t, root, "capabilities/agents/reviewer.md", `---
id: agents.reviewer
title: Doc reviewer
description: Delegates a documentation review with its own tools.
type: agent
status: stable
agent:
  name: reviewer
  tools: [Read, Grep]
---

# Doc reviewer
`)

	writeDoc(t, root, "engineering/tagged.md", `---
id: eng.tagged
title: A tagged convention
description: Selected by a where clause rather than by id, which explain used to miss.
type: concept
status: stable
tags: [ops]
---

# A tagged convention
`)

	writeDoc(t, root, "meta/packages.yaml", `core:
  rules:
    - eng.logging
    - eng.errors
capable:
  skills:
    - skills.review
  agents:
    - agents.reviewer
tagged:
  rules:
    - where: { tags: [ops] }
`)

	return root
}

func writeDoc(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, body string) {
	t.Helper()
	existing, err := os.ReadFile(path) //nolint:gosec // a fixture path this test built
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(existing, []byte("\n"+body)...), 0o600); err != nil {
		t.Fatal(err)
	}
}

func explainOutput(t *testing.T, root, id string) string {
	t.Helper()
	command := newExplainCmd()
	command.root = root

	var out bytes.Buffer
	if err := command.run(&out, id); err != nil {
		t.Fatalf("explain %s: %v", id, err)
	}
	return out.String()
}

// A document is not always a rule, and saying so was wrong twice over: the noun
// was wrong, and the gate reported beside it was the one that governs rules.
func TestExplainNamesTheKindItIsSelectedAs(t *testing.T) {
	root := explainFixture(t)

	tests := map[string]struct{ id, want string }{
		"rule":  {"eng.logging", "materializes as a rule via"},
		"skill": {"skills.review", "materializes as a skill via"},
		"agent": {"agents.reviewer", "materializes as an agent via"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := explainOutput(t, root, test.id)
			if !strings.Contains(got, test.want) {
				t.Errorf("want %q in:\n%s", test.want, got)
			}
			if name != "rule" && strings.Contains(got, "as a rule") {
				t.Errorf("a %s was reported as a rule:\n%s", name, got)
			}
		})
	}
}

// The gate governs capabilities too. A draft skill used to ship, tool grants and
// all, because requires was applied to rules alone; explain has to report that
// the same way the compiler decides it.
func TestExplainReportsACapabilityTheGateRejects(t *testing.T) {
	root := explainFixture(t)

	// Select the draft skill, which the base fixture deliberately leaves alone.
	appendTo(t, filepath.Join(root, "meta", "packages.yaml"), `drafty:
  skills:
    - skills.draft-review
`)

	got := explainOutput(t, root, "skills.draft-review")
	if !strings.Contains(got, "which it cannot be") || !strings.Contains(got, `status is "draft"`) {
		t.Errorf("a gate-rejected skill was not reported as such:\n%s", got)
	}
	if strings.Contains(got, "materializes as a skill") {
		t.Errorf("explain says a draft skill ships:\n%s", got)
	}

	// And the claim is checked against the thing that decides it.
	_, err := preflight.Run(preflight.Options{Root: root})
	if err == nil {
		t.Fatal("the compiler accepted a draft skill that explain says it refuses")
	}
	if !strings.Contains(err.Error(), `status is "draft"`) {
		t.Errorf("compiler refused it for a different reason: %v", err)
	}
}

// A selection may be a predicate instead of an id. explain skipped those
// outright, so a store using where clauses got a confidently short answer.
func TestExplainSeesAPredicateSelection(t *testing.T) {
	root := explainFixture(t)

	got := explainOutput(t, root, "eng.tagged")
	if !strings.Contains(got, "tagged") {
		t.Errorf("a where-clause selection was not reported:\n%s", got)
	}
}

// The guard that matters. explain resolves selection itself rather than
// compiling, so it can work on a store that does not build, and every one of
// these defects was that second implementation drifting from the real one. This
// pins them together: whatever the compiler decided, explain has to say.
func TestExplainAgreesWithTheCompiler(t *testing.T) {
	root := explainFixture(t)

	result, err := preflight.Run(preflight.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}

	compiled := map[string]map[pack.Kind][]string{}
	for _, c := range result.Bundle.Packages {
		for _, sel := range []struct {
			kind pack.Kind
			docs []pack.Doc
		}{
			{pack.KindRule, c.Rules},
			{pack.KindSkill, c.Skills},
			{pack.KindAgent, c.Agents},
		} {
			for _, d := range sel.docs {
				if compiled[d.ID] == nil {
					compiled[d.ID] = map[pack.Kind][]string{}
				}
				compiled[d.ID][sel.kind] = append(compiled[d.ID][sel.kind], c.Name)
			}
		}
	}

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := loadPackages(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := doc.Load(root, []string{"dist"})
	if err != nil {
		t.Fatal(err)
	}

	var checked int
	for _, d := range docs {
		id := d.ID()
		if id == "" {
			continue
		}

		reported := map[pack.Kind][]string{}
		for _, sel := range packagesReferencing(packages, id, d.Frontmatter) {
			names := slices.Clone(sel.packages)
			slices.Sort(names)
			reported[sel.kind] = names
		}

		want := compiled[id]
		for kind, names := range want {
			slices.Sort(names)
			if !slices.Equal(reported[kind], names) {
				t.Errorf("%s as %s: explain says %v, the compiler says %v", id, kind, reported[kind], names)
			}
		}
		for kind, names := range reported {
			if _, ok := want[kind]; !ok {
				t.Errorf("%s as %s: explain says %v, the compiler selected it as no such thing", id, kind, names)
			}
		}
		if len(want) > 0 {
			checked++
		}
	}

	// A fixture that stopped selecting anything would pass every assertion above
	// while proving nothing.
	if checked < 4 {
		t.Fatalf("only %d selected document(s) compared; the fixture is not exercising the kinds", checked)
	}
}
