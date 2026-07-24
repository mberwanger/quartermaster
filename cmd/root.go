package cmd

import (
	"errors"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/output"
	"github.com/mberwanger/quartermaster/internal/version"
)

type rootCmd struct {
	cmd  *cobra.Command
	exit func(int)

	verbose      bool
	outputFormat string
}

// Execute builds the root command and runs it with the provided arguments.
func Execute(ver version.Version, exit func(int), args []string) {
	newRootCmd(ver, exit).Execute(args)
}

func (cmd *rootCmd) Execute(args []string) {
	cmd.cmd.SetArgs(args)

	if err := cmd.cmd.Execute(); err != nil {
		code := 1
		if eerr, ok := errors.AsType[*exitError](err); ok {
			code = eerr.code
		}
		output.Writef(os.Stderr, "Error: %s\n", formatError(err))
		cmd.exit(code)
	}
}

func newRootCmd(ver version.Version, exit func(int)) *rootCmd {
	root := &rootCmd{
		exit: exit,
	}

	cmd := &cobra.Command{
		Use:   "qm",
		Short: "Materialize a knowledge bundle into a repository",
		Long: `Quartermaster pulls a versioned knowledge bundle by digest and
materializes it into a repository in the shape each agent harness expects.

Every transformation is deterministic and reproducible from its inputs. No
model call is made at any point.`,
		Version:       ver.String(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if root.verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
				slog.Debug("debug logs enabled")
			}

			if _, err := output.ParseFormat(root.outputFormat); err != nil {
				return err
			}

			return nil
		},
	}
	cmd.SetVersionTemplate("{{.Version}}")

	// Global flags
	cmd.PersistentFlags().StringVarP(&root.outputFormat, "output", "o", "text", "output format: text, json, yaml")
	cmd.PersistentFlags().BoolVarP(&root.verbose, "verbose", "v", false, "enable verbose mode")
	cmd.PersistentFlags().BoolP("help", "h", false, "help for qm")

	// Producer commands, grouped under `bundle`.
	cmd.AddCommand(newBundleCmd().cmd)

	// Consumer commands stay at the top level: they are the tool's primary
	// surface, run in every consuming repository.
	cmd.AddCommand(
		newInitCmd().cmd,
		newSyncCmd().cmd,
		newVerifyCmd().cmd,
		newStatusCmd().cmd,
		newTargetsCmd().cmd,
		newStatsCmd().cmd,
		newUsageCmd().cmd,
	)

	// Utility commands
	cmd.AddCommand(
		newCompletionCmd(),
		newVersionCmd(ver),
	)

	root.cmd = cmd

	return root
}
