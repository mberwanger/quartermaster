package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
)

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newStore builds a store with an ordinary document, a skill with an asset, and
// a hand-written preamble in one listing.
func newStore(t *testing.T) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()

	write(t, root, "bundle.yaml", "include: [\"**/*.md\"]\nskills:\n  type: [skill]\n")
	write(t, root, "engineering/knowledge/pipeline.md",
		"---\nid: eng.pipeline\ntitle: Pipeline\ndescription: How it flows.\ntype: concept\n---\n# Pipeline\n")
	write(t, root, "engineering/skills/gcp-expert/skill.md",
		"---\nid: skills.gcp\ntitle: GCP Expert\ndescription: Reach for this for GCP.\ntype: skill\n---\n# GCP\n")
	write(t, root, "engineering/skills/gcp-expert/references/iam.md", "# IAM\n\nNo frontmatter; part of the skill.\n")

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestNewRootDeclaresCurrentOKFVersion(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}

	got := read(t, root, "index.md")
	if !strings.HasPrefix(got, "---\nokf_version: \"0.2\"\n---\n") {
		t.Fatalf("root index does not declare OKF 0.2:\n%s", got)
	}
}

// A skill is one unit an agent loads whole, so it gets no listing of its parts —
// and such a listing would travel into every repository that materializes it.
func TestSkillDirectoryGetsNoListing(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "engineering/skills/gcp-expert/index.md")); !os.IsNotExist(err) {
		t.Fatal("a skill directory was given its own listing")
	}
	if _, err := os.Stat(filepath.Join(root, "engineering/skills/index.md")); err != nil {
		t.Fatalf("the directory holding skills should have a listing: %v", err)
	}
}

// The skill must still be reachable from the directory above it, or nesting it
// would make it invisible.
func TestSkillIsDiscoverableFromAbove(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}

	eng := read(t, root, "engineering/index.md")
	if !strings.Contains(eng, "skills/") {
		t.Fatalf("engineering listing does not mention skills:\n%s", eng)
	}
	skills := read(t, root, "engineering/skills/index.md")
	if !strings.Contains(skills, "gcp-expert/") {
		t.Fatalf("skills listing does not mention the skill:\n%s", skills)
	}
}

// An asset carries no frontmatter and is part of a skill, so it must never be
// listed as a document.
func TestAssetIsNotListed(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"index.md", "engineering/index.md", "engineering/skills/index.md"} {
		if strings.Contains(read(t, root, p), "iam.md") {
			t.Fatalf("%s lists a skill asset as a document", p)
		}
	}
}

// The generator owns a marked region, not the file. Prose above it explains why
// a directory exists, which no generator can write.
func TestHandWrittenPreambleSurvives(t *testing.T) {
	root, cfg := newStore(t)
	write(t, root, "engineering/index.md", "# engineering\n\nWhy this directory exists.\n")

	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	got := read(t, root, "engineering/index.md")
	if !strings.Contains(got, "Why this directory exists.") {
		t.Fatalf("hand-written preamble was lost:\n%s", got)
	}
	if !strings.Contains(got, "BEGIN "+Marker) {
		t.Fatalf("no managed region was added:\n%s", got)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	changed, err := Sync(root, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("second sync rewrote %v", changed)
	}
}

// Check mode reports staleness without touching the tree, which is what a pull
// request gate wants.
func TestCheckWritesNothing(t *testing.T) {
	root, cfg := newStore(t)
	changed, err := Sync(root, cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) == 0 {
		t.Fatal("expected a fresh store to report stale listings")
	}
	if _, err := os.Stat(filepath.Join(root, "engineering/index.md")); !os.IsNotExist(err) {
		t.Fatal("check mode wrote a listing")
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
