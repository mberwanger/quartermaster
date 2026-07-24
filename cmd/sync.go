package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/target"
)

type syncCmd struct {
	cmd   *cobra.Command
	dir   string
	quiet bool
}

func newSyncCmd() *syncCmd {
	v := &syncCmd{}

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Resolve, materialize, and prune",
		Long: `Resolve the bundles a repository declares, materialize the selected
rulesets into each target harness, and prune any file a previous sync produced
that this one does not.

The retrievable knowledge tree is written whole under .quartermaster/knowledge.
Each ruleset selection becomes rule files under the harness path, resident when
unscoped and path-scoped otherwise. Generated output is gitignored and
reproducible from the manifest, so it is safe to delete and regenerate.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory to sync")
	cmd.Flags().BoolVar(&v.quiet, "quiet", false, "Suppress the summary output")

	v.cmd = cmd
	return v
}

func (v *syncCmd) run(out io.Writer) error {
	return syncRepo(v.dir, out, v.quiet)
}

// syncRepo resolves the manifest in dir, materializes it, prunes what a prior
// sync produced but this one does not, and records the new state. It is shared
// by qm sync and qm init.
func syncRepo(dir string, out io.Writer, quiet bool) error {
	r, err := plan.Compute(dir)
	if err != nil {
		return err
	}

	prior, err := state.Load(dir)
	if err != nil {
		return err
	}

	pruned, err := materialize(dir, r.Outputs, prior.Files)
	if err != nil {
		return err
	}

	if err := applyBlocks(dir, r.Blocks); err != nil {
		return err
	}

	next := &state.State{Bundles: r.Bundles, Targets: r.Targets, Files: keys(r.Outputs)}
	if err := state.Save(dir, next); err != nil {
		return err
	}

	if !quiet {
		writeReport(out, r, pruned)
	}
	return nil
}

func writeReport(out io.Writer, r *plan.Result, pruned int) {
	var resident, scoped int
	for _, d := range r.Docs {
		if len(d.Scope) == 0 {
			resident++
		} else {
			scoped++
		}
	}

	var b strings.Builder
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "! %s\n", w)
	}
	for _, bd := range r.Bundles {
		fmt.Fprintf(&b, "✓ %s  %s\n", bd.Source, bd.Digest)
		fmt.Fprintf(&b, "  rulesets  %s\n", strings.Join(bd.Rulesets, ", "))
	}
	fmt.Fprintf(&b, "  → rules       %d resident, %d scoped  (%d B resident)\n", resident, scoped, r.ResidentBytes)
	if len(r.Skills) > 0 {
		var assets int
		for _, s := range r.Skills {
			assets += len(s.Assets)
		}
		fmt.Fprintf(&b, "  → skills      %d on demand (%d asset file(s))\n", len(r.Skills), assets)
	}
	fmt.Fprintf(&b, "  → knowledge   %d docs retrievable\n", len(r.Knowledge))
	for _, blk := range r.Blocks {
		fmt.Fprintf(&b, "  → %-11s managed block updated\n", blk.Path)
	}
	fmt.Fprintf(&b, "  synced %d file(s), pruned %d\n", len(r.Outputs), pruned)

	_, _ = io.WriteString(out, b.String())
}

// materialize writes every output file and deletes every prior file this sync no
// longer produces, all sandboxed to the repository directory. It returns the
// number of files pruned.
func materialize(dir string, outputs map[string][]byte, prior []string) (int, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.Close() }()

	if err := guardHandWritten(r, outputs, prior); err != nil {
		return 0, err
	}

	for _, p := range keys(outputs) {
		if err := mkdirAll(r, path.Dir(p)); err != nil {
			return 0, err
		}
		if err := r.WriteFile(p, outputs[p], 0o600); err != nil {
			return 0, err
		}
	}

	var pruned int
	for _, p := range prior {
		if _, keep := outputs[p]; keep {
			continue
		}
		if err := r.Remove(p); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		pruned++
	}
	return pruned, nil
}

// applyBlocks splices each managed block into its committed file, preserving
// whatever the author wrote outside the markers. A block's file is never pruned;
// it belongs to the author, and only the marked region is ours.
func applyBlocks(dir string, blocks []target.Block) error {
	if len(blocks) == 0 {
		return nil
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, blk := range blocks {
		existing, err := r.ReadFile(blk.Path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		next := target.ApplyBlock(string(existing), blk.Marker, blk.Body)
		if err := mkdirAll(r, path.Dir(blk.Path)); err != nil {
			return err
		}
		if err := r.WriteFile(blk.Path, []byte(next), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// guardHandWritten refuses to overwrite a file that Quartermaster did not write.
//
// Rules live under a directory this tool owns whole, so they cannot collide. A
// skill cannot: the harness reads the directory name as the skill's identity, so
// a generated skill sits directly beside one somebody wrote. Silently replacing
// that would be the worst kind of failure, because the author would only notice
// when their skill stopped saying what they wrote.
//
// The check runs before anything is written, so a conflict leaves the tree
// untouched rather than half-synced.
func guardHandWritten(r *os.Root, outputs map[string][]byte, prior []string) error {
	ours := make(map[string]bool, len(prior))
	for _, p := range prior {
		ours[p] = true
	}

	var clashes []string
	for _, p := range keys(outputs) {
		if !strings.HasSuffix(p, "/SKILL.md") || ours[p] {
			continue
		}
		existing, err := r.ReadFile(p)
		if err != nil {
			continue // absent, or unreadable and the write will say so
		}
		if !bytes.Contains(existing, []byte(target.GeneratedMarker)) {
			clashes = append(clashes, path.Dir(p))
		}
	}

	if len(clashes) > 0 {
		return fmt.Errorf("refusing to overwrite hand-written skill(s) at %s; rename the skill in the store, or remove the directory to let it be managed",
			strings.Join(clashes, ", "))
	}
	return nil
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mkdirAll creates dir and its parents inside the root.
func mkdirAll(r *os.Root, dir string) error {
	if dir == "." || dir == "/" || dir == "" {
		return nil
	}
	var built string
	for part := range strings.SplitSeq(dir, "/") {
		if part == "" || part == "." {
			continue
		}
		built = path.Join(built, part)
		if err := r.Mkdir(built, 0o750); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}
