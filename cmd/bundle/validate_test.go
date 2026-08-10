package bundle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mberwanger/quartermaster/internal/validate"
)

// storeFixture is a small store known to validate and build cleanly, shared by
// every cmd-level test that needs a real store rather than a scaffolded one.
const storeFixture = "../../internal/bundle/testdata/store"

func copyStoreFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(storeFixture)); err != nil {
		t.Fatal(err)
	}
	return dir
}

// breakStore adds a document that fails the store's own schema, so a caller
// gets a *validate.Error to test rendering against.
func breakStore(t *testing.T, storeRoot string) {
	t.Helper()
	invalidPath := filepath.Join(storeRoot, "broken.md")
	if err := os.WriteFile(invalidPath, []byte("---\nid: broken.doc\n---\n# Broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// validate, build, and bundle init all compile a store through the same
// bundle.Compile call. Every one of them must return a *validate.Error bare,
// with no extra wrapping, so the CLI renders the same failure the same way
// everywhere a store fails to compile.
func TestValidateReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := copyStoreFixture(t)
	breakStore(t, storeRoot)

	command := newValidateCmd()
	command.root = storeRoot

	err := command.run(&bytes.Buffer{})
	assertBareValidationError(t, err)
}

func TestValidatePassesOnAValidStore(t *testing.T) {
	command := newValidateCmd()
	command.root = storeFixture

	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := copyStoreFixture(t)
	breakStore(t, storeRoot)

	command := newBuildCmd()
	command.root = storeRoot
	command.out = filepath.Join(t.TempDir(), "dist")

	err := command.run(&bytes.Buffer{})
	assertBareValidationError(t, err)
}

func TestBundleInitReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newInitCmd()
	command.root = storeRoot
	command.name = "new-store"
	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	breakStore(t, storeRoot)

	// force so the second run reaches the compile instead of the overwrite gate;
	// that gate is covered separately in bundle_init_test.go.
	command.force = true
	err := command.run(&bytes.Buffer{})
	assertBareValidationError(t, err)
}

// assertBareValidationError requires that err both is a *validate.Error and
// stringifies to exactly that error's own message, so a caller cannot have
// added a prefix or suffix on top of it.
func assertBareValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("an invalid store passed compilation")
	}

	var validationErr *validate.Error
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v (%T), want *validate.Error", err, err)
	}
	if err.Error() != validationErr.Error() {
		t.Fatalf("error was wrapped instead of returned bare:\ngot  = %q\nwant = %q", err.Error(), validationErr.Error())
	}
}
