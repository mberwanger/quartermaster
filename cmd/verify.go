package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/target"
)

type verifyCmd struct {
	cmd *cobra.Command
	dir string
}

func newVerifyCmd() *verifyCmd {
	v := &verifyCmd{}

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Fail if materialized content drifted from the manifest",
		Long: `Recompute what a sync would produce and compare it to the working tree,
without writing anything. It fails when a generated file is missing, was edited
by hand, or is stale from a previous sync that should have pruned it.

This is the CI guard against hand-edits to generated content and against a
working tree that a branch switch left out of date.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory to verify")

	v.cmd = cmd
	return v
}

func (v *verifyCmd) run(out io.Writer) error {
	r, err := plan.Compute(v.dir)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(v.dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	var drift []string

	// Every expected file must exist and match byte-for-byte.
	for _, p := range keys(r.Outputs) {
		got, err := root.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				drift = append(drift, "missing: "+p)
				continue
			}
			return err
		}
		if !bytes.Equal(got, r.Outputs[p]) {
			drift = append(drift, "changed: "+p)
		}
	}

	// Each managed block's region must exist and match. The file around it is
	// the author's, so only the marked region is compared.
	for _, blk := range r.Blocks {
		existing, err := root.ReadFile(blk.Path)
		if err != nil {
			if os.IsNotExist(err) {
				drift = append(drift, "missing block: "+blk.Path)
				continue
			}
			return err
		}
		got, ok := target.BlockRegion(string(existing), blk.Marker)
		if !ok {
			drift = append(drift, "missing block: "+blk.Path)
			continue
		}
		if got != strings.Trim(blk.Body, "\n") {
			drift = append(drift, "changed block: "+blk.Path)
		}
	}

	// A file the last sync produced that this plan does not, still on disk, is
	// stale: a prune that never ran.
	prior, err := state.Load(v.dir)
	if err != nil {
		return err
	}
	for _, p := range prior.Files {
		if _, expected := r.Outputs[p]; expected {
			continue
		}
		if _, err := root.Stat(p); err == nil {
			drift = append(drift, "stale: "+p)
		}
	}

	sort.Strings(drift)

	if len(drift) > 0 {
		var b strings.Builder
		for _, d := range drift {
			fmt.Fprintf(&b, "%s\n", d)
		}
		fmt.Fprintf(&b, "\n%d file(s) drifted; run qm sync\n", len(drift))
		_, _ = io.WriteString(out, b.String())
		return &exitError{err: errors.New("verify failed"), code: 1}
	}

	fmt.Fprintf(out, "ok: %d file(s) match the manifest\n", len(r.Outputs))
	return nil
}
