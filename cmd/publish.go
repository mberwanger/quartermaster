package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/oci"
)

type publishCmd struct {
	cmd  *cobra.Command
	dir  string
	ref  string
	tags []string
}

func newPublishCmd() *publishCmd {
	v := &publishCmd{}

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Push a built bundle to a provider",
		Long: `Push a built bundle to an OCI registry.

The bundle travels as a single deterministic tarball layer, so republishing
unchanged content does not churn the registry. The manifest's annotations carry
the source repository, the source commit, and the bundle's own content digest,
so an artifact can always be traced back to the tree that produced it.

Extra tags let one push be addressed both by version and by source commit.`,
		Example: "  qm bundle publish --dir dist --ref ghcr.io/org/knowledge:v0.14.2 --tag latest",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", "dist", "Directory holding the built bundle")
	cmd.Flags().StringVar(&v.ref, "ref", "", "Target reference, e.g. ghcr.io/org/knowledge:v0.14.2")
	cmd.Flags().StringArrayVar(&v.tags, "tag", nil, "Additional tag to apply (repeatable)")

	v.cmd = cmd
	return v
}

func (v *publishCmd) run(out io.Writer) error {
	if v.ref == "" {
		return fmt.Errorf("--ref is required, e.g. ghcr.io/org/knowledge:v0.14.2")
	}

	// Reading the bundle first proves the directory really holds one, and gives
	// the provenance the annotations carry.
	b, err := bundle.Read(v.dir)
	if err != nil {
		return fmt.Errorf("read bundle at %s: %w", v.dir, err)
	}

	repo, err := oci.Open(v.ref)
	if err != nil {
		return err
	}

	annotations := map[string]string{
		oci.AnnotationBundleDigest:    b.Meta.Digest,
		ocispec.AnnotationSource:      b.Meta.Source.Repo,
		ocispec.AnnotationRevision:    b.Meta.Source.Commit,
		ocispec.AnnotationVersion:     repo.Reference(),
		ocispec.AnnotationDescription: fmt.Sprintf("%s knowledge bundle, %d docs", b.Meta.Name, b.Meta.Docs),
	}
	if names := rulesetNames(b); names != "" {
		annotations[oci.AnnotationRulesets] = names
	}
	for k, val := range annotations {
		if val == "" {
			delete(annotations, k)
		}
	}

	tags := append([]string{repo.Reference()}, v.tags...)
	// The source commit is a tag too, so an artifact is addressable by the tree
	// that produced it and not only by the version someone chose.
	if c := b.Meta.Source.Commit; c != "" {
		tags = append(tags, c)
	}

	desc, err := repo.Push(context.Background(), v.dir, annotations, tags)
	if err != nil {
		return err
	}

	var report strings.Builder
	fmt.Fprintf(&report, "published %s\n", v.ref)
	fmt.Fprintf(&report, "  artifact  %s\n", desc.Digest)
	fmt.Fprintf(&report, "  bundle    %s\n", b.Meta.Digest)
	fmt.Fprintf(&report, "  tags      %s\n", strings.Join(dedupe(tags), ", "))

	_, err = io.WriteString(out, report.String())
	return err
}

func rulesetNames(b *bundle.Bundle) string {
	names := make([]string, 0, len(b.Rulesets))
	for _, rs := range b.Rulesets {
		names = append(names, rs.Name)
	}
	return strings.Join(names, ",")
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
