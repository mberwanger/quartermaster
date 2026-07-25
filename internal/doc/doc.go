// Package doc reads a knowledge store: every markdown file and the YAML
// frontmatter it carries.
//
// Frontmatter is returned in its JSON shape so it can be validated directly
// against a JSON Schema. The schema is the single owner of what a valid doc
// looks like, so nothing here re-states its rules.
package doc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Doc is one markdown file in the store together with its frontmatter.
type Doc struct {
	// Path is slash-separated and relative to the store root.
	Path string
	// Frontmatter is the parsed block, in its JSON shape.
	Frontmatter map[string]any
	// Prose is the markdown following the frontmatter block. A bundle chunks
	// this rather than the raw file, so frontmatter never reaches a retrieved
	// chunk, where it would read as content.
	Prose []byte
}

// ID returns the doc's id, or the empty string when it has none. A missing id
// is a schema violation, reported there rather than here.
func (d Doc) ID() string {
	id, _ := d.Frontmatter["id"].(string)
	return id
}

// Str returns a string-typed frontmatter field, or the empty string when the
// field is absent or not a string.
func (d Doc) Str(field string) string {
	s, _ := d.Frontmatter[field].(string)
	return s
}

// Restricted reports whether a document must never leave the store.
//
// It lives here rather than in whichever package happens to need it, because
// more than one does and the answer has to be the same everywhere. A restricted
// document is withheld from the artifact, and it must also be absent from
// anything derived from the artifact: a listing that names one leaks its title
// and description, which is most of what made it worth restricting.
func (d Doc) Restricted() bool {
	return d.Str("visibility") == "restricted"
}

// Reserved filenames, defined by the Open Knowledge Format. These are directory
// listings and update logs, not concept documents, and they are the only
// markdown in the store allowed to omit frontmatter.
const (
	IndexName = "index.md"
	LogName   = "log.md"
)

// Reserved reports whether a filename is reserved.
func Reserved(name string) bool {
	return name == IndexName || name == LogName
}

// ReservedFile is an index.md or log.md, returned with its raw bytes because
// its rules are about the file's shape rather than its frontmatter.
type ReservedFile struct {
	// Path is slash-separated and relative to the store root.
	Path string
	// Body is the file verbatim.
	Body []byte
}

// LoadReserved returns every reserved file in the store, ordered by path.
// Load deliberately ignores these, so conformance rules about them need their
// own pass.
func LoadReserved(root string, skip []string) ([]ReservedFile, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	fsys := r.FS()

	var files []ReservedFile

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
		if !Reserved(e.Name()) || skipped(p, skip) {
			return nil
		}

		body, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		files = append(files, ReservedFile{Path: p, Body: body})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// errNoBlock reports a file that does not open with a frontmatter block.
var errNoBlock = errors.New("no frontmatter block")

// Parse extracts the frontmatter from a markdown file. It returns a nil map for
// a file with no frontmatter block, which the caller reports as a problem for
// any file that is not reserved. A block that opens and never closes is an
// error rather than a nil map, since that is malformed instead of absent.
func Parse(src []byte) (map[string]any, error) {
	body, err := block(src)
	if errors.Is(err, errNoBlock) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(body, &node); err != nil {
		return nil, fmt.Errorf("frontmatter is not valid yaml: %w", err)
	}
	if len(node.Content) == 0 {
		return map[string]any{}, nil
	}

	v, err := decode(node.Content[0])
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("frontmatter is %T, want a mapping", v)
	}
	return jsonShape(m)
}

// Prose returns the markdown following the frontmatter block, or the whole
// file when it carries none. A block that opens and never closes yields
// nothing, matching Parse, which treats that as malformed rather than absent.
func Prose(src []byte) []byte {
	// SplitAfter keeps the line terminators, so the running offset stays
	// exact and the returned slice shares the caller's backing array.
	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) == 0 || fence(string(lines[0])) != "---" {
		return src
	}

	off := len(lines[0])
	for _, l := range lines[1:] {
		off += len(l)
		if f := fence(string(l)); f == "---" || f == "..." {
			return src[off:]
		}
	}
	return nil
}

// block returns the bytes between the opening and closing frontmatter fences.
func block(src []byte) ([]byte, error) {
	s := bufio.NewScanner(bytes.NewReader(src))
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !s.Scan() {
		return nil, errNoBlock
	}
	if fence(s.Text()) != "---" {
		return nil, errNoBlock
	}

	var body bytes.Buffer
	for s.Scan() {
		if f := fence(s.Text()); f == "---" || f == "..." {
			return body.Bytes(), s.Err()
		}
		body.WriteString(s.Text())
		body.WriteByte('\n')
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("frontmatter block is never closed")
}

func fence(line string) string {
	return strings.TrimSpace(strings.TrimRight(line, "\r"))
}

// decode converts a YAML node tree into plain Go values.
//
// Timestamps keep the text the author wrote. YAML resolves an unquoted
// 2026-07-01 to a timestamp, and rendering that back through time.Time would
// add a time component that the schema's "format": "date" then rejects. Authors
// should not have to quote their dates to satisfy a round trip.
func decode(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return decode(n.Content[0])

	case yaml.AliasNode:
		return decode(n.Alias)

	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := decode(n.Content[i])
			if err != nil {
				return nil, err
			}
			v, err := decode(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[fmt.Sprint(k)] = v
		}
		return m, nil

	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := decode(c)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil

	case yaml.ScalarNode:
		if n.Tag == "!!timestamp" {
			return n.Value, nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	}

	return nil, fmt.Errorf("unsupported yaml node kind %d", n.Kind)
}

// jsonShape round-trips through JSON so the validator sees exactly the types a
// JSON document would produce. Numbers stay json.Number to keep precision.
func jsonShape(m map[string]any) (map[string]any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Load walks root and returns every markdown file that carries frontmatter,
// ordered by path. Paths under a skip prefix are excluded, as is anything in a
// dot directory.
func Load(root string, skip []string) ([]Doc, error) {
	// Walk a root-scoped filesystem rather than raw paths. A symlink pointing
	// outside the store is rejected instead of followed, which matters for a
	// tree that agents write into, and it keeps every path here relative and
	// slash-separated with no conversion.
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	fsys := r.FS()

	var docs []Doc

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
		if !strings.HasSuffix(e.Name(), ".md") {
			return nil
		}
		// index.md and log.md are reserved by the Open Knowledge Format for
		// directory listings and update logs. They are not concept documents
		// and carry no frontmatter, so they are not loaded at all.
		if Reserved(e.Name()) {
			return nil
		}
		if skipped(p, skip) {
			return nil
		}

		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		fm, err := Parse(src)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}

		// A file with no frontmatter is returned with a nil map rather than
		// skipped. Skipping it would let a doc that lost its frontmatter drop
		// out of validation without anyone noticing.
		docs = append(docs, Doc{Path: p, Frontmatter: fm, Prose: Prose(src)})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}

func skipped(rel string, skip []string) bool {
	for _, s := range skip {
		s = strings.Trim(filepath.ToSlash(s), "/")
		if s == "" {
			continue
		}
		if rel == s || strings.HasPrefix(rel, s+"/") {
			return true
		}
	}
	return false
}
