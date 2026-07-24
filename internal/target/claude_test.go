package target

import (
	"slices"
	"strings"
	"testing"
)

func TestClaudeRenderResident(t *testing.T) {
	out, err := claude{}.Render(Input{Docs: []Doc{{
		ID:          "eng.errors",
		Path:        "engineering/errors.md",
		Description: "Wrap errors with %w.",
		Prose:       []byte("\n# Error wrapping\n\nWrap errors.\n"),
		Digest:      "sha256:abc",
	}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(out.Files))
	}
	f := out.Files[0]
	if f.Path != ".claude/rules/qm/eng.errors.md" {
		t.Fatalf("path = %q", f.Path)
	}
	body := string(f.Body)
	if strings.Contains(body, "paths:") {
		t.Fatal("resident rule must not declare paths")
	}
	for _, want := range []string{"source: eng.errors", "bundle: sha256:abc", "# Error wrapping"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestClaudeRenderScoped(t *testing.T) {
	out, _ := claude{}.Render(Input{Docs: []Doc{{
		ID:     "eng.logging",
		Path:   "engineering/logging.md",
		Scope:  []string{"**/*.go", "cmd/**"},
		Prose:  []byte("# Logging\n\nUse structured logs.\n"),
		Digest: "sha256:abc",
		Commit: "deadbeef",
	}}})
	body := string(out.Files[0].Body)
	for _, want := range []string{"paths:", "\"**/*.go\"", "\"cmd/**\"", "commit: deadbeef"} {
		if !strings.Contains(body, want) {
			t.Fatalf("scoped body missing %q:\n%s", want, body)
		}
	}
}

func TestRegistry(t *testing.T) {
	if _, ok := Get("claude"); !ok {
		t.Fatal("claude target not registered")
	}
	if _, ok := Get("nope"); ok {
		t.Fatal("unknown target resolved")
	}
	if got := Names(); !slices.Equal(got, []string{"agents-md", "claude", "codex", "copilot", "cursor"}) {
		t.Fatalf("Names() = %v", got)
	}
}
