package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/validate"
)

type validateCmd struct {
	cmd  *cobra.Command
	root string
}

func newValidateCmd() *validateCmd {
	v := &validateCmd{}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run build checks without emitting a bundle",
		Long: `Validate a store's machine-checkable rules: frontmatter matches the
schema, every id is unique, and every supersede link resolves to a doc that
exists.

This is the pull-request gate. It emits nothing and fails when any document is
malformed, so a reviewer only spends attention on whether the content is true,
not on whether it is well-formed.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(v.root)
			if err != nil {
				return err
			}

			res, err := validate.Run(v.root, cfg)
			if err != nil {
				return err
			}

			var report strings.Builder
			for _, f := range res.Findings {
				fmt.Fprintf(&report, "%s: %s\n", f.Path, f.Message)
			}

			if !res.OK() {
				fmt.Fprintf(&report, "\n%d finding(s) across %d checked doc(s)\n",
					len(res.Findings), res.Checked)
				_, _ = io.WriteString(cmd.OutOrStderr(), report.String())
				return &exitError{err: errors.New("validation failed"), code: 1}
			}

			fmt.Fprintf(&report, "ok: %d doc(s) checked, no findings\n", res.Checked)
			_, err = io.WriteString(cmd.OutOrStdout(), report.String())
			return err
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the knowledge store")

	v.cmd = cmd
	return v
}
