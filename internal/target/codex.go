package target

import "bytes"

// CodexSkillsDir is where this harness looks for project skills.
const CodexSkillsDir = ".codex/skills"

// codex renders skills for the Codex CLI.
//
// It renders only skills, because this harness has no notion of a rule. Its
// instructions come from AGENTS.md, which is a format several tools share rather
// than one this target owns, so it is left to the agents-md target. A repository
// using Codex therefore configures both, and neither writes over the other.
type codex struct{}

func (codex) Name() string { return "codex" }

// IgnorePaths is empty. A generated skill sits beside hand-written ones and
// ignores itself, and this target writes nothing else.
func (codex) IgnorePaths() []string { return nil }

func (codex) Render(in Input) (Output, error) {
	var files []File
	for _, s := range in.Skills {
		files = append(files, skillFiles(CodexSkillsDir+"/"+s.Name, s, func(*bytes.Buffer) {})...)
	}
	return Output{Files: files}, nil
}
