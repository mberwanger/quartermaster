package target

import (
	"strings"
	"testing"
)

func TestClaudeRendersSkillWithAssets(t *testing.T) {
	out, err := claude{}.Render(Input{Skills: []Skill{{
		ID:           "skills.gcp-expert",
		Name:         "gcp-expert",
		Description:  "Reach for this when designing GCP IAM and networking.",
		AllowedTools: []string{"Read", "Grep"},
		Prose:        []byte("# GCP Expert\n\nSee `references/iam.md`.\n"),
		Digest:       "sha256:abc",
		Assets:       []File{{Path: "references/iam.md", Body: []byte("# IAM\n")}},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]string{}
	for _, f := range out.Files {
		byPath[f.Path] = string(f.Body)
	}

	skill, ok := byPath[".claude/skills/gcp-expert/SKILL.md"]
	if !ok {
		t.Fatalf("skill not rendered; got %v", keysOf(byPath))
	}
	for _, want := range []string{"name: gcp-expert", "allowed-tools: Read, Grep", "# GCP Expert"} {
		if !strings.Contains(skill, want) {
			t.Fatalf("SKILL.md missing %q:\n%s", want, skill)
		}
	}

	// The asset keeps its relative layout, so the skill's own reference to
	// references/iam.md is true once materialized.
	if _, ok := byPath[".claude/skills/gcp-expert/references/iam.md"]; !ok {
		t.Fatalf("asset not rendered beside the skill; got %v", keysOf(byPath))
	}
}

// A generated skill sits beside hand-written ones, because the harness reads the
// directory name as the skill's identity. No repository-level pattern can
// separate the two, so each generated directory ignores itself instead.
func TestSkillDirectoryIgnoresItself(t *testing.T) {
	out, _ := claude{}.Render(Input{Skills: []Skill{{ID: "skills.x", Name: "x"}}})

	var ignore string
	for _, f := range out.Files {
		if f.Path == ".claude/skills/x/.gitignore" {
			ignore = string(f.Body)
		}
	}
	if ignore == "" {
		t.Fatal("generated skill directory carries no .gitignore")
	}
	if !strings.Contains(ignore, "*") {
		t.Fatalf(".gitignore does not ignore the directory: %q", ignore)
	}

	// Skills must not be covered by a repository-level pattern, or a
	// hand-written skill would be ignored along with the generated ones.
	for _, p := range (claude{}).IgnorePaths() {
		if strings.Contains(p, "skills") {
			t.Fatalf("skills must not be covered by a repository pattern: %q", p)
		}
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// A harness never loads a skill by path, and an agent editing a test file does
// not reliably ask for the testing skill. A scoped skill therefore gets a small
// path-scoped rule beside it that names the skill when a matching file is open.
func TestClaudeRendersAPointerRuleForAScopedSkill(t *testing.T) {
	out, err := claude{}.Render(Input{Skills: []Skill{
		{
			ID:          "skills.go-testing",
			Name:        "go-testing",
			Description: "Reach for this when writing or reviewing a Go test.",
			Scope:       []string{"**/*_test.go"},
			Prose:       []byte("# Go testing\n\nLong guidance.\n"),
			Digest:      "sha256:abc",
		},
		{
			ID:     "skills.planning",
			Name:   "planning",
			Prose:  []byte("# Planning\n"),
			Digest: "sha256:abc",
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]string{}
	for _, f := range out.Files {
		byPath[f.Path] = string(f.Body)
	}

	pointer, ok := byPath[".claude/rules/qm/skills.go-testing.md"]
	if !ok {
		t.Fatalf("no pointer rule for the scoped skill; got %v", keysOf(byPath))
	}
	for _, want := range []string{
		"paths:\n  - \"**/*_test.go\"",
		"`go-testing` skill",
		"Invoke it with the Skill tool",
		"Reach for this when writing or reviewing a Go test.",
		GeneratedMarker,
	} {
		if !strings.Contains(pointer, want) {
			t.Errorf("pointer rule lacks %q:\n%s", want, pointer)
		}
	}
	// The pointer carries none of the skill's body: that stays in the one
	// place the store holds it.
	if strings.Contains(pointer, "Long guidance") {
		t.Errorf("pointer rule inlined the skill body:\n%s", pointer)
	}
	if _, ok := byPath[".claude/rules/qm/skills.planning.md"]; ok {
		t.Error("an unscoped skill got a pointer rule")
	}
}
