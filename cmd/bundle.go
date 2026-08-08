package cmd

import (
	"github.com/spf13/cobra"
)

// bundleCmd groups the producer commands: the ones that scaffold, index,
// validate, compile, explain, and publish a bundle. They run in a store
// repository, a different
// place and a different audience from the consumer commands, which stay at the
// top level as the tool's primary surface.
type bundleCmd struct {
	cmd *cobra.Command
}

func newBundleCmd() *bundleCmd {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Create, check, build, and publish knowledge bundles",
		Long: `Producer commands, run in a store repository.

They scaffold and index a source tree, verify every compilation check, build and
publish the artifact, and explain why a document does or does not become a rule.
Consumer commands — init, sync, verify, status — stay at the top level.`,
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
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
