package search

import (
	"slices"
	"strings"
	"testing"

	"github.com/mberwanger/quartermaster/internal/doc"
)

// entry builds a document the way loading the tree does, with its prose
// starting after a frontmatter block of the given length.
func entry(path string, fm map[string]any, prose string) Entry {
	const front = "---\nid: x\n---\n"
	return NewEntry(doc.Doc{Path: path, Frontmatter: fm, Prose: []byte(prose)}, []byte(front+prose))
}

func ids(hits []Hit) []string {
	var out []string
	for _, h := range hits {
		out = append(out, h.ID())
	}
	return out
}

func tree() []Entry {
	return []Entry{
		entry("engineering/go-cli.md", map[string]any{"id": "skills.go-cli", "title": "Go CLI", "description": "Adding a command or flag, choosing an exit code."},
			"# Go CLI\n\n## Exit codes\n\nA command exits 0 on success and 2 on a usage error.\n"),
		entry("engineering/api.md", map[string]any{"id": "skills.api-design", "title": "API resource design", "description": "How a resource should be shaped."},
			"How should a field be named? A cli client reads it.\n"),
		entry("growth/aeo.md", map[string]any{"id": "growth.aeo", "title": "Answer engine optimization", "description": "Citations."},
			"# Answer engine optimization\n\n## Two goals, one page\n\nSearch earns a click. One page serves both goals.\n"),
		entry("growth/seo.md", map[string]any{"id": "skills.seo", "title": "Search visibility", "description": "One page, two readers."},
			"Every page has two readers. Goals vary.\n"),
		entry("product/map.md", map[string]any{"id": "engineering.system-map", "title": "The system, repo by repo", "description": "Which repository owns which service."},
			"Start here.\n"),
		entry("records/publish.md", map[string]any{"id": "records.a-repository-publishes", "title": "A repository publishes by declaration", "description": "Publishing."},
			"A repository publishes. The repository owns its tags.\n"),
		entry("records/old.md", map[string]any{"id": "records.partial-apply", "title": "A partial apply holds the environment", "status": "superseded", "superseded_by": "records.middle"},
			"A partial apply holds.\n"),
		entry("records/middle.md", map[string]any{"id": "records.middle", "title": "A stopped apply is finished", "status": "superseded", "superseded_by": "records.current"},
			"Stopped.\n"),
		entry("records/current.md", map[string]any{"id": "records.current", "title": "An apply holds only when it left something unwritten"},
			"The hold.\n"),
		entry("records/what.md", map[string]any{"id": "product.what-acme-is", "title": "What Acme is"}, "Acme.\n"),
		entry("records/acme-a.md", map[string]any{"id": "records.acme-ships", "title": "Acme ships Acme"}, "Acme acme acme acme.\n"),
		entry("records/decision.md", map[string]any{"id": "records.decision", "title": "A decision", "description": "Decisions decide."}, "A decision.\n"),
	}
}

// A question asked the way a person asks it finds the document that answers
// it, though most of its words appear nowhere in that document.
func TestQueryAnswersAQuestion(t *testing.T) {
	got := ids(Query(tree(), []string{"how should a cli choose an exit code"}, Options{}).Hits)
	if len(got) == 0 || got[0] != "skills.go-cli" {
		t.Errorf("want skills.go-cli first, got %v", got)
	}
}

// A heading copied from a document finds that document first, and the hit
// points at the heading's line in the file.
func TestQueryFindsAHeadingAndItsLine(t *testing.T) {
	res := Query(tree(), []string{"two", "goals,", "one", "page"}, Options{})
	if len(res.Hits) == 0 || res.Hits[0].ID() != "growth.aeo" {
		t.Fatalf("want growth.aeo first, got %v", ids(res.Hits))
	}
	var found bool
	for _, l := range res.Hits[0].Lines {
		// Three lines of frontmatter, then the heading is the third line of prose.
		if l.N == 6 && strings.Contains(l.Text, "Two goals, one page") {
			found = true
		}
	}
	if !found {
		t.Errorf("want line 6 with the heading, got %+v", res.Hits[0].Lines)
	}
}

// A word standing alone outranks the same letters inside a longer word.
func TestQueryPrefersAWholeWord(t *testing.T) {
	got := ids(Query(tree(), []string{"which", "repo", "owns", "what"}, Options{}).Hits)
	if len(got) == 0 || got[0] != "engineering.system-map" {
		t.Errorf("want engineering.system-map first, got %v", got)
	}
}

// A title that holds every typed word, filler included, is the answer, even
// when the one word left after filler is in every document.
func TestQueryTitleHoldingEveryWordWins(t *testing.T) {
	got := ids(Query(tree(), []string{"what is acme"}, Options{}).Hits)
	if len(got) == 0 || got[0] != "product.what-acme-is" {
		t.Errorf("want product.what-acme-is first, got %v", got)
	}
}

// A superseded document is never listed. What replaced it, at the end of the
// chain, takes its rank and names it.
func TestQueryRanksTheCurrentReplacement(t *testing.T) {
	res := Query(tree(), []string{"partial apply holds environment"}, Options{})
	got := ids(res.Hits)
	if slices.Contains(got, "records.partial-apply") || slices.Contains(got, "records.middle") {
		t.Errorf("a superseded document was listed: %v", got)
	}
	if len(got) == 0 || got[0] != "records.current" {
		t.Fatalf("want records.current first, got %v", got)
	}
	if !slices.Contains(res.Hits[0].Replaces, "records.partial-apply") {
		t.Errorf("want it to name what it replaces, got %v", res.Hits[0].Replaces)
	}
}

// A short word must stand alone, so "ci" does not match "decision".
func TestShortWordsMatchWhole(t *testing.T) {
	if got := Query(tree(), []string{"ci"}, Options{}).Hits; len(got) != 0 {
		t.Errorf("ci matched %v", ids(got))
	}
}

// A document holding under half the words is noise and is not listed.
func TestQueryNeedsMostOfTheWords(t *testing.T) {
	for _, h := range Query(tree(), []string{"exit", "code", "answer", "engine", "citations"}, Options{}).Hits {
		if h.Matched*2 < 5 {
			t.Errorf("%s matched %d of 5", h.ID(), h.Matched)
		}
	}
}

func TestQueryHonoursType(t *testing.T) {
	es := tree()
	es[0].Frontmatter["type"] = "skill"
	if got := ids(Query(es, []string{"exit code"}, Options{Type: "record"}).Hits); slices.Contains(got, "skills.go-cli") {
		t.Errorf("--type did not narrow: %v", got)
	}
}

func TestWords(t *testing.T) {
	for _, tt := range []struct {
		in     string
		words  []string
		phrase string
	}{
		{"how should a cli choose an exit code", []string{"cli", "choose", "exit", "code"}, "how should a cli choose an exit code"},
		{"Two goals, one page", []string{"two", "goals", "one", "page"}, "two goals one page"},
		{"api.example.com", []string{"api.example.com"}, ""},
		{"auto-merge?", []string{"auto-merge"}, ""},
		{"what is", []string{"what", "is"}, "what is"},
	} {
		w, p := Words([]string{tt.in})
		if !slices.Equal(w, tt.words) || p != tt.phrase {
			t.Errorf("Words(%q) = %v, %q; want %v, %q", tt.in, w, p, tt.words, tt.phrase)
		}
	}
}

func TestStem(t *testing.T) {
	for in, want := range map[string]string{"dashes": "dash", "policies": "policy", "goals": "goal", "process": "process", "boxes": "box"} {
		if got := stem(in); got != want {
			t.Errorf("stem(%q) = %q, want %q", in, got, want)
		}
	}
}

// A chain of supersession is followed to its end, and a loop stops.
func TestCurrent(t *testing.T) {
	d := func(id, by string) doc.Doc {
		return doc.Doc{Frontmatter: map[string]any{"id": id, "superseded_by": by}}
	}
	byID := map[string]doc.Doc{"a": d("a", "b"), "b": d("b", "c"), "c": d("c", ""), "x": d("x", "y"), "y": d("y", "x")}
	if got := Current(byID, "a"); got != "c" {
		t.Errorf("Current(a) = %q, want c", got)
	}
	if got := Current(byID, "x"); got != "x" && got != "y" {
		t.Errorf("Current(x) = %q, want the loop to stop", got)
	}
	if got := Current(byID, "missing"); got != "missing" {
		t.Errorf("Current(missing) = %q", got)
	}
}

// A document retired by status alone, with no replacement named, hands its
// rank to nothing, and a file with no id never stands in for one.
func TestQueryRetiredByStatusAlone(t *testing.T) {
	es := []Entry{
		entry("a/old.md", map[string]any{"id": "a.old", "title": "Widget frobnication", "status": "superseded"}, "widget frobnication\n"),
		entry("a/ref.md", map[string]any{}, "a reference\n"),
	}
	if got := Query(es, []string{"widget", "frobnication"}, Options{}).Hits; len(got) != 0 {
		t.Errorf("want nothing, got %v", ids(got))
	}
}

// An alias is a word a reader searches for when a document is the answer. A
// document that names "login" only as an alias outranks one that says login
// again and again, and the hit says which alias matched.
func TestQueryRanksAnAliasLikeATitle(t *testing.T) {
	es := []Entry{
		entry("records/sessions.md", map[string]any{"id": "records.sessions", "title": "A session outlives the tab"},
			"A login starts a session. Login is cheap. Every login counts. A login again.\n"),
		entry("product/glossary.md", map[string]any{"id": "product.glossary", "title": "Glossary", "aliases": []any{"login", "log in", "sign on"}},
			"| sign in | entering your account. Never called logging in |\n"),
	}
	for _, q := range []string{"login", "log in", "sign on"} {
		res := Query(es, []string{q}, Options{})
		if len(res.Hits) == 0 || res.Hits[0].ID() != "product.glossary" {
			t.Errorf("%q: want product.glossary first, got %v", q, ids(res.Hits))
			continue
		}
		if res.Hits[0].Alias == "" {
			t.Errorf("%q: the hit does not say which alias matched", q)
		}
	}
}

// An alias ranks like a title, never above one. A document titled with the
// word and one carrying it as an alias score the same, and listing a title
// again as an alias earns nothing more than the word itself.
func TestAnAliasRanksNoHigherThanATitle(t *testing.T) {
	titled := entry("a.md", map[string]any{"id": "a.titled", "title": "Login"}, "Text.\n")
	aliased := entry("b.md", map[string]any{"id": "b.aliased", "title": "Glossary", "aliases": []any{"login"}}, "Text.\n")
	for _, q := range []string{"login", "log in"} {
		es := []Entry{titled, aliased}
		if q == "log in" {
			es = []Entry{
				entry("a.md", map[string]any{"id": "a.titled", "title": "Log in"}, "Text.\n"),
				entry("b.md", map[string]any{"id": "b.aliased", "title": "Glossary", "aliases": []any{"log in"}}, "Text.\n"),
			}
		}
		res := Query(es, []string{q}, Options{})
		if len(res.Hits) != 2 || res.Hits[0].Score != res.Hits[1].Score {
			t.Errorf("%q: want equal scores, got %+v", q, scores(res.Hits))
		}
	}

	self := entry("c.md", map[string]any{"id": "c.self", "title": "Checkout", "aliases": []any{"checkout"}}, "Text.\n")
	plain := entry("d.md", map[string]any{"id": "d.plain", "title": "Checkout"}, "Text.\n")
	res := Query([]Entry{self, plain}, []string{"checkout"}, Options{})
	if len(res.Hits) != 2 || res.Hits[0].Score > 1.5*res.Hits[1].Score {
		t.Errorf("an alias repeating the title bought a large lead: %+v", scores(res.Hits))
	}
}

// Documents without aliases rank exactly as they did before aliases existed,
// whatever other documents carry.
func TestAliasesChangeNothingForDocumentsWithout(t *testing.T) {
	base := tree()
	before := Query(base, []string{"how should a cli choose an exit code"}, Options{}).Hits
	with := append(tree(), entry("z.md", map[string]any{"id": "z.unrelated", "title": "Unrelated", "aliases": []any{"zebra"}}, "Zebra.\n"))
	after := Query(with, []string{"how should a cli choose an exit code"}, Options{}).Hits
	if !slices.Equal(ids(before), ids(after)) {
		t.Errorf("ranking moved: %v then %v", ids(before), ids(after))
	}
}

// The hit names the alias that earned the score, not the first that touched
// a word.
func TestTheNamedAliasIsTheOneThatScored(t *testing.T) {
	e := entry("a.md", map[string]any{"id": "a.x", "title": "Shipping", "aliases": []any{"redeployment", "deploy"}}, "Text.\n")
	res := Query([]Entry{e}, []string{"deploy now"}, Options{})
	if len(res.Hits) != 1 || res.Hits[0].Alias != "deploy" {
		t.Errorf("want alias deploy, got %+v", res.Hits)
	}
}

// An alias that normalizes to a single character is dropped.
func TestAPunctuationAliasClaimsNothing(t *testing.T) {
	e := entry("a.md", map[string]any{"id": "a.x", "title": "Languages", "aliases": []any{"C++"}}, "Text.\n")
	if got := Query([]Entry{e}, []string{"c"}, Options{}).Hits; len(got) != 0 {
		t.Errorf("C++ claimed the search for c: %v", ids(got))
	}
}

func scores(hits []Hit) []float64 {
	var out []float64
	for _, h := range hits {
		out = append(out, h.Score)
	}
	return out
}

// An aliases field of the wrong shape yields no aliases rather than an error.
func TestAliasesToleratesAWrongShape(t *testing.T) {
	for _, v := range []any{"login", 3, []any{1, " ", "undo"}} {
		got := Aliases(doc.Doc{Frontmatter: map[string]any{"aliases": v}})
		if len(got) > 1 || (len(got) == 1 && got[0] != "undo") {
			t.Errorf("Aliases(%v) = %v", v, got)
		}
	}
}
