package plan

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/state"
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
		"`file://" + store + "`, following the newest",
		"packages core",
		"moves it to the store's newest published version",
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

// The guide says how current the tree is from the manifest, because a pinned
// source re-delivers one version however often it syncs.
func TestGuideSaysPinnedOrFollowing(t *testing.T) {
	const reg = "oci://registry.example/knowledge"
	follow := state.Bundle{Source: reg, Commit: "0123456789abcdef0123456789abcdef01234567", Packages: []string{"core"}}
	pin := state.Bundle{Source: reg + "@sha256:" + strings.Repeat("a", 64), Commit: "fd93b6f", Packages: []string{"core"}}
	tree := knowledgeTree{total: 1, dirs: map[string]int{"records": 1}}

	for _, tt := range []struct {
		name    string
		bundles []state.Bundle
		want    []string
		not     []string
	}{
		{"following", []state.Bundle{follow},
			[]string{"follows what the store publishes", "`" + reg + "`, following the newest", "store commit `0123456789ab` at the last sync"},
			[]string{"pinned"}},
		{"pinned", []state.Bundle{pin},
			[]string{"pinned to one published version", "never a newer one", ", pinned", "store commit `fd93b6f`"},
			[]string{"follows what the store publishes", "following the newest"}},
		{"mixed", []state.Bundle{follow, pin},
			[]string{"Each source below says which", "following the newest", ", pinned"},
			nil},
	} {
		got := string(guide(tt.bundles, tree).Prose)
		for _, w := range tt.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: guide lacks %q:\n%s", tt.name, w, got)
			}
		}
		for _, w := range tt.not {
			if strings.Contains(got, w) {
				t.Errorf("%s: guide says %q:\n%s", tt.name, w, got)
			}
		}
	}
}
