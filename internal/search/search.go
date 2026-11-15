// Package search ranks the documents of a delivered knowledge tree against the
// words an agent typed, and points at the lines that matched.
//
// It exists so an agent never has a reason to grep. The first version required
// every word to match and weighed every mention the same, so a question asked
// the way a person asks it ("how should a cli choose an exit code") matched
// nothing useful, and an agent that got nothing useful went to grep, then to a
// sibling checkout of the store. The ranking here is built for the question,
// not the keyword: filler words are dropped, a document that matches most of
// the words still ranks, a rare word counts for more than a common one, and the
// exact phrase or a heading that holds it counts for most.
package search

import (
	"bytes"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/mberwanger/quartermaster/internal/doc"
)

// Entry is one document with the raw file it came from, so a hit can name the
// line in the file an agent will open rather than a line in the prose.
type Entry struct {
	doc.Doc
	// Offset is the number of lines in the file before the prose starts, the
	// frontmatter block and its fences.
	Offset int
}

// NewEntry pairs a document with its raw file bytes.
func NewEntry(d doc.Doc, raw []byte) Entry {
	off := 0
	if pre := len(raw) - len(d.Prose); pre > 0 && pre <= len(raw) {
		off = bytes.Count(raw[:pre], []byte("\n"))
	}
	return Entry{Doc: d, Offset: off}
}

// Line is one line of a document that matched, numbered as it is in the file.
type Line struct {
	N    int
	Text string
}

// Hit is a ranked document and the lines that best show why.
type Hit struct {
	Entry
	Score float64
	// Matched is how many of the query's words the document holds.
	Matched int
	Lines   []Line
	// Replaces names superseded documents that matched and handed this one
	// their rank, so a reader who asked by the old name sees why it is here.
	Replaces []string
	// Alias is the alias that matched, when one did, so a reader who searched
	// a word the document does not use as its own sees why it is here.
	Alias string
}

// Result is everything one query found.
type Result struct {
	// Words are the words that were searched for, after filler was dropped.
	Words []string
	Hits  []Hit
}

// Options narrow a query.
type Options struct {
	// Type keeps only documents of this frontmatter type.
	Type string
}

// Field weights. What a document is (id, title, and the aliases a person wrote
// for it) outranks what it is for (the description), which outranks its section
// headings, which outrank a mention.
const (
	wID    = 4.0
	wTitle = 4.0
	// An alias is a word a reader searches for when this document is the
	// answer, which is what a title is for.
	wAlias = wTitle
	wDesc  = 2.0
	wHead  = 2.0
	wPath  = 1.0
	wBody  = 1.0
	// A mention repeated is evidence the document is about the word, with
	// diminishing returns so a long document does not win by length.
	maxBodyCount = 8
	// The query's words found together, in order.
	wPhraseTitle = 10.0
	wPhraseHead  = 8.0
	wPhraseBody  = 4.0
)

// Query ranks entries against what was typed.
func Query(entries []Entry, typed []string, opts Options) Result {
	words, phrase := Words(typed)
	res := Result{Words: words}
	if len(words) == 0 {
		return res
	}

	idx := make([]fields, len(entries))
	for i, e := range entries {
		idx[i] = index(e)
	}
	idf := inverseFrequency(idx, words)

	var replaced []Hit
	for i, e := range entries {
		if e.ID() == "" {
			continue
		}
		if opts.Type != "" && e.Str("type") != opts.Type {
			continue
		}
		h, ok := score(e, idx[i], words, phrase, idf)
		if !ok {
			continue
		}
		if IsSuperseded(e.Doc) {
			replaced = append(replaced, h)
			continue
		}
		res.Hits = append(res.Hits, h)
	}

	// A superseded document is never the answer, but a match on it is evidence
	// about the document that replaced it: whoever asked used the old name.
	// The current replacement takes the replaced one's rank when that is
	// higher, and joins the list if it matched nothing itself.
	byID := map[string]Entry{}
	for _, e := range entries {
		if id := e.ID(); id != "" {
			byID[id] = e
		}
	}
	docs := make(map[string]doc.Doc, len(byID))
	for id, e := range byID {
		docs[id] = e.Doc
	}
	at := map[string]int{}
	for i, h := range res.Hits {
		at[h.ID()] = i
	}
	for _, r := range replaced {
		// Retired by status alone names nothing to send the reader to.
		if r.Str("superseded_by") == "" {
			continue
		}
		by := Current(docs, r.Str("superseded_by"))
		e, ok := byID[by]
		if !ok || IsSuperseded(e.Doc) || (opts.Type != "" && e.Str("type") != opts.Type) {
			continue
		}
		i, ok := at[by]
		if !ok {
			res.Hits = append(res.Hits, Hit{Entry: e})
			i = len(res.Hits) - 1
			at[by] = i
		}
		h := &res.Hits[i]
		h.Replaces = append(h.Replaces, r.ID())
		if r.Score > h.Score {
			h.Score = r.Score
			h.Matched = max(h.Matched, r.Matched)
		}
	}

	rank(res.Hits)
	return res
}

func rank(hits []Hit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].ID() < hits[j].ID()
	})
}

// fields is one document lowered and split, ready to match against.
type fields struct {
	id, title, desc, path string
	aliases               []string
	heads                 []Line
	body                  []Line
	prose                 string
}

func index(e Entry) fields {
	f := fields{
		id:    strings.ToLower(e.ID()),
		title: strings.ToLower(e.Str("title")),
		desc:  strings.ToLower(e.Str("description")),
		path:  strings.ToLower(e.Path),
		prose: strings.ToLower(string(e.Prose)),
	}
	for _, a := range Aliases(e.Doc) {
		// Normalizing folds punctuation away, so "C++" would become "c" and
		// claim every search for a single letter. An alias that keeps fewer
		// than two characters names nothing a reader would search for.
		if n := normalize(a); len([]rune(n)) >= 2 {
			f.aliases = append(f.aliases, n)
		}
	}
	inFence := false
	for i, l := range strings.Split(string(e.Prose), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
		}
		if t == "" {
			continue
		}
		ln := Line{N: e.Offset + i + 1, Text: l}
		if !inFence && strings.HasPrefix(t, "#") {
			f.heads = append(f.heads, ln)
		}
		f.body = append(f.body, ln)
	}
	return f
}

// inverseFrequency weighs each word by how few documents hold it, so a word
// every document mentions decides nothing and a rare one decides a lot.
func inverseFrequency(idx []fields, words []string) map[string]float64 {
	n := float64(len(idx))
	out := make(map[string]float64, len(words))
	for _, w := range words {
		df := 0
		for _, f := range idx {
			if contains(f.id, w) || contains(f.title, w) || contains(f.desc, w) || contains(f.prose, w) || aliasWeight(f.aliases, w) > 0 {
				df++
			}
		}
		out[w] = math.Log(1 + (n-float64(df)+0.5)/(float64(df)+0.5))
	}
	return out
}

func score(e Entry, f fields, words []string, phrase string, idf map[string]float64) (Hit, bool) {
	h := Hit{Entry: e}
	total := 0.0
	for _, w := range words {
		s := 0.0
		s += wID * weight(f.id, w)
		// The title and the aliases are one field: a word earns the better of
		// the two, never both, so repeating the title as an alias buys nothing.
		s += max(wTitle*weight(f.title, w), wAlias*aliasWeight(f.aliases, w))
		s += wDesc * weight(f.desc, w)
		s += wPath * weight(f.path, w)
		for _, l := range f.heads {
			if x := weight(strings.ToLower(l.Text), w); x > 0 {
				s += wHead * x
				break
			}
		}
		if c := count(f.prose, w); c > 0 {
			s += wBody * weight(f.prose, w) * (1 + math.Log(float64(min(c, maxBodyCount))))
		}
		if s > 0 {
			h.Matched++
			total += s * idf[w]
		}
	}
	if h.Matched == 0 {
		return h, false
	}

	// A document has to hold most of what was asked. One that holds a single
	// word of a five-word question is noise, however rare that word is.
	need := (len(words) + 1) / 2
	if h.Matched < need {
		return h, false
	}
	coverage := float64(h.Matched) / float64(len(words))
	total *= coverage * coverage

	typedAll := typedWords(phrase, words)
	h.Alias = bestAlias(f.aliases, words)

	switch {
	// A title, or an alias, that holds every typed word is naming the
	// document. The same rule applies to both, so an alias never ranks above
	// a title that says the same thing.
	case titleHoldsAll(f.title, typedAll):
		total += wPhraseTitle * float64(len(words))
	case anyHoldsAll(f.aliases, typedAll):
		total += wPhraseTitle * float64(len(words))
	case phrase != "":
		switch {
		case strings.Contains(f.title, phrase) || strings.Contains(f.id, strings.ReplaceAll(phrase, " ", "-")):
			total += wPhraseTitle * float64(len(words))
		case headHolds(f.heads, phrase):
			total += wPhraseHead * float64(len(words))
		case strings.Contains(normalize(f.prose), phrase):
			total += wPhraseBody * float64(len(words))
		}
	}

	h.Score = total
	h.Lines = best(f, words, phrase)
	return h, true
}

// Aliases returns a document's aliases: the words a reader searches for when
// this document is the answer, written by a person in the aliases field. A
// field of the wrong shape yields none rather than an error, because the
// store's schema, not the search, is what refuses it.
func Aliases(d doc.Doc) []string {
	list, _ := d.Frontmatter["aliases"].([]any)
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// bestAlias is the alias that earned the most, the one aliasWeight scored, so
// the hit names the alias that actually put the document where it is.
func bestAlias(aliases, words []string) string {
	best, at := 0.0, ""
	for _, a := range aliases {
		got := 0.0
		for _, w := range words {
			got += weight(a, w)
		}
		if got > best {
			best, at = got, a
		}
	}
	return at
}

func anyHoldsAll(aliases, words []string) bool {
	for _, a := range aliases {
		if titleHoldsAll(a, words) {
			return true
		}
	}
	return false
}

// aliasWeight is how strongly the best of a document's aliases holds a word.
func aliasWeight(aliases []string, w string) float64 {
	best := 0.0
	for _, a := range aliases {
		best = max(best, weight(a, w))
	}
	return best
}

// typedWords is every word typed, filler included, because a title that holds
// "what", "is" and "acme" is the answer to "what is acme" in a way a
// title that only holds "acme" is not.
func typedWords(phrase string, words []string) []string {
	if phrase == "" {
		return words
	}
	return strings.Fields(phrase)
}

func titleHoldsAll(title string, words []string) bool {
	if len(words) < 2 {
		return false
	}
	have := map[string]bool{}
	for _, w := range strings.Fields(normalize(title)) {
		have[w] = true
		have[stem(w)] = true
	}
	for _, w := range words {
		if !have[w] && !have[stem(w)] {
			return false
		}
	}
	return true
}

func headHolds(heads []Line, phrase string) bool {
	for _, l := range heads {
		if strings.Contains(normalize(l.Text), phrase) {
			return true
		}
	}
	return false
}

// best picks up to two lines that show why the document matched: a line with
// the phrase first, then the line holding the most words, a heading winning a
// tie because it names the section to read.
func best(f fields, words []string, phrase string) []Line {
	type cand struct {
		Line
		n    int
		head bool
	}
	heads := map[int]bool{}
	for _, l := range f.heads {
		heads[l.N] = true
	}
	var cs []cand
	for _, l := range f.body {
		low := strings.ToLower(l.Text)
		n := 0
		for _, w := range words {
			if contains(low, w) {
				n++
			}
		}
		if phrase != "" && strings.Contains(normalize(l.Text), phrase) {
			n += len(words) + 1
		}
		if n > 0 {
			cs = append(cs, cand{Line: l, n: n, head: heads[l.N]})
		}
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].n != cs[j].n {
			return cs[i].n > cs[j].n
		}
		if cs[i].head != cs[j].head {
			return cs[i].head
		}
		return cs[i].N < cs[j].N
	})
	var out []Line
	for _, c := range cs {
		out = append(out, Line{N: c.N, Text: trim(c.Text)})
		if len(out) == 2 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N < out[j].N })
	return out
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	const limit = 140
	if r := []rune(s); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return s
}

// Current follows a chain of superseded_by from id to the document that has
// not been replaced, so a reader is never sent to a second dead end. A chain
// that loops, or names a document the tree does not hold, stops at the last id
// it reached.
func Current(byID map[string]doc.Doc, id string) string {
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		seen[id] = true
		d, ok := byID[id]
		if !ok {
			return id
		}
		next := d.Str("superseded_by")
		if next == "" {
			return id
		}
		id = next
	}
	return id
}

// IsSuperseded reports whether a document has been replaced, by the field that
// names its successor or by a status that says so.
func IsSuperseded(d doc.Doc) bool {
	if d.Str("superseded_by") != "" {
		return true
	}
	switch d.Str("status") {
	case "superseded", "deprecated", "retired":
		return true
	}
	return false
}

// Words turns what was typed into the words to match and the phrase they form.
// Filler is dropped unless nothing else is left, a word keeps the dots and
// hyphens inside it (api.example.com, auto-merge), and the phrase is the
// typed text with punctuation folded to spaces.
func Words(typed []string) ([]string, string) {
	raw := strings.Fields(normalize(strings.Join(typed, " ")))
	var words []string
	seen := map[string]bool{}
	for _, w := range raw {
		if stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		words = append(words, w)
	}
	if len(words) == 0 {
		for _, w := range raw {
			if !seen[w] {
				seen[w] = true
				words = append(words, w)
			}
		}
	}
	phrase := ""
	if len(raw) > 1 {
		phrase = strings.Join(raw, " ")
	}
	return words, phrase
}

// normalize lowers text and folds punctuation to spaces, keeping a dot, a
// hyphen or an underscore that sits between two letters or digits.
func normalize(s string) string {
	r := []rune(strings.ToLower(s))
	var b strings.Builder
	b.Grow(len(r))
	space := true
	for i, c := range r {
		keep := unicode.IsLetter(c) || unicode.IsDigit(c)
		if !keep && (c == '.' || c == '-' || c == '_') && i > 0 && i+1 < len(r) {
			keep = isWord(r[i-1]) && isWord(r[i+1])
		}
		if keep {
			b.WriteRune(c)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func isWord(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }

// contains matches a word in text. A short word must stand alone, or "ci"
// would match every "decision". A longer one matches inside a longer word, and
// a plural matches its singular, so "dashes" finds "dash".
func contains(text, w string) bool {
	return count(text, w) > 0
}

func count(text, w string) int {
	if len([]rune(w)) <= 3 {
		return wholeWord(text, w)
	}
	if n := strings.Count(text, w); n > 0 {
		return n
	}
	if s := stem(w); s != w {
		return strings.Count(text, s)
	}
	return 0
}

// weight is how strongly a field holds a word: fully when the word stands
// alone, half when it only sits inside a longer word, so "repo" prefers the
// document titled "repo by repo" over one titled "repository".
func weight(text, w string) float64 {
	if count(text, w) == 0 {
		return 0
	}
	if wholeWord(text, w) > 0 || (stem(w) != w && wholeWord(text, stem(w)) > 0) {
		return 1
	}
	return 0.5
}

func wholeWord(text, w string) int {
	n := 0
	for i := 0; ; {
		j := strings.Index(text[i:], w)
		if j < 0 {
			return n
		}
		start, end := i+j, i+j+len(w)
		if (start == 0 || !isWordByte(text[start-1])) && (end == len(text) || !isWordByte(text[end])) {
			n++
		}
		i = end
	}
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c >= 0x80
}

// stem strips a plural ending. It is deliberately crude: the store is English
// written by a small number of people, and a wrong stem costs a weak match,
// never a missed one, because the word itself is always tried first.
func stem(w string) string {
	switch {
	case len(w) > 5 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 4 && (strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "sses") || strings.HasSuffix(w, "xes")):
		return w[:len(w)-2]
	case len(w) > 4 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		return w[:len(w)-1]
	}
	return w
}

// stop is the filler a question carries that says nothing about its subject.
var stop = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a an and are as at be been but by can could
		do does did for from had has have how i if in into is it its me my
		of on or our should so than that the their them then there these they
		this to us was we were what when where which who whom why will with
		would you your about any all also just not no yes get got use used using`) {
		stop[w] = true
	}
}
