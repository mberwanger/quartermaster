package plan

import (
	"path/filepath"
	"strings"
	"testing"
)

// The guide is generated from the sync, so it describes the tree that is there:
// the directory counts, the sources, and the packages. It is resident, and it
// is the first rule every harness with rules renders.
func TestGuideDescribesTheDeliveredTree(t *testing.T) {
	store, err := filepath.Abs("../bundle/testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeManifest(t, dir, "bundles:\n"+
		"  - source: file://"+store+"\n"+
		"    use: [core]\n"+
		"targets:\n  - claude\n  - agents-md\n")

	r, err := Compute(dir)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	rule, ok := r.Outputs[".claude/rules/qm/"+GuideID+".md"]
	if !ok {
		t.Fatalf("guide not rendered; outputs %v", keys(r.Outputs))
	}
	for _, want := range []string{
		"# Delivered knowledge",
		"`.qm/knowledge/`",
		"`engineering/`: 3", // errors, logging and the draft travel; the restricted doc does not
		"`file://" + store + "`, packages core",
		"qm find",
		"run `qm sync`",
	} {
		if !strings.Contains(string(rule), want) {
			t.Errorf("guide lacks %q:\n%s", want, rule)
		}
	}
	// The tool wrote it, so it names no bundle in its provenance line.
	if strings.Contains(string(rule), "bundle: \n") || strings.Contains(string(rule), "bundle:  ") {
		t.Errorf("guide prints an empty bundle:\n%s", rule)
	}
	if r.ResidentBytes < len(r.Guide.Prose) {
		t.Errorf("guide not counted against the resident set: %d < %d", r.ResidentBytes, len(r.Guide.Prose))
	}

	var block string
	for _, blk := range r.Blocks {
		block = blk.Body
	}
	if !strings.Contains(block, "`"+GuideID+"`") {
		t.Errorf("AGENTS.md block does not list the guide:\n%s", block)
	}
}
