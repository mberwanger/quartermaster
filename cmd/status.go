package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/plan"
)

type statusCmd struct {
	cmd *cobra.Command
	dir string
}

func newStatusCmd() *statusCmd {
	v := &statusCmd{}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show resolved bundles, rulesets, and the resident budget",
		Long: `Resolve the manifest and report what it produces: the bundles and their
digests, the rulesets applied, the targets, and the resident set against its
budget. It writes nothing.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory")

	v.cmd = cmd
	return v
}

func (v *statusCmd) run(out io.Writer) error {
	r, err := plan.Compute(v.dir)
	if err != nil {
		return err
	}

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

	fmt.Fprintf(&b, "targets:  %s\n", strings.Join(r.Targets, ", "))
	fmt.Fprintln(&b, "bundles:")
	for _, bd := range r.Bundles {
		fmt.Fprintf(&b, "  %s\n", bd.Source)
		fmt.Fprintf(&b, "    digest    %s\n", bd.Digest)
		fmt.Fprintf(&b, "    rulesets  %s\n", strings.Join(bd.Rulesets, ", "))
		if len(bd.Knowledge) > 0 {
			// A filtered tree is a partial one; say so rather than let the doc
			// count read as the whole store.
			fmt.Fprintf(&b, "    knowledge filtered on %s\n", strings.Join(bd.Knowledge, ", "))
		}
	}

	fmt.Fprintf(&b, "rules:     %d resident, %d scoped\n", resident, scoped)
	fmt.Fprintf(&b, "knowledge: %d docs retrievable\n", len(r.Knowledge))

	if r.Budget.ResidentBytes > 0 {
		fmt.Fprintf(&b, "resident:  %d / %d B budget\n", r.ResidentBytes, r.Budget.ResidentBytes)
	} else {
		fmt.Fprintf(&b, "resident:  %d B (no budget declared)\n", r.ResidentBytes)
	}

	_, err = io.WriteString(out, b.String())
	return err
}
