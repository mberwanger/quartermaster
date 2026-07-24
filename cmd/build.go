package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/ruleset"
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
rulesets.json is the named selections, compiled against the gate so only
qualifying documents can become rules.

Restricted documents never enter the bundle. A link that resolves to no doc,
and a ruleset that references an unknown or gate-rejected id, both fail the
build.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(v.root)
			if err != nil {
				return err
			}

			var rs ruleset.File
			if cfg.Rulesets != "" {
				rs, err = ruleset.Load(filepath.Join(v.root, cfg.Rulesets))
				if err != nil {
					return err
				}
			}

			b, err := bundle.Build(bundle.Options{
				Root:     v.root,
				Config:   cfg,
				Rulesets: rs,
				Repo:     v.repo,
				Commit:   v.commit,
			})
			if err != nil {
				return err
			}

			if err := bundle.Write(b, v.out); err != nil {
				return err
			}

			var report strings.Builder
			fmt.Fprintf(&report, "wrote %s\n", v.out)
			fmt.Fprintf(&report, "  %d docs, %d files, %d rulesets, %d KB concatenated\n",
				b.Meta.Docs, b.Meta.Files, b.Meta.Rulesets, b.Meta.StoreBytes/1024)
			fmt.Fprintf(&report, "  %s\n", b.Meta.Digest)

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
