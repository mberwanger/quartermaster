// Package trace spools a pointer to each agent session, so a later pass can
// read the transcript and derive what the session actually needed.
//
// A session record is a pointer and not a summary. Everything about what
// happened is already in the transcript the harness wrote, and re-reading it is
// cheap and repeatable; what the transcript cannot reconstruct is the repository
// state at the time, which is the bundle set. So that is what this records,
// alongside enough identity to join on later.
//
// The spool is append-only and user-scoped rather than per repository, because
// the interesting signal is cross-repository even on one machine. Nothing here
// reads prompt or file content, and nothing here is evaluative about a person.
package trace

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Version is the session record's own version. It is written on every line so
// that a reader can tell what it is looking at after the shape changes, and so
// that changing the shape does not mean discarding what came before.
const Version = 1

// Session is one line of the spool: where a session's transcript is, and what
// the repository looked like while it ran.
type Session struct {
	Version int    `json:"v"`
	ID      string `json:"session_id"`
	// Harness names which agent tool wrote the payload this came from, since
	// the transcript's shape depends on it.
	Harness string `json:"harness,omitempty"`
	// TranscriptPath is where the session's own record lives, on this machine.
	// It is a local path and is deliberately not the content: raw transcripts
	// hold pasted credentials and candid remarks, and they stay put.
	TranscriptPath string `json:"transcript_path,omitempty"`
	// Repo identifies the repository rather than the checkout, so every
	// worktree of one repository counts as one. See repo.Identity.
	Repo string `json:"repo"`
	// Worktree and Branch are separate dimensions from Repo, never substitutes
	// for it. Two worktrees running different bundles against comparable work is
	// the only clean before-and-after a single operator can run.
	Worktree string `json:"worktree,omitempty"`
	Branch   string `json:"branch,omitempty"`
	// Bundles is every bundle installed in the repository when the session
	// ended. This is the one field the transcript cannot reconstruct, and
	// without it a later analysis can count what happened but can never
	// attribute a change to the document that caused it.
	Bundles []Bundle  `json:"bundles,omitempty"`
	EndedAt time.Time `json:"ended_at"`
}

// Bundle names one installed bundle. The digest is the identity; the source
// travels with it because the repository's state file will have moved on by the
// time anything reads this.
type Bundle struct {
	Source string `json:"source"`
	Digest string `json:"digest"`
}

// fileName is the spool inside Dir.
const fileName = "pending.jsonl"

// Dir returns the spool directory. QM_TRACE_DIR overrides it, which is what
// tests and CI use to stay off a developer's real spool.
func Dir() (string, error) {
	if override := os.Getenv("QM_TRACE_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".quartermaster", "spool"), nil
}

// Append writes one session record.
//
// Sessions end concurrently, since worktrees exist so that several can run at
// once. The record is small enough to land in a single write and the file is
// opened O_APPEND, so two sessions ending together cannot interleave into a
// line that parses as neither.
func Append(s Session) error {
	if s.Version == 0 {
		s.Version = Version
	}

	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	line, err := json.Marshal(s)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path is derived from Dir
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Close()
}

// Load reads the spool, oldest first, keeping one record per session id.
//
// A malformed line is skipped rather than fatal: a hook interrupted mid-write
// must cost one session, not the whole spool. A session recorded twice keeps the
// later record, since a repeated SessionEnd is a re-run rather than a second
// session.
func Load() ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filepath.Join(dir, fileName)) //nolint:gosec // path is derived from Dir
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	byID := map[string]Session{}
	var order []string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var s Session
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			continue // a torn line loses one session, not the file
		}
		if s.ID == "" {
			continue
		}
		if _, seen := byID[s.ID]; !seen {
			order = append(order, s.ID)
		}
		byID[s.ID] = s
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	out := make([]Session, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].EndedAt.Before(out[j].EndedAt) })
	return out, nil
}
