package plan

import (
	"path/filepath"
	"strings"
	"testing"
)

// The example store is documentation people run, so it is tested like code. A
// broken example is worse than none: it teaches the wrong model.
const exampleStore = "../../examples/store"

func exampleManifest(t *testing.T, body string) string {
	t.Helper()
	store, err := filepath.Abs(exampleStore)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeManifest(t, dir, strings.ReplaceAll(body, "{{store}}", store))
	return dir
}

// The walkthrough in examples/README.md claims a specific outcome for a billing
// repository. This is that claim, asserted.
func TestExampleBillingRepository(t *testing.T) {
	dir := exampleManifest(t, "bundles:\n"+
		"  - source: file://{{store}}\n"+
		"    rulesets: [engineering, billing]\n"+
		"    knowledge:\n      domain: [engineering, billing]\n"+
		"targets:\n  - claude\n")

	r, err := Compute(dir)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	var resident, scoped []string
	for _, d := range r.Docs {
		if len(d.Scope) == 0 {
			resident = append(resident, d.ID)
		} else {
			scoped = append(scoped, d.ID)
		}
	}
	if len(resident) != 2 || len(scoped) != 3 {
		t.Fatalf("resident=%v scoped=%v, want 2 and 3", resident, scoped)
	}

	// The other team's document is not on the disk at all.
	for p := range r.Knowledge {
		if strings.Contains(p, "eligibility") {
			t.Fatalf("eligibility knowledge leaked into a billing repository: %s", p)
		}
	}

	// The draft is on disk and is not a rule.
	var draftOnDisk bool
	for p := range r.Knowledge {
		if strings.HasSuffix(p, "experimental-cache.md") {
			draftOnDisk = true
		}
	}
	if !draftOnDisk {
		t.Fatal("the draft should still be retrievable on disk")
	}
	for _, d := range r.Docs {
		if d.ID == "eng.experimental-cache" {
			t.Fatal("a draft became a rule")
		}
	}
}

// The same document is Go-scoped under one ruleset and always-loaded under
// another. This is the property the walkthrough leads with.
func TestExampleScopeIsPerConsumer(t *testing.T) {
	scopeOf := func(t *testing.T, rulesets string) []string {
		t.Helper()
		dir := exampleManifest(t, "bundles:\n"+
			"  - source: file://{{store}}\n"+
			"    rulesets: ["+rulesets+"]\n"+
			"targets:\n  - claude\n")
		r, err := Compute(dir)
		if err != nil {
			t.Fatalf("compute %s: %v", rulesets, err)
		}
		for _, d := range r.Docs {
			if d.ID == "eng.logging" {
				return d.Scope
			}
		}
		t.Fatalf("eng.logging missing from ruleset %s", rulesets)
		return nil
	}

	if got := scopeOf(t, "engineering"); len(got) != 1 || got[0] != "**/*.go" {
		t.Fatalf("engineering scope = %v, want [**/*.go]", got)
	}
	if got := scopeOf(t, "incident-review"); len(got) != 0 {
		t.Fatalf("incident-review scope = %v, want none (always loaded)", got)
	}
}

// The contradiction the walkthrough shows must actually be reported.
func TestExampleFilterContradiction(t *testing.T) {
	dir := exampleManifest(t, "bundles:\n"+
		"  - source: file://{{store}}\n"+
		"    rulesets: [eligibility]\n"+
		"    knowledge:\n      domain: [engineering, billing]\n"+
		"targets:\n  - claude\n")

	_, err := Compute(dir)
	if err == nil {
		t.Fatal("expected the ruleset and the filter to contradict")
	}
	if !strings.Contains(err.Error(), "eligibility.coverage-effective-dates") {
		t.Fatalf("error should name the document, got: %v", err)
	}
}
