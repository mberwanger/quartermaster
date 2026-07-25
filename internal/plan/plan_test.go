package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".quartermaster.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompute(t *testing.T) {
	store, err := filepath.Abs("../bundle/testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeManifest(t, dir, "bundles:\n"+
		"  - source: file://"+store+"\n"+
		"    use: [core, go-service]\n"+
		"targets:\n  - claude\n"+
		"budget:\n  resident_bytes: 10\n")

	r, err := Compute(dir)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	// eng.logging (scoped in both rulesets) and eng.errors (resident) dedup to
	// two docs.
	if len(r.Docs) != 2 {
		t.Fatalf("docs = %d, want 2", len(r.Docs))
	}

	wantOutputs := []string{
		".claude/rules/qm/eng.errors.md",
		".claude/rules/qm/eng.logging.md",
		".quartermaster/knowledge/engineering/errors.md",
	}
	for _, p := range wantOutputs {
		if _, ok := r.Outputs[p]; !ok {
			t.Fatalf("outputs missing %s", p)
		}
	}

	// The restricted doc never reaches the knowledge tree.
	if _, ok := r.Outputs[".quartermaster/knowledge/engineering/secret.md"]; ok {
		t.Fatal("restricted doc leaked into outputs")
	}

	if r.ResidentBytes == 0 {
		t.Fatal("expected non-zero resident bytes (eng.errors is resident)")
	}

	// Budget of 10 B is far under the resident set, so a warning must fire.
	var budgetWarned bool
	for _, w := range r.Warnings {
		if strings.Contains(w, "budget") {
			budgetWarned = true
		}
	}
	if !budgetWarned {
		t.Fatalf("expected a budget warning, got %v", r.Warnings)
	}
}

func TestComputeUnknownRulesetFails(t *testing.T) {
	store, _ := filepath.Abs("../bundle/testdata/store")
	dir := t.TempDir()
	writeManifest(t, dir, "bundles:\n"+
		"  - source: file://"+store+"\n"+
		"    use: [nope]\n"+
		"targets:\n  - claude\n")

	if _, err := Compute(dir); err == nil {
		t.Fatal("expected error for unknown package")
	}
}
