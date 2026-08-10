package bundle

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	bundlepkg "github.com/mberwanger/quartermaster/internal/bundle"
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
exists. Then compile packages, links, and agent permissions in memory, through
the same build the bundle command runs.

This is the pull-request gate. It emits nothing and fails when any document is
malformed, so a reviewer only spends attention on whether the content is true,
not on whether it is well-formed.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the store")

	v.cmd = cmd
	return v
}

func (v *validateCmd) run(out io.Writer) error {
	// Validate compiles the artifact and throws it away: the check that a store
	// is sound is that it builds, so anything less than a full compile would let
	// validate pass a store that build then refuses.
	compiled, err := bundlepkg.Compile(v.root, bundlepkg.Source{})
	if err != nil {
		// A bare return lets root.go's single "Error: %s" path render this, the
		// same way build and bundle init already do, rather than validate
		// printing its own findings block and then a second, unrelated
		// "validation failed" line on top of it.
		return err
	}

	var report strings.Builder
	fmt.Fprintf(&report, "ok: %d doc(s) checked, bundle compiles\n", compiled.Meta.Checked)
	_, err = io.WriteString(out, report.String())
	return err
}
