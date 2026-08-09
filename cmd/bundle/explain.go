package bundle

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/pack"
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
include and exclude patterns, which packages select it and as what, and whether
it can be that.

Ask it when a document is well-formed and still does not turn up where you
expected. A malformed one is qm bundle validate's job, and it says so loudly;
this is for the case where nothing failed and the document is simply not
selected, which no other command reports.

What has to hold depends on the kind. The requires gate in bundle.yaml decides
rule eligibility only. A skill is judged by the skills predicate, an agent by
whether it declares an agent block, and a restricted document can be none of
them.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args[0])
		},
	}

	cmd.Flags().StringVar(&v.root, "root", ".", "Path to the store")

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

	packages, err := loadPackages(v.root, cfg)
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

	// The requires gate decides rule eligibility and nothing else. A skill is
	// judged by the skills predicate and an agent by whether it declares an
	// agent block, so reporting this as though it governed all three would say
	// a draft skill cannot ship when it can.
	ruleAllowed, ruleReason := cfg.Requires.Allows(found.Frontmatter)
	fmt.Fprintf(&b, "  rule gate   %s\n", metOrNot(ruleAllowed, ruleReason))

	referencing := packagesReferencing(packages, id, found.Frontmatter)
	if len(referencing) == 0 {
		fmt.Fprintf(&b, "  packages    none reference it\n")
	}
	for _, sel := range referencing {
		fmt.Fprintf(&b, "  packages    as %s: %s\n", sel.kind, strings.Join(sel.packages, ", "))
	}

	// The bottom line, which is per kind because the qualifying predicate is.
	switch {
	case len(referencing) == 0 && isDoc && !restricted:
		fmt.Fprintf(&b, "  → on disk only; no package selects it\n")
	case len(referencing) == 0:
		fmt.Fprintf(&b, "  → not materialized\n")
	}
	for _, sel := range referencing {
		ok, reason := v.qualifies(cfg, sel.kind, found, ruleAllowed, ruleReason)
		if ok {
			fmt.Fprintf(&b, "  → materializes as %s via %s\n", article(sel.kind), strings.Join(sel.packages, ", "))
			continue
		}
		fmt.Fprintf(&b, "  → selected as %s by %s, which it cannot be: %s, so the build fails until fixed\n",
			article(sel.kind), strings.Join(sel.packages, ", "), reason)
	}

	_, err = io.WriteString(out, b.String())
	return err
}

// qualifies mirrors what the package compiler asks of a document for each kind.
// Restricted is checked first and for every kind, because a restricted document
// never enters the bundle and so cannot be selected as anything.
func (v *explainCmd) qualifies(cfg *config.Config, kind pack.Kind, d *doc.Doc, ruleAllowed bool, ruleReason string) (bool, string) {
	if d.Restricted() {
		return false, "it is restricted and never leaves the store"
	}
	switch kind {
	case pack.KindRule:
		return ruleAllowed, ruleReason
	case pack.KindSkill:
		if ok, _ := cfg.Skills.Allows(d.Frontmatter); !ok {
			return false, "it is not a skill"
		}
	case pack.KindAgent:
		if _, ok := d.Frontmatter["agent"].(map[string]any); !ok {
			return false, "it is not an agent"
		}
	}
	return true, ""
}

func metOrNot(ok bool, reason string) string {
	if ok {
		return "met"
	}
	return "not met: " + reason
}

// article names one kind in the singular, since the compiler's Kind values are
// the plural list names a package writes.
func article(kind pack.Kind) string {
	switch kind {
	case pack.KindRule:
		return "a rule"
	case pack.KindSkill:
		return "a skill"
	case pack.KindAgent:
		return "an agent"
	}
	return string(kind)
}

func loadPackages(root string, cfg *config.Config) (pack.File, error) {
	if cfg.Packages == "" {
		return pack.File{}, nil
	}
	return pack.Load(filepath.Join(root, cfg.Packages))
}

// selection is the packages that name a document as one kind. A document is
// usually selected as one thing, but nothing stops two packages disagreeing, so
// the kind is carried rather than assumed.
type selection struct {
	kind     pack.Kind
	packages []string
}

// packagesReferencing reports which packages name a document, and as what. The
// kind is the whole point: it decides both what the document becomes and which
// predicate has to hold for it, and those differ. A pattern or a where clause is
// reported when it matches, so an author can see that a document is selected
// without a package naming it outright.
func packagesReferencing(f pack.File, id string, frontmatter map[string]any) []selection {
	byKind := map[pack.Kind][]string{}
	for _, name := range f.Names() {
		p := f[name]
		for _, sel := range []struct {
			kind pack.Kind
			refs []pack.Ref
		}{
			{pack.KindRule, p.Rules},
			{pack.KindSkill, p.Skills},
			{pack.KindAgent, p.Agents},
		} {
			if selects(sel.refs, id, frontmatter) {
				byKind[sel.kind] = append(byKind[sel.kind], name)
			}
		}
	}

	var out []selection
	for _, kind := range []pack.Kind{pack.KindRule, pack.KindSkill, pack.KindAgent} {
		if names := byKind[kind]; len(names) > 0 {
			out = append(out, selection{kind: kind, packages: names})
		}
	}
	return out
}

func selects(refs []pack.Ref, id string, frontmatter map[string]any) bool {
	for _, ref := range refs {
		if ref.Matches(id, frontmatter) {
			return true
		}
	}
	return false
}

func yesNo(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
