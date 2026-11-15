package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func show(t *testing.T, dir string, asIs bool, refs ...string) (string, error) {
	t.Helper()
	c := newShowCmd()
	c.dir, c.asIs = dir, asIs
	var out bytes.Buffer
	err := c.run(&out, refs)
	return out.String(), err
}

// A document is named by its id, by the link other documents use, or by its
// path in the tree, and a directory prints its index.
func TestShowResolvesEveryWayOfNamingADocument(t *testing.T) {
	dir := knowledgeTree(t)
	for ref, want := range map[string]string{
		"eng.apply":                          "==> .qm/knowledge/engineering/apply.md <==",
		"/engineering/apply.md":              "==> .qm/knowledge/engineering/apply.md <==",
		"/engineering/apply.md#a-section":    "==> .qm/knowledge/engineering/apply.md <==",
		".qm/knowledge/engineering/apply.md": "==> .qm/knowledge/engineering/apply.md <==",
		"engineering/apply":                  "==> .qm/knowledge/engineering/apply.md <==",
		"engineering/":                       "==> .qm/knowledge/engineering/index.md <==",
		"/engineering":                       "==> .qm/knowledge/engineering/index.md <==",
	} {
		got, err := show(t, dir, false, ref)
		if err != nil {
			t.Errorf("%s: %v", ref, err)
			continue
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s: want %q in:\n%s", ref, want, got)
		}
	}
}

// A superseded document prints what replaced it and says so; --as-is prints
// the one that was named.
func TestShowFollowsSupersession(t *testing.T) {
	dir := knowledgeTree(t)
	got, err := show(t, dir, false, "records.old-drift")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "records.old-drift is superseded; showing records.drift") || !strings.Contains(got, "records/drift.md <==") {
		t.Errorf("did not follow supersession:\n%s", got)
	}
	got, err = show(t, dir, true, "records.old-drift")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "records/old-drift.md <==") || strings.Contains(got, "is superseded") {
		t.Errorf("--as-is did not print the named document:\n%s", got)
	}
}

// A name the tree does not hold fails, and offers the closest documents.
func TestShowOffersTheClosest(t *testing.T) {
	_, err := show(t, knowledgeTree(t), false, "drift")
	if err == nil || !strings.Contains(err.Error(), "Closest: records.drift") {
		t.Errorf("want the closest ids, got %v", err)
	}
}

// Show never reads outside the tree, which is where another checkout of the
// store would be.
func TestShowStaysInsideTheTree(t *testing.T) {
	dir := knowledgeTree(t)
	for _, ref := range []string{"../../../outside/", "../../../outside", "..", "/../x.md"} {
		if got, err := show(t, dir, false, ref); err == nil {
			t.Errorf("%s: read outside the tree:\n%s", ref, got)
		}
	}
}
