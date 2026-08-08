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
	root  string
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

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to scaffold the store at")
	cmd.Flags().StringVar(&v.name, "name", "", "Store name (defaults to the directory name)")
	cmd.Flags().BoolVar(&v.force, "force", false, "Overwrite scaffold-owned files in an existing store")

	v.cmd = cmd
	return v
}

func (v *bundleInitCmd) run(out io.Writer) error {
	name := v.name
	if name == "" {
		abs, err := filepath.Abs(v.root)
		if err != nil {
			return err
		}
		name = filepath.Base(abs)
	}
	if err := scaffold.ValidateName(name); err != nil {
		return fmt.Errorf("invalid store name: %w", err)
	}

	if existing, found := scaffold.ExistingOwnedFile(v.root); found && !v.force {
		return fmt.Errorf("%s already exists here; re-run with --force to scaffold over it", existing)
	}

	written, err := scaffold.Write(v.root, name)
	if err != nil {
		return err
	}

	cfg, err := config.Load(v.root)
	if err != nil {
		return err
	}

	// A fresh scaffold carries no distributed document, so this has nothing to
	// list yet and is a no-op. It matters when --force re-scaffolds a store
	// that already has content: the listings come back current rather than
	// stale from before the overwrite.
	if _, err := index.Sync(v.root, cfg, true); err != nil {
		return err
	}
	// A bare return renders a preflight failure the same way validate and build
	// already do, rather than adding an extra prefix on top of the same
	// findings block.
	preflightResult, err := preflight.Run(preflight.Options{Root: v.root})
	if err != nil {
		return err
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
