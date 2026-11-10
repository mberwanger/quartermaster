package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bundlepkg "github.com/mberwanger/quartermaster/internal/bundle"
)

// fileSource is a file:// URL for a path, which is how every scheme-independent
// part of pull is exercised without a registry.
func fileSource(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return "file://" + filepath.ToSlash(abs)
}

func pull(t *testing.T, source, out string, force bool) (string, error) {
	t.Helper()
	command := newPullCmd()
	command.out = out
	command.force = force

	var buf bytes.Buffer
	err := command.run(&buf, source)
	return buf.String(), err
}

// A source naming a store tree rather than a built bundle is compiled on the way
// through, so one command reads both. The digests agreeing is the proof it is
// the same artifact either way, not merely a similar one.
func TestPullReadsAStoreTreeAndABuiltBundleAlike(t *testing.T) {
	built := filepath.Join(t.TempDir(), "dist")
	if _, err := pull(t, fileSource(t, storeFixture), built, false); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "dist")
	got, err := pull(t, fileSource(t, built), out, false)
	if err != nil {
		t.Fatal(err)
	}

	fromStore := readMeta(t, built)
	fromBundle := readMeta(t, out)
	if fromStore.Digest != fromBundle.Digest {
		t.Fatalf("digest differs by source shape:\nstore  = %s\nbundle = %s", fromStore.Digest, fromBundle.Digest)
	}
	if !strings.Contains(got, fromBundle.Digest) {
		t.Errorf("the report does not name the digest it wrote:\n%s", got)
	}
}

func TestPullWritesTheWholeArtifact(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist")
	if _, err := pull(t, fileSource(t, storeFixture), out, false); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		bundlepkg.MetaName,
		bundlepkg.CatalogName,
		bundlepkg.PackagesName,
		filepath.Join(bundlepkg.StoreDir, "engineering", "errors.md"),
	} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("artifact is missing %s: %v", name, err)
		}
	}
}

// Writing an artifact clears the destination first. pull takes the source as its
// argument, which makes --out the incidental part and far likelier than a
// build's to be pointed at something real, so it is checked before anything is
// removed.
func TestPullRefusesToClearADirectoryThatIsNotABundle(t *testing.T) {
	out := t.TempDir()
	keep := filepath.Join(out, "notes.txt")
	if err := os.WriteFile(keep, []byte("important"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := pull(t, fileSource(t, storeFixture), out, false)
	if err == nil {
		t.Fatal("pull cleared a directory holding something else")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal does not say how to proceed: %v", err)
	}

	body, readErr := os.ReadFile(keep)
	if readErr != nil {
		t.Fatalf("the file was removed despite the refusal: %v", readErr)
	}
	if string(body) != "important" {
		t.Errorf("the file was rewritten: %q", body)
	}
}

func TestPullForceClearsIt(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("important"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := pull(t, fileSource(t, storeFixture), out, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); err == nil {
		t.Error("--force left the previous contents in place")
	}
	if _, err := os.Stat(filepath.Join(out, bundlepkg.MetaName)); err != nil {
		t.Errorf("--force did not write the artifact: %v", err)
	}
}

// Pulling a newer bundle over an older one is the ordinary case and must not
// need --force. An empty directory is likewise free to write.
func TestPullOverwritesAnEarlierPull(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist")
	if _, err := pull(t, fileSource(t, storeFixture), out, false); err != nil {
		t.Fatal(err)
	}
	if _, err := pull(t, fileSource(t, storeFixture), out, false); err != nil {
		t.Fatalf("re-pulling over a bundle required --force: %v", err)
	}
}

func TestPullAcceptsAnEmptyDirectory(t *testing.T) {
	out := t.TempDir()
	if _, err := pull(t, fileSource(t, storeFixture), out, false); err != nil {
		t.Fatalf("an empty directory required --force: %v", err)
	}
}

func TestPullRejectsAnUnknownScheme(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist")
	if _, err := pull(t, "ftp://example.com/bundle", out, false); err == nil {
		t.Fatal("an unknown scheme was accepted")
	}
}

func readMeta(t *testing.T, dir string) bundlepkg.Meta {
	t.Helper()
	b, err := bundlepkg.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	return b.Meta
}
