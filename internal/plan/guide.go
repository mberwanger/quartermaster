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
// Having the guide was not enough on its own. An agent that read it still
// grepped, because it said find searched only ids, titles and descriptions,
// which read as useless for a phrase, and because the tree is gitignored, so
// the agent's grep came back empty and it went to a sibling checkout. The
// guide now gives an order to follow rather than a description of tools, says
// why a grep of the repository finds nothing, and names the dead end.
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

	b.WriteString("## Finding something in it\n\n")
	b.WriteString("Look here first for anything about how this organization builds, decides, names or writes, before answering from memory and before any other search.\n\n")
	b.WriteString("1. `qm find <words>`. Ask it the way you would ask a person, or paste a phrase or a heading. It searches ids, titles, descriptions, headings and bodies, ranks the documents, and shows the lines that matched with their line numbers. A document that holds most of the words still ranks, so a question works as well as a keyword. If nothing fits, try other words before anything else.\n")
	b.WriteString("2. `qm show <id>` prints a document. It also takes the links documents use for each other, such as `/growth/aeo.md`, and a directory, which prints its `index.md`. A superseded document is never the answer: `find` ranks what replaced it in its place, and `show` prints the replacement and says so.\n")
	fmt.Fprintf(&b, "3. Grep only as a last step, and only by naming `%s/` as the path. The tree is gitignored, so a search tool that respects `.gitignore` skips it and reports nothing.\n\n", KnowledgeDir)
	b.WriteString("Never read another checkout of the store, a sibling directory, a branch or an archive. It may be ahead of or behind what this repository was given, and its superseded documents read as current.\n\n")
	b.WriteString("A sync runs when a session opens, and the session's first note says whether it worked. If that note says the refresh failed, use the tree as it is, tell the person, and run `qm sync` once they say access is fixed. There is no need to sync on a timer.\n\n")
	b.WriteString("Every document carries frontmatter with an `id`, a `title`, a `description`, a `type`, and a `status`, and each directory's `index.md` lists what it holds. `--type` narrows `qm find` to one kind of document.\n\n")
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
