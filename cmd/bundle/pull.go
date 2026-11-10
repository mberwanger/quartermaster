package bundle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	bundlepkg "github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/provider"
)

type pullCmd struct {
	cmd   *cobra.Command
	out   string
	force bool
}

func newPullCmd() *pullCmd {
	v := &pullCmd{}

	cmd := &cobra.Command{
		Use:   "pull <source>",
		Short: "Resolve a bundle source and write the artifact to disk",
		Long: `Resolve a bundle source and write the artifact out, without a consuming
repository.

qm init and qm sync are how a repository consumes knowledge, and they materialize
the parts a harness needs. This writes the artifact itself: meta.json, the
catalog, the compiled packages, and the store tree verbatim. It is for looking at
a bundle, diffing two of them, or feeding one to something that is not a
Quartermaster consumer.

Every source scheme works, and a source naming a store tree rather than a built
bundle is compiled on the way through, so the same command reads both. Remote
schemes cache by digest, so pulling a bundle a repository already resolved costs
nothing; relative file:// paths resolve against the working directory.

Credentials are ambient, matching the rest of the tool: the Docker credential
store for oci, git's own credentials for git+https.`,
		Example: "  qm bundle pull oci://ghcr.io/org/knowledge:latest --out dist",
		Args:    cobra.ExactArgs(1),
		// A source is a URL, not a path, so file completion would offer the
		// wrong thing everywhere but file://.
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args[0])
		},
	}

	cmd.Flags().StringVar(&v.out, "out", "dist", "Directory to write the artifact into")
	cmd.Flags().BoolVar(&v.force, "force", false, "Overwrite the output directory even if it holds something else")

	v.cmd = cmd
	return v
}

func (v *pullCmd) run(out io.Writer, source string) error {
	// Checked before resolving, so a doomed write fails before a large download
	// rather than after it.
	if err := checkOutDir(v.out, v.force); err != nil {
		return err
	}

	b, err := provider.Resolve(source, ".", provider.Auth{})
	if err != nil {
		return err
	}

	if err := bundlepkg.Write(b, v.out); err != nil {
		return err
	}

	var report strings.Builder
	fmt.Fprintf(&report, "wrote %s from %s\n", v.out, source)
	fmt.Fprintf(&report, "  %d docs, %d files, %d packages, %d KB store\n",
		b.Meta.Docs, b.Meta.Files, b.Meta.Packages, b.Meta.StoreBytes/1024)
	fmt.Fprintf(&report, "  %s\n", b.Meta.Digest)

	_, err = io.WriteString(out, report.String())
	return err
}

// checkOutDir refuses to clear a directory that is not this command's to clear.
//
// Writing an artifact removes the destination first, so that a document deleted
// upstream cannot survive into the output and go on being read. That is right
// for a build, whose --out is conventionally dist, and dangerous here: a source
// is the argument, so --out is the incidental part and far likelier to be
// pointed somewhere real.
//
// An empty or absent directory is free to write. So is one already holding a
// bundle, which is the ordinary case of pulling a newer version over an older
// one. Anything else is someone's data, and --force is how they say otherwise.
func checkOutDir(dir string, force bool) error {
	if force {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	if _, err := os.Stat(filepath.Join(dir, bundlepkg.MetaName)); err == nil {
		return nil
	}

	return fmt.Errorf("%s is not empty and holds no %s; writing a bundle clears it first, so pass --force to overwrite it anyway",
		dir, bundlepkg.MetaName)
}
