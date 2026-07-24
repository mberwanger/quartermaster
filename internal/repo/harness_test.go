package repo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSessionHookCreatesSettings(t *testing.T) {
	dir := t.TempDir()

	wrote, err := InstallSessionHook(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("reported no write on a repository with no settings")
	}
	if !HasSessionHook(dir) {
		t.Fatal("hook not found after install")
	}

	body := readSettings(t, dir)
	// The literal command, unescaped. Go escapes > and & by default, and a
	// committed file full of > is unreadable to whoever reviews it.
	if !strings.Contains(body, sessionHookCommand) {
		t.Errorf("command is not written literally:\n%s", body)
	}
	for _, escaped := range []string{"u003e", "u0026"} {
		if strings.Contains(body, escaped) {
			t.Errorf("shell operators were html-escaped:\n%s", body)
		}
	}
	if i, j := strings.Index(body, `"type"`), strings.Index(body, `"command"`); i > j {
		t.Errorf("keys are not in the documented order:\n%s", body)
	}

	// It has to be valid JSON for the harness to read any of the file at all.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("wrote invalid json: %v\n%s", err, body)
	}
}

func TestInstallSessionHookIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	if _, err := InstallSessionHook(dir); err != nil {
		t.Fatal(err)
	}
	first := readSettings(t, dir)

	wrote, err := InstallSessionHook(dir)
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Error("second run wrote again")
	}
	if got := readSettings(t, dir); got != first {
		t.Errorf("second run changed the file:\n%s", got)
	}
	if n := strings.Count(first, sessionHookMark); n != 1 {
		t.Errorf("hook appears %d times, want 1", n)
	}
}

// A settings file is committed and hand-edited. Adding a hook must not reorder
// what somebody else wrote, or a one-line change arrives as a whole-file diff.
func TestInstallSessionHookPreservesTheFile(t *testing.T) {
	dir := t.TempDir()
	existing := `{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "model": "opus",
  "permissions": {
    "allow": [
      "Bash(go test:*)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Read",
        "hooks": [
          {
            "type": "command",
            "command": "somebody-elses-tool"
          }
        ]
      }
    ]
  }
}`
	writeSettings(t, dir, existing)

	if _, err := InstallSessionHook(dir); err != nil {
		t.Fatal(err)
	}

	body := readSettings(t, dir)

	// Key order survives.
	for _, pair := range [][2]string{
		{"$schema", "model"},
		{"model", "permissions"},
		{"permissions", "hooks"},
	} {
		if strings.Index(body, pair[0]) > strings.Index(body, pair[1]) {
			t.Errorf("%q now sorts after %q:\n%s", pair[0], pair[1], body)
		}
	}

	// Somebody else's hook on another event is untouched.
	if !strings.Contains(body, "somebody-elses-tool") || !strings.Contains(body, `"PostToolUse"`) {
		t.Errorf("lost an existing hook:\n%s", body)
	}
	if !strings.Contains(body, `"Bash(go test:*)"`) {
		t.Errorf("lost existing permissions:\n%s", body)
	}
	if !HasSessionHook(dir) {
		t.Errorf("did not add the hook:\n%s", body)
	}
}

// Another tool's hook on the same event is a sibling, not something to replace.
func TestInstallSessionHookKeepsOtherHooksOnTheSameEvent(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, `{
  "hooks": {
    "SessionEnd": [
      { "hooks": [ { "type": "command", "command": "somebody-elses-tool" } ] }
    ]
  }
}`)

	if _, err := InstallSessionHook(dir); err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Hooks struct {
			SessionEnd []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"SessionEnd"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readSettings(t, dir)), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Hooks.SessionEnd) != 2 {
		t.Fatalf("SessionEnd has %d entries, want 2", len(parsed.Hooks.SessionEnd))
	}
	if parsed.Hooks.SessionEnd[0].Hooks[0].Command != "somebody-elses-tool" {
		t.Error("the other tool's hook was displaced")
	}
}

func TestInstallSessionHookRefusesMalformedSettings(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, "{ not json")

	if _, err := InstallSessionHook(dir); err == nil {
		t.Fatal("a malformed settings file should be an error, not an overwrite")
	}
	// The author's file is still theirs.
	if got := readSettings(t, dir); got != "{ not json" {
		t.Errorf("clobbered a file it could not parse: %q", got)
	}
}

func TestHasSessionHookOnRepositoryWithout(t *testing.T) {
	if HasSessionHook(t.TempDir()) {
		t.Error("reported a hook where there is no settings file")
	}
}

func writeSettings(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, claudeSettings), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, dir string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, claudeSettings))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
