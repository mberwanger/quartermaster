package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
)

// orphan makes engineering/knowledge/ hold no documents any more, leaving the
// listing Sync wrote for it behind. This is what moving or deleting a directory's
// last document does.
func orphan(t *testing.T, root string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, "engineering", "knowledge", "pipeline.md")); err != nil {
		t.Fatal(err)
	}
}

func TestOrphansReportsAListingNothingRegenerates(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	orphan(t, root)

	got, err := Orphans(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("orphans = %+v, want exactly engineering/knowledge/index.md", got)
	}
	if got[0].Path != "engineering/knowledge/index.md" {
		t.Errorf("path = %q", got[0].Path)
	}
	if got[0].Entries != 1 {
		t.Errorf("entries = %d, want 1: the region still lists the document that left", got[0].Entries)
	}
}

// The report exists because nothing else says it. Sync visits only what Build
// asks for, so an emptied directory's listing is neither rewritten nor removed,
// and it goes on advertising documents that are gone.
func TestSyncLeavesAnOrphanAlone(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	orphan(t, root)

	before := read(t, root, "engineering/knowledge/index.md")
	changed, err := Sync(root, cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range changed {
		if p == "engineering/knowledge/index.md" {
			t.Fatal("sync rewrote an orphaned listing")
		}
	}
	if after := read(t, root, "engineering/knowledge/index.md"); after != before {
		t.Fatalf("an orphaned listing changed on disk:\nbefore = %q\nafter  = %q", before, after)
	}
}

// A store that compiles is not a store that tells the truth about itself, so a
// report is all this is. Nothing about the artifact changes.
func TestOrphansNeverWrites(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	orphan(t, root)

	before := read(t, root, "engineering/knowledge/index.md")
	if _, err := Orphans(root, cfg); err != nil {
		t.Fatal(err)
	}
	if after := read(t, root, "engineering/knowledge/index.md"); after != before {
		t.Fatal("Orphans wrote to the tree")
	}
}

// The three guards. A false positive here ends with a person deleting prose no
// generator can put back, so each is pinned separately.

// A file with no generated region was never this tool's output. Reporting it
// would invite deleting a hand-written listing, and ApplyBlock would append a
// region to it rather than replace one.
func TestOrphansIgnoresAFileWithNoGeneratedRegion(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	write(t, root, "handbook/index.md", "# Why this exists\n\n* [a link](/somewhere.md) - prose, not a listing\n")

	assertNoOrphan(t, root, cfg, "handbook/index.md")
}

// An empty region makes no claim about what a directory holds, so it is not
// stale. A store commonly keeps one where only machinery lives: the bundle
// fixture's own meta/index.md is exactly this.
func TestOrphansIgnoresAnEmptyRegion(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	write(t, root, "meta/index.md", "<!-- BEGIN GENERATED -->\n<!-- END GENERATED -->\n")

	assertNoOrphan(t, root, cfg, "meta/index.md")
}

// A listing stops at a skill rather than descending into it, so every file
// inside one belongs to the skill. One that happens to carry a region, from
// before the directory became a skill, is still not this package's to judge.
func TestOrphansIgnoresAFileInsideASkill(t *testing.T) {
	root, cfg := newStore(t)
	if _, err := Sync(root, cfg, true); err != nil {
		t.Fatal(err)
	}
	write(t, root, "engineering/skills/gcp-expert/index.md",
		"<!-- BEGIN GENERATED -->\n# Concepts\n\n* [Left over](/engineering/skills/gcp-expert/gone.md) - from before\n\n<!-- END GENERATED -->\n")

	assertNoOrphan(t, root, cfg, "engineering/skills/gcp-expert/index.md")
}

func assertNoOrphan(t *testing.T, root string, cfg *config.Config, path string) {
	t.Helper()
	got, err := Orphans(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range got {
		if o.Path == path {
			t.Fatalf("%s was reported as an orphan", path)
		}
	}
}
