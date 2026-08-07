package bundle

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/pack"
)

const fixture = "testdata/store"

func buildFixture(t *testing.T) *Bundle {
	t.Helper()

	cfg, err := config.Load(fixture)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	rs, err := pack.Load(filepath.Join(fixture, cfg.Packages))
	if err != nil {
		t.Fatalf("load rulesets: %v", err)
	}

	b, err := Build(Options{Root: fixture, Config: cfg, Packages: rs, Repo: "example", Commit: "deadbeef"})
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
	if b.Meta.Format != "0.4" {
		t.Fatalf("format = %q, want 0.4", b.Meta.Format)
	}
}

func TestBuildPropagatesRootOKFVersion(t *testing.T) {
	tests := []struct {
		name      string
		rootIndex string
		want      string
	}{
		{
			name:      "declared 0.1",
			rootIndex: "---\nokf_version: \"0.1\"\n---\n\n# Store\n",
			want:      "0.1",
		},
		{
			name:      "declared 0.2",
			rootIndex: "---\nokf_version: \"0.2\"\n---\n\n# Store\n",
			want:      "0.2",
		},
		{
			name:      "missing declaration",
			rootIndex: "# Store\n",
			want:      "0.1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storeRoot := t.TempDir()
			if err := os.CopyFS(storeRoot, os.DirFS(fixture)); err != nil {
				t.Fatalf("copy fixture: %v", err)
			}
			if err := os.WriteFile(filepath.Join(storeRoot, "index.md"), []byte(test.rootIndex), 0o644); err != nil {
				t.Fatalf("write root index: %v", err)
			}

			cfg, err := config.Load(storeRoot)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			packages, err := pack.Load(filepath.Join(storeRoot, cfg.Packages))
			if err != nil {
				t.Fatalf("load packages: %v", err)
			}
			builtBundle, err := Build(Options{Root: storeRoot, Config: cfg, Packages: packages})
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			if builtBundle.Meta.OKFVersion != test.want {
				t.Fatalf("okf_version = %q, want %q", builtBundle.Meta.OKFVersion, test.want)
			}
		})
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
	for _, c := range b.Packages {
		byName[c.Name] = c
	}

	core, ok := byName["core"]
	if !ok || len(core.Rules) != 2 {
		t.Fatalf("core ruleset = %+v, want 2 docs", core)
	}
	// eng.logging picks up its document-default scope; eng.errors stays
	// unscoped (resident).
	for _, d := range core.Rules {
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
	if len(gs.Rules) != 1 || !slices.Equal(gs.Rules[0].Scope, []string{"**/*.go"}) {
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
type Compiled = pack.Compiled
