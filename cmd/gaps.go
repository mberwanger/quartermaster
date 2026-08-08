package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/facet"
	"github.com/mberwanger/quartermaster/internal/gaps"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/repo"
)

type gapsCmd struct {
	cmd    *cobra.Command
	dir    string
	all    bool
	since  string
	min    int
	top    int
	drafts string
}

func newGapsCmd() *gapsCmd {
	v := &gapsCmd{}

	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Rank what the knowledge store should probably say",
		Long: `Cluster the questions sessions kept having to answer, and rank what is worth
writing down.

Every recurring question is checked against the store you have installed before
anything is proposed, which splits the output in two. A question the store cannot
answer is a content gap. A question it answers, that an agent still burned turns
on, is a discoverability problem, and writing a second document is the one
response guaranteed not to help.

Ranking is frequency times non-recoverability, not frequency. The most common
thing an agent asks is what something does, which is recoverable by reading the
file and ages fast, so a document about it is a bet that the document stays
truer than the code. Questions that sent somebody outside the repository, or
that nobody could answer, rank highest, because those answers are not coming
back on their own.

Read the output as a shortlist to look at, not a measurement. Clustering is
lexical, so it groups rephrasings and misses paraphrases, and at this volume
every number is directional.`,
		Example: "  qm gaps --all --min 2 --drafts ./drafts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository to analyze")
	cmd.Flags().BoolVar(&v.all, "all", false, "Every repository in the corpus")
	cmd.Flags().StringVar(&v.since, "since", "", "Only sessions this recent (e.g. 30d)")
	cmd.Flags().IntVar(&v.min, "min", 1, "Ignore questions seen fewer times than this")
	cmd.Flags().IntVar(&v.top, "top", 10, "How many clusters to show")
	cmd.Flags().StringVar(&v.drafts, "drafts", "", "Write candidate documents into this directory")

	v.cmd = cmd
	return v
}

func (v *gapsCmd) run(out io.Writer) error {
	facets, err := facet.LoadAll()
	if err != nil {
		return err
	}

	since, err := parseWindow(v.since)
	if err != nil {
		return err
	}

	want := ""
	if !v.all {
		want = repo.Identity(v.dir)
	}

	var window []facet.Facet
	var annotated int
	for _, f := range facets {
		if want != "" && f.Repo != want {
			continue
		}
		if !since.IsZero() && f.EndedAt.Before(since) {
			continue
		}
		window = append(window, f)
		if f.Source == facet.SourceModel {
			annotated++
		}
	}

	if len(window) == 0 {
		_, err := fmt.Fprintf(out, "no sessions in the corpus%s. qm digest --backfill builds one.\n", scopeSuffix(want))
		return err
	}

	store := installedStore(v.dir)
	clusters := gaps.Analyze(window, gaps.Options{Store: store, MinOccurrences: v.min})

	var b strings.Builder
	v.writeHeader(&b, window, annotated, store, want)
	if len(clusters) == 0 {
		b.WriteString("\nNothing recurring yet. Annotate more sessions: qm digest list --pending\n")
		_, err := io.WriteString(out, b.String())
		return err
	}
	v.writeClusters(&b, clusters)

	if v.drafts != "" {
		written, err := writeDrafts(v.drafts, clusters)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "\nwrote %d draft(s) to %s\n", written, v.drafts)
		fmt.Fprintf(&b, "They are drafts on purpose: status draft, provenance asserted, so the\n")
		fmt.Fprintf(&b, "store's own gate keeps them out of every package until a person promotes one.\n")
	}

	_, err = io.WriteString(out, b.String())
	return err
}

func (v *gapsCmd) writeHeader(b *strings.Builder, window []facet.Facet, annotated int, store []gaps.Doc, want string) {
	fmt.Fprintf(b, "gaps  %d session(s)%s · %d annotated\n", len(window), scopeSuffix(want), annotated)

	// What the corpus cannot support is worth saying before what it suggests.
	if annotated == 0 {
		fmt.Fprintf(b, "\nNo session carries questions yet, so there is nothing to cluster.\n")
		fmt.Fprintf(b, "Questions are what a person or a model adds; see qm digest list --pending.\n")
		return
	}
	if annotated < len(window) {
		fmt.Fprintf(b, "  %d session(s) carry no questions and are invisible to this.\n", len(window)-annotated)
	}
	if len(store) == 0 {
		fmt.Fprintf(b, "  No store is installed here, so nothing can be called a discoverability\n")
		fmt.Fprintf(b, "  problem: there is nothing an agent could have failed to find.\n")
	}
}

func (v *gapsCmd) writeClusters(b *strings.Builder, clusters []gaps.Cluster) {
	for i, c := range clusters {
		if i >= v.top {
			fmt.Fprintf(b, "\n%d more below the cut.\n", len(clusters)-v.top)
			break
		}

		fmt.Fprintf(b, "\n%d. %s\n", i+1, c.Label)
		fmt.Fprintf(b, "   %s · seen %d× in %d repositor%s · %d tool call(s) spent\n",
			c.Kind, len(c.Occurrences), len(c.Repos), plural(len(c.Repos), "y", "ies"), c.ToolCalls)

		switch c.Kind {
		case gaps.KindDiscoverability:
			fmt.Fprintf(b, "   the store already answers this: %s\n", strings.Join(c.Answers, ", "))
			fmt.Fprintf(b, "   fix the description or the index entry, do not write another document\n")
		case gaps.KindRecoverable:
			fmt.Fprintf(b, "   answered from the repository itself, so a document would mostly copy the code\n")
		case gaps.KindContent:
			fmt.Fprintf(b, "   nothing in the store answers it, and it was not answered from the code\n")
		}

		for _, o := range c.Occurrences {
			fmt.Fprintf(b, "     %-16s %s  %s\n", o.Resolution, o.Session[:8], shorten(o.Question, 68))
		}
	}
}

// installedStore reads what the repository has materialized, so a question is
// checked against the knowledge that was actually on disk to be found.
//
// The retrievable tree rather than the whole bundle, because that is what an
// agent could have opened. A document the repository filtered out was never
// available, so failing to find it is not a discoverability problem.
func installedStore(dir string) []gaps.Doc {
	r, err := plan.Compute(dir)
	if err != nil {
		return nil
	}

	var out []gaps.Doc
	for _, body := range r.Knowledge {
		fm, err := doc.Parse(body)
		if err != nil || fm == nil {
			continue
		}
		id, _ := fm["id"].(string)
		if id == "" {
			continue
		}
		title, _ := fm["title"].(string)
		description, _ := fm["description"].(string)
		out = append(out, gaps.Doc{ID: id, Title: title, Description: description})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// writeDrafts emits one candidate document per content gap, with the evidence
// in the body. Evidence is what makes a weekly review fast enough that somebody
// actually does it.
func writeDrafts(dir string, clusters []gaps.Cluster) (int, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return 0, err
	}

	var written int
	for _, c := range clusters {
		if c.Kind != gaps.KindContent {
			continue
		}

		name := slug(c.Label)
		var b strings.Builder
		b.WriteString("---\n")
		fmt.Fprintf(&b, "id: drafts.%s\n", name)
		fmt.Fprintf(&b, "title: %s\n", title(c.Label))
		fmt.Fprintf(&b, "description: %s\n", c.Label)
		b.WriteString("type: concept\n")
		b.WriteString("status: draft\n")
		b.WriteString("provenance: asserted\n")
		b.WriteString("---\n\n")

		fmt.Fprintf(&b, "# %s\n\n", title(c.Label))
		b.WriteString("Nothing here yet. This is a question sessions kept having to answer, and\n")
		b.WriteString("the store had no answer for it.\n\n")

		b.WriteString("## Evidence\n\n")
		fmt.Fprintf(&b, "Seen %d time(s) across %d repositor%s, costing %d recorded tool call(s).\n\n",
			len(c.Occurrences), len(c.Repos), plural(len(c.Repos), "y", "ies"), c.ToolCalls)
		for _, o := range c.Occurrences {
			fmt.Fprintf(&b, "- `%s` in %s, answered by %s\n  > %s\n", o.Session[:8], o.Repo, o.Resolution, o.Question)
		}

		b.WriteString("\n## Before writing this\n\n")
		b.WriteString("Check that the answer is not recoverable from the code. A document that\n")
		b.WriteString("restates what a file already says is a bet that the document stays truer\n")
		b.WriteString("than the file, and that bet loses on a schedule. What belongs here is the\n")
		b.WriteString("part no file states: why it is this way, and what it rules out.\n")

		path := filepath.Join(dir, name+".md")
		if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func slug(s string) string {
	var b strings.Builder
	var dash bool
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 60 {
		out = strings.Trim(out[:60], "-")
	}
	if out == "" {
		out = "candidate"
	}
	return out
}

func title(s string) string {
	if s == "" {
		return "Candidate"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func scopeSuffix(want string) string {
	if want == "" {
		return ""
	}
	return " in " + want
}

// parseWindow turns a --since value into a cutoff. Days are the unit a person
// reaches for and the one Go's parser does not have.
func parseWindow(since string) (time.Time, error) {
	if since == "" {
		return time.Time{}, nil
	}
	s := strings.TrimSpace(since)
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err == nil && n >= 0 {
			return time.Now().UTC().AddDate(0, 0, -n), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read --since %q; use 30d or 24h", since)
}
