package plan

import (
	"fmt"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/target"
)

// resolveAgent turns an agent id into everything a target needs: the system
// prompt and the harness fields the store declared.
func resolveAgent(b *bundle.Bundle, id string, bodyByPath map[string][]byte) (*target.Agent, error) {
	entry, ok := catalogEntry(b, id)
	if !ok {
		return nil, fmt.Errorf("bundle has no document %q to materialize as an agent", id)
	}

	body, ok := bodyByPath[entry.Path]
	if !ok {
		return nil, fmt.Errorf("agent %s at %s is absent from the store tree", id, entry.Path)
	}

	block, _ := entry.Frontmatter["agent"].(map[string]any)
	desc, _ := entry.Frontmatter["description"].(string)

	return &target.Agent{
		ID:             id,
		Name:           agentName(id, block),
		Description:    desc,
		Tools:          stringsFrom(block, "tools"),
		Model:          stringFrom(block, "model"),
		Effort:         stringFrom(block, "effort"),
		Color:          stringFrom(block, "color"),
		PermissionMode: stringFrom(block, "permission-mode"),
		Prose:          doc.Prose(body),
	}, nil
}

// agentName prefers the name the store declared and falls back to the last
// segment of the id. The harness identifies an agent by this name rather than by
// its path, so it is what a person types.
func agentName(id string, block map[string]any) string {
	if n := stringFrom(block, "name"); n != "" {
		return n
	}
	if i := strings.LastIndex(id, "."); i >= 0 && i+1 < len(id) {
		return id[i+1:]
	}
	return id
}

func stringFrom(block map[string]any, key string) string {
	if block == nil {
		return ""
	}
	s, _ := block[key].(string)
	return s
}

func stringsFrom(block map[string]any, key string) []string {
	if block == nil {
		return nil
	}
	list, ok := block[key].([]any)
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
