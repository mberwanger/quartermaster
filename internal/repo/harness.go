package repo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// claudeSettings is where Claude Code reads a repository's own settings. It is
// committed rather than ignored, which is what carries the hook to every
// worktree and every clone the same way the manifest travels.
var claudeSettings = filepath.Join(".claude", "settings.json")

// sessionHookEvent fires once when a session ends, while nobody is waiting.
const sessionHookEvent = "SessionEnd"

// sessionHookCommand records the session that just ended.
//
// The guard is the same one the git hooks use. This file is committed, so it
// reaches people who have never installed Quartermaster, and for them the hook
// has to be silently inert rather than an error at the end of every session.
const sessionHookCommand = "command -v qm >/dev/null 2>&1 && qm trace record"

// sessionHookMark identifies our hook inside a settings file somebody else also
// edits. Matching on the command rather than on a marker comment is what makes
// it work: JSON has no comments.
const sessionHookMark = "qm trace record"

// InstallSessionHook adds the session hook to the repository's Claude Code
// settings, and reports whether it wrote anything.
//
// It is idempotent and additive. An existing settings file keeps its contents,
// its key order, and any hooks somebody else configured, including other hooks
// on the same event.
func InstallSessionHook(dir string) (bool, error) {
	path := filepath.Join(dir, claudeSettings)

	raw, err := os.ReadFile(path) //nolint:gosec // path is inside the repository being initialized
	switch {
	case errors.Is(err, fs.ErrNotExist):
		raw = []byte("{}")
	case err != nil:
		return false, err
	}

	settings, err := parseObject(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", claudeSettings, err)
	}

	hooks, err := parseObject(settings.get("hooks", []byte("{}")))
	if err != nil {
		return false, fmt.Errorf("%s: hooks: %w", claudeSettings, err)
	}

	var entries []json.RawMessage
	if existing := hooks.get(sessionHookEvent, nil); existing != nil {
		if err := json.Unmarshal(existing, &entries); err != nil {
			return false, fmt.Errorf("%s: hooks.%s: %w", claudeSettings, sessionHookEvent, err)
		}
	}
	for _, e := range entries {
		if bytes.Contains(e, []byte(sessionHookMark)) {
			return false, nil
		}
	}

	entry, err := marshal(hookEntry{Hooks: []hookCommand{{Type: "command", Command: sessionHookCommand}}})
	if err != nil {
		return false, err
	}
	entries = append(entries, entry)

	encoded, err := marshal(entries)
	if err != nil {
		return false, err
	}
	hooks.set(sessionHookEvent, encoded)

	encodedHooks, err := hooks.marshal()
	if err != nil {
		return false, err
	}
	settings.set("hooks", encodedHooks)

	out, err := settings.marshal()
	if err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644) //nolint:gosec // settings are committed and read by other tools
}

// hookEntry and hookCommand shape one hook the way the harness documents it,
// rather than as a map, so the keys land in the order a person would write them.
type hookEntry struct {
	Hooks []hookCommand `json:"hooks"`
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// marshal encodes without escaping HTML, which the standard marshaller does by
// default. The hook command contains shell redirection, and a committed file
// full of > is unreadable to the people who have to review it.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// object is a JSON object that keeps the order its keys were written in, and
// keeps every value it did not have to change byte for byte.
//
// Settings files are committed and hand-edited. Round-tripping one through a Go
// map would reorder every key its author chose the order of, which turns adding
// one hook into a diff across the whole file.
type object struct {
	keys   []string
	values map[string]json.RawMessage
}

func parseObject(raw []byte) (*object, error) {
	o := &object{values: map[string]json.RawMessage{}}
	if len(bytes.TrimSpace(raw)) == 0 {
		return o, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("want a json object, got %v", tok)
	}

	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("want an object key, got %v", tok)
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		o.set(key, value)
	}

	if _, err := dec.Token(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return o, nil
}

func (o *object) get(key string, fallback json.RawMessage) json.RawMessage {
	if v, ok := o.values[key]; ok {
		return v
	}
	return fallback
}

func (o *object) set(key string, value json.RawMessage) {
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// marshal writes the object back, indented two spaces, in its original key
// order with new keys appended.
func (o *object) marshal() ([]byte, error) {
	if len(o.keys) == 0 {
		return []byte("{}"), nil
	}

	var b bytes.Buffer
	b.WriteString("{\n")
	for i, key := range o.keys {
		name, err := marshal(key)
		if err != nil {
			return nil, err
		}

		var value bytes.Buffer
		if err := json.Indent(&value, o.values[key], "  ", "  "); err != nil {
			return nil, err
		}

		fmt.Fprintf(&b, "  %s: %s", name, value.String())
		if i < len(o.keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}")

	return b.Bytes(), nil
}

// SessionHookIgnored reports whether git is ignoring the settings file the
// session hook lives in.
//
// The hook is written there because the file is committed, which is what carries
// it to every worktree and every clone the way the manifest travels. A
// repository that ignores .claude/ breaks that, and the hook then exists only in
// the checkout it was installed in, silently, which is the failure the placement
// was chosen to avoid. It is worth one line of warning at install time.
//
// git answers this rather than a pattern match here, because gitignore
// precedence has negations and nested files and is not worth reimplementing. No
// git, no answer, and no warning.
func SessionHookIgnored(dir string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", "--", claudeSettings)
	cmd.Dir = dir

	// Exit 0 means ignored. Every other outcome, including git not being here at
	// all, means no warning rather than a wrong one.
	return cmd.Run() == nil
}

// HasSessionHook reports whether the repository's settings already carry the
// session hook, so a status command can tell a worktree that is recording from
// one that only looks like it is.
func HasSessionHook(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, claudeSettings)) //nolint:gosec // path is inside the repository
	if err != nil {
		return false
	}
	return strings.Contains(string(raw), sessionHookMark)
}
