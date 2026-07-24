package trace

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
	t.Setenv("QM_TRACE_DIR", t.TempDir())

	for _, s := range []Session{
		{ID: "s2", Repo: "github.com/admiral/api", EndedAt: at(2)},
		{ID: "s1", Repo: "github.com/admiral/api", EndedAt: at(1)},
	} {
		if err := Append(s); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d sessions, want 2", len(got))
	}
	if got[0].ID != "s1" {
		t.Fatalf("sessions are not in time order: %+v", got)
	}
	// The version is stamped even when the caller does not set it, so a reader
	// can always tell what shape it is looking at.
	if got[0].Version != Version {
		t.Fatalf("version = %d, want %d", got[0].Version, Version)
	}
}

func TestLoadMissingSpoolIsEmpty(t *testing.T) {
	t.Setenv("QM_TRACE_DIR", filepath.Join(t.TempDir(), "never-written"))
	got, err := Load()
	if err != nil {
		t.Fatalf("a missing spool should not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d sessions, want none", len(got))
	}
}

// A hook interrupted mid-write leaves a torn line. That must cost one session,
// not every session recorded before it.
func TestLoadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QM_TRACE_DIR", dir)

	body := `{"v":1,"session_id":"a","repo":"r","ended_at":"2026-07-01T12:00:00Z"}
{"v":1,"session_id":"b","repo":"r","ended_at":"2026-07-0
{"v":1,"session_id":"c","repo":"r","ended_at":"2026-07-02T12:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d sessions, want 2 (the torn line skipped)", len(got))
	}
}

// A session recorded twice is a re-run of the hook, not two sessions. The later
// record wins, since it saw the repository last.
func TestLoadKeepsTheLatestRecordPerSession(t *testing.T) {
	t.Setenv("QM_TRACE_DIR", t.TempDir())

	for _, s := range []Session{
		{ID: "s1", Repo: "r", Branch: "old", EndedAt: at(1)},
		{ID: "s1", Repo: "r", Branch: "new", EndedAt: at(2)},
	} {
		if err := Append(s); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("loaded %d sessions, want 1", len(got))
	}
	if got[0].Branch != "new" {
		t.Fatalf("branch = %q, want the later record", got[0].Branch)
	}
}

// The bundle set is the whole reason this record exists: it is the one thing a
// transcript cannot reconstruct, so it has to survive the round trip whole.
func TestBundlesRoundTrip(t *testing.T) {
	t.Setenv("QM_TRACE_DIR", t.TempDir())

	want := []Bundle{
		{Source: "oci://ghcr.io/admiral/knowledge:v1", Digest: "sha256:aaa"},
		{Source: "file://../local", Digest: "sha256:bbb"},
	}
	if err := Append(Session{ID: "s1", Repo: "r", Bundles: want, EndedAt: at(1)}); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Bundles) != 2 {
		t.Fatalf("bundles lost: %+v", got)
	}
	for i, b := range got[0].Bundles {
		if b != want[i] {
			t.Errorf("bundle %d = %+v, want %+v", i, b, want[i])
		}
	}
}
