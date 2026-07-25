package bundle

import (
	"path/filepath"
	"testing"
)

// TestWriteReadRoundTrip builds the fixture, writes it, reads it back, and
// checks the reconstructed bundle matches what a provider would need: same
// digest, catalog, rulesets, and store tree.
func TestWriteReadRoundTrip(t *testing.T) {
	orig := buildFixture(t)

	dir := t.TempDir()
	if err := Write(orig, dir); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Read(filepath.Join(dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.Meta.Digest != orig.Meta.Digest {
		t.Fatalf("digest: read %s, built %s", got.Meta.Digest, orig.Meta.Digest)
	}
	if got.Meta.Format != orig.Meta.Format {
		t.Fatalf("format: read %s, built %s", got.Meta.Format, orig.Meta.Format)
	}
	if len(got.Catalog) != len(orig.Catalog) {
		t.Fatalf("catalog: read %d, built %d", len(got.Catalog), len(orig.Catalog))
	}
	if len(got.Packages) != len(orig.Packages) {
		t.Fatalf("rulesets: read %d, built %d", len(got.Packages), len(orig.Packages))
	}
	if len(got.Files) != len(orig.Files) {
		t.Fatalf("files: read %d, built %d", len(got.Files), len(orig.Files))
	}
}
