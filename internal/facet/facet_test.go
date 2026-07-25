package facet

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func at(minute int) time.Time {
	return time.Date(2026, 7, 24, 12, minute, 0, 0, time.UTC)
}

func TestSaveAndLoad(t *testing.T) {
	t.Setenv("QM_FACET_DIR", t.TempDir())

	span := 14
	want := Facet{
		Session:       "s1",
		Repo:          "github.com/admiral/api",
		Branch:        "feature/x",
		DiscoverySpan: &span,
		Bundles:       []Bundle{{Source: "oci://x", Digest: "sha256:a"}},
		StoreReads:    []StoreRead{{ID: "voice.base", Path: "voice/base.md", ArrivedVia: ArrivedViaIndex}},
		Outcome:       OutcomeCommitted,
		EndedAt:       at(30),
		Source:        SourceStructural,
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}

	got, err := Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != Version {
		t.Errorf("version = %d, want it stamped", got.Version)
	}
	if got.Repo != want.Repo || got.Outcome != want.Outcome {
		t.Errorf("round trip lost fields: %+v", got)
	}
	if got.DiscoverySpan == nil || *got.DiscoverySpan != span {
		t.Errorf("discovery span lost: %+v", got.DiscoverySpan)
	}
	if len(got.Bundles) != 1 || len(got.StoreReads) != 1 {
		t.Errorf("nested records lost: %+v", got)
	}
}

// Digesting is idempotent by session, so a re-run corrects rather than
// accumulates.
func TestSaveReplacesTheSameSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QM_FACET_DIR", dir)

	if err := Save(Facet{Session: "s1", Outcome: OutcomeExplored}); err != nil {
		t.Fatal(err)
	}
	if err := Save(Facet{Session: "s1", Outcome: OutcomeCommitted}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("wrote %d records for one session", len(entries))
	}

	got, err := Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != OutcomeCommitted {
		t.Errorf("outcome = %q, want the later record", got.Outcome)
	}
}

func TestHave(t *testing.T) {
	t.Setenv("QM_FACET_DIR", t.TempDir())
	if Have("s1") {
		t.Error("reported a record that was never written")
	}
	if err := Save(Facet{Session: "s1"}); err != nil {
		t.Fatal(err)
	}
	if !Have("s1") {
		t.Error("did not report a record that exists")
	}
}

// Session ids come from a harness, not from us, so they are not trusted to be
// filenames.
func TestSessionIdCannotEscapeTheDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QM_FACET_DIR", dir)

	if err := Save(Facet{Session: "../../escaped"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(dir)), "escaped.json")); err == nil {
		t.Fatal("wrote outside the facet directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected the record to land inside the directory, got %v", entries)
	}
}

func TestLoadAllOrdersByEndAndSurvivesJunk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QM_FACET_DIR", dir)

	for _, f := range []Facet{
		{Session: "late", EndedAt: at(30)},
		{Session: "early", EndedAt: at(10)},
	} {
		if err := Save(f); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	all, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("loaded %d records, want 2 (the broken one skipped)", len(all))
	}
	if all[0].Session != "early" {
		t.Errorf("records are not oldest first: %+v", all)
	}
}

func TestAnnotate(t *testing.T) {
	span := 9
	f := Facet{Session: "s1", Outcome: OutcomeCommitted, DiscoverySpan: &span, Source: SourceStructural}

	err := f.Annotate([]Question{
		{Question: "why does the envelope keep the payload", Resolution: ResolutionSourceRead, Resolved: true, ToolCalls: 14},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != SourceModel {
		t.Errorf("source = %q, want the record to say it had help", f.Source)
	}
	// What the transcript showed is not a model's to revise.
	if f.Outcome != OutcomeCommitted || f.DiscoverySpan == nil || *f.DiscoverySpan != span {
		t.Errorf("annotating changed a structural field: %+v", f)
	}
}

// Re-annotating corrects a session rather than accumulating near-duplicates.
func TestAnnotateReplaces(t *testing.T) {
	var f Facet
	first := []Question{{Question: "a", Resolution: ResolutionUnresolved}}
	second := []Question{{Question: "b", Resolution: ResolutionStoreRead}}

	if err := f.Annotate(first); err != nil {
		t.Fatal(err)
	}
	if err := f.Annotate(second); err != nil {
		t.Fatal(err)
	}
	if len(f.Questions) != 1 || f.Questions[0].Question != "b" {
		t.Errorf("questions = %+v, want only the later set", f.Questions)
	}
}

// An open resolution set cannot be clustered, so the closed one is enforced
// here rather than trusted upstream.
func TestAnnotateRejectsMalformedQuestions(t *testing.T) {
	cases := map[string][]Question{
		"unknown resolution": {{Question: "x", Resolution: "vibes"}},
		"empty question":     {{Question: "  ", Resolution: ResolutionStoreRead}},
		"negative calls":     {{Question: "x", Resolution: ResolutionStoreRead, ToolCalls: -1}},
	}

	for name, questions := range cases {
		var f Facet
		if err := f.Annotate(questions); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if f.Source == SourceModel {
			t.Errorf("%s: marked the record annotated despite refusing it", name)
		}
	}
}

// A session that genuinely established nothing is a legitimate record, and
// saying so is better than a plausible fabrication.
func TestAnnotateWithNoQuestions(t *testing.T) {
	var f Facet
	if err := f.Annotate(nil); err != nil {
		t.Fatal(err)
	}
	if f.Source != SourceModel {
		t.Error("an empty extraction is still an extraction that happened")
	}
}

func TestLoadAllMissingDirIsEmpty(t *testing.T) {
	t.Setenv("QM_FACET_DIR", filepath.Join(t.TempDir(), "never-written"))
	all, err := LoadAll()
	if err != nil {
		t.Fatalf("a missing corpus should not be an error: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("got %d records", len(all))
	}
}
