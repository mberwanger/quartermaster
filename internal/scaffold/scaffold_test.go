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
	written, err := Write(dir, "my-store")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("nothing written")
	}

	yaml, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yaml), "name: my-store") {
		t.Fatalf("name not substituted:\n%s", yaml)
	}
	if strings.Contains(string(yaml), "{{") {
		t.Fatalf("an unsubstituted token survived:\n%s", yaml)
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
