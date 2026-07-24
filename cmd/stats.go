package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/usage"
)

type statsCmd struct {
	cmd  *cobra.Command
	dir  string
	days int
	top  int
}

func newStatsCmd() *statsCmd {
	v := &statsCmd{}

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Report which knowledge was actually read",
		Long: `Report which documents agents opened, across every repository that records
usage.

The log is global, because promotion is a cross-repository decision: a document
opened once in each of twelve repositories is a stronger candidate than one
opened twelve times in a single repository.

Treat the numbers as a shortlist rather than a verdict. They reliably identify
documents nobody ever opens; they cannot tell a document that helped from one an
agent opened and discarded, and what is already always-loaded shapes what an
agent thinks to look for.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository to cross-reference against")
	cmd.Flags().IntVar(&v.days, "days", 0, "Only count the last N days (0 means all)")
	cmd.Flags().IntVar(&v.top, "top", 10, "How many documents to list")

	v.cmd = cmd
	return v
}

func (v *statsCmd) run(out io.Writer) error {
	events, err := usage.Load()
	if err != nil {
		return err
	}

	window := "all time"
	if v.days > 0 {
		events = usage.Since(events, time.Now().UTC().AddDate(0, 0, -v.days))
		window = fmt.Sprintf("last %d days", v.days)
	}

	docs := usage.Aggregate(events)
	repos := usage.Repos(events)

	var b strings.Builder
	fmt.Fprintf(&b, "usage  %s · %d event(s) · %d document(s) · %d repositor%s\n",
		window, len(events), len(docs), len(repos), plural(len(repos), "y", "ies"))

	if len(events) == 0 {
		fmt.Fprintf(&b, "\nNothing recorded yet. Usage is written by an agent-harness hook on\n")
		fmt.Fprintf(&b, "reads under %s; see the README for wiring it up.\n", plan.KnowledgeDir)
		_, err := io.WriteString(out, b.String())
		return err
	}

	v.writeMostOpened(&b, docs)

	// The repository sections need to know what is materialized here. A missing
	// or unresolvable manifest is not an error: the global counts still stand.
	if r, err := plan.Compute(v.dir); err == nil {
		v.writeRepoSections(&b, docs, repos, r)
	}

	fmt.Fprintf(&b, "\nRead counts show what an agent chose to open. Nothing here proposes a\n")
	fmt.Fprintf(&b, "demotion: an always-loaded rule is never opened, so its absence from this\n")
	fmt.Fprintf(&b, "log means nothing at all.\n")

	_, err = io.WriteString(out, b.String())
	return err
}

func (v *statsCmd) writeMostOpened(b *strings.Builder, docs []usage.Doc) {
	fmt.Fprintf(b, "\nmost opened\n")
	for i, d := range docs {
		if i >= v.top {
			break
		}
		fmt.Fprintf(b, "  %4d  %-40s %d repo%s   last %s\n",
			d.Opens, d.ID, len(d.Repos), plural(len(d.Repos), "", "s"),
			d.Last.Local().Format("2006-01-02"))
	}
}

func (v *statsCmd) writeRepoSections(b *strings.Builder, docs []usage.Doc, repos []string, r *plan.Result) {
	opened := make(map[string]usage.Doc, len(docs))
	for _, d := range docs {
		opened[d.ID] = d
	}

	onDisk := knowledgeIDs(r)
	isRule := make(map[string]bool, len(r.Docs))
	for _, d := range r.Docs {
		isRule[d.ID] = true
	}

	// Only documents an agent had to choose to open can be judged by silence. A
	// rule is delivered by the harness whether or not anyone reads the copy in
	// the knowledge tree, so its absence from the log means nothing and it is
	// left out rather than listed as unused.
	var never []string
	for _, id := range onDisk {
		if isRule[id] {
			continue
		}
		if _, ok := opened[id]; !ok {
			never = append(never, id)
		}
	}
	if len(never) > 0 {
		fmt.Fprintf(b, "\nretrievable here, never opened\n")
		for _, id := range never {
			fmt.Fprintf(b, "  %s\n", id)
		}
		fmt.Fprintf(b, "  A document nobody opens is either described badly or not needed;\n")
		fmt.Fprintf(b, "  description is all an agent sees when it decides.\n")
	}

	// A document that is merely on disk, yet agents keep opening it across most
	// repositories, is asking to become a rule. A strict majority, so that one
	// repository out of two is not mistaken for a trend.
	var promote []string
	for _, id := range onDisk {
		d, ok := opened[id]
		if !ok || isRule[id] {
			continue
		}
		if len(repos) > 0 && len(d.Repos)*2 > len(repos) {
			promote = append(promote, fmt.Sprintf("%-40s %d/%d repos", id, len(d.Repos), len(repos)))
		}
	}
	if len(promote) > 0 {
		fmt.Fprintf(b, "\npromotion candidates  (on disk here, opened in most repositories)\n")
		for _, line := range promote {
			fmt.Fprintf(b, "  %s\n", line)
		}
	}

	if r.Budget.ResidentBytes > 0 {
		fmt.Fprintf(b, "\nresident  %d / %d B budget\n", r.ResidentBytes, r.Budget.ResidentBytes)
	} else {
		fmt.Fprintf(b, "\nresident  %d B\n", r.ResidentBytes)
	}
}

// knowledgeIDs reads the id out of each materialized document's frontmatter,
// which is where it already lives, so nothing has to be kept in step with it.
func knowledgeIDs(r *plan.Result) []string {
	var ids []string
	for _, body := range r.Knowledge {
		fm, err := doc.Parse(body)
		if err != nil || fm == nil {
			continue
		}
		if id, _ := fm["id"].(string); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
