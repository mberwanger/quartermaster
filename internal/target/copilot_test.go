package target

import (
	"strings"
	"testing"
)

func TestCopilotResidentAppliesEverywhere(t *testing.T) {
	out, err := copilot{}.Render(Input{Docs: []Doc{{
		ID:          "eng.api-versioning",
		Description: "Add fields, never repurpose them.",
		Prose:       []byte("# API versioning\n\nAdd fields.\n"),
		Digest:      "sha256:abc",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(out.Files))
	}
	f := out.Files[0]
	if f.Path != ".github/instructions/qm-eng.api-versioning.instructions.md" {
		t.Fatalf("path = %s", f.Path)
	}
	body := string(f.Body)
	if !strings.Contains(body, `applyTo: "**"`) {
		t.Fatalf("resident instruction should apply everywhere:\n%s", body)
	}
	if !strings.Contains(body, "# API versioning") {
		t.Fatalf("body missing prose:\n%s", body)
	}
}

func TestCopilotScopedUsesApplyTo(t *testing.T) {
	out, _ := copilot{}.Render(Input{Docs: []Doc{{
		ID:     "eng.logging",
		Scope:  []string{"**/*.go", "cmd/**"},
		Prose:  []byte("# Logging\n"),
		Digest: "sha256:abc",
	}}})
	body := string(out.Files[0].Body)
	if !strings.Contains(body, `applyTo: "**/*.go,cmd/**"`) {
		t.Fatalf("scoped instruction should join globs into applyTo:\n%s", body)
	}
}

// Copilot has no skill or agent concept and must not invent files for them.
func TestCopilotRendersRulesOnly(t *testing.T) {
	out, _ := copilot{}.Render(Input{
		Skills: []Skill{{ID: "skills.x", Name: "x"}},
		Agents: []Agent{{ID: "agents.y", Name: "y"}},
	})
	if len(out.Files) != 0 {
		t.Fatalf("copilot rendered %d files for skills/agents, want 0", len(out.Files))
	}
}

// The generated instructions are covered by one gitignore glob, so a
// hand-written instruction beside them is left alone.
func TestCopilotIgnorePattern(t *testing.T) {
	paths := copilot{}.IgnorePaths()
	if len(paths) != 1 || !strings.Contains(paths[0], "qm-*.instructions.md") {
		t.Fatalf("ignore pattern = %v, want a qm- glob", paths)
	}
}
