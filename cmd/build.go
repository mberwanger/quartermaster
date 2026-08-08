package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/preflight"
)

type buildCmd struct {
	cmd    *cobra.Command
	root   string
	out    string
	repo   string
	commit string
}

func newBuildCmd() *buildCmd {
	v := &buildCmd{}

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Compile a source tree into a bundle",
		Long: `Compile a store's source tree into the bundle agents consume.

The store tree is carried verbatim under store/, and everything else is a
derived view that could be deleted and rebuilt from it, so the markdown stays
authoritative by construction. catalog.json is the frontmatter of every doc.
packages.json is the named selections, compiled against the gate so only
qualifying documents can become rules.

Restricted documents never enter the bundle. A link that resolves to no doc,
and a package that references an unknown or gate-rejected id, both fail the
build.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			preflightResult, err := preflight.Run(preflight.Options{
				Root:   v.root,
				Repo:   v.repo,
				Commit: v.commit,
			})
			if err != nil {
				return err
			}
			builtBundle := preflightResult.Bundle

			if err := bundle.Write(builtBundle, v.out); err != nil {
				return err
			}

			var report strings.Builder
			fmt.Fprintf(&report, "wrote %s\n", v.out)
			fmt.Fprintf(&report, "  %d docs, %d files, %d packages, %d KB concatenated\n",
				builtBundle.Meta.Docs, builtBundle.Meta.Files, builtBundle.Meta.Packages,
				builtBundle.Meta.StoreBytes/1024)
			fmt.Fprintf(&report, "  %s\n", builtBundle.Meta.Digest)

			_, err = io.WriteString(cmd.OutOrStdout(), report.String())
			return err
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the knowledge store")
	cmd.Flags().StringVar(&v.out, "out", "dist", "Directory to write the bundle into")
	cmd.Flags().StringVar(&v.repo, "repo", "", "Repository name recorded in the bundle")
	cmd.Flags().StringVar(&v.commit, "commit", "", "Source commit recorded in the bundle")

	v.cmd = cmd
	return v
}
