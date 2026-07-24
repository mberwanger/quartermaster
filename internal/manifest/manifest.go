// Package manifest reads the per-repository declaration a consumer commits: the
// bundles it draws from, the rulesets it applies, and the harnesses it targets.
//
// The manifest is the whole of what a repository declares. Everything
// materialized is reproducible from it, which is why the harness-specific output
// directories are gitignored and only the manifest is committed.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/mberwanger/quartermaster/internal/gate"
)

// FileName is the manifest a consumer commits at its repository root.
const FileName = ".quartermaster.yaml"

// Manifest is a parsed .quartermaster.yaml.
type Manifest struct {
	// Bundles are listed in precedence order. When two bundles produce a rule
	// with the same id, the later entry wins. Precedence is declared rather than
	// inferred, because the alternative is whichever text landed last in the
	// context window, which is not a policy.
	Bundles []Bundle `yaml:"bundles"`
	// Targets names the harnesses to render for.
	Targets []string `yaml:"targets"`
	// Telemetry opts the repository in to usage logging.
	Telemetry bool `yaml:"telemetry"`
	// Budget bounds the resident set.
	Budget Budget `yaml:"budget"`
}

// Bundle is one source a repository draws from.
type Bundle struct {
	// Source is the provider URL: oci://, file://, git+https://, or https://.
	Source string `yaml:"source"`
	// Digest pins the content. It is authoritative; the tag in Source is for
	// readability. A file:// source tree has no stable digest and may leave it
	// empty.
	Digest string `yaml:"digest"`
	// Rulesets names the selections to apply from this bundle.
	Rulesets []string `yaml:"rulesets"`
	// Skills names the skills to materialize, by document id. A skill is loaded
	// on demand rather than pushed at the agent, so it costs no context until it
	// is chosen and is never counted against the resident budget.
	Skills []string `yaml:"skills"`
	// Agents names the agent definitions to materialize, by document id. An
	// agent is a capability rather than text, carrying its own tool allowlist
	// and permissions, so a repository opts into each one by name.
	Agents []string `yaml:"agents"`
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

// Load reads the manifest from a repository directory.
func Load(dir string) (*Manifest, error) {
	p := filepath.Join(dir, FileName)
	raw, err := os.ReadFile(p) //nolint:gosec // dir is a CLI flag, chosen by whoever runs the tool
	if err != nil {
		return nil, err
	}

	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &m, nil
}
