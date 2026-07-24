package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/version"
)

func newVersionCmd(ver version.Version) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), ver.String())
			return err
		},
	}
}
