package pack

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/gate"
)

func parse(t *testing.T, body string) File {
	t.Helper()
	var f File
	if err := yaml.Unmarshal([]byte(body), &f); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func mk(id, docType string, extra ...string) doc.Doc {
	fm := map[string]any{"id": id, "type": docType, "status": "active"}
	for i := 0; i+1 < len(extra); i += 2 {
		fm[extra[i]] = extra[i+1]
	}
	return doc.Doc{Path: strings.ReplaceAll(id, ".", "/") + ".md", Frontmatter: fm}
}

func opts() Options {
	return Options{
		IsSkill: func(d doc.Doc) bool { return d.Str("type") == "skill" },
		IsAgent: func(d doc.Doc) bool { return d.Str("type") == "agent" },
	}
}

var store = []doc.Doc{
	mk("engineering.commit-messages", "concept"),
	mk("engineering.go-imports", "concept", "scope-marker", "yes"),
	mk("skills.data.warehouse", "skill"),
	mk("skills.data.pipelines", "skill"),
	mk("skills.engineering.review-a-diff", "skill"),
	mk("agents.doc-reviewer", "agent"),
}

// The point of the whole change: a team's skills are selected without naming
// them, so adding one is a change here rather than in every repository.
func TestPatternSelectsASet(t *testing.T) {
	f := parse(t, `
data-engineering:
  skills:
    - skills.data.*
`)
	got, err := Compile(f, store, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Skills) != 2 {
		t.Fatalf("got %+v, want two skills", got)
	}
}

// A single star stops at a segment boundary; a double star crosses it. Anyone
// who has written a glob expects both.
func TestStarDoesNotCrossSegments(t *testing.T) {
	one, err := Compile(parse(t, "p:\n  skills: [skills.*]\n"), store, opts())
	if err == nil {
		t.Fatalf("skills.* should match nothing, got %+v", one[0].Skills)
	}

	all, err := Compile(parse(t, "p:\n  skills: [skills.**]\n"), store, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(all[0].Skills) != 3 {
		t.Errorf("skills.** matched %d, want 3", len(all[0].Skills))
	}
}

// A shared skill appears in two packages and exists once.
func TestOverlapBetweenPackages(t *testing.T) {
	got, err := Compile(parse(t, `
platform:
  skills: [skills.engineering.review-a-diff]
data-engineering:
  skills:
    - skills.engineering.review-a-diff
    - skills.data.*
`), store, opts())
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]Compiled{}
	for _, c := range got {
		byName[c.Name] = c
	}
	if len(byName["platform"].Skills) != 1 {
		t.Errorf("platform: %+v", byName["platform"].Skills)
	}
	if len(byName["data-engineering"].Skills) != 3 {
		t.Errorf("data-engineering: %+v", byName["data-engineering"].Skills)
	}
}

// Selecting the same document twice in one package yields it once, in the order
// it was first selected.
func TestDeduplicatesWithinAPackage(t *testing.T) {
	got, err := Compile(parse(t, `
p:
  skills:
    - skills.data.warehouse
    - skills.data.*
`), store, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Skills) != 2 {
		t.Fatalf("got %d skills, want 2: %+v", len(got[0].Skills), got[0].Skills)
	}
	if got[0].Skills[0].ID != "skills.data.warehouse" {
		t.Errorf("first is %q, want the explicitly named one", got[0].Skills[0].ID)
	}
}

// Naming something that cannot be what it was selected as is the author's
// mistake and fails. A pattern that sweeps one up skips it instead, or drafting
// a single document would break every package globbing its neighbours.
func TestExplicitFailsAndPatternSkips(t *testing.T) {
	if _, err := Compile(parse(t, "p:\n  skills: [engineering.commit-messages]\n"), store, opts()); err == nil {
		t.Error("naming a concept as a skill should fail")
	}

	got, err := Compile(parse(t, "p:\n  skills: [engineering.**]\n"), store, opts())
	if err == nil && len(got[0].Skills) != 0 {
		t.Errorf("a pattern should skip non-skills, got %+v", got[0].Skills)
	}
}

// Agents glob like everything else. A team that grows a second reviewer should
// not have to remember to add it to six packages.
func TestPatternsSelectAgentsToo(t *testing.T) {
	docs := append([]doc.Doc{}, store...)
	docs = append(docs, mk("agents.release-notes", "agent"))

	got, err := Compile(parse(t, "p:\n  agents: [agents.*]\n"), docs, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Agents) != 2 {
		t.Fatalf("matched %d agents, want 2: %+v", len(got[0].Agents), got[0].Agents)
	}
	// And a pattern still refuses to make an agent out of something that is not.
	if _, err := Compile(parse(t, "p:\n  agents: [engineering.commit-messages]\n"), docs, opts()); err == nil {
		t.Error("naming a concept as an agent should fail")
	}
}

func TestUnknownIdFails(t *testing.T) {
	if _, err := Compile(parse(t, "p:\n  skills: [skills.nope]\n"), store, opts()); err == nil {
		t.Error("an unknown id should fail")
	}
}

// A pattern matching nothing is almost always a typo, and selecting nothing
// silently is how a repository ends up with no skills and no explanation.
func TestPatternMatchingNothingFails(t *testing.T) {
	_, err := Compile(parse(t, "p:\n  skills: [skills.finance.*]\n"), store, opts())
	if err == nil {
		t.Fatal("a pattern matching nothing should fail")
	}
	if !strings.Contains(err.Error(), "matches nothing") {
		t.Errorf("error does not say why: %v", err)
	}
}

// Rules are gated; skills and agents are not gated the same way, because what
// may become a rule is a different question from what is a skill.
func TestRulesAreGated(t *testing.T) {
	docs := append([]doc.Doc{}, store...)
	docs = append(docs, mk("engineering.draft-thing", "concept", "status", "draft"))

	o := opts()
	o.Requires = mustGate(t, "status: [active]\n")

	if _, err := Compile(parse(t, "p:\n  rules: [engineering.draft-thing]\n"), docs, o); err == nil {
		t.Error("naming a draft as a rule should fail")
	}

	got, err := Compile(parse(t, "p:\n  rules: [engineering.**]\n"), docs, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got[0].Rules {
		if r.ID == "engineering.draft-thing" {
			t.Error("a pattern selected a document the gate rejects")
		}
	}
}

// A restricted document never leaves the store, so it can never be a rule, and
// that holds whatever the store's requirements say.
func TestRestrictedIsNeverARule(t *testing.T) {
	docs := append([]doc.Doc{}, mk("engineering.secret", "concept", "visibility", "restricted"))
	if _, err := Compile(parse(t, "p:\n  rules: [engineering.secret]\n"), docs, opts()); err == nil {
		t.Error("a restricted document should not be selectable as a rule")
	}
}

// Scope is the consumer's business: the same document is resident in one
// package and scoped in another.
func TestScopeOverridePerPackage(t *testing.T) {
	docs := []doc.Doc{{
		Path:        "engineering/go-imports.md",
		Frontmatter: map[string]any{"id": "engineering.go-imports", "scope": []any{"**/*.go"}},
	}}

	got, err := Compile(parse(t, `
inherits:
  rules: [engineering.go-imports]
overrides:
  rules:
    - id: engineering.go-imports
      scope: ["cmd/**"]
resident:
  rules:
    - id: engineering.go-imports
      scope: []
`), docs, opts())
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{
		"inherits":  {"**/*.go"},
		"overrides": {"cmd/**"},
		"resident":  nil,
	}
	for _, c := range got {
		scope := c.Rules[0].Scope
		if len(scope) != len(want[c.Name]) || (len(scope) > 0 && scope[0] != want[c.Name][0]) {
			t.Errorf("%s: scope = %v, want %v", c.Name, scope, want[c.Name])
		}
	}
}

// A predicate survives documents being renamed and moved, which an id pattern
// does not.
func TestWhereClause(t *testing.T) {
	docs := []doc.Doc{
		mk("skills.one", "skill", "team", "platform"),
		mk("skills.two", "skill", "team", "growth"),
	}
	got, err := Compile(parse(t, `
platform:
  skills:
    - where: {team: [platform]}
`), docs, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Skills) != 1 || got[0].Skills[0].ID != "skills.one" {
		t.Errorf("got %+v", got[0].Skills)
	}
}

func TestRefRejectsBothIdAndWhere(t *testing.T) {
	var f File
	err := yaml.Unmarshal([]byte("p:\n  skills:\n    - {id: a.b, where: {team: [x]}}\n"), &f)
	if err == nil {
		t.Fatal("a selection with both an id and a where clause should be rejected")
	}
}

func mustGate(t *testing.T, body string) gate.Gate {
	t.Helper()
	var g gate.Gate
	if err := yaml.Unmarshal([]byte(body), &g); err != nil {
		t.Fatal(err)
	}
	return g
}
