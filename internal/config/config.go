// Package config reads a store repository's bundle.yaml: the declaration that
// turns a tree of markdown into a buildable bundle.
//
// A store declares itself with bundle.yaml at its root. It names the schema its
// documents are validated against, the rulesets file, the include and exclude
// patterns, and the gate. None of these are required by Quartermaster: a tree
// with no bundle.yaml still builds, it just carries every markdown file, gates
// nothing, and injects nothing.
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/gate"
)

// FileName is the declaration a store repository carries at its root.
const FileName = "bundle.yaml"

// Config is a parsed bundle.yaml. Paths in it (Schema, Rulesets) are relative
// to the store root.
type Config struct {
	// Name is the store's name, recorded in the bundle for readability.
	Name string `yaml:"name"`
	// Schema points at the frontmatter JSON Schema, relative to the store root.
	Schema string `yaml:"schema"`
	// Rulesets points at the rulesets file, relative to the store root.
	Rulesets string `yaml:"rulesets"`
	// Include is the set of glob patterns a file must match to be a document.
	// It defaults to every markdown file.
	Include []string `yaml:"include"`
	// Exclude drops files that would otherwise be included. Store machinery
	// (conventions, taxonomy, templates) lives here.
	Exclude []string `yaml:"exclude"`
	// Controls names files carried into the bundle but partitioned away from
	// everything an agent grounds on, for the review and audit jobs to read.
	Controls []string `yaml:"controls"`
	// Requires is what a document must be to become a rule.
	Requires gate.Gate `yaml:"requires"`
	// Skills identifies which documents are skills. A skill is a directory
	// rather than a file: everything beside the skill document travels with it
	// as an asset and needs no frontmatter of its own, because a skill is a unit
	// an agent loads whole rather than a topic it reads.
	//
	// It is a predicate rather than a path glob, so a store says what a skill is
	// in its own vocabulary, the same way it says what may become a rule.
	Skills gate.Gate `yaml:"skills"`
}

// SkillDirs returns the directory of every skill document, which is what makes
// the files beside it assets. Paths of the skill documents themselves are
// returned too, so a caller can tell a skill from its own asset.
func (c *Config) SkillDirs(docs []doc.Doc) (dirs, skillPaths map[string]bool) {
	dirs, skillPaths = map[string]bool{}, map[string]bool{}
	if c.Skills.Empty() {
		return dirs, skillPaths
	}
	for _, d := range docs {
		if d.Frontmatter == nil {
			continue
		}
		if ok, _ := c.Skills.Allows(d.Frontmatter); ok {
			dirs[path.Dir(d.Path)] = true
			skillPaths[d.Path] = true
		}
	}
	return dirs, skillPaths
}

// IsAsset reports whether a path belongs to a skill rather than standing on its
// own. A skill document is not an asset of its own directory.
func IsAsset(rel string, dirs, skillPaths map[string]bool) bool {
	if len(dirs) == 0 || skillPaths[rel] {
		return false
	}
	for dir := range dirs {
		if strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

// defaultInclude is the include set for a store that declares none.
var defaultInclude = []string{"**/*.md"}

// Load reads bundle.yaml from the store root. A store with no bundle.yaml is not
// an error: Load returns a permissive default that includes every markdown file
// and gates nothing.
func Load(root string) (*Config, error) {
	p := filepath.Join(root, FileName)
	raw, err := os.ReadFile(p) //nolint:gosec // root is a CLI flag, chosen by whoever runs the tool
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Include: defaultInclude}, nil
		}
		return nil, err
	}

	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if len(c.Include) == 0 {
		c.Include = defaultInclude
	}
	return &c, nil
}

// IsDocument reports whether a store-relative path is a document: it matches an
// include pattern and no exclude pattern. A control file is still a document —
// it is validated and linked like any other — so this does not consider
// Controls.
func (c *Config) IsDocument(rel string) bool {
	return matchAny(c.Include, rel) && !matchAny(c.Exclude, rel)
}

// IsControl reports whether a store-relative path is a control fixture.
func (c *Config) IsControl(rel string) bool {
	return matchAny(c.Controls, rel)
}

func matchAny(patterns []string, rel string) bool {
	rel = path.Clean(filepath.ToSlash(rel))
	for _, pat := range patterns {
		if ok, err := doublestar.Match(pat, rel); err == nil && ok {
			return true
		}
	}
	return false
}
