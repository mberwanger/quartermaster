// Package state records what the last sync produced, so a later run can prune
// files it no longer produces and a verify can run without re-resolving.
//
// Without this record, removing a ruleset from the manifest would leave its
// rules materialized forever: the generated files are gitignored, so git never
// deletes them, and nothing else knows they were ours.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Dir is the per-repository directory Quartermaster owns.
const Dir = ".quartermaster"

// fileName is the state file inside Dir.
const fileName = "state.json"

// State is the record of the last sync.
type State struct {
	Bundles []Bundle `json:"bundles"`
	Targets []string `json:"targets"`
	// Files is every generated file, repository-relative and sorted. It is what
	// prune diffs against.
	Files []string `json:"files"`
}

// Bundle records a resolved source and the digest it resolved to.
type Bundle struct {
	Source   string   `json:"source"`
	Digest   string   `json:"digest"`
	Packages []string `json:"packages"`
	// Knowledge names the frontmatter fields the repository filtered the
	// knowledge tree on, so a partial tree is never mistaken for the whole one.
	Knowledge []string `json:"knowledge,omitempty"`
}

// Load reads the state from a repository directory. A missing state file is not
// an error: the zero State is the correct starting point for a repository that
// has never synced.
func Load(dir string) (*State, error) {
	raw, err := os.ReadFile(filepath.Join(dir, Dir, fileName)) //nolint:gosec // dir is a CLI flag
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{}, nil
		}
		return nil, err
	}

	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes the state, sorting Files so the record is stable across runs.
func Save(dir string, s *State) error {
	sort.Strings(s.Files)

	d := filepath.Join(dir, Dir)
	if err := os.MkdirAll(d, 0o750); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, fileName), append(raw, '\n'), 0o600)
}
