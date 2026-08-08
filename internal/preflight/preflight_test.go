package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const fixture = "../bundle/testdata/store"

func TestRunCompilesValidStore(t *testing.T) {
	result, err := Run(Options{Root: fixture})
	if err != nil {
		t.Fatal(err)
	}
	if result.Bundle == nil {
		t.Fatal("preflight returned no bundle")
	}
	if !result.Validation.OK() {
		t.Fatalf("validation findings = %+v", result.Validation.Findings)
	}
}

func TestRunRejectsSchemaViolationBeforeBuild(t *testing.T) {
	storeRoot := copyFixture(t)
	writeFile(t, storeRoot, "engineering/invalid.md", "---\nid: invalid.doc\n---\n# Invalid\n")

	result, err := Run(Options{Root: storeRoot})
	if result != nil {
		t.Fatal("invalid store returned a compiled result")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validationErr.Result.OK() {
		t.Fatal("validation error carries no findings")
	}
}

func TestRunRejectsPackageCompileFailure(t *testing.T) {
	storeRoot := copyFixture(t)
	writeFile(t, storeRoot, "meta/packages.yaml", "broken:\n  rules:\n    - missing.doc\n")

	result, err := Run(Options{Root: storeRoot})
	if result != nil {
		t.Fatal("uncompilable store returned a compiled result")
	}
	if err == nil {
		t.Fatal("uncompilable package returned no error")
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		t.Fatalf("package compile failure reported as document validation: %v", err)
	}
}

func copyFixture(t *testing.T) string {
	t.Helper()

	storeRoot := t.TempDir()
	if err := os.CopyFS(storeRoot, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	return storeRoot
}

func writeFile(t *testing.T, root, relativePath, body string) {
	t.Helper()

	fullPath := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
