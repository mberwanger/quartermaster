package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
)

func TestBundleInitCreatesCurrentStore(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.dir = storeRoot
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
	if !strings.HasPrefix(string(rootIndex), "---\nokf_version: \"0.2\"\n---\n") {
		t.Fatalf("root index does not declare OKF 0.2:\n%s", rootIndex)
	}
	if !strings.Contains(output.String(), "preflight passes") {
		t.Fatalf("output does not report preflight success:\n%s", output.String())
	}
}

func TestBundleInitRejectsInvalidNameWithoutWriting(t *testing.T) {
	storeRoot := filepath.Join(t.TempDir(), "new-store")
	command := newBundleInitCmd()
	command.dir = storeRoot
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
	command.dir = storeRoot
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
