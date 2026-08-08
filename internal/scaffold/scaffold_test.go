package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/validate"
)

func TestWriteSubstitutesName(t *testing.T) {
	dir := t.TempDir()
	written, err := Write(dir, `my: "store"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("nothing written")
	}

	storeConfig, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if storeConfig.Name != `my: "store"` {
		t.Fatalf("name = %q", storeConfig.Name)
	}

	yaml, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yaml), "{{") {
		t.Fatalf("an unsubstituted token survived:\n%s", yaml)
	}
}

func TestWriteRejectsInvalidNameBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "bad\nname"); err == nil {
		t.Fatal("multiline name was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("bundle.yaml was written before rejecting the name: %v", err)
	}
}

func TestWriteDeclaresCurrentOKFVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "my-store"); err != nil {
		t.Fatal(err)
	}

	rootIndex, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(rootIndex), "---\nokf_version: \"0.2\"\n---\n") {
		t.Fatalf("root index does not declare OKF 0.2:\n%s", rootIndex)
	}
}

// The whole promise of init is that what it writes is already sound. If the
// scaffold and the validator ever disagree, this fails.
func TestScaffoldValidates(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "my-store"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := validate.Run(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("scaffold did not validate: %+v", res.Findings)
	}
	if res.Checked == 0 {
		t.Fatal("expected the scaffold to include at least the template document")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Fatal("an empty directory reported as a store")
	}
	if _, err := Write(dir, "x"); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Fatal("a scaffolded directory not reported as a store")
	}
}
