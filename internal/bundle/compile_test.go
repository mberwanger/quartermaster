package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mberwanger/quartermaster/internal/validate"
)

func TestCompileBuildsAValidStore(t *testing.T) {
	b, err := Compile(fixture, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if b == nil {
		t.Fatal("compile returned no bundle")
	}
}

// Checked is the reason validate and bundle init need nothing but the artifact.
// It counts every document held to the schema, so it is not Docs: the fixture
// store excludes its own machinery from distribution, and those documents are
// validated all the same.
func TestCompileRecordsWhatWasChecked(t *testing.T) {
	b, err := Compile(fixture, Source{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Meta.Checked == 0 {
		t.Fatal("a compiled store reports no checked docs")
	}
	if b.Meta.Checked < b.Meta.Docs {
		t.Fatalf("checked (%d) is below docs (%d): validation must cover at least what ships",
			b.Meta.Checked, b.Meta.Docs)
	}
}

// A bundle assembled from parts never ran validation, so it must not claim it
// did. Zero is the honest answer rather than a count borrowed from the catalog.
func TestBuildClaimsNothingWasChecked(t *testing.T) {
	if b := buildFixture(t); b.Meta.Checked != 0 {
		t.Fatalf("checked = %d, want 0: Build validates nothing", b.Meta.Checked)
	}
}

func TestCompileRejectsSchemaViolationBeforeBuilding(t *testing.T) {
	storeRoot := copyFixtureStore(t)
	writeStoreFile(t, storeRoot, "engineering/invalid.md", "---\nid: invalid.doc\n---\n# Invalid\n")

	b, err := Compile(storeRoot, Source{})
	if b != nil {
		t.Fatal("invalid store returned a compiled bundle")
	}

	var validationErr *validate.Error
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v (%T), want *validate.Error", err, err)
	}
	if validationErr.Result.OK() {
		t.Fatal("validation error carries no findings")
	}
}

// A package naming a document that does not exist is a compile failure, not a
// document being malformed. Reporting it as validation would send whoever hit it
// to the frontmatter of a file that is fine.
func TestCompileRejectsPackageCompileFailure(t *testing.T) {
	storeRoot := copyFixtureStore(t)
	writeStoreFile(t, storeRoot, "meta/packages.yaml", "broken:\n  rules:\n    - missing.doc\n")

	b, err := Compile(storeRoot, Source{})
	if b != nil {
		t.Fatal("uncompilable store returned a compiled bundle")
	}
	if err == nil {
		t.Fatal("uncompilable package returned no error")
	}

	var validationErr *validate.Error
	if errors.As(err, &validationErr) {
		t.Fatalf("package compile failure reported as document validation: %v", err)
	}
}

func copyFixtureStore(t *testing.T) string {
	t.Helper()

	storeRoot := t.TempDir()
	if err := os.CopyFS(storeRoot, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	return storeRoot
}

func writeStoreFile(t *testing.T, root, relativePath, body string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
