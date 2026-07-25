// Package gaps turns a corpus of facet records into a ranked shortlist of
// things the knowledge store should probably say.
//
// Two rules shape everything here, and both come from the same place: a store
// that grows by frequency alone becomes a stale mirror of the code.
//
// The first is that a recurring question is checked against the store before it
// is proposed. A question the store already answers, that an agent still burned
// turns on, is a discoverability problem, and writing a second document is the
// one response guaranteed not to help.
//
// The second is that ranking is frequency times non-recoverability. The most
// common thing an agent asks is what something does, which is recoverable by
// reading the file and ages on a schedule. What cannot be recovered at any token
// budget is why a thing is the way it is, and what somebody had to go outside
// the repository to find out. The resolution recorded on each question is what
// separates the two, which is why it is a closed set.
//
// Clustering is lexical rather than semantic. It groups questions that share
// their significant words, which catches rephrasings and misses paraphrases. A
// model would cluster better; whether to spend one on this is undecided, so this
// does the honest cheap thing and says so in the output.
package gaps

import (
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/facet"
)

// recoverability is how much a question of each resolution argues for writing
// something down.
//
// A question the store answered is not a gap at all. One answered by reading
// this repository's own code argues weakly, because a document about it is a bet
// that the document stays truer than the code, and that bet loses on a schedule.
// One that sent somebody outside the repository, or that nobody could answer,
// argues strongly: that answer is not coming back on its own.
var recoverability = map[string]float64{
	facet.ResolutionStoreRead:       0.0,
	facet.ResolutionSourceRead:      0.2,
	facet.ResolutionBashExploration: 0.3,
	facet.ResolutionExternalDocs:    0.7,
	facet.ResolutionDelegated:       0.8,
	facet.ResolutionAskedHuman:      0.9,
	facet.ResolutionUnresolved:      1.0,
}

// Occurrence is one question, kept with enough context to go back to it. A
// cluster without the sessions behind it is a claim rather than evidence.
type Occurrence struct {
	Question   string
	Resolution string
	Resolved   bool
	ToolCalls  int
	Session    string
	Repo       string
}

// Cluster is a group of questions that look like the same question.
type Cluster struct {
	// Label is the occurrence that best represents the group.
	Label       string
	Occurrences []Occurrence
	// Repos is every repository the question came up in, sorted.
	Repos []string
	// ToolCalls is what the cluster cost in total, as far as the records say.
	ToolCalls int
	// Score is frequency times mean non-recoverability. It orders the output
	// and means nothing on its own.
	Score float64
	// Kind is what to do about it, once the store has been consulted.
	Kind Kind
	// Answers names the store documents that already cover this, when any do.
	Answers []string
}

// Kind is what a cluster asks of a person.
type Kind string

const (
	// KindContent is a question the store cannot answer. Write something.
	KindContent Kind = "content"
	// KindDiscoverability is a question the store answers and the agent did not
	// find. Fix the description or the index, do not write a second document.
	KindDiscoverability Kind = "discoverability"
	// KindRecoverable is a question the repository answers itself. Usually
	// leave it alone: a document would mostly be a copy of the code.
	KindRecoverable Kind = "recoverable"
)

// Doc is what the installed store can say about itself, reduced to what
// matching needs.
type Doc struct {
	ID          string
	Title       string
	Description string
}

// Options configures an analysis.
type Options struct {
	// Store is the installed knowledge, or empty when none is installed. With
	// no store, nothing can be called a discoverability problem, because there
	// is nothing to have failed to find.
	Store []Doc
	// MinOccurrences drops clusters seen fewer times. One occurrence is an
	// anecdote at this volume.
	MinOccurrences int
}

// Analyze clusters the questions in a corpus and ranks what is worth acting on.
func Analyze(facets []facet.Facet, opts Options) []Cluster {
	var occurrences []Occurrence
	for _, f := range facets {
		for _, q := range f.Questions {
			occurrences = append(occurrences, Occurrence{
				Question:   q.Question,
				Resolution: q.Resolution,
				Resolved:   q.Resolved,
				ToolCalls:  q.ToolCalls,
				Session:    f.Session,
				Repo:       f.Repo,
			})
		}
	}

	clusters := cluster(occurrences)
	for i := range clusters {
		score(&clusters[i])
		classify(&clusters[i], opts.Store)
	}

	kept := clusters[:0]
	for _, c := range clusters {
		if len(c.Occurrences) >= max(opts.MinOccurrences, 1) {
			kept = append(kept, c)
		}
	}

	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Score != kept[j].Score {
			return kept[i].Score > kept[j].Score
		}
		return kept[i].Label < kept[j].Label
	})
	return kept
}

// cluster groups occurrences by shared significant words, single linkage.
func cluster(occurrences []Occurrence) []Cluster {
	tokens := make([]map[string]bool, len(occurrences))
	for i, o := range occurrences {
		tokens[i] = significant(o.Question)
	}

	// Union-find over pairs that look like the same question.
	parent := make([]int, len(occurrences))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}

	for i := range occurrences {
		for j := i + 1; j < len(occurrences); j++ {
			if similar(tokens[i], tokens[j]) {
				parent[find(i)] = find(j)
			}
		}
	}

	byRoot := map[int][]Occurrence{}
	for i, o := range occurrences {
		root := find(i)
		byRoot[root] = append(byRoot[root], o)
	}

	out := make([]Cluster, 0, len(byRoot))
	for _, group := range byRoot {
		out = append(out, Cluster{Label: label(group), Occurrences: group})
	}
	return out
}

// similar reports whether two questions are close enough to be the same one.
//
// Jaccard over significant words, at a threshold tuned to be conservative:
// merging two questions that are not the same invents a cluster, and an invented
// cluster is worse than a missed one because it is the thing a person acts on.
func similar(a, b map[string]bool) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	var shared int
	for w := range a {
		if b[w] {
			shared++
		}
	}
	if shared == 0 {
		return false
	}

	union := len(a) + len(b) - shared
	return float64(shared)/float64(union) >= 0.4
}

// label picks the occurrence that best represents a group: the one sharing most
// with the rest, and the shortest of those, since the shortest phrasing of a
// recurring question is usually the clearest.
func label(group []Occurrence) string {
	if len(group) == 1 {
		return group[0].Question
	}

	tokens := make([]map[string]bool, len(group))
	for i, o := range group {
		tokens[i] = significant(o.Question)
	}

	best, bestShared := 0, -1
	for i := range group {
		var shared int
		for j := range group {
			if i == j {
				continue
			}
			for w := range tokens[i] {
				if tokens[j][w] {
					shared++
				}
			}
		}
		if shared > bestShared || (shared == bestShared && len(group[i].Question) < len(group[best].Question)) {
			best, bestShared = i, shared
		}
	}
	return group[best].Question
}

// score sets frequency times mean non-recoverability, and fills the totals the
// output shows alongside it so a person can disagree with the ranking.
func score(c *Cluster) {
	repos := map[string]bool{}
	var weight float64

	for _, o := range c.Occurrences {
		c.ToolCalls += o.ToolCalls
		if o.Repo != "" {
			repos[o.Repo] = true
		}
		weight += recoverability[o.Resolution]
	}

	c.Repos = make([]string, 0, len(repos))
	for r := range repos {
		c.Repos = append(c.Repos, r)
	}
	sort.Strings(c.Repos)

	mean := weight / float64(len(c.Occurrences))
	c.Score = float64(len(c.Occurrences)) * mean
}

// classify decides what a cluster asks of a person.
//
// The store is consulted before anything is proposed, which is the step most
// implementations skip. A question the store already answers is not a request
// for another document.
func classify(c *Cluster, store []Doc) {
	c.Answers = matches(c.Label, store)
	switch {
	case len(c.Answers) > 0:
		c.Kind = KindDiscoverability
	case c.Score/float64(len(c.Occurrences)) < 0.4:
		c.Kind = KindRecoverable
	default:
		c.Kind = KindContent
	}
}

// matches finds store documents that look like they already answer a question.
//
// Matching is on the description as well as the title, because the description
// is what an agent reads when it decides whether to open a document. A document
// that covers the topic but describes it in words nobody would search is exactly
// the discoverability failure this is looking for, and it will not match here
// either, which is the correct outcome: the fix is the description.
func matches(question string, store []Doc) []string {
	want := significant(question)
	if len(want) == 0 {
		return nil
	}

	var out []string
	for _, d := range store {
		have := significant(d.Title + " " + d.Description)
		var shared int
		for w := range want {
			if have[w] {
				shared++
			}
		}
		if shared >= 2 && float64(shared)/float64(len(want)) >= 0.5 {
			out = append(out, d.ID)
		}
	}
	sort.Strings(out)
	return out
}

// stopwords carry no signal about what a question was about.
var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "but": true, "by": true, "can": true, "do": true, "does": true,
	"for": true, "from": true, "how": true, "in": true, "into": true, "is": true,
	"it": true, "its": true, "of": true, "on": true, "or": true, "our": true,
	"should": true, "so": true, "that": true, "the": true, "their": true,
	"them": true, "there": true, "these": true, "they": true, "this": true,
	"to": true, "we": true, "what": true, "when": true, "where": true,
	"which": true, "why": true, "with": true, "you": true, "your": true,
	"actually": true, "any": true, "anything": true, "does_it": true,
	"here": true, "must": true, "not": true, "use": true, "used": true,
	"using": true, "was": true, "were": true, "will": true, "would": true,
}

// significant reduces a question to the words that say what it was about.
func significant(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' && r != '/'
	}) {
		w = strings.Trim(w, "-._/")
		if len(w) < 3 || stopwords[w] {
			continue
		}
		out[singular(w)] = true
	}
	return out
}

// singular folds the plural of a word onto the singular, so "imports" and
// "import" are the same word. Crude on purpose: a stemmer would be a dependency
// and a source of surprises, and the questions here are short.
func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "sses"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3:
		return w[:len(w)-1]
	}
	return w
}
