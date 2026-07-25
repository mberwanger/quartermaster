// Package transcript reads the record a harness writes for an agent session.
//
// The format belongs to the harness and changes without notice, so this package
// is the one place that knows its shape. It reads the few fields the analysis
// needs and ignores everything else, and a line it cannot parse costs that line
// rather than the session. A transcript that has drifted too far to read yields
// a session with no events rather than an error, because a failed digest must
// not be indistinguishable from a session where nothing happened.
//
// Nothing here leaves the machine. The transcript holds prompts, file contents,
// pasted credentials, and candid remarks about people; what is derived from it
// is counts and paths.
package transcript

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Kind classifies an entry. Only what the analysis distinguishes is modelled.
type Kind string

const (
	// KindPrompt is a person typing. It bounds a turn and is otherwise not
	// interesting: what an agent did about a request says more than the request.
	KindPrompt Kind = "prompt"
	// KindToolUse is the agent reaching for something. This is the signal.
	KindToolUse Kind = "tool_use"
	// KindReasoning is the agent's own account of what it is trying to
	// establish. An excellent candidate generator and not evidence, since it is
	// a rationalization rather than a cause.
	KindReasoning Kind = "reasoning"
)

// Event is one thing that happened, in order.
type Event struct {
	Kind Kind
	At   time.Time
	// Tool is the tool name for KindToolUse.
	Tool string
	// Path is the file a tool acted on, when it named one.
	Path string
	// Command is the shell command a tool ran, when it ran one.
	Command string
	// Text is the prompt or reasoning body.
	Text string
	// Asked is what the agent put to a person, when the tool was the one that
	// stops and asks. A session that had to ask is a session where neither the
	// code nor the store could answer, and the transcript says so outright
	// rather than by inference.
	Asked []string
}

// Session is one agent session, normalized.
type Session struct {
	ID      string
	CWD     string
	Branch  string
	Model   string
	Harness string
	Started time.Time
	Ended   time.Time
	Events  []Event
}

// entry is the subset of a transcript line this package reads. Everything not
// named here is ignored by construction rather than by choice, which is what
// keeps a format change from breaking the parse.
type entry struct {
	Type        string          `json:"type"`
	SessionID   string          `json:"sessionId"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Timestamp   string          `json:"timestamp"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type     string          `json:"type"`
	Name     string          `json:"name"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Input    json.RawMessage `json:"input"`
}

type toolInput struct {
	FilePath  string `json:"file_path"`
	Path      string `json:"path"`
	Command   string `json:"command"`
	Pattern   string `json:"pattern"`
	Questions []struct {
		Question string `json:"question"`
	} `json:"questions"`
}

// Read parses the transcript at path.
//
// Sidechain entries are skipped. Those are subagent turns, and a subagent's
// exploration is its own session's business: counting it against the parent
// would inflate every span for work that was deliberately delegated.
func Read(path string) (*Session, error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the harness's own spool or project directory
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	s := &Session{Harness: "claude-code"}

	sc := bufio.NewScanner(f)
	// Transcript lines carry whole file contents and whole tool results, so the
	// default limit is far too small.
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // one unreadable line, not one unreadable session
		}
		if e.IsSidechain {
			continue
		}

		if s.ID == "" {
			s.ID = e.SessionID
		}
		if e.CWD != "" {
			s.CWD = e.CWD
		}
		if e.GitBranch != "" {
			s.Branch = e.GitBranch
		}

		at := parseTime(e.Timestamp)
		if !at.IsZero() {
			if s.Started.IsZero() {
				s.Started = at
			}
			s.Ended = at
		}

		if e.Type != "assistant" && e.Type != "user" {
			continue
		}
		s.Events = append(s.Events, events(e, at, s)...)
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// events turns one transcript entry into the events it contains.
func events(e entry, at time.Time, s *Session) []Event {
	var m message
	if err := json.Unmarshal(e.Message, &m); err != nil {
		return nil
	}
	if m.Model != "" && m.Model != "<synthetic>" {
		s.Model = m.Model
	}

	// A prompt's content is a bare string; everything else is a block list.
	var text string
	if err := json.Unmarshal(m.Content, &text); err == nil {
		if e.Type == "user" && strings.TrimSpace(text) != "" {
			return []Event{{Kind: KindPrompt, At: at, Text: text}}
		}
		return nil
	}

	var blocks []block
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}

	var out []Event
	for _, b := range blocks {
		switch b.Type {
		case "tool_use":
			var in toolInput
			_ = json.Unmarshal(b.Input, &in)
			path := in.FilePath
			if path == "" {
				path = in.Path
			}
			var asked []string
			for _, q := range in.Questions {
				if q.Question != "" {
					asked = append(asked, q.Question)
				}
			}

			out = append(out, Event{
				Kind:    KindToolUse,
				At:      at,
				Tool:    b.Name,
				Path:    path,
				Command: in.Command,
				Asked:   asked,
			})
		case "thinking":
			if b.Thinking != "" {
				out = append(out, Event{Kind: KindReasoning, At: at, Text: b.Thinking})
			}
		case "text":
			// Assistant prose is neither a request nor an action. It is skipped
			// on purpose: it narrates work that the tool calls already record.
			continue
		}
	}
	return out
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// Head reads only what a transcript says about where it ran: its session id and
// its working directory.
//
// Deciding whether a transcript is worth digesting should not cost reading it.
// The harness names its project directories after a mangled working path, so
// the name cannot be trusted to say which repository a session belonged to, but
// the first line of the file can.
func Head(path string) (id, cwd string, err error) {
	f, err := os.Open(path) //nolint:gosec // the path comes from the harness's own project directory
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)

	// The first line usually carries both, but a session that opens with a
	// harness-internal entry may not, so read until they are known or the
	// interesting part of the file is past.
	for i := 0; sc.Scan() && i < 50; i++ {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if id == "" {
			id = e.SessionID
		}
		if cwd == "" {
			cwd = e.CWD
		}
		if id != "" && cwd != "" {
			break
		}
	}
	return id, cwd, sc.Err()
}

// ToolCalls counts the tool-use events.
func (s *Session) ToolCalls() int {
	var n int
	for _, e := range s.Events {
		if e.Kind == KindToolUse {
			n++
		}
	}
	return n
}
