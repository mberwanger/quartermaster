package cache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPopulateFillsOnceThenHits(t *testing.T) {
	t.Setenv("QM_CACHE_DIR", t.TempDir())

	var fills int
	fill := func(dir string) error {
		fills++
		return os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{}"), 0o600)
	}

	dir, err := Populate("oci", "sha256:abc", fill)
	if err != nil {
		t.Fatal(err)
	}
	if fills != 1 {
		t.Fatalf("fills = %d, want 1", fills)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Fatalf("entry not populated: %v", err)
	}

	// A second call for the same key is a hit: fill must not run again.
	dir2, err := Populate("oci", "sha256:abc", fill)
	if err != nil {
		t.Fatal(err)
	}
	if fills != 1 {
		t.Fatalf("fills = %d after second call, want 1", fills)
	}
	if dir2 != dir {
		t.Fatalf("dir changed: %s vs %s", dir2, dir)
	}
}

// A failed fetch must leave nothing behind, or the next run would trust a
// half-populated entry.
func TestPopulateFailureLeavesNothing(t *testing.T) {
	root := t.TempDir()
	t.Setenv("QM_CACHE_DIR", root)

	boom := errors.New("boom")
	if _, err := Populate("oci", "sha256:bad", func(string) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}

	if _, err := os.Stat(filepath.Join(root, "oci", "sha256-bad")); !os.IsNotExist(err) {
		t.Fatal("failed entry was committed to the cache")
	}
}

func TestSeparateKinds(t *testing.T) {
	t.Setenv("QM_CACHE_DIR", t.TempDir())

	a, err := Populate("oci", "k", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	b, err := Populate("git", "k", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("different kinds shared a directory")
	}
}
