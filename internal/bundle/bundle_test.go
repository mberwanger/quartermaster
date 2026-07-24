package bundle

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/ruleset"
)

const fixture = "testdata/store"

func buildFixture(t *testing.T) *Bundle {
	t.Helper()

	cfg, err := config.Load(fixture)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	rs, err := ruleset.Load(filepath.Join(fixture, cfg.Rulesets))
	if err != nil {
		t.Fatalf("load rulesets: %v", err)
	}

	b, err := Build(Options{Root: fixture, Config: cfg, Rulesets: rs, Repo: "example", Commit: "deadbeef"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return b
}

func TestBuildCatalog(t *testing.T) {
	b := buildFixture(t)

	var ids []string
	for _, e := range b.Catalog {
		ids = append(ids, e.ID)
	}
	slices.Sort(ids)

	// The draft is retrievable and stays in the catalog; the restricted doc is
	// excluded entirely.
	want := []string{"eng.draft", "eng.errors", "eng.logging"}
	if !slices.Equal(ids, want) {
		t.Fatalf("catalog ids = %v, want %v", ids, want)
	}
	if b.Meta.Format != "0.3" {
		t.Fatalf("format = %q, want 0.3", b.Meta.Format)
	}
}

func TestBuildExcludesRestrictedFromTree(t *testing.T) {
	b := buildFixture(t)
	for _, f := range b.Files {
		if f.Path == "engineering/secret.md" {
			t.Fatal("restricted doc leaked into the store tree")
		}
	}
}

func TestBuildRulesetScopeResolution(t *testing.T) {
	b := buildFixture(t)

	byName := map[string]Compiled{}
	for _, c := range b.Rulesets {
		byName[c.Name] = c
	}

	core, ok := byName["core"]
	if !ok || len(core.Docs) != 2 {
		t.Fatalf("core ruleset = %+v, want 2 docs", core)
	}
	// eng.logging picks up its document-default scope; eng.errors stays
	// unscoped (resident).
	for _, d := range core.Docs {
		switch d.ID {
		case "eng.logging":
			if !slices.Equal(d.Scope, []string{"**/*.go"}) {
				t.Fatalf("core/eng.logging scope = %v, want [**/*.go]", d.Scope)
			}
		case "eng.errors":
			if len(d.Scope) != 0 {
				t.Fatalf("core/eng.errors scope = %v, want none (resident)", d.Scope)
			}
		}
	}

	// go-service overrides the scope for its own use.
	gs := byName["go-service"]
	if len(gs.Docs) != 1 || !slices.Equal(gs.Docs[0].Scope, []string{"**/*.go"}) {
		t.Fatalf("go-service = %+v, want one doc scoped [**/*.go]", gs)
	}
}

func TestBuildDeterministic(t *testing.T) {
	a := buildFixture(t)
	b := buildFixture(t)
	if a.Meta.Digest != b.Meta.Digest {
		t.Fatalf("digest not stable: %s != %s", a.Meta.Digest, b.Meta.Digest)
	}
	if a.Meta.Digest == "" {
		t.Fatal("digest is empty")
	}
}

// Compiled is re-exported from the ruleset package for brevity in this test.
type Compiled = ruleset.Compiled
