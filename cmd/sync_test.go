package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The example store stands in for a published bundle: it is what a consuming
// repository would resolve, minus the registry.
const exampleStore = "../examples/store"

// A repository set up by an earlier release carries .quartermaster.yaml and a
// .quartermaster/ tree. One sync brings it to the current names, with the
// knowledge regenerated in the new place and the old place gone.
func TestSyncMigratesALegacyRepository(t *testing.T) {
	store, err := filepath.Abs(exampleStore)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(".quartermaster.yaml", "bundles:\n  - source: file://"+store+"\n    use: [engineering]\ntargets:\n  - claude\ntelemetry: true\n")
	write(".quartermaster/knowledge/stale.md", "left by the last release\n")
	write(".quartermaster/state.json", `{"bundles":[],"targets":["claude"],"files":[".quartermaster/knowledge/stale.md"]}`)
	write(".gitignore", "/.quartermaster/\n/.claude/rules/qm/\n")

	var out bytes.Buffer
	if err := syncRepo(dir, &out, false); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}

	for _, want := range []string{"migrated: renamed", "migrated: removed", "migrated: .gitignore", "session hook"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".qm.yaml")); err != nil {
		t.Error("manifest not renamed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".quartermaster")); err == nil {
		t.Error("legacy directory survived the sync")
	}
	if _, err := os.Stat(filepath.Join(dir, ".qm", "knowledge", "index.md")); err != nil {
		t.Error("knowledge was not regenerated under .qm/")
	}
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(ignore), "/.qm/") || strings.Contains(string(ignore), "/.quartermaster/") {
		t.Errorf("gitignore not retargeted:\n%s", ignore)
	}
	settings, _ := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if !strings.Contains(string(settings), "qm sync") {
		t.Errorf("session hook not installed:\n%s", settings)
	}

	// A second sync is an ordinary one.
	out.Reset()
	if err := syncRepo(dir, &out, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "migrated") {
		t.Errorf("second sync migrated again:\n%s", out.String())
	}
}
