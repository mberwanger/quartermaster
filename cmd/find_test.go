package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func knowledgeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, ".qm", "knowledge", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("records/drift.md", "---\nid: records.drift\ntitle: Drift is reported\ndescription: What drift means and who reports it.\ntype: record\nstatus: active\n---\n\n# Drift\n")
	write("records/old-drift.md", "---\nid: records.old-drift\ntitle: Drift, first cut\ndescription: Superseded.\ntype: record\nstatus: active\nsuperseded_by: records.drift\n---\n\n# Old\n")
	write("engineering/apply.md", "---\nid: eng.apply\ntitle: Applying a change set\ndescription: How an apply runs.\ntype: concept\nstatus: active\n---\n\nAn apply that detects drift holds.\n")
	write("engineering/index.md", "# listing\n")
	return dir
}

func find(t *testing.T, dir, docType string, terms ...string) string {
	t.Helper()
	c := newFindCmd()
	c.dir, c.docType, c.top = dir, docType, 20
	var out bytes.Buffer
	if err := c.run(&out, terms); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// A document about the term lists above one that mentions it, a superseded
// one is never listed, and what replaced it says so.
func TestFindRanksAboutAboveMentions(t *testing.T) {
	got := find(t, knowledgeTree(t), "", "drift")

	first, second := strings.Index(got, "records.drift"), strings.Index(got, "eng.apply")
	if first < 0 || second < 0 || first > second {
		t.Errorf("want records.drift before eng.apply:\n%s", got)
	}
	if strings.Contains(got, "records.old-drift  (") {
		t.Errorf("a superseded document was listed:\n%s", got)
	}
	if !strings.Contains(got, "in place of superseded records.old-drift") {
		t.Errorf("the replacement does not name what it replaces:\n%s", got)
	}
	if !strings.Contains(got, ".qm/knowledge/records/drift.md") {
		t.Errorf("no path to open:\n%s", got)
	}
}

// A hit shows the line that matched, numbered as it is in the file, so the
// next step is to open the file there.
func TestFindShowsTheMatchingLine(t *testing.T) {
	got := find(t, knowledgeTree(t), "", "detects drift")
	if !strings.Contains(got, "      9: An apply that detects drift holds.") {
		t.Errorf("want line 9 of the file:\n%s", got)
	}
}

// Every word matching ranks first, and a document holding most of the words
// still ranks below it and says how many it holds.
func TestFindRanksFullMatchesFirstAndHonoursType(t *testing.T) {
	dir := knowledgeTree(t)
	got := find(t, dir, "", "drift", "apply")
	if a, d := strings.Index(got, "eng.apply"), strings.Index(got, "records.drift"); a < 0 || d < 0 || a > d {
		t.Errorf("want eng.apply, which holds both words, first:\n%s", got)
	}
	if !strings.Contains(got, "(matches 1 of 2 words)") {
		t.Errorf("a partial match does not say so:\n%s", got)
	}
	if got := find(t, dir, "concept", "drift"); strings.Contains(got, "records.drift") {
		t.Errorf("--type did not narrow:\n%s", got)
	}
}

// Nothing matching sends the agent to other words and the index, never to
// another checkout.
func TestFindWithNothingMatching(t *testing.T) {
	got := find(t, knowledgeTree(t), "", "nosuchterm")
	for _, want := range []string{"nothing in .qm/knowledge/ matches nosuchterm", "another checkout of the store is not", "index.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("no-match report lacks %q:\n%s", want, got)
		}
	}
}

func TestFindWithoutATreeSaysToSync(t *testing.T) {
	c := newFindCmd()
	c.dir = t.TempDir()
	err := c.run(&bytes.Buffer{}, []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "qm sync") {
		t.Errorf("want an error pointing at sync, got %v", err)
	}
}
