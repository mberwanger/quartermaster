package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/ruleset"
)

type explainCmd struct {
	cmd  *cobra.Command
	root string
}

func newExplainCmd() *explainCmd {
	v := &explainCmd{}

	cmd := &cobra.Command{
		Use:   "explain <id>",
		Short: "Show why a document was or was not materialized",
		Long: `Explain a document's fate: whether it is a document at all under the
include and exclude patterns, whether it passes the gate, and which rulesets
reference it.

Selection is a filter over frontmatter, so when a document does not appear where
an author expects, the failing predicate should be one command away. This is
that command.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args[0])
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the knowledge store")

	v.cmd = cmd
	return v
}

func (v *explainCmd) run(out io.Writer, id string) error {
	cfg, err := config.Load(v.root)
	if err != nil {
		return err
	}

	docs, err := doc.Load(v.root, []string{"dist"})
	if err != nil {
		return err
	}

	var found *doc.Doc
	for i := range docs {
		if docs[i].ID() == id {
			found = &docs[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("no document has id %q under %s", id, v.root)
	}

	rulesets, err := loadRulesets(v.root, cfg)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", id)
	fmt.Fprintf(&b, "  path        %s\n", found.Path)

	isDoc := cfg.IsDocument(found.Path)
	fmt.Fprintf(&b, "  document    %s\n", yesNo(isDoc, "included", "excluded by include/exclude patterns"))
	if cfg.IsControl(found.Path) {
		fmt.Fprintf(&b, "  control     yes (partitioned out of the catalog and corpus)\n")
	}

	restricted := found.Str("visibility") == "restricted"
	fmt.Fprintf(&b, "  on disk     %s\n", yesNo(isDoc && !restricted && !cfg.IsControl(found.Path),
		"yes", "no"))

	allowed, reason := cfg.Requires.Allows(found.Frontmatter)
	if allowed {
		fmt.Fprintf(&b, "  requires    met\n")
	} else {
		fmt.Fprintf(&b, "  requires    not met: %s\n", reason)
	}

	referencing := rulesetsReferencing(rulesets, id)
	if len(referencing) == 0 {
		fmt.Fprintf(&b, "  rulesets    none reference it\n")
	} else {
		fmt.Fprintf(&b, "  rulesets    %s\n", strings.Join(referencing, ", "))
	}

	// The bottom line: a document becomes a rule only when a ruleset references
	// it and it passes the gate.
	switch {
	case len(referencing) > 0 && allowed:
		fmt.Fprintf(&b, "  → materializes as a rule via %s\n", strings.Join(referencing, ", "))
	case len(referencing) > 0 && !allowed:
		fmt.Fprintf(&b, "  → referenced but does not meet the requirements, so the build fails until fixed\n")
	case isDoc && !restricted:
		fmt.Fprintf(&b, "  → on disk only; no ruleset turns it into a rule\n")
	default:
		fmt.Fprintf(&b, "  → not materialized\n")
	}

	_, err = io.WriteString(out, b.String())
	return err
}

func loadRulesets(root string, cfg *config.Config) (ruleset.File, error) {
	if cfg.Rulesets == "" {
		return ruleset.File{}, nil
	}
	return ruleset.Load(filepath.Join(root, cfg.Rulesets))
}

func rulesetsReferencing(f ruleset.File, id string) []string {
	var out []string
	for _, name := range f.Names() {
		for _, ref := range f[name].Docs {
			if ref.ID == id {
				out = append(out, name)
				break
			}
		}
	}
	return out
}

func yesNo(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
