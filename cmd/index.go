package cmd

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
request gate wants.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the knowledge store")
	cmd.Flags().BoolVar(&v.check, "check", false, "Fail when a listing is out of date, writing nothing")

	v.cmd = cmd
	return v
}

func (v *indexCmd) run(out io.Writer) error {
	cfg, err := config.Load(v.root)
	if err != nil {
		return err
	}

	changed, err := index.Sync(v.root, cfg, !v.check)
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
			_, _ = io.WriteString(out, b.String())
			return &exitError{err: errors.New("listings are stale"), code: 1}
		}
		fmt.Fprintf(&b, "ok: every listing is current\n")
		_, err = io.WriteString(out, b.String())
		return err
	}

	for _, p := range changed {
		fmt.Fprintf(&b, "wrote %s\n", p)
	}
	if len(changed) == 0 {
		fmt.Fprintf(&b, "ok: every listing was already current\n")
	}
	_, err = io.WriteString(out, b.String())
	return err
}
