// Package config reads a store repository's bundle.yaml: the declaration that
// turns a tree of markdown into a buildable bundle.
//
// A store declares itself with bundle.yaml at its root. It names the schema its
// documents are validated against, the packages file, the include and exclude
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

// Config is a parsed bundle.yaml. Paths in it (Schema, Packages) are relative
// to the store root.
type Config struct {
	// Name is the store's name, recorded in the bundle for readability.
	Name string `yaml:"name"`
	// Schema points at the frontmatter JSON Schema, relative to the store root.
	Schema string `yaml:"schema"`
	// Packages points at the packages file, relative to the store root.
	Packages string `yaml:"packages"`
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
	// Skills and Agents override how a capability is recognized. Both default to
	// the document's type, so a store declares neither unless its vocabulary
	// differs from the obvious one.
	//
	// They are predicates rather than path globs, so a store that does need to
	// override says what a skill is in its own vocabulary, the same way it says
	// what may become a rule.
	Skills gate.Gate `yaml:"skills"`
	Agents gate.Gate `yaml:"agents"`
}

// Capability types a document declares to be recognized as one without the store
// overriding anything. They are Quartermaster's, not the standard's: OKF leaves
// type open, and these are the two values the tool acts on.
const (
	TypeSkill = "skill"
	TypeAgent = "agent"
)

// IsSkill reports whether a document is a skill, and IsAgent whether it is an
// agent.
//
// The type decides it unless the store says otherwise. Type is the one field the
// standard requires, so it is always there; it is what a person reads to know
// what a document is, and what the generated listings group by. A block cannot
// serve the same purpose, because a store may leave it out entirely once nothing
// inside it is mandatory.
//
// Defaulting matters more than the symmetry. An absent gate used to be an empty
// gate, and an empty gate admits everything, so a store that simply never
// declared skills: could have any document selected as a skill, while the files
// beside a real skill were held to the frontmatter rule instead of travelling as
// its assets. The default was permissive exactly where it should have been
// strict and strict exactly where it should have been permissive.
func (c *Config) IsSkill(d doc.Doc) bool { return recognized(c.Skills, d, TypeSkill) }

// IsAgent reports whether a document is an agent. See IsSkill.
func (c *Config) IsAgent(d doc.Doc) bool { return recognized(c.Agents, d, TypeAgent) }

func recognized(override gate.Gate, d doc.Doc, defaultType string) bool {
	if !override.Empty() {
		ok, _ := override.Allows(d.Frontmatter)
		return ok
	}
	return d.Str("type") == defaultType
}

// SkillDirs returns the directory of every skill document, which is what makes
// the files beside it assets. Paths of the skill documents themselves are
// returned too, so a caller can tell a skill from its own asset.
func (c *Config) SkillDirs(docs []doc.Doc) (dirs, skillPaths map[string]bool) {
	dirs, skillPaths = map[string]bool{}, map[string]bool{}
	for _, d := range docs {
		if d.Frontmatter == nil {
			continue
		}
		if c.IsSkill(d) {
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

// IsDocument reports whether a store-relative path is a document of this store.
//
// Exclusion is deliberately not considered here. A document the store keeps to
// itself is still a document: it is validated, it is listed, and its frontmatter
// is held to the same schema. Conflating "not distributed" with "not a document"
// would quietly drop the store's own operating documents out of every check that
// keeps them honest.
func (c *Config) IsDocument(rel string) bool {
	return matchAny(c.Include, rel)
}

// IsDistributed reports whether a document travels in the bundle. Excluded
// documents stay in the store repository, where they are still checked, and
// never reach a consuming repository's disk.
func (c *Config) IsDistributed(rel string) bool {
	return c.IsDocument(rel) && !matchAny(c.Exclude, rel)
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
