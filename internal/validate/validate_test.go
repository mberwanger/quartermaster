package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func cfg() *config.Config {
	return &config.Config{Include: []string{"**/*.md"}}
}

func TestRunAcceptsRootIndexWithNoFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "# Store\n")

	res, err := Run(root, cfg())
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("findings = %+v", res.Findings)
	}
}

func TestRunAcceptsValidRootOKFVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "---\nokf_version: \"0.2\"\n---\n\n# Store\n")

	res, err := Run(root, cfg())
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("findings = %+v", res.Findings)
	}
}

func TestRunRejectsMalformedRootOKFVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "---\nokf_version: \"current\"\n---\n\n# Store\n")

	res, err := Run(root, cfg())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Fatal("a malformed okf_version was accepted")
	}
}

func TestRunRejectsRootFrontmatterMissingOKFVersion(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "---\nname: store\n---\n\n# Store\n")

	res, err := Run(root, cfg())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Fatal("root frontmatter with no okf_version was accepted")
	}
}

func TestRunRejectsNonRootIndexWithFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "# Store\n")
	writeFile(t, root, "engineering/index.md", "---\nokf_version: \"0.2\"\n---\n\n# Engineering\n")

	res, err := Run(root, cfg())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Fatal("a non-root index with frontmatter was accepted")
	}
}

// A store that writes its listings at build fails validation on a committed one.
func TestRunRejectsCommittedListingAtBuild(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.md", "# Store\n\n<!-- BEGIN GENERATED -->\n- [a](a.md)\n<!-- END GENERATED -->\n")
	writeFile(t, root, "a.md", "---\nid: a\ntitle: A\ndescription: A.\ntype: concept\n---\n# A\n")

	c := cfg()
	res, err := Run(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("committed listings flagged in a committed store: %+v", res.Findings)
	}

	c.Listings = config.ListingsBuild
	res, err = Run(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || res.Findings[0].Path != "index.md" || !strings.Contains(res.Findings[0].Message, "--strip") {
		t.Errorf("findings = %+v, want the committed listing named with the fix", res.Findings)
	}
}
