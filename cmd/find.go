package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/search"
)

type findCmd struct {
	cmd     *cobra.Command
	dir     string
	docType string
	top     int
}

func newFindCmd() *findCmd {
	v := &findCmd{}

	cmd := &cobra.Command{
		Use:   "find <words>...",
		Short: "Search the delivered knowledge, and point at the lines that answer",
		Long: `Search the knowledge tree a sync wrote, rank the documents, and show the
lines that matched with their line numbers.

Ask the way you would ask a person: "qm find how should a cli choose an exit
code" works, and so does a phrase or a heading copied from a document. Filler
words are dropped, a document that holds most of the words still ranks, a rare
word counts for more than a common one, and the exact phrase counts for most.
A match in the id, the title, or an alias outranks one in the description,
then a heading, then the body. An alias is a word a person wrote in the
document's aliases field because readers search for it when that document is
the answer, though the document does not use it as its own term.

A superseded document is never listed. When one matches, the document that
replaced it now, following the whole chain, takes its rank and says so.

Each hit shows the lines that matched, numbered as they are in the file, so
the next step is to read that file at that line, or run qm show with the id.

The tree is gitignored, so search tools that respect .gitignore never see it.
This is what to reach for instead of grepping, and never search another
checkout of the store: it is not what this repository was given.`,
		Example: "  qm find drift\n" +
			"  qm find how should a cli choose an exit code\n" +
			"  qm find --type record hold apply",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args)
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory")
	cmd.Flags().StringVar(&v.docType, "type", "", "Only documents of this type")
	cmd.Flags().IntVar(&v.top, "top", 10, "How many documents to show")

	v.cmd = cmd
	return v
}

func (v *findCmd) run(out io.Writer, typed []string) error {
	entries, err := loadTree(v.dir)
	if err != nil {
		return err
	}

	res := search.Query(entries, typed, search.Options{Type: v.docType})

	var b strings.Builder
	if len(res.Hits) == 0 {
		fmt.Fprintf(&b, "nothing in %s/ matches %s.\n", plan.KnowledgeDir, strings.Join(typed, " "))
		b.WriteString("This tree is everything this repository was given; another checkout of the store is not.\n")
		fmt.Fprintf(&b, "Try fewer or different words, or read %s/index.md for what each directory holds.\n", plan.KnowledgeDir)
		_, err = io.WriteString(out, b.String())
		return err
	}

	shown := res.Hits
	if len(shown) > v.top {
		shown = shown[:v.top]
	}
	for _, h := range shown {
		fmt.Fprintf(&b, "%s", h.ID())
		if t := h.Str("type"); t != "" {
			fmt.Fprintf(&b, "  (%s)", t)
		}
		if title := h.Str("title"); title != "" {
			fmt.Fprintf(&b, "  %s", title)
		}
		b.WriteByte('\n')
		if d := h.Str("description"); d != "" {
			fmt.Fprintf(&b, "    %s\n", d)
		}
		p := path.Join(plan.KnowledgeDir, h.Path)
		fmt.Fprintf(&b, "    %s\n", p)
		for _, l := range h.Lines {
			fmt.Fprintf(&b, "      %d: %s\n", l.N, l.Text)
		}
		if h.Alias != "" {
			fmt.Fprintf(&b, "    (matched its alias %q)\n", h.Alias)
		}
		if len(h.Replaces) > 0 {
			names := h.Replaces
			more := ""
			if len(names) > 2 {
				more = fmt.Sprintf(" and %d more", len(names)-2)
				names = names[:2]
			}
			fmt.Fprintf(&b, "    (in place of superseded %s%s, which matched)\n", strings.Join(names, ", "), more)
		}
		if h.Matched < len(res.Words) {
			fmt.Fprintf(&b, "    (matches %d of %d words)\n", h.Matched, len(res.Words))
		}
	}
	if len(res.Hits) > len(shown) {
		fmt.Fprintf(&b, "%d more; narrow the words or raise --top\n", len(res.Hits)-len(shown))
	}

	_, err = io.WriteString(out, b.String())
	return err
}

// loadTree reads the delivered tree with each document's raw file, so a hit
// can be numbered by the line an agent will open.
func loadTree(dir string) ([]search.Entry, error) {
	root := filepath.Join(dir, filepath.FromSlash(plan.KnowledgeDir))
	docs, err := doc.Load(root, nil)
	if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
		return nil, fmt.Errorf("no knowledge tree at %s; run qm sync", plan.KnowledgeDir)
	}
	if err != nil {
		return nil, err
	}
	entries := make([]search.Entry, 0, len(docs))
	for _, d := range docs {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(d.Path))) //nolint:gosec // path came from walking the tree
		if err != nil {
			return nil, err
		}
		entries = append(entries, search.NewEntry(d, raw))
	}
	return entries, nil
}

func isNotDir(err error) bool {
	var pe *os.PathError
	return errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "not a directory")
}
