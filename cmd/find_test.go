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

// A document about the term lists above one that mentions it, and a superseded
// one is not listed at all.
func TestFindRanksAboutAboveMentions(t *testing.T) {
	got := find(t, knowledgeTree(t), "", "drift")

	first, second := strings.Index(got, "records.drift"), strings.Index(got, "eng.apply")
	if first < 0 || second < 0 || first > second {
		t.Errorf("want records.drift before eng.apply:\n%s", got)
	}
	if strings.Contains(got, "old-drift") {
		t.Errorf("a superseded document was listed:\n%s", got)
	}
	if !strings.Contains(got, ".qm/knowledge/records/drift.md") {
		t.Errorf("no path to open:\n%s", got)
	}
}

func TestFindRequiresEveryTermAndHonoursType(t *testing.T) {
	dir := knowledgeTree(t)
	if got := find(t, dir, "", "drift", "apply"); strings.Contains(got, "records.drift") || !strings.Contains(got, "eng.apply") {
		t.Errorf("both terms must match:\n%s", got)
	}
	if got := find(t, dir, "concept", "drift"); strings.Contains(got, "records.drift") {
		t.Errorf("--type did not narrow:\n%s", got)
	}
	if got := find(t, dir, "", "nosuchterm"); !strings.Contains(got, "nothing matches") {
		t.Errorf("no-match report missing:\n%s", got)
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
