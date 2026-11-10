package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A skill's scope flows from the store through the package to the renderer,
// which writes the path-scoped pointer rule beside the skill. The pointer is not
// resident, so it costs nothing against the budget.
func TestSkillScopeBecomesAPointerRule(t *testing.T) {
	store := storeWithACapability(t)
	p := filepath.Join(store, "capabilities", "review", "skill.md")
	body, err := os.ReadFile(p) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	scoped := strings.Replace(string(body), "type: skill\n", "type: skill\nscope: [\"**/*.go\"]\n", 1)
	if err := os.WriteFile(p, []byte(scoped), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	writeManifest(t, dir, "bundles:\n"+
		"  - source: file://"+store+"\n"+
		"    use: [capable]\n"+
		"targets:\n  - claude\n")

	r, err := Compute(dir)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	if len(r.Skills) != 1 || len(r.Skills[0].Scope) != 1 || r.Skills[0].Scope[0] != "**/*.go" {
		t.Fatalf("skill scope did not flow through: %+v", r.Skills)
	}
	pointer, ok := r.Outputs[".claude/rules/qm/skills.review.md"]
	if !ok {
		t.Fatalf("no pointer rule in outputs; have %v", keys(r.Outputs))
	}
	if !strings.Contains(string(pointer), "`review` skill") {
		t.Errorf("pointer rule does not name the skill:\n%s", pointer)
	}
	if r.ResidentBytes != 0 {
		t.Errorf("a path-scoped pointer counted as resident: %d B", r.ResidentBytes)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
