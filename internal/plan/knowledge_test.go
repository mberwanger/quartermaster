package plan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// manifestWith builds a consumer manifest against the fixture store with the
// given packages and knowledge filter block.
func manifestWith(t *testing.T, packages, knowledge string) string {
	t.Helper()
	store, err := filepath.Abs("../bundle/testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeManifest(t, dir, "bundles:\n"+
		"  - source: file://"+store+"\n"+
		"    use: ["+packages+"]\n"+
		knowledge+
		"targets:\n  - claude\n")
	return dir
}

func knowledgeIDs(r *Result) []string {
	var out []string
	for p := range r.Knowledge {
		out = append(out, filepath.Base(p))
	}
	return out
}

func has(list []string, want string) bool {
	return slices.Contains(list, want)
}

// Without a filter the whole tree lands, index listings included.
func TestNoFilterShipsEverything(t *testing.T) {
	r, err := Compute(manifestWith(t, "core", ""))
	if err != nil {
		t.Fatal(err)
	}
	names := knowledgeIDs(r)
	for _, want := range []string{"logging.md", "errors.md", "draft-idea.md", "index.md"} {
		if !has(names, want) {
			t.Fatalf("expected %s in the tree, got %v", want, names)
		}
	}
}

// A list-valued field matches when any entry does: tags [go, observability]
// matches a filter on observability.
func TestFilterOnListField(t *testing.T) {
	r, err := Compute(manifestWith(t, "go-service",
		"    knowledge:\n      tags: [observability]\n"))
	if err != nil {
		t.Fatal(err)
	}

	names := knowledgeIDs(r)
	if !has(names, "logging.md") {
		t.Fatalf("logging.md should match tags [observability], got %v", names)
	}
	// errors.md is tagged only go, and the draft has no tags at all.
	for _, unwanted := range []string{"errors.md", "draft-idea.md"} {
		if has(names, unwanted) {
			t.Fatalf("%s should have been filtered out, got %v", unwanted, names)
		}
	}
	// Reserved listings are dropped once a filter is on, so nothing dangles.
	if has(names, "index.md") {
		t.Fatalf("index listings should be dropped when filtering, got %v", names)
	}
}

// A scalar field filter behaves the same way.
func TestFilterOnScalarField(t *testing.T) {
	r, err := Compute(manifestWith(t, "core",
		"    knowledge:\n      domain: [engineering]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Knowledge) == 0 {
		t.Fatal("expected engineering docs to survive the filter")
	}
	for p := range r.Knowledge {
		if strings.HasSuffix(p, "index.md") {
			t.Fatalf("listing survived a filtered tree: %s", p)
		}
	}
}

// Selecting a package whose document the filter excludes is a contradiction, and
// must be reported rather than silently resolved either way.
func TestFilterContradictingPackageFails(t *testing.T) {
	_, err := Compute(manifestWith(t, "core",
		"    knowledge:\n      tags: [observability]\n"))
	if err == nil {
		t.Fatal("expected a contradiction between the package and the filter")
	}
	// core needs eng.errors, which is tagged go rather than observability.
	if !strings.Contains(err.Error(), "eng.errors") || !strings.Contains(err.Error(), "knowledge filter") {
		t.Fatalf("error should name the document and the filter, got: %v", err)
	}
}

// storeWithACapability copies the fixture and adds a skill and an agent, which
// the base store has none of. That absence is why the contradiction below went
// unnoticed: every test exercising the filter used a store made only of rules.
func storeWithACapability(t *testing.T) string {
	t.Helper()
	src, err := filepath.Abs("../bundle/testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}

	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("capabilities/review/skill.md", "---\nid: skills.review\ntitle: Review a diff\n"+
		"description: Read a change the way a reviewer would.\ntype: skill\nstatus: stable\n"+
		"skill:\n  allowed-tools: [Read]\n---\n\n# Review\n")
	write("capabilities/reviewer.md", "---\nid: agents.reviewer\ntitle: Doc reviewer\n"+
		"description: Delegates a documentation review.\ntype: agent\nstatus: stable\n"+
		"agent:\n  tools: [Read]\n---\n\n# Reviewer\n")
	write("meta/packages.yaml", "capable:\n  skills:\n    - skills.review\n  agents:\n    - agents.reviewer\n")

	return root
}

// The contradiction check covered rules and not capabilities, so a repository
// could filter a document out of its knowledge tree and still be handed the same
// text as a loadable skill, with whatever tools it declared. Verified against the
// real store before the fix: type: [concept] dropped the skill from the tree and
// materialized it into .claude/skills anyway.
func TestFilterContradictingCapabilityFails(t *testing.T) {
	store := storeWithACapability(t)

	for _, kind := range []string{"skill", "agent"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			writeManifest(t, dir, "bundles:\n"+
				"  - source: file://"+store+"\n"+
				"    use: [capable]\n"+
				"    knowledge:\n      type: { not: ["+kind+"] }\n"+
				"targets:\n  - claude\n")

			_, err := Compute(dir)
			if err == nil {
				t.Fatalf("a %s the knowledge filter excludes was materialized anyway", kind)
			}
			if !strings.Contains(err.Error(), "knowledge filter") {
				t.Fatalf("error should name the filter, got: %v", err)
			}
		})
	}
}

// The filter is recorded so a partial tree is never mistaken for the whole one.
func TestFilterRecordedForStatus(t *testing.T) {
	r, err := Compute(manifestWith(t, "go-service",
		"    knowledge:\n      tags: [observability]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Bundles) != 1 || len(r.Bundles[0].Knowledge) != 1 || r.Bundles[0].Knowledge[0] != "tags" {
		t.Fatalf("filter fields = %+v, want [tags]", r.Bundles)
	}
}
