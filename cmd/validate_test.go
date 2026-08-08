package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mberwanger/quartermaster/internal/preflight"
)

// storeFixture is a small store known to validate and build cleanly, shared by
// every cmd-level test that needs a real store rather than a scaffolded one.
const storeFixture = "../internal/bundle/testdata/store"

func copyStoreFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(storeFixture)); err != nil {
		t.Fatal(err)
	}
	return dir
}

// breakStore adds a document that fails the store's own schema, so a caller
// gets a *preflight.ValidationError to test rendering against.
func breakStore(t *testing.T, storeRoot string) {
	t.Helper()
	invalidPath := filepath.Join(storeRoot, "broken.md")
	if err := os.WriteFile(invalidPath, []byte("---\nid: broken.doc\n---\n# Broken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// validate, build, and bundle init all compile a store through the same
// internal/preflight call. Every one of them must return a *ValidationError
// bare, with no extra wrapping, so the CLI renders the same failure the same
// way everywhere a store fails to compile.
func TestValidateReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := copyStoreFixture(t)
	breakStore(t, storeRoot)

	command := newValidateCmd()
	command.root = storeRoot
	command.cmd.SetOut(&bytes.Buffer{})
	command.cmd.SetErr(&bytes.Buffer{})

	err := command.cmd.RunE(command.cmd, nil)
	assertBareValidationError(t, err)
}

func TestValidatePassesOnAValidStore(t *testing.T) {
	command := newValidateCmd()
	command.root = storeFixture
	command.cmd.SetOut(&bytes.Buffer{})
	command.cmd.SetErr(&bytes.Buffer{})

	if err := command.cmd.RunE(command.cmd, nil); err != nil {
		t.Fatal(err)
	}
}

func TestBuildReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := copyStoreFixture(t)
	breakStore(t, storeRoot)

	command := newBuildCmd()
	command.root = storeRoot
	command.out = filepath.Join(t.TempDir(), "dist")
	command.cmd.SetOut(&bytes.Buffer{})
	command.cmd.SetErr(&bytes.Buffer{})

	err := command.cmd.RunE(command.cmd, nil)
	assertBareValidationError(t, err)
}

func TestBundleInitReturnsValidationErrorUnwrapped(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.root = storeRoot
	command.name = "new-store"
	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	breakStore(t, storeRoot)

	// force so the second run reaches preflight instead of the overwrite gate;
	// that gate is covered separately in bundle_init_test.go.
	command.force = true
	err := command.run(&bytes.Buffer{})
	assertBareValidationError(t, err)
}

// assertBareValidationError requires that err both is a *preflight.ValidationError
// and stringifies to exactly that error's own message, so a caller cannot have
// added a prefix or suffix on top of it.
func assertBareValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("an invalid store passed compilation")
	}

	var validationErr *preflight.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v (%T), want *preflight.ValidationError", err, err)
	}
	if err.Error() != validationErr.Error() {
		t.Fatalf("error was wrapped instead of returned bare:\ngot  = %q\nwant = %q", err.Error(), validationErr.Error())
	}
}
