package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/search"
)

type showCmd struct {
	cmd     *cobra.Command
	dir     string
	asIs    bool
	entries []search.Entry
}

func newShowCmd() *showCmd {
	v := &showCmd{}

	cmd := &cobra.Command{
		Use:   "show <id|link|path>...",
		Short: "Print a delivered document by its id or by a link to it",
		Long: `Print a document from the knowledge tree a sync wrote.

A document can be named by its id (growth.aeo), by the link other documents use
for it (/growth/aeo.md, with or without an #anchor), or by its path in the tree
(.qm/knowledge/growth/aeo.md). A directory prints its index.md, which lists
what the directory holds.

A superseded document is not the current answer, so show prints the document
that replaced it, following the chain to the end, and says so. --as-is prints
the one that was named, for reading the history.`,
		Example: "  qm show growth.aeo\n" +
			"  qm show /growth/skills/aeo/skill.md\n" +
			"  qm show records/",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args)
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory")
	cmd.Flags().BoolVar(&v.asIs, "as-is", false, "Print a superseded document instead of what replaced it")

	v.cmd = cmd
	return v
}

func (v *showCmd) run(out io.Writer, refs []string) error {
	entries, err := loadTree(v.dir)
	if err != nil {
		return err
	}
	v.entries = entries
	byID := map[string]doc.Doc{}
	byPath := map[string]doc.Doc{}
	for _, e := range entries {
		if id := e.ID(); id != "" {
			byID[id] = e.Doc
		}
		byPath[e.Path] = e.Doc
	}
	root := filepath.Join(v.dir, filepath.FromSlash(plan.KnowledgeDir))

	var missing []error
	for i, ref := range refs {
		p, note, err := v.resolve(root, ref, byID, byPath)
		if err != nil {
			missing = append(missing, err)
			continue
		}
		if i > 0 {
			fmt.Fprintln(out)
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))) //nolint:gosec // resolve refuses a path that leaves the tree
		if err != nil {
			return err
		}
		if note != "" {
			fmt.Fprintf(out, "%s\n", note)
		}
		fmt.Fprintf(out, "==> %s <==\n", path.Join(plan.KnowledgeDir, p))
		if _, err := out.Write(raw); err != nil {
			return err
		}
	}
	return errors.Join(missing...)
}

// resolve turns what was typed into a path in the tree, and a note when what
// is printed is not what was named.
func (v *showCmd) resolve(root, ref string, byID, byPath map[string]doc.Doc) (string, string, error) {
	d, ok := byID[ref]
	if !ok {
		p, isDir := treePath(ref)
		if p == ".." || strings.HasPrefix(p, "../") {
			return "", "", fmt.Errorf("%s is outside %s/; show reads only the tree this repository was given", ref, plan.KnowledgeDir)
		}
		if isDir || p == "" {
			idx := path.Join(p, doc.IndexName)
			if fileExists(root, idx) {
				return idx, "", nil
			}
		}
		d, ok = byPath[p]
		if !ok && !strings.HasSuffix(p, ".md") {
			d, ok = byPath[p+".md"]
			if !ok && fileExists(root, path.Join(p, doc.IndexName)) {
				return path.Join(p, doc.IndexName), "", nil
			}
		}
		if !ok {
			return "", "", notFound(ref, v.entries)
		}
	}

	if v.asIs || !search.IsSuperseded(d) {
		return d.Path, "", nil
	}
	if d.Str("superseded_by") == "" {
		return d.Path, fmt.Sprintf("%s is %s and names no replacement; showing it as it is.", d.ID(), d.Str("status")), nil
	}
	cur := search.Current(byID, d.Str("superseded_by"))
	next, ok := byID[cur]
	if !ok {
		return d.Path, fmt.Sprintf("%s is superseded, and what replaced it (%s) is not in this tree; showing it as it is.", d.ID(), cur), nil
	}
	return next.Path, fmt.Sprintf("%s is superseded; showing %s, which replaced it. Use --as-is for the old one.", d.ID(), cur), nil
}

// treePath reads a link the way the store writes them (/growth/aeo.md#anchor),
// or a path in the tree, as a slash path relative to the tree's root, and says
// whether it named a directory.
func treePath(ref string) (string, bool) {
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}
	ref = filepath.ToSlash(ref)
	isDir := strings.HasSuffix(ref, "/")
	ref = strings.TrimPrefix(ref, "./")
	ref = strings.TrimPrefix(ref, plan.KnowledgeDir)
	ref = strings.Trim(ref, "/")
	if ref == "" {
		return "", true
	}
	return path.Clean(ref), isDir
}

func fileExists(root, p string) bool {
	fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
	return err == nil && !fi.IsDir()
}

// notFound says what was not there and offers the documents that come
// closest, ranked the way qm find ranks them, so a mistyped id costs one more
// call rather than a search of something else.
func notFound(ref string, entries []search.Entry) error {
	words := strings.FieldsFunc(strings.ToLower(ref), func(r rune) bool {
		return r == '.' || r == '-' || r == '/' || r == '_' || r == '#'
	})
	var near []string
	for _, h := range search.Query(entries, words, search.Options{}).Hits {
		near = append(near, h.ID())
		if len(near) == 5 {
			break
		}
	}
	msg := fmt.Sprintf("%s is not in %s/.", ref, plan.KnowledgeDir)
	if len(near) > 0 {
		msg += " Closest: " + strings.Join(near, ", ") + "."
	}
	return errors.New(msg)
}
