package bundle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteReadRoundTrip builds the fixture, writes it, reads it back, and
// checks the reconstructed bundle matches what a provider would need: same
// digest, catalog, packages, and store tree.
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
		t.Fatalf("packages: read %d, built %d", len(got.Packages), len(orig.Packages))
	}
	if len(got.Files) != len(orig.Files) {
		t.Fatalf("files: read %d, built %d", len(got.Files), len(orig.Files))
	}
}

// writeWithFormat writes the fixture and then rewrites the declared format, so a
// test can present an artifact this build should refuse.
func writeWithFormat(t *testing.T, format string) string {
	t.Helper()

	dir := t.TempDir()
	if err := Write(buildFixture(t), dir); err != nil {
		t.Fatalf("write: %v", err)
	}

	metaPath := filepath.Join(dir, MetaName)
	raw, err := os.ReadFile(metaPath) //nolint:gosec // a path this test just wrote
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if format == "" {
		delete(meta, "format")
	} else {
		meta["format"] = format
	}
	edited, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The failure this guards against is silent rather than loud. Before the format
// check, reading an artifact whose layout had moved found no packages.json and
// no store tree where it looked, returned no error, and produced a bundle
// carrying nothing. A sync would then write nothing and report success.
func TestReadRefusesAFormatItCannotRead(t *testing.T) {
	tests := map[string]struct {
		format   string
		wantHint string
	}{
		"older layout":  {"0.3", "rebuild it with qm bundle build"},
		"newer layout":  {"9.0", "upgrade qm"},
		"newer minor":   {"0.5", "upgrade qm"},
		"not a version": {"latest", "not a major.minor version"},
		"absent":        {"", "not built by qm bundle build"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Read(writeWithFormat(t, test.format))
			if err == nil {
				t.Fatal("an artifact this build cannot read was accepted, and would have resolved to nothing")
			}
			if !strings.Contains(err.Error(), test.wantHint) {
				t.Errorf("error %q does not say what to do about it (want %q)", err, test.wantHint)
			}
			if test.format != "" && !strings.Contains(err.Error(), test.format) {
				t.Errorf("error %q does not name the format it found (%s)", err, test.format)
			}
		})
	}
}

// The current format is readable, which is what keeps the check from being a
// test that only ever proves things fail.
func TestReadAcceptsTheCurrentFormat(t *testing.T) {
	b, err := Read(writeWithFormat(t, Version))
	if err != nil {
		t.Fatalf("the format this build writes was refused: %v", err)
	}
	if len(b.Packages) == 0 {
		t.Fatal("read a bundle with no packages; the fixture has some")
	}
}
