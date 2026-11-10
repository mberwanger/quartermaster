// Package manifest reads the per-repository declaration a consumer commits: the
// bundles it draws from, the packages it applies, and the harnesses it targets.
//
// The manifest is the whole of what a repository declares. Everything
// materialized is reproducible from it, which is why the harness-specific output
// directories are gitignored and only the manifest is committed.
package manifest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/gate"
)

// FileName is the manifest a consumer commits at its repository root.
const FileName = ".qm.yaml"

// LegacyFileName is what an earlier release called the manifest. A repository
// still carrying it loads, and qm sync renames it.
const LegacyFileName = ".quartermaster.yaml"

// Manifest is a parsed .qm.yaml.
type Manifest struct {
	// Bundles are listed in precedence order. When two bundles produce a rule
	// with the same id, the later entry wins. Precedence is declared rather than
	// inferred, because the alternative is whichever text landed last in the
	// context window, which is not a policy.
	Bundles []Bundle `yaml:"bundles"`
	// Targets names the harnesses to render for.
	Targets []string `yaml:"targets"`
	// Budget bounds the resident set.
	Budget Budget `yaml:"budget"`
}

// Bundle is one source a repository draws from.
type Bundle struct {
	// Source is the provider URL: oci://, file://, git+https://, or https://.
	Source string `yaml:"source"`
	// Digest is the bundle's own content digest, the one meta.json carries. It
	// is optional. Absent, the repository follows whatever the source resolves
	// to, and the hooks keep it current. Present, a sync still resolves the
	// source but warns when the result is not this, so a repository that
	// wants to move deliberately sees the move. A hard pin is a registry digest
	// in the source itself, as oci://host/repo@sha256:..., which names one
	// artifact.
	Digest string `yaml:"digest"`
	// Use names the packages to apply from this bundle, in precedence order.
	//
	// One name rather than three lists. A package carries the rules, skills, and
	// agents that belong together, so a repository declares which team's set it
	// wants and the store decides what is in it. Adding a skill to a team is
	// then a change in the store rather than a pull request against every
	// repository that should have it.
	Use []string `yaml:"use"`
	// Knowledge scopes which of the bundle's documents are written to the
	// knowledge tree, matching on any frontmatter field. Empty means all of
	// them.
	//
	// This is a relevance filter, not a correctness one: a repository that cares
	// about billing has no use for another team's documents on disk, and a store
	// large enough to serve an organization is too large to land whole in every
	// repository. What may become a rule is still decided by the store, at build
	// time.
	Knowledge gate.Gate `yaml:"knowledge"`
}

// Budget bounds what may become resident.
type Budget struct {
	ResidentBytes int `yaml:"resident_bytes"`
}

// Load reads the manifest from a repository directory. The legacy name is read
// when the current one is absent, so a repository that has not synced since the
// rename still resolves; the rename itself is sync's job, not a reader's.
func Load(dir string) (*Manifest, error) {
	p := filepath.Join(dir, FileName)
	raw, err := os.ReadFile(p) //nolint:gosec // dir is a CLI flag, chosen by whoever runs the tool
	if errors.Is(err, fs.ErrNotExist) {
		if legacy, lerr := os.ReadFile(filepath.Join(dir, LegacyFileName)); lerr == nil { //nolint:gosec // same directory
			raw, err = legacy, nil
		}
	}
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &m, nil
}
