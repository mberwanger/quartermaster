package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

// A transcript in the shape the harness actually writes, reduced to the fields
// this package reads.
const sample = `{"type":"user","sessionId":"s1","cwd":"/repo","gitBranch":"master","timestamp":"2026-07-24T12:00:00.000Z","message":{"role":"user","content":"why does the envelope keep the payload"}}
{"type":"assistant","sessionId":"s1","cwd":"/repo","gitBranch":"master","timestamp":"2026-07-24T12:00:05.000Z","message":{"model":"claude-opus-5","content":[{"type":"thinking","thinking":"I need to find the envelope type"},{"type":"tool_use","name":"Grep","input":{"pattern":"Envelope"}}]}}
{"type":"user","sessionId":"s1","timestamp":"2026-07-24T12:00:06.000Z","message":{"content":[{"type":"tool_result","content":"internal/events/envelope.go"}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-07-24T12:00:10.000Z","message":{"model":"claude-opus-5","content":[{"type":"text","text":"Found it."},{"type":"tool_use","name":"Read","input":{"file_path":"/repo/internal/events/envelope.go"}}]}}
{"type":"assistant","sessionId":"s1","isSidechain":true,"timestamp":"2026-07-24T12:00:12.000Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/delegated.go"}}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-07-24T12:00:20.000Z","message":{"model":"claude-opus-5","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}}
{"type":"file-history-snapshot","sessionId":"s1","timestamp":"2026-07-24T12:00:21.000Z"}
{"type":"assistant","sessionId":"s1","timestamp":"2026-07-24T12:00:30.000Z","message":{"model":"claude-opus-5","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/internal/events/envelope.go"}}]}}
`

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s1.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRead(t *testing.T) {
	s, err := Read(write(t, sample))
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "s1" || s.CWD != "/repo" || s.Branch != "master" {
		t.Errorf("session metadata wrong: %+v", s)
	}
	if s.Model != "claude-opus-5" {
		t.Errorf("model = %q", s.Model)
	}
	if s.Started.IsZero() || s.Ended.IsZero() || !s.Ended.After(s.Started) {
		t.Errorf("times wrong: %v to %v", s.Started, s.Ended)
	}

	if got := s.ToolCalls(); got != 4 {
		t.Errorf("tool calls = %d, want 4 (the sidechain call excluded)", got)
	}

	var tools []string
	var prompts, reasoning int
	for _, e := range s.Events {
		switch e.Kind {
		case KindToolUse:
			tools = append(tools, e.Tool)
		case KindPrompt:
			prompts++
		case KindReasoning:
			reasoning++
		}
	}
	if prompts != 1 {
		t.Errorf("prompts = %d, want 1", prompts)
	}
	if reasoning != 1 {
		t.Errorf("reasoning spans = %d, want 1", reasoning)
	}
	want := []string{"Grep", "Read", "Bash", "Edit"}
	for i, w := range want {
		if i >= len(tools) || tools[i] != w {
			t.Fatalf("tools = %v, want %v", tools, want)
		}
	}
}

// A subagent's exploration is its own session's business. Counting it against
// the parent would inflate every span for work that was deliberately delegated.
func TestSidechainTurnsAreExcluded(t *testing.T) {
	s, err := Read(write(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range s.Events {
		if e.Path == "/repo/delegated.go" {
			t.Fatal("a sidechain tool call reached the session")
		}
	}
}

// The format belongs to the harness and changes without notice. One line it
// cannot read must cost that line.
func TestUnreadableLinesCostOneLine(t *testing.T) {
	body := `{"type":"assistant","sessionId":"s1","timestamp":"2026-07-24T12:00:00.000Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/a"}}]}}
{"type":"assistant","sessionId":"s1","timestamp":"2026-07-2
{"type":"assistant","sessionId":"s1","timestamp":"2026-07-24T12:00:02.000Z","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/b"}}]}}
`
	s, err := Read(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ToolCalls(); got != 2 {
		t.Errorf("tool calls = %d, want 2 (the torn line skipped)", got)
	}
}

// A shape that has drifted past recognition yields an empty session rather than
// an error, so a failed digest is distinguishable from a quiet one by the
// caller rather than by a panic here.
func TestUnrecognizedShapeYieldsNoEvents(t *testing.T) {
	s, err := Read(write(t, "{\"type\":\"assistant\",\"message\":{\"content\":{\"unexpected\":true}}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Events) != 0 {
		t.Errorf("got %d events from an unreadable shape", len(s.Events))
	}
}

// Deciding whether a transcript belongs to this repository must not cost
// reading it, and the harness's directory name cannot answer it.
func TestHead(t *testing.T) {
	id, cwd, err := Head(write(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if id != "s1" || cwd != "/repo" {
		t.Errorf("head = %q, %q; want s1, /repo", id, cwd)
	}
}

// A session that opens with harness-internal entries still has to be placed.
func TestHeadSkipsLeadingEntriesWithoutCwd(t *testing.T) {
	body := `{"type":"permission-mode","sessionId":"s1"}
{"type":"mode","sessionId":"s1"}
{"type":"user","sessionId":"s1","cwd":"/repo","timestamp":"2026-07-24T12:00:00.000Z","message":{"content":"hi"}}
`
	id, cwd, err := Head(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if id != "s1" || cwd != "/repo" {
		t.Errorf("head = %q, %q; want s1, /repo", id, cwd)
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Read(filepath.Join(t.TempDir(), "nope.jsonl")); err == nil {
		t.Fatal("a missing transcript should be an error")
	}
}
