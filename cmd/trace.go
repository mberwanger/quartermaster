package cmd

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/repo"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/trace"
)

type traceCmd struct {
	cmd *cobra.Command
}

func newTraceCmd() *traceCmd {
	v := &traceCmd{}

	cmd := &cobra.Command{
		Use:    "trace",
		Short:  "Record that an agent session happened",
		Hidden: true, // a machine entry point for harness hooks, not a human one
	}

	cmd.AddCommand(newTraceRecordCmd())

	v.cmd = cmd
	return v
}

func newTraceRecordCmd() *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Spool a pointer to a finished session",
		Long: `Spool a pointer to a finished session.

Reads a session hook payload on stdin and appends one line naming the session,
its transcript, the repository, and the bundles installed there. It records no
prompt and no file content: the transcript already holds those and stays on this
machine, and what the transcript cannot reconstruct is which bundles were
installed when the session ran.

This runs as a harness hook when a session ends, so it never fails loudly and
never blocks. A repository that has opted out of telemetry, a payload it does not
recognise, and an unwritable spool are all silent no-ops. Breaking somebody's
session to record a statistic would be a poor trade.`,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			recordSession(cmd.InOrStdin(), dir)
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Repository the session ran in (default: the payload's cwd, else the working directory)")
	return cmd
}

// sessionPayload is the part of a harness session hook envelope this needs.
// Payload shapes differ by harness and change often, so this reads the few
// fields it can use and treats anything else as absent rather than as an error.
type sessionPayload struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	CWD            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
}

// recordSession appends one session record, giving up quietly at the first sign
// this is not a session in an opted-in repository.
func recordSession(stdin io.Reader, dir string) {
	p, ok := sessionFromStdin(stdin)
	if !ok {
		return
	}

	root, ok := sessionRoot(dir, p.CWD)
	if !ok {
		return
	}

	m, err := manifest.Load(root)
	if err != nil || !m.Telemetry {
		return
	}

	var bundles []trace.Bundle
	if s, err := state.Load(root); err == nil {
		for _, b := range s.Bundles {
			bundles = append(bundles, trace.Bundle{Source: b.Source, Digest: b.Digest})
		}
	}

	_ = trace.Append(trace.Session{
		ID:             p.SessionID,
		Harness:        harnessOf(p),
		TranscriptPath: p.TranscriptPath,
		Repo:           repo.Identity(root),
		Worktree:       root,
		Branch:         repo.Branch(root),
		Bundles:        bundles,
		EndedAt:        time.Now().UTC(),
	})
}

// sessionFromStdin reads the hook payload. A session with no id cannot be joined
// to anything later, so it is not worth recording.
func sessionFromStdin(r io.Reader) (sessionPayload, bool) {
	raw, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil || len(raw) == 0 {
		return sessionPayload{}, false
	}

	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return sessionPayload{}, false
	}
	return p, p.SessionID != ""
}

// harnessOf names the tool that wrote the payload. Only one harness delivers
// this shape today, and the field exists so that a second one is a new value
// rather than a silent mixing of two transcript formats under one name.
func harnessOf(p sessionPayload) string {
	if p.TranscriptPath == "" && p.HookEventName == "" {
		return ""
	}
	return "claude-code"
}

// sessionRoot resolves the repository the session ran in: the flag, else the
// payload's own cwd, else the process working directory. It walks up to the
// directory that declares a manifest, since a session usually runs somewhere
// below the repository root rather than at it.
func sessionRoot(dir, cwd string) (string, bool) {
	for _, candidate := range []string{dir, cwd} {
		if candidate == "" {
			continue
		}
		if root, ok := repoRootAt(candidate); ok {
			return root, true
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	return repoRootAt(wd)
}
