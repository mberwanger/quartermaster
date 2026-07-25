package bundle

import (
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/pack"
)

// buildWith runs a build against the fixture with an explicit ruleset file and
// config, which is how the no-requirements cases are exercised.
func buildWith(t *testing.T, cfg *config.Config, rs pack.File) (*Bundle, error) {
	t.Helper()
	return Build(Options{Root: fixture, Config: cfg, Packages: rs})
}

// A store may decline to state any requirements. Everything then qualifies, and
// a ruleset can name a draft — the author opted out of the guard knowingly.
func TestNoRequirementsAllowsDraft(t *testing.T) {
	cfg := &config.Config{Include: []string{"**/*.md"}, Exclude: []string{"meta/templates/**"}}
	rs := pack.File{"core": {Rules: []pack.Ref{{ID: "eng.draft"}}}}

	b, err := buildWith(t, cfg, rs)
	if err != nil {
		t.Fatalf("build with no requirements: %v", err)
	}
	if len(b.Packages) != 1 || len(b.Packages[0].Rules) != 1 {
		t.Fatalf("rulesets = %+v, want one ruleset with one doc", b.Packages)
	}
}

// Visibility is not negotiable: even with no requirements declared, a restricted
// document cannot be shipped as a rule. Otherwise the bundle would carry a
// ruleset pointing at a file it does not contain.
func TestNoRequirementsStillBlocksRestricted(t *testing.T) {
	cfg := &config.Config{Include: []string{"**/*.md"}, Exclude: []string{"meta/templates/**"}}
	rs := pack.File{"core": {Rules: []pack.Ref{{ID: "eng.secret"}}}}

	_, err := buildWith(t, cfg, rs)
	if err == nil {
		t.Fatal("expected a restricted reference to fail the build")
	}
	if !strings.Contains(err.Error(), "restricted") {
		t.Fatalf("error should name the reason, got: %v", err)
	}
}

// The compiled ruleset must never reference a path the bundle does not carry;
// that invariant is what the restricted check protects.
func TestCompiledRulesetsResolveToCarriedFiles(t *testing.T) {
	b := buildFixture(t)

	carried := make(map[string]bool, len(b.Files))
	for _, f := range b.Files {
		carried[f.Path] = true
	}

	for _, rs := range b.Packages {
		for _, d := range rs.Rules {
			if !carried[d.Path] {
				t.Fatalf("ruleset %q references %s, which the bundle does not carry", rs.Name, d.Path)
			}
		}
	}
}
