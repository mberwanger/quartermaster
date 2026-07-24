package bundle

import (
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/doc"
)

func agentDoc(mode string) doc.Doc {
	block := map[string]any{"name": "code-reviewer"}
	if mode != "" {
		block["permission-mode"] = mode
	}
	return doc.Doc{
		Path: "engineering/agents/code-reviewer.md",
		Frontmatter: map[string]any{
			"id": "agents.code-reviewer", "type": "agent", "agent": block,
		},
	}
}

// An agent grants a capability rather than offering advice, and this particular
// mode removes the last place a person could say no. Arriving by sync is what
// makes it unacceptable, so the build refuses to carry it.
func TestBundleRefusesPermissionBypass(t *testing.T) {
	err := checkAgentPermissions([]doc.Doc{agentDoc("bypassPermissions")})
	if err == nil {
		t.Fatal("expected a bundle to refuse a permission bypass")
	}
	for _, want := range []string{"bypassPermissions", "locally"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should name the mode and the remedy, got: %v", err)
		}
	}
}

// The modes that keep a person in the loop are fine to distribute.
func TestBundleAllowsSupervisedModes(t *testing.T) {
	for _, mode := range []string{"", "default", "acceptEdits", "plan"} {
		if err := checkAgentPermissions([]doc.Doc{agentDoc(mode)}); err != nil {
			t.Fatalf("mode %q should be distributable: %v", mode, err)
		}
	}
}

// A document that is not an agent carries no such block and must not trip the
// check.
func TestNonAgentDocsAreUnaffected(t *testing.T) {
	d := doc.Doc{Path: "voice/base.md", Frontmatter: map[string]any{"id": "voice.base", "type": "policy"}}
	if err := checkAgentPermissions([]doc.Doc{d}); err != nil {
		t.Fatalf("a non-agent document tripped the check: %v", err)
	}
}
