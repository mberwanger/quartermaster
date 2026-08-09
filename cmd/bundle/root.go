// Package bundle holds the producer commands: the ones that scaffold, index,
// validate, compile, explain, and publish a bundle.
//
// They live apart from the consumer commands because they are run somewhere
// else, by someone else. These run in a store repository, where knowledge is
// authored; the consumer commands run in every repository that draws from one.
// Keeping them in separate packages makes that seam something the compiler
// holds rather than something a reader has to infer from a file listing.
package bundle

import (
	"github.com/spf13/cobra"
)

// New builds the bundle command group. It is this package's only export: the
// subcommands are reachable through it and nowhere else.
func New() *cobra.Command {
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
		newInitCmd().cmd,
		newBuildCmd().cmd,
		newValidateCmd().cmd,
		newExplainCmd().cmd,
		newIndexCmd().cmd,
		newPublishCmd().cmd,
	)

	return cmd
}
