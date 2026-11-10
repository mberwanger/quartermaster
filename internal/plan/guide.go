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
// knowledge is, how current it is, and how to search it.
//
// It exists because of what agents did without it. Given a tree on disk and
// nothing in context naming it, they read the store through a sibling checkout
// on whatever branch it happened to be on, and searched it with shell greps,
// hundreds of them in a month. The tree was current and indexed the whole time;
// nothing said so. This is the thing that says so, and it is generated from the
// sync rather than written by hand so it cannot describe a tree that is not
// there.
//
// How current the tree is depends on the manifest, so the guide says which it
// is rather than promising one. A source that names a tag follows it, and every
// sync moves the tree to what the store published last. A source pinned by
// digest re-delivers that one version however often it syncs, and an agent told
// otherwise would treat a months-old decision as the store's latest.
func guide(bundles []state.Bundle, tree knowledgeTree) target.Doc {
	var b strings.Builder

	pinned := 0
	for _, bd := range bundles {
		if isPinned(bd.Source) {
			pinned++
		}
	}

	b.WriteString("# Delivered knowledge\n\n")
	switch {
	case pinned == 0:
		b.WriteString("This repository's knowledge is delivered by Quartermaster and follows what the store publishes: a sync runs when a session opens and after every checkout and merge, and each one moves it to the store's newest published version. Read it from here, not from another checkout of the store, which may be ahead of or behind what this repository was given. If you have reason to think it is stale, run `qm sync`.\n\n")
	case pinned == len(bundles):
		b.WriteString("This repository's knowledge is delivered by Quartermaster and pinned to one published version of the store, named below. A sync delivers that version again and never a newer one, so the store may have moved on, and a decision made since then is not here. Read it from here, not from another checkout of the store; to take a newer version, the pin in the manifest changes.\n\n")
	default:
		b.WriteString("This repository's knowledge is delivered by Quartermaster. Some of it follows what the store publishes, moving to the newest version on every sync, which runs when a session opens and after every checkout and merge; some is pinned to one version and may lag the store. Each source below says which. Read it from here, not from another checkout of the store.\n\n")
	}

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
		if isPinned(bd.Source) {
			b.WriteString(", pinned")
		} else {
			b.WriteString(", following the newest")
		}
		if bd.Commit != "" {
			fmt.Fprintf(&b, ", store commit `%s` at the last sync", shortCommit(bd.Commit))
		}
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

// isPinned reports whether a source names one artifact by digest. Anything else,
// a tag or a local directory, is read afresh on every sync.
func isPinned(source string) bool {
	return strings.Contains(source, "@sha256:")
}

// shortCommit is the abbreviated form a person would type, when the commit is a
// full hash.
func shortCommit(c string) string {
	if len(c) > 12 && !strings.ContainsAny(c, "-+") {
		return c[:12]
	}
	return c
}
