package bundle

import (
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/pack"
)

// buildWith runs a build against the fixture with an explicit package file and
// config, which is how the no-requirements cases are exercised.
func buildWith(t *testing.T, cfg *config.Config, rs pack.File) (*Bundle, error) {
	t.Helper()
	return Build(Options{Root: fixture, Config: cfg, Packages: rs})
}

// A store may decline to state any requirements. Everything then qualifies, and
// a package can name a draft — the author opted out of the guard knowingly.
func TestNoRequirementsAllowsDraft(t *testing.T) {
	cfg := &config.Config{Include: []string{"**/*.md"}, Exclude: []string{"meta/templates/**"}}
	rs := pack.File{"core": {Rules: []pack.Ref{{ID: "eng.draft"}}}}

	b, err := buildWith(t, cfg, rs)
	if err != nil {
		t.Fatalf("build with no requirements: %v", err)
	}
	if len(b.Packages) != 1 || len(b.Packages[0].Rules) != 1 {
		t.Fatalf("packages = %+v, want one package with one doc", b.Packages)
	}
}

// Visibility is not negotiable: even with no requirements declared, a restricted
// document cannot be shipped as a rule. Otherwise the bundle would carry a
// package pointing at a file it does not contain.
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

// Withholding a restricted document is Quartermaster's guarantee rather than a
// policy a store opts into, so it has to hold with no requires: at all. The
// scaffold used to declare visibility: { not: [restricted] } alongside status,
// which read as the line doing this work. It never was, and an author who
// deleted it would have believed they had changed something.
func TestNoRequirementsStillWithholdsRestrictedFromTheArtifact(t *testing.T) {
	cfg := &config.Config{Include: []string{"**/*.md"}, Exclude: []string{"meta/templates/**"}}

	b, err := buildWith(t, cfg, pack.File{})
	if err != nil {
		t.Fatalf("build with no requirements: %v", err)
	}

	const restricted = "engineering/secret.md"
	for _, f := range b.Files {
		if f.Path == restricted {
			t.Errorf("%s is carried in the artifact with no gate declared", restricted)
		}
	}
	for _, e := range b.Catalog {
		if e.Path == restricted {
			t.Errorf("%s appears in the catalog with no gate declared", restricted)
		}
	}

	// The fixture has to actually contain the document, or this proves nothing.
	if len(b.Files) == 0 {
		t.Fatal("the build carried no files at all; the fixture is not exercising this")
	}
}

// The compiled package must never reference a path the bundle does not carry;
// that invariant is what the restricted check protects.
func TestCompiledPackagesResolveToCarriedFiles(t *testing.T) {
	b := buildFixture(t)

	carried := make(map[string]bool, len(b.Files))
	for _, f := range b.Files {
		carried[f.Path] = true
	}

	for _, rs := range b.Packages {
		for _, d := range rs.Rules {
			if !carried[d.Path] {
				t.Fatalf("package %q references %s, which the bundle does not carry", rs.Name, d.Path)
			}
		}
	}
}
