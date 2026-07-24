package cmd

import (
	"github.com/spf13/cobra"
)

// bundleCmd groups the producer commands: the ones that compile, check, explain,
// and (eventually) publish a bundle. They run in a store repository, a different
// place and a different audience from the consumer commands, which stay at the
// top level as the tool's primary surface.
type bundleCmd struct {
	cmd *cobra.Command
}

func newBundleCmd() *bundleCmd {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Compile, check, and explain knowledge bundles",
		Long: `Producer commands, run in a store repository.

They compile a source tree into a bundle, run the same checks without emitting,
and explain why a document does or does not become a rule. Consumer commands —
sync, verify, status — stay at the top level.`,
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
	}

	cmd.AddCommand(
		newBundleInitCmd().cmd,
		newBuildCmd().cmd,
		newValidateCmd().cmd,
		newExplainCmd().cmd,
		newIndexCmd().cmd,
		newPublishCmd().cmd,
	)

	return &bundleCmd{cmd: cmd}
}
