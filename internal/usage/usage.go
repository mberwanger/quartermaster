// Package usage records which documents an agent actually opened.
//
// The log is global rather than per repository, because promotion is a
// cross-repository decision: a document opened once in each of twelve
// repositories is a stronger candidate than one opened twelve times in a single
// repository, and a per-repository log cannot tell those apart.
//
// An event carries a document id, the bundles installed where it was read, the
// repository, a timestamp, and the kind of event. It never carries prompt or
// file content. If aggregation across a team is ever built on top of this, it
// stays opt-in and must never feed anything evaluative about a person.
package usage

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EventOpen records an agent reading a document out of the knowledge tree.
const EventOpen = "open"

// Event is one line of the log.
type Event struct {
	ID string `json:"id"`
	// Repo identifies the repository rather than the checkout, so every
	// worktree of one repository counts as one. See repo.Identity.
	Repo string `json:"repo"`
	// Worktree is the checkout the read happened in. It is a separate dimension
	// from Repo and never a substitute for it: a document read in three
	// worktrees of one repository is one repository's worth of evidence.
	Worktree string `json:"worktree,omitempty"`
	// Bundles is every bundle installed in that repository at the time, in
	// manifest order. A repository composes several sources and nothing here
	// knows which one carried the document, so recording only the first would
	// attribute the read to a bundle that may not contain it.
	Bundles []Bundle  `json:"bundles,omitempty"`
	Time    time.Time `json:"ts"`
	Event   string    `json:"event"`
}

// Bundle names one installed bundle. The digest is the identity; the source is
// carried with it so a later reader can tell where it came from without the
// repository's state file, which will have moved on by then.
type Bundle struct {
	Source string `json:"source"`
	Digest string `json:"digest"`
}

// Dir returns the log directory. QM_USAGE_DIR overrides it, which is what tests
// and CI use to stay off a developer's real log.
func Dir() (string, error) {
	if override := os.Getenv("QM_USAGE_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".quartermaster", "usage"), nil
}

// Append writes one event. Files are split by month so the log stays bounded and
// an old one can be dropped without parsing it.
func Append(e Event) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}

	name := filepath.Join(dir, e.Time.UTC().Format("2006-01")+".jsonl")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path is derived from Dir
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Close()
}

// Load reads every event in the log, oldest first. A malformed line is skipped
// rather than fatal: a truncated write from an interrupted hook must not make
// the whole history unreadable.
func Load() ([]Event, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []Event
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		fileEvents, err := readFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		events = append(events, fileEvents...)
	}

	sort.Slice(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, nil
}

func readFile(path string) ([]Event, error) {
	f, err := os.Open(path) //nolint:gosec // path is derived from Dir
	if err != nil {
		if err, ok := err.(*fs.PathError); ok && os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var events []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // a torn line loses one event, not the file
		}
		if e.ID != "" {
			events = append(events, e)
		}
	}
	return events, sc.Err()
}

// Doc is what the log says about one document.
type Doc struct {
	ID    string
	Opens int
	Repos []string
	Last  time.Time
}

// Aggregate collapses events into per-document counts, with the repositories a
// document was opened in, sorted by opens then id so output is stable.
func Aggregate(events []Event) []Doc {
	byID := map[string]*Doc{}
	repos := map[string]map[string]bool{}

	for _, e := range events {
		d, ok := byID[e.ID]
		if !ok {
			d = &Doc{ID: e.ID}
			byID[e.ID] = d
			repos[e.ID] = map[string]bool{}
		}
		d.Opens++
		if e.Time.After(d.Last) {
			d.Last = e.Time
		}
		if e.Repo != "" {
			repos[e.ID][e.Repo] = true
		}
	}

	out := make([]Doc, 0, len(byID))
	for id, d := range byID {
		for repo := range repos[id] {
			d.Repos = append(d.Repos, repo)
		}
		sort.Strings(d.Repos)
		out = append(out, *d)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Opens != out[j].Opens {
			return out[i].Opens > out[j].Opens
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Repos returns every repository that appears in the log.
func Repos(events []Event) []string {
	seen := map[string]bool{}
	for _, e := range events {
		if e.Repo != "" {
			seen[e.Repo] = true
		}
	}
	out := make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// Since filters events to those at or after t.
func Since(events []Event, t time.Time) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		if !e.Time.Before(t) {
			out = append(out, e)
		}
	}
	return out
}
