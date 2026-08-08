package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/index"
)

func TestBundleInitCreatesCurrentStore(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.root = storeRoot
	command.name = "new-store"

	var output bytes.Buffer
	if err := command.run(&output); err != nil {
		t.Fatal(err)
	}

	storeConfig, err := config.Load(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if storeConfig.Name != "new-store" {
		t.Fatalf("name = %q", storeConfig.Name)
	}

	rootIndex, err := os.ReadFile(filepath.Join(storeRoot, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantFrontmatter := "---\nokf_version: \"" + index.CurrentOKFVersion + "\"\n---\n"
	if !strings.HasPrefix(string(rootIndex), wantFrontmatter) {
		t.Fatalf("root index does not declare the current OKF version %q:\n%s", index.CurrentOKFVersion, rootIndex)
	}
	if !strings.Contains(output.String(), "preflight passes") {
		t.Fatalf("output does not report preflight success:\n%s", output.String())
	}
}

func TestBundleInitRejectsInvalidNameWithoutWriting(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.root = storeRoot
	command.name = "bad\nname"

	if err := command.run(&bytes.Buffer{}); err == nil {
		t.Fatal("multiline name was accepted")
	}
	if _, err := os.Stat(filepath.Join(storeRoot, "bundle.yaml")); !os.IsNotExist(err) {
		t.Fatalf("bundle.yaml was written before rejecting the name: %v", err)
	}
}

func TestBundleInitForceReplacesOnlyScaffoldOwnedFiles(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.root = storeRoot
	command.name = "new-store"
	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	customPath := filepath.Join(storeRoot, "custom.txt")
	if err := os.WriteFile(customPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	packagesPath := filepath.Join(storeRoot, "meta", "packages.yaml")
	if err := os.WriteFile(packagesPath, []byte("replace me"), 0o600); err != nil {
		t.Fatal(err)
	}

	command.force = true
	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	customBody, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(customBody) != "keep me" {
		t.Fatalf("custom file changed to %q", customBody)
	}

	packagesBody, err := os.ReadFile(packagesPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(packagesBody) == "replace me" {
		t.Fatal("scaffold-owned packages file was not replaced")
	}
}

// A store can lose bundle.yaml alone (a bad merge, a careless rm) without
// ceasing to be a store. Re-running init must still require --force, rather
// than silently replacing the schema, packages, and root index it finds.
func TestBundleInitRequiresForceWhenBundleYAMLIsMissingButOtherFilesRemain(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.root = storeRoot
	command.name = "new-store"
	if err := command.run(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(storeRoot, "bundle.yaml")); err != nil {
		t.Fatal(err)
	}
	packagesPath := filepath.Join(storeRoot, "meta", "packages.yaml")
	if err := os.WriteFile(packagesPath, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	command.force = false
	if err := command.run(&bytes.Buffer{}); err == nil {
		t.Fatal("init without --force succeeded despite an existing scaffold-owned file")
	}

	packagesBody, err := os.ReadFile(packagesPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(packagesBody) != "keep me" {
		t.Fatalf("packages.yaml was overwritten without --force: %q", packagesBody)
	}
}
