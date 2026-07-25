// Package digest turns a session transcript into a facet record.
//
// Everything here is derived structurally: counted, matched, and ordered from
// what the transcript already records. No model is called, so a digest is
// reproducible, costs nothing, and can be re-run over a whole corpus whenever
// the derivation improves. That is deliberate rather than a limitation being
// worked around. The fields a model would fill in, chiefly what a session was
// trying to establish in its own words, are left empty rather than guessed, and
// a record says which kind it is.
//
// What structure alone can answer turns out to be most of what the metrics need:
// how long an agent flailed before it landed anything, which documents it chose
// to open, how it arrived at them, and whether opening one ended the search.
package digest

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/facet"
	"github.com/mberwanger/quartermaster/internal/transcript"
)

// KnowledgeDir is the materialized knowledge tree, relative to a repository.
// It is the path a read has to fall under to count as a store read.
const KnowledgeDir = ".quartermaster/knowledge"

// Repo is what the repository can say about itself at digest time. It is passed
// in rather than looked up, so digesting is a pure function of its inputs and a
// test needs no repository on disk.
type Repo struct {
	Identity string
	Worktree string
	Bundles  []facet.Bundle
}

// Tools that mean the agent is looking rather than acting. The split is what
// makes a discovery span meaningful: everything before the first act is search.
var searchTools = map[string]bool{
	"Read": true, "Grep": true, "Glob": true, "Bash": true,
	"WebFetch": true, "WebSearch": true, "Task": true, "Agent": true,
}

// Tools that change something.
var editTools = map[string]bool{
	"Edit": true, "Write": true, "NotebookEdit": true, "MultiEdit": true,
}

// Digest derives a facet from a session.
func Digest(s *transcript.Session, r Repo) facet.Facet {
	f := facet.Facet{
		Version:   facet.Version,
		Session:   s.ID,
		Repo:      r.Identity,
		Worktree:  r.Worktree,
		Branch:    s.Branch,
		Harness:   s.Harness,
		Model:     s.Model,
		Bundles:   r.Bundles,
		StartedAt: s.Started,
		EndedAt:   s.Ended,
		ToolCalls: s.ToolCalls(),
		Outcome:   facet.OutcomeUnknown,
		Source:    facet.SourceStructural,
	}

	for _, e := range s.Events {
		if e.Kind == transcript.KindPrompt {
			f.Prompts++
		}
	}

	f.DiscoverySpan = discoverySpan(s)
	f.StoreReads = storeReads(s)
	f.Outcome = outcome(s)
	f.HumanAsks = humanAsks(s)

	return f
}

// humanAsks counts the times the agent stopped and put something to a person.
//
// Only the count. Recording each one as a question the session was trying to
// answer looked free and was wrong: what agents put to a person is usually a
// decision to be made rather than a fact to be recovered, so the text is a
// naming argument or a sequencing choice far more often than a gap. Counting
// keeps the part that is true, that the session could not go on by itself, and
// throws away the part that would pollute a corpus meant for clustering.
func humanAsks(s *transcript.Session) int {
	var n int
	for _, e := range s.Events {
		if e.Kind == transcript.KindToolUse {
			n += len(e.Asked)
		}
	}
	return n
}

// discoverySpan counts the tool calls before the first edit landed.
//
// A session that edited nothing has no span rather than a large one. Reading
// code to answer a question is not flailing, and scoring it as the worst
// possible session would make the metric say the opposite of what it means.
func discoverySpan(s *transcript.Session) *int {
	var n int
	for _, e := range s.Events {
		if e.Kind != transcript.KindToolUse {
			continue
		}
		if editTools[e.Tool] {
			span := n
			return &span
		}
		if searchTools[e.Tool] {
			n++
		}
	}
	return nil
}

// storeReads finds the knowledge documents the agent opened, in order.
func storeReads(s *transcript.Session) []facet.StoreRead {
	var out []facet.StoreRead
	var sawIndex bool

	for i, e := range s.Events {
		if e.Kind != transcript.KindToolUse || e.Tool != "Read" || e.Path == "" {
			continue
		}
		rel, ok := underKnowledge(e.Path)
		if !ok {
			continue
		}

		// An index is navigation rather than an answer, so reading one is not a
		// store read. It is what the next read arrived through.
		if doc.Reserved(filepath.Base(rel)) {
			sawIndex = true
			continue
		}

		arrived := facet.ArrivedViaDirect
		if sawIndex {
			arrived = facet.ArrivedViaIndex
		}

		out = append(out, facet.StoreRead{
			ID:                    docID(e.Path),
			Path:                  rel,
			ArrivedVia:            arrived,
			FollowedByExploration: exploredAfter(s.Events[i+1:]),
			At:                    e.At,
		})
	}
	return out
}

// exploredAfter reports whether the agent went on searching before it acted.
//
// This is the failed-document signal. The document was consulted, and the agent
// carried on looking, which says the answer was not there. Searching that
// happens after an edit is the next piece of work rather than the same one, so
// the scan stops at the first edit.
func exploredAfter(rest []transcript.Event) bool {
	for _, e := range rest {
		if e.Kind != transcript.KindToolUse {
			continue
		}
		if editTools[e.Tool] {
			return false
		}
		if searchTools[e.Tool] {
			return true
		}
	}
	return false
}

// underKnowledge reports whether a path is inside a materialized knowledge tree,
// and returns the path relative to it.
//
// Matching on the directory rather than on a repository root is what makes this
// work across repositories in one pass: the transcript's paths are absolute and
// the tree is always at the same place inside whichever repository it is.
func underKnowledge(path string) (string, bool) {
	p := filepath.ToSlash(path)
	i := strings.Index(p, KnowledgeDir+"/")
	if i < 0 {
		return "", false
	}
	return p[i+len(KnowledgeDir)+1:], true
}

// docID reads the document's own frontmatter for its id.
//
// The id is the stable key that a proposal and a removal both reference, so it
// is worth a file read. A document that has since been deleted or rewritten
// yields nothing, and the record keeps the path, which is still attributable.
func docID(path string) string {
	body, err := os.ReadFile(path) //nolint:gosec // the path came out of a transcript the harness wrote
	if err != nil {
		return ""
	}
	fm, err := doc.Parse(body)
	if err != nil || fm == nil {
		return ""
	}
	id, _ := fm["id"].(string)
	return id
}

// outcome reports what the session produced, as far as the transcript shows.
//
// These are proxies and they are ordered by how much they claim. A commit or a
// pull request is a thing that happened; an edit is a thing that was written and
// may have been reverted a minute later. Nothing here can see a revert, so the
// weaker readings stay weak on purpose.
func outcome(s *transcript.Session) facet.Outcome {
	var edited, explored bool

	for _, e := range s.Events {
		if e.Kind != transcript.KindToolUse {
			continue
		}
		if editTools[e.Tool] {
			edited = true
		}
		if searchTools[e.Tool] {
			explored = true
		}
		if e.Tool == "Bash" {
			switch {
			case commandRuns(e.Command, "gh pr create"):
				return facet.OutcomePROpened
			case commandRuns(e.Command, "git commit"):
				edited = true
				explored = true
				// keep scanning: a pull request may still follow
				continue
			}
		}
	}

	switch {
	case edited && committed(s):
		return facet.OutcomeCommitted
	case edited:
		return facet.OutcomeEditLanded
	case explored:
		return facet.OutcomeExplored
	default:
		return facet.OutcomeUnknown
	}
}

func committed(s *transcript.Session) bool {
	for _, e := range s.Events {
		if e.Kind == transcript.KindToolUse && e.Tool == "Bash" && commandRuns(e.Command, "git commit") {
			return true
		}
	}
	return false
}

// commandRuns reports whether a shell command actually runs the named program,
// rather than merely mentioning it. A command that greps for "git commit" is not
// a commit, and a session that searched for one should not be recorded as having
// made one.
func commandRuns(command, want string) bool {
	for _, seg := range splitCommand(command) {
		if strings.HasPrefix(strings.TrimSpace(seg), want) {
			return true
		}
	}
	return false
}

// splitCommand breaks a shell command at the separators that start a new
// command, so each segment begins where a program name would.
func splitCommand(command string) []string {
	fields := strings.FieldsFunc(command, func(r rune) bool {
		return r == '\n' || r == ';' || r == '|' || r == '&'
	})
	return fields
}

// Elapsed is how long the session ran, for a report that wants it.
func Elapsed(f facet.Facet) time.Duration {
	if f.StartedAt.IsZero() || f.EndedAt.IsZero() {
		return 0
	}
	return f.EndedAt.Sub(f.StartedAt)
}
