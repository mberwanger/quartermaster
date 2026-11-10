package plan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/target"
)

// GuideID names the one document Quartermaster writes itself.
const GuideID = "qm.knowledge"

// knowledgeTree counts the documents that landed in the retrievable tree, by
// top-level directory, so the guide can say what is there without listing it.
type knowledgeTree struct {
	total int
	dirs  map[string]int
}

func (k *knowledgeTree) add(storePath string) {
	k.total++
	dir := "."
	if i := strings.IndexByte(storePath, '/'); i >= 0 {
		dir = storePath[:i]
	}
	k.dirs[dir]++
}

// guide writes the resident document that tells an agent where the delivered
// knowledge is, that it is current, and how to search it.
//
// It exists because of what agents did without it. Given a tree on disk and
// nothing in context naming it, they read the store through a sibling checkout
// on whatever branch it happened to be on, and searched it with shell greps,
// hundreds of them in a month. The tree was current and indexed the whole time;
// nothing said so. This is the thing that says so, and it is generated from the
// sync rather than written by hand so it cannot describe a tree that is not
// there.
func guide(bundles []state.Bundle, tree knowledgeTree) target.Doc {
	var b strings.Builder

	b.WriteString("# Delivered knowledge\n\n")
	b.WriteString("This repository's knowledge is delivered by Quartermaster and kept current: a sync runs when a session opens and after every checkout and merge, so what is on disk is what the store published most recently. Read it from here, not from another checkout of the store, which may be ahead of or behind what this repository was given. If you have reason to think it is stale, run `qm sync`.\n\n")

	fmt.Fprintf(&b, "The retrievable tree is `%s/`, %d document(s):\n\n", KnowledgeDir, tree.total)
	dirs := make([]string, 0, len(tree.dirs))
	for d := range tree.dirs {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		if d == "." {
			fmt.Fprintf(&b, "- at the root: %d\n", tree.dirs[d])
			continue
		}
		fmt.Fprintf(&b, "- `%s/`: %d\n", d, tree.dirs[d])
	}
	b.WriteString("\n")

	b.WriteString("Every document carries frontmatter with an `id`, a `title`, a `description`, a `type`, and a `status`. Each directory's `index.md` lists what it holds with descriptions, so read that before opening documents one by one. `qm find <term>` searches ids, titles, and descriptions across the tree and skips superseded documents; `--type` narrows it to one kind. A document that names a `superseded_by` is history: read the one it names instead.\n\n")

	b.WriteString("Delivered from:\n\n")
	for _, bd := range bundles {
		fmt.Fprintf(&b, "- `%s`", bd.Source)
		if len(bd.Packages) > 0 {
			fmt.Fprintf(&b, ", packages %s", strings.Join(bd.Packages, ", "))
		}
		if bd.Digest != "" {
			fmt.Fprintf(&b, ", digest `%s`", bd.Digest)
		}
		if len(bd.Knowledge) > 0 {
			fmt.Fprintf(&b, ", knowledge filtered on %s", strings.Join(bd.Knowledge, ", "))
		}
		b.WriteString("\n")
	}

	return target.Doc{
		ID:          GuideID,
		Description: "Where the delivered knowledge lives, what it holds, and how to search it.",
		Prose:       []byte(b.String()),
	}
}
