package doc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	t.Run("well-formed", func(t *testing.T) {
		fm, err := Parse([]byte("---\nid: eng.example\ntitle: Example\n---\n\n# Example\n\nbody\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm["id"] != "eng.example" {
			t.Fatalf("id = %v, want eng.example", fm["id"])
		}
		if fm["title"] != "Example" {
			t.Fatalf("title = %v, want Example", fm["title"])
		}
	})

	t.Run("no block returns nil map", func(t *testing.T) {
		fm, err := Parse([]byte("# Just prose\n\nno frontmatter\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fm != nil {
			t.Fatalf("expected nil map, got %v", fm)
		}
	})

	t.Run("unclosed block is an error", func(t *testing.T) {
		_, err := Parse([]byte("---\nid: eng.example\n\nbody with no closing fence\n"))
		if err == nil {
			t.Fatal("expected error for unclosed block")
		}
	})

	t.Run("date stays a string", func(t *testing.T) {
		fm, err := Parse([]byte("---\nid: eng.example\ncreated: 2026-07-01\n---\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, ok := fm["created"].(string); !ok || got != "2026-07-01" {
			t.Fatalf("created = %v (%T), want string 2026-07-01", fm["created"], fm["created"])
		}
	})
}

func TestProse(t *testing.T) {
	got := Prose([]byte("---\nid: x\n---\n\n# Title\n\nbody\n"))
	want := "\n# Title\n\nbody\n"
	if string(got) != want {
		t.Fatalf("prose = %q, want %q", got, want)
	}

	whole := "# No frontmatter\n\nbody\n"
	if got := Prose([]byte(whole)); string(got) != whole {
		t.Fatalf("prose = %q, want whole file %q", got, whole)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	write(t, root, "engineering/a.md", "---\nid: eng.a\n---\n# A\n")
	write(t, root, "engineering/index.md", "# listing\n")          // reserved: skipped
	write(t, root, "meta/templates/tpl.md", "---\nid: tpl\n---\n") // skipped by prefix
	write(t, root, "notes.txt", "not markdown")                    // not .md

	docs, err := Load(root, []string{"meta/templates"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("loaded %d docs, want 1: %+v", len(docs), docs)
	}
	if docs[0].ID() != "eng.a" {
		t.Fatalf("id = %q, want eng.a", docs[0].ID())
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
