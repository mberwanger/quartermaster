package target

import (
	"strings"
	"testing"
)

func TestApplyBlockPreservesOutside(t *testing.T) {
	existing := "# My Project\n\nHand-authored.\n"
	got := ApplyBlock(existing, "QM", "pointer body")

	if !strings.Contains(got, "# My Project") || !strings.Contains(got, "Hand-authored.") {
		t.Fatalf("author content lost:\n%s", got)
	}
	if !strings.Contains(got, "<!-- BEGIN QM -->") || !strings.Contains(got, "<!-- END QM -->") {
		t.Fatalf("markers missing:\n%s", got)
	}

	region, ok := BlockRegion(got, "QM")
	if !ok || region != "pointer body" {
		t.Fatalf("region = %q ok=%v", region, ok)
	}
}

func TestApplyBlockReplacesRegion(t *testing.T) {
	first := ApplyBlock("# T\n", "QM", "one")
	second := ApplyBlock(first, "QM", "two")

	if strings.Contains(second, "one") {
		t.Fatalf("old region survived:\n%s", second)
	}
	region, _ := BlockRegion(second, "QM")
	if region != "two" {
		t.Fatalf("region = %q, want two", region)
	}
	// The block must not accumulate: exactly one BEGIN marker.
	if n := strings.Count(second, "<!-- BEGIN QM -->"); n != 1 {
		t.Fatalf("expected 1 begin marker, got %d", n)
	}
}

func TestCursorScopedAndResident(t *testing.T) {
	out, _ := cursor{}.Render(Input{Docs: []Doc{
		{ID: "a", Scope: []string{"**/*.go"}, Prose: []byte("x\n"), Digest: "d"},
		{ID: "b", Prose: []byte("y\n"), Digest: "d"},
	}})
	if len(out.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(out.Files))
	}
	scoped := string(out.Files[0].Body)
	if !strings.Contains(scoped, "globs: **/*.go") || !strings.Contains(scoped, "alwaysApply: false") {
		t.Fatalf("scoped mdc wrong:\n%s", scoped)
	}
	if !strings.HasSuffix(out.Files[0].Path, "a.mdc") {
		t.Fatalf("path = %s", out.Files[0].Path)
	}
	resident := string(out.Files[1].Body)
	if !strings.Contains(resident, "alwaysApply: true") || strings.Contains(resident, "globs:") {
		t.Fatalf("resident mdc wrong:\n%s", resident)
	}
}

func TestAgentsMDPointer(t *testing.T) {
	out, _ := agentsMD{}.Render(Input{
		Docs: []Doc{
			{ID: "r", Description: "resident one"},
			{ID: "s", Scope: []string{"**/*.go"}, Description: "scoped one"},
		},
		Bundles: []Bundle{{Digest: "sha256:xyz", Rulesets: []string{"voice"}}},
	})
	if len(out.Files) != 0 || len(out.Blocks) != 1 {
		t.Fatalf("expected 0 files, 1 block; got %d/%d", len(out.Files), len(out.Blocks))
	}
	blk := out.Blocks[0]
	if blk.Path != "AGENTS.md" || blk.Marker != "QUARTERMASTER" {
		t.Fatalf("block target = %s/%s", blk.Path, blk.Marker)
	}
	for _, want := range []string{"sha256:xyz", "voice", "`r`", "resident one", "`s` (**/*.go)", "scoped one"} {
		if !strings.Contains(blk.Body, want) {
			t.Fatalf("pointer missing %q:\n%s", want, blk.Body)
		}
	}
}
