package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/index"
	"github.com/mberwanger/quartermaster/internal/preflight"
	"github.com/mberwanger/quartermaster/internal/scaffold"
)

type bundleInitCmd struct {
	cmd   *cobra.Command
	dir   string
	name  string
	force bool
}

func newBundleInitCmd() *bundleInitCmd {
	v := &bundleInitCmd{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new knowledge store",
		Long: `Write the files a knowledge store needs: the bundle declaration, a
frontmatter schema, an empty packages file, a root index, and a template to copy
from.

The result validates and builds as it stands, so there is a working store to add
documents to rather than a pile of files to repair first.

With --force, every scaffold-owned file is replaced, including bundle.yaml, the
schema, packages, root index, and template. Other files are left untouched.`,
		Example: "  qm bundle init --name my-knowledge",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Directory to scaffold the store in")
	cmd.Flags().StringVar(&v.name, "name", "", "Store name (defaults to the directory name)")
	cmd.Flags().BoolVar(&v.force, "force", false, "Overwrite scaffold-owned files in an existing store")

	v.cmd = cmd
	return v
}

func (v *bundleInitCmd) run(out io.Writer) error {
	name := v.name
	if name == "" {
		abs, err := filepath.Abs(v.dir)
		if err != nil {
			return err
		}
		name = filepath.Base(abs)
	}
	if err := scaffold.ValidateName(name); err != nil {
		return fmt.Errorf("invalid store name: %w", err)
	}

	if scaffold.Exists(v.dir) && !v.force {
		return fmt.Errorf("%s already exists here; re-run with --force to scaffold over it", scaffold.ConfigFile)
	}

	written, err := scaffold.Write(v.dir, name)
	if err != nil {
		return err
	}

	cfg, err := config.Load(v.dir)
	if err != nil {
		return err
	}

	// Generate the listings so the store is index-current from the first commit.
	if _, err := index.Sync(v.dir, cfg, true); err != nil {
		return err
	}
	preflightResult, err := preflight.Run(preflight.Options{Root: v.dir})
	if err != nil {
		return fmt.Errorf("scaffold preflight failed: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "scaffolded store %q\n", name)
	for _, f := range written {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	fmt.Fprintf(&b, "\npreflight passes (%d doc). Next: copy meta/templates/concept.md to a domain\n",
		preflightResult.Validation.Checked)
	fmt.Fprintf(&b, "directory, then `qm bundle index`, `qm bundle validate`, and `qm bundle build`.\n")

	_, err = io.WriteString(out, b.String())
	return err
}
