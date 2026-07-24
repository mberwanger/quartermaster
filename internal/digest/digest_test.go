package digest

import (
	"testing"
	"time"

	"github.com/mberwanger/quartermaster/internal/facet"
	"github.com/mberwanger/quartermaster/internal/transcript"
)

func at(minute int) time.Time {
	return time.Date(2026, 7, 24, 12, minute, 0, 0, time.UTC)
}

func tool(name, path, command string, minute int) transcript.Event {
	return transcript.Event{Kind: transcript.KindToolUse, Tool: name, Path: path, Command: command, At: at(minute)}
}

func session(events ...transcript.Event) *transcript.Session {
	return &transcript.Session{ID: "s1", Events: events, Started: at(0), Ended: at(30)}
}

// The primary metric: how much looking happened before anything landed.
func TestDiscoverySpanCountsSearchBeforeTheFirstEdit(t *testing.T) {
	s := session(
		tool("Read", "/repo/main.go", "", 1),
		tool("Grep", "", "", 2),
		tool("Bash", "", "go test ./...", 3),
		tool("Edit", "/repo/main.go", "", 4),
		tool("Read", "/repo/other.go", "", 5), // after the edit, not discovery
	)

	f := Digest(s, Repo{})
	if f.DiscoverySpan == nil {
		t.Fatal("no span computed for a session that edited")
	}
	if *f.DiscoverySpan != 3 {
		t.Errorf("span = %d, want 3", *f.DiscoverySpan)
	}
}

// A session that read code to answer a question was not flailing, and scoring it
// as the worst possible session would invert the metric.
func TestDiscoverySpanAbsentWhenNothingWasEdited(t *testing.T) {
	f := Digest(session(
		tool("Read", "/repo/main.go", "", 1),
		tool("Grep", "", "", 2),
	), Repo{})

	if f.DiscoverySpan != nil {
		t.Errorf("span = %d, want none for a session that edited nothing", *f.DiscoverySpan)
	}
	if f.Outcome != facet.OutcomeExplored {
		t.Errorf("outcome = %q, want explored", f.Outcome)
	}
}

func TestStoreReadsAreFoundAndAttributed(t *testing.T) {
	f := Digest(session(
		tool("Read", "/repo/.quartermaster/knowledge/index.md", "", 1),
		tool("Read", "/repo/.quartermaster/knowledge/voice/base.md", "", 2),
		tool("Edit", "/repo/main.go", "", 3),
	), Repo{})

	if len(f.StoreReads) != 1 {
		t.Fatalf("got %d store reads, want 1 (the index is navigation, not an answer)", len(f.StoreReads))
	}
	r := f.StoreReads[0]
	if r.Path != "voice/base.md" {
		t.Errorf("path = %q, want it relative to the knowledge tree", r.Path)
	}
	if r.ArrivedVia != facet.ArrivedViaIndex {
		t.Errorf("arrived_via = %q, want index", r.ArrivedVia)
	}
	if r.FollowedByExploration {
		t.Error("the agent edited straight after reading, so nothing followed it")
	}
}

func TestStoreReadDirectAndFollowedByExploration(t *testing.T) {
	f := Digest(session(
		tool("Read", "/repo/.quartermaster/knowledge/voice/base.md", "", 1),
		tool("Grep", "", "", 2),
		tool("Read", "/repo/main.go", "", 3),
		tool("Edit", "/repo/main.go", "", 4),
	), Repo{})

	if len(f.StoreReads) != 1 {
		t.Fatalf("got %d store reads, want 1", len(f.StoreReads))
	}
	if f.StoreReads[0].ArrivedVia != facet.ArrivedViaDirect {
		t.Error("no index was read first, so it was reached directly")
	}
	// The failed-document signal: consulted, and the search carried on.
	if !f.StoreReads[0].FollowedByExploration {
		t.Error("exploration continued after the read and was not recorded")
	}
}

// A read outside a knowledge tree is ordinary work, not a store read.
func TestReadsOutsideTheKnowledgeTreeAreNotStoreReads(t *testing.T) {
	f := Digest(session(
		tool("Read", "/repo/internal/doc/doc.go", "", 1),
		tool("Read", "/repo/docs/knowledge.md", "", 2),
	), Repo{})

	if len(f.StoreReads) != 0 {
		t.Errorf("got %d store reads, want none: %+v", len(f.StoreReads), f.StoreReads)
	}
}

func TestOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		events []transcript.Event
		want   facet.Outcome
	}{
		{"pr", []transcript.Event{tool("Bash", "", "gh pr create --title x", 1)}, facet.OutcomePROpened},
		{"commit", []transcript.Event{tool("Edit", "/f", "", 1), tool("Bash", "", "git commit -m x", 2)}, facet.OutcomeCommitted},
		{"edit", []transcript.Event{tool("Edit", "/f", "", 1)}, facet.OutcomeEditLanded},
		{"explore", []transcript.Event{tool("Grep", "", "", 1)}, facet.OutcomeExplored},
		{"nothing", nil, facet.OutcomeUnknown},
	}

	for _, c := range cases {
		if got := Digest(session(c.events...), Repo{}).Outcome; got != c.want {
			t.Errorf("%s: outcome = %q, want %q", c.name, got, c.want)
		}
	}
}

// Searching for a commit is not making one. Without this the corpus would record
// outcomes for sessions that only ever looked at history.
func TestMentioningACommandIsNotRunningIt(t *testing.T) {
	f := Digest(session(
		tool("Bash", "", `grep -rn "git commit" internal/`, 1),
		tool("Bash", "", `echo "run gh pr create when ready"`, 2),
	), Repo{})

	if f.Outcome != facet.OutcomeExplored {
		t.Errorf("outcome = %q, want explored: a grep for a command is not the command", f.Outcome)
	}
}

func TestRepoIdentityAndBundlesAreCarried(t *testing.T) {
	f := Digest(session(tool("Edit", "/f", "", 1)), Repo{
		Identity: "github.com/admiral/api",
		Worktree: "/Users/x/api",
		Bundles:  []facet.Bundle{{Source: "oci://x", Digest: "sha256:a"}},
	})

	if f.Repo != "github.com/admiral/api" || f.Worktree != "/Users/x/api" {
		t.Errorf("identity not carried: %+v", f)
	}
	if len(f.Bundles) != 1 || f.Bundles[0].Digest != "sha256:a" {
		t.Errorf("bundles not carried: %+v", f.Bundles)
	}
	if f.Source != facet.SourceStructural {
		t.Errorf("source = %q, want a record to say how it was made", f.Source)
	}
}
