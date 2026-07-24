package target

import (
	"strings"
	"testing"
)

func aSkill() Skill {
	return Skill{
		ID:           "skills.gcp-expert",
		Name:         "gcp-expert",
		Description:  "Reach for this when designing GCP IAM.",
		AllowedTools: []string{"Read", "Grep"},
		Prose:        []byte("# GCP Expert\n\nSee `references/iam.md`.\n"),
		Digest:       "sha256:abc",
		Assets:       []File{{Path: "references/iam.md", Body: []byte("# IAM\n")}},
	}
}

func bodyAt(out Output, path string) (string, bool) {
	for _, f := range out.Files {
		if f.Path == path {
			return string(f.Body), true
		}
	}
	return "", false
}

// Every harness reads a skill from its own directory, and each gets the assets
// beside it so the skill's own references resolve.
func TestSkillReachesEveryHarness(t *testing.T) {
	cases := []struct {
		target Target
		dir    string
	}{
		{claude{}, ".claude/skills/gcp-expert"},
		{cursor{}, ".cursor/skills/gcp-expert"},
		{codex{}, ".codex/skills/gcp-expert"},
	}

	for _, tc := range cases {
		t.Run(tc.target.Name(), func(t *testing.T) {
			out, err := tc.target.Render(Input{Skills: []Skill{aSkill()}})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{tc.dir + "/SKILL.md", tc.dir + "/references/iam.md", tc.dir + "/.gitignore"} {
				if _, ok := bodyAt(out, want); !ok {
					t.Fatalf("missing %s", want)
				}
			}
		})
	}
}

// The prose is written once and shared, so the same skill delivered to two tools
// is the same text rather than two copies that drift.
func TestSkillBodyIsIdenticalAcrossHarnesses(t *testing.T) {
	body := func(tg Target, dir string) string {
		out, _ := tg.Render(Input{Skills: []Skill{aSkill()}})
		s, ok := bodyAt(out, dir+"/SKILL.md")
		if !ok {
			t.Fatalf("%s rendered no SKILL.md", tg.Name())
		}
		_, after, _ := strings.Cut(s, "---\n\n")
		return after
	}

	claudeBody := body(claude{}, ".claude/skills/gcp-expert")
	for _, tc := range []struct {
		tg  Target
		dir string
	}{{cursor{}, ".cursor/skills/gcp-expert"}, {codex{}, ".codex/skills/gcp-expert"}} {
		if got := body(tc.tg, tc.dir); got != claudeBody {
			t.Fatalf("%s body differs from claude:\n%q\nvs\n%q", tc.tg.Name(), got, claudeBody)
		}
	}
}

// allowed-tools is Claude's contract alone. Emitting it elsewhere would put a
// field in a file whose reader has no meaning for it.
func TestAllowedToolsOnlyForClaude(t *testing.T) {
	claudeOut, _ := claude{}.Render(Input{Skills: []Skill{aSkill()}})
	s, _ := bodyAt(claudeOut, ".claude/skills/gcp-expert/SKILL.md")
	if !strings.Contains(s, "allowed-tools: Read, Grep") {
		t.Fatalf("claude should carry allowed-tools:\n%s", s)
	}

	for _, tc := range []struct {
		tg  Target
		dir string
	}{{cursor{}, ".cursor/skills/gcp-expert"}, {codex{}, ".codex/skills/gcp-expert"}} {
		out, _ := tc.tg.Render(Input{Skills: []Skill{aSkill()}})
		s, _ := bodyAt(out, tc.dir+"/SKILL.md")
		if strings.Contains(s, "allowed-tools") {
			t.Fatalf("%s must not emit allowed-tools:\n%s", tc.tg.Name(), s)
		}
	}
}

// Codex takes its instructions from AGENTS.md, which several tools share, so
// this target owns skills and nothing else.
func TestCodexRendersSkillsOnly(t *testing.T) {
	out, err := codex{}.Render(Input{
		Docs:   []Doc{{ID: "eng.logging", Prose: []byte("x\n")}},
		Agents: []Agent{{ID: "agents.x", Name: "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 0 || len(out.Blocks) != 0 {
		t.Fatalf("codex rendered %d files and %d blocks for rules and agents, want none",
			len(out.Files), len(out.Blocks))
	}
}
