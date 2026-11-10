package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mberwanger/quartermaster/internal/doc"
)

func load(t *testing.T, body string) *Config {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mk(path string, fm map[string]any) doc.Doc {
	return doc.Doc{Path: path, Frontmatter: fm}
}

// A store that declares nothing gets the obvious answer. This used to be the
// opposite: an absent gate was an empty gate, an empty gate admits everything,
// and so every document in a store that never mentioned skills could be selected
// as one. Verified against the fixture store before the change, where a
// type: concept document compiled as a skill without complaint.
func TestCapabilitiesDefaultToType(t *testing.T) {
	c := load(t, "name: s\n")

	tests := map[string]struct {
		fm           map[string]any
		skill, agent bool
	}{
		"a skill":   {map[string]any{"type": "skill"}, true, false},
		"an agent":  {map[string]any{"type": "agent"}, false, true},
		"a concept": {map[string]any{"type": "concept"}, false, false},
		"no type":   {map[string]any{}, false, false},
		// A block is configuration, not identity. An agent block on something
		// that does not say it is an agent used to make it one.
		"a concept carrying an agent block": {
			map[string]any{"type": "concept", "agent": map[string]any{"tools": []any{"Read"}}}, false, false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d := mk("x.md", test.fm)
			if got := c.IsSkill(d); got != test.skill {
				t.Errorf("IsSkill = %v, want %v", got, test.skill)
			}
			if got := c.IsAgent(d); got != test.agent {
				t.Errorf("IsAgent = %v, want %v", got, test.agent)
			}
		})
	}
}

// A store whose vocabulary differs says so, and then its predicate decides
// rather than the default.
func TestDeclaredGateOverridesTheDefault(t *testing.T) {
	c := load(t, "name: s\nskills:\n  type: [playbook]\nagents:\n  type: [delegate]\n")

	if !c.IsSkill(mk("a.md", map[string]any{"type": "playbook"})) {
		t.Error("the declared skills predicate was not applied")
	}
	if c.IsSkill(mk("b.md", map[string]any{"type": "skill"})) {
		t.Error("the default still applied alongside a declared predicate")
	}
	if !c.IsAgent(mk("c.md", map[string]any{"type": "delegate"})) {
		t.Error("the declared agents predicate was not applied")
	}
	if c.IsAgent(mk("d.md", map[string]any{"type": "agent"})) {
		t.Error("the default still applied alongside a declared predicate")
	}
}

// A skill is a directory, so the files beside it are its assets and are held to
// no frontmatter rule. That used to depend on declaring skills:, which meant a
// store that declared nothing rejected a skill's own reference file for missing
// frontmatter.
func TestSkillDirsWithoutADeclaredGate(t *testing.T) {
	c := load(t, "name: s\n")

	docs := []doc.Doc{
		mk("caps/review/skill.md", map[string]any{"type": "skill"}),
		mk("engineering/logging.md", map[string]any{"type": "concept"}),
	}
	dirs, skillPaths := c.SkillDirs(docs)

	if !dirs["caps/review"] {
		t.Fatalf("the skill's directory was not recognized: %v", dirs)
	}
	if !skillPaths["caps/review/skill.md"] {
		t.Errorf("the skill document itself was not recorded: %v", skillPaths)
	}
	if !IsAsset("caps/review/severity.md", dirs, skillPaths) {
		t.Error("a file beside a skill is an asset, and was not treated as one")
	}
	if IsAsset("caps/review/skill.md", dirs, skillPaths) {
		t.Error("a skill is not an asset of its own directory")
	}
	if IsAsset("engineering/logging.md", dirs, skillPaths) {
		t.Error("an ordinary document was treated as a skill asset")
	}
}

func TestLoadListings(t *testing.T) {
	for _, tt := range []struct {
		yaml, want string
		fails      bool
	}{
		{"name: s\n", ListingsCommitted, false},
		{"listings: committed\n", ListingsCommitted, false},
		{"listings: build\n", ListingsBuild, false},
		{"listings: generated\n", "", true},
	} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, FileName), []byte(tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Load(root)
		if tt.fails {
			if err == nil {
				t.Errorf("%q: loaded, want an error", tt.yaml)
			}
			continue
		}
		if err != nil || c.Listings != tt.want || c.ListingsAtBuild() != (tt.want == ListingsBuild) {
			t.Errorf("%q: listings = %+v, %v, want %s", tt.yaml, c, err, tt.want)
		}
	}
	// No bundle.yaml at all is the default too.
	if c, err := Load(t.TempDir()); err != nil || c.Listings != ListingsCommitted {
		t.Errorf("no bundle.yaml: %+v, %v", c, err)
	}
}
