package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/digest"
	"github.com/mberwanger/quartermaster/internal/facet"
	"github.com/mberwanger/quartermaster/internal/repo"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/trace"
	"github.com/mberwanger/quartermaster/internal/transcript"
)

type digestCmd struct {
	cmd      *cobra.Command
	backfill bool
	all      bool
	dir      string
	since    string
	rerun    bool
	show     string
	dryRun   bool
	limit    int
}

func newDigestCmd() *digestCmd {
	v := &digestCmd{}

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Turn session transcripts into facet records",
		Long: `Read the transcripts of finished sessions and write one facet record each.

A facet record says what a session did, not what was said in it: how many tool
calls came before the first edit landed, which knowledge documents were opened
and whether opening one ended the search, and what the session produced. The
transcript stays where the harness put it. Only the record is derived, and it is
the only thing about a session ever meant to leave this machine.

Everything is derived structurally, so a digest costs nothing, is reproducible,
and can be re-run over the whole corpus whenever the derivation improves. The
fields that need a model to fill in, chiefly what a session was trying to
establish in its own words, stay empty rather than guessed.

By default it digests the sessions the SessionEnd hook spooled. Use --backfill
to sweep the transcripts already on disk instead, which is how a corpus exists
today rather than in three weeks.

A backfill sweeps this repository, including every worktree of it, since a
command run in one repository should not reach into sixteen. Records accumulate
in one place across runs either way, so the corpus still spans repositories;
--all sweeps them all at once.`,
		Example: "  qm digest --backfill --since 30d",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVar(&v.backfill, "backfill", false, "Sweep transcripts already on disk instead of the spool")
	cmd.Flags().BoolVar(&v.all, "all", false, "With --backfill, sweep every repository on this machine")
	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository to sweep")
	cmd.Flags().StringVar(&v.since, "since", "", "Only sessions this recent (e.g. 7d, 24h)")
	cmd.Flags().BoolVar(&v.rerun, "rerun", false, "Digest sessions that already have a record")
	cmd.Flags().StringVar(&v.show, "show", "", "Print one session's record and exit")
	cmd.Flags().BoolVar(&v.dryRun, "dry-run", false, "Report what would be digested, write nothing")
	cmd.Flags().IntVar(&v.limit, "limit", 0, "Stop after this many sessions (0 means all)")

	v.cmd = cmd
	return v
}

func (v *digestCmd) run(out io.Writer) error {
	if v.show != "" {
		return v.showOne(out)
	}

	since, err := v.window()
	if err != nil {
		return err
	}

	sources, err := v.sources(since)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return v.reportEmpty(out)
	}

	var digested, skipped, unreadable int
	for _, src := range sources {
		if v.limit > 0 && digested >= v.limit {
			break
		}
		if !v.rerun && src.ID != "" && facet.Have(src.ID) {
			skipped++
			continue
		}

		s, err := transcript.Read(src.Path)
		if err != nil && s == nil {
			unreadable++
			continue
		}
		if s.ID == "" {
			s.ID = src.ID
		}
		if s.ID == "" || len(s.Events) == 0 {
			unreadable++
			continue
		}
		if !since.IsZero() && !s.Ended.IsZero() && s.Ended.Before(since) {
			skipped++
			continue
		}

		f := digest.Digest(s, resolveRepo(s.CWD, src))
		if v.dryRun {
			digested++
			continue
		}
		if err := facet.Save(f); err != nil {
			return err
		}
		digested++
	}

	return v.report(out, digested, skipped, unreadable)
}

// source is one transcript to digest, and whatever the spool already knew about
// the session that produced it.
type source struct {
	Path     string
	ID       string
	CWD      string
	Repo     string
	Worktree string
	Bundles  []facet.Bundle
	Spooled  bool
}

// sources gathers the transcripts to digest, from the spool or from disk.
func (v *digestCmd) sources(since time.Time) ([]source, error) {
	if v.backfill {
		want := ""
		if !v.all {
			want = repo.Identity(v.dir)
		}
		return backfillSources(since, want)
	}

	sessions, err := trace.Load()
	if err != nil {
		return nil, err
	}

	var out []source
	for _, s := range sessions {
		if !since.IsZero() && s.EndedAt.Before(since) {
			continue
		}
		bundles := make([]facet.Bundle, 0, len(s.Bundles))
		for _, b := range s.Bundles {
			bundles = append(bundles, facet.Bundle{Source: b.Source, Digest: b.Digest})
		}
		out = append(out, source{
			Path: s.TranscriptPath, ID: s.ID, Repo: s.Repo,
			Worktree: s.Worktree, Bundles: bundles, Spooled: true,
		})
	}
	return out, nil
}

// backfillSources finds the transcripts the harness has already written, keeping
// those that belong to the wanted repository, or all of them when want is empty.
//
// Scoping happens on resolved identity rather than on the harness's directory
// name. The harness keys its project directories by working path, so three
// worktrees of one repository are three project directories, and matching on the
// name would sweep one third of the sessions that actually belong to the
// repository being asked about.
//
// Only the sweep is scoped. Records accumulate in one directory across runs, so
// a repository-scoped sweep still builds the cross-repository corpus that makes
// a recurring question worth acting on.
func backfillSources(since time.Time, want string) ([]source, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".claude", "projects")

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []source
	for _, project := range entries {
		if !project.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, project.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			if !since.IsZero() && info.ModTime().Before(since) {
				continue
			}

			path := filepath.Join(root, project.Name(), f.Name())
			id, cwd, err := transcript.Head(path)
			if err != nil {
				continue
			}
			if id == "" {
				id = strings.TrimSuffix(f.Name(), ".jsonl")
			}
			if want != "" && (cwd == "" || repo.Identity(cwd) != want) {
				continue
			}

			out = append(out, source{Path: path, ID: id, CWD: cwd})
		}
	}

	// Newest first, so a --limit run digests the sessions most worth having.
	sort.Slice(out, func(i, j int) bool { return out[i].Path > out[j].Path })
	return out, nil
}

// resolveRepo works out which repository a session ran in.
//
// A spooled session already knows, because the hook resolved it while the
// repository was in front of it. A backfilled one is resolved from the
// transcript's own cwd, which is the only reason backfill can attribute
// anything: the harness's directory name is a mangled path and says nothing
// reliable about which repository it meant.
func resolveRepo(cwd string, src source) digest.Repo {
	if src.Repo != "" {
		return digest.Repo{Identity: src.Repo, Worktree: src.Worktree, Bundles: src.Bundles}
	}
	if cwd == "" {
		return digest.Repo{}
	}

	r := digest.Repo{Identity: repo.Identity(cwd), Worktree: cwd}
	if root, ok := repoRootAt(cwd); ok {
		if s, err := state.Load(root); err == nil {
			for _, b := range s.Bundles {
				r.Bundles = append(r.Bundles, facet.Bundle{Source: b.Source, Digest: b.Digest})
			}
		}
	}
	return r
}

// window turns --since into a cutoff.
func (v *digestCmd) window() (time.Time, error) {
	if v.since == "" {
		return time.Time{}, nil
	}

	s := strings.TrimSpace(v.since)
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	// Days are the unit a person reaches for here and the one Go's parser does
	// not have.
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err == nil && n >= 0 {
			return time.Now().UTC().AddDate(0, 0, -n), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot read --since %q; use 7d or 24h", v.since)
}

func (v *digestCmd) reportEmpty(out io.Writer) error {
	if v.backfill {
		if v.all {
			_, err := fmt.Fprintf(out, "no transcripts found on disk\n")
			return err
		}
		_, err := fmt.Fprintf(out, "no transcripts on disk for %s; --all sweeps every repository\n", repo.Identity(v.dir))
		return err
	}
	_, err := fmt.Fprintf(out, `nothing spooled yet.

Sessions are spooled by the SessionEnd hook, which qm init installs into a
repository's .claude/settings.json. To work from the transcripts already on
disk instead, run qm digest --backfill.
`)
	return err
}

func (v *digestCmd) report(out io.Writer, digested, skipped, unreadable int) error {
	var b strings.Builder

	verb := "digested"
	if v.dryRun {
		verb = "would digest"
	}
	fmt.Fprintf(&b, "%s %d session(s)", verb, digested)
	if skipped > 0 {
		fmt.Fprintf(&b, " · skipped %d already digested", skipped)
	}
	if unreadable > 0 {
		fmt.Fprintf(&b, " · %d unreadable", unreadable)
	}
	b.WriteString("\n")

	if digested > 0 && !v.dryRun {
		dir, err := facet.Dir()
		if err == nil {
			fmt.Fprintf(&b, "records in %s\n", dir)
		}
		if err := summarize(&b); err != nil {
			return err
		}
	}

	_, err := io.WriteString(out, b.String())
	return err
}

// summarize reports what the corpus now says, which is the point of having one.
func summarize(b *strings.Builder) error {
	facets, err := facet.LoadAll()
	if err != nil || len(facets) == 0 {
		return err
	}

	repos := map[string]int{}
	var spans []int
	var reads, withBundles int
	for _, f := range facets {
		repos[f.Repo]++
		if f.DiscoverySpan != nil {
			spans = append(spans, *f.DiscoverySpan)
		}
		reads += len(f.StoreReads)
		if len(f.Bundles) > 0 {
			withBundles++
		}
	}

	fmt.Fprintf(b, "\ncorpus  %d session(s) · %d repositor%s\n",
		len(facets), len(repos), plural(len(repos), "y", "ies"))

	if len(spans) > 0 {
		sort.Ints(spans)
		fmt.Fprintf(b, "  discovery span   median %d tool calls before the first edit (%d session(s) measured)\n",
			spans[len(spans)/2], len(spans))
	}
	fmt.Fprintf(b, "  store reads      %d\n", reads)

	// The limit worth stating plainly, because it decides what the corpus can
	// be asked. A record with no bundle cannot say whether a bundle helped.
	if withBundles < len(facets) {
		fmt.Fprintf(b, "  %d of %d session(s) name no bundle, so they can locate a gap but cannot\n",
			len(facets)-withBundles, len(facets))
		fmt.Fprintf(b, "  measure whether one helped.\n")
	}
	return nil
}

func (v *digestCmd) showOne(out io.Writer) error {
	f, err := facet.Load(v.show)
	if err != nil {
		return fmt.Errorf("no record for session %q; digest it first", v.show)
	}

	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\n", raw)
	return err
}
