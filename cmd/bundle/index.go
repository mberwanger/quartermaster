package bundle

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/index"
)

type indexCmd struct {
	cmd   *cobra.Command
	root  string
	check bool
	strip bool
}

func newIndexCmd() *indexCmd {
	v := &indexCmd{}

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Regenerate the directory listings",
		Long: `Regenerate the index.md listing in every directory that holds documents,
so a reader can see what a directory contains without opening all of it.

Only a marked region of each file is owned here. The prose above it explains why
the directory exists, which no generator can write, and is left alone.

A skill gets no listing: it is one unit an agent loads whole, so a list of its
parts describes nothing a reader needs.

With --check nothing is written and a stale listing fails, which is what a pull
request gate wants.

A listing whose directory has been emptied is reported but never touched. It is
not regenerated, because nothing asks for it any more, and not deleted, because
the prose above the generated region is not this tool's to remove.

A store with listings: build in bundle.yaml keeps only the prose committed and
has the bundle write each listing when it is built, so two pull requests that
each add a document never conflict over one. There this command writes nothing:
it prints the listings the bundle will carry, --check fails on any listing still
committed, and --strip removes them, which is the one-time move to build.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the store")
	cmd.Flags().BoolVar(&v.check, "check", false, "Fail when a listing is out of date, writing nothing")
	cmd.Flags().BoolVar(&v.strip, "strip", false, "Remove committed listings, for a store with listings: build")
	cmd.MarkFlagsMutuallyExclusive("check", "strip")

	v.cmd = cmd
	return v
}

func (v *indexCmd) run(out io.Writer) error {
	cfg, err := config.Load(v.root)
	if err != nil {
		return err
	}
	if v.strip && !cfg.ListingsAtBuild() {
		return errors.New("--strip removes committed listings, which only a store with listings: build in bundle.yaml does without; set it first")
	}
	if cfg.ListingsAtBuild() {
		return v.runAtBuild(out, cfg)
	}

	changed, err := index.Sync(v.root, cfg, !v.check)
	if err != nil {
		return err
	}

	orphans, err := index.Orphans(v.root, cfg)
	if err != nil {
		return err
	}

	var b strings.Builder
	if v.check {
		for _, p := range changed {
			fmt.Fprintf(&b, "%s: out of date\n", p)
		}
		if len(changed) > 0 {
			fmt.Fprintf(&b, "\n%d listing(s) out of date; run qm bundle index\n", len(changed))
			writeOrphans(&b, orphans)
			_, _ = io.WriteString(out, b.String())
			// A plain error, not an explicit code: the root command already
			// exits 1 on any error, which is the signal --check exists to give
			// CI. Reaching for exitError here would only restate the default.
			return errors.New("listings are stale")
		}
		fmt.Fprintf(&b, "ok: every listing is current\n")
		writeOrphans(&b, orphans)
		_, err = io.WriteString(out, b.String())
		return err
	}

	for _, p := range changed {
		fmt.Fprintf(&b, "wrote %s\n", p)
	}
	if len(changed) == 0 {
		fmt.Fprintf(&b, "ok: every listing was already current\n")
	}
	writeOrphans(&b, orphans)
	_, err = io.WriteString(out, b.String())
	return err
}

// runAtBuild is the command for a store whose bundle writes the listings.
func (v *indexCmd) runAtBuild(out io.Writer, cfg *config.Config) error {
	var b strings.Builder
	switch {
	case v.strip:
		stripped, err := index.Strip(v.root, true)
		if err != nil {
			return err
		}
		for _, p := range stripped {
			fmt.Fprintf(&b, "stripped %s\n", p)
		}
		fmt.Fprintf(&b, "ok: %d committed listing(s) removed; the bundle writes them at build\n", len(stripped))

	case v.check:
		committed, err := index.Committed(v.root)
		if err != nil {
			return err
		}
		for _, p := range committed {
			fmt.Fprintf(&b, "%s: carries a generated listing\n", p)
		}
		if len(committed) > 0 {
			fmt.Fprintf(&b, "\n%d listing(s) committed; this store writes them at build, run qm bundle index --strip\n", len(committed))
			_, _ = io.WriteString(out, b.String())
			return errors.New("listings are committed")
		}
		fmt.Fprintf(&b, "ok: no listing is committed; the bundle writes them at build\n")

	default:
		listings, err := index.Build(v.root, cfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "listings: build; nothing is written to the store. The bundle will carry:\n")
		for _, l := range listings {
			fmt.Fprintf(&b, "\n== %s\n%s", l.Path, l.Region)
			if !strings.HasSuffix(l.Region, "\n") {
				b.WriteString("\n")
			}
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// writeOrphans reports listings nothing regenerates, and deliberately does not
// affect the exit code.
//
// --check fails on a stale listing because `qm bundle index` fixes it, and a
// gate that fails with a working remedy is useful. There is no remedy to name
// here: the command that would run next leaves these files alone by design. So
// this is said wherever it is seen soonest, which is the local run right after
// documents were moved, and CI is not failed over a decision only a person can
// make.
func writeOrphans(b *strings.Builder, orphans []index.Orphan) {
	if len(orphans) == 0 {
		return
	}

	fmt.Fprintf(b, "\n%d listing(s) no longer regenerated:\n", len(orphans))
	for _, o := range orphans {
		fmt.Fprintf(b, "  %s: lists %s, but the directory holds none\n",
			o.Path, documents(o.Entries))
	}
	fmt.Fprintf(b, "\nEdit or delete these by hand. Nothing here rewrites them, because the\n")
	fmt.Fprintf(b, "prose above the generated region is yours and a generator cannot restore it.\n")
}

func documents(n int) string {
	if n == 1 {
		return "1 document"
	}
	return fmt.Sprintf("%d documents", n)
}
