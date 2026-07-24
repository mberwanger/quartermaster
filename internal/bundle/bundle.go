// Package bundle packages a store into the artifact agents consume.
//
// This is packaging, not transformation. The store tree is carried verbatim
// under store/, and everything else in the artifact is a derived view that
// could be deleted and rebuilt from it. That keeps the markdown authoritative
// by construction rather than by intention, so no derived file can quietly
// become the thing people edit.
//
// Two consumers are served without choosing between them. An agent with file
// tools mounts store/ and greps it, which is how a coding agent actually works.
// An agent that stuffs a prompt takes store.md, which is stable per digest and
// so caches as a fixed prefix. catalog.json is the frontmatter of every doc, for
// orientation and tooling.
//
// A bundle additionally carries rulesets: named selections of documents,
// compiled at build time against the store's gate. A ruleset holds no prose,
// only references, so a consumer renders it into a harness without evaluating
// any predicate. Everything in the bundle is retrievable; a ruleset is what
// makes a subset resident.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/ruleset"
)

// Version is the artifact layout's own version, independent of the Open
// Knowledge Format version the store declares. 0.3 adds compiled rulesets to
// the 0.2 layout.
const Version = "0.3"

// okfVersion is the format version the store declares at its root index.
const okfVersion = "0.1"

// Meta is the artifact header, written to meta.json.
type Meta struct {
	Format     string `json:"format"`
	OKFVersion string `json:"okf_version"`
	Name       string `json:"name,omitempty"`
	Source     Source `json:"source"`
	Docs       int    `json:"docs"`
	Files      int    `json:"files"`
	Rulesets   int    `json:"rulesets"`
	Controls   int    `json:"controls"`
	StoreBytes int    `json:"store_bytes"`
	Digest     string `json:"digest"`
}

// Source records where the artifact was built from, so a claim an agent makes
// can be traced back to the exact commit that produced the corpus.
type Source struct {
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
}

// Entry is one doc in the catalog. Frontmatter is carried whole rather than
// projected down to the fields retrieval would want, because projecting is a
// transformation and this artifact does not transform.
type Entry struct {
	ID          string         `json:"id"`
	Path        string         `json:"path"`
	Frontmatter map[string]any `json:"frontmatter"`
}

// File is one file of the store tree, carried verbatim.
type File struct {
	Path string
	Body []byte
}

// Bundle is the built artifact, ready to write.
type Bundle struct {
	Meta     Meta
	Catalog  []Entry
	Rulesets []ruleset.Compiled
	StoreMD  string
	Files    []File
	// Controls are fixtures for the review and audit jobs, partitioned out of
	// everything an agent grounds on.
	Controls []File
}

// Options configures a build.
type Options struct {
	// Root is the store root.
	Root string
	// Config is the parsed bundle.yaml.
	Config *config.Config
	// Rulesets is the parsed rulesets file.
	Rulesets ruleset.File
	// Repo and Commit are recorded in the bundle for traceability.
	Repo   string
	Commit string
}

// outputSkip keeps a build from reading its own previous output back in.
var outputSkip = []string{"dist"}

// Build packages the store at opts.Root.
//
// A document missing frontmatter is an error rather than a warning. Validate
// reports it and the merge gate stops it, so reaching a build means the store is
// already known to be well-formed.
//
// Restricted documents never enter the artifact — not the catalog, not the
// concatenated corpus, not the tree. Materialized knowledge lands in working
// trees across every consuming repository, a wider blast radius than retrieval
// from a single store, so the visibility check is enforced here unconditionally
// and not left to the gate.
func Build(opts Options) (*Bundle, error) {
	all, err := doc.Load(opts.Root, outputSkip)
	if err != nil {
		return nil, err
	}

	// Keep only files the store declares as documents.
	var docs []doc.Doc
	for _, d := range all {
		if opts.Config.IsDocument(d.Path) {
			docs = append(docs, d)
		}
	}

	for _, d := range docs {
		if d.Frontmatter == nil {
			return nil, fmt.Errorf("%s: no frontmatter, run validate", d.Path)
		}
	}

	// A restricted document never leaves the store, so no ruleset may reference
	// one. This is checked here rather than left to the declared requirements: a
	// store that omits visibility from requires, or declares no requires at all,
	// must still not be able to ship a restricted document as a rule. Without
	// this the build would succeed and emit a ruleset pointing at a file the
	// bundle does not carry, and the failure would surface later in a consuming
	// repository, far from the author who caused it.
	if err := checkRestrictedRefs(opts.Rulesets, docs); err != nil {
		return nil, err
	}

	// Rulesets are compiled against every document, so a reference to a
	// document that falls short fails with the reason rather than a misleading
	// "unknown id".
	compiled, err := ruleset.Compile(opts.Rulesets, docs, opts.Config.Requires)
	if err != nil {
		return nil, err
	}

	// Link targets resolve only to documents that will actually ship. A link to
	// a restricted document dangles in the materialized tree, so it is not a
	// valid target.
	byPath := make(map[string]string, len(docs))
	for _, d := range docs {
		if restricted(d) {
			continue
		}
		byPath["/"+d.Path] = d.ID()
	}

	b := &Bundle{
		Meta: Meta{
			Format:     Version,
			OKFVersion: okfVersion,
			Name:       opts.Config.Name,
			Source:     Source{Repo: opts.Repo, Commit: opts.Commit},
		},
		Catalog:  []Entry{},
		Rulesets: compiled,
	}

	var store strings.Builder
	store.WriteString("# Knowledge store\n\n")
	store.WriteString("Every document in the store, concatenated in path order. ")
	store.WriteString("Each block is preceded by its path, which carries information the prose does not.\n")

	for _, d := range docs {
		if restricted(d) {
			continue
		}
		if err := checkLinks(d.Prose, byPath); err != nil {
			return nil, fmt.Errorf("%s: %w", d.Path, err)
		}

		// A control fixture is still linked and still validated, it just never
		// reaches the catalog or the concatenated corpus.
		if opts.Config.IsControl(d.Path) {
			continue
		}

		b.Catalog = append(b.Catalog, Entry{
			ID:          d.ID(),
			Path:        d.Path,
			Frontmatter: d.Frontmatter,
		})

		fmt.Fprintf(&store, "\n\n---\n\n## %s\n\n", d.Path)
		store.Write(trimTitle(d.Prose))
	}

	b.StoreMD = store.String()

	// The verbatim tree carries the same set: document and control files, minus
	// restricted ones.
	restrictedPaths := make(map[string]bool)
	for _, d := range docs {
		if restricted(d) {
			restrictedPaths[d.Path] = true
		}
	}
	files, err := tree(opts.Root, opts.Config)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if restrictedPaths[f.Path] {
			continue
		}
		if opts.Config.IsControl(f.Path) {
			b.Controls = append(b.Controls, f)
			continue
		}
		b.Files = append(b.Files, f)
	}

	b.Meta.Docs = len(b.Catalog)
	b.Meta.Files = len(b.Files)
	b.Meta.Rulesets = len(b.Rulesets)
	b.Meta.Controls = len(b.Controls)
	b.Meta.StoreBytes = len(b.StoreMD)

	digest, err := b.digest()
	if err != nil {
		return nil, err
	}
	b.Meta.Digest = digest

	return b, nil
}

// restricted reports whether a document must never enter the artifact.
func restricted(d doc.Doc) bool {
	return d.Str("visibility") == "restricted"
}

// checkRestrictedRefs fails the build when a ruleset names a restricted document.
func checkRestrictedRefs(rulesets ruleset.File, docs []doc.Doc) error {
	byID := make(map[string]doc.Doc, len(docs))
	for _, d := range docs {
		if id := d.ID(); id != "" {
			byID[id] = d
		}
	}

	for _, name := range rulesets.Names() {
		for _, ref := range rulesets[name].Docs {
			if d, ok := byID[ref.ID]; ok && restricted(d) {
				return fmt.Errorf("ruleset %q references %q, which is restricted and never leaves the store",
					name, ref.ID)
			}
		}
	}
	return nil
}

// trimTitle drops a doc's leading h1 and the blank line after it. The path
// heading above it already names the doc, and two titles in a row reads as an
// error to anything summarising the file.
func trimTitle(prose []byte) []byte {
	s := strings.TrimLeft(string(prose), "\n")
	if !strings.HasPrefix(s, "# ") {
		return []byte(s)
	}
	if _, rest, ok := strings.Cut(s, "\n"); ok {
		return []byte(strings.TrimLeft(rest, "\n"))
	}
	return nil
}

// mdLink matches an inline markdown link with a store-absolute target.
var mdLink = regexp.MustCompile(`\[[^\]]*\]\((/[^)\s]+)\)`)

// checkLinks fails the build on an inline link that resolves to no doc. Fenced
// blocks are skipped so a document that documents the link rule with a fenced
// example is not failed on its own explanation.
func checkLinks(prose []byte, byPath map[string]string) error {
	var inFence bool
	var fenceTag string

	for l := range strings.SplitSeq(string(prose), "\n") {
		if tag := fenceOpen(l); tag != "" {
			switch {
			case !inFence:
				inFence, fenceTag = true, tag
			case tag == fenceTag:
				inFence, fenceTag = false, ""
			}
			continue
		}
		if inFence {
			continue
		}

		for _, m := range mdLink.FindAllStringSubmatch(l, -1) {
			target, _, _ := strings.Cut(m[1], "#")
			if !strings.HasSuffix(target, ".md") {
				continue
			}
			if _, ok := byPath[target]; !ok {
				return fmt.Errorf("link to %s resolves to no doc", target)
			}
		}
	}
	return nil
}

// fenceOpen returns the fence marker when a line opens or closes a fenced block,
// and the empty string otherwise.
func fenceOpen(line string) string {
	t := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(t, "```"):
		return "```"
	case strings.HasPrefix(t, "~~~"):
		return "~~~"
	}
	return ""
}

// tree collects the store files carried verbatim into the artifact: the
// document and control markdown, read straight from disk. It reuses the config's
// document and control predicates, so the tree carries exactly what the catalog
// and rulesets were built from.
func tree(root string, cfg *config.Config) ([]File, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	fsys := r.FS()

	var files []File

	err = fs.WalkDir(fsys, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if p != "." && strings.HasPrefix(e.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !cfg.IsDocument(p) && !cfg.IsControl(p) {
			return nil
		}

		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files = append(files, File{Path: p, Body: body})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// digest is the artifact's content address, computed over the catalog, the
// compiled rulesets, the concatenated store, and every carried file. It excludes
// the source commit, so two builds of identical content agree even from
// different commits, which is what makes the digest usable as a cache key and as
// an eval pin.
func (b *Bundle) digest() (string, error) {
	h := sha256.New()

	if err := json.NewEncoder(h).Encode(b.Catalog); err != nil {
		return "", err
	}
	if err := json.NewEncoder(h).Encode(b.Rulesets); err != nil {
		return "", err
	}
	if _, err := h.Write([]byte(b.StoreMD)); err != nil {
		return "", err
	}
	for _, set := range [][]File{b.Files, b.Controls} {
		for _, f := range set {
			if _, err := fmt.Fprintf(h, "%s\n", f.Path); err != nil {
				return "", err
			}
			if _, err := h.Write(f.Body); err != nil {
				return "", err
			}
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
