package target

import (
	"strings"
	"testing"
)

func TestClaudeRendersAgent(t *testing.T) {
	out, err := claude{}.Render(Input{Agents: []Agent{{
		ID:          "agents.code-reviewer",
		Name:        "code-reviewer",
		Description: "Reviews a diff against the house standards.",
		Tools:       []string{"Read", "Grep"},
		Model:       "inherit",
		Prose:       []byte("You review a change.\n"),
		Digest:      "sha256:abc",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(out.Files))
	}

	f := out.Files[0]
	// Discovery recurses and identity comes from the name field, so generated
	// agents namespace under qm/ the way rules do.
	if f.Path != ".claude/agents/qm/agents.code-reviewer.md" {
		t.Fatalf("path = %s", f.Path)
	}

	body := string(f.Body)
	for _, want := range []string{
		"name: code-reviewer",
		"tools: Read, Grep",
		"model: inherit",
		"You review a change.",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("agent missing %q:\n%s", want, body)
		}
	}
}

// A field the store did not set must not be emitted, or the harness would read
// an empty value as a deliberate one.
func TestAgentOmitsUnsetFields(t *testing.T) {
	out, _ := claude{}.Render(Input{Agents: []Agent{{
		ID: "agents.x", Name: "x", Prose: []byte("prompt\n"),
	}}})
	body := string(out.Files[0].Body)
	for _, unwanted := range []string{"tools:", "model:", "effort:", "color:", "permissionMode:"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("emitted unset field %q:\n%s", unwanted, body)
		}
	}
}

// Quartermaster owns the agents directory whole, so one pattern covers it.
func TestAgentsAreIgnorable(t *testing.T) {
	var found bool
	for _, p := range (claude{}).IgnorePaths() {
		if strings.Contains(p, ".claude/agents/qm") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no gitignore pattern covers generated agents: %v", (claude{}).IgnorePaths())
	}
}

// Cursor has no agent concept; it must not invent files for them.
func TestCursorIgnoresAgents(t *testing.T) {
	out, _ := cursor{}.Render(Input{Agents: []Agent{{ID: "agents.x", Name: "x"}}})
	if len(out.Files) != 0 {
		t.Fatalf("cursor rendered %d files for an agent, want 0", len(out.Files))
	}
}
