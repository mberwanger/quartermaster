// Package bundle packages a store into the artifact agents consume.
//
// This is packaging, not transformation. The store tree is carried verbatim
// under store/, and everything else in the artifact is a derived view that
// could be deleted and rebuilt from it. That keeps the markdown authoritative
// by construction rather than by intention, so no derived file can quietly
// become the thing people edit.
//
// The consumer this is built for is an agent with file tools: it mounts
// store/ and greps it, which is how a coding agent actually works.
// catalog.json is the frontmatter of every doc, for orientation and tooling.
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
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/pack"
)

// Version is the artifact layout's own version, independent of the Open
// Knowledge Format version the store declares. 0.4 replaces the 0.3 layout's
// rulesets, which selected documents only, with packages, which select rules,
// skills, and agents together under one name.
const Version = "0.4"

// defaultOKFVersion preserves the format assumed before root declarations were
// propagated into bundle metadata.
const defaultOKFVersion = "0.1"

// Meta is the artifact header, written to meta.json.
type Meta struct {
	Format     string `json:"format"`
	OKFVersion string `json:"okf_version"`
	Name       string `json:"name,omitempty"`
	Source     Source `json:"source"`
	Docs       int    `json:"docs"`
	Files      int    `json:"files"`
	Packages   int    `json:"packages"`
	Controls   int    `json:"controls"`
	// StoreBytes is the total size of the store/ tree carried into the
	// artifact, summed directly from the carried files.
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
	Packages []pack.Compiled
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
	// Packages is the parsed packages file.
	Packages pack.File
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
// tree. Materialized knowledge lands in working trees across every consuming
// repository, a wider blast radius than retrieval from a single store, so the
// visibility check is enforced here unconditionally and not left to the gate.
func Build(opts Options) (*Bundle, error) {
	okfVersion, err := loadOKFVersion(opts.Root)
	if err != nil {
		return nil, err
	}

	all, err := doc.Load(opts.Root, outputSkip)
	if err != nil {
		return nil, err
	}

	// Keep only files the store declares as documents.
	var docs []doc.Doc
	for _, d := range all {
		if opts.Config.IsDistributed(d.Path) {
			docs = append(docs, d)
		}
	}

	// A skill's assets travel with it rather than standing alone, so they are
	// carried into the tree but never catalogued and never held to the
	// frontmatter rule.
	skillDirs, skillPaths := opts.Config.SkillDirs(docs)
	kept := docs[:0]
	for _, d := range docs {
		if !config.IsAsset(d.Path, skillDirs, skillPaths) {
			kept = append(kept, d)
		}
	}
	docs = kept

	for _, d := range docs {
		if d.Frontmatter == nil {
			return nil, fmt.Errorf("%s: no frontmatter, run validate", d.Path)
		}
	}

	if err := checkAgentPermissions(docs); err != nil {
		return nil, err
	}

	// Packages are compiled against every document, so a selection naming a
	// document that falls short fails with the reason rather than a misleading
	// "unknown id". A restricted document is refused as any kind, which is
	// enforced there rather than left to the declared requirements: a store that
	// omits visibility from requires, or declares none at all, must still not be
	// able to ship one.
	skillDirSet, skillPathSet := opts.Config.SkillDirs(docs)
	_ = skillDirSet
	_ = skillPathSet
	compiled, err := pack.Compile(opts.Packages, docs, pack.Options{
		Requires: opts.Config.Requires,
		IsSkill:  func(d doc.Doc) bool { ok, _ := opts.Config.Skills.Allows(d.Frontmatter); return ok },
		IsAgent:  func(d doc.Doc) bool { _, ok := d.Frontmatter["agent"].(map[string]any); return ok },
	})
	if err != nil {
		return nil, err
	}

	// Link targets resolve only to documents that will actually ship. A link to
	// a restricted document dangles in the materialized tree, so it is not a
	// valid target.
	byPath := make(map[string]string, len(docs))
	for _, d := range docs {
		if d.Restricted() {
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
		Packages: compiled,
	}

	for _, d := range docs {
		if d.Restricted() {
			continue
		}
		if err := checkLinks(d.Prose, byPath); err != nil {
			return nil, fmt.Errorf("%s: %w", d.Path, err)
		}

		// A control fixture is still linked and still validated, it just never
		// reaches the catalog.
		if opts.Config.IsControl(d.Path) {
			continue
		}

		b.Catalog = append(b.Catalog, Entry{
			ID:          d.ID(),
			Path:        d.Path,
			Frontmatter: d.Frontmatter,
		})
	}

	// The verbatim tree carries the same set: document and control files, minus
	// restricted ones.
	restrictedPaths := make(map[string]bool)
	for _, d := range docs {
		if d.Restricted() {
			restrictedPaths[d.Path] = true
		}
	}
	files, err := tree(opts.Root, opts.Config)
	if err != nil {
		return nil, err
	}

	// An index is a navigation aid, so it may only advertise what travels with
	// it. A directory whose documents are all excluded still has an index.md on
	// disk in the store, and carrying it would put a listing in a consuming
	// repository pointing at files the bundle deliberately left behind.
	indexed := indexedDirs(docs, restrictedPaths)

	for _, f := range files {
		if restrictedPaths[f.Path] {
			continue
		}
		if doc.Reserved(path.Base(f.Path)) && !indexed[path.Dir(f.Path)] {
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
	b.Meta.Packages = len(b.Packages)
	b.Meta.Controls = len(b.Controls)
	b.Meta.StoreBytes = storeBytes(b.Files)

	digest, err := b.digest()
	if err != nil {
		return nil, err
	}
	b.Meta.Digest = digest

	return b, nil
}

func loadOKFVersion(root string) (string, error) {
	storeRoot, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer func() { _ = storeRoot.Close() }()

	indexBody, err := storeRoot.ReadFile(doc.IndexName)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultOKFVersion, nil
		}
		return "", err
	}

	frontmatter, err := doc.Parse(indexBody)
	if err != nil {
		return "", fmt.Errorf("%s: %w", doc.IndexName, err)
	}
	okfVersion, _ := frontmatter["okf_version"].(string)
	if okfVersion == "" {
		return defaultOKFVersion, nil
	}
	return okfVersion, nil
}

// indexedDirs is every directory the bundle carries a document in, plus their
// ancestors, which is exactly where an index still has something to point at.
func indexedDirs(docs []doc.Doc, restricted map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, d := range docs {
		if restricted[d.Path] {
			continue
		}
		for dir := path.Dir(d.Path); ; dir = path.Dir(dir) {
			out[dir] = true
			if dir == "." {
				break
			}
		}
	}
	return out
}

// bypassPermissions is the one permission mode a bundle may not carry.
const bypassPermissions = "bypassPermissions"

// checkAgentPermissions refuses to distribute an agent that waives the
// permission prompt.
//
// Every other kind of document a bundle carries is text: a rule advises, a skill
// informs, and a person still approves whatever the agent then tries to do. An
// agent definition is different, because it grants a capability, and this
// particular grant removes the last place a person could say no.
//
// Arriving by sync is what makes it unacceptable rather than the mode itself. A
// repository that genuinely wants it can write the agent by hand, deliberately
// and locally, which is a decision somebody makes rather than one that lands in
// twenty-one repositories because a document was edited.
func checkAgentPermissions(docs []doc.Doc) error {
	for _, d := range docs {
		block, ok := d.Frontmatter["agent"].(map[string]any)
		if !ok {
			continue
		}
		if mode, _ := block["permission-mode"].(string); mode == bypassPermissions {
			return fmt.Errorf("%s declares permission-mode %s, which a bundle may not distribute; write such an agent locally instead",
				d.Path, bypassPermissions)
		}
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
		if !cfg.IsDistributed(p) && !cfg.IsControl(p) {
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

// storeBytes sums the size of every file carried into the store/ tree.
func storeBytes(files []File) int {
	total := 0
	for _, f := range files {
		total += len(f.Body)
	}
	return total
}

// digest is the artifact's content address, computed over the catalog, the
// compiled packages, and every carried file. It excludes the source commit, so
// two builds of identical content agree even from different commits, which is
// what makes the digest usable as a cache key and as an eval pin.
func (b *Bundle) digest() (string, error) {
	h := sha256.New()

	if err := json.NewEncoder(h).Encode(b.Catalog); err != nil {
		return "", err
	}
	if err := json.NewEncoder(h).Encode(b.Packages); err != nil {
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
