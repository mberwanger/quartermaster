package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/plan"
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
		Use:   "find <term>...",
		Short: "Search the delivered knowledge by id, title, and description",
		Long: `Search the knowledge tree a sync wrote, and rank what matches.

Every term must match somewhere, case-insensitively. A hit in the id or title
outranks one in the description, which outranks one in the path or the body, so
a document about a thing lists above one that mentions it. A document that has
been superseded is left out: the one that replaced it is the answer.

This is what an agent reaches for instead of grepping the tree. A grep finds
every mention; this finds the document.`,
		Example:           "  qm find drift\n  qm find --type record hold apply",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return v.run(cmd.OutOrStdout(), args)
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory")
	cmd.Flags().StringVar(&v.docType, "type", "", "Only documents of this type")
	cmd.Flags().IntVar(&v.top, "top", 20, "How many to show")

	v.cmd = cmd
	return v
}

// hit is one matching document with the score that ranks it.
type hit struct {
	doc.Doc
	score int
}

func (v *findCmd) run(out io.Writer, terms []string) error {
	root := filepath.Join(v.dir, filepath.FromSlash(plan.KnowledgeDir))
	docs, err := doc.Load(root, nil)
	if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
		return fmt.Errorf("no knowledge tree at %s; run qm sync", plan.KnowledgeDir)
	}
	if err != nil {
		return err
	}

	hits := search(docs, terms, v.docType)
	if len(hits) == 0 {
		fmt.Fprintf(out, "nothing matches %s\n", strings.Join(terms, " "))
		return nil
	}

	var b strings.Builder
	shown := hits
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
		fmt.Fprintf(&b, "    %s\n", path.Join(plan.KnowledgeDir, h.Path))
	}
	if len(hits) > len(shown) {
		fmt.Fprintf(&b, "%d more; narrow the terms or raise --top\n", len(hits)-len(shown))
	}

	_, err = io.WriteString(out, b.String())
	return err
}

// search ranks the documents every term matches. Weights put a document that is
// about the term above one that mentions it: id and title are what a document
// is, the description is what it is for, and the body is everything it says.
func search(docs []doc.Doc, terms []string, docType string) []hit {
	var hits []hit
	for _, d := range docs {
		if d.ID() == "" || superseded(d) {
			continue
		}
		if docType != "" && d.Str("type") != docType {
			continue
		}
		id := strings.ToLower(d.ID())
		title := strings.ToLower(d.Str("title"))
		desc := strings.ToLower(d.Str("description"))
		p := strings.ToLower(d.Path)
		body := strings.ToLower(string(d.Prose))

		score := 0
		for _, t := range terms {
			t = strings.ToLower(t)
			s := 0
			if strings.Contains(id, t) {
				s += 4
			}
			if strings.Contains(title, t) {
				s += 4
			}
			if strings.Contains(desc, t) {
				s += 2
			}
			if strings.Contains(p, t) {
				s++
			}
			if strings.Contains(body, t) {
				s++
			}
			if s == 0 {
				score = 0
				break
			}
			score += s
		}
		if score > 0 {
			hits = append(hits, hit{Doc: d, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].ID() < hits[j].ID()
	})
	return hits
}

// superseded reports whether a document has been replaced, by the field that
// names its successor or by a status that says so.
func superseded(d doc.Doc) bool {
	if d.Str("superseded_by") != "" {
		return true
	}
	switch d.Str("status") {
	case "superseded", "deprecated", "retired":
		return true
	}
	return false
}

func isNotDir(err error) bool {
	var pe *os.PathError
	return errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "not a directory")
}
