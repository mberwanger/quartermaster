package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/state"
)

func TestMigrateLeavesACurrentRepositoryAlone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, manifest.FileName), "bundles: []\n")
	writeFile(t, filepath.Join(dir, ".gitignore"), "/.qm/\n")

	done, err := Migrate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 0 {
		t.Fatalf("migrated something in a current repository: %v", done)
	}
}

func TestMigrateRenamesTheManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, manifest.LegacyFileName), "bundles: []\ntelemetry: true\n")

	done, err := Migrate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.FileName)); err != nil {
		t.Fatalf("manifest not renamed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.LegacyFileName)); err == nil {
		t.Fatal("legacy manifest still present")
	}
	if len(done) != 1 || !strings.Contains(done[0], "renamed") {
		t.Errorf("report: %v", done)
	}
}

// A repository that already carries both names is somebody's deliberate state,
// not ours to resolve by deleting one of them.
func TestMigrateKeepsBothManifestsWhenBothExist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, manifest.LegacyFileName), "legacy\n")
	writeFile(t, filepath.Join(dir, manifest.FileName), "current\n")

	if _, err := Migrate(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, manifest.FileName))
	if string(got) != "current\n" {
		t.Errorf("current manifest was overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.LegacyFileName)); err != nil {
		t.Error("legacy manifest was removed")
	}
}

// The state travels so the next sync prunes what the old one wrote. Knowledge
// paths are rewritten to where the tree now lives; harness paths are unchanged.
func TestMigrateCarriesStateAndRemovesTheLegacyDir(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, state.LegacyDir)
	writeFile(t, filepath.Join(legacy, "knowledge", "eng", "a.md"), "stale\n")
	writeFile(t, filepath.Join(legacy, "state.json"), `{"bundles":[],"targets":["claude"],"files":[".claude/rules/qm/eng.a.md",".quartermaster/knowledge/eng/a.md"]}`)

	if _, err := Migrate(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(legacy); err == nil {
		t.Fatal("legacy directory still present")
	}
	s, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".claude/rules/qm/eng.a.md", ".qm/knowledge/eng/a.md"}
	if strings.Join(s.Files, ",") != strings.Join(want, ",") {
		t.Errorf("carried files = %v, want %v", s.Files, want)
	}
}

func TestMigrateDoesNotOverwriteNewerState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, state.LegacyDir, "state.json"), `{"files":["old"]}`)
	writeFile(t, filepath.Join(dir, state.Dir, "state.json"), `{"files":["new"]}`)

	if _, err := Migrate(dir); err != nil {
		t.Fatal(err)
	}
	s, _ := state.Load(dir)
	if len(s.Files) != 1 || s.Files[0] != "new" {
		t.Errorf("newer state lost: %v", s.Files)
	}
}

func TestMigrateRetargetsGitignore(t *testing.T) {
	for name, tc := range map[string]struct{ before, after string }{
		"replaces the entry in place": {
			"*.log\n/.quartermaster/\n/.claude/rules/qm/\n",
			"*.log\n/.qm/\n/.claude/rules/qm/\n",
		},
		"drops it when the current entry is already there": {
			"/.quartermaster/\n/.qm/\n",
			"/.qm/\n",
		},
		"touches nothing otherwise": {
			"/.qm/\n",
			"/.qm/\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, ".gitignore"), tc.before)
			if _, err := Migrate(dir); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if string(got) != tc.after {
				t.Errorf("gitignore = %q, want %q", got, tc.after)
			}
		})
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
