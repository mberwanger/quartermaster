package plan

import (
	"fmt"
	"path"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/target"
)

// resolveSkill turns a skill id into everything a target needs to render it: the
// prose, the harness fields the store declared, and the assets that live beside
// it in the store.
func resolveSkill(b *bundle.Bundle, id string, bodyByPath map[string][]byte) (*target.Skill, error) {
	entry, ok := catalogEntry(b, id)
	if !ok {
		return nil, fmt.Errorf("bundle has no document %q to materialize as a skill", id)
	}

	body, ok := bodyByPath[entry.Path]
	if !ok {
		return nil, fmt.Errorf("skill %s at %s is absent from the store tree", id, entry.Path)
	}

	desc, _ := entry.Frontmatter["description"].(string)
	title, _ := entry.Frontmatter["title"].(string)

	s := &target.Skill{
		ID:           id,
		Name:         skillName(id, entry.Frontmatter),
		Title:        title,
		Description:  desc,
		AllowedTools: allowedTools(entry.Frontmatter),
		Prose:        doc.Prose(body),
		Assets:       assetsFor(b, path.Dir(entry.Path), entry.Path),
	}
	return s, nil
}

func catalogEntry(b *bundle.Bundle, id string) (bundle.Entry, bool) {
	for _, e := range b.Catalog {
		if e.ID == id {
			return e, true
		}
	}
	return bundle.Entry{}, false
}

// skillName prefers the name the store declared and falls back to the last
// segment of the id, so a store need not repeat itself.
func skillName(id string, fm map[string]any) string {
	if block, ok := fm["skill"].(map[string]any); ok {
		if n, _ := block["name"].(string); n != "" {
			return n
		}
	}
	if i := strings.LastIndex(id, "."); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

func allowedTools(fm map[string]any) []string {
	block, ok := fm["skill"].(map[string]any)
	if !ok {
		return nil
	}
	list, ok := block["allowed-tools"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// assetsFor collects the files beside a skill document, with paths relative to
// the skill's directory so they render into the shape they were authored in.
//
// Reserved files are left out. A directory listing describes a place in the
// store, not a part of the skill, and shipping one into a materialized skill
// would put a stale map of somebody else's repository inside it.
func assetsFor(b *bundle.Bundle, dir, skillPath string) []target.File {
	var out []target.File
	for _, f := range b.Files {
		if f.Path == skillPath || !strings.HasPrefix(f.Path, dir+"/") {
			continue
		}
		if doc.Reserved(path.Base(f.Path)) {
			continue
		}
		out = append(out, target.File{
			Path: strings.TrimPrefix(f.Path, dir+"/"),
			Body: f.Body,
		})
	}
	return out
}
