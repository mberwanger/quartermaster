package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/doc"
	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/plan"
	"github.com/mberwanger/quartermaster/internal/repo"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/usage"
)

type usageCmd struct {
	cmd *cobra.Command
}

func newUsageCmd() *usageCmd {
	v := &usageCmd{}

	cmd := &cobra.Command{
		Use:    "usage",
		Short:  "Record which documents an agent opened",
		Hidden: true, // a machine entry point for harness hooks, not a human one
	}

	cmd.AddCommand(newUsageRecordCmd())

	v.cmd = cmd
	return v
}

func newUsageRecordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "record [path...]",
		Short: "Record that a knowledge document was opened",
		Long: `Record that a knowledge document was opened.

Paths may be given as arguments or as a hook payload on stdin. Anything outside
a repository's knowledge tree is ignored.

This runs inside an agent harness hook on every file read, so it never fails
loudly and never blocks: a repository that has opted out of telemetry, a path
that is not knowledge, and an unreadable log are all silent no-ops. Breaking
someone's editor to record a statistic would be a poor trade.`,
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = pathsFromStdin(cmd.InOrStdin())
			}
			for _, p := range paths {
				record(p)
			}
			return nil
		},
	}
}

// pathsFromStdin reads a harness hook payload and pulls a file path out of it.
// The shape differs by harness, so both the nested and the flat spelling are
// accepted and anything unrecognised yields nothing.
func pathsFromStdin(r io.Reader) []string {
	raw, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil || len(raw) == 0 {
		return nil
	}

	var payload struct {
		ToolInput struct {
			FilePath string `json:"file_path"`
		} `json:"tool_input"`
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	for _, p := range []string{payload.ToolInput.FilePath, payload.FilePath} {
		if p != "" {
			return []string{p}
		}
	}
	return nil
}

// record appends one open event, giving up quietly at the first sign this is not
// a knowledge document in an opted-in repository.
func record(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}

	root, ok := repoRootFor(abs)
	if !ok {
		return
	}

	// Only reads under the knowledge tree are usage; a rule file is loaded by
	// the harness rather than chosen by the agent, so reading one says nothing.
	knowledge := filepath.Join(root, filepath.FromSlash(plan.KnowledgeDir))
	if !strings.HasPrefix(abs, knowledge+string(filepath.Separator)) {
		return
	}

	m, err := manifest.Load(root)
	if err != nil || !m.Telemetry {
		return
	}

	// The id is in the document's own frontmatter, so no separate index has to
	// be materialized or kept in step.
	body, err := os.ReadFile(abs) //nolint:gosec // path is inside the repository's knowledge tree
	if err != nil {
		return
	}
	fm, err := doc.Parse(body)
	if err != nil || fm == nil {
		return
	}
	id, _ := fm["id"].(string)
	if id == "" {
		return
	}

	// Every installed bundle, not the first: a repository composes sources in
	// precedence order and the state file does not say which one carried this
	// document, so naming one of them would be a guess recorded as a fact.
	var bundles []usage.Bundle
	if s, err := state.Load(root); err == nil {
		for _, b := range s.Bundles {
			bundles = append(bundles, usage.Bundle{Source: b.Source, Digest: b.Digest})
		}
	}

	_ = usage.Append(usage.Event{
		ID:       id,
		Repo:     repo.Identity(root),
		Worktree: root,
		Bundles:  bundles,
		Time:     time.Now().UTC(),
		Event:    usage.EventOpen,
	})
}

// repoRootFor walks up from a file path to the repository that declares a
// manifest.
func repoRootFor(path string) (string, bool) {
	return repoRootAt(filepath.Dir(path))
}

// repoRootAt walks up from a directory to the repository that declares a
// manifest. A session runs somewhere below the root rather than at it, so the
// walk is what finds the repository the work happened in.
func repoRootAt(dir string) (string, bool) {
	for {
		if _, err := os.Stat(filepath.Join(dir, manifest.FileName)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
