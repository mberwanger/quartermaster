// Package index generates the index.md directory listings the Open Knowledge
// Format defines in §6, so a reader or an agent can see what a directory holds
// without opening every document in it.
//
// The tool owns a marked region inside each index.md, not the whole file. The
// prose above the region explains why a directory exists, which no generator can
// write, and the region below it is the listing, which no human should have to
// maintain by hand.
//
// A skill directory gets no listing and is not descended into. A skill is one
// unit an agent loads whole, so a listing of its parts describes nothing a
// reader needs, and it would travel into every repository that materializes the
// skill as a stale map of somewhere else.
package index

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/target"
)

// Marker names the region this tool owns inside each index.md.
const Marker = "GENERATED"

// Name is the reserved filename for a directory listing.
const Name = doc.IndexName

// CurrentOKFVersion is the Open Knowledge Format version a new store declares.
// It is the single source this package and internal/scaffold both read, so a
// version bump cannot update one and miss the other.
const CurrentOKFVersion = "0.2"

// rootFrontmatter declares the format version. The store root index is the only
// index file where frontmatter is permitted.
const rootFrontmatter = "---\nokf_version: \"" + CurrentOKFVersion + "\"\n---\n"

// File is one index.md and the content its generated region should hold.
type File struct {
	// Path is slash-separated and relative to the store root.
	Path string
	// Region is the body between the markers.
	Region string
}

// headings maps a document type to the section it is listed under. The order is
// the taxonomy's, so a listing reads the same way in every directory.
var headings = []struct{ docType, heading string }{
	{"concept", "Concepts"},
	{"reference", "References"},
	{"record", "Records"},
	{"skill", "Skills"},
	{"agent", "Agents"},
	// Types no store here uses, kept because the enum belongs to each store's
	// own schema rather than to Quartermaster. A store that declares one of
	// these gets a section instead of falling into Other.
	{"runbook", "Runbooks"},
	{"guide", "Guides"},
	{"policy", "Policies"},
	{"decision", "Decisions"},
	{"strategy", "Strategies"},
}

// Build computes the index files the store should have. A directory gets one
// when it holds documents, or when a directory beneath it does.
func Build(root string, cfg *config.Config) ([]File, error) {
	files, _, err := build(root, cfg)
	return files, err
}

// build is Build plus the skill directories it stopped at, which Orphans needs
// in order to leave a skill's own files alone.
func build(root string, cfg *config.Config) ([]File, map[string]bool, error) {
	all, err := doc.Load(root, []string{"dist"})
	if err != nil {
		return nil, nil, err
	}

	// Distributed rather than merely a document. An index is a navigation aid
	// that travels in the bundle and lands in a consuming repository, so listing
	// something the bundle does not carry points an agent at a file that is not
	// there. Templates are the case that shows it: they are documents, so they
	// are validated, and they are excluded, so they must not be advertised.
	var docs []doc.Doc
	for _, d := range all {
		// Restricted as well as undistributed. A listing that names a restricted
		// document leaks its title and description, which is most of what made
		// it worth restricting, and it points at a file the bundle does not
		// carry.
		if cfg.IsDistributed(d.Path) && !d.Restricted() {
			docs = append(docs, d)
		}
	}
	skillDirs, skillPaths := cfg.SkillDirs(docs)

	byDir := make(map[string][]doc.Doc)
	for _, d := range docs {
		if d.Frontmatter == nil || config.IsAsset(d.Path, skillDirs, skillPaths) {
			continue
		}
		byDir[path.Dir(d.Path)] = append(byDir[path.Dir(d.Path)], d)
	}

	// A directory needs an index if it holds documents or is an ancestor of one.
	// A skill's own directory is the exception: it is a leaf the listing stops
	// at. Its ancestors still need one, or the skill would be undiscoverable
	// from the directory above it.
	needed := make(map[string]bool)
	for dir := range byDir {
		start := dir
		if skillDirs[dir] {
			start = path.Dir(dir)
		}
		for d := start; ; d = path.Dir(d) {
			needed[d] = true
			if d == "." {
				break
			}
		}
	}

	kids := make(map[string][]string)
	for dir := range needed {
		if dir == "." {
			continue
		}
		kids[path.Dir(dir)] = append(kids[path.Dir(dir)], dir)
	}
	// A skill directory is listed by its parent even though it holds no index of
	// its own, so the skill is discoverable from above.
	for dir := range skillDirs {
		kids[path.Dir(dir)] = append(kids[path.Dir(dir)], dir)
	}

	files := make([]File, 0, len(needed))
	for dir := range needed {
		files = append(files, File{
			Path:   path.Join(dir, Name),
			Region: render(kids[dir], byDir[dir], skillDirs, func(p string) int { return count(docs, p) }),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, skillDirs, nil
}

// count reports how many documents live at or below a directory.
func count(docs []doc.Doc, dir string) int {
	n := 0
	for _, d := range docs {
		if d.Frontmatter != nil && strings.HasPrefix(d.Path, dir+"/") {
			n++
		}
	}
	return n
}

func render(subdirs []string, docs []doc.Doc, skillDirs map[string]bool, countAt func(string) int) string {
	var b strings.Builder

	if len(subdirs) > 0 {
		sort.Strings(subdirs)
		b.WriteString("# Directories\n\n")
		for _, s := range subdirs {
			if skillDirs[s] {
				fmt.Fprintf(&b, "* [%s/](/%s/) - a skill\n", path.Base(s), s)
				continue
			}
			fmt.Fprintf(&b, "* [%s/](/%s/) - %s\n", path.Base(s), s, documents(countAt(s)))
		}
		b.WriteString("\n")
	}

	grouped := make(map[string][]doc.Doc)
	for _, d := range docs {
		grouped[d.Str("type")] = append(grouped[d.Str("type")], d)
	}

	seen := make(map[string]bool)
	for _, h := range headings {
		seen[h.docType] = true
		writeSection(&b, h.heading, grouped[h.docType])
	}

	// A type outside the taxonomy is still listed rather than dropped. The
	// schema rejects one today, but the listing must not depend on that.
	var other []doc.Doc
	for t, list := range grouped {
		if !seen[t] {
			other = append(other, list...)
		}
	}
	writeSection(&b, "Other", other)

	return b.String()
}

func writeSection(b *strings.Builder, heading string, docs []doc.Doc) {
	if len(docs) == 0 {
		return
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })

	fmt.Fprintf(b, "# %s\n\n", heading)
	for _, d := range docs {
		title := d.Str("title")
		if title == "" {
			title = path.Base(d.Path)
		}
		fmt.Fprintf(b, "* [%s](/%s) - %s\n", title, d.Path, d.Str("description"))
	}
	b.WriteString("\n")
}

func documents(n int) string {
	if n == 1 {
		return "1 document"
	}
	return fmt.Sprintf("%d documents", n)
}

// Orphan is a listing for a directory the store no longer has documents in,
// whose generated region still names some.
type Orphan struct {
	// Path is slash-separated and relative to the store root.
	Path string
	// Entries is how many documents the stale region still lists, which is the
	// measure of how wrong it is.
	Entries int
}

// Orphans reports listings nothing regenerates any more.
//
// Sync only visits the files Build asks for, so an index whose directory has
// been emptied is not rewritten and not deleted: it keeps advertising documents
// that moved or went away. The build refuses to carry it (a reserved file in an
// unindexed directory is dropped there), so this never reaches a consumer. It
// misleads whoever reads the store itself, which is reason enough to say so.
//
// This reports and never writes. Three rules keep it off files that are not
// stale listings at all, because the cost of a false positive here is a person
// deleting prose a generator cannot put back:
//
//   - The file carries the markers. A hand-written index.md has no generated
//     region, and appending one to it is the specific damage to avoid.
//   - The region names at least one document. An empty region tells no lie, and
//     a store commonly keeps one in a directory that holds only machinery.
//   - The directory is not a skill. A listing stops at a skill rather than
//     descending, so a skill's own files are never this package's to judge.
func Orphans(root string, cfg *config.Config) ([]Orphan, error) {
	files, skillDirs, err := build(root, cfg)
	if err != nil {
		return nil, err
	}

	needed := make(map[string]bool, len(files))
	for _, f := range files {
		needed[f.Path] = true
	}

	reserved, err := doc.LoadReserved(root, []string{"dist"})
	if err != nil {
		return nil, err
	}

	var orphans []Orphan
	for _, f := range reserved {
		if path.Base(f.Path) != Name || needed[f.Path] {
			continue
		}
		if inSkill(path.Dir(f.Path), skillDirs) {
			continue
		}
		region, ok := target.BlockRegion(string(f.Body), Marker)
		if !ok {
			continue
		}
		if n := entries(region); n > 0 {
			orphans = append(orphans, Orphan{Path: f.Path, Entries: n})
		}
	}

	sort.Slice(orphans, func(i, j int) bool { return orphans[i].Path < orphans[j].Path })
	return orphans, nil
}

// inSkill reports whether a directory is a skill or sits inside one.
func inSkill(dir string, skillDirs map[string]bool) bool {
	for d := dir; ; d = path.Dir(d) {
		if skillDirs[d] {
			return true
		}
		if d == "." {
			return false
		}
	}
}

// entries counts the documents a rendered region lists. Headings and blank
// lines are not claims about what a directory holds; the bullets are.
func entries(region string) int {
	n := 0
	for line := range strings.SplitSeq(region, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "* [") {
			n++
		}
	}
	return n
}

// Sync brings every index file in line with the store and reports the ones that
// were not already current. With write false it changes nothing, which is how
// the check reports staleness without touching the tree.
//
// It never deletes an index file. A generator that owned whole files would
// remove one whose directory no longer holds documents, and take the
// human-written prose with it.
func Sync(root string, cfg *config.Config, write bool) ([]string, error) {
	files, err := Build(root, cfg)
	if err != nil {
		return nil, err
	}

	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	var changed []string
	for _, f := range files {
		existing, err := r.ReadFile(f.Path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}

		next := apply(string(existing), f.Region, f.Path)
		if string(existing) == next {
			continue
		}

		changed = append(changed, f.Path)
		if write {
			if err := r.WriteFile(f.Path, []byte(next), 0o644); err != nil { //nolint:gosec // a listing is committed, not generated output
				return nil, err
			}
		}
	}
	return changed, nil
}

// apply splices the region into an existing listing, giving the store root its
// format declaration when the file is new.
func apply(existing, region, indexPath string) string {
	next := target.ApplyBlock(existing, Marker, region)
	if strings.TrimSpace(existing) == "" && indexPath == Name {
		return rootFrontmatter + "\n" + next
	}
	return next
}
