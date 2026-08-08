package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/preflight"
)

type validateCmd struct {
	cmd  *cobra.Command
	root string
}

func newValidateCmd() *validateCmd {
	v := &validateCmd{}

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run every compilation check without emitting a bundle",
		Long: `Validate a store's machine-checkable rules: frontmatter matches the
schema, every id is unique, and every supersede link resolves to a doc that
exists. Then compile packages, links, and agent permissions in memory using the
same preflight as build.

This is the pull-request gate. It emits nothing and fails when any document is
malformed, so a reviewer only spends attention on whether the content is true,
not on whether it is well-formed.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			preflightResult, err := preflight.Run(preflight.Options{Root: v.root})
			if err != nil {
				var validationErr *preflight.ValidationError
				if errors.As(err, &validationErr) {
					_, _ = io.WriteString(cmd.OutOrStderr(), validationErr.Error()+"\n")
					return &exitError{err: errors.New("validation failed"), code: 1}
				}
				return err
			}

			var report strings.Builder
			fmt.Fprintf(&report, "ok: %d doc(s) checked, bundle compiles\n",
				preflightResult.Validation.Checked)
			_, err = io.WriteString(cmd.OutOrStdout(), report.String())
			return err
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the knowledge store")

	v.cmd = cmd
	return v
}
