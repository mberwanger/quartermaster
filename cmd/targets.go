package cmd

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/target"
)

type targetsCmd struct {
	cmd *cobra.Command
	dir string
}

func newTargetsCmd() *targetsCmd {
	v := &targetsCmd{}

	cmd := &cobra.Command{
		Use:   "targets",
		Short: "List the known harness targets",
		Long: `List every target renderer this build knows. When a manifest is present,
the ones it configures are marked.`,
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

func (v *targetsCmd) run(out io.Writer) error {
	// A manifest is optional here: qm targets is useful outside a consumer repo
	// just to see what renderers exist.
	var configured []string
	if m, err := manifest.Load(v.dir); err == nil {
		configured = m.Targets
	}

	var b strings.Builder
	for _, name := range target.Names() {
		mark := " "
		if slices.Contains(configured, name) {
			mark = "✓"
		}
		fmt.Fprintf(&b, "%s %s\n", mark, name)
	}

	_, err := io.WriteString(out, b.String())
	return err
}
