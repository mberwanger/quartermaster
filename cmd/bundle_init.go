package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/index"
	"github.com/mberwanger/quartermaster/internal/scaffold"
	"github.com/mberwanger/quartermaster/internal/validate"
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
frontmatter schema, an empty rulesets file, a root index, and a template to copy
from.

The result validates and builds as it stands, so there is a working store to add
documents to rather than a pile of files to repair first.`,
		Example: "  qm bundle init --name my-knowledge",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Directory to scaffold the store in")
	cmd.Flags().StringVar(&v.name, "name", "", "Store name (defaults to the directory name)")
	cmd.Flags().BoolVar(&v.force, "force", false, "Scaffold even if a store already exists")

	v.cmd = cmd
	return v
}

func (v *bundleInitCmd) run(out io.Writer) error {
	if scaffold.Exists(v.dir) && !v.force {
		return fmt.Errorf("%s already exists here; re-run with --force to scaffold over it", scaffold.ConfigFile)
	}

	name := v.name
	if name == "" {
		abs, err := filepath.Abs(v.dir)
		if err != nil {
			return err
		}
		name = filepath.Base(abs)
	}

	written, err := scaffold.Write(v.dir, name)
	if err != nil {
		return err
	}

	cfg, err := config.Load(v.dir)
	if err != nil {
		return err
	}

	// Generate the listings so the store is index-current from the first commit,
	// and validate so init never leaves behind something that does not check.
	if _, err := index.Sync(v.dir, cfg, true); err != nil {
		return err
	}
	res, err := validate.Run(v.dir, cfg)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "scaffolded store %q\n", name)
	for _, f := range written {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	if !res.OK() {
		// The scaffold is fixed, so this means the tool and its own template
		// disagree. Surface it rather than claim success.
		fmt.Fprintf(&b, "\nwarning: the scaffold did not validate cleanly:\n")
		for _, f := range res.Findings {
			fmt.Fprintf(&b, "  %s: %s\n", f.Path, f.Message)
		}
	} else {
		fmt.Fprintf(&b, "\nvalidates (%d doc). Next: copy meta/templates/concept.md to a domain\n", res.Checked)
		fmt.Fprintf(&b, "directory, then `qm bundle validate` and `qm bundle build`.\n")
	}

	_, err = io.WriteString(out, b.String())
	return err
}
