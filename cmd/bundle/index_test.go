package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runIndex(t *testing.T, root string, check, strip bool) (string, error) {
	t.Helper()
	command := newIndexCmd()
	command.root, command.check, command.strip = root, check, strip
	var out bytes.Buffer
	err := command.run(&out)
	return out.String(), err
}

// Moving a store to listings: build: --check fails while listings are
// committed, --strip removes them, and the command then only previews.
func TestIndexAtBuild(t *testing.T) {
	root := copyStoreFixture(t)
	if _, err := runIndex(t, root, false, false); err != nil {
		t.Fatalf("committed store: %v", err)
	}
	if _, err := runIndex(t, root, false, true); err == nil {
		t.Error("--strip ran on a store with listings: committed")
	}

	cfgPath := filepath.Join(root, "bundle.yaml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, append(raw, []byte("\nlistings: build\n")...), 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := runIndex(t, root, true, false); err == nil || !strings.Contains(out, "--strip") {
		t.Errorf("--check with committed listings = %q, %v, want a failure naming --strip", out, err)
	}
	if out, err := runIndex(t, root, false, true); err != nil || !strings.Contains(out, "stripped") {
		t.Fatalf("--strip = %q, %v", out, err)
	}
	if out, err := runIndex(t, root, true, false); err != nil || !strings.Contains(out, "ok:") {
		t.Errorf("--check after strip = %q, %v", out, err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "index.md"))
	out, err := runIndex(t, root, false, false)
	if err != nil || !strings.Contains(out, "== engineering/index.md") {
		t.Errorf("preview = %q, %v, want the listings the bundle will carry", out, err)
	}
	if after, _ := os.ReadFile(filepath.Join(root, "index.md")); !bytes.Equal(before, after) {
		t.Error("the preview wrote to the store")
	}
}
