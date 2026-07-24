// Package validate enforces a store's machine-checkable rules: frontmatter
// matches the schema, every id is unique, and every supersede link resolves to
// a doc that exists.
//
// These are the rules CI gates on. Everything a machine cannot decide, whether
// the content is actually true, is left to the human who merges the pull
// request.
package validate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/doc"
)

// Finding is one problem with one doc.
type Finding struct {
	Path    string
	Message string
}

// Result is what a run found.
type Result struct {
	Findings []Finding
	// Checked counts the docs that carried frontmatter.
	Checked int
}

// OK reports whether the run found no problems.
func (r Result) OK() bool {
	return len(r.Findings) == 0
}

// outputSkip keeps validation from reading a previous build's output.
var outputSkip = []string{"dist"}

// Run validates every document under root, using the store's declared schema and
// include/exclude patterns.
func Run(root string, cfg *config.Config) (Result, error) {
	var schema *jsonschema.Schema
	if cfg.Schema != "" {
		s, err := compile(filepath.Join(root, cfg.Schema))
		if err != nil {
			return Result{}, err
		}
		schema = s
	}

	all, err := doc.Load(root, outputSkip)
	if err != nil {
		return Result{}, err
	}

	var docs []doc.Doc
	for _, d := range all {
		if cfg.IsDocument(d.Path) {
			docs = append(docs, d)
		}
	}

	var res Result
	ids := make(map[string]string, len(docs))

	for _, d := range docs {
		if d.Frontmatter == nil {
			res.Findings = append(res.Findings, Finding{
				Path:    d.Path,
				Message: "missing frontmatter block, only index.md and log.md may omit it",
			})
			continue
		}
		res.Checked++

		if schema != nil {
			res.Findings = append(res.Findings, schemaFindings(schema, d)...)
		}

		id := d.ID()
		if id == "" {
			continue // a missing id is already a schema finding
		}
		if first, dup := ids[id]; dup {
			res.Findings = append(res.Findings, Finding{
				Path:    d.Path,
				Message: fmt.Sprintf("duplicate id %q, already used by %s", id, first),
			})
			continue
		}
		ids[id] = d.Path
	}

	// Link resolution needs every id first, so it is a second pass.
	for _, d := range docs {
		if d.Frontmatter == nil {
			continue
		}
		for _, ref := range idList(d.Frontmatter["supersedes"]) {
			if _, ok := ids[ref]; !ok {
				res.Findings = append(res.Findings, Finding{
					Path:    d.Path,
					Message: fmt.Sprintf("supersedes unknown id %q", ref),
				})
			}
		}
		if by, ok := d.Frontmatter["superseded_by"].(string); ok && by != "" {
			if _, ok := ids[by]; !ok {
				res.Findings = append(res.Findings, Finding{
					Path:    d.Path,
					Message: fmt.Sprintf("superseded_by unknown id %q", by),
				})
			}
		}
	}

	reserved, err := doc.LoadReserved(root, outputSkip)
	if err != nil {
		return Result{}, err
	}
	res.Findings = append(res.Findings, conformance(reserved)...)

	sort.Slice(res.Findings, func(i, j int) bool {
		if res.Findings[i].Path != res.Findings[j].Path {
			return res.Findings[i].Path < res.Findings[j].Path
		}
		return res.Findings[i].Message < res.Findings[j].Message
	})
	return res, nil
}

// logDate matches the ISO 8601 date headings a log.md groups its entries under.
var logDate = regexp.MustCompile(`^## \d{4}-\d{2}-\d{2}\s*$`)

// conformance checks the Open Knowledge Format rules that apply to reserved
// files, which the schema cannot cover because these files carry no frontmatter.
func conformance(files []doc.ReservedFile) []Finding {
	var out []Finding

	for _, f := range files {
		switch path.Base(f.Path) {
		case doc.IndexName:
			// OKF §11: the bundle root index is the only index file permitted
			// frontmatter, and only to declare the format version.
			if f.Path != doc.IndexName && bytes.HasPrefix(f.Body, []byte("---")) {
				out = append(out, Finding{
					Path:    f.Path,
					Message: "index.md carries frontmatter, which only the store root may do",
				})
			}

		case doc.LogName:
			// OKF §7: entries are grouped under ISO 8601 date headings.
			for i, line := range strings.Split(string(f.Body), "\n") {
				if strings.HasPrefix(line, "## ") && !logDate.MatchString(line) {
					out = append(out, Finding{
						Path:    f.Path,
						Message: fmt.Sprintf("line %d: log heading %q is not an ISO 8601 date", i+1, line),
					})
				}
			}
		}
	}

	return out
}

// compile loads the schema from disk. Format is asserted rather than treated as
// an annotation, which is the JSON Schema default, so a malformed date or url
// fails CI instead of passing silently.
func compile(path string) (*jsonschema.Schema, error) {
	f, err := os.Open(path) //nolint:gosec // the schema path is a CLI flag, chosen by whoever runs the tool
	if err != nil {
		return nil, fmt.Errorf("open schema: %w", err)
	}
	defer func() { _ = f.Close() }()

	raw, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	const url = "frontmatter.schema.json"
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(url, raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	schema, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return schema, nil
}

func schemaFindings(schema *jsonschema.Schema, d doc.Doc) []Finding {
	inst, err := instance(d.Frontmatter)
	if err != nil {
		return []Finding{{Path: d.Path, Message: err.Error()}}
	}

	err = schema.Validate(inst)
	if err == nil {
		return nil
	}

	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []Finding{{Path: d.Path, Message: err.Error()}}
	}

	var out []Finding
	for _, u := range ve.BasicOutput().Errors {
		if u.Error == nil {
			continue
		}
		where := u.InstanceLocation
		if where == "" {
			where = "/"
		}
		out = append(out, Finding{
			Path:    d.Path,
			Message: fmt.Sprintf("%s %s", where, u.Error.String()),
		})
	}
	if len(out) == 0 {
		out = append(out, Finding{Path: d.Path, Message: ve.Error()})
	}
	return out
}

func instance(m map[string]any) (any, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

// idList reads a frontmatter value that should be a list of ids.
func idList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
