// Package facet is the record a digested session produces, and the only thing
// about a session that is ever intended to leave the machine.
//
// That makes it the wire format, so it carries its own version from the first
// line written, and it is self-contained: a reader needs no other file to know
// which repository a record describes or which bundles were installed while it
// was made. Repository identity in particular cannot be added later, because a
// record that does not say which repository it meant cannot be told.
package facet

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Version is the record's schema version.
const Version = 1

// Facet is one digested session.
type Facet struct {
	Version int    `json:"facet_version"`
	Session string `json:"session_id"`
	// Repo identifies the repository, never the checkout. See repo.Identity.
	Repo     string `json:"repo"`
	Worktree string `json:"worktree,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Harness  string `json:"harness,omitempty"`
	Model    string `json:"model,omitempty"`
	// Bundles is what was installed while the session ran. A session digested
	// from a transcript that predates the manifest has none, and then this
	// record can locate a gap but can say nothing about whether a bundle helped.
	Bundles   []Bundle  `json:"bundles,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`

	// ToolCalls is the whole session's count, the denominator DiscoverySpan is
	// read against.
	ToolCalls int `json:"tool_calls"`
	// DiscoverySpan is how many tool calls came before the first edit landed.
	// It is the primary metric: whether context reduces flailing. A session that
	// never edited anything has none rather than a large one, since it was not
	// necessarily flailing, it may not have been that kind of session.
	DiscoverySpan *int `json:"discovery_span,omitempty"`
	// StoreReads are the knowledge documents the agent chose to open.
	StoreReads []StoreRead `json:"store_reads,omitempty"`
	// Outcome is what the session produced, as far as the transcript shows.
	Outcome Outcome `json:"outcome"`
	// Prompts is how many times a person had to say something. A session that
	// took ten prompts to land an edit went differently from one that took one,
	// and both are cheap to count.
	Prompts int `json:"prompts"`

	// Questions is what the session was trying to establish, in its own words.
	// Deriving it needs a model, so a record written by structural extraction
	// alone leaves it empty rather than guessing.
	Questions []Question `json:"questions,omitempty"`

	// Source records how this record was made, so a later reader can tell a
	// record that had a model's help from one that did not.
	Source string `json:"source"`
}

// Bundle names one bundle installed in the repository.
type Bundle struct {
	Source string `json:"source"`
	Digest string `json:"digest"`
}

// StoreRead is one knowledge document the agent opened.
type StoreRead struct {
	// ID is the document's frontmatter id, and Path is where it was read from.
	// Both are kept: the id is the stable key, and the path is what the
	// transcript actually recorded, so a document that has since moved or been
	// deleted is still attributable.
	ID   string `json:"doc_id,omitempty"`
	Path string `json:"path"`
	// ArrivedVia says whether an index was read first or the document was opened
	// directly. Retrieval that needs the index is working; retrieval that never
	// touches it may mean the index is not worth reading.
	ArrivedVia string `json:"arrived_via"`
	// FollowedByExploration reports whether the agent kept searching afterward.
	// A document read and then followed by more exploration was consulted and
	// did not answer, which is the strongest removal signal short of a
	// correction.
	FollowedByExploration bool      `json:"followed_by_further_exploration"`
	At                    time.Time `json:"at"`
}

// How a document was reached.
const (
	ArrivedViaIndex  = "index"
	ArrivedViaDirect = "direct"
)

// Outcome is what a session produced.
type Outcome string

const (
	OutcomeEditLanded Outcome = "edit_landed"
	OutcomeCommitted  Outcome = "committed"
	OutcomePROpened   Outcome = "pr_opened"
	OutcomeExplored   Outcome = "explored"
	OutcomeUnknown    Outcome = "unknown"
)

// How a record was produced.
const (
	SourceStructural = "structural"
	SourceModel      = "structural+model"
)

// Question is one thing a session was trying to establish. Populated only by a
// pass that has a model available.
type Question struct {
	Question   string   `json:"question"`
	Resolution string   `json:"resolution"`
	Resolved   bool     `json:"resolved"`
	ToolCalls  int      `json:"tool_calls"`
	StoreDocs  []string `json:"store_docs_read,omitempty"`
}

// How a question was answered.
const (
	ResolutionStoreRead       = "store_read"
	ResolutionSourceRead      = "source_read"
	ResolutionBashExploration = "bash_exploration"
	ResolutionAskedHuman      = "asked_human"
	ResolutionUnresolved      = "unresolved"
)

// Dir returns the facet directory. QM_FACET_DIR overrides it, which is what
// tests and CI use to stay off a developer's real corpus.
func Dir() (string, error) {
	if override := os.Getenv("QM_FACET_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".quartermaster", "facets"), nil
}

// Save writes one record, replacing any earlier record for the same session.
// Digesting is idempotent by session id, so a re-run corrects rather than
// duplicates.
func Save(f Facet) error {
	if f.Version == 0 {
		f.Version = Version
	}

	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, safeName(f.Session)+".json"), append(raw, '\n'), 0o600)
}

// Load reads one record by session id.
func Load(session string) (*Facet, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, safeName(session)+".json")) //nolint:gosec // path is derived from Dir
	if err != nil {
		return nil, err
	}

	var f Facet
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Have reports whether a session has already been digested.
func Have(session string) bool {
	dir, err := Dir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, safeName(session)+".json"))
	return err == nil
}

// LoadAll reads every record, oldest session first.
func LoadAll() ([]Facet, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var out []Facet
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // path is derived from Dir
		if err != nil {
			continue
		}
		var f Facet
		if err := json.Unmarshal(raw, &f); err != nil {
			continue // one unreadable record, not an unreadable corpus
		}
		out = append(out, f)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].EndedAt.Before(out[j].EndedAt) })
	return out, nil
}

// safeName keeps a session id from escaping the facet directory. Ids come from
// a harness rather than from us, so they are not trusted to be filenames.
func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, id)
}
