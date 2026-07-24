package qm

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/target"
)

func openFixture(t *testing.T) *Bundle {
	t.Helper()
	store, err := filepath.Abs("../internal/bundle/testdata/store")
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenAt("file://"+store, ".")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return b
}

func TestCatalogAndDocument(t *testing.T) {
	b := openFixture(t)

	var ids []string
	for _, e := range b.Catalog() {
		ids = append(ids, e.ID)
		if e.Description == "" {
			t.Fatalf("catalog entry %s has no description", e.ID)
		}
	}
	slices.Sort(ids)
	// The draft is retrievable; the restricted document never entered the bundle.
	if want := []string{"eng.draft", "eng.errors", "eng.logging"}; !slices.Equal(ids, want) {
		t.Fatalf("catalog ids = %v, want %v", ids, want)
	}

	body, err := b.Document("eng.errors")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Wrap errors") {
		t.Fatalf("document body unexpected:\n%s", body)
	}
	if strings.Contains(string(body), "id: eng.errors") {
		t.Fatal("Document returned frontmatter; it should return prose only")
	}

	if _, err := b.Document("eng.secret"); err == nil {
		t.Fatal("a restricted document should not be fetchable")
	}
}

// The whole reason the library exists: a document rendered into an instruction
// string is the same text as the same document rendered into a rule file. This
// asserts it against the actual claude target rather than trusting they match.
func TestRulesEqualRuleBody(t *testing.T) {
	b := openFixture(t)

	instruction, err := b.Rules("go-service")
	if err != nil {
		t.Fatal(err)
	}

	// Render the one document go-service selects through the claude target, the
	// way the CLI would, and take the body below its frontmatter.
	prose, err := b.Document("eng.logging")
	if err != nil {
		t.Fatal(err)
	}
	claude, _ := target.Get("claude")
	out, err := claude.Render(target.Input{Docs: []target.Doc{{
		ID: "eng.logging", Prose: prose, Scope: []string{"**/*.go"}, Digest: b.Digest(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	_, ruleBody, ok := strings.Cut(string(out.Files[0].Body), "---\n\n")
	if !ok {
		t.Fatalf("rule file has no body:\n%s", out.Files[0].Body)
	}

	if instruction != ruleBody {
		t.Fatalf("instruction differs from the rule body:\n--- instruction ---\n%q\n--- rule body ---\n%q", instruction, ruleBody)
	}
}

func TestRulesDedupeAndOrder(t *testing.T) {
	b := openFixture(t)

	// core selects eng.logging then eng.errors; go-service selects eng.logging
	// again. The repeat must not appear twice.
	inst, err := b.Rules("core", "go-service")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(inst, "structured key-value pairs"); n != 1 {
		t.Fatalf("eng.logging appears %d times, want 1", n)
	}
	if !strings.Contains(inst, "Wrap errors") {
		t.Fatal("eng.errors missing from the instruction")
	}
}

func TestRulesUnknownRuleset(t *testing.T) {
	b := openFixture(t)
	if _, err := b.Rules("nope"); err == nil {
		t.Fatal("expected an error for an unknown ruleset")
	}
}

func TestRulesetsAndDigest(t *testing.T) {
	b := openFixture(t)
	if got := b.Rulesets(); !slices.Equal(got, []string{"core", "go-service"}) {
		t.Fatalf("rulesets = %v", got)
	}
	if !strings.HasPrefix(b.Digest(), "sha256:") {
		t.Fatalf("digest = %q", b.Digest())
	}
}
