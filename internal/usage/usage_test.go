package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func at(day int) time.Time {
	return time.Date(2026, 7, day, 12, 0, 0, 0, time.UTC)
}

func TestAppendAndLoad(t *testing.T) {
	t.Setenv("QM_USAGE_DIR", t.TempDir())

	events := []Event{
		{ID: "a", Repo: "/r1", Time: at(1), Event: EventOpen},
		{ID: "b", Repo: "/r2", Time: at(2), Event: EventOpen},
		{ID: "a", Repo: "/r2", Time: at(3), Event: EventOpen},
	}
	for _, e := range events {
		if err := Append(e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("loaded %d events, want 3", len(got))
	}
	// Oldest first.
	if !got[0].Time.Equal(at(1)) || !got[2].Time.Equal(at(3)) {
		t.Fatalf("events are not in time order: %+v", got)
	}
}

func TestLoadMissingDirIsEmpty(t *testing.T) {
	t.Setenv("QM_USAGE_DIR", filepath.Join(t.TempDir(), "never-written"))
	got, err := Load()
	if err != nil {
		t.Fatalf("a missing log should not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d events, want none", len(got))
	}
}

// A hook interrupted mid-write leaves a torn line. That must cost one event, not
// the whole history.
func TestLoadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QM_USAGE_DIR", dir)

	body := `{"id":"a","repo":"/r1","ts":"2026-07-01T12:00:00Z","event":"open"}
{"id":"b","repo":"/r1","ts":"2026-07-0
{"id":"c","repo":"/r1","ts":"2026-07-02T12:00:00Z","event":"open"}
`
	if err := os.WriteFile(filepath.Join(dir, "2026-07.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d events, want 2 (the torn line skipped)", len(got))
	}
}

func TestAggregate(t *testing.T) {
	events := []Event{
		{ID: "a", Repo: "/r1", Time: at(1)},
		{ID: "a", Repo: "/r1", Time: at(2)},
		{ID: "a", Repo: "/r2", Time: at(3)},
		{ID: "b", Repo: "/r1", Time: at(1)},
	}

	docs := Aggregate(events)
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}

	// Sorted by opens, so a is first.
	if docs[0].ID != "a" || docs[0].Opens != 3 {
		t.Fatalf("docs[0] = %+v, want a with 3 opens", docs[0])
	}
	// Repeated opens in one repository count once toward the spread, which is
	// what makes the cross-repository signal meaningful.
	if len(docs[0].Repos) != 2 {
		t.Fatalf("a spread over %v, want 2 repositories", docs[0].Repos)
	}
	if !docs[0].Last.Equal(at(3)) {
		t.Fatalf("last = %v, want %v", docs[0].Last, at(3))
	}
}

func TestReposAndSince(t *testing.T) {
	events := []Event{
		{ID: "a", Repo: "/r1", Time: at(1)},
		{ID: "b", Repo: "/r2", Time: at(5)},
		{ID: "c", Repo: "", Time: at(6)},
	}

	if got := Repos(events); len(got) != 2 {
		t.Fatalf("repos = %v, want 2 (an empty repo is not one)", got)
	}
	if got := Since(events, at(5)); len(got) != 2 {
		t.Fatalf("since day 5 = %d events, want 2", len(got))
	}
}
